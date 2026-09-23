package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/datasources"
)

func wantInvalidArg(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want INVALID_ARGUMENT, got nil")
	}
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrInvalidArgument {
		t.Fatalf("want INVALID_ARGUMENT, got %v", err)
	}
}

// ── validateJodiArgs (pure-local validation, zero network) ──────────────────

func TestValidateJodiArgs(t *testing.T) {
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	if _, _, err := validateJodiArgs("US", "production", "", "", now); err != nil {
		t.Fatalf("valid defaults: %v", err)
	}
	// Defaults per flow: production→KBD, closing_stocks→KBBL.
	if _, unit, err := validateJodiArgs("US", "production", "", "", now); err != nil || unit != "KBD" {
		t.Fatalf("production default unit: unit=%q err=%v", unit, err)
	}
	if _, unit, err := validateJodiArgs("US", "closing_stocks", "", "", now); err != nil || unit != "KBBL" {
		t.Fatalf("closing_stocks default unit: unit=%q err=%v", unit, err)
	}
	// CONVBBL is rejected as a query unit (it is not a flow value unit).
	_, _, err := validateJodiArgs("US", "production", "CONVBBL", "", now)
	wantInvalidArg(t, err)
	// closing_stocks + KBD is a meaningless combo: rejected, no conversion.
	_, _, err = validateJodiArgs("US", "closing_stocks", "KBD", "", now)
	wantInvalidArg(t, err)
	// Bad flow / geo / unit / month shapes.
	for _, args := range [][4]string{
		{"US", "production2", "", ""},
		{"us", "production", "", ""},
		{"USA", "production", "", ""},
		{"US", "production", "bbl", ""},
		{"US", "production", "", "20261"},
		{"US", "production", "", "2026-13"},
		{"US", "production", "", "1999-01"},
		{"US", "production", "", "2026-10"}, // future month relative to now
	} {
		_, _, err := validateJodiArgs(args[0], args[1], args[2], args[3], now)
		wantInvalidArg(t, err)
	}
}

// ── parseJodiPrimary over the energy-mcp US fixture ──────────────────────────

func TestParseJodiPrimaryFixture(t *testing.T) {
	rows, err := parseJodiPrimary(loadFixture(t, "jodi-us.csv"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("fixture must yield rows")
	}
	// US,2026-01,CRUDEOIL,CLOSTLV,KBBL,676671.0000,1 (explore-findings sample)
	var found bool
	for _, r := range rows {
		if r.geo == "US" && r.period == "2026-01" && r.product == "CRUDEOIL" && r.flow == "CLOSTLV" && r.unit == "KBBL" {
			found = true
			if !r.hasValue || r.value != 676671 {
				t.Fatalf("CLOSTLV KBBL 2026-01: got has=%v val=%v", r.hasValue, r.value)
			}
			if r.assessmentCode != "1" {
				t.Fatalf("assessment code passthrough: %q", r.assessmentCode)
			}
		}
	}
	if !found {
		t.Fatal("sample row not found")
	}
	// 'x' marker (KBD × closing stocks) maps to null, marker kept.
	for _, r := range rows {
		if r.geo == "US" && r.product == "CRUDEOIL" && r.flow == "CLOSTLV" && r.unit == "KBD" && !r.hasValue {
			if r.rawValue != "x" || r.missingReason == "" {
				t.Fatalf("'x' marker must be null+kept: %+v", r)
			}
		}
	}
}

func TestParseJodiPrimaryDrift(t *testing.T) {
	base := string(loadFixture(t, "jodi-us.csv"))
	// Unknown OBS_VALUE marker → SCHEMA_CHANGED (never 0, never skipped).
	bad := strings.Replace(base, "676671.0000", "???", 1)
	_, err := parseJodiPrimary([]byte(bad))
	wantSchemaChanged(t, err) // unknown marker
	// Column count drift.
	short := strings.Replace(base, "US,2026-01,CRUDEOIL,CLOSTLV,CONVBBL,7400.0000,3",
		"US,2026-01,CRUDEOIL,CLOSTLV,CONVBBL", 1)
	if _, err := parseJodiPrimary([]byte(short)); err == nil {
		t.Fatal("column drift must fail")
	}
	// Header drift.
	hdr := strings.Replace(base, "REF_AREA", "AREA", 1)
	if _, err := parseJodiPrimary([]byte(hdr)); err == nil {
		t.Fatal("header drift must fail")
	}
}

// ── Fetch over httptest ──────────────────────────────────────────────────────

// newJODIOverServer serves per-year bodies: map[year]body (absent year → 404).
func newJODIOverServer(t *testing.T, byYear map[string][]byte) (*JODI, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		year := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/primaryyear"), ".csv")
		body, ok := byYear[year]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 32 << 20})
	f.SetClient(testClient())
	j := NewJODI(f, datasources.NewTTLCache(time.Minute))
	j.urlTemplate = srv.URL + "/primaryyear%d.csv"
	return j, &calls
}

