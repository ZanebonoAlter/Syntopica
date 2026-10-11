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

// ── GDELT DOC 时间线取数器（add-fast-frequency-datasources tasks 1.4）──────────

func newGdeltOverServer(t *testing.T, handler http.HandlerFunc, hits *int) *Gdelt {
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
	g := NewGdelt(f, datasources.NewTTLCache(time.Minute))
	g.baseURL = srv.URL + "/doc"
	return g
}

// TestGdeltFetchValidTimeline: 合法 query 返回带时间戳的热度点序列（period=
// YYYY-MM-DD、value、series 标签）；缺省 mode=timelinevol / timespan=7d。
func TestGdeltFetchValidTimeline(t *testing.T) {
	var gotPath, gotQuery string
	g := newGdeltOverServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		_, _ = w.Write([]byte(`{"timeline":[{"series":"Volume Intensity","data":[
			{"date":"20261001T040000Z","value":0.12},
			{"date":"20261001T050000Z","value":0.34}
		]}]}`))
	}, nil)

	res, err := g.Fetch(context.Background(), "iran oil", "", "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != "/doc" || gotQuery != "iran oil" {
		t.Fatalf("url: %s q=%s", gotPath, gotQuery)
	}
	if res["mode"] != "timelinevol" || res["timespan"] != "7d" {
		t.Fatalf("defaults: %v", res)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 2 {
		t.Fatalf("want 2 points, got %d", len(obs))
	}
	if obs[0]["period"] != "2026-10-01" || obs[0]["value"].(float64) != 0.12 {
		t.Fatalf("point: %v", obs[0])
	}
	if obs[1]["timestamp"] != "20261001T050000Z" {
		t.Fatalf("raw timestamp kept: %v", obs[1]["timestamp"])
	}
}

// TestGdeltInputValidation: mode 枚举外（artlist）、timespan 枚举外、query
// 超长/控制字符/纯空白 → INVALID_ARGUMENT 发网前拒绝（零网络请求）。
func TestGdeltInputValidation(t *testing.T) {
	var hits int
	g := newGdeltOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}, &hits)
	ctx := context.Background()
	before := hits

	if _, err := g.Fetch(ctx, "iran oil", "artlist", ""); err == nil {
		t.Fatal("mode=artlist must fail (no article bodies)")
	} else {
		wantInvalidArg(t, err)
	}
	if _, err := g.Fetch(ctx, "iran oil", "", "14d"); err == nil {
		t.Fatal("timespan=14d must fail")
	} else {
		wantInvalidArg(t, err)
	}
	long := strings.Repeat("a", 121)
	if _, err := g.Fetch(ctx, long, "", ""); err == nil {
		t.Fatal("121-char query must fail")
	} else {
		wantInvalidArg(t, err)
	}
	if _, err := g.Fetch(ctx, "iran\x00oil", "", ""); err == nil {
		t.Fatal("control-char query must fail")
	} else {
		wantInvalidArg(t, err)
	}
	for _, blank := range []string{"", "   ", "\t"} {
		if _, err := g.Fetch(ctx, blank, "", ""); err == nil {
			t.Fatalf("blank query %q must fail", blank)
		} else {
			wantInvalidArg(t, err)
		}
	}
	if hits != before {
		t.Fatalf("INVALID_ARGUMENT must not hit the network: %d extra calls", hits-before)
	}

	// 120 字符恰在界内（≤120 含边界）。
	ok := newGdeltOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"timeline":[]}`))
	}, nil)
	if _, err := ok.Fetch(ctx, strings.Repeat("a", 120), "", ""); err != nil {
		t.Fatalf("120-char query must pass: %v", err)
	}
}

// TestGdeltEmptyResultAndDrift: 零结果如实空序列；时间戳不可解析 →
// SCHEMA_CHANGED；value 缺失如实 nil。
func TestGdeltEmptyResultAndDrift(t *testing.T) {
	g := newGdeltOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"timeline":[]}`))
	}, nil)
	res, err := g.Fetch(context.Background(), "zzz-unknown-topic", "timelinetone", "30d")
	if err != nil {
		t.Fatalf("zero result is legal: %v", err)
	}
	if !res["no_data"].(bool) || res["mode"] != "timelinetone" || res["timespan"] != "30d" {
		t.Fatalf("empty result: %v", res)
	}
	if len(res["observations"].([]map[string]any)) != 0 {
		t.Fatal("zero result must be an empty (non-nil) series")
	}

	g2 := newGdeltOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"timeline":[{"series":"s","data":[{"date":"not-a-timestamp","value":1}]}]}`))
	}, nil)
	if _, err := g2.Fetch(context.Background(), "oil", "", ""); err == nil {
		t.Fatal("unparseable timestamp must be SCHEMA_CHANGED")
	} else if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSchemaChanged {
		t.Fatalf("want SCHEMA_CHANGED, got %v", err)
	}

	g3 := newGdeltOverServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"timeline":[{"series":"s","data":[{"date":"20261001T040000Z"}]}]}`))
	}, nil)
	res3, err := g3.Fetch(context.Background(), "oil", "", "")
	if err != nil {
		t.Fatalf("missing value is honest null: %v", err)
	}
	obs3 := res3["observations"].([]map[string]any)
	if obs3[0]["value"] != nil {
		t.Fatalf("missing value stays nil: %v", obs3[0])
	}
}
