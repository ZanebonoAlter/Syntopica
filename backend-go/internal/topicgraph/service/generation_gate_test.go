package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/testutil"
)

// ── guard-daily-report-generate 1.1：日报生成域闸（CAS 互斥）──
//
// 契约锚：change specs/daily-report-system/spec.md「日报生成 API — 异步触发」
// （MODIFIED）——同一时刻至多一条日报生成管线；闸在管线结束后必须释放。

func TestDailyReportGenerationGate(t *testing.T) {
	t.Run("first acquire succeeds", func(t *testing.T) {
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("first TryAcquire on a free gate must succeed")
		}
		ReleaseDailyReportGeneration()
	})

	t.Run("second acquire while held fails", func(t *testing.T) {
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("setup: first acquire must succeed")
		}
		if TryAcquireDailyReportGeneration() {
			t.Fatal("TryAcquire while the gate is held must fail")
		}
		ReleaseDailyReportGeneration()
	})

	t.Run("acquire succeeds after release", func(t *testing.T) {
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("setup: first acquire must succeed")
		}
		ReleaseDailyReportGeneration()
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("TryAcquire after Release must succeed")
		}
		ReleaseDailyReportGeneration()
	})

	t.Run("100 concurrent acquires exactly one winner", func(t *testing.T) {
		var winners atomic.Int32
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if TryAcquireDailyReportGeneration() {
					winners.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := winners.Load(); got != 1 {
			t.Fatalf("exactly 1 winner expected among 100 concurrent TryAcquire, got %d", got)
		}
		ReleaseDailyReportGeneration()
	})
}

// ── fix-daily-report-cross-process-gate 1.2：数据库级跨实例互斥 ──
//
// 契约锚：change specs/daily-report-system/spec.md「日报生成 API — 异步触发」
// （MODIFIED）——互斥范围覆盖共享同一 PG 的全部后端实例；持闸进程崩溃锁随
// 连接终结自动释放；DB 不可达回退进程内互斥（test-cases.md 故事 A）。

// newTestGate builds a private gate instance backed by sqlDB — one instance
// per simulated "process"; two instances sharing one server reproduce the
// cross-process topology without touching the package-level processGate.
func newTestGate(sqlDB *sql.DB) *dailyReportGate {
	return &dailyReportGate{db: func() *sql.DB { return sqlDB }}
}

// A1-A4: two "instances" (independent connection pools, same server) are
// mutually exclusive through the DB-level advisory lock, and the lock frees
// on release with no residue.
func TestDailyReportGateCrossInstance(t *testing.T) {
	dbA := testutil.SetupTestDB(t)
	dbB := testutil.NewIsolatedDB(t)
	sqlA, err := dbA.DB()
	if err != nil {
		t.Fatalf("sql handle A: %v", err)
	}
	sqlB, err := dbB.DB()
	if err != nil {
		t.Fatalf("sql handle B: %v", err)
	}
	gateA, gateB := newTestGate(sqlA), newTestGate(sqlB)

	if !gateA.tryAcquire() {
		t.Fatal("A1: instance A must acquire the free gate")
	}
	if gateB.tryAcquire() {
		t.Fatal("A2: instance B must be rejected while A holds the DB-level gate")
	}
	gateA.release()
	if !gateB.tryAcquire() {
		t.Fatal("A3: instance B must acquire after A releases")
	}
	gateB.release()
	if !gateA.tryAcquire() {
		t.Fatal("A4: no residual lock may survive release")
	}
	gateA.release()
}

