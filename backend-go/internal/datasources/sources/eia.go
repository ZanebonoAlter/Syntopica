package sources

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/datasources"
)

// eiaWpsrURL is the fixed official URL (weekly refresh overwrites the same
// URL; no URL parameter is ever exposed).
const eiaWpsrURL = "https://ir.eia.gov/wpsr/table1.csv"

// eiaArchiveBase is the official WPSR archived-edition root (no key). History
// windows come from archived edition CSVs only; api.eia.gov v2 is forbidden
// (it would add an EIA key surface — apply-controller ruling, phase-0).
const eiaArchiveBase = "https://www.eia.gov/petroleum/supply/weekly/archive"

// EIAMaxWeeks is the upper bound of the explicit weekly history window.
const EIAMaxWeeks = 12

// eiaCacheKey is the live-file document cache (default 2-period mode;
// unchanged). Explicit windows cache their ASSEMBLED response under
// "eia:table1:weeks=N" so a 2-period and an N-week response can never
// cross-contaminate (HR-6: cache key covers all effective parameters).
const eiaCacheKey = "eia:table1"

// eiaEditionDayOffsets are the archive edition publication dates probed for a
// week-ending Friday, in order: the Wednesday after the close (usual case,
// Friday+5), the Thursday of holiday weeks (Friday+6; observed 2026_09_10 /
// 2026_01_22 editions), and the Tuesday (Friday+4) as the last probe. The
// edition-date mapping must never be a pure Wednesday step — holiday weeks
// jump to Thursday (verified against the live archive).
var eiaEditionDayOffsets = []int{5, 6, 4}

// Target rows (normalized labels → flow ids), translated from energy-mcp.
var eiaStocksRows = []struct{ label, flow string }{
	{"Crude Oil", "crude_stocks_total_incl_spr"},
	{"Commercial (Excluding SPR)", "crude_stocks_commercial"},
	{"Strategic Petroleum Reserve (SPR)", "crude_stocks_spr"},
}

var eiaSupplyRows = []struct{ label, flow string }{
	{"Domestic Production", "crude_production"},
	{"Imports", "crude_imports"},
	{"Exports", "crude_exports"},
}

const eiaSupplyGroup = "Crude Oil Supply"

// EIA fetches the EIA WPSR table 1 with in-memory TTL caching.
type EIA struct {
	fetch       *datasources.Fetcher
	cache       *datasources.TTLCache
	baseURL     string // production default below; tests point it at httptest
	archiveBase string // archived-edition root; tests point it at httptest
}

// NewEIA builds the EIA fetcher against the official URL.
func NewEIA(f *datasources.Fetcher, c *datasources.TTLCache) *EIA {
	return &EIA{fetch: f, cache: c, baseURL: eiaWpsrURL, archiveBase: eiaArchiveBase}
}

type eiaRow struct {
	sourceRow    int
	label        string
	curVal       float64
	curHas       bool
	curRaw       string
	curMissing   string
	priorVal     float64
	priorHas     bool
	priorRaw     string
	priorMissing string
}

type eiaSection struct {
	rows                 map[string]*eiaRow
	currentWeekEnding    string
	priorWeekEnding      string
	currentWeekEndingSet bool
	priorWeekEndingSet   bool
}

// Fetch returns the WPSR section ("stocks" or "supply") as a JSON-ready map,
// mirroring energy-mcp's eia_result shape (observations + provenance notes).
//
// The optional weeks argument (explicit only, integer 1..EIAMaxWeeks) extends
// the response to a flat weekly series of the most recent N week-endings,
// assembled from official archived edition CSVs (no key). Without it the
// behavior is byte-identical to the historical current+prior pair. weeks is
// validated BEFORE any network call (INVALID_ARGUMENT never fetches).
func (e *EIA) Fetch(ctx context.Context, section string, weeks ...int) (map[string]any, error) {
	if section != "stocks" && section != "supply" {
		return nil, datasources.InvalidArg("eia_wpsr", `section 必须是 "stocks" 或 "supply"，收到 `+strconv.Quote(section))
	}
	window, err := validateEiaWeeks(weeks)
	if err != nil {
		return nil, err
	}
	if window == 0 {
		return e.fetchCurrent(ctx, section)
	}
	return e.fetchWindow(ctx, section, window)
}

