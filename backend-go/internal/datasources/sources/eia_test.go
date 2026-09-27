package sources

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/datasources"
)

// ── unit helpers (translated from energy-mcp test_eia.py) ───────────────────

func TestParseNumber(t *testing.T) {
	cases := []struct {
		cell   string
		val    float64
		has    bool
		misses bool // expect a missing marker (not an unknown marker)
	}{
		{`13,862`, 13862, true, false},
		{`424.460`, 424.46, true, false},
		{"-7.572", -7.572, true, false},
		{"0", 0, true, false}, // real zero stays zero
		{"0.0000", 0, true, false},
		{"– –", 0, false, true}, // en-dash pair → null marker
		{"–", 0, false, true},
		{"", 0, false, true},          // empty → null marker
		{"NaN", 0, false, false},      // non-finite → unknown (caller: SCHEMA_CHANGED)
		{"Infinity", 0, false, false}, // ditto
		{"1e999", 0, false, false},    // overflow → unknown
		{"abc", 0, false, false},      // unknown drift
	}
	for _, c := range cases {
		val, has, missing := parseNumber(c.cell)
		if has != c.has || (c.has && val != c.val) {
			t.Errorf("parseNumber(%q) = (%v,%v,%q), want has=%v val=%v", c.cell, val, has, missing, c.has, c.val)
		}
		if c.misses && missing == "" {
			t.Errorf("parseNumber(%q): expected a missing marker", c.cell)
		}
		if !c.has && !c.misses && missing != "" {
			t.Errorf("parseNumber(%q): unknown content must NOT be flagged as a known marker", c.cell)
		}
	}
}

func TestParseMDY(t *testing.T) {
	if got := parseMDY("8/28/26"); got != "2026-08-28" {
		t.Fatalf("want 2026-08-28, got %q", got)
	}
	if got := parseMDY("12/31/99"); got != "1999-12-31" { // yy>=70 → 1900s pivot
		t.Fatalf("want 1999-12-31, got %q", got)
	}
	for _, bad := range []string{"13/45/26", "8-28-26", "abc", "", "8/28/2026"} {
		if got := parseMDY(bad); got != "" {
			t.Fatalf("parseMDY(%q) should fail, got %q", bad, got)
		}
	}
}

func TestNormalizeLabel(t *testing.T) {
	if got := normalizeLabel("(1)     Domestic Production"); got != "Domestic Production" {
		t.Fatalf("footnote strip failed: %q", got)
	}
	if got := normalizeLabel("Crude Oil Supply "); got != "Crude Oil Supply" {
		t.Fatalf("trailing space collapse failed: %q", got)
	}
	if got := normalizeLabel("(12)        Exports"); got != "Exports" {
		t.Fatalf("multi-space collapse failed: %q", got)
	}
}

