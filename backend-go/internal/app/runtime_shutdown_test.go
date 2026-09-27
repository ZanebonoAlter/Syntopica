package app

// 优雅关停白盒用例（change: graceful-shutdown-hardening）
//
// 契约 = design.md D8，分支表 = test-cases.md（TC-01~TC-24）。红线：
//   - 禁 DB：不碰 database.*、不调 tagging.StopAllWorkers 真身；fake scheduler 经
//     admin.NewSchedulerRegistry() + Register 注入 Runtime。
//   - 禁真实信号：signalChanFactory 注入假通道，不发 SIGTERM/SIGINT——唯一例外是
//     TestSetupGracefulShutdown_RealSignalDoubleFire（主线程指定：真实 SIGTERM 双发、
//     第二枚被丢弃），该用例禁 t.Parallel 且 t.Cleanup signal.Reset。
//   - 禁真实长等待：三个上限统一覆写为 ≤200ms，t.Cleanup 还原。
//   - 禁在子 goroutine 断言：子 goroutine 只回传结果（channel），Fatal 只在测试主 goroutine。

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/admin"
	"syntopica-backend/internal/platform/logging"
)

// ── 并发安全 fixture ──

// syncBuffer 是并发安全的日志捕获缓冲：关停序列内 workers 步骤在独立 goroutine 里
// 调用 stopWorkersFn 并可能写日志，与主 goroutine 的汇总行写入并发，故不能用裸 bytes.Buffer。
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// shutdownTestFixture 收敛用例间共享的可变全局状态（日志 writer + 4 个注入缝 + 3 个上限），
// 由 t.Cleanup 机械还原（INV-8 测试独立性）。
type shutdownTestFixture struct {
	log *syncBuffer

	origHTTP       time.Duration
	origRegistry   time.Duration
	origWorkers    time.Duration
	origStopWorker func()
	origStopReg    func(*Runtime, time.Duration)
	origSignalChan func() <-chan os.Signal
	origServeFn    func(*http.Server) error
	origHTTPStep   func(*http.Server) error
}

// newShutdownTestFixture 安装短上限（200ms）与安全默认 fake，并注册还原。
func newShutdownTestFixture(t *testing.T) *shutdownTestFixture {
	t.Helper()
	f := &shutdownTestFixture{
		log:            &syncBuffer{},
		origHTTP:       httpShutdownTimeout,
		origRegistry:   registryShutdownTimeout,
		origWorkers:    workersShutdownTimeout,
		origStopWorker: stopWorkersFn,
		origStopReg:    stopRegistryFn,
		origSignalChan: signalChanFactory,
		origServeFn:    listenAndServeFn,
		origHTTPStep:   shutdownHTTPStepFn,
	}

	logging.SetWriters(f.log, f.log)
	// 三个上限统一收敛到 200ms：单文件总时长目标 <15s（test-cases.md 禁项 3）。
	httpShutdownTimeout = 200 * time.Millisecond
	registryShutdownTimeout = 200 * time.Millisecond
	workersShutdownTimeout = 200 * time.Millisecond
	// 默认 workers fake（记录型、不碰真身）；需要自定义行为的用例覆写 recorder.fn。
	stopWorkersFn = func() {}

	t.Cleanup(f.restore)
	return f
}

func (f *shutdownTestFixture) restore() {
	logging.ResetWriters()
	httpShutdownTimeout = f.origHTTP
	registryShutdownTimeout = f.origRegistry
	workersShutdownTimeout = f.origWorkers
	stopWorkersFn = f.origStopWorker
	stopRegistryFn = f.origStopReg
	signalChanFactory = f.origSignalChan
	listenAndServeFn = f.origServeFn
	shutdownHTTPStepFn = f.origHTTPStep
}

// ── 判定辅助 ──

// countLines 返回 s 中包含 substr 的行数（汇总行唯一性/顺序断言用）。
func countLines(s, substr string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, substr) {
			n++
		}
	}
	return n
}

// asyncTimings 在独立 goroutine 跑同步序列，把结果回传主 goroutine。
func asyncTimings(fn func() shutdownTimings) <-chan shutdownTimings {
	ch := make(chan shutdownTimings, 1)
	go func() { ch <- fn() }()
	return ch
}

// boundedWait 按 B-5 口径（三档注入上限之和 + 1s）等待序列完成；超时即 Fatal（INV-3）。
func boundedWait(t *testing.T, ch <-chan shutdownTimings) shutdownTimings {
	t.Helper()
	limit := httpShutdownTimeout + registryShutdownTimeout + workersShutdownTimeout + time.Second
	select {
	case got := <-ch:
		return got
	case <-time.After(limit):
		t.Fatalf("shutdown sequence exceeded bound %v（INV-3: done 必关闭）", limit)
		return shutdownTimings{}
	}
}

// dialFails 返回对 addr 拨号是否失败（端口已摘除的证据，INV-1）。
func dialFails(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err == nil {
		_ = conn.Close()
		return false
	}
	return true
}