// validateEiaWeeks checks the optional explicit window: absent → 0 (default
// current+prior pair); present → integer 1..EIAMaxWeeks; more than one value
// or out of range is INVALID_ARGUMENT before any network call (HR-1).
func validateEiaWeeks(weeks []int) (int, error) {
	switch len(weeks) {
	case 0:
		return 0, nil
	case 1:
		if weeks[0] < 1 || weeks[0] > EIAMaxWeeks {
			return 0, datasources.InvalidArg("eia_wpsr",
				fmt.Sprintf("weeks 必须是 1~%d 的整数，收到 %d", EIAMaxWeeks, weeks[0]))
		}
		return weeks[0], nil
	default:
		return 0, datasources.InvalidArg("eia_wpsr", "weeks 只能传入一个整数")
	}
}

func (e *EIA) fetchCurrent(ctx context.Context, section string) (map[string]any, error) {
	var doc datasources.FetchedDoc
	if cached, ok := e.cache.Get(eiaCacheKey); ok {
		doc = cached
	} else {
		fetched, err := e.fetch.Get(ctx, e.baseURL, map[string]string{
			"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
		})
		if err != nil {
			return nil, err
		}
		doc = *fetched
		e.cache.Put(eiaCacheKey, doc)
	}

	sections, err := parseEiaTable1(doc.Payload)
	if err != nil {
		e.cache.Evict(eiaCacheKey) // bad body must not replay from cache
		return nil, err
	}

	var targets []struct{ label, flow string }
	var unit string
	var chosen *eiaSection
	if section == "stocks" {
		targets, unit, chosen = eiaStocksRows, "MMbbl", sections["stocks"]
	} else {
		targets, unit, chosen = eiaSupplyRows, "Mb/d", sections["supply"]
	}

	observations := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		row := chosen.rows[t.label]
		obs := map[string]any{
			"geo":             "US",
			"product":         "Crude Oil",
			"flow":            t.flow,
			"label":           row.label,
			"frequency":       "weekly",
			"unit":            unit,
			"period":          chosen.currentWeekEnding,
			"value":           nil,
			"raw_value":       row.curRaw,
			"prior_period":    chosen.priorWeekEnding,
			"prior_value":     nil,
			"prior_raw_value": row.priorRaw,
			"source_row":      row.sourceRow,
		}
		if row.curHas {
			obs["value"] = row.curVal
		}
		if row.priorHas {
			obs["prior_value"] = row.priorVal
		}
		if row.curMissing != "" {
			obs["missing_reason"] = row.curMissing
		}
		if row.priorMissing != "" {
			obs["prior_missing_reason"] = row.priorMissing
		}
		observations = append(observations, obs)
	}

	return map[string]any{
		"source":        "EIA Weekly Petroleum Status Report Table 1 (US only, weekly)",
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"last_modified": doc.LastModified,
		"source_sha256": doc.SHA256,
		"from_cache":    doc.FromCache,
		"notes": []string{
			"US data only; weekly period ending Friday per WPSR convention.",
			"Stocks in million barrels (MMbbl); supply flows in thousand barrels per day (Mb/d) per WPSR Table 1 definitions (units are not stated inside the CSV).",
			"No cross-source totals or unit conversions are performed.",
		},
		"observations": observations,
	}, nil
}

