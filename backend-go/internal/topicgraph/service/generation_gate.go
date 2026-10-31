package service

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"syntopica-backend/internal/platform/logging"
	repository "syntopica-backend/internal/topicgraph/repository"
)

// dailyReportGateKey is the process-external (PostgreSQL session-level)
// advisory-lock key for the daily-report generation pipeline
// (fix-daily-report-cross-process-gate D1/D5). The in-process atomic gate
// alone cannot stop a second backend instance sharing the same database from
// running a concurrent pipeline (the 2026-10-23 incident), so the lock lives
// in the database.
//
// Constant = fnv64a("syntopica:daily-report:generation-gate"); it MUST be
// compile-time stable so every instance computes the same key.
const dailyReportGateKey int64 = 2200772608100431505 // 0x1E8AB706E13C7691

// gateProbeTimeout bounds the advisory-lock roundtrip: a DB hiccup degrades
// to the in-process gate (D3) instead of stalling the trigger surface.
const gateProbeTimeout = 5 * time.Second

// dailyReportGate is one process's handle on the cross-instance generation
// gate. The zero value is the in-process-only fallback (used before the DB is
// wired and in unit tests); tryAcquire upgrades it to a PostgreSQL session
// advisory lock held on a dedicated connection (advisory locks bind to a
// single connection — pooling would let the lock "leak" onto a pooled conn
// while release lands on another, D2).
type dailyReportGate struct {
	running atomic.Bool
	mu      sync.Mutex // guards conn (acquire/release/panic races)
	conn    *sql.Conn  // held while the DB-level lock is owned; nil in fallback mode
	db      func() *sql.DB
}

// defaultGateDBSource resolves the shared *sql.DB from the repository
// singleton. Returns nil before wiring (main runs Init before StartRuntime,
// but tests and early startup hit this path) — nil selects the in-process
// fallback.
func defaultGateDBSource() *sql.DB {
	if repository.Repo == nil {
		return nil
	}
	sqlDB, err := repository.Repo.DB().DB()
	if err != nil {
		return nil
	}
	return sqlDB
}

var processGate = &dailyReportGate{db: defaultGateDBSource}

// TryAcquireDailyReportGeneration attempts to claim the generation gate.
// Returns true when the caller is now the exclusive owner of the generation
// pipeline across EVERY backend instance sharing this database (PostgreSQL
// session advisory lock on a dedicated connection), and MUST pair the claim
// with ReleaseDailyReportGeneration (deferred, so panics also release).
//
// Returns false when a pipeline is already running (this process or another
// instance) — the caller must skip (scheduler) or answer 409
// already_running (manual endpoint) without starting a second pipeline or
// sending any terminal notification.
//
// D3 fail-open: when the database is unreachable (Conn/try-lock probe
// fails), the gate degrades to the pre-upgrade in-process semantics and the
// acquisition SUCCEEDS — a DB hiccup must not block generation on a
// single-instance deployment (behavior no worse than before the upgrade).
func TryAcquireDailyReportGeneration() bool {
	return processGate.tryAcquire()
}

func (g *dailyReportGate) tryAcquire() bool {
	// T1: in-process CAS first — serializes this process's trigger surfaces
	// even in fallback mode and cheaply rejects local double-triggering.
	if !g.running.CompareAndSwap(false, true) {
		return false
	}

	sqlDB := g.db()
	if sqlDB == nil {
		// T2: no DB wired — in-process-only mode (unit tests / pre-wiring).
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), gateProbeTimeout)
	defer cancel()

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		// T3: DB unreachable — fall back to the in-process gate (D3).
		logging.Warnf("daily-report gate: db conn unavailable (%v); falling back to in-process mutex", err)
		return true
	}

	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", dailyReportGateKey).Scan(&locked); err != nil {
		// T4: probe failed (timeout, restart in flight, network reset) — fall
		// back (D3). DANGER: the probe may have TIMED OUT client-side while the
		// server already GRANTED the lock — conn.Close alone would return a
		// healthy session to the pool with the session-level lock still held,
		// leaking a permanent already_running onto every future acquire
		// (review H-1). Terminate the session instead: a dead session releases
		// its advisory locks, and the pool discards the broken connection.
		killSession(context.Background(), conn)
		// Hand the dedicated connection back to the pool — same discipline as
		// T5 and release(). A *sql.Conn is a CHECKED-OUT pool slot: skipping
		// Close leaks one slot per failed probe, and a single-connection pool
		// (the SQLite test DB) then blocks on the next query forever.
		//
		// Returning it cannot leak the advisory lock back into the pool: the
		// session behind the handle was just terminated, and a terminated
		// session drops its session-level locks with it — that is exactly why
		// we kill it instead of relying on Close alone (see the DANGER note
		// above). The dead session is dropped by the pool on its next borrow
		// (same exposure as the unlock-failure path in release()).
		_ = conn.Close()
		logging.Warnf("daily-report gate: advisory lock probe failed (%v); falling back to in-process mutex", err)
		return true
	}
	if !locked {
		// T5: another instance holds the DB-level gate — roll the in-process
		// flag back and reject.
		_ = conn.Close()
		g.running.Store(false)
		return false
	}

	// T6: DB-level lock owned on this dedicated connection.
	g.mu.Lock()
	g.conn = conn
	g.mu.Unlock()
	return true
}

// ReleaseDailyReportGeneration releases the generation gate. Safe to call
// only by the owner (the path whose TryAcquire returned true). Idempotent
// against an already-free gate (R1); pairing discipline is acquire → defer
// release, so every terminal path (success, failure, panic) frees the gate.
// A failed unlock is logged and swallowed (R2): a dead connection means the
// session — and with it the advisory lock — has already been released by
// PostgreSQL, so no manual lock cleanup is ever needed.
func ReleaseDailyReportGeneration() {
	processGate.release()
}

func (g *dailyReportGate) release() {
	g.mu.Lock()
	conn := g.conn
	g.conn = nil
	g.mu.Unlock()

	if conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), gateProbeTimeout)
		defer cancel()
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", dailyReportGateKey); err != nil {
			// The unlock may have timed out client-side while actually
			// succeeding server-side — the session-level lock would then
			// survive inside the pooled connection and leak a permanent
			// already_running (review H-1, same window as T4). Terminate the
			// session to guarantee release; a truly dead connection fails the
			// kill harmlessly (PG already dropped the session and the lock).
			logging.Warnf("daily-report gate: unlock failed (%v); terminating the session to guarantee lock release", err)
			killSession(context.Background(), conn)
		}
		_ = conn.Close()
	}
	g.running.Store(false)
}

// killSession terminates the backend session behind conn (self-termination:
// pg_backend_pid() of the connection it runs on). Best-effort: failures are
// swallowed by the caller — either the session is already dead (lock gone)
// or the kill itself lands and PG releases the session's advisory locks.
func killSession(kctx context.Context, conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(kctx, gateProbeTimeout)
	defer cancel()
	_, _ = conn.ExecContext(ctx, "SELECT pg_terminate_backend(pg_backend_pid())")
}