// newTestHTTPClient 关掉代理：本机代理环境变量不得干扰 127.0.0.1 拨号的确定性。
func newTestHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{Proxy: nil},
	}
}

// fireGet 在子 goroutine 发起 GET 并把结果回传（子 goroutine 不做断言）。
func fireGet(client *http.Client, url string) (<-chan *http.Response, <-chan error) {
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := client.Get(url)
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()
	return respCh, errCh
}

// ── 自建 listener / HTTP server ──

// recordingListener 包一层 net.Listener，把 srv.Shutdown 的「关 listener」动作变成可等待事件：
// Shutdown 的第一步就是关 listener（端口释放），用它做「摘端口先行」的确定性检查点。
type recordingListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *recordingListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}

// testHTTPServer 是自建 listener + http.Server 的统一 fixture（不用 httptest.Server：
// 需要真实 listener 才能观察摘端口时序与端口占用错误）。
type testHTTPServer struct {
	srv    *http.Server
	ln     *recordingListener
	addr   string
	closed <-chan struct{}
}

// newTestHTTPServer 只建 listener + srv（是否 Serve 由用例决定），并在 Cleanup 关闭。
func newTestHTTPServer(t *testing.T, handler http.Handler) *testHTTPServer {
	t.Helper()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ln := &recordingListener{Listener: raw, closed: make(chan struct{})}
	srv := &http.Server{Handler: handler}
	t.Cleanup(func() { _ = srv.Close() })
	return &testHTTPServer{srv: srv, ln: ln, addr: raw.Addr().String(), closed: ln.closed}
}

// startTestHTTPServer = newTestHTTPServer + 立即 Serve（大多数用例的形态）。
// startTestHTTPServer = newTestHTTPServer + 立即 Serve（大多数用例的形态）。
// 同步等 Serve 进 accept 循环再返回：Serve 注册 listener 前调 srv.Shutdown 对 server 是
// no-op（listener 不关、返回 nil），会让「端口已释放」断言随机误红，也会让 http 步骤失效
// 却仍全绿（覆盖空洞，review High-1）。就绪探针走 /__ready，不经过用例 handler（阻塞型
// handler 用例也能就绪）。
func startTestHTTPServer(t *testing.T, handler http.Handler) *testHTTPServer {
	t.Helper()
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == readyPath {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler.ServeHTTP(w, r)
	})
	ts := newTestHTTPServer(t, wrapped)
	go func() { _ = ts.srv.Serve(ts.ln) }()
	waitUntilServing(t, ts.addr)
	return ts
}

// readyPath 是就绪探针路径（不与任何用例 handler 的业务路径冲突）。
const readyPath = "/__ready"

// waitUntilServing 打通一次真实请求，证明 Serve 已进入 accept 循环
// （trackListener 已完成，Shutdown 一定关得到这个 listener）。
func waitUntilServing(t *testing.T, addr string) {
	t.Helper()
	client := newTestHTTPClient()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + readyPath)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server %s 未在 2s 内就绪", addr)
}

// okHandler 是无阻塞 200 handler，用于顺序/边界用例。
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// ── 注入替身 ──

// fakeScheduler 是 scheduler.Scheduler 的测试替身（经 admin.NewSchedulerRegistry() 注册，
// 替代真身——禁 DB 红线）：只关心 Stop 的计数/时刻/行为与端口拨号证据。
type fakeScheduler struct {
	mu              sync.Mutex
	stopCalls       int
	stopStartedAt   time.Time
	stopEndedAt     time.Time
	dialErr         error
	dialChecked     bool
	handlerFinished bool
	stopFn          func() // 锁外执行的自定义行为（拨号检查/阻塞）
}

func (f *fakeScheduler) Start() error                       { return nil }
func (f *fakeScheduler) TriggerNow() map[string]interface{} { return map[string]interface{}{} }
func (f *fakeScheduler) UpdateInterval(seconds int) error   { return nil }
func (f *fakeScheduler) ResetStats() error                  { return nil }

func (f *fakeScheduler) Stop() {
	f.mu.Lock()
	f.stopCalls++
	f.stopStartedAt = time.Now()
	fn := f.stopFn
	f.mu.Unlock()

	if fn != nil {
		fn()
	}

	f.mu.Lock()
	f.stopEndedAt = time.Now()
	f.mu.Unlock()
}

// dialCheck 记录「Stop 时刻对端口拨号」的证据（INV-1）。加锁存储：registry 卡死用例中
// 该写入可能晚于/并发于测试主 goroutine 的读取。
func (f *fakeScheduler) dialCheck(addr string, timeout time.Duration) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err == nil {
		_ = conn.Close()
	}
	f.mu.Lock()
	f.dialErr = err
	f.dialChecked = true
	f.mu.Unlock()
}

func (f *fakeScheduler) markHandlerFinished() {
	f.mu.Lock()
	f.handlerFinished = true
	f.mu.Unlock()
}

func (f *fakeScheduler) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

func (f *fakeScheduler) stopStartTime() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopStartedAt
}