// parseEiaTable1 parses WPSR table1.csv into sections. Translated 1:1 from
// energy-mcp parse_eia_table1: two sections each with their own header; date
// columns located by header regex (not hardcoded positions); any structural
// drift raises SCHEMA_CHANGED.
func parseEiaTable1(cp1252Body []byte) (map[string]*eiaSection, error) {
	text, err := cp1252Decode(cp1252Body)
	if err != nil {
		return nil, datasources.SchemaChangedDetail("eia_wpsr", "CP1252 解码失败", err.Error())
	}
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1 // column count varies between sections

	sections := map[string]*eiaSection{"stocks": {rows: map[string]*eiaRow{}}, "supply": {rows: map[string]*eiaRow{}}}
	currentSection := ""
	var dateCols []int
	stocksStarted, supplyStarted := false, false

	rowNo := 0
	for {
		rowNo++
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, datasources.SchemaChangedDetail("eia_wpsr",
				fmt.Sprintf("EIA table1 CSV 解析失败（第 %d 行）", rowNo), readErr.Error())
		}
		if len(row) == 0 || allBlank(row) {
			continue
		}
		first := cell(row, 0)
		second := cell(row, 1)

		if first == "STUB_1" {
			switch {
			case second == "STUB_2":
				if supplyStarted {
					return nil, datasources.SchemaChangedDetail("eia_wpsr",
						fmt.Sprintf("第 %d 行：supply 分区表头重复", rowNo), "")
				}
				currentSection, supplyStarted = "supply", true
			case isMDYHeader(second):
				if stocksStarted {
					return nil, datasources.SchemaChangedDetail("eia_wpsr",
						fmt.Sprintf("第 %d 行：stocks 分区表头重复", rowNo), "")
				}
				currentSection, stocksStarted = "stocks", true
			default:
				return nil, datasources.SchemaChangedDetail("eia_wpsr",
					fmt.Sprintf("第 %d 行：无法识别的分区表头第二单元格 %q", rowNo, second), "")
			}
			dateCols = nil
			for i, c := range row {
				if isMDYHeader(c) {
					dateCols = append(dateCols, i)
				}
			}
			if len(dateCols) < 2 {
				return nil, datasources.SchemaChangedDetail("eia_wpsr",
					fmt.Sprintf("第 %d 行：日期列不足 2 个（%d 个）", rowNo, len(dateCols)), "")
			}
			sections[currentSection].currentWeekEnding = parseMDY(cell(row, dateCols[0]))
			sections[currentSection].currentWeekEndingSet = true
			sections[currentSection].priorWeekEnding = parseMDY(cell(row, dateCols[1]))
			sections[currentSection].priorWeekEndingSet = true
			continue
		}
		if currentSection == "" {
			continue // preamble before first header (none observed, tolerated)
		}

		if currentSection == "stocks" {
			label := normalizeLabel(first)
			if label == "" {
				continue
			}
			if err := recordEiaRow(sections["stocks"].rows, rowNo, label,
				cell(row, dateCols[0]), cell(row, dateCols[1])); err != nil {
				return nil, err
			}
		} else {
			group := normalizeLabel(first)
			label := normalizeLabel(second)
			if group == eiaSupplyGroup && label != "" {
				if err := recordEiaRow(sections["supply"].rows, rowNo, label,
					cell(row, dateCols[0]), cell(row, dateCols[1])); err != nil {
					return nil, err
				}
			}
		}
	}

	var missing []string
	for _, t := range eiaStocksRows {
		if _, ok := sections["stocks"].rows[t.label]; !ok {
			missing = append(missing, t.label)
		}
	}
	for _, t := range eiaSupplyRows {
		if _, ok := sections["supply"].rows[t.label]; !ok {
			missing = append(missing, t.label)
		}
	}
	if len(missing) > 0 {
		return nil, datasources.SchemaChangedDetail("eia_wpsr",
			"必需行标签缺失", strings.Join(missing, ", "))
	}
	for name, sec := range sections {
		if !sec.currentWeekEndingSet || sec.currentWeekEnding == "" || sec.priorWeekEnding == "" {
			return nil, datasources.SchemaChangedDetail("eia_wpsr",
				fmt.Sprintf("%s 分区周结束日期解析失败", name), "")
		}
	}
	return sections, nil
}

