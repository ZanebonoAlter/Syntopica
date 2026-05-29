package database

import (
	"strings"
	"testing"
)

func TestSanitizeSlowSQLTruncatesLongSQL(t *testing.T) {
	sql := "SELECT '" + strings.Repeat("0.1,", 1000) + "0.2'::vector"

	got := sanitizeSlowSQL(sql)

	if len(got) >= len(sql) {
		t.Fatalf("expected SQL to be truncated, got length %d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}
