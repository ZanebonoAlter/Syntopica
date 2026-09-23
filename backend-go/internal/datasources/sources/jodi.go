package sources

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/datasources"
)

// jodiURLTemplate is the fixed annual-file template (only {year} varies).
const jodiURLTemplate = "https://www.jodidata.org/_resources/files/downloads/oil-data/annual-csv/primary/primaryyear%d.csv"

const jodiMinYear = 2002

// Flow → FLOW_BREAKDOWN code (translated from energy-mcp; there are no
// literal production/import/export codes in the JODI primary dataset).
var jodiFlowMap = map[string]string{
	"production":     "INDPROD",
	"imports":        "TOTIMPSB",
	"exports":        "TOTEXPSB",
	"closing_stocks": "CLOSTLV",
}

// Default units per flow (no cross-unit conversion is ever performed).
var jodiDefaultUnit = map[string]string{
	"production":     "KBD",
	"imports":        "KBD",
	"exports":        "KBD",
	"closing_stocks": "KBBL",
}

// JODIMaxYears is the upper bound of the explicit calendar-year window.
const JODIMaxYears = 5

var jodiMissingMarkers = map[string]bool{"-": true, "..": true, "x": true}

var jodiMonthRe = regexp.MustCompile(`^(\d{4})-(0[1-9]|1[0-2])$`)

const jodiProduct = "CRUDEOIL"

// JODI fetches the JODI Oil Primary annual CSV.
type JODI struct {
	fetch       *datasources.Fetcher
	cache       *datasources.TTLCache
	urlTemplate string // production default below; tests point it at httptest
}

// NewJODI builds the JODI fetcher.
func NewJODI(f *datasources.Fetcher, c *datasources.TTLCache) *JODI {
	return &JODI{fetch: f, cache: c, urlTemplate: jodiURLTemplate}
}

type jodiRow struct {
	sourceRow      int
	sourceYear     int // annual file the row came from (0 in single-file mode)
	geo            string
	period         string
	product        string
	flow           string
	unit           string
	value          float64
	hasValue       bool
	rawValue       string
	missingReason  string
	assessmentCode string
}

// validateJodiWindow validates the explicit `years` window (HR-3) BEFORE any
// network call: absent → nil (default or explicit-month mode unchanged);
// present → integer 1..JODIMaxYears, mutually exclusive with month, and the
// whole window must stay within the supported year range. The returned slice
// lists the calendar years to fetch, newest first.
func validateJodiWindow(month string, years []int, now time.Time) ([]int, error) {
	switch len(years) {
	case 0:
		return nil, nil
	case 1:
	default:
		return nil, datasources.InvalidArg("jodi_oil_primary", "years 只能传入一个整数")
	}
	n := years[0]
	if n < 1 || n > JODIMaxYears {
		return nil, datasources.InvalidArg("jodi_oil_primary",
			fmt.Sprintf("years 必须是 1~%d 的整数，收到 %d", JODIMaxYears, n))
	}
	if month != "" {
		return nil, datasources.InvalidArg("jodi_oil_primary",
			"years 与 month 互斥：显式多年窗口和精确单月不能同时指定")
	}
	oldest := now.Year() - n + 1
	if oldest < jodiMinYear {
		return nil, datasources.InvalidArg("jodi_oil_primary",
			fmt.Sprintf("years=%d 的窗口回溯到 %d，超出支持范围 %d..%d", n, oldest, jodiMinYear, now.Year()))
	}
	window := make([]int, 0, n)
	for y := now.Year(); y >= oldest; y-- {
		window = append(window, y)
	}
	return window, nil
}

