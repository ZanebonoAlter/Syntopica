package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Pure-function tests for the manual 首义 rewrite helpers
// (lane-manual-correction). No DB — runs under `go test -short`.

// TestDedupeSectionIDs pins the selection-pool dedup: duplicates collapse,
// the caller's order survives.
func TestDedupeSectionIDs(t *testing.T) {
	assert.Equal(t, []uint{3, 1, 2}, dedupeSectionIDs([]uint{3, 1, 3, 2, 1}))
	assert.Empty(t, dedupeSectionIDs(nil))
}

// TestAnchorInputSummary pins the audit trail's「输入摘要」shape: the chosen
// section ids as a JSON array.
func TestAnchorInputSummary(t *testing.T) {
	assert.Equal(t, "[101,105,110]", anchorInputSummary([]uint{101, 105, 110}))
	assert.Equal(t, "[]", anchorInputSummary(nil))
}

// TestRefreshCentroidsIsolated_KeepsGoingAfterFailure covers spec「重算失败
// 保留待重算标记、不中断其余话题的刷新」at the pure-function level: the
// failing topic is skipped (its flag is never cleared), the rest are refreshed
// and cleared.
func TestRefreshCentroidsIsolated_KeepsGoingAfterFailure(t *testing.T) {
	cleared := make([]uint, 0, 2)
	updated, failed := refreshCentroidsIsolated(
		[]uint{1, 2, 3},
		func(topicID uint) (int64, error) { return 0, nil },
		func(topicID uint) error {
			if topicID == 2 {
				return assert.AnError
			}
			return nil
		},
		func(topicID uint, version int64) error {
			cleared = append(cleared, topicID)
			return nil
		},
	)

	assert.Equal(t, 2, updated)
	assert.Equal(t, 1, failed)
	assert.Equal(t, []uint{1, 3}, cleared, "失败的话题不清标记，其余照常清")
}

// TestRefreshCentroidsIsolated_KeepsFlagWhenSnapshotFails covers review R-11's
// safe direction: when the trail version cannot be read, the pending flag is
// kept (the next round recomputes again — idempotent) instead of being wiped
// blind.
func TestRefreshCentroidsIsolated_KeepsFlagWhenSnapshotFails(t *testing.T) {
	cleared := make([]uint, 0, 1)
	updated, failed := refreshCentroidsIsolated(
		[]uint{1},
		func(topicID uint) (int64, error) { return 0, assert.AnError },
		func(topicID uint) error { return nil },
		func(topicID uint, version int64) error {
			cleared = append(cleared, topicID)
			return nil
		},
	)

	assert.Equal(t, 1, updated)
	assert.Equal(t, 0, failed)
	assert.Empty(t, cleared, "读不到留痕版本时不清标记（安全方向）")
}
