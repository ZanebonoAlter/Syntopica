package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/datasources"
)

// ── Yahoo chart 取数器（add-fast-frequency-datasources tasks 1.2）──────────────

func newYahooOverServer(t *testing.T, handler http.HandlerFunc, hits *int) *Yahoo {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20})
	f.SetClient(testClient())
	y := NewYahoo(f, datasources.NewTTLCache(time.Minute))
	y.baseURL = srv.URL + "/v8/finance/chart"
	return y
}

func yahooChartBody(ts []int64, opens, highs, lows, closes []any, currency any) string {
	meta := map[string]any{"symbol": "CL=F", "regularMarketTime": 1759363200}
	if currency != nil {
		meta["currency"] = currency
	}
	return fmt.Sprintf(`{"chart":{"result":[{"meta":%s,"timestamp":[%s],"indicators":{"quote":[{"open":%s,"high":%s,"low":%s,"close":%s}]}}],"error":null}}`,
		mustJSONString(meta), joinInts(ts), joinJSONAny(opens), joinJSONAny(highs), joinJSONAny(lows), joinJSONAny(closes))
}

func joinInts(ts []int64) string {
	parts := make([]string, len(ts))
	for i, v := range ts {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ",")
}

func joinJSONAny(vs []any) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = mustJSONString(v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func mustJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestYahooFetchValidSymbol: 合法 symbol 返回日度 OHLC 序列（日期+开高低收+
// 货币 USD），close 同时投影为 value（计算索引口径）。
func TestYahooFetchValidSymbol(t *testing.T) {
	// 2026-09-28/29 20:00 UTC（美东 16:00 收盘 → UTC 日期=交易日日期）。
	day1 := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC).Unix()
	day2 := time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC).Unix()
	var hits int
	y := newYahooOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		body := yahooChartBody(
			[]int64{day1, day2},
			[]any{65.1, 66.2}, []any{66.0, 67.1}, []any{64.8, 65.9},
			[]any{65.5, 66.8}, "USD")
		_, _ = w.Write([]byte(body))
	}, &hits)
	ctx := context.Background()

	res, err := y.Fetch(ctx, "CL=F", "1mo")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 2 {
		t.Fatalf("want 2 observations, got %d", len(obs))
	}
	if obs[0]["period"] != "2026-09-28" || obs[1]["period"] != "2026-09-29" {
		t.Fatalf("periods: %v / %v", obs[0]["period"], obs[1]["period"])
	}
	if obs[1]["close"].(float64) != 66.8 || obs[1]["value"].(float64) != 66.8 {
		t.Fatalf("close/value: %v / %v", obs[1]["close"], obs[1]["value"])
	}
	if obs[1]["open"].(float64) != 66.2 || obs[1]["high"].(float64) != 67.1 || obs[1]["low"].(float64) != 65.9 {
		t.Fatalf("ohlc: %v", obs[1])
	}
	if res["currency"] != "USD" || obs[1]["currency"] != "USD" {
		t.Fatalf("currency: %v", res["currency"])
	}
	if res["symbol"] != "CL=F" || res["interval"] != "1d" {
		t.Fatalf("meta: %v", res)
	}
	if hits != 1 {
		t.Fatalf("exactly one upstream call, got %d", hits)
	}
}

// TestYahooSymbolClosedEnumeration: 全 17 个核定 symbol 逐一过校验；枚举外
// （AAPL/小写/空）发网前 INVALID_ARGUMENT（零网络请求）。
func TestYahooSymbolClosedEnumeration(t *testing.T) {
	var hits int
	y := newYahooOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		body := yahooChartBody([]int64{1759089600}, []any{1.0}, []any{1.0}, []any{1.0}, []any{1.0}, "USD")
		_, _ = w.Write([]byte(body))
	}, &hits)
	ctx := context.Background()

	if len(yahooSymbols) != 15 {
		t.Fatalf("first-batch symbol set must be exactly 15 (spec 列举清单；design/tasks 原写 17 系计数笔误，已勘误), got %d", len(yahooSymbols))
	}
	for symbol := range yahooSymbols {
		if _, err := y.Fetch(ctx, symbol, "1mo"); err != nil {
			t.Fatalf("enumerated symbol %s must pass: %v", symbol, err)
		}
	}

	before := hits
	for _, bad := range []string{"AAPL", "cl=f", "", "CL", "GC=F " /* 带空白 */, "CL=F;DROP"} {
		_, err := y.Fetch(ctx, bad, "1mo")
		wantInvalidArg(t, err)
	}
	if hits != before {
		t.Fatalf("INVALID_ARGUMENT must not hit the network: %d extra calls", hits-before)
	}
}

