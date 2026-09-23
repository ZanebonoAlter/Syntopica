package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"syntopica-backend/internal/datasources"
)

const wdiBaseURL = "https://api.worldbank.org/v2"

var (
	wdiIndicatorRe = regexp.MustCompile(`^[A-Z0-9]+(\.[A-Z0-9]+)+$`) // e.g. NE.EXP.GNFS.ZS
	wdiISO3Re      = regexp.MustCompile(`^[A-Z]{3}$`)                // e.g. CHN
)

// WDI fetches World Bank WDI indicators (anonymous API, annual data).
type WDI struct {
	fetch   *datasources.Fetcher
	cache   *datasources.TTLCache
	baseURL string
}

// NewWDI builds the WDI fetcher.
func NewWDI(f *datasources.Fetcher, c *datasources.TTLCache) *WDI {
	return &WDI{fetch: f, cache: c, baseURL: wdiBaseURL}
}

// WDIObservation is one country×year data point (value may be null).
type WDIObservation struct {
	CountryISO3 string   `json:"country_iso3"`
	CountryName string   `json:"country_name"`
	IndicatorID string   `json:"indicator_id"`
	Year        string   `json:"year"`
	Value       *float64 `json:"value"`
}

// Fetch queries one indicator across ISO3 countries for a year range.
// Local validation rejects bad indicators/countries/years BEFORE any network
// call (spec「非法参数拒绝」).
func (w *WDI) Fetch(ctx context.Context, indicator string, countries []string, fromYear, toYear int) (map[string]any, error) {
	now := time.Now().UTC().Year()
	indicator = strings.TrimSpace(indicator)
	if !wdiIndicatorRe.MatchString(indicator) {
		return nil, datasources.InvalidArg("wb_wdi",
			fmt.Sprintf("indicator 必须是点分大写指标码（如 NE.EXP.GNFS.ZS），收到 %q", indicator))
	}
	seen := map[string]bool{}
	var codes []string
	for _, c := range countries {
		c = strings.TrimSpace(strings.ToUpper(c))
		if !wdiISO3Re.MatchString(c) {
			return nil, datasources.InvalidArg("wb_wdi",
				fmt.Sprintf("国家必须是 ISO3 三大写字母（如 CHN），收到 %q", c))
		}
		if !seen[c] {
			seen[c] = true
			codes = append(codes, c)
		}
	}
	if len(codes) == 0 {
		return nil, datasources.InvalidArg("wb_wdi", "至少提供一个国家（ISO3）")
	}
	sort.Strings(codes)
	if fromYear < 1960 || toYear > now+1 || fromYear > toYear {
		return nil, datasources.InvalidArg("wb_wdi",
			fmt.Sprintf("年份范围非法：%d..%d（支持 1960..%d，from≤to）", fromYear, toYear, now+1))
	}

	key := fmt.Sprintf("wdi:%s:%s:%d:%d", indicator, strings.Join(codes, ";"), fromYear, toYear)
	var doc datasources.FetchedDoc
	if cached, ok := w.cache.Get(key); ok {
		doc = cached
	} else {
		url := fmt.Sprintf("%s/country/%s/indicator/%s?format=json&date=%d:%d&per_page=20000",
			w.baseURL, strings.Join(codes, ";"), indicator, fromYear, toYear)
		fetched, err := w.fetch.Get(ctx, url, map[string]string{
			"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
		})
		if err != nil {
			return nil, err
		}
		doc = *fetched
		w.cache.Put(key, doc)
	}

	var payload []json.RawMessage
	if err := json.Unmarshal(doc.Payload, &payload); err != nil {
		w.cache.Evict(key)
		return nil, datasources.SchemaChangedDetail("wb_wdi", "WDI 响应非预期 JSON 数组结构", err.Error())
	}
	if len(payload) != 2 {
		// World Bank error shape: [{"message":{...}}]
		return nil, datasources.UnavailableDetail("wb_wdi",
			"WDI 响应结构异常（可能为无效指标或源端错误）", fmt.Sprintf("payload parts=%d", len(payload)))
	}
	var meta struct {
		// The real payload carries lastupdated as a JSON *string* of a unix
		// timestamp (2026-09-19 live smoke); tolerate both shapes.
		LastUpdated json.RawMessage `json:"lastupdated"`
		Total       int             `json:"total"`
		Pages       int             `json:"pages"`
	}
	if err := json.Unmarshal(payload[0], &meta); err != nil {
		w.cache.Evict(key)
		return nil, datasources.SchemaChangedDetail("wb_wdi", "WDI 元数据段解析失败", err.Error())
	}
	var rows []struct {
		Indicator struct {
			ID string `json:"id"`
		} `json:"indicator"`
		Country struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		} `json:"country"`
		CountryISO3Code string   `json:"countryiso3code"`
		Date            string   `json:"date"`
		Value           *float64 `json:"value"`
	}
	if err := json.Unmarshal(payload[1], &rows); err != nil {
		w.cache.Evict(key)
		return nil, datasources.SchemaChangedDetail("wb_wdi", "WDI 数据段解析失败", err.Error())
	}

	observations := make([]WDIObservation, 0, len(rows))
	for _, r := range rows {
		iso3 := r.CountryISO3Code
		if iso3 == "" {
			iso3 = r.Country.ID // aggregates report iso2-ish id; keep for traceability
		}
		observations = append(observations, WDIObservation{
			CountryISO3: iso3, CountryName: r.Country.Value,
			IndicatorID: r.Indicator.ID, Year: r.Date, Value: r.Value,
		})
	}

	return map[string]any{
		"source":        "World Bank World Development Indicators (WDI)",
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"source_sha256": doc.SHA256,
		"from_cache":    doc.FromCache,
		"lastupdated":   rawOrString(meta.LastUpdated),
		"total":         meta.Total,
		"no_data":       len(observations) == 0,
		"observations":  observations,
		"notes": []string{
			"年度数据；lastupdated 为源端数据集更新时间戳（Unix 秒），非本次抓取时间。",
			"value 为 null 表示该年无观测（保留空期，不删除）。",
			"指标原生单位按指标定义（%/美元等），不做换算。",
		},
	}, nil
}

// rawOrString normalizes a scalar JSON value: strip quotes when it is a
// string so consumers see "1783987200" for both "..."-quoted and bare forms.
func rawOrString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s
}
