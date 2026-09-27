package notification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// databaseDB returns the global test DB wired by testutil.SetupTestDB.
// Re-reading the global (instead of capturing) keeps helpers correct after
// ResetTestData swaps the pool.
func databaseDB() *gorm.DB { return database.DB }

// createdAtX returns a distinct timestamp minutesAgo in the past, giving
// deterministic created_at ordering for eviction tests.
func createdAtX(minutesAgo int) time.Time {
	return time.Now().Add(-time.Duration(minutesAgo) * time.Minute)
}

// mustListAll returns all notification rows newest-first (List semantics).
func mustListAll(t *testing.T) []models.Notification {
	t.Helper()
	rows, _, err := List(false, 100, 0)
	require.NoError(t, err, "list notifications")
	return rows
}