// ── parseEiaTable1 over the energy-mcp mini fixture ─────────────────────────

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func TestParseEiaStocksSection(t *testing.T) {
	sections, err := parseEiaTable1(loadFixture(t, "eia-mini.csv"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	stocks := sections["stocks"]
	if stocks.currentWeekEnding != "2026-08-28" || stocks.priorWeekEnding != "2026-08-21" {
		t.Fatalf("week endings: cur=%q prior=%q", stocks.currentWeekEnding, stocks.priorWeekEnding)
	}
	want := map[string]float64{
		"Crude Oil":                         711.064, // total incl SPR — must NOT be confused with commercial
		"Commercial (Excluding SPR)":        424.460,
		"Strategic Petroleum Reserve (SPR)": 286.604,
	}
	for label, v := range want {
		row, ok := stocks.rows[label]
		if !ok {
			t.Fatalf("stocks row missing: %s", label)
		}
		if !row.curHas || row.curVal != v {
			t.Fatalf("%s: want %v, got has=%v val=%v", label, v, row.curHas, row.curVal)
		}
	}
	if row := stocks.rows["Commercial (Excluding SPR)"]; row.priorVal != 428.910 {
		t.Fatalf("prior week value: %v", row.priorVal)
	}
}

func TestParseEiaSupplySection(t *testing.T) {
	sections, err := parseEiaTable1(loadFixture(t, "eia-mini.csv"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	supply := sections["supply"]
	want := map[string]float64{
		"Domestic Production": 13862, // thousands separator stripped, footnote stripped
		"Imports":             6770,
		"Exports":             4483,
	}
	for label, v := range want {
		row, ok := supply.rows[label]
		if !ok {
			t.Fatalf("supply row missing: %s", label)
		}
		if !row.curHas || row.curVal != v {
			t.Fatalf("%s: want %v, got has=%v val=%v", label, v, row.curHas, row.curVal)
		}
	}
	// Other Supply group rows must NOT leak into the crude supply section:
	// "(25) Imports" (Other Supply, 1,049) shares the normalized label with
	// "(8) Imports" (Crude Oil Supply, 6,770) — the stored value must be the
	// crude one.
	if supply.rows["Imports"].curVal != 6770 {
		t.Fatalf("Imports must come from Crude Oil Supply group, got %v", supply.rows["Imports"].curVal)
	}
}

func TestParseEiaEnDashMissing(t *testing.T) {
	// Replace one value with a CP1252 en-dash pair (0x96 0x20 0x96).
	body := strings.ReplaceAll(string(loadFixture(t, "eia-mini.csv")),
		`"424.460","428.910"`, "\"\x96 \x96\",\"428.910\"")
	sections, err := parseEiaTable1([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	row := sections["stocks"].rows["Commercial (Excluding SPR)"]
	if row.curHas || row.curMissing == "" {
		t.Fatalf("en-dash must map to null+missing: has=%v missing=%q", row.curHas, row.curMissing)
	}
	if row.priorVal != 428.910 {
		t.Fatalf("prior unaffected: %v", row.priorVal)
	}
}

func wantSchemaChanged(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want SCHEMA_CHANGED, got nil")
	}
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSchemaChanged {
		t.Fatalf("want SCHEMA_CHANGED, got %v", err)
	}
}

func TestParseEiaSchemaDrift(t *testing.T) {
	base := string(loadFixture(t, "eia-mini.csv"))

	// Unknown numeric marker in a target cell → SCHEMA_CHANGED.
	drifted := strings.ReplaceAll(base, `"424.460"`, `"???"`)
	_, err := parseEiaTable1([]byte(drifted))
	wantSchemaChanged(t, err) // unknown marker

	// Required label removed → SCHEMA_CHANGED.
	missing := strings.ReplaceAll(base, `"Strategic Petroleum Reserve (SPR)","286.604","289.726","-3.122","-1.100","404.710","-118.106","-29.200"`, ``)
	_, err = parseEiaTable1([]byte(missing))
	wantSchemaChanged(t, err) // missing label

	// Duplicate conflicting stocks header (extra section) → SCHEMA_CHANGED.
	dup := base + "\n" + base
	_, err = parseEiaTable1([]byte(dup))
	wantSchemaChanged(t, err) // duplicate section

	// Conflicting duplicate label rows → SCHEMA_CHANGED; identical duplicates merge.
	dupConflict := strings.ReplaceAll(base,
		`"Commercial (Excluding SPR)","424.460","428.910","-4.450","-1.000","420.707","3.752","0.900"`,
		`"Commercial (Excluding SPR)","424.460","428.910","-4.450","-1.000","420.707","3.752","0.900"`+"\n"+
			`"Commercial (Excluding SPR)","999.999","428.910","-4.450","-1.000","420.707","3.752","0.900"`)
	_, err = parseEiaTable1([]byte(dupConflict))
	wantSchemaChanged(t, err) // conflicting duplicates
	dupIdentical := strings.ReplaceAll(base,
		`"Commercial (Excluding SPR)","424.460","428.910","-4.450","-1.000","420.707","3.752","0.900"`,
		`"Commercial (Excluding SPR)","424.460","428.910","-4.450","-1.000","420.707","3.752","0.900"`+"\n"+
			`"Commercial (Excluding SPR)","424.460","428.910","-4.450","-1.000","420.707","3.752","0.900"`)
	if _, err = parseEiaTable1([]byte(dupIdentical)); err != nil {
		t.Fatalf("identical duplicates must merge, got %v", err)
	}
}

// ── Fetch over httptest (cache + section selection + INVALID_ARGUMENT) ──────

func testClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // test-only
	}
}

func newEIAOverServer(t *testing.T, body []byte) (*EIA, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20})
	f.SetClient(testClient())
	eia := NewEIA(f, datasources.NewTTLCache(time.Minute))
	eia.baseURL = srv.URL + "/table1.csv"
	return eia, srv
}

func TestEiaFetchStocksAndSupply(t *testing.T) {
	eia, _ := newEIAOverServer(t, loadFixture(t, "eia-mini.csv"))
	ctx := context.Background()

	res, err := eia.Fetch(ctx, "stocks")
	if err != nil {
		t.Fatalf("fetch stocks: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 3 {
		t.Fatalf("stocks must return 3 observations, got %d", len(obs))
	}
	byFlow := map[string]map[string]any{}
	for _, o := range obs {
		byFlow[o["flow"].(string)] = o
	}
	if byFlow["crude_stocks_commercial"]["value"].(float64) != 424.46 {
		t.Fatalf("commercial stocks value: %v", byFlow["crude_stocks_commercial"]["value"])
	}
	if byFlow["crude_stocks_commercial"]["unit"].(string) != "MMbbl" {
		t.Fatalf("stocks unit must be MMbbl: %v", byFlow["crude_stocks_commercial"]["unit"])
	}
	if byFlow["crude_stocks_commercial"]["period"].(string) != "2026-08-28" {
		t.Fatalf("period: %v", byFlow["crude_stocks_commercial"]["period"])
	}
	if res["retrieved_at"] == "" || res["source_sha256"] == "" {
		t.Fatal("provenance metadata missing")
	}

	res, err = eia.Fetch(ctx, "supply")
	if err != nil {
		t.Fatalf("fetch supply: %v", err)
	}
	obs = res["observations"].([]map[string]any)
	if len(obs) != 3 {
		t.Fatalf("supply must return 3 observations, got %d", len(obs))
	}
	if obs[0]["unit"].(string) != "Mb/d" {
		t.Fatalf("supply unit must be Mb/d: %v", obs[0]["unit"])
	}

	// Bad section → INVALID_ARGUMENT, zero network calls.
	_, errB := eia.Fetch(ctx, "bogus")
	if errB == nil {
		t.Fatal("bad section must fail")
	} else if se, ok := datasources.AsSourceError(errB); !ok || se.Kind != datasources.ErrInvalidArgument {
		t.Fatalf("bad section: want INVALID_ARGUMENT, got %v", errB)
	}
}

// ── explicit weekly history window (HR-1..HR-5, archived edition CSVs) ──────

// TestParseEiaArchiveFixtures proves the REAL archived editions parse through
// the same code path (phase-0 controller ruling: archive CSV only, no api v2).
func TestParseEiaArchiveFixtures(t *testing.T) {
	cases := []struct {
		file          string
		cur, prior    string
		commercialCur float64
	}{
		{"eia-wpsr-archive-2026-08-26-table1.csv", "2026-08-21", "2026-08-14", 428.910},
		{"eia-wpsr-archive-2019-01-04-table1.csv", "2018-12-28", "2018-12-21", 441.418},
	}
	for _, c := range cases {
		sections, err := parseEiaTable1(loadFixture(t, c.file))
		if err != nil {
			t.Fatalf("%s: parse: %v", c.file, err)
		}
		stocks := sections["stocks"]
		if stocks.currentWeekEnding != c.cur || stocks.priorWeekEnding != c.prior {
			t.Fatalf("%s: weeks cur=%q prior=%q, want %s/%s", c.file, stocks.currentWeekEnding, stocks.priorWeekEnding, c.cur, c.prior)
		}
		if row := stocks.rows["Commercial (Excluding SPR)"]; !row.curHas || row.curVal != c.commercialCur {
			t.Fatalf("%s: commercial cur: has=%v val=%v, want %v", c.file, row.curHas, row.curVal, c.commercialCur)
		}
	}
}

// eiaEditionServer serves the live file at /table1.csv and archived editions
// at /{YYYY}/{YYYY_MM_DD}/csv/table1.csv. Paths absent from editions → 404.
// requests logs request paths in order; calls stays authoritative.
type eiaEditionServer struct {
	live     []byte
	editions map[string][]byte
	requests []string
	calls    int
}

func newEIAOverEditions(t *testing.T, srv *eiaEditionServer) *EIA {
	t.Helper()
	httpSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.calls++
		srv.requests = append(srv.requests, r.URL.Path)
		if r.URL.Path == "/table1.csv" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(srv.live)
			return
		}
		if body, ok := srv.editions[strings.TrimPrefix(r.URL.Path, "/")]; ok {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(httpSrv.Close)
	host := strings.TrimPrefix(httpSrv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20})
	f.SetClient(testClient())
	eia := NewEIA(f, datasources.NewTTLCache(time.Minute))
	eia.baseURL = httpSrv.URL + "/table1.csv"
	eia.archiveBase = httpSrv.URL
	return eia
}

// shiftEiaWeeks rewrites the eia-mini.csv date columns so the synthetic body
// reports the given current/prior week endings (MDY). Values are carried over
// unchanged — stepping-logic test bodies only; real archived editions stay in
// the fixtures above.
func shiftEiaWeeks(t *testing.T, cur, prior string) []byte {
	t.Helper()
	mdy := func(period string) string {
		ts, err := time.Parse("2006-01-02", period)
		if err != nil {
			t.Fatalf("bad period %q: %v", period, err)
		}
		return ts.Format("1/2/06")
	}
	body := string(loadFixture(t, "eia-mini.csv"))
	body = strings.ReplaceAll(body, "8/28/26", mdy(cur))
	body = strings.ReplaceAll(body, "8/21/26", mdy(prior))
	return []byte(body)
}

func TestEiaFetchWeeksValidationBeforeNetwork(t *testing.T) {
	srv := &eiaEditionServer{live: loadFixture(t, "eia-mini.csv"), editions: map[string][]byte{}}
	eia := newEIAOverEditions(t, srv)
	ctx := context.Background()

	// Out of range / non-positive → INVALID_ARGUMENT, zero upstream calls (HR-1).
	for _, weeks := range []int{-1, 0, 13, 100} {
		_, err := eia.Fetch(ctx, "stocks", weeks)
		if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrInvalidArgument {
			t.Fatalf("weeks=%d: want INVALID_ARGUMENT, got %v", weeks, err)
		}
	}
	if _, err := eia.Fetch(ctx, "stocks", 3, 3); err == nil {
		t.Fatal("more than one weeks value must be rejected")
	}
	if srv.calls != 0 {
		t.Fatalf("validation must happen before any network call, calls = %d", srv.calls)
	}
}

func TestEiaFetchWeeksWindowAssemblesArchivedEditions(t *testing.T) {
	// Part A: live file (2026-08-28 + 2026-08-21) + one stepping edition
	// carrying 2026-08-14 + 2026-08-07.
	srv := &eiaEditionServer{
		live: loadFixture(t, "eia-mini.csv"),
		editions: map[string][]byte{
			// Synthetic stepping edition: 2026-08-14 + 2026-08-07. Probed for
			// uncovered week 08_14 at its Wednesday URL 08_19.
			"2026/2026_08_19/csv/table1.csv": shiftEiaWeeks(t, "2026-08-14", "2026-08-07"),
		},
	}
	eia := newEIAOverEditions(t, srv)
	ctx := context.Background()

	// Default (no weeks) stays the current+prior pair, untouched.
	res, err := eia.Fetch(ctx, "stocks")
	if err != nil {
		t.Fatalf("default fetch: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 3 || obs[0]["prior_period"] != "2026-08-21" {
		t.Fatalf("default mode must keep the current+prior pair shape: %+v", obs[0])
	}
	if _, has := res["periods"]; has {
		t.Fatal("default mode must not expose series fields")
	}

	// weeks=4 → flat series across live + 1 edition (each edition adds 2 weeks).
	res, err = eia.Fetch(ctx, "supply", 4)
	if err != nil {
		t.Fatalf("window fetch: %v", err)
	}
	if res["weeks"].(int) != 4 {
		t.Fatalf("weeks echo: %v", res["weeks"])
	}
	if got := res["periods"].([]string); len(got) != 4 || got[0] != "2026-08-07" || got[3] != "2026-08-28" {
		t.Fatalf("periods must ascend oldest→newest: %v", got)
	}
	obs = res["observations"].([]map[string]any)
	if len(obs) != 12 { // 3 supply flows × 4 weeks
		t.Fatalf("want 12 flat observations, got %d", len(obs))
	}
	// Grouped by flow in row order, periods descending within a flow.
	if obs[0]["flow"] != "crude_production" || obs[0]["period"] != "2026-08-28" {
		t.Fatalf("first observation: %+v", obs[0])
	}
	if obs[1]["period"] != "2026-08-21" || obs[3]["period"] != "2026-08-07" {
		t.Fatalf("periods must descend within a flow: %+v %+v %+v", obs[0], obs[1], obs[3])
	}
	if obs[0]["unit"] != "Mb/d" {
		t.Fatalf("unit must stay native Mb/d: %v", obs[0]["unit"])
	}
	// source_edition traces the document each cell came from; overlapping
	// weeks (2026-08-21) keep the live file's (freshest) revision.
	if obs[0]["source_edition"] != "current" || obs[3]["source_edition"] != "2026_08_19" {
		t.Fatalf("source_edition provenance: %+v / %+v", obs[0], obs[3])
	}
	docs := res["documents"].([]map[string]any)
	if len(docs) != 2 || docs[0]["kind"] != "current" || docs[1]["edition"] != "2026_08_19" {
		t.Fatalf("documents provenance: %+v", docs)
	}
	if res["source_sha256"] == "" || res["retrieved_at"] == "" {
		t.Fatal("top-level live-file provenance missing")
	}

	// weeks=1 trims to the single newest week (live file only).
	res, err = eia.Fetch(ctx, "stocks", 1)
	if err != nil {
		t.Fatalf("weeks=1: %v", err)
	}
	obs = res["observations"].([]map[string]any)
	if len(obs) != 3 || obs[0]["period"] != "2026-08-28" {
		t.Fatalf("weeks=1 must return only the newest week: %+v", obs)
	}

	// Part B: a REAL archived edition as the live body (weeks 2026-08-21 +
	// 2026-08-14, downloaded 2026-09-22) chains through the same path.
	srvB := &eiaEditionServer{
		live: loadFixture(t, "eia-wpsr-archive-2026-08-26-table1.csv"),
		editions: map[string][]byte{
			"2026/2026_08_12/csv/table1.csv": shiftEiaWeeks(t, "2026-08-07", "2026-07-31"),
		},
	}
	eiaB := newEIAOverEditions(t, srvB)
	res, err = eiaB.Fetch(context.Background(), "stocks", 4)
	if err != nil {
		t.Fatalf("real-archive live window: %v", err)
	}
	if got := res["periods"].([]string); len(got) != 4 || got[0] != "2026-07-31" || got[3] != "2026-08-21" {
		t.Fatalf("real-archive window periods: %v", got)
	}
	// Commercial stocks 2026-08-21 = 428.910 straight from the real edition.
	for _, o := range res["observations"].([]map[string]any) {
		if o["flow"] == "crude_stocks_commercial" && o["period"] == "2026-08-21" {
			if o["value"] != 428.91 {
				t.Fatalf("real edition value: %v", o["value"])
			}
		}
	}
}

// TestEiaFetchWeeksEditionMismatchFailsLoudly: an edition that resolves
// without the expected week-ending (real 2019 bytes probed for a 2026 week)
// must fail SOURCE_UNAVAILABLE instead of silently leaving a hole in the
// window.
func TestEiaFetchWeeksEditionMismatchFailsLoudly(t *testing.T) {
	srv := &eiaEditionServer{
		live: loadFixture(t, "eia-mini.csv"),
		editions: map[string][]byte{
			"2026/2026_08_19/csv/table1.csv": loadFixture(t, "eia-wpsr-archive-2019-01-04-table1.csv"),
		},
	}
	eia := newEIAOverEditions(t, srv)
	_, err := eia.Fetch(context.Background(), "stocks", 4)
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("edition mismatch: want SOURCE_UNAVAILABLE, got %v", err)
	} else if !strings.Contains(err.Error(), "漂移") || !strings.Contains(err.Error(), "2026-08-14") {
		t.Fatalf("error must name the missing week and mapping drift: %v", err)
	}
}

func TestEiaFetchWeeksHolidayThursdayProbe(t *testing.T) {
	// The edition-date mapping must NOT be a pure Wednesday step (phase-0:
	// holiday weeks jump to Thursday, e.g. 2026_09_10). Probe order per
	// week-ending: Wednesday → Thursday → Tuesday.
	srv := &eiaEditionServer{
		live: loadFixture(t, "eia-mini.csv"),
		editions: map[string][]byte{
			"2026/2026_08_26/csv/table1.csv": loadFixture(t, "eia-wpsr-archive-2026-08-26-table1.csv"),
			// Week 2026-08-14: Wednesday 08_19 is 404, Thursday 08_20 carries it.
			"2026/2026_08_20/csv/table1.csv": shiftEiaWeeks(t, "2026-08-14", "2026-08-07"),
		},
	}
	eia := newEIAOverEditions(t, srv)
	res, err := eia.Fetch(context.Background(), "stocks", 4)
	if err != nil {
		t.Fatalf("holiday-probe window: %v", err)
	}
	if got := res["periods"].([]string); len(got) != 4 || got[0] != "2026-08-07" {
		t.Fatalf("thursday edition must complete the window: %v", got)
	}
	wantPaths := []string{
		"/table1.csv",
		"/2026/2026_08_19/csv/table1.csv", // week 08_14: Wednesday probe (404)
		"/2026/2026_08_20/csv/table1.csv", // …then Thursday hits
	}
	if len(srv.requests) != len(wantPaths) {
		t.Fatalf("request sequence = %v", srv.requests)
	}
	for i, p := range wantPaths {
		if srv.requests[i] != p {
			t.Fatalf("request[%d] = %s, want %s (full: %v)", i, srv.requests[i], p, srv.requests)
		}
	}

	// All three candidates 404 → explicit SOURCE_UNAVAILABLE naming the week.
	srv2 := &eiaEditionServer{live: loadFixture(t, "eia-mini.csv"), editions: map[string][]byte{}}
	eia2 := newEIAOverEditions(t, srv2)
	_, err = eia2.Fetch(context.Background(), "stocks", 3)
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("all-404 probe: want SOURCE_UNAVAILABLE, got %v", err)
	} else if !strings.Contains(err.Error(), "2026-08-14") {
		t.Fatalf("error must name the missing week-ending: %v", err)
	}
}