// recordEiaRow parses and stores one target row; unknown cell contents and
// conflicting duplicates raise SCHEMA_CHANGED (translated from
// _record_eia_row).
func recordEiaRow(store map[string]*eiaRow, rowNo int, label, curCell, priorCell string) error {
	curVal, curHas, curMissing := parseNumber(curCell)
	if !curHas && curMissing == "" {
		return datasources.SchemaChangedDetail("eia_wpsr",
			fmt.Sprintf("第 %d 行 %q：当周单元格 %q 既非数值也非已知缺失标记", rowNo, label, curCell), "")
	}
	priorVal, priorHas, priorMissing := parseNumber(priorCell)
	if !priorHas && priorMissing == "" {
		return datasources.SchemaChangedDetail("eia_wpsr",
			fmt.Sprintf("第 %d 行 %q：上周单元格 %q 既非数值也非已知缺失标记", rowNo, label, priorCell), "")
	}
	entry := &eiaRow{
		sourceRow: rowNo, label: label,
		curVal: curVal, curHas: curHas, curRaw: strings.TrimSpace(curCell), curMissing: curMissing,
		priorVal: priorVal, priorHas: priorHas, priorRaw: strings.TrimSpace(priorCell), priorMissing: priorMissing,
	}
	if existing, ok := store[label]; ok && eiaRowsConflict(existing, entry) {
		return datasources.SchemaChangedDetail("eia_wpsr",
			fmt.Sprintf("标签 %q 存在冲突的重复行：第 %d 行 vs 第 %d 行", label, existing.sourceRow, rowNo), "")
	}
	store[label] = entry
	return nil
}

func eiaRowsConflict(a, b *eiaRow) bool {
	return a.curVal != b.curVal || a.priorVal != b.priorVal ||
		a.curRaw != b.curRaw || a.priorRaw != b.priorRaw
}

func allBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// cell returns row[i] or "" when out of range (mirrors Python row + [""][:n]).
func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

// ── explicit weekly history window (archived edition CSVs, no key) ──────────

var eiaUAHeaders = map[string]string{
	"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
}

// eiaWeekCell is one row × one week-ending observation cell.
type eiaWeekCell struct {
	val        float64
	has        bool
	raw        string
	missing    string
	sourceRow  int
	editionTag string // "current" for the live file, else the edition YYYY_MM_DD
}

// eiaSeriesCollector accumulates one cell per (week-ending × target label).
// The live file is absorbed FIRST, so overlapping periods keep the freshest
// revision (official data may be revised; archive cells never overwrite
// live-file cells). Structural conflicts stay SCHEMA_CHANGED via
// parseEiaTable1; differing values between editions for the same week are a
// legitimate revision, not drift.
type eiaSeriesCollector struct {
	targets []struct{ label, flow string }
	byWeek  map[string]map[string]*eiaWeekCell // period → label → cell
}

func newEiaSeriesCollector(targets []struct{ label, flow string }) *eiaSeriesCollector {
	return &eiaSeriesCollector{targets: targets, byWeek: map[string]map[string]*eiaWeekCell{}}
}

// absorb records both week-endings of one parsed section and returns the
// OLDEST period now covered (as time, for stepping further back).
func (c *eiaSeriesCollector) absorb(sec *eiaSection, editionTag string) (time.Time, error) {
	oldest := time.Time{}
	for _, p := range []struct {
		period string
		cell   func(*eiaRow) *eiaWeekCell
	}{
		{sec.currentWeekEnding, func(r *eiaRow) *eiaWeekCell {
			return &eiaWeekCell{r.curVal, r.curHas, r.curRaw, r.curMissing, r.sourceRow, editionTag}
		}},
		{sec.priorWeekEnding, func(r *eiaRow) *eiaWeekCell {
			return &eiaWeekCell{r.priorVal, r.priorHas, r.priorRaw, r.priorMissing, r.sourceRow, editionTag}
		}},
	} {
		if p.period == "" {
			continue
		}
		week, err := time.ParseInLocation("2006-01-02", p.period, time.UTC)
		if err != nil {
			return oldest, datasources.SchemaChangedDetail("eia_wpsr",
				fmt.Sprintf("周结束日期 %q 无法解析", p.period), err.Error())
		}
		if _, seen := c.byWeek[p.period]; !seen {
			c.byWeek[p.period] = map[string]*eiaWeekCell{}
		}
		weekCells := c.byWeek[p.period]
		for _, t := range c.targets {
			if _, ok := weekCells[t.label]; ok {
				continue // freshest revision wins; never overwritten
			}
			weekCells[t.label] = p.cell(sec.rows[t.label])
		}
		if oldest.IsZero() || week.Before(oldest) {
			oldest = week
		}
	}
	return oldest, nil
}