// validateJodiArgs performs pure-local validation BEFORE any network call
// (spec: INVALID_ARGUMENT 不发起网络请求). Empty unit selects the flow
// default; empty month selects the file's latest period later.
func validateJodiArgs(geo, flow, unit, month string, now time.Time) (flowCode, resolvedUnit string, err error) {
	code, ok := jodiFlowMap[flow]
	if !ok {
		return "", "", datasources.InvalidArg("jodi_oil_primary",
			fmt.Sprintf("flow 必须是 production/imports/exports/closing_stocks 之一，收到 %q", flow))
	}
	if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(geo) {
		return "", "", datasources.InvalidArg("jodi_oil_primary",
			fmt.Sprintf("geo 必须是恰好两个大写字母（ISO 风格代码），收到 %q", geo))
	}
	if unit != "" && unit != "KBD" && unit != "KBBL" {
		return "", "", datasources.InvalidArg("jodi_oil_primary",
			fmt.Sprintf("unit 必须是 KBD、KBBL 或留空（不自动换算；CONVBBL 不作为取数单位），收到 %q", unit))
	}
	resolved := unit
	if resolved == "" {
		resolved = jodiDefaultUnit[flow]
	}
	if flow == "closing_stocks" && resolved == "KBD" {
		return "", "", datasources.InvalidArg("jodi_oil_primary",
			"closing_stocks 拒绝单位 KBD（库存为 KBBL；不做换算）")
	}
	if month != "" {
		m := jodiMonthRe.FindStringSubmatch(month)
		if m == nil {
			return "", "", datasources.InvalidArg("jodi_oil_primary",
				fmt.Sprintf("month 必须是 YYYY-MM，收到 %q", month))
		}
		year, _ := strconv.Atoi(m[1])
		if year < jodiMinYear || year > now.Year() {
			return "", "", datasources.InvalidArg("jodi_oil_primary",
				fmt.Sprintf("month 年份 %d 超出支持范围 %d..%d", year, jodiMinYear, now.Year()))
		}
		current := fmt.Sprintf("%04d-%02d", now.Year(), int(now.Month()))
		if month > current {
			return "", "", datasources.InvalidArg("jodi_oil_primary",
				fmt.Sprintf("month %s 是未来月份（当前 %s）", month, current))
		}
	}
	return code, resolved, nil
}