func (f *fakeScheduler) stopEndTime() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopEndedAt
}

func (f *fakeScheduler) dialEvidence() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dialChecked, f.dialErr
}

func (f *fakeScheduler) sawHandlerFinished() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handlerFinished
}

// workersRecorder 是 stopWorkersFn 的记录型 fake（不调 tagging.StopAllWorkers 真身）。
type workersRecorder struct {
	mu      sync.Mutex
	calls   int
	started time.Time
	fn      func() // 锁外执行：阻塞/panic 分支用
}

func (w *workersRecorder) stop() {
	w.mu.Lock()
	w.calls++
	w.started = time.Now()
	fn := w.fn
	w.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (w *workersRecorder) callCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func (w *workersRecorder) startTime() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.started
}

// newFakeRuntime 用 admin.NewSchedulerRegistry() + Register("fake", …) 注入 Runtime（禁 DB 路径）。
func newFakeRuntime(fs *fakeScheduler) *Runtime {
	reg := admin.NewSchedulerRegistry()
	reg.Register("fake", fs)
	return &Runtime{Registry: reg}
}

// ── TC-01 顺序-正常路径 ──

// 端口先于 ② 释放（fake.Stop 内拨号失败即 INV-1 证据）、② 恰好 1 次、汇总行存在。
func TestShutdownSequence_PortReleasedBeforeRegistryStop(t *testing.T) {
	f := newShutdownTestFixture(t)
	ts := startTestHTTPServer(t, okHandler)

	fs := &fakeScheduler{}
	fs.stopFn = func() { fs.dialCheck(ts.addr, 100*time.Millisecond) }
	rt := newFakeRuntime(fs)

	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Equal(t, 1, fs.stopCount(), "② registry 步骤必须恰好执行一次")
	checked, err := fs.dialEvidence()
	require.True(t, checked, "fake.Stop 内必须执行端口拨号检查")
	require.Error(t, err, "Stop 时刻端口必须已释放（拨号失败）")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-02 顺序-在途阻塞期 ──

// Shutdown 尚未返回（② 未开始、在途请求未完成）时 listener 已关闭：
// 证明摘端口先行、排空在后。
func TestShutdownSequence_InFlightRequestDrainsBeforeRegistryStop(t *testing.T) {
	f := newShutdownTestFixture(t)

	started := make(chan struct{})
	release := make(chan struct{})
	ts := startTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("drained"))
	}))

	respCh, errCh := fireGet(newTestHTTPClient(), "http://"+ts.addr)
	<-started

	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	seq := asyncTimings(func() shutdownTimings { return runShutdownSequence(rt, ts.srv) })

	// 检查点：Shutdown 已关 listener（端口释放），但 handler 仍阻塞 → ② 不得开始。
	select {
	case <-ts.closed:
	case <-time.After(httpShutdownTimeout):
		t.Fatal("Shutdown 未在上限内关闭 listener（摘端口先行被破坏）")
	}
	require.True(t, dialFails(ts.addr, 100*time.Millisecond), "listener 关闭后拨号必须失败")
	require.Equal(t, 0, fs.stopCount(), "在途请求排空前不得进入 ② registry 步骤")
	select {
	case <-respCh:
		t.Fatal("在途请求不应在 release 前完成")
	case err := <-errCh:
		t.Fatalf("在途请求不应在 release 前失败: %v", err)
	default:
	}

	close(release)
	_ = boundedWait(t, seq)
	require.Equal(t, 1, fs.stopCount(), "② 必须恰好执行一次")

	select {
	case resp := <-respCh:
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	case err := <-errCh:
		t.Fatalf("排空后原请求应收到 200: %v", err)
	case <-time.After(time.Second):
		t.Fatal("排空后原请求应收到 200（超时）")
	}
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-03 排空-正常 ──

// 在途请求在上限内完成，且 ① Shutdown 等它完成才返回（fake.Stop 时刻 handlerFinished 已为 true）。
func TestShutdownSequence_DrainsInFlightRequest(t *testing.T) {
	f := newShutdownTestFixture(t)

	started := make(chan struct{})
	handlerDone := make(chan struct{})
	ts := startTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("drained"))
		close(handlerDone)
	}))

	fs := &fakeScheduler{}
	fs.stopFn = func() {
		select {
		case <-handlerDone:
			fs.markHandlerFinished()
		default:
		}
	}
	rt := newFakeRuntime(fs)

	respCh, errCh := fireGet(newTestHTTPClient(), "http://"+ts.addr)
	<-started

	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.True(t, fs.sawHandlerFinished(), "① Shutdown 必须等在途 handler 完成后再返回")
	select {
	case resp := <-respCh:
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "drained", string(body))
	case err := <-errCh:
		t.Fatalf("在途请求必须排空成功: %v", err)
	case <-time.After(time.Second):
		t.Fatal("在途请求必须排空成功（超时）")
	}
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-04 排空-超时 ──

