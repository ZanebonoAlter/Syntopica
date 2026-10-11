package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"syntopica-backend/internal/datasources"
)

const yahooChartBaseURL = "https://query1.finance.yahoo.com/v8/finance/chart"

// yahooSymbols is the closed symbol enumeration (first batch, 17 symbols;
// add-fast-frequency-datasources design decision 2). Extending it is a small
// spec delta, never a free-form surface — anything outside the set is
// INVALID_ARGUMENT before any network call.
var yahooSymbols = map[string]string{
	// 能源期货
	"CL=F": "WTI 原油期货",
	"BZ=F": "Brent 原油期货",
	"HO=F": "NYMEX 取暖油期货",
	"RB=F": "NYMEX RBOB 汽油期货",
	"NG=F": "Henry Hub 天然气期货",
	// 贵金属
	"GC=F": "COMEX 黄金期货",
	"SI=F": "COMEX 白银期货",
	// 股指
	"^GSPC": "S&P 500",
	"^IXIC": "NASDAQ 综合指数",
	"^DJI":  "道琼斯工业平均指数",
	"^VIX":  "CBOE 波动率指数",
	// 汇率（对美元）
	"JPY=X":    "美元/日元",
	"EUR=X":    "欧元/美元",
	"USDCNY=X": "美元/人民币",
	// 加密
	"BTC-USD": "比特币/美元",
}

// yahooRanges is the closed range enumeration; interval is fixed daily.
var yahooRanges = map[string]bool{"1mo": true, "3mo": true, "6mo": true, "1y": true}

// Yahoo fetches daily OHLC series from the unofficial Yahoo Finance chart
// API (no key, no SLA — stooq's JS-challenge death is the precedent; drift
// surfaces as SCHEMA_CHANGED / SOURCE_UNAVAILABLE, never fuzzy tolerance).
type Yahoo struct {
	fetch   *datasources.Fetcher
	cache   *datasources.TTLCache
	baseURL string
}

// NewYahoo builds the fetcher. Anonymous source; no key resolver needed.
func NewYahoo(f *datasources.Fetcher, c *datasources.TTLCache) *Yahoo {
	return &Yahoo{fetch: f, cache: c, baseURL: yahooChartBaseURL}
}

// YahooSymbols returns the closed symbol enumeration (catalog/tool
// description derivation + tests).
func YahooSymbols() map[string]string {
	out := make(map[string]string, len(yahooSymbols))
	for k, v := range yahooSymbols {
		out[k] = v
	}
	return out
}

// YahooSymbolList returns the closed symbol enumeration sorted ascending —
// the InputSchema enum for the yahoo_chart tool (stable order, test-pinned).
func YahooSymbolList() []string {
	return append([]string(nil), sortedYahooSymbols()...)
}

