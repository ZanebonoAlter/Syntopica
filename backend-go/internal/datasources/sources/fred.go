package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/datasources"
)

const fredBaseURL = "https://api.stlouisfed.org/fred"

// fredSeriesIDRe: series_id is format-bounded, not enumerated (FRED exposes
// ~800k series; the format IS the boundary — add-fast-frequency-datasources
// design decision 3).
var fredSeriesIDRe = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)

// fredMaxWindow bounds the observation window to ≤10 years (calendar
// comparison, leap-safe: end ≤ start+10y passes, one day more is rejected).
const fredMaxWindowYears = 10

// Fred fetches FRED series observations. Requires the free API key
// (stlouisfed.org 注册 1 分钟；resolver 链 DB > env FRED_API_KEY > config.yaml
// fred.api_key，同 Comtrade 动态链).
type Fred struct {
	fetch   *datasources.Fetcher
	cache   *datasources.TTLCache
	baseURL string
	keyFn   func() string
}

// NewFred builds the fetcher. keyFn is resolved on every Fetch (dynamic:
// UI-configured key takes effect without restart); "" → explicit
// missing-key SOURCE_UNAVAILABLE before any network call.
func NewFred(f *datasources.Fetcher, c *datasources.TTLCache, keyFn func() string) *Fred {
	return &Fred{fetch: f, cache: c, baseURL: fredBaseURL, keyFn: keyFn}
}

// Fetch returns the observation series (date + value) plus the series
// metadata (title/units/frequency) passed through verbatim. FRED "." missing
// values map to null, never 0. Default window = last 1 year; explicit
// start/end bounded to ≤10 years.
func (fr *Fred) Fetch(ctx context.Context, seriesID, startDate, endDate string) (map[string]any, error) {
	apiKey := strings.TrimSpace(fr.keyFn())
	if apiKey == "" {
		return nil, datasources.UnavailableDetail("fred",
			"缺少 FRED API key", "请在设置界面「研究数据源」配置 FRED key（即时生效），或配置 FRED_API_KEY / config.yaml fred.api_key；key 于 stlouisfed.org 免费注册")
	}
	seriesID = strings.TrimSpace(seriesID)
	if !fredSeriesIDRe.MatchString(seriesID) {
		return nil, datasources.InvalidArg("fred",
			fmt.Sprintf("series_id 必须匹配 ^[A-Z0-9_]{1,64}$（大写字母/数字/下划线，如 DCOILWTICO），收到 %q", seriesID))
	}

	now := time.Now().UTC()
	start, end, err := fredWindow(startDate, endDate, now)
	if err != nil {
		return nil, err
	}

	obsURL := fmt.Sprintf("%s/series/observations?series_id=%s&observation_start=%s&observation_end=%s&api_key=%s&file_type=json",
		fr.baseURL, url.QueryEscape(seriesID), start, end, url.QueryEscape(apiKey))
	infoURL := fmt.Sprintf("%s/series?series_id=%s&api_key=%s&file_type=json",
		fr.baseURL, url.QueryEscape(seriesID), url.QueryEscape(apiKey))

	obsDoc, err := fr.getJSON(ctx, "fred:obs:"+seriesID+":"+start+":"+end, obsURL)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(obsDoc.Payload, &payload); err != nil {
		fr.cache.Evict("fred:obs:" + seriesID + ":" + start + ":" + end)
		return nil, datasources.SchemaChangedDetail("fred", "FRED observations 响应解析失败", err.Error())
	}

	type obs = map[string]any
	observations := make([]obs, 0, len(payload.Observations))
	for _, o := range payload.Observations {
		if _, err := time.Parse("2006-01-02", o.Date); err != nil {
			fr.cache.Evict("fred:obs:" + seriesID + ":" + start + ":" + end)
			return nil, datasources.SchemaChangedDetail("fred",
				"FRED 观测 date 不是 YYYY-MM-DD（上游结构漂移，零模糊容错）", o.Date)
		}
		var value any
		if o.Value != "." {
			v, err := strconv.ParseFloat(o.Value, 64)
			if err != nil {
				fr.cache.Evict("fred:obs:" + seriesID + ":" + start + ":" + end)
				return nil, datasources.SchemaChangedDetail("fred",
					"FRED 观测 value 既不是 . 也不是数值（上游结构漂移）", o.Value)
			}
			value = v
		}
		observations = append(observations, obs{
			"series_id": seriesID,
			"period":    o.Date,
			"value":     value,
		})
	}

	// Series metadata (title/units/frequency) passes through verbatim; the
	// info request failing structurally degrades those fields to null — it
	// never blocks the observations (白盒用例「units/frequency 字段缺失→透传」).
	var title, units, frequency any
	if infoDoc, err := fr.getJSON(ctx, "fred:info:"+seriesID, infoURL); err == nil {
		var info struct {
			Series []struct {
				ID        string `json:"id"`
				Title     string `json:"title"`
				Units     string `json:"units"`
				Frequency string `json:"frequency"`
			} `json:"seriess"`
		}
		if json.Unmarshal(infoDoc.Payload, &info) == nil && len(info.Series) > 0 {
			title, units, frequency = info.Series[0].Title, info.Series[0].Units, info.Series[0].Frequency
		}
	} else if se, ok := datasources.AsSourceError(err); ok && se.Kind == datasources.ErrSchemaChanged {
		fr.cache.Evict("fred:info:" + seriesID)
	}

	return map[string]any{
		"source":        "FRED (Federal Reserve Bank of St. Louis)",
		"url":           obsDoc.URL,
		"retrieved_at":  obsDoc.RetrievedAt.Format(time.RFC3339),
		"source_sha256": obsDoc.SHA256,
		"from_cache":    obsDoc.FromCache,
		"series_id":     seriesID,
		"title":         title,
		"units":         units,
		"frequency":     frequency,
		"rows":          len(observations),
		"no_data":       len(observations) == 0,
		"observations":  observations,
		"notes": []string{
			"units/frequency/title 为序列元数据原样透传，不换算不推断；取不到时为 null。",
			"缺失观测（FRED \".\" 值）如实为 null，绝不转 0。",
			"观测窗口缺省最近 1 年；显式 start/end 受 ≤10 年约束。日度序列典型滞后 T+0~T+1。",
			"常用序列：DCOILWTICO/DCOILBRENTEU（WTI/Brent 日度现货，Dollars per Barrel）、SOFR、DGS10（10 年期美债）、DTWEXBGS（美元指数）。",
		},
	}, nil
}