// fetchEditionForWeek resolves the archived edition that carries the given
// week-ending: probe Wednesday → Thursday → Tuesday (eiaEditionDayOffsets);
// 404 moves to the next candidate, any other failure aborts immediately (a
// 5xx does not mean the date is wrong). All candidates 404 → explicit
// SOURCE_UNAVAILABLE, never a silent gap.
func (e *EIA) fetchEditionForWeek(ctx context.Context, week time.Time) (datasources.FetchedDoc, string, error) {
	var last404 string
	for _, offset := range eiaEditionDayOffsets {
		edition := week.AddDate(0, 0, offset)
		tag := edition.Format("2006_01_02")
		url := fmt.Sprintf("%s/%04d/%s/csv/table1.csv", e.archiveBase, edition.Year(), tag)
		fetched, ferr := e.fetch.Get(ctx, url, eiaUAHeaders)
		if ferr == nil {
			return *fetched, tag, nil
		}
		if se, ok := datasources.AsSourceError(ferr); ok && se.StatusCode == 404 {
			last404 = tag
			continue
		}
		return datasources.FetchedDoc{}, "", ferr
	}
	return datasources.FetchedDoc{}, "", datasources.UnavailableDetail("eia_wpsr",
		fmt.Sprintf("归档版次定位失败：周结 %s 的周三/周四/周二候选版次均不可得（最后 404 版次 %s）",
			week.Format("2006-01-02"), last404), "")
}

// fetchWindow assembles the most recent `window` week-endings for one
// section. The live file provides the two newest weeks; each archived edition
// contributes up to two more, stepping back one week at a time. The assembled
// response is cached under "eia:table1:weeks=N" so distinct windows never
// share a cache entry; provenance metadata (retrieved_at/sha256/last_modified)
// is the live document's, preserved verbatim on cache hits (HR-6).
func (e *EIA) fetchWindow(ctx context.Context, section string, window int) (map[string]any, error) {
	cacheKey := fmt.Sprintf("%s:weeks=%d", eiaCacheKey, window)
	if cached, ok := e.cache.Get(cacheKey); ok {
		var res map[string]any
		if json.Unmarshal(cached.Payload, &res) == nil {
			res["from_cache"] = true
			return res, nil
		}
		e.cache.Evict(cacheKey) // undecodable entry → treat as a miss
	}

	var doc datasources.FetchedDoc
	if cached, ok := e.cache.Get(eiaCacheKey); ok {
		doc = cached
	} else {
		fetched, err := e.fetch.Get(ctx, e.baseURL, eiaUAHeaders)
		if err != nil {
			return nil, err
		}
		doc = *fetched
		e.cache.Put(eiaCacheKey, doc)
	}
	sections, err := parseEiaTable1(doc.Payload)
	if err != nil {
		e.cache.Evict(eiaCacheKey)
		return nil, err
	}

	targets, unit := eiaTargets(section)
	collector := newEiaSeriesCollector(targets)
	oldest, err := collector.absorb(sections[section], "current")
	if err != nil {
		e.cache.Evict(cacheKey)
		return nil, err
	}
	documents := []map[string]any{eiaDocProvenance("current", "", doc)}

	for len(collector.byWeek) < window {
		next := oldest.AddDate(0, 0, -7)
		editionDoc, tag, ferr := e.fetchEditionForWeek(ctx, next)
		if ferr != nil {
			return nil, ferr
		}
		editionSections, perr := parseEiaTable1(editionDoc.Payload)
		if perr != nil {
			return nil, perr // nothing edition-scoped was cached; no eviction needed
		}
		editionOldest, aerr := collector.absorb(editionSections[section], tag)
		if aerr != nil {
			return nil, aerr
		}
		// The probed edition MUST carry the expected week-ending: a candidate
		// that resolves to a different edition (date mapping drift) must fail
		// loudly instead of silently leaving a hole inside the window.
		if _, ok := collector.byWeek[next.Format("2006-01-02")]; !ok {
			return nil, datasources.UnavailableDetail("eia_wpsr",
				fmt.Sprintf("归档版次 %s 未包含期望周结 %s，版次映射疑似漂移", tag, next.Format("2006-01-02")), editionDoc.URL)
		}
		if !editionOldest.Before(oldest) {
			return nil, datasources.UnavailableDetail("eia_wpsr",
				fmt.Sprintf("归档版次 %s 未包含更早周结（期望 %s），历史窗口无法延伸", tag, next.Format("2006-01-02")), editionDoc.URL)
		}
		documents = append(documents, eiaDocProvenance("archive", tag, editionDoc))
		oldest = editionOldest
	}

	res := collector.result(window, unit, documents, doc)
	if payload, merr := json.Marshal(res); merr == nil {
		e.cache.Put(cacheKey, datasources.FetchedDoc{
			URL: doc.URL, RetrievedAt: doc.RetrievedAt,
			LastModified: doc.LastModified, SHA256: doc.SHA256, Payload: payload,
		})
	}
	return res, nil
}

