# Test Cases — graceful-shutdown-hardening（复杂档白盒用例）

<!-- 对应 proposal.md 头部 complexity: complex；本文件是 case-first-testing（docs/reference/开发执行规范.md §2）复杂档要求的白盒用例清单。

用途：把 change 的 11 条断言判据机械展开为可判定的测试分支，供实现阶段逐条落成 `backend-go/internal/app/runtime_shutdown_test.go`
（tasks.md 3.1）。本文件只写用例，不写测试代码；实现命名以「前置实现契约」为准，若改名须同步本文件。 -->

- 唯一测试文件：`backend-go/internal/app/runtime_shutdown_test.go`（package `app`，同包白盒）
- 唯一测试入口：内部序列函数（建议 `runShutdownSequence(runtime, srv) <-chan struct{}`），**不发真实信号**
- 判定口径：所有「有界」断言 = `select { case <-done: case <-time.After(注入上限 + 1s): t.Fatal }`

## 前置实现契约（测试依赖的钩子）

> 定稿 = design.md **D8**（apply 冻结）。下表的符号名已按 D8 对齐，TC 正文中的旧建议名按「旧名 → D8 名」映射读：`runShutdownSequence` 为**同步**函数（返回 `shutdownTimings`，不是 channel，测试在主 goroutine 或自起 goroutine 调用均可）；`StartHTTPServer` + `IsFatalServeError` → `RunServer(srv, done) error`（语义等价）。

| 符号（D8 定稿） | 形态 | 用途 |
| --- | --- | --- |
| `SetupGracefulShutdown(rt *Runtime, srv *http.Server) <-chan struct{}` | 签名固定（change 定稿） | 信号 → 序列 → `close(done)`；TC-23 用注入的 `signalChanFactory` 触发 |
| `runShutdownSequence(rt *Runtime, srv *http.Server) shutdownTimings` | 包级**同步**函数，跑完四步返回各步耗时 | **测试主入口**（禁真实信号） |
| `httpShutdownTimeout` / `registryShutdownTimeout` / `workersShutdownTimeout` | 包级 `var time.Duration` = 5s / 30s / 10s | ①②③ 上限；覆写注入（常量契约见 TC-21） |
| `stopWorkersFn` | 包级 `var func() = tagging.StopAllWorkers` | ③ 注入缝（禁真身依赖 DB） |
| `stopRegistryFn` | 包级 `var func(*Runtime, time.Duration)` | ② 注入缝：TC-10 注入 panic、TC-12 注入阻塞（A2 定稿走 (i) 缝方案，scheduler 包零改动） |
| `shutdownHTTPStepFn` | 包级 `var func(*http.Server) error` | ① 注入缝：TC-11 注入同步 panic（默认实现 = `httpShutdownTimeout` ctx 的 `srv.Shutdown`） |
| `signalChanFactory` | 包级 `var func() <-chan os.Signal` | TC-23 注入假通道，不真发信号 |
| `listenAndServeFn` | 包级 `var func(*http.Server) error` | TC-17 注入 `http.ErrServerClosed` |
| `RunServer(srv *http.Server, done <-chan struct{}) error` | 包级函数（main 的启动封装） | TC-17/18/19：`ErrServerClosed` 不返回错误、必须等 done；其它错误原样返回（main 据此 Fatalf） |
| `runShutdownStep(name string, fn func()) time.Duration` | 包级函数（独立 recover） | TC-09~TC-11 的 panic 隔离在步级验证 |

统一 fixture（每个用例）：`logging.SetWriters(&buf, &buf)` + `defer logging.ResetWriters()`；覆写 `stopWorkersFn` 为记录型 fake + defer 还原；三个超时 var 覆写后 defer 还原；listener 用 `net.Listen("tcp", "127.0.0.1:0")` + `go srv.Serve(ln)` 自建；fake scheduler 经 `admin.NewSchedulerRegistry()` + `Register("fake", fs)` 注入 `&Runtime{Registry: reg}`。

## 分支表（24 例）