// fredWindow validates/normalizes the observation window: empty start/end →
// last 1 year; both must be YYYY-MM-DD; start ≤ end; span ≤ 10 years.
func fredWindow(startDate, endDate string, now time.Time) (string, string, error) {
	parse := func(s string) (time.Time, bool) {
		if strings.TrimSpace(s) == "" {
			return time.Time{}, true
		}
		t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
		return t, err == nil
	}
	start, ok := parse(startDate)
	if !ok {
		return "", "", datasources.InvalidArg("fred",
			fmt.Sprintf("start 必须是 YYYY-MM-DD，收到 %q", startDate))
	}
	end, ok := parse(endDate)
	if !ok {
		return "", "", datasources.InvalidArg("fred",
			fmt.Sprintf("end 必须是 YYYY-MM-DD，收到 %q", endDate))
	}
	switch {
	case start.IsZero() && end.IsZero():
		end = now
		start = end.AddDate(-1, 0, 0)
	case end.IsZero():
		end = now
	case start.IsZero():
		start = end.AddDate(-10, 0, 0)
	}
	if start.After(end) {
		return "", "", datasources.InvalidArg("fred",
			fmt.Sprintf("start（%s）不得晚于 end（%s）", start.Format("2006-01-02"), end.Format("2006-01-02")))
	}
	if end.After(start.AddDate(fredMaxWindowYears, 0, 0)) {
		return "", "", datasources.InvalidArg("fred",
			fmt.Sprintf("观测窗口不得超过 10 年（%s ~ %s），请分段取数", start.Format("2006-01-02"), end.Format("2006-01-02")))
	}
	if end.After(now.AddDate(0, 0, 1)) {
		return "", "", datasources.InvalidArg("fred",
			fmt.Sprintf("end 不得晚于当前日期，收到 %s", end.Format("2006-01-02")))
	}
	return start.Format("2006-01-02"), end.Format("2006-01-02"), nil
}

// getJSON is the shared cache+fetch helper for FRED's two endpoints.
func (fr *Fred) getJSON(ctx context.Context, key, u string) (*datasources.FetchedDoc, error) {
	if cached, ok := fr.cache.Get(key); ok {
		return &cached, nil
	}
	doc, err := fr.fetch.Get(ctx, u, map[string]string{
		"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
	})
	if err != nil {
		return nil, err
	}
	fr.cache.Put(key, *doc)
	return doc, nil
}