// Fetch queries one geo×flow×unit observation set.
//
//   - month="" and no years: the current file's latest period even when that
//     period's value is null (no silent fallback); current-year 404 falls back
//     to the previous year once, annotated (spec「本年 404 回退一次」).
//   - month="YYYY-MM": exact historical single month; 404 never falls back.
//   - years=N (explicit only, HR-2/3): the most recent N calendar years' ALL
//     available months, mutually exclusive with month. Only the current-year
//     404 may fall back to the previous year once (which is already part of
//     the window for N≥2, so the fetch is deduplicated, never issued twice);
//     a historical-year 404 is a recorded gap without cascading (HR-4/5).
func (j *JODI) Fetch(ctx context.Context, geo, flow, unit, month string, years ...int) (map[string]any, error) {
	flowCode, resolvedUnit, err := validateJodiArgs(geo, flow, unit, month, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	window, err := validateJodiWindow(month, years, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if window == nil {
		return j.fetchSingle(ctx, geo, flowCode, resolvedUnit, month)
	}
	return j.fetchYearsWindow(ctx, geo, flowCode, resolvedUnit, window)
}

func (j *JODI) fetchSingle(ctx context.Context, geo, flowCode, resolvedUnit, month string) (map[string]any, error) {
	doc, yearUsed, strategy, err := j.fetchYear(ctx, month)
	if err != nil {
		return nil, err
	}

	rows, err := parseJodiPrimary(doc.Payload)
	if err != nil {
		j.cache.Evict(j.cacheKey(yearUsed))
		return nil, err
	}

	// Filter to the requested dimension.
	var matched []jodiRow
	for _, r := range rows {
		if r.geo == geo && r.product == jodiProduct && r.flow == flowCode && r.unit == resolvedUnit {
			if month != "" && r.period != month {
				continue
			}
			matched = append(matched, r)
		}
	}
	var selected []jodiRow
	if month != "" {
		selected = matched
	} else if len(matched) > 0 {
		latest := matched[0].period
		for _, r := range matched[1:] {
			if r.period > latest {
				latest = r.period
			}
		}
		for _, r := range matched {
			if r.period == latest {
				selected = append(selected, r)
			}
		}
	}

	// Group by full dimension key: identical rows merge (source_rows kept);
	// any semantic conflict is SCHEMA_CHANGED (energy-mcp semantics).
	type dimKey struct{ geo, product, flow, unit, period string }
	groups := map[dimKey][]jodiRow{}
	var order []dimKey
	for _, r := range selected {
		k := dimKey{r.geo, r.product, r.flow, r.unit, r.period}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}

	observations := make([]map[string]any, 0, len(order))
	for _, k := range order {
		group := groups[k]
		first := group[0]
		for _, other := range group[1:] {
			if other.hasValue != first.hasValue || other.value != first.value ||
				other.rawValue != first.rawValue || other.missingReason != first.missingReason ||
				other.assessmentCode != first.assessmentCode {
				return nil, datasources.SchemaChangedDetail("jodi_oil_primary",
					fmt.Sprintf("维度 %v 存在冲突的重复行：第 %d 行 vs 第 %d 行",
						[]string{k.geo, k.product, k.flow, k.unit, k.period}, first.sourceRow, other.sourceRow), "")
			}
		}
		obs := map[string]any{
			"geo": first.geo, "product": first.product, "flow": first.flow,
			"period": first.period, "frequency": "monthly", "unit": first.unit,
			"value": nil, "raw_value": first.rawValue,
			"source_row": first.sourceRow, "assessment_code": nilIfEmpty(first.assessmentCode),
		}
		if first.hasValue {
			obs["value"] = first.value
		}
		if first.missingReason != "" {
			obs["missing_reason"] = first.missingReason
		}
		if len(group) > 1 {
			rowsList := make([]int, 0, len(group))
			for _, g := range group {
				rowsList = append(rowsList, g.sourceRow)
			}
			obs["source_rows"] = rowsList
		}
		observations = append(observations, obs)
	}

	return map[string]any{
		"source":        "JODI Oil Primary World Database (annual CSV snapshot)",
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"last_modified": doc.LastModified,
		"source_sha256": doc.SHA256,
		"from_cache":    doc.FromCache,
		"year_used":     yearUsed,
		"year_strategy": strategy,
		"no_data":       len(observations) == 0,
		"observations":  observations,
		"notes": []string{
			"TIME_PERIOD 是数据期而非发布期；年度文件滞后数据月约 1.5-2 个月，覆盖式更新无修订历史列。",
			"assessment_code 原样透传（1/2/3 语义未核实，不是质量好坏标志）。",
			"缺失标记 '-'/'..'/'x' 映射 null 并保留在 raw_value，绝不转 0。",
			"closing_stocks 的 SPR 覆盖口径不假定与 EIA 商业库存可比；不做跨源比较。",
		},
	}, nil
}

// fetchYear resolves which annual file to read, applying the 404 fallback:
// month unspecified + current-year 404 → try previous year ONCE (annotated);
// explicit month + 404 → straight error naming the year; double 404 →
// SOURCE_UNAVAILABLE (translated from energy-mcp server-layer semantics).
func (j *JODI) fetchYear(ctx context.Context, month string) (doc datasources.FetchedDoc, yearUsed int, strategy string, err error) {
	now := time.Now().UTC()
	target := now.Year()
	if month != "" {
		target, _ = strconv.Atoi(month[:4])
	}

	tryFetch := func(year int) (datasources.FetchedDoc, error) {
		key := j.cacheKey(year)
		if cached, ok := j.cache.Get(key); ok {
			return cached, nil
		}
		url := fmt.Sprintf(j.urlTemplate, year)
		fetched, ferr := j.fetch.Get(ctx, url, map[string]string{
			"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
		})
		if ferr != nil {
			return datasources.FetchedDoc{}, ferr
		}
		j.cache.Put(key, *fetched)
		return *fetched, nil
	}

	doc, err = tryFetch(target)
	if err == nil {
		return doc, target, fmt.Sprintf("direct:%d", target), nil
	}
	se, ok := datasources.AsSourceError(err)
	is404 := ok && se.StatusCode == 404
	if !is404 {
		return datasources.FetchedDoc{}, 0, "", err
	}
	if month != "" {
		return datasources.FetchedDoc{}, 0, "", datasources.UnavailableDetail("jodi_oil_primary",
			fmt.Sprintf("%d 年度文件不存在（404）", target), err.Error())
	}
	// Fallback: previous year, once.
	doc, err = tryFetch(target - 1)
	if err != nil {
		if se2, ok2 := datasources.AsSourceError(err); ok2 && se2.StatusCode == 404 {
			return datasources.FetchedDoc{}, 0, "", datasources.UnavailableDetail("jodi_oil_primary",
				fmt.Sprintf("%d 与 %d 年度文件均不存在（404）", target, target-1), "")
		}
		return datasources.FetchedDoc{}, 0, "", err
	}
	return doc, target - 1, fmt.Sprintf("fallback:%d->%d（本年 404，回退上一年一次）", target, target-1), nil
}

func (j *JODI) cacheKey(year int) string { return fmt.Sprintf("jodi:primary:%d", year) }

// tryFetchYear fetches one annual file through the per-year TTL cache.
func (j *JODI) tryFetchYear(ctx context.Context, year int) (*datasources.FetchedDoc, error) {
	key := j.cacheKey(year)
	if cached, ok := j.cache.Get(key); ok {
		return &cached, nil
	}
	url := fmt.Sprintf(j.urlTemplate, year)
	fetched, ferr := j.fetch.Get(ctx, url, map[string]string{
		"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
	})
	if ferr != nil {
		return nil, ferr
	}
	j.cache.Put(key, *fetched)
	return fetched, nil
}

func isJodi404(err error) bool {
	if se, ok := datasources.AsSourceError(err); ok {
		return se.StatusCode == 404
	}
	return false
}

// fetchYearsWindow fetches the explicit multi-year window (newest first).
// Rules (design §5, HR-4/5): only the current-year 404 may fall back to the
// previous year ONCE — for a window ≥2 that year is fetched by the loop
// anyway, so the fallback is the window itself (deduplicated, never fetched
// twice); any OTHER year's 404 is a recorded gap without cascading; a
// non-404 failure (network/5xx) fails the whole call — a transient error must
// not masquerade as an honest data gap.
func (j *JODI) fetchYearsWindow(ctx context.Context, geo, flowCode, resolvedUnit string, window []int) (map[string]any, error) {
	docs := map[int]datasources.FetchedDoc{}
	var order []int // years fetched, newest first
	var gaps []map[string]any
	var strategies []string

	current := window[0]
	doc, err := j.tryFetchYear(ctx, current)
	switch {
	case err == nil:
		docs[current] = *doc
		order = append(order, current)
	case isJodi404(err):
		if len(window) >= 2 {
			// The fallback target (current-1) is already in the window and is
			// fetched exactly once by the loop below — the reuse IS the dedupe.
			gaps = append(gaps, map[string]any{"year": current, "reason": "current year file 404; window continues with previous years"})
			strategies = append(strategies, fmt.Sprintf("current:%d:404（多年窗口继续取上一年，不重复请求）", current))
		} else {
			prev := current - 1
			prevDoc, ferr := j.tryFetchYear(ctx, prev)
			switch {
			case ferr == nil:
				docs[prev] = *prevDoc
				order = append(order, prev)
				strategies = append(strategies, fmt.Sprintf("fallback:%d->%d（本年 404，回退上一年一次）", current, prev))
			case isJodi404(ferr):
				return nil, datasources.UnavailableDetail("jodi_oil_primary",
					fmt.Sprintf("%d 与 %d 年度文件均不存在（404）", current, prev), "")
			default:
				return nil, ferr
			}
		}
	default:
		return nil, err
	}

	for _, year := range window[1:] {
		if _, ok := docs[year]; ok {
			continue // fallback target already fetched — dedupe, no second request
		}
		yearDoc, ferr := j.tryFetchYear(ctx, year)
		if ferr == nil {
			docs[year] = *yearDoc
			order = append(order, year)
			continue
		}
		if isJodi404(ferr) {
			gaps = append(gaps, map[string]any{"year": year, "reason": "annual file 404; gap recorded, no cascading fallback"})
			strategies = append(strategies, fmt.Sprintf("gap:%d:404（历史年缺口，不级联回退）", year))
			continue
		}
		return nil, ferr
	}

	// Parse every fetched file; tag rows with their source year so each
	// observation carries unambiguous provenance.
	var rows []jodiRow
	documents := make([]map[string]any, 0, len(order))
	for _, year := range order {
		yearRows, perr := parseJodiPrimary(docs[year].Payload)
		if perr != nil {
			j.cache.Evict(j.cacheKey(year)) // drift evicts its own file cache
			return nil, perr
		}
		for i := range yearRows {
			yearRows[i].sourceYear = year
		}
		rows = append(rows, yearRows...)
		documents = append(documents, map[string]any{
			"year":          year,
			"url":           docs[year].URL,
			"retrieved_at":  docs[year].RetrievedAt.Format(time.RFC3339),
			"last_modified": docs[year].LastModified,
			"source_sha256": docs[year].SHA256,
		})
	}

	// Filter to the requested dimension; a multi-year window returns ALL
	// available months in the window (month is guaranteed empty here).
	var matched []jodiRow
	for _, r := range rows {
		if r.geo == geo && r.product == jodiProduct && r.flow == flowCode && r.unit == resolvedUnit {
			matched = append(matched, r)
		}
	}

	observations, err := buildJodiObservations(matched)
	if err != nil {
		return nil, err
	}
	sortJodiObservationsByPeriod(observations)

	newest := order[0]
	newestDoc := docs[newest]
	strategy := "direct:multi-year"
	if len(strategies) > 0 {
		strategy = strings.Join(strategies, "; ")
	}
	return map[string]any{
		"source":          "JODI Oil Primary World Database (annual CSV snapshot)",
		"url":             newestDoc.URL,
		"retrieved_at":    newestDoc.RetrievedAt.Format(time.RFC3339),
		"last_modified":   newestDoc.LastModified,
		"source_sha256":   newestDoc.SHA256,
		"from_cache":      newestDoc.FromCache,
		"year_used":       newest,
		"years_requested": len(window),
		"years_used":      order,
		"year_gaps":       gaps,
		"year_strategy":   strategy,
		"documents":       documents,
		"no_data":         len(observations) == 0,
		"observations":    observations,
		"notes": []string{
			fmt.Sprintf("Explicit multi-year window (%d calendar years): all available months of each fetched annual file; a historical-year 404 is a recorded gap, never silently skipped or backfilled.", len(window)),
			"TIME_PERIOD 是数据期而非发布期；年度文件滞后数据月约 1.5-2 个月，覆盖式更新无修订历史列。",
			"assessment_code 原样透传（1/2/3 语义未核实，不是质量好坏标志）。",
			"缺失标记 '-'/'..'/'x' 映射 null 并保留在 raw_value，绝不转 0。",
			"closing_stocks 的 SPR 覆盖口径不假定与 EIA 商业库存可比；不做跨源比较。",
		},
	}, nil
}

// buildJodiObservations groups matched rows by full dimension key: identical
// rows merge (source_rows kept); any semantic conflict is SCHEMA_CHANGED
// (energy-mcp semantics). Shared shape with the single-file path.
func buildJodiObservations(selected []jodiRow) ([]map[string]any, error) {
	type dimKey struct{ geo, product, flow, unit, period string }
	groups := map[dimKey][]jodiRow{}
	var order []dimKey
	for _, r := range selected {
		k := dimKey{r.geo, r.product, r.flow, r.unit, r.period}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}

	observations := make([]map[string]any, 0, len(order))
	for _, k := range order {
		group := groups[k]
		first := group[0]
		for _, other := range group[1:] {
			if other.hasValue != first.hasValue || other.value != first.value ||
				other.rawValue != first.rawValue || other.missingReason != first.missingReason ||
				other.assessmentCode != first.assessmentCode {
				return nil, datasources.SchemaChangedDetail("jodi_oil_primary",
					fmt.Sprintf("维度 %v 存在冲突的重复行：第 %d 行 vs 第 %d 行",
						[]string{k.geo, k.product, k.flow, k.unit, k.period}, first.sourceRow, other.sourceRow), "")
			}
		}
		obs := map[string]any{
			"geo": first.geo, "product": first.product, "flow": first.flow,
			"period": first.period, "frequency": "monthly", "unit": first.unit,
			"value": nil, "raw_value": first.rawValue,
			"source_row": first.sourceRow, "assessment_code": nilIfEmpty(first.assessmentCode),
			"source_year": first.sourceYear,
		}
		if first.hasValue {
			obs["value"] = first.value
		}
		if first.missingReason != "" {
			obs["missing_reason"] = first.missingReason
		}
		if len(group) > 1 {
			rowsList := make([]int, 0, len(group))
			for _, g := range group {
				rowsList = append(rowsList, g.sourceRow)
			}
			obs["source_rows"] = rowsList
		}
		observations = append(observations, obs)
	}
	return observations, nil
}

// sortJodiObservationsByPeriod orders observations oldest→newest (multi-year
// windows are consumed as chronological series).
func sortJodiObservationsByPeriod(observations []map[string]any) {
	sort.SliceStable(observations, func(i, k int) bool {
		return observations[i]["period"].(string) < observations[k]["period"].(string)
	})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// parseJodiPrimary parses the 7-column annual CSV (UTF-8). Missing markers
// map to null; anything neither numeric nor a known marker is SCHEMA_CHANGED.
func parseJodiPrimary(body []byte) ([]jodiRow, error) {
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, datasources.SchemaChangedDetail("jodi_oil_primary", "JODI CSV 读取失败", err.Error())
	}
	want := []string{"REF_AREA", "TIME_PERIOD", "ENERGY_PRODUCT", "FLOW_BREAKDOWN", "UNIT_MEASURE", "OBS_VALUE", "ASSESSMENT_CODE"}
	for i, h := range header {
		if strings.TrimSpace(h) != want[i] {
			return nil, datasources.SchemaChangedDetail("jodi_oil_primary",
				"JODI 表头不符", fmt.Sprintf("col %d: %q", i, strings.TrimSpace(h)))
		}
	}
	var rows []jodiRow
	rowNo := 1
	for {
		rowNo++
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, datasources.SchemaChangedDetail("jodi_oil_primary",
				fmt.Sprintf("JODI CSV 解析失败（第 %d 行）", rowNo), readErr.Error())
		}
		if len(row) == 0 || allBlank(row) {
			continue
		}
		if len(row) != 7 {
			return nil, datasources.SchemaChangedDetail("jodi_oil_primary",
				fmt.Sprintf("第 %d 行：%d 列，应为 7 列", rowNo, len(row)), "")
		}
		for i := range row {
			row[i] = strings.TrimSpace(row[i])
		}
		geo, period, product, flow, unit, obs, code := row[0], row[1], row[2], row[3], row[4], row[5], row[6]
		r := jodiRow{sourceRow: rowNo, geo: geo, period: period, product: product, flow: flow, unit: unit,
			rawValue: obs, assessmentCode: nilIfEmpty(code).(string)}
		if jodiMissingMarkers[obs] {
			r.missingReason = fmt.Sprintf("missing marker %q", obs)
		} else {
			v, has, missing := parseNumber(obs)
			if !has && missing == "" {
				return nil, datasources.SchemaChangedDetail("jodi_oil_primary",
					fmt.Sprintf("第 %d 行：OBS_VALUE %q 既非数值也非已知缺失标记", rowNo, obs), "")
			}
			if has {
				r.value, r.hasValue = v, true
			} else {
				r.missingReason = missing
			}
		}
		rows = append(rows, r)
	}
	return rows, nil
}