// 在途慢请求超过注入上限：序列继续（②③ 与汇总照走），Shutdown 在 handler 结束前返回。
func TestShutdownSequence_DrainTimeoutContinues(t *testing.T) {
	f := newShutdownTestFixture(t)
	httpShutdownTimeout = 100 * time.Millisecond

	started := make(chan struct{})
	ts := startTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond) // 超过注入上限：Shutdown 必须放弃等待
		w.WriteHeader(http.StatusOK)
	}))

	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	_, _ = fireGet(newTestHTTPClient(), "http://"+ts.addr)
	<-started

	seqStart := time.Now()
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))
	elapsed := time.Since(seqStart)

	require.GreaterOrEqual(t, elapsed, 100*time.Millisecond, "Shutdown 必须等满注入上限")
	require.Less(t, elapsed, time.Second, "超时后必须继续后续步骤并快速结束")
	require.Equal(t, 1, fs.stopCount(), "② 必须照走")
	require.Equal(t, 1, rec.callCount(), "③ 必须照走")
	stopDelta := fs.stopStartTime().Sub(seqStart)
	require.GreaterOrEqual(t, stopDelta, 100*time.Millisecond)
	require.Less(t, stopDelta, 250*time.Millisecond, "Shutdown 必须在 handler 300ms 结束前返回")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行仍恰好一行")
}

// ── TC-05 边界：排空上限内（50ms < 100ms） ──

func TestShutdownSequence_DrainWithinLimit(t *testing.T) {
	f := newShutdownTestFixture(t)
	httpShutdownTimeout = 100 * time.Millisecond

	started := make(chan struct{})
	handlerDone := make(chan struct{})
	ts := startTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		close(handlerDone)
	}))

	fs := &fakeScheduler{}
	fs.stopFn = func() {
		select {
		case <-handlerDone:
			fs.markHandlerFinished()
		default:
		}
	}
	rt := newFakeRuntime(fs)

	respCh, _ := fireGet(newTestHTTPClient(), "http://"+ts.addr)
	<-started

	seqStart := time.Now()
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.True(t, fs.sawHandlerFinished(), "① 未提前返回（handler 已完成）")
	require.Less(t, time.Since(seqStart), 500*time.Millisecond)
	require.NotContains(t, f.log.String(), "deadline exceeded", "无排空超时告警")
	select {
	case resp := <-respCh:
		_ = resp.Body.Close()
	case <-time.After(time.Second):
		t.Fatal("排空成功的请求必须收到响应")
	}
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-06 边界：恰好等于上限（双结果接受，A5 唯一非全确定用例） ──

// 只断言：有界 + 序列继续 + 请求不悬挂；err 为 nil / deadline exceeded 均接受。
func TestShutdownSequence_DrainExactlyAtLimit(t *testing.T) {
	f := newShutdownTestFixture(t)
	httpShutdownTimeout = 100 * time.Millisecond

	started := make(chan struct{})
	ts := startTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	respCh, errCh := fireGet(newTestHTTPClient(), "http://"+ts.addr)
	<-started

	seqStart := time.Now()
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(nil, ts.srv)
	}))

	elapsed := time.Since(seqStart)
	require.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	require.Less(t, elapsed, time.Second)
	require.Equal(t, 1, rec.callCount(), "序列必须继续到 ③")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	// 请求不悬挂：收到 200 或连接被关（err）均接受。
	select {
	case resp := <-respCh:
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("请求悬挂（既非 200 也非连接关闭）")
	}
}

// ── TC-07 workers-卡死超上限 ──

// 有界 + 超时只告警不重试（调用计数 ==1）+ ② 整体先于 ③（INV-6/INV-7）。
func TestShutdownSequence_WorkersTimeoutBounded(t *testing.T) {
	f := newShutdownTestFixture(t)
	workersShutdownTimeout = 100 * time.Millisecond

	rec := &workersRecorder{fn: func() { select {} }} // 永久阻塞；禁项 6：不 Join 回收
	stopWorkersFn = rec.stop

	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	ts := startTestHTTPServer(t, okHandler)

	seqStart := time.Now()
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Equal(t, 1, rec.callCount(), "超时只告警不重试（INV-7）")
	require.Less(t, time.Since(seqStart), time.Second, "workers 卡死必须有界")
	require.False(t, fs.stopEndTime().IsZero(), "② 必须已完成（时间戳非零）")
	require.False(t, rec.startTime().Before(fs.stopEndTime()), "② 必须整体先于 ③（INV-6）")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	require.Contains(t, f.log.String(), "workers=", "汇总行含 workers 字段")
}

// ── TC-08 边界：workers 在上限内返回（50ms < 100ms） ──

