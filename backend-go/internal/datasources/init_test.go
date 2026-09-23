package datasources

import (
	"strings"
	"testing"
)

// ── Init seeding with the live resolver (task 3.9 状态源统一) ──
//
// Init used to build a config-only resolver internally, ignoring the UI
// chain (wiring.ComtradeKeyResolver), so the catalog status stayed disabled
// even after the key was configured in the UI. These tests pin the new
// contract: Init takes the resolver its caller supplies, and the UI save
// path flips the status live via UpdateStatus — no restart.

// emptyResolver / uiKeyResolver stand in for the live resolver chains main
// injects (nothing configured / COMTRADE_API_KEY present).
func emptyResolver(configKeyName string) string { return "" }

func uiKeyResolver(configKeyName string) string {
	if configKeyName == "COMTRADE_API_KEY" {
		return "test-key"
	}
	return ""
}

// TestInitEmptyKeyDisablesComtradeWithConfigKeyReason: no key anywhere →
// un_comtrade seeded disabled with a reason naming the config key; every
// key-free source seeds enabled.
func TestInitEmptyKeyDisablesComtradeWithConfigKeyReason(t *testing.T) {
	db := setupTestDB(t)
	Init(db, emptyResolver)

	rows, err := ListCatalog(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.Code != "un_comtrade" {
			if r.Status != StatusEnabled {
				t.Fatalf("%s requires no key, want enabled, got %s (%s)", r.Code, r.Status, r.StatusReason)
			}
			continue
		}
		if r.Status != StatusDisabled {
			t.Fatalf("comtrade without key must be disabled, got %s", r.Status)
		}
		if !strings.Contains(r.StatusReason, "COMTRADE_API_KEY") {
			t.Fatalf("reason must point at the config key: %q", r.StatusReason)
		}
	}
}

// TestInitKeyPresentEnablesComtrade: the live resolver reporting a configured
// key → un_comtrade seeds enabled with no reason.
func TestInitKeyPresentEnablesComtrade(t *testing.T) {
	db := setupTestDB(t)
	Init(db, uiKeyResolver)

	rows, err := ListCatalog(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.Code != "un_comtrade" {
			continue
		}
		if r.Status != StatusEnabled {
			t.Fatalf("comtrade with key must be enabled, got %s (%s)", r.Status, r.StatusReason)
		}
		if r.StatusReason != "" {
			t.Fatalf("enabled row must carry no reason, got %q", r.StatusReason)
		}
	}
}

// TestUpdateStatusAfterKeySaveFlipsWithoutRestart covers the UI save path
// (SaveComtradeSettings → UpdateStatus): seed with no key (disabled), then
// refresh the single row with a resolver that now sees the key — the status
// flips enabled WITHOUT re-running Init (no restart), and only the status
// columns move.
func TestUpdateStatusAfterKeySaveFlipsWithoutRestart(t *testing.T) {
	db := setupTestDB(t)
	Init(db, emptyResolver)

	if err := UpdateStatus(db, "un_comtrade", uiKeyResolver); err != nil {
		t.Fatalf("update status: %v", err)
	}
	rows, err := ListCatalog(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range rows {
		if r.Code == "un_comtrade" {
			if r.Status != StatusEnabled || r.StatusReason != "" {
				t.Fatalf("saved key must flip status live: got %s (%s)", r.Status, r.StatusReason)
			}
		}
	}

	// And the reverse direction: a cleared key disables again (same live
	// path, e.g. save with enabled=false / empty key).
	if err := UpdateStatus(db, "un_comtrade", emptyResolver); err != nil {
		t.Fatalf("update status back: %v", err)
	}
	rows, _ = ListCatalog(db)
	for _, r := range rows {
		if r.Code == "un_comtrade" && r.Status != StatusDisabled {
			t.Fatalf("cleared key must disable again, got %s", r.Status)
		}
	}

	// Unknown source → explicit error, not a silent no-op.
	if err := UpdateStatus(db, "no_such_source", uiKeyResolver); err == nil {
		t.Fatal("unknown source must error")
	}
}
