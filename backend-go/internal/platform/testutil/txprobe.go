package testutil

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
)

// BeginProbe observes whether the first database transaction has begun on an
// isolated gorm handle, so tests can attribute "external" (non-DB) calls —
// e.g. LLM embedder invocations — to before/after that point.
//
// Motivation (fix-aux-label-idle-tx-cache design D1): the idle-in-transaction
// root cause was external AI API calls made inside open transactions. The
// probe cannot wrap *sql.Tx commits (BeginTx's concrete return type is
// opaque), so Began() is monotonic for the handle's lifetime — exactly what
// the assertion needs: "no external call happens after the first BEGIN" is
// equivalent to "no external call happens inside any transaction" for a
// service under test that runs its slow I/O strictly before its writes.
//
// The handle is a SEPARATE gorm.DB on the shared test-container DSN (its own
// connection pool), so swapping ConnPool never leaks into other tests.
//
// Usage:
//
//	db := SetupTestDB(t) // reset data via the shared handle
//	probeDB, probe := testutil.NewBeginProbeDB(t)
//	embedder := func(...) { probe.NoteExternal("embedder"); ... }
//	// run the service on probeDB, then:
//	require.Empty(t, probe.Violations())
type BeginProbe struct {
	mu         sync.Mutex
	began      bool
	violations []string
}

// Began reports whether at least one transaction has begun on the probe.
func (p *BeginProbe) Began() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.began
}

// NoteExternal records an external (non-DB) call; it becomes a violation when
// a transaction has already begun.
func (p *BeginProbe) NoteExternal(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.began {
		p.violations = append(p.violations, name)
	}
}

// Violations returns the external calls observed inside a transaction.
func (p *BeginProbe) Violations() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.violations))
	copy(out, p.violations)
	return out
}

// beginProbePool delegates every ConnPool method to the wrapped pool and only
// intercepts BeginTx to flip the probe's began flag.
type beginProbePool struct {
	gorm.ConnPool
	probe *BeginProbe
}

func (p *beginProbePool) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	tx, err := p.ConnPool.(gorm.TxBeginner).BeginTx(ctx, opts)
	if err == nil {
		p.probe.mu.Lock()
		p.probe.began = true
		p.probe.mu.Unlock()
	}
	return tx, err
}

// NewBeginProbeDB opens an isolated gorm.DB against the shared test-container
// DSN whose connection pool records transaction begins, plus the probe.
// Call SetupTestDB(t) first so the schema exists and data is reset.
func NewBeginProbeDB(t *testing.T) (*gorm.DB, *BeginProbe) {
	t.Helper()

	if startErr != nil || cachedDSN == "" {
		t.Fatalf("test container not started; call SetupTestDB first (startErr=%v)", startErr)
	}
	db, err := gorm.Open(postgres.Open(cachedDSN), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   database.NewSlowLogger(200 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("open isolated probe handle: %v", err)
	}
	probe := &BeginProbe{}
	pool := &beginProbePool{ConnPool: db.ConnPool, probe: probe}
	db.ConnPool = pool
	db.Statement.ConnPool = pool
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, probe
}