func TestEiaFetchWeeksCacheKeysNeverCross(t *testing.T) {
	srv := &eiaEditionServer{
		live: loadFixture(t, "eia-mini.csv"),
		editions: map[string][]byte{
			"2026/2026_08_19/csv/table1.csv": shiftEiaWeeks(t, "2026-08-14", "2026-08-07"),
			"2026/2026_08_05/csv/table1.csv": shiftEiaWeeks(t, "2026-07-31", "2026-07-24"),
		},
	}
	eia := newEIAOverEditions(t, srv)
	ctx := context.Background()

	// Default fetch: live file only (1 call).
	if _, err := eia.Fetch(ctx, "stocks"); err != nil {
		t.Fatalf("default: %v", err)
	}
	if srv.calls != 1 {
		t.Fatalf("default fetch calls = %d", srv.calls)
	}
	// weeks=3 reuses the cached live doc + fetches the edition (calls +1).
	first, err := eia.Fetch(ctx, "stocks", 3)
	if err != nil {
		t.Fatalf("weeks=3: %v", err)
	}
	if srv.calls != 2 {
		t.Fatalf("weeks=3 must reuse the live cache and fetch one edition, calls = %d", srv.calls)
	}
	// Same weeks key → assembled response served from cache (calls stay).
	second, err := eia.Fetch(ctx, "supply", 3)
	if err != nil {
		t.Fatalf("weeks=3 hit: %v", err)
	}
	if srv.calls != 2 {
		t.Fatalf("assembled weeks response must be cached, calls = %d", srv.calls)
	}
	if first["retrieved_at"] != second["retrieved_at"] || second["from_cache"] != true {
		t.Fatalf("cache hit must preserve original metadata (HR-6): first=%v second=%v", first["retrieved_at"], second["from_cache"])
	}
	// A different window is a different key: weeks=2 uses only the live doc
	// (already cached), never replays the weeks=3 payload.
	w2, err := eia.Fetch(ctx, "stocks", 2)
	if err != nil {
		t.Fatalf("weeks=2: %v", err)
	}
	if srv.calls != 2 {
		t.Fatalf("weeks=2 shares the live doc cache, calls = %d", srv.calls)
	}
	if got := len(w2["periods"].([]string)); got != 2 {
		t.Fatalf("weeks=2 must have its own window, got %d periods", got)
	}
	// Drift on a freshly fetched live body must fail loudly and not poison
	// the cache: a NEW window (weeks=5, never assembled before) re-fetches the
	// drifted live body → SCHEMA_CHANGED, then a clean refetch succeeds.
	eia.cache.Evict("eia:table1")
	srv.live = []byte("\"STUB_1\",\"bad\"\n")
	if _, err := eia.Fetch(ctx, "stocks", 5); err == nil {
		t.Fatal("drifted live body must fail a fresh window")
	} else if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSchemaChanged {
		t.Fatalf("drift must be SCHEMA_CHANGED, got %v", err)
	}
	srv.live = loadFixture(t, "eia-mini.csv")
	res, err := eia.Fetch(ctx, "stocks", 5)
	if err != nil {
		t.Fatalf("post-drift refetch must succeed: %v", err)
	}
	if got := len(res["periods"].([]string)); got != 5 {
		t.Fatalf("post-drift weeks=5 must assemble fully, got %d periods", got)
	}
}