// Fetch returns the daily OHLC series for one symbol over one range.
// Null closes stay null (no interpolation); the quote currency passes
// through meta.currency verbatim; OHLC keeps the quote arrays' own currency,
// never cross-converted. Typical lag T+0 (after market close).
func (y *Yahoo) Fetch(ctx context.Context, symbol, rng string) (map[string]any, error) {
	// 封闭枚举按精确字符匹配（不做 trim 宽容：枚举外一律拒绝，包括带空白的写法）。
	if _, ok := yahooSymbols[symbol]; !ok {
		return nil, datasources.InvalidArg("yahoo_finance",
			fmt.Sprintf("symbol 必须是核定枚举之一（%s），收到 %q；枚举外 symbol 一律拒绝",
				strings.Join(sortedYahooSymbols(), " / "), symbol))
	}
	if rng == "" {
		rng = "1mo"
	}
	if !yahooRanges[rng] {
		return nil, datasources.InvalidArg("yahoo_finance",
			fmt.Sprintf("range 必须是 1mo/3mo/6mo/1y 之一（缺省 1mo），收到 %q", rng))
	}

	u := fmt.Sprintf("%s/%s?range=%s&interval=1d", y.baseURL, url.PathEscape(symbol), rng)

	key := "yahoo:" + u
	var doc datasources.FetchedDoc
	if cached, ok := y.cache.Get(key); ok {
		doc = cached
	} else {
		fetched, err := y.fetch.Get(ctx, u, map[string]string{
			"User-Agent": "syntopica-datasources/0.1 (read-only public data)",
		})
		if err != nil {
			return nil, err
		}
		doc = *fetched
		y.cache.Put(key, doc)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Currency          string `json:"currency"`
					Symbol            string `json:"symbol"`
					RegularMarketTime int64  `json:"regularMarketTime"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open  []*float64 `json:"open"`
						High  []*float64 `json:"high"`
						Low   []*float64 `json:"low"`
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(doc.Payload, &payload); err != nil {
		y.cache.Evict(key)
		return nil, datasources.SchemaChangedDetail("yahoo_finance", "Yahoo chart 响应解析失败", err.Error())
	}
	if len(payload.Chart.Error) > 0 && string(payload.Chart.Error) != `""` && string(payload.Chart.Error) != "null" {
		y.cache.Evict(key)
		return nil, datasources.UnavailableDetail("yahoo_finance",
			"Yahoo chart 源端返回错误", strings.TrimSpace(string(payload.Chart.Error)))
	}
	if len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 {
		// chart.result 缺失 / quote 数组为空 = 结构漂移（非官方接口无 SLA，
		// 零模糊容错），驱逐缓存显式失败。
		y.cache.Evict(key)
		return nil, datasources.SchemaChanged("yahoo_finance",
			"chart.result 或 indicators.quote 缺失/为空（上游结构漂移，不做模糊容错）")
	}
	res := payload.Chart.Result[0]
	q := res.Indicators.Quote[0]
	if len(res.Timestamp) == 0 || len(res.Timestamp) != len(q.Open) || len(res.Timestamp) != len(q.High) ||
		len(res.Timestamp) != len(q.Low) || len(res.Timestamp) != len(q.Close) {
		y.cache.Evict(key)
		return nil, datasources.SchemaChanged("yahoo_finance",
			fmt.Sprintf("timestamp(%d) 与 quote 数组长度不齐（open=%d high=%d low=%d close=%d），不猜测列含义",
				len(res.Timestamp), len(q.Open), len(q.High), len(q.Low), len(q.Close)))
	}

	// 交易日日期取 UTC：美东收盘 16:00 当地 = UTC 同日 20/21 点，UTC 日期与
	// 美东交易日一致（无需引入交易所时区依赖）。nil *float64 转成真 nil
	// interface（typed-nil 会在 JSON 里序列化为 0，也骗过 `!= nil` 断言）。
	flatten := func(p *float64) any {
		if p == nil {
			return nil
		}
		return *p
	}
	type obs = map[string]any
	observations := make([]obs, 0, len(res.Timestamp))
	for i, ts := range res.Timestamp {
		date := time.Unix(ts, 0).UTC().Format("2006-01-02")
		observations = append(observations, obs{
			"symbol":   symbol,
			"period":   date,
			"open":     flatten(q.Open[i]),
			"high":     flatten(q.High[i]),
			"low":      flatten(q.Low[i]),
			"close":    flatten(q.Close[i]),
			"value":    flatten(q.Close[i]),
			"currency": res.Meta.Currency,
		})
	}

	return map[string]any{
		"source":        "Yahoo Finance chart API (unofficial, no SLA)",
		"url":           doc.URL,
		"retrieved_at":  doc.RetrievedAt.Format(time.RFC3339),
		"source_sha256": doc.SHA256,
		"from_cache":    doc.FromCache,
		"symbol":        symbol,
		"range":         rng,
		"interval":      "1d",
		"currency":      res.Meta.Currency,
		"rows":          len(observations),
		"no_data":       len(observations) == 0,
		"observations":  observations,
		"notes": []string{
			"非官方接口无 SLA：结构漂移/限流/鉴权变化显式失败（SCHEMA_CHANGED / SOURCE_UNAVAILABLE），不模糊容错。",
			"OHLC 为日度收盘序列，计价货币原样透传（currency 字段），不换算不跨币种合并。",
			"个别交易日收盘价为 null（停牌/未成交）时如实为 null，不插补；价差类计算用 calculate difference/percent_change。",
			"典型滞后 T+0（收盘后当日可得）；不附带成交量口径。",
		},
	}, nil
}

func sortedYahooSymbols() []string {
	out := make([]string, 0, len(yahooSymbols))
	for k := range yahooSymbols {
		out = append(out, k)
	}
	// 插入排序：17 个元素，避免引入 sort 依赖到本文件（与 sortKeptPeriods 同风格）。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