| 用例 ID | 分支/触发条件 | 输入（含注入值） | 期望结果（可机械判定） | 覆盖的 spec Scenario | 测试文件 |
| --- | --- | --- | --- | --- | --- |
| TC-01 | 顺序-正常路径（runtime 非 nil，无在途请求） | `shutdownHTTPTimeout=1s`、registry/workers=200ms；fake.Stop 内 `net.DialTimeout(addr, 100ms)`；handler 无阻塞 | done ≤1.5s 关闭；fake.Stop 调用 1 次；Stop 内 dial 返回非 nil err（refused/EOF）；汇总行存在 | SIGTERM 后端口先行释放 | runtime_shutdown_test.go |
| TC-02 | 顺序-在途阻塞期（Shutdown 未返回时不得出现「Stop 内拨号失败」证据） | handler 阻塞在 `release` chan 且不释放；触发序列；检查点等待 100ms 后 `close(release)` | 检查点：`fake.stopCalled==false` 且 `net.DialTimeout(addr)` 已失败（listener 已关）；释放后 done ≤1.5s 关闭、fake.Stop 恰好 1 次、原请求收到 200 | SIGTERM 后端口先行释放；在途请求排空 | runtime_shutdown_test.go |
| TC-03 | 排空-正常（在途请求在上限内完成，且 Shutdown 等它完成） | `shutdownHTTPTimeout=1s`；handler 阻塞 50ms 后写 200 + body `"drained"`；fake.Stop 内断言 `handlerFinished==true` | HTTP 200 且 body==`"drained"`；fake.Stop 内 `handlerFinished` 为 true（即 ① 未提前返回）；done 关闭；总耗时 <1.5s | 在途请求排空 | runtime_shutdown_test.go |
| TC-04 | 排空-超时（在途慢请求 > 注入上限，序列继续） | `shutdownHTTPTimeout=100ms`；handler 阻塞 300ms；记录序列起点与 fake.Stop 时刻 | done 在 [100ms, 1s] 关闭；fake.Stop 调用时刻 − 起点 ∈ [100ms, 250ms)（证明 Shutdown 在 handler 300ms 结束前返回）；workers 调用 1 次；汇总行仍恰好一行；无挂死 | 在途请求排空；逐步异常不阻断退出 | runtime_shutdown_test.go |
| TC-05 | 边界：排空上限内（50ms < 100ms） | `shutdownHTTPTimeout=100ms`；handler 阻塞 50ms 后 200 | fake.Stop 内 `handlerFinished==true`；done <500ms 关闭；无超时告警行 | 在途请求排空 | runtime_shutdown_test.go |
| TC-06 | 边界：恰好等于上限（100ms == 100ms，**双结果接受**） | `shutdownHTTPTimeout=100ms`；handler 阻塞 100ms 后 200 | 仅断言：done ∈ [100ms, 1s] 关闭；序列继续（workers + 汇总行）；请求不悬挂（收到 200 或连接被关）；**err 为 nil / deadline exceeded 均接受** | 在途请求排空 | runtime_shutdown_test.go |
| TC-07 | workers-卡死超上限（有界 + 后续照走） | `shutdownWorkersStepTimeout=100ms`；`stopWorkersFn=func(){ select{} }`；registry 步正常 | done ∈ [100ms, 1s] 关闭；`stopWorkersFn` 调用计数 ==1（不重试）；fake.Stop（②）在 workers 步开始前已完成（时间戳/顺序记录）；汇总行恰好一行且含 `workers=` | 逐步异常不阻断退出；耗时汇总行出现 | runtime_shutdown_test.go |
| TC-08 | 边界：workers 在上限内返回（50ms < 100ms） | `shutdownWorkersStepTimeout=100ms`；`stopWorkersFn` 阻塞 50ms 后返回 | done <500ms 关闭；无 workers 超时告警（若实现约定文案则断言其不出现，否则仅判耗时） | 逐步异常不阻断退出 | runtime_shutdown_test.go |
| TC-09 | panic-③workers（步级隔离） | `stopWorkersFn=func(){ panic("boom") }`；超时注入短值 | 测试进程存活且执行到断言（panic 被 recover）；workers 之后的汇总行恰好一行；done ≤1s 关闭；捕获日志非空（warn/error 文案不硬编码） | 逐步异常不阻断退出 | runtime_shutdown_test.go |
| TC-10 | panic-②registry（fake `Stop()` panic） | runtime 非 nil，fake scheduler 的 `Stop()` 内 `panic("registry boom")`；超时注入短值 | 进程不崩溃；workers 步与汇总行照走；done ≤1s 关闭 | 逐步异常不阻断退出 | runtime_shutdown_test.go（依赖见歧义 A2） |
| TC-11 | panic-①http（步函数缝注入 panic） | `shutdownHTTPStepFn`（步缝）替换为 `func(*http.Server) error { panic("http boom") }`；srv 非 nil | 进程不崩溃；registry 与 workers 步照走（调用计数各 1）；汇总行恰好一行；done ≤1s 关闭 | 逐步异常不阻断退出 | runtime_shutdown_test.go（依赖见歧义 A2） |
| TC-12 | registry-卡死超上限（② 有界，顺序不变量仍成立） | `shutdownRegistryStepTimeout=100ms`；fake.Stop 先做 dial 检查再 `select{}` 阻塞 | StopAll ≤1s 返回（不挂死）；dial 检查失败（端口已先释放）；workers + 汇总行照走；done ≤1s 关闭；fake.Stop 调用计数 ==1 | SIGTERM 后端口先行释放；逐步异常不阻断退出 | runtime_shutdown_test.go |
| TC-13 | nil-runtime（demo 模式自动化部分） | `runtime=nil`；srv 正常；`stopWorkersFn` 记录型 | 不 panic；http 步执行（dial 失败）；workers 调用 1 次；汇总行恰好一行且 `registry=` 字段存在（数值可为 0.0s，不得缺字段）；done ≤1.5s 关闭 | demo 模式优雅摘端口 | runtime_shutdown_test.go |
| TC-14 | 边界：nil-srv | `srv=nil`；runtime 正常 fake；`stopWorkersFn` 记录型 | 不 panic；跳过 http 步；fake.Stop 调用 1 次；workers 调用 1 次；汇总行恰好一行；done ≤1.5s 关闭 | 逐步异常不阻断退出（边界） | runtime_shutdown_test.go |
| TC-15 | 边界：runtime 与 srv 同时 nil | 两者均 nil；`stopWorkersFn` 记录型 | 不 panic；workers 调用 1 次；汇总行恰好一行；done ≤1.5s 关闭 | demo 模式优雅摘端口 | runtime_shutdown_test.go |
| TC-16 | 边界：runtime 非 nil 但 `Registry==nil` | `&Runtime{Registry: nil}`；srv 正常 | 不 panic；registry 步跳过（无任何 scheduler 被 Stop）；http/workers/汇总照走；done 关闭 | 逐步异常不阻断退出（边界） | runtime_shutdown_test.go |
| TC-17 | ErrServerClosed 分类（正常关闭不误判） | 真实 listener + `StartHTTPServer(srv)`；短 ctx `srv.Shutdown` 后从 channel 收 err | `errors.Is(err, http.ErrServerClosed)==true`；`IsFatalServeError(err)==false`；进程存活（未走 Fatalf） | ErrServerClosed 不误判 | runtime_shutdown_test.go |
| TC-18 | 端口占用分类（行为不变） | `net.Listen("tcp","127.0.0.1:0")` 占住 addr；对同 addr `StartHTTPServer(&http.Server{Addr: addr})` | channel 收到非 nil err 且 `!errors.Is(err, http.ErrServerClosed)`；`IsFatalServeError(err)==true`；进程存活；源码检查 `grep -n "Fatalf" cmd/server/main.go` 仍存在该分支 | 端口占用启动失败行为不变 | runtime_shutdown_test.go（+ main.go 源码检查） |
| TC-19 | 边界：nil err 分类 | `IsFatalServeError(nil)` | 返回 false（正常退出不误报） | ErrServerClosed 不误判（边界） | runtime_shutdown_test.go |
| TC-20 | 汇总日志唯一性 + 字段完整 + 位置 | 正常序列（短注入值）跑完，读取捕获 buf | 含 `shutdown steps:` 的行数 == 1；该行正则命中 `http=([0-9.]+)s registry=([0-9.]+)s workers=([0-9.]+)s total=([0-9.]+)s`；四值可解析为非负浮点且 `total ≥ max(http,registry,workers)`（容差 0.05s）；该行之后不再出现 `Stopped scheduler:` | 耗时汇总行出现 | runtime_shutdown_test.go |
| TC-21 | 超时上限可注入（包级 var 契约） | 读取三个包级 var 默认值；测试覆写后回读；defer 还原 | 默认值精确等于 `5*time.Second` / `30*time.Second` / `10*time.Second`；覆写后回读为新值（即实现必须是 `var` 而非 `const`，由编译机械保证） | —（支撑全部超时用例） | runtime_shutdown_test.go |
| TC-22 | 单测禁 DB 自检 | 无 PostgreSQL/无 DB env 跑 `go test ./internal/app -run 'Shutdown' -count=1`；`grep -nE 'database\.(InitDB\|DB)\|GetTagQueue\|tagging\.StopAllWorkers\(\)' backend-go/internal/app/runtime_shutdown_test.go` | 测试 PASS；grep 无输出（不碰 DB / 不直调真身）；fake 经 `admin.NewSchedulerRegistry()` + `Register` 注入 | —（测试红线，见禁用项清单） | runtime_shutdown_test.go |
| TC-23 | 信号注册路径（不发真实信号，边界） | `SetupGracefulShutdown(nil, srv)` 后立即观察 | 立即返回非 nil `<-chan struct{}`；不 panic；返回瞬间无副作用（workers 未被调用、端口仍可 dial 成功）；泄漏的注册 goroutine 允许随测试进程结束 | demo 模式优雅摘端口（注册部分） | runtime_shutdown_test.go |
| TC-24 | defer 链恢复执行（**自动化部分**） | 任一正常序列用例完成后继续执行断言；`grep -n 'os.Exit' backend-go/internal/app/runtime.go` | 序列返回后测试进程仍存活（若调 `os.Exit(0)`，测试进程立即终止、该用例不可能 PASS）；runtime.go 的 `os.Exit` 字面量命中数为 0；done 已关闭 | defer 链恢复执行 | runtime_shutdown_test.go（+ runtime.go 源码检查） |