func TestJodiFetchExplicitMonth(t *testing.T) {
	j, _ := newJODIOverServer(t, map[string][]byte{"2026": loadFixture(t, "jodi-us.csv")})
	res, err := j.Fetch(context.Background(), "US", "closing_stocks", "KBBL", "2026-01")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res["year_used"].(int) != 2026 {
		t.Fatalf("year_used: %v", res["year_used"])
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 1 {
		t.Fatalf("want 1 observation, got %d", len(obs))
	}
	if obs[0]["value"].(float64) != 676671 {
		t.Fatalf("value: %v", obs[0]["value"])
	}
	if obs[0]["period"].(string) != "2026-01" {
		t.Fatalf("period: %v", obs[0]["period"])
	}
}

func TestJodiFetchLatestPeriodWithoutFallback(t *testing.T) {
	j, _ := newJODIOverServer(t, map[string][]byte{"2026": loadFixture(t, "jodi-us.csv")})
	res, err := j.Fetch(context.Background(), "US", "production", "KBD", "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	// Latest period in the 2026 file is 2026-06 (13818.0 KBD per evidence).
	if obs[0]["period"].(string) != "2026-06" {
		t.Fatalf("latest period: %v", obs[0]["period"])
	}
	if obs[0]["value"].(float64) != 13818 {
		t.Fatalf("latest value: %v", obs[0]["value"])
	}
}

func TestJodiFetchLatestNullNotSilentlyOlder(t *testing.T) {
	// Build a body whose LATEST period value is a missing marker: the tool
	// must return the latest period with null, never an older value.
	body := "REF_AREA,TIME_PERIOD,ENERGY_PRODUCT,FLOW_BREAKDOWN,UNIT_MEASURE,OBS_VALUE,ASSESSMENT_CODE\n" +
		"US,2026-05,CRUDEOIL,INDPROD,KBD,13000.0000,1\n" +
		"US,2026-06,CRUDEOIL,INDPROD,KBD,..,2\n"
	j, _ := newJODIOverServer(t, map[string][]byte{"2026": []byte(body)})
	res, err := j.Fetch(context.Background(), "US", "production", "", "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 1 || obs[0]["period"] != "2026-06" || obs[0]["value"] != nil {
		t.Fatalf("latest null must not fall back: %+v", obs)
	}
	if obs[0]["missing_reason"] == "" {
		t.Fatal("missing_reason required for null latest")
	}
}

func TestJodiFetch404FallbackOnce(t *testing.T) {
	// Current year 404 (month unspecified) → fall back to previous year once.
	j, _ := newJODIOverServer(t, map[string][]byte{"2025": loadFixture(t, "jodi-us.csv")})
	res, err := j.Fetch(context.Background(), "US", "production", "", "")
	if err != nil {
		t.Fatalf("fetch with fallback: %v", err)
	}
	if res["year_used"].(int) != 2025 {
		t.Fatalf("year_used must be 2025, got %v", res["year_used"])
	}
	if !strings.Contains(res["year_strategy"].(string), "fallback") {
		t.Fatalf("strategy must note fallback: %v", res["year_strategy"])
	}

	// Explicit month + 404 → straight error naming the year, NO fallback.
	j2, _ := newJODIOverServer(t, map[string][]byte{"2025": loadFixture(t, "jodi-us.csv")})
	_, err = j2.Fetch(context.Background(), "US", "production", "", "2026-01")
	if err == nil {
		t.Fatal("explicit-month 404 must fail")
	}
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("want SOURCE_UNAVAILABLE, got %v", err)
	} else if !strings.Contains(err.Error(), "2026") {
		t.Fatalf("error must name the year: %v", err)
	}

	// Double 404 → SOURCE_UNAVAILABLE.
	j3, _ := newJODIOverServer(t, map[string][]byte{})
	_, err = j3.Fetch(context.Background(), "US", "production", "", "")
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("double 404: want SOURCE_UNAVAILABLE, got %v", err)
	}
}

