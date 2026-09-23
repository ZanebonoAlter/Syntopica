package datasources

import (
	"strings"
	"testing"
)

// TestEiaArchiveHostPassesProductionAllowlist is the regression for the first
// real signal report (result id=19, step 15): weeks=8 builds archived-edition
// URLs on https://www.eia.gov while the catalog allowlist only carried
// ir.eia.gov, so the fetch gate rejected every weeks>2 call with
// SOURCE_UNAVAILABLE「目标 host 不在核定白名单内」— production broken while
// eia_test.go stayed green (its httptest fetcher bypassed the production
// gate). The allowlist under test is derived from Catalog() exactly like the
// production stack (wiring.newStack), and assertions go through the real
// Fetcher gate (checkURL) — never a hand-written host list nor a test-local
// fetcher.
func TestEiaArchiveHostPassesProductionAllowlist(t *testing.T) {
	hosts := make([]string, 0, 4)
	for _, d := range Catalog() {
		hosts = append(hosts, d.HostAllowlist...)
	}
	fetcher := NewFetcher(hosts, DefaultFetchBudget())

	for _, u := range []string{
		"https://ir.eia.gov/wpsr/table1.csv", // live weekly file (default 2-period mode)
		// Archived edition the weeks>2 window path builds
		// ({eiaArchiveBase}/{YYYY}/{YYYY_MM_DD}/csv/table1.csv): previously
		// rejected by the gate, now must pass it.
		"https://www.eia.gov/petroleum/supply/weekly/archive/2026/2026_09_09/csv/table1.csv",
	} {
		if err := fetcher.checkURL(u); err != nil {
			t.Fatalf("production allowlist must accept %s, got: %v", u, err)
		}
	}

	// The gate stays closed for hosts no catalog entry allowlists.
	err := fetcher.checkURL("https://evil.example.com/petroleum/supply/weekly/archive/2026/2026_09_09/csv/table1.csv")
	if err == nil {
		t.Fatal("non-allowlisted host must be rejected by the production gate")
	}
	if !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("rejection must come from the allowlist gate, got: %v", err)
	}
}