## 边界值与不变式清单

| 编号 | 类型 | 内容 | 判定位置 |
| --- | --- | --- | --- |
| INV-1 | 不变式 | 端口先于 ② 释放：唯一证据 = `fake.Stop` 内 dial 失败（refused/EOF），且该证据只在 Shutdown 返回后才出现 | TC-01/02/12 + MAN-04 |
| INV-2 | 不变式 | 任何完成路径（正常/超时/panic/nil 跳过）恰好一行 `shutdown steps:` 且四字段齐全 | TC-04/07/09/10/11/12/13/14/15/20 |
| INV-3 | 不变式 | done 必关闭：任何路径 ≤ 注入上限 + 1s，超时即 `t.Fatal` | 全部用例 |
| INV-4 | 不变式 | panic 不吞退出也不静默：进程存活 + 后续步骤执行 + 捕获日志非空 | TC-09/10/11 |
| INV-5 | 不变式 | 关停路径无 `os.Exit`：源码零命中 + 测试进程存活双向验证 | TC-24 + MAN-01 |
| INV-6 | 不变式 | 顺序单调 ①→②→③→④：以调用时间戳/记录断言，不以日志文案断言 | TC-07/12/20 |
| INV-7 | 不变式 | 超时只告警不重试：卡死步的调用计数 ==1 | TC-07/12 |
| INV-8 | 不变式 | 测试独立性：包级 var（3 超时 + `stopWorkersFn`）覆写必须 defer 还原，用例间不共享可变状态 | 全部用例 |
| INV-9 | 不变式 | `startTestHTTPServer` 必须等 Serve 注册 listener（`/__ready` 探针）再返回：Serve 前 `srv.Shutdown` 是 no-op（listener 不关），否则「端口已释放」断言随机误红 + http 步覆盖空洞（review High，已修：ready 探针不经用例 handler，阻塞型 handler 用例也能就绪） | 全部序列用例（夹具层统一保障） |
| B-1 | 边界 | 排空 50ms / 恰好 100ms / 300ms（上限 100ms）三档 | TC-05/06/04 |
| B-2 | 边界 | workers 50ms / 永久阻塞（上限 100ms）两档 | TC-08/07 |
| B-3 | 边界 | registry 永久阻塞（上限 100ms） | TC-12 |
| B-4 | 边界 | 默认超时值精确 5s/30s/10s | TC-21 |
| B-5 | 边界 | 有界余量口径统一为「注入上限 + 1s」 | 全部有界断言 |

