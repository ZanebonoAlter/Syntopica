package daily_report

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildDailyReportProgressMessageMatchesFrontendContract(t *testing.T) {
	msg := buildProgressMessage("job-1", "generating", 2849, "刚果（金）局势", 0, "0/1")

	require.Equal(t, "daily_report_progress", msg["type"])
	require.Equal(t, "job-1", msg["job_id"])
	require.Equal(t, uint(2849), msg["board_id"])
	require.Equal(t, "刚果（金）局势", msg["board_name"])
	require.Equal(t, "generating", msg["status"])
	require.Equal(t, 0, msg["saved"])
	require.Equal(t, "0/1", msg["progress"])
	require.NotEmpty(t, msg["timestamp"])
}

func TestBuildDailyReportDoneMessageMatchesFrontendContract(t *testing.T) {
	msg := buildDoneMessage("job-1", 1, 1)

	require.Equal(t, "daily_report_done", msg["type"])
	require.Equal(t, "job-1", msg["job_id"])
	require.Equal(t, 1, msg["total_saved"])
	require.Equal(t, 1, msg["total_boards"])
	require.NotEmpty(t, msg["timestamp"])
}