// TestYahooRangeEnumAndNullClose: range 枚举外拒绝（1mo/3mo/6mo/1y 合法、
// 缺省 1mo）；接口 null close 如实为 nil 不插补。
func TestYahooRangeEnumAndNullClose(t *testing.T) {
	day1 := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC).Unix()
	day2 := time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC).Unix()
	var hits int
	y := newYahooOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		body := yahooChartBody(
			[]int64{day1, day2},
			[]any{65.1, nil}, []any{66.0, nil}, []any{64.8, nil},
			[]any{65.5, nil}, "USD")
		_, _ = w.Write([]byte(body))
	}, &hits)
	ctx := context.Background()

	before := hits
	for _, bad := range []string{"5mo", "2y", "1w", "max"} {
		_, err := y.Fetch(ctx, "CL=F", bad)
		wantInvalidArg(t, err)
	}
	if hits != before {
		t.Fatalf("range INVALID_ARGUMENT must not hit the network")
	}

	// 缺省 range=1mo，null close 如实 nil。
	res, err := y.Fetch(ctx, "CL=F", "")
	if err != nil {
		t.Fatalf("default range fetch: %v", err)
	}
	if res["range"] != "1mo" {
		t.Fatalf("default range: %v", res["range"])
	}
	obs := res["observations"].([]map[string]any)
	if obs[1]["close"] != nil || obs[1]["value"] != nil {
		t.Fatalf("null close must stay nil: %v", obs[1])
	}
}

// TestYahooStructureDrift: chart.result 缺失/空、quote 空、timestamp 与 quote
// 长度不齐 → SCHEMA_CHANGED（非官方接口零模糊容错），meta.currency 缺失时
// 货币字段如实空串透传。
func TestYahooStructureDrift(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		body string
	}{
		{"result missing", `{"chart":{"result":[],"error":null}}`},
		{"quote missing", `{"chart":{"result":[{"meta":{"currency":"USD"},"timestamp":[1],"indicators":{"quote":[]}}],"error":null}}`},
		{"length mismatch", `{"chart":{"result":[{"meta":{"currency":"USD"},"timestamp":[1,2],"indicators":{"quote":[{"open":[1,2],"high":[1,2],"low":[1,2],"close":[1]}]}}],"error":null}}`},
	}
	for _, tc := range cases {
		y := newYahooOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(tc.body))
		}, nil)
		_, err := y.Fetch(ctx, "CL=F", "1mo")
		if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSchemaChanged {
			t.Fatalf("%s: want SCHEMA_CHANGED, got %v", tc.name, err)
		}
	}

	// meta.currency 缺失：货币如实空串（透传不编造），不判失败。
	y := newYahooOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		body := yahooChartBody([]int64{1759089600}, []any{1.0}, []any{1.0}, []any{1.0}, []any{1.0}, nil)
		_, _ = w.Write([]byte(body))
	}, nil)
	res, err := y.Fetch(ctx, "CL=F", "1mo")
	if err != nil {
		t.Fatalf("currency-less meta is not drift: %v", err)
	}
	if res["currency"] != "" {
		t.Fatalf("missing currency passes through as empty, got %v", res["currency"])
	}
}

// TestYahooUpstreamError: chart.error 非空 → SOURCE_UNAVAILABLE 携带上游信息。
func TestYahooUpstreamError(t *testing.T) {
	y := newYahooOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":null,"error":{"code":"Unauthorized","description":"crumb required"}}}`))
	}, nil)
	_, err := y.Fetch(context.Background(), "CL=F", "1mo")
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("want SOURCE_UNAVAILABLE, got %v", err)
	}
}