func TestShutdownSequence_WorkersWithinLimit(t *testing.T) {
	f := newShutdownTestFixture(t)
	workersShutdownTimeout = 100 * time.Millisecond

	rec := &workersRecorder{fn: func() { time.Sleep(50 * time.Millisecond) }}
	stopWorkersFn = rec.stop

	rt := newFakeRuntime(&fakeScheduler{})
	ts := startTestHTTPServer(t, okHandler)

	seqStart := time.Now()
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Equal(t, 1, rec.callCount())
	require.Less(t, time.Since(seqStart), 500*time.Millisecond, "50ms workers 必须在上限内返回")
	require.NotContains(t, f.log.String(), "timed out", "无超时告警")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-09 panic-③ workers（步级隔离） ──

func TestShutdownSequence_WorkersPanicIsolated(t *testing.T) {
	f := newShutdownTestFixture(t)
	rec := &workersRecorder{fn: func() { panic("boom") }}
	stopWorkersFn = rec.stop
	rt := newFakeRuntime(&fakeScheduler{})
	ts := startTestHTTPServer(t, okHandler)

	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Equal(t, 1, rec.callCount())
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "panic 后汇总行照走")
	require.Contains(t, f.log.String(), "level=WARN", "panic 必须留下 warn 日志（INV-4）")
}

// ── TC-10 panic-② registry（A2 定稿：经 stopRegistryFn 缝注入同步 panic） ──

func TestShutdownSequence_RegistryPanicIsolated(t *testing.T) {
	f := newShutdownTestFixture(t)
	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	stopRegistryFn = func(*Runtime, time.Duration) { panic("registry boom") }
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop
	ts := startTestHTTPServer(t, okHandler)

	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Equal(t, 0, fs.stopCount(), "panic 的 ② 不得触达 registry（缝已替换）")
	require.Equal(t, 1, rec.callCount(), "③ 必须照走")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	require.Contains(t, f.log.String(), "level=WARN", "panic 必须留下 warn 日志（INV-4）")
}

// ── TC-11 panic-①http（A2 定稿：经 shutdownHTTPStepFn 缝注入同步 panic） ──

// 全序列验证：① panic 后 ②③ 照走、汇总行恰好一行、进程存活（INV-4）。
func TestShutdownSequence_HTTPPanicIsolated(t *testing.T) {
	f := newShutdownTestFixture(t)
	shutdownHTTPStepFn = func(*http.Server) error { panic("http boom") }

	ts := startTestHTTPServer(t, okHandler)
	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Equal(t, 1, fs.stopCount(), "② registry 必须照走")
	require.Equal(t, 1, rec.callCount(), "③ workers 必须照走")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	require.Contains(t, f.log.String(), "level=WARN", "panic 必须留下 warn 日志（INV-4）")
}

// ── TC-11 runShutdownStep 的 panic 隔离（http 步级验证面的原语层补充） ──

func TestShutdownStep_PanicIsolated(t *testing.T) {
	f := newShutdownTestFixture(t)

	elapsed := runShutdownStep("panic-step", func() {
		time.Sleep(20 * time.Millisecond)
		panic("http boom")
	})

	require.GreaterOrEqual(t, elapsed, 20*time.Millisecond, "panic 路径也必须计到耗时（命名返回值 + defer 赋值）")
	require.Contains(t, f.log.String(), "level=WARN", "panic 必须留 warn 日志")

	ran := false
	runShutdownStep("after", func() { ran = true })
	require.True(t, ran, "panic 不传播，后续步骤照走")
}

// ── TC-12 registry-卡死超上限（② 有界，顺序不变量仍成立） ──

func TestShutdownSequence_RegistryTimeoutBounded(t *testing.T) {
	f := newShutdownTestFixture(t)
	registryShutdownTimeout = 100 * time.Millisecond

	ts := startTestHTTPServer(t, okHandler)
	fs := &fakeScheduler{}
	fs.stopFn = func() {
		fs.dialCheck(ts.addr, 100*time.Millisecond)
		select {} // 永久阻塞：验证 ② 有界（禁项 6：不 Join 回收）
	}
	rt := newFakeRuntime(fs)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	seqStart := time.Now()
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	require.Less(t, time.Since(seqStart), time.Second, "② 卡死不得挂住整个序列")
	require.Equal(t, 1, fs.stopCount(), "超时只告警不重试（INV-7）")
	checked, err := fs.dialEvidence()
	require.True(t, checked, "Stop 内必须先做端口拨号检查")
	require.Error(t, err, "端口必须先于 ② 释放（INV-1）")
	require.Equal(t, 1, rec.callCount(), "③ 必须照走（逐步异常不阻断）")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-13 nil-runtime（demo 模式自动化部分） ──

func TestShutdownSequence_NilRuntime(t *testing.T) {
	f := newShutdownTestFixture(t)
	ts := startTestHTTPServer(t, okHandler)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	timings := boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(nil, ts.srv)
	}))

	require.Zero(t, timings.Registry, "nil runtime 必须跳过 ②")
	require.True(t, dialFails(ts.addr, 100*time.Millisecond), "demo 模式也必须摘掉端口")
	require.Equal(t, 1, rec.callCount(), "demo 模式 workers 步骤照走")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	require.Contains(t, f.log.String(), "registry=", "跳过步骤字段照打（0.0s）")
}

// ── TC-14 边界：nil-srv ──