// A5: the DB-level lock lives on a dedicated session — when the owning
// process dies (connection closed), PostgreSQL frees the advisory lock
// automatically; the next instance acquires immediately, no manual cleanup.
func TestDailyReportGateCrashRelease(t *testing.T) {
	dbA := testutil.SetupTestDB(t)
	dbB := testutil.NewIsolatedDB(t)
	sqlA, _ := dbA.DB()
	sqlB, _ := dbB.DB()
	gateA, gateB := newTestGate(sqlA), newTestGate(sqlB)

	if !gateA.tryAcquire() {
		t.Fatal("setup: instance A must acquire")
	}
	// Simulate the crash the way the server sees it: terminate the session
	// holding the advisory lock (a dead process = torn-down TCP session =
	// PG backend cleanup). sql.Conn.Close alone would merely RETURN the
	// healthy connection to the pool — session alive, lock still held.
	gateA.mu.Lock()
	conn := gateA.conn
	gateA.mu.Unlock()
	if conn == nil {
		t.Fatal("a DB-level acquire must hold a dedicated connection")
	}
	var pid int64
	qctx, qcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer qcancel()
	if err := conn.QueryRowContext(qctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatalf("backend pid of the gate connection: %v", err)
	}
	if _, err := sqlB.Exec("SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatalf("terminate lock-holding session: %v", err)
	}
	gateA.mu.Lock()
	gateA.conn = nil // the dead "process" never runs release code
	gateA.mu.Unlock()
	// Deliberately DO NOT conn.Close(): the terminate notice arrives
	// asynchronously, so the pool cannot know the connection is dead yet —
	// returning it would hand a zombie connection to the next borrower
	// (observed as a delayed 57P01 in unrelated follow-up tests). Leaking
	// the handle to process exit is the safe simulation of a crashed
	// process, which also never returns its connections.

	// Session teardown on the server is asynchronous from the survivor's
	// perspective; a real crash→restart cycle has seconds of latency, so
	// poll briefly instead of assuming instant visibility.
	acquired := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if gateB.tryAcquire() {
			acquired = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !acquired {
		t.Fatal("A5: the session lock must free with the dead connection — no manual cleanup")
	}
	gateB.release()
}

// A6: DB unreachable (conn to a dead endpoint) — acquisition falls back to
// the in-process gate and SUCCEEDS (D3 fail-open): a DB hiccup must not
// block generation; the fallback still rejects local double-acquire.
func TestDailyReportGateDBFallback(t *testing.T) {
	bad, err := sql.Open("pgx", "host=127.0.0.1 port=1 user=nop dbname=nop sslmode=disable connect_timeout=2")
	if err != nil {
		t.Fatalf("open bad handle: %v", err)
	}
	t.Cleanup(func() { _ = bad.Close() })
	g := newTestGate(bad)

	if !g.tryAcquire() {
		t.Fatal("A6: DB-unreachable acquire must fall back to the in-process gate and succeed")
	}
	if g.tryAcquire() {
		t.Fatal("A6: the in-process fallback still rejects a second local acquire")
	}
	g.release()
	if !g.tryAcquire() {
		t.Fatal("A6: release frees the fallback gate")
	}
	g.release()
}

// ── D13：闸探测失败路径必须归还连接（不得泄漏池连接）──
//
// 契约锚：change add-radar-pipeline-status design.md D13——DB 级闸的探测失败
// 分支（T4）终止会话后必须 Close 那条专用连接。不归还则每失败一次漏一条池
// 连接；单连接池（MaxOpenConns(1)，如 internal/admin/scheduler 的 SQLite 测试
// 库）随即被占满，后续查询永久阻塞——那是该包「测试挂死」的根因。
//
// 用 SQLite 触发真实的 T4 路径：没有 pg_try_advisory_lock → 探测必然失败。
func TestDailyReportGateProbeFailureReturnsConnection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql handle: %v", err)
	}
	// 单连接池：漏一条连接即饿死，后续查询直接阻塞。
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	g := newTestGate(sqlDB)

	// SQLite 没有 pg_try_advisory_lock → 探测失败 → T4 分支（fail-open 回退）。
	if !g.tryAcquire() {
		t.Fatal("探测失败必须回退进程内闸并成功（fail-open）")
	}
	if inUse := sqlDB.Stats().InUse; inUse != 0 {
		t.Fatalf("探测失败后专用连接必须归还连接池（InUse=%d，期望 0）", inUse)
	}
	g.release()

	// 池必须仍然可用：漏连接时下面这条查询会一直阻塞到 ctx 超时。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var one int
	if err := sqlDB.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("连接池被饿死——探测失败路径泄漏了连接：%v", err)
	}
}