func eiaTargets(section string) ([]struct{ label, flow string }, string) {
	if section == "stocks" {
		return eiaStocksRows, "MMbbl"
	}
	return eiaSupplyRows, "Mb/d"
}

func eiaDocProvenance(kind, editionTag string, doc datasources.FetchedDoc) map[string]any {
	entry := map[string]any{
		"kind":          kind,
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"last_modified": doc.LastModified,
		"source_sha256": doc.SHA256,
	}
	if editionTag != "" {
		entry["edition"] = editionTag
	}
	return entry
}

// result renders the flat weekly series: observations grouped by target row,
// periods descending (newest first), trimmed to the newest `window`
// week-endings; `periods` is ascending for chart consumption.
func (c *eiaSeriesCollector) result(window int, unit string, documents []map[string]any, liveDoc datasources.FetchedDoc) map[string]any {
	periods := make([]string, 0, len(c.byWeek))
	for p := range c.byWeek {
		periods = append(periods, p)
	}
	sort.Slice(periods, func(i, j int) bool { return periods[i] > periods[j] }) // descending
	if len(periods) > window {
		periods = periods[:window]
	}
	kept := map[string]bool{}
	for _, p := range periods {
		kept[p] = true
	}
	ascending := make([]string, len(periods))
	for i, p := range periods {
		ascending[len(periods)-1-i] = p
	}

	observations := make([]map[string]any, 0, len(c.targets)*len(periods))
	for _, t := range c.targets {
		for _, period := range periods {
			cell := c.byWeek[period][t.label]
			obs := map[string]any{
				"geo":            "US",
				"product":        "Crude Oil",
				"flow":           t.flow,
				"label":          t.label,
				"frequency":      "weekly",
				"unit":           unit,
				"period":         period,
				"value":          nil,
				"raw_value":      cell.raw,
				"source_row":     cell.sourceRow,
				"source_edition": cell.editionTag,
			}
			if cell.has {
				obs["value"] = cell.val
			}
			if cell.missing != "" {
				obs["missing_reason"] = cell.missing
			}
			observations = append(observations, obs)
		}
	}

	return map[string]any{
		"source":        "EIA Weekly Petroleum Status Report Table 1 (US only, weekly)",
		"url":           liveDoc.URL,
		"retrieved_at":  liveDoc.RetrievedAt.Format(time.RFC3339),
		"last_modified": liveDoc.LastModified,
		"source_sha256": liveDoc.SHA256,
		"from_cache":    liveDoc.FromCache,
		"weeks":         window,
		"periods":       ascending,
		"documents":     documents,
		"observations":  observations,
		"notes": []string{
			"US data only; weekly period ending Friday per WPSR convention.",
			fmt.Sprintf("Explicit history window (%d weeks) assembled from official archived edition CSVs; the live file is fetched first and overlapping weeks keep its (freshest) revision.", window),
			"Official data may be revised: archived observations are NOT guaranteed point-in-time historical versions.",
			"Stocks in million barrels (MMbbl); supply flows in thousand barrels per day (Mb/d) per WPSR Table 1 definitions (units are not stated inside the CSV).",
			"No cross-source totals or unit conversions are performed.",
		},
	}
}