func TestEiaFetchCachesAndEvictsOnDrift(t *testing.T) {
	body := append([]byte(nil), loadFixture(t, "eia-mini.csv")...)
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20})
	f.SetClient(testClient())
	eia := NewEIA(f, datasources.NewTTLCache(time.Minute))
	eia.baseURL = srv.URL + "/table1.csv"
	ctx := context.Background()

	if _, err := eia.Fetch(ctx, "stocks"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := eia.Fetch(ctx, "supply"); err != nil {
		t.Fatalf("second fetch (cache hit): %v", err)
	}
	if calls != 1 {
		t.Fatalf("TTL cache must serve the second call, upstream calls = %d", calls)
	}

	// A live cache serves the good body even after upstream drifts (by design:
	// drift is detected at most once per TTL window). The eviction contract is
	// about a FRESHLY fetched bad body: parse failure evicts it so the next
	// call re-requests instead of replaying garbage.
	eia.cache.Evict("eia:table1") // simulate TTL expiry so the next call hits upstream
	for i := range body {
		body[i] = 'x'
	}
	body = append(body[:0], []byte("\"STUB_1\",\"bad\",\"worse\"\n")...)
	_, err := eia.Fetch(ctx, "stocks")
	wantSchemaChanged(t, err) // freshly fetched garbage → parse failure
	if calls != 2 {
		t.Fatalf("post-expiry fetch must hit upstream, calls = %d", calls)
	}
	// The bad body was evicted; restore upstream and verify a clean re-fetch.
	good := loadFixture(t, "eia-mini.csv")
	body = append(body[:0], good...)
	if _, err := eia.Fetch(ctx, "stocks"); err != nil {
		t.Fatalf("post-evict refetch: %v", err)
	}
	if calls != 3 {
		t.Fatalf("evicted bad body must force a refetch, calls = %d", calls)
	}
}