func TestJodiFetchCachesPerYear(t *testing.T) {
	j, calls := newJODIOverServer(t, map[string][]byte{"2026": loadFixture(t, "jodi-us.csv")})
	ctx := context.Background()
	if _, err := j.Fetch(ctx, "US", "production", "", ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := j.Fetch(ctx, "US", "closing_stocks", "KBBL", "2026-01"); err != nil {
		t.Fatalf("second (cache hit): %v", err)
	}
	if *calls != 1 {
		t.Fatalf("per-year TTL cache must serve the second call, calls = %d", *calls)
	}
}

func TestJodiFetchConflictingDuplicates(t *testing.T) {
	body := "REF_AREA,TIME_PERIOD,ENERGY_PRODUCT,FLOW_BREAKDOWN,UNIT_MEASURE,OBS_VALUE,ASSESSMENT_CODE\n" +
		"US,2026-06,CRUDEOIL,INDPROD,KBD,13818.0000,1\n" +
		"US,2026-06,CRUDEOIL,INDPROD,KBD,99999.0000,1\n"
	j, _ := newJODIOverServer(t, map[string][]byte{"2026": []byte(body)})
	_, err := j.Fetch(context.Background(), "US", "production", "", "")
	wantSchemaChanged(t, err)

	// Identical duplicates merge, source_rows preserved.
	body2 := "REF_AREA,TIME_PERIOD,ENERGY_PRODUCT,FLOW_BREAKDOWN,UNIT_MEASURE,OBS_VALUE,ASSESSMENT_CODE\n" +
		"US,2026-06,CRUDEOIL,INDPROD,KBD,13818.0000,1\n" +
		"US,2026-06,CRUDEOIL,INDPROD,KBD,13818.0000,1\n"
	j2, _ := newJODIOverServer(t, map[string][]byte{"2026": []byte(body2)})
	res, err := j2.Fetch(context.Background(), "US", "production", "", "")
	if err != nil {
		t.Fatalf("identical duplicates must merge: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 1 {
		t.Fatalf("merged into one observation, got %d", len(obs))
	}
	if _, ok := obs[0]["source_rows"]; !ok {
		t.Fatal("merged duplicate must expose source_rows")
	}
}

func TestJodiFetchNoDataIsNotError(t *testing.T) {
	// A legal query with zero matches (unknown geo in file) is no_data, not
	// an error (spec: 三类错误可区分, 常规无数据不与错误混淆).
	j, _ := newJODIOverServer(t, map[string][]byte{"2026": loadFixture(t, "jodi-us.csv")})
	res, err := j.Fetch(context.Background(), "JP", "production", "", "")
	if err != nil {
		t.Fatalf("no-data must not be an error: %v", err)
	}
	if res["no_data"] != true {
		t.Fatalf("no_data flag: %v", res["no_data"])
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 0 {
		t.Fatalf("observations must be empty, got %d", len(obs))
	}
}

var _ = fmt.Sprintf // keep fmt if future edits need it

// ── explicit multi-year window (HR-2..HR-5) ─────────────────────────────────

// jodiYearBody synthesizes one annual CSV for a year with 12 INDPROD months
// (per-month distinct values so ordering/coverage assertions are exact).
func jodiYearBody(year int) []byte {
	var b strings.Builder
	b.WriteString("REF_AREA,TIME_PERIOD,ENERGY_PRODUCT,FLOW_BREAKDOWN,UNIT_MEASURE,OBS_VALUE,ASSESSMENT_CODE\n")
	for m := 1; m <= 12; m++ {
		fmt.Fprintf(&b, "US,%04d-%02d,CRUDEOIL,INDPROD,KBD,%d.0000,1\n", year, m, 10000+(year%100)*100+m)
	}
	return []byte(b.String())
}

func TestValidateJodiWindow(t *testing.T) {
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)

	// Absent → nil window (default / explicit-month mode unchanged, HR-2).
	w, err := validateJodiWindow("", nil, now)
	if err != nil || w != nil {
		t.Fatalf("absent years: window=%v err=%v", w, err)
	}
	w, err = validateJodiWindow("2026-01", nil, now)
	if err != nil || w != nil {
		t.Fatalf("explicit month only: window=%v err=%v", w, err)
	}
	// Explicit range → newest-first calendar-year window (HR-3).
	w, err = validateJodiWindow("", []int{3}, now)
	if err != nil || len(w) != 3 || w[0] != 2026 || w[2] != 2024 {
		t.Fatalf("years=3: window=%v err=%v", w, err)
	}
	// Out of range / non-single / mutex / window past the supported floor.
	for _, years := range [][]int{{0}, {6}, {1, 2}} {
		if _, err := validateJodiWindow("", years, now); err == nil {
			t.Fatalf("years=%v must be rejected", years)
		}
	}
	if _, err := validateJodiWindow("2026-01", []int{3}, now); err == nil {
		t.Fatal("years + month must be mutually exclusive (HR-3)")
	}
	if _, err := validateJodiWindow("", []int{5}, time.Date(2004, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("window reaching below jodiMinYear must be rejected")
	}
}

func TestJodiFetchYearsWindow(t *testing.T) {
	y := time.Now().UTC().Year()
	j, calls := newJODIOverServer(t, map[string][]byte{
		fmt.Sprintf("%d", y):   loadFixture(t, "jodi-us.csv"), // 6 months 2026-01..06
		fmt.Sprintf("%d", y-1): jodiYearBody(y - 1),
		fmt.Sprintf("%d", y-2): jodiYearBody(y - 2),
	})
	res, err := j.Fetch(context.Background(), "US", "production", "KBD", "", 3)
	if err != nil {
		t.Fatalf("years fetch: %v", err)
	}
	if got := res["years_used"].([]int); len(got) != 3 || got[0] != y || got[2] != y-2 {
		t.Fatalf("years_used: %v", got)
	}
	if gaps := res["year_gaps"].([]map[string]any); len(gaps) != 0 {
		t.Fatalf("no gaps expected: %v", gaps)
	}
	obs := res["observations"].([]map[string]any)
	// 6 months from the real fixture + 12 from each synthetic year.
	if len(obs) != 30 {
		t.Fatalf("want 30 observations (all available months), got %d", len(obs))
	}
	if obs[0]["period"].(string) != fmt.Sprintf("%d-01", y-2) || obs[29]["period"].(string) != fmt.Sprintf("%d-06", y) {
		t.Fatalf("observations must span the window ascending: first=%v last=%v", obs[0], obs[29])
	}
	if obs[29]["value"].(float64) != 13818 {
		t.Fatalf("newest observation must come from the real fixture: %v", obs[29])
	}
	if obs[0]["source_year"].(int) != y-2 || obs[29]["source_year"].(int) != y {
		t.Fatalf("source_year provenance: %v / %v", obs[0]["source_year"], obs[29]["source_year"])
	}
	if *calls != 3 {
		t.Fatalf("one fetch per distinct annual file, calls = %d", *calls)
	}
	// A repeated years call replays the per-year cache with zero new fetches
	// (month/years are post-filter dimensions; keys stay per-year, HR-6).
	if _, err := j.Fetch(context.Background(), "US", "production", "KBD", "", 3); err != nil {
		t.Fatalf("repeated years fetch: %v", err)
	}
	if *calls != 3 {
		t.Fatalf("per-year cache must serve the repeat, calls = %d", *calls)
	}
}

func TestJodiFetchYearsCurrentYear404FallbackDedupe(t *testing.T) {
	y := time.Now().UTC().Year()
	// Current year 404, window ≥2: the fallback target (y-1) is already in
	// the window and is fetched EXACTLY once (dedupe, HR-4).
	j, calls := newJODIOverServer(t, map[string][]byte{
		fmt.Sprintf("%d", y-1): loadFixture(t, "jodi-us.csv"),
		fmt.Sprintf("%d", y-2): jodiYearBody(y - 2),
	})
	res, err := j.Fetch(context.Background(), "US", "production", "", "", 3)
	if err != nil {
		t.Fatalf("years with current-year 404: %v", err)
	}
	if got := res["years_used"].([]int); len(got) != 2 || got[0] != y-1 {
		t.Fatalf("years_used must skip the 404 year: %v", got)
	}
	gaps := res["year_gaps"].([]map[string]any)
	if len(gaps) != 1 || gaps[0]["year"] != y {
		t.Fatalf("current-year 404 must be a recorded gap: %v", gaps)
	}
	if !strings.Contains(res["year_strategy"].(string), "不重复请求") {
		t.Fatalf("strategy must note the dedupe: %v", res["year_strategy"])
	}
	if *calls != 3 { // y(404) + y-1 + y-2, never a second y-1 request
		t.Fatalf("fallback target must be fetched once only, calls = %d", *calls)
	}

	// years=1 + current-year 404 keeps the default-mode parity: fall back to
	// the previous year ONCE, annotated.
	j2, _ := newJODIOverServer(t, map[string][]byte{
		fmt.Sprintf("%d", y-1): loadFixture(t, "jodi-us.csv"),
	})
	res, err = j2.Fetch(context.Background(), "US", "production", "", "", 1)
	if err != nil {
		t.Fatalf("years=1 fallback: %v", err)
	}
	if res["year_used"].(int) != y-1 || !strings.Contains(res["year_strategy"].(string), "fallback") {
		t.Fatalf("years=1 fallback parity: used=%v strategy=%v", res["year_used"], res["year_strategy"])
	}
}

func TestJodiFetchYearsHistorical404GapNoCascade(t *testing.T) {
	y := time.Now().UTC().Year()
	// A historical year's 404 is a gap; the chain must NOT probe y-3 (HR-5).
	j, calls := newJODIOverServer(t, map[string][]byte{
		fmt.Sprintf("%d", y):   loadFixture(t, "jodi-us.csv"),
		fmt.Sprintf("%d", y-1): jodiYearBody(y - 1),
	})
	res, err := j.Fetch(context.Background(), "US", "production", "", "", 3)
	if err != nil {
		t.Fatalf("historical gap must not fail the call: %v", err)
	}
	if got := res["years_used"].([]int); len(got) != 2 || got[0] != y {
		t.Fatalf("years_used: %v", got)
	}
	gaps := res["year_gaps"].([]map[string]any)
	if len(gaps) != 1 || gaps[0]["year"] != y-2 {
		t.Fatalf("gap must name the missing historical year: %v", gaps)
	}
	if *calls != 3 { // y + y-1 + y-2(404); y-3 never requested
		t.Fatalf("no cascading fallback beyond the gap, calls = %d", *calls)
	}
}

func TestJodiFetchYearsNon404FailsWholeCall(t *testing.T) {
	y := time.Now().UTC().Year()
	// A 5xx on a historical year is a transient failure, not an honest data
	// gap: the whole call fails SOURCE_UNAVAILABLE (never partial output).
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		year := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/primaryyear"), ".csv")
		if year == fmt.Sprintf("%d", y-1) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(loadFixture(t, "jodi-us.csv"))
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 32 << 20})
	f.SetClient(testClient())
	j := NewJODI(f, datasources.NewTTLCache(time.Minute))
	j.urlTemplate = srv.URL + "/primaryyear%d.csv"

	_, err := j.Fetch(context.Background(), "US", "production", "", "", 2)
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("non-404 must fail the whole call, got %v", err)
	}
}

func TestJodiFetchYearsMonthMutexRejectedBeforeNetwork(t *testing.T) {
	j, calls := newJODIOverServer(t, map[string][]byte{
		"2026": loadFixture(t, "jodi-us.csv"),
	})
	_, err := j.Fetch(context.Background(), "US", "production", "", "2026-01", 3)
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrInvalidArgument {
		t.Fatalf("years + month must be INVALID_ARGUMENT, got %v", err)
	}
	if *calls != 0 {
		t.Fatalf("mutex rejection must happen before any network call, calls = %d", *calls)
	}
}
