package daily_report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeReportDateKeepsRequestedDate(t *testing.T) {
	requested, err := time.ParseInLocation("2006-01-02", "2026-05-26", time.FixedZone("UTC+8", 8*60*60))
	require.NoError(t, err)

	got := normalizeReportDate(requested)

	require.Equal(t, "2026-05-26", got.Format("2006-01-02"))
	require.Equal(t, time.UTC, got.Location())
	require.Equal(t, 12, got.Hour())
}