## 禁用项清单（测试红线）

1. **禁 DB**：不得调用 `database.InitDB` / `database.DB`，不得直调 `tagging.StopAllWorkers()` 真身（其依赖 `GetTagQueue` → DB 单例）；workers 步一律用 `stopWorkersFn` 注入。
2. **禁真实信号**：不得 `syscall.Kill` / `os.Process.Signal` / `signal.Notify` 后向自身进程发 SIGTERM/SIGINT；测试只调 `runShutdownSequence`。真实信号路径仅由实战验收（MAN-01/02/04）覆盖。
3. **禁真实长等待**：不得出现 5s/30s/10s 的真实等待；三个上限必须覆写为 ≤200ms，单文件总运行时长目标 <15s。
4. **禁触发 Fatalf**：`Fatalf` 内部 `os.Exit`，测试只断言 `IsFatalServeError` 分类，不实际走 Fatalf。
5. **禁硬编码未定义日志文案**：warn/超时文案以实现为准，断言只做「行存在 / 字段存在 / 计数器」，不匹配整句。
6. **禁 Join 卡死 goroutine**：`stopWorkersFn` 永久阻塞的 goroutine 不等待回收（实现语义：随进程退出，队列持久化保证可恢复）。

## Scenario 覆盖矩阵（8/8）

