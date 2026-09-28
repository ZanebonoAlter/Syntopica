package database

import (
	"bytes"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// TestSequenceNeedsHealBoundaryMatrix covers the six input classes required by
// the sequence-drift-self-heal spec (tasks 2.5): called/uncalled × behind/ahead,
// the empty table (MAX(id) is NULL → 0), and the exact tie (next == max(id)),
// which the spec explicitly says is still drift. It runs without a database —
// the decision is a pure function (design D6).
func TestSequenceNeedsHealBoundaryMatrix(t *testing.T) {
	cases := []struct {
		name       string
		lastValue  int64
		isCalled   bool
		maxID      int64
		wantHeal   bool
		wantTarget int64
	}{
		{"called and behind", 1, true, 34, true, 35},
		{"called and ahead", 40, true, 34, false, 35},
		{"uncalled and behind", 1, false, 34, true, 35},
		{"uncalled and ahead", 10, false, 5, false, 6},
		{"empty table", 1, false, 0, false, 1},
		{"next equals max", 34, false, 34, true, 35},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			needsHeal, target := SequenceNeedsHeal(tc.lastValue, tc.isCalled, tc.maxID)
			require.Equal(t, tc.wantHeal, needsHeal)
			require.Equal(t, tc.wantTarget, target)
		})
	}
}

// TestHealSequenceDriftSkipsWhenCatalogUnavailable covers the enumeration-error
// branch (tasks 4 「目录查询失败不影响启动」): on a database without the
// PostgreSQL catalogs the candidate query fails, the helper logs a WARN and
// reports zero repairs — it must not surface an error that would block startup
// (design D7).
func TestHealSequenceDriftSkipsWhenCatalogUnavailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	var buf bytes.Buffer
	logging.SetWriters(&buf, &buf)
	t.Cleanup(logging.ResetWriters)

	require.Zero(t, HealSequenceDrift(db))
	require.Contains(t, buf.String(), "Sequence drift self-check skipped",
		"a failed catalog enumeration must be logged as non-fatal")
}