func TestShutdownSequence_NilServer(t *testing.T) {
	f := newShutdownTestFixture(t)
	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	timings := boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, nil)
	}))

	require.Zero(t, timings.HTTP, "nil srv 必须跳过 ①")
	require.Equal(t, 1, fs.stopCount())
	require.Equal(t, 1, rec.callCount())
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-15 边界：runtime 与 srv 同时 nil ──

func TestShutdownSequence_NilRuntimeAndServer(t *testing.T) {
	f := newShutdownTestFixture(t)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	timings := boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(nil, nil)
	}))

	require.Zero(t, timings.HTTP)
	require.Zero(t, timings.Registry)
	require.Equal(t, 1, rec.callCount())
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-16 边界：runtime 非 nil 但 Registry==nil ──

func TestShutdownSequence_NilRegistry(t *testing.T) {
	f := newShutdownTestFixture(t)
	ts := startTestHTTPServer(t, okHandler)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	timings := boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(&Runtime{}, ts.srv)
	}))

	require.Zero(t, timings.Registry, "Registry==nil 必须跳过 ②（无 scheduler 被 Stop）")
	require.True(t, dialFails(ts.addr, 100*time.Millisecond), "http 步骤照走")
	require.Equal(t, 1, rec.callCount())
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-17 ErrServerClosed 分类：不误判、也不解除 RunServer 的 select ──

// 真实 listener + 真实 srv.Serve；SIGTERM 假信号触发 ① Shutdown → Serve 返回
// http.ErrServerClosed；② 卡在 hold 期间 Serve 已返回但 done 未关，RunServer 必须继续等 done
// （否则 main 提前 return 会跳过 ②③）。
func TestRunServer_ErrServerClosedDoesNotResolveSelect(t *testing.T) {
	f := newShutdownTestFixture(t)
	ts := newTestHTTPServer(t, okHandler)

	serveReturned := make(chan error, 1)
	listenAndServeFn = func(s *http.Server) error {
		err := s.Serve(ts.ln)
		serveReturned <- err
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signalChanFactory = func() <-chan os.Signal { return sigCh }

	hold := make(chan struct{})
	stopRegistryFn = func(*Runtime, time.Duration) { <-hold } // 卡住 ②，制造检查窗口

	rec := &workersRecorder{}
	stopWorkersFn = rec.stop
	rt := newFakeRuntime(&fakeScheduler{})

	done := SetupGracefulShutdown(rt, ts.srv)
	runErr := make(chan error, 1)
	go func() { runErr <- RunServer(ts.srv, done) }()
	waitUntilServing(t, ts.addr)

	sigCh <- syscall.SIGTERM

	select {
	case err := <-serveReturned:
		require.True(t, errors.Is(err, http.ErrServerClosed),
			"Shutdown 引发的 Serve 返回必须是 ErrServerClosed，实际: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve 未在 Shutdown 后返回")
	}
	// Serve 已返回 ErrServerClosed 且 done 未关（② 卡在 hold）→ RunServer 不得提前返回。
	select {
	case err := <-runErr:
		t.Fatalf("ErrServerClosed 不得解除 RunServer 的 select，却提前返回: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(hold)
	select {
	case <-done:
	case <-time.After(httpShutdownTimeout + registryShutdownTimeout + workersShutdownTimeout + time.Second):
		t.Fatal("释放 ② 后 done 必须有界关闭（INV-3）")
	}
	select {
	case err := <-runErr:
		require.NoError(t, err, "done 关闭后 RunServer 必须返回 nil（ErrServerClosed 不误判）")
	case <-time.After(time.Second):
		t.Fatal("done 关闭后 RunServer 必须返回")
	}
	require.Equal(t, 1, rec.callCount(), "③ 必须执行（若 main 提前 return 就会被跳过）")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── TC-18 端口占用分类（行为不变） ──

// 真实占用 addr 后 RunServer 必须把非 ErrServerClosed 的启动错误原样返回（main 据此 Fatalf）。
func TestRunServer_PortInUseReturnsError(t *testing.T) {
	f := newShutdownTestFixture(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = occupied.Close() })

	srv := &http.Server{Addr: occupied.Addr().String()}
	done := make(chan struct{}) // 永不关闭：错误只能来自启动失败

	runErr := make(chan error, 1)
	go func() { runErr <- RunServer(srv, done) }()

	select {
	case err := <-runErr:
		require.Error(t, err)
		require.False(t, errors.Is(err, http.ErrServerClosed), "端口占用必须是真实启动错误")
	case <-time.After(time.Second):
		t.Fatal("端口占用必须立即返回启动错误（main 据此 Fatalf）")
	}
	require.Zero(t, countLines(f.log.String(), "shutdown steps:"), "启动失败路径不得出现关停汇总行")
}

// ── TC-19 边界：nil err 分类 ──

// ListenAndServe 正常返回（nil）不算启动失败：RunServer 不得误报错误，必须等 done。
func TestRunServer_NilServeErrorNotReported(t *testing.T) {
	f := newShutdownTestFixture(t)
	listenAndServeFn = func(*http.Server) error { return nil }

	done := make(chan struct{})
	runErr := make(chan error, 1)
	go func() { runErr <- RunServer(&http.Server{}, done) }()

	select {
	case err := <-runErr:
		t.Fatalf("nil 启动错误不得上报（nil err 分类为 false）: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(done)
	select {
	case err := <-runErr:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("done 关闭后 RunServer 必须返回 nil")
	}
	require.Zero(t, countLines(f.log.String(), "shutdown steps:"))
}

// ── TC-20 汇总日志唯一性 + 字段完整 + 位置 ──

var shutdownSummaryRe = regexp.MustCompile(
	`shutdown steps: http=([0-9.]+)s registry=([0-9.]+)s workers=([0-9.]+)s total=([0-9.]+)s`)

func TestShutdownSequence_SummaryLineContract(t *testing.T) {
	f := newShutdownTestFixture(t)
	ts := startTestHTTPServer(t, okHandler)
	fs := &fakeScheduler{}
	rt := newFakeRuntime(fs)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(rt, ts.srv)
	}))

	logStr := f.log.String()
	require.Equal(t, 1, countLines(logStr, "shutdown steps:"), "汇总行唯一（INV-2）")
	require.Equal(t, 1, fs.stopCount(), "registry 步骤必须执行（保证 Stopped scheduler 日志存在）")

	lines := strings.Split(logStr, "\n")
	summaryIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "shutdown steps:") {
			summaryIdx = i
		}
	}
	require.NotEqual(t, -1, summaryIdx, "汇总行必须存在")
	schedulerLogs := 0
	for i, line := range lines {
		if strings.Contains(line, "Stopped scheduler:") {
			schedulerLogs++
			require.Less(t, i, summaryIdx, "② 的 scheduler 日志必须先于汇总行（INV-6）")
		}
	}
	require.Equal(t, 1, schedulerLogs)

	m := shutdownSummaryRe.FindStringSubmatch(logStr)
	require.NotNil(t, m, "汇总行格式必须匹配 %s", shutdownSummaryRe.String())
	httpSec, err := strconv.ParseFloat(m[1], 64)
	require.NoError(t, err)
	registrySec, err := strconv.ParseFloat(m[2], 64)
	require.NoError(t, err)
	workersSec, err := strconv.ParseFloat(m[3], 64)
	require.NoError(t, err)
	totalSec, err := strconv.ParseFloat(m[4], 64)
	require.NoError(t, err)
	require.GreaterOrEqual(t, httpSec, 0.0)
	require.GreaterOrEqual(t, registrySec, 0.0)
	require.GreaterOrEqual(t, workersSec, 0.0)
	maxStep := httpSec
	if registrySec > maxStep {
		maxStep = registrySec
	}
	if workersSec > maxStep {
		maxStep = workersSec
	}
	require.GreaterOrEqual(t, totalSec, maxStep-0.05, "total 必须 ≥ 各步耗时（容差 0.05s）")
}

