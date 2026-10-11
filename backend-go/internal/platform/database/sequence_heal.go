package database

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// SequenceNeedsHeal decides whether an integer primary-key sequence has drifted
// behind the rows of its table (fix-feed-delete-and-sequence-drift, design D5).
//
// next is the value the next nextval would hand out: last_value when the
// sequence has never been called (is_called=false), last_value+1 otherwise.
// Drift means next <= maxID — the next insert would collide with an id that is
// already taken. The repair target is maxID+1 with is_called=false, so the next
// nextval returns exactly maxID+1; an already-ahead sequence is never touched
// (setval does not roll back, so "never regress" is a hard requirement).
//
// Pure by design (tasks 2.1 / design D6): no database connection, so the
// boundary matrix (called/uncalled × behind/ahead × empty table × exact tie) is
// unit-testable without PostgreSQL sequence semantics. target is only
// meaningful when needsHeal is true.
func SequenceNeedsHeal(lastValue int64, isCalled bool, maxID int64) (needsHeal bool, target int64) {
	return nextSequenceValue(lastValue, isCalled) <= maxID, maxID + 1
}

// nextSequenceValue is the value the next nextval would return for a sequence in
// the given state. Shared by the pure decision and the repair log.
func nextSequenceValue(lastValue int64, isCalled bool) int64 {
	if isCalled {
		return lastValue + 1
	}
	return lastValue
}

// sequenceCandidate is one public-schema table that carries both an `id` column
// and the conventional `<table>_id_seq` sequence.
type sequenceCandidate struct {
	Schema string `gorm:"column:schema_name"`
	Table  string `gorm:"column:table_name"`
	Seq    string `gorm:"column:seq_name"`
}

// HealSequenceDrift scans the public schema for tables with an `id` column and a
// matching `<table>_id_seq` sequence, and realigns every sequence whose next
// nextval would collide with an existing row (design D4/D5). It returns the
// number of sequences repaired, for the caller's startup log and for tests.
//
// It is best-effort by design (design D7): every failure — catalog enumeration,
// reading one sequence, reading one table's MAX(id), or the setval itself — is
// logged (WARN) and never aborts application startup. The lone caller is
// StartRuntime, which runs before the scheduler and HTTP servers, so the
// MAX(id) snapshot is taken with no concurrent writers. Idempotent: no drift
// means no write.
func HealSequenceDrift(db *gorm.DB) int {
	healed, err := healSequenceDrift(db)
	if err != nil {
		logging.Warnf("Sequence drift self-check skipped (non-fatal): %v", err)
		return 0
	}
	return healed
}

// healSequenceDrift is the error-returning core of HealSequenceDrift: a catalog
// enumeration failure is the only error surfaced (the caller logs it); per-table
// failures are logged here and skipped so the remaining tables are still
// processed.
func healSequenceDrift(db *gorm.DB) (int, error) {
	candidates, err := listSequenceCandidates(db)
	if err != nil {
		return 0, err
	}

	healed, failed := 0, 0
	for _, candidate := range candidates {
		repaired, err := healSequenceCandidate(db, candidate)
		if err != nil {
			failed++
			logging.Warnf("Sequence drift self-check failed for table %q (non-fatal): %v", candidate.Table, err)
			continue
		}
		if repaired {
			healed++
		}
	}

	// 全部无漂移时留一条 INFO，使真库复验能区分「没跑」与「跑了但 0 漂移」。
	if healed == 0 && failed == 0 {
		logging.Infof("Sequence drift self-check: %d table(s) checked, 0 drifted", len(candidates))
	}
	return healed, nil
}

// listSequenceCandidates enumerates the tables eligible for drift detection:
// public-schema ordinary tables that have an `id` column AND the conventional
// `<table>_id_seq` sequence (composite-key tables without a single-column
// serial sequence never match). The condition set is the one verified against
// the local incident database; ORDER BY keeps repair logs deterministic.
func listSequenceCandidates(db *gorm.DB) ([]sequenceCandidate, error) {
	var candidates []sequenceCandidate
	err := db.Raw(`
		SELECT n.nspname AS schema_name, c.relname AS table_name, s.sequencename AS seq_name
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_sequences s ON s.sequencename = c.relname || '_id_seq' AND s.schemaname = n.nspname
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'id'
		WHERE c.relkind = 'r' AND n.nspname = 'public'
		ORDER BY c.relname
	`).Scan(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("list sequence candidates: %w", err)
	}
	return candidates, nil
}

// healSequenceCandidate reads one table's sequence state and MAX(id), decides
// via SequenceNeedsHeal, and realigns the sequence when it drifted. A repaired
// table is reported with a WARN line carrying the pre-repair next value, MAX(id)
// and the target, plus the usual root cause, so the incident is traceable after
// the fact.
func healSequenceCandidate(db *gorm.DB, candidate sequenceCandidate) (bool, error) {
	lastValue, isCalled, err := readSequenceState(db, candidate)
	if err != nil {
		return false, err
	}
	maxID, err := readTableMaxID(db, candidate)
	if err != nil {
		return false, err
	}

	needsHeal, target := SequenceNeedsHeal(lastValue, isCalled, maxID)
	if !needsHeal {
		return false, nil
	}

	// setval is non-transactional (BEGIN/ROLLBACK cannot cover it), so the call
	// is deliberately final and idempotent. The sequence name and the numeric
	// target are bound parameters — never spliced into SQL text.
	if err := db.Exec(
		"SELECT setval(?::regclass, ?, false)",
		pgx.Identifier{candidate.Schema, candidate.Seq}.Sanitize(),
		target,
	).Error; err != nil {
		return false, fmt.Errorf("setval %s.%s to %d: %w", candidate.Schema, candidate.Seq, target, err)
	}

	logging.Warnf(
		"Sequence drift repaired: table=%s sequence=%s next_would_be=%d max_id=%d new_next=%d; "+
			"cause is usually a TRUNCATE ... RESTART IDENTITY followed by a table-filtered "+
			"pg_restore that skipped SEQUENCE SET",
		candidate.Table, candidate.Seq, nextSequenceValue(lastValue, isCalled), maxID, target)
	return true, nil
}

// readSequenceState reads last_value/is_called straight from the sequence
// relation (pg_sequences has no is_called column). The identifier comes from the
// catalog and is quoted by pgx.Identifier.
func readSequenceState(db *gorm.DB, candidate sequenceCandidate) (int64, bool, error) {
	row := db.Raw("SELECT last_value, is_called FROM " +
		pgx.Identifier{candidate.Schema, candidate.Seq}.Sanitize()).Row()

	var lastValue int64
	var isCalled bool
	if err := row.Scan(&lastValue, &isCalled); err != nil {
		return 0, false, fmt.Errorf("read sequence %s.%s: %w", candidate.Schema, candidate.Seq, err)
	}
	return lastValue, isCalled, nil
}

// readTableMaxID returns COALESCE(MAX(id), 0) for the candidate table. The
// identifier comes from the catalog and is quoted by pgx.Identifier.
func readTableMaxID(db *gorm.DB, candidate sequenceCandidate) (int64, error) {
	row := db.Raw("SELECT COALESCE(MAX(id), 0) FROM " +
		pgx.Identifier{candidate.Schema, candidate.Table}.Sanitize()).Row()

	var maxID int64
	if err := row.Scan(&maxID); err != nil {
		return 0, fmt.Errorf("read max(id) of %s.%s: %w", candidate.Schema, candidate.Table, err)
	}
	return maxID, nil
}
