package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/datasources"
)

// ── FRED 序列取数器（add-fast-frequency-datasources tasks 1.3）────────────────

func newFredOverServer(t *testing.T, handler http.HandlerFunc, hits *int) *Fred {
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
	fr := NewFred(f, datasources.NewTTLCache(time.Minute), func() string { return "test-fred-key" })
	fr.baseURL = srv.URL + "/fred"
	return fr
}

// TestFredKeyMissingFailsBeforeNetwork: key 缺失 → 配置缺失型
// SOURCE_UNAVAILABLE（detail 指向配置），不发上游请求。
func TestFredKeyMissingFailsBeforeNetwork(t *testing.T) {
	var hits int
	fr := newFredOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}, &hits)
	fr.keyFn = func() string { return "" }

	_, err := fr.Fetch(context.Background(), "DCOILWTICO", "", "")
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("want SOURCE_UNAVAILABLE (config missing), got %v", err)
	} else if !strings.Contains(se.Detail, "FRED") {
		t.Fatalf("detail must name the config: %q", se.Detail)
	}
	if hits != 0 {
		t.Fatalf("missing key must not hit upstream, got %d calls", hits)
	}
}

// TestFredFetchValidSeries: 合法 series 返回观测序列（date+value+units+
// frequency 原样透传）；`.` 缺失值映射 null 不转 0。
func TestFredFetchValidSeries(t *testing.T) {
	fr := newFredOverServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/series/observations") {
			_, _ = w.Write([]byte(`{"observations":[
				{"date":"2026-09-28","value":"82.53"},
				{"date":"2026-09-29","value":"."},
				{"date":"2026-09-30","value":"83.10"}
			]}`))
			return
		}
		_, _ = w.Write([]byte(`{"seriess":[{"id":"DCOILWTICO","title":"Crude Oil Prices: WTI","units":"Dollars per Barrel","frequency":"Daily"}]}`))
	}, nil)

	res, err := fr.Fetch(context.Background(), "DCOILWTICO", "", "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 3 {
		t.Fatalf("want 3 observations, got %d", len(obs))
	}
	if obs[0]["period"] != "2026-09-28" || obs[0]["value"].(float64) != 82.53 {
		t.Fatalf("first obs: %v", obs[0])
	}
	if obs[1]["value"] != nil {
		t.Fatalf("\".\" must map to null, got %v", obs[1]["value"])
	}
	if res["units"] != "Dollars per Barrel" || res["frequency"] != "Daily" {
		t.Fatalf("units/frequency passthrough: %v / %v", res["units"], res["frequency"])
	}
	if res["title"] != "Crude Oil Prices: WTI" {
		t.Fatalf("title: %v", res["title"])
	}
}

// TestFredSeriesIDFormat: series_id 小写/特殊字符/超长 → INVALID_ARGUMENT
// 发网前拒绝（格式即边界，非枚举）。
func TestFredSeriesIDFormat(t *testing.T) {
	var hits int
	fr := newFredOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}, &hits)
	ctx := context.Background()
	before := hits
	for _, bad := range []string{"dcoilwtico", "DCOIL WTICO", "DCOILWTICO!", "a", "", strings.Repeat("A", 65), "dc-oil"} {
		_, err := fr.Fetch(ctx, bad, "", "")
		wantInvalidArg(t, err)
	}
	if hits != before {
		t.Fatalf("INVALID_ARGUMENT must not hit the network")
	}
}

// TestFredWindowBounds: start>end、窗口>10 年、end 晚于当前 → INVALID_ARGUMENT；
// 恰 10 年边界通过（≤10 年含边界）。
func TestFredWindowBounds(t *testing.T) {
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)

	if s, e, err := fredWindow("", "", now); err != nil || s != "2025-10-02" || e != "2026-10-02" {
		t.Fatalf("default window = last 1 year, got %s~%s err=%v", s, e, err)
	}
	if s, e, err := fredWindow("2016-10-02", "2026-10-02", now); err != nil || s != "2016-10-02" || e != "2026-10-02" {
		t.Fatalf("exactly 10y must pass, got %s~%s err=%v", s, e, err)
	}
	if _, _, err := fredWindow("2016-10-01", "2026-10-02", now); err == nil {
		t.Fatal("window > 10y must fail")
	} else {
		wantInvalidArg(t, err)
	}
	if _, _, err := fredWindow("2026-01-05", "2026-01-01", now); err == nil {
		t.Fatal("start>end must fail")
	} else {
		wantInvalidArg(t, err)
	}
	if _, _, err := fredWindow("", "2026-12-31", now); err == nil {
		t.Fatal("future end must fail")
	} else {
		wantInvalidArg(t, err)
	}
	if _, _, err := fredWindow("2026-9-1", "", now); err == nil {
		t.Fatal("malformed date must fail")
	} else {
		wantInvalidArg(t, err)
	}
}

// TestFredEmptyWindowAndDrift: 零行观测如实空序列；date/value 形态漂移 →
// SCHEMA_CHANGED（零模糊容错）；info 元数据缺失 → units/frequency null 透传
// 不阻塞观测。
func TestFredEmptyWindowAndDrift(t *testing.T) {
	fr := newFredOverServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/series/observations") {
			_, _ = w.Write([]byte(`{"observations":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"seriess":[]}`))
	}, nil)
	res, err := fr.Fetch(context.Background(), "SOFR", "", "")
	if err != nil {
		t.Fatalf("empty observations are legal: %v", err)
	}
	if !res["no_data"].(bool) || len(res["observations"].([]map[string]any)) != 0 {
		t.Fatalf("empty window: %v", res)
	}
	if res["units"] != nil || res["frequency"] != nil {
		t.Fatalf("missing metadata passes through as null: %v/%v", res["units"], res["frequency"])
	}

	fr2 := newFredOverServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/series/observations") {
			_, _ = w.Write([]byte(`{"observations":[{"date":"202609","value":"1"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}, nil)
	if _, err := fr2.Fetch(context.Background(), "SOFR", "", ""); err == nil {
		t.Fatal("non YYYY-MM-DD date must be SCHEMA_CHANGED")
	} else if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSchemaChanged {
		t.Fatalf("want SCHEMA_CHANGED, got %v", err)
	}

	fr3 := newFredOverServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/series/observations") {
			_, _ = w.Write([]byte(`{"observations":[{"date":"2026-09-30","value":"abc"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}, nil)
	if _, err := fr3.Fetch(context.Background(), "SOFR", "", ""); err == nil {
		t.Fatal("non-numeric non-dot value must be SCHEMA_CHANGED")
	}
}