// ── TC-21 超时上限可注入（包级 var 契约） ──

// 注意：本用例不经 fixture（fixture 会把三档改成短值），直接读真实默认值。
func TestShutdownTimeoutVars_Defaults(t *testing.T) {
	require.Equal(t, 5*time.Second, httpShutdownTimeout)
	require.Equal(t, 30*time.Second, registryShutdownTimeout)
	require.Equal(t, 10*time.Second, workersShutdownTimeout)

	origHTTP, origRegistry, origWorkers := httpShutdownTimeout, registryShutdownTimeout, workersShutdownTimeout
	t.Cleanup(func() {
		httpShutdownTimeout, registryShutdownTimeout, workersShutdownTimeout = origHTTP, origRegistry, origWorkers
	})

	httpShutdownTimeout = 111 * time.Millisecond
	registryShutdownTimeout = 222 * time.Millisecond
	workersShutdownTimeout = 333 * time.Millisecond
	require.Equal(t, 111*time.Millisecond, httpShutdownTimeout, "必须可覆写（var 非 const）")
	require.Equal(t, 222*time.Millisecond, registryShutdownTimeout)
	require.Equal(t, 333*time.Millisecond, workersShutdownTimeout)
}

// ── TC-22 单测禁 DB 自检（grep 等价实现，不调 shell） ──

func TestShutdownTestFileHasNoDBDependency(t *testing.T) {
	src, err := os.ReadFile("runtime_shutdown_test.go")
	require.NoError(t, err)
	// 禁用模式用字符串拼接写出：自检模式自身不得命中该 grep（TC-22 口径）。
	for _, forbidden := range []string{
		"database." + "InitDB",
		"database." + "DB",
		"GetTag" + "Queue",
		"tagging." + "StopAllWorkers()",
	} {
		require.NotContains(t, string(src), forbidden, "单测红线：不得依赖 %s", forbidden)
	}
	require.Contains(t, string(src), "admin.NewSchedulerRegistry()", "fake scheduler 必须经 Registry 注入")
	require.Contains(t, string(src), `Register("fake"`)
}