| spec Scenario | 自动化用例 | 实战/人工 |
| --- | --- | --- |
| SIGTERM 后端口先行释放 | TC-01、TC-02、TC-12 | MAN-04 |
| 在途请求排空 | TC-03、TC-04、TC-05、TC-06 | — |
| 逐步异常不阻断退出 | TC-04、TC-07、TC-08、TC-09、TC-10、TC-11、TC-12、TC-14、TC-16 | — |
| defer 链恢复执行 | TC-24（自动化可测部分：无 os.Exit + 进程存活） | MAN-01（tracer flush / logging.Close 副作用 + 退出码 0；单测不可达 main 的 defer） |
| 端口占用启动失败行为不变 | TC-18 | MAN-03 |
| ErrServerClosed 不误判 | TC-17、TC-19 | — |
| demo 模式优雅摘端口 | TC-13、TC-15、TC-23（自动化可测部分：nil runtime 序列） | MAN-02（`DEMO_READ_ONLY=1` 真进程 TERM → 端口释放 + 退出码 0） |
| 耗时汇总行出现 | TC-20（+ TC-04/07/09/12/13 的汇总行期望） | MAN-04（实战采集真实耗时构成） |

## 实战/人工验收（非单测，tasks.md §6 留痕）

| 用例 ID | 场景 | 操作 | 期望 | Scenario |
| --- | --- | --- | --- | --- |
| MAN-01 | defer 链恢复执行 | 起后端 → `kill -TERM <pid>` | 退出码 0；tracer Shutdown 与 logging.Close 副作用可见（遥测 flush、日志落盘、无 `Failed to shutdown tracer`） | defer 链恢复执行 |
| MAN-02 | demo 模式优雅摘端口 | `DEMO_READ_ONLY=1` 起后端 → TERM | 端口 ≤5s 释放；退出码 0（现状为默认硬杀） | demo 模式优雅摘端口 |
| MAN-03 | 端口占用退出行为 | 先占用端口 → 起后端 | 错误日志 + 非 0 退出码 | 端口占用启动失败行为不变 |
| MAN-04 | 端口时限 + 耗时汇总采集 | `start-dev.sh --restart back` → `kill -TERM <监听 pid>` | 端口 ≤5s 释放；进程 ≤60s 退出且退出码 0；nohup.out 恰好一行 `shutdown steps:` 含四字段；`start-dev.sh stop` 不再触发 KILL 升级 | SIGTERM 后端口先行释放；耗时汇总行出现 |

## 歧义与待定（实现阶段需定稿，本文件不擅自放宽/收紧）

- **A1（断言 #1 第二句的读法）**：取「Stop 内的拨号失败证据不得早于 Shutdown 返回」这一读法（TC-02）；另一读法「Shutdown 返回前端口仍接受连接」与 net/http Shutdown 语义（先关 listener 再排空）矛盾，TC-02 的检查点实际会反证它。
- ~~**A2（断言 #5 的 registry/http panic 注入面）**~~ 已定稿（D8）：走 (i) —— runtime 侧加 `stopRegistryFn` 缝；scheduler 包零改动。注意 `Registry.StopAll` 内部 goroutine 的真 panic 仍不可 recover（D8 只保证**同步调用面**的 panic 隔离），TC-10 用缝注入同步 panic。
- ~~**A3（汇总行字段值）**~~ 已定稿（D8 不变式 4）：跳过步骤字段照打、值 `0.0s`（不用 `n/a`）。
- ~~**A4（符号命名）**~~ 已定稿（D8 + 上表）：`runShutdownSequence` 为同步函数；`StartHTTPServer`/`IsFatalServeError` 由 `RunServer(srv, done) error` 取代。
- **A5（TC-06 的非确定性）**：`time.After(100ms)` 与 handler 阻塞 100ms 竞争，err 结果不可判定；已把 oracle 降为「双结果接受 + 有界不挂死」，这是本文件唯一非全确定用例。
- **A6（defer 链可测性）**：`tp.Shutdown` / `logging.Close` 位于 `package main` 的 defer 中，`internal/app` 单测不可达；自动化只能覆盖「序列不调 os.Exit + 源码零 os.Exit」，副作用本身归 MAN-01。
