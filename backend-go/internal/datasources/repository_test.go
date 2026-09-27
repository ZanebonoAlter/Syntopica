package datasources

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&DataSource{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestUpsertCatalogSeedsAllSources(t *testing.T) {
	db := setupTestDB(t)
	if err := UpsertCatalog(db, func(string) string { return "" }); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rows, err := ListCatalog(db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("want 4 sources, got %d", len(rows))
	}
	want := []string{"eia_wpsr", "jodi_oil_primary", "un_comtrade", "wb_wdi"}
	for i, r := range rows {
		if r.Code != want[i] {
			t.Fatalf("row %d: want %s got %s", i, want[i], r.Code)
		}
	}
}

func TestUpsertCatalogKeyMissingDisablesComtrade(t *testing.T) {
	db := setupTestDB(t)
	if err := UpsertCatalog(db, func(string) string { return "" }); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rows, _ := ListCatalog(db)
	for _, r := range rows {
		if r.Code == "un_comtrade" {
			if r.Status != StatusDisabled {
				t.Fatalf("comtrade without key must be disabled, got %s", r.Status)
			}
			if !strings.Contains(r.StatusReason, "COMTRADE_API_KEY") {
				t.Fatalf("reason must name the config key: %q", r.StatusReason)
			}
			continue
		}
		if r.Status != StatusEnabled {
			t.Fatalf("%s must be enabled, got %s", r.Code, r.Status)
		}
	}
}

func TestUpsertCatalogKeyPresentEnablesComtrade(t *testing.T) {
	db := setupTestDB(t)
	resolve := func(name string) string {
		if name == "COMTRADE_API_KEY" {
			return "test-key"
		}
		return ""
	}
	if err := UpsertCatalog(db, resolve); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rows, _ := ListCatalog(db)
	for _, r := range rows {
		if r.Status != StatusEnabled {
			t.Fatalf("%s must be enabled, got %s", r.Code, r.Status)
		}
	}
}

func TestUpsertCatalogIdempotentPreservesRuntimeColumns(t *testing.T) {
	db := setupTestDB(t)
	if err := UpsertCatalog(db, func(string) string { return "" }); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Simulate a probe having written runtime state.
	probeAt := time.Now().UTC()
	if err := db.Model(&DataSource{}).Where("code = ?", "eia_wpsr").
		Updates(map[string]any{"last_probe_at": probeAt, "last_error": "upstream blip"}).Error; err != nil {
		t.Fatalf("seed runtime state: %v", err)
	}
	// Second startup upsert: idempotent, runtime columns preserved (M19).
	if err := UpsertCatalog(db, func(string) string { return "key" }); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	rows, _ := ListCatalog(db)
	if len(rows) != 4 {
		t.Fatalf("re-upsert must not duplicate rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Code == "eia_wpsr" {
			if r.LastProbeAt == nil || !r.LastProbeAt.Equal(probeAt) {
				t.Fatalf("last_probe_at must be preserved: %v", r.LastProbeAt)
			}
			if r.LastError != "upstream blip" {
				t.Fatalf("last_error must be preserved: %q", r.LastError)
			}
		}
		if r.Code == "un_comtrade" && r.Status != StatusEnabled {
			t.Fatalf("status must be recomputed with key present, got %s", r.Status)
		}
	}
}

func TestRowToDTOTopicsArray(t *testing.T) {
	r := DataSource{Code: "x", TopicsJSON: `["crude-oil","stocks"]`, Status: StatusEnabled}
	dto := r.ToDTO()
	if len(dto.Topics) != 2 || dto.Topics[0] != "crude-oil" {
		t.Fatalf("topics must decode to array: %v", dto.Topics)
	}
	// Malformed topics JSON degrades to empty array, not a failure.
	r2 := DataSource{Code: "y", TopicsJSON: "{bad", Status: StatusEnabled}
	if got := r2.ToDTO().Topics; len(got) != 0 || got == nil {
		t.Fatalf("malformed topics must degrade to empty array: %v", got)
	}
}