// ── TC-23 信号注册路径（不发真实信号） ──

// 注入假信号通道：返回瞬间无副作用；推一个 SIGTERM 后序列执行、done 关闭、端口摘除。
func TestSetupGracefulShutdown_SignalPath(t *testing.T) {
	f := newShutdownTestFixture(t)
	sigCh := make(chan os.Signal, 1)
	signalChanFactory = func() <-chan os.Signal { return sigCh }

	ts := startTestHTTPServer(t, okHandler)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	done := SetupGracefulShutdown(nil, ts.srv)
	require.NotNil(t, done)
	// 返回瞬间无副作用：workers 未调用、端口仍可连接。
	require.Equal(t, 0, rec.callCount())
	require.False(t, dialFails(ts.addr, 100*time.Millisecond), "注册信号后端口必须仍可连接")

	sigCh <- syscall.SIGTERM

	limit := httpShutdownTimeout + registryShutdownTimeout + workersShutdownTimeout + time.Second
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("SIGTERM 后 done 必须在 %v 内关闭", limit)
	}

	require.Equal(t, 1, rec.callCount(), "③ workers 必须执行一次")
	require.True(t, dialFails(ts.addr, 100*time.Millisecond), "端口必须已摘除")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	require.Contains(t, f.log.String(), "Received signal", "信号必须被记录")
}

// ── 真实信号双发（主线程指定；test-cases 禁项 2 的显式例外） ──

// 默认 signalChanFactory 在 Setup 返回前同步注册真实 SIGINT/SIGTERM（Notify 已生效），
// 随后向自身进程连发两枚 SIGTERM：容量 1 的缓冲通道只承接一枚，第二枚被丢弃——
// 不得触发第二次关停序列（workers 恰一次）。禁 t.Parallel（signal.Notify 进程级），
// t.Cleanup signal.Reset 还原默认处置。
func TestSetupGracefulShutdown_RealSignalDoubleFire(t *testing.T) {
	f := newShutdownTestFixture(t)

	ts := startTestHTTPServer(t, okHandler)
	rec := &workersRecorder{}
	stopWorkersFn = rec.stop

	done := SetupGracefulShutdown(nil, ts.srv)
	t.Cleanup(func() { signal.Reset(syscall.SIGINT, syscall.SIGTERM) })

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	limit := httpShutdownTimeout + registryShutdownTimeout + workersShutdownTimeout + time.Second
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("真实 SIGTERM 后 done 必须在 %v 内关闭", limit)
	}

	require.Equal(t, 1, rec.callCount(), "第二枚信号被丢弃：恰一次关停序列")
	require.True(t, dialFails(ts.addr, 100*time.Millisecond), "端口必须已摘除")
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
	require.Equal(t, 1, countLines(f.log.String(), "Received signal"), "信号接收日志恰好一行")
}

// ── TC-24 defer 链恢复执行（自动化可测部分：关停路径无 os.Exit） ──

func TestShutdownSourceHasNoOsExit(t *testing.T) {
	src, err := os.ReadFile("runtime.go")
	require.NoError(t, err)
	require.NotContains(t, string(src), "os.Exit", "关停路径不得再出现 os.Exit（defer 链必须真实执行）")

	f := newShutdownTestFixture(t)
	ts := startTestHTTPServer(t, okHandler)
	_ = boundedWait(t, asyncTimings(func() shutdownTimings {
		return runShutdownSequence(nil, ts.srv)
	}))
	// 若序列调 os.Exit(0)，本进程会立即终止 → 不可能执行到这里。
	require.True(t, dialFails(ts.addr, 100*time.Millisecond))
	require.Equal(t, 1, countLines(f.log.String(), "shutdown steps:"), "汇总行恰好一行")
}

// ── main.go 行为兼容源码契约（TC-18 附注 + 行为兼容清单） ──

// main.go 保留 Fatalf 启动失败分支（端口占用行为不变）、不再用 r.Run(addr)、不引入 os.Exit，
// 并接上 SetupGracefulShutdown/RunServer 与 5s tracer 超时。
func TestShutdownMainSourceContract(t *testing.T) {
	src, err := os.ReadFile("../../cmd/server/main.go")
	require.NoError(t, err)
	mainSrc := string(src)

	require.Contains(t, mainSrc, `logging.Fatalf("Failed to start server: %v", err)`,
		"端口占用仍须 Fatalf（错误日志 + 非 0 退出码行为不变）")
	require.Contains(t, mainSrc, "appbootstrap.SetupGracefulShutdown(runtime, srv)")
	require.Contains(t, mainSrc, "appbootstrap.RunServer(srv, done)")
	require.Contains(t, mainSrc, "context.WithTimeout(context.Background(), 5*time.Second)",
		"tracer flush 必须有界（替换 context.Background()）")
	require.NotContains(t, mainSrc, "r.Run(addr)", "启动须显式持有 *http.Server")
	require.NotContains(t, mainSrc, "os.Exit", "main 正常 return，不得 os.Exit")
}
