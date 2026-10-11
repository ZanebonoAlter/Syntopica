package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"syntopica-backend/internal/datasources"
)

const gdeltDocBaseURL = "https://api.gdeltproject.org/api/v2/doc/doc"

// gdeltModes / gdeltTimespans are the closed enumerations (design decision
// 4): timelinevol = volume heat, timelinetone = tone. artlist (article
// bodies) is deliberately absent — this source exposes structured timeline
// aggregates only, never news text.
var gdeltModes = map[string]bool{"timelinevol": true, "timelinetone": true}

var gdeltTimespans = map[string]bool{"7d": true, "30d": true, "90d": true}

// gdeltQueryMaxRunes bounds the query keywords (≤120 chars, no control
// characters, non-blank) — validated BEFORE any network call.
const gdeltQueryMaxRunes = 120

// Gdelt fetches structured news-volume timeline aggregates from the GDELT
// DOC API (anonymous). It is the event-layer opening for the research loop:
// timestamped heat/tone points only, NOT fact-checking and NOT news
// retrieval — the tool description says so verbatim (spec SHALL NOT).
type Gdelt struct {
	fetch   *datasources.Fetcher
	cache   *datasources.TTLCache
	baseURL string
}

// NewGdelt builds the fetcher. Anonymous source.
func NewGdelt(f *datasources.Fetcher, c *datasources.TTLCache) *Gdelt {
	return &Gdelt{fetch: f, cache: c, baseURL: gdeltDocBaseURL}
}

// Fetch returns the timeline point series for one query. timespan=7d yields
// hourly points, 30d/90d daily points (as-of implementation the GDELT docs
// describe; zero results return an honest empty series, never fabricated).
func (g *Gdelt) Fetch(ctx context.Context, query, mode, timespan string) (map[string]any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, datasources.InvalidArg("gdelt", "query 不能为空（关键词或关键词组合，如 \"iran oil\"）")
	}
	if r := []rune(query); len(r) > gdeltQueryMaxRunes {
		return nil, datasources.InvalidArg("gdelt",
			fmt.Sprintf("query 不得超过 %d 字符，收到 %d 字符", gdeltQueryMaxRunes, len(r)))
	}
	for _, r := range query {
		if unicode.IsControl(r) {
			return nil, datasources.InvalidArg("gdelt",
				fmt.Sprintf("query 不得含控制字符（收到 U+%04X），拒绝注入式串", r))
		}
	}
	if mode == "" {
		mode = "timelinevol"
	}
	if !gdeltModes[mode] {
		return nil, datasources.InvalidArg("gdelt",
			fmt.Sprintf("mode 必须是 timelinevol（声量热度）或 timelinetone（情绪基调）之一，收到 %q；本源只提供结构化时间线聚合，不提供新闻正文", mode))
	}
	if timespan == "" {
		timespan = "7d"
	}
	if !gdeltTimespans[timespan] {
		return nil, datasources.InvalidArg("gdelt",
			fmt.Sprintf("timespan 必须是 7d/30d/90d 之一（缺省 7d），收到 %q", timespan))
	}

	u := fmt.Sprintf("%s?query=%s&mode=%s&timespan=%s&format=json",
		g.baseURL, url.QueryEscape(query), mode, timespan)

	key := "gdelt:" + mode + ":" + timespan + ":" + query
	var doc datasources.FetchedDoc
	if cached, ok := g.cache.Get(key); ok {
		doc = cached
	} else {
		fetched, err := g.fetch.Get(ctx, u, map[string]string{
			"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
		})
		if err != nil {
			return nil, err
		}
		doc = *fetched
		g.cache.Put(key, doc)
	}

	var payload struct {
		Timeline []struct {
			Series string `json:"series"`
			Data   []struct {
				Date  string   `json:"date"`
				Value *float64 `json:"value"`
			} `json:"data"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal(doc.Payload, &payload); err != nil {
		g.cache.Evict(key)
		return nil, datasources.SchemaChangedDetail("gdelt", "GDELT DOC 响应解析失败", err.Error())
	}

	// 零结果（极冷门 query）如实返回空序列不编造；timeline 缺键视为零结果
	// 同语义（GDELT 空命中返回的形状不稳定，缺键不猜列义）。
	type obs = map[string]any
	var observations []obs
	var seriesLabels []string
	for _, tl := range payload.Timeline {
		seriesLabels = append(seriesLabels, tl.Series)
		for _, p := range tl.Data {
			ts, err := time.Parse("20060102T150405Z", p.Date)
			if err != nil {
				g.cache.Evict(key)
				return nil, datasources.SchemaChangedDetail("gdelt",
					"GDELT 时间线 date 不是 ISO 时间戳（上游结构漂移，零模糊容错）", p.Date)
			}
			observations = append(observations, obs{
				"query":     query,
				"mode":      mode,
				"series":    tl.Series,
				"period":    ts.UTC().Format("2006-01-02"),
				"timestamp": p.Date,
				"value":     flattenFloatPtr(p.Value),
			})
		}
	}
	if observations == nil {
		observations = []obs{}
	}

	return map[string]any{
		"source":        "GDELT DOC 2.0 API (global news aggregation)",
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"source_sha256": doc.SHA256,
		"from_cache":    doc.FromCache,
		"query":         query,
		"mode":          mode,
		"timespan":      timespan,
		"series":        seriesLabels,
		"rows":          len(observations),
		"no_data":       len(observations) == 0,
		"observations":  observations,
		"notes": []string{
			"结构化热度/情绪走势（无量纲指数），非事实核查、非新闻正文：不得据此直接断言某事件发生与否，只用于感知报道热度与基调变化。",
			"timespan=7d 为小时级点，30d/90d 为日级点；period 取时间戳的 UTC 日期。",
			"零结果（极冷门关键词）如实返回空序列；典型滞后小时级（准实时聚合）。",
		},
	}, nil
}

// flattenFloatPtr turns a typed-nil *float64 into a real nil interface
// (typed nils serialize as 0 in JSON and defeat != nil assertions).
func flattenFloatPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
