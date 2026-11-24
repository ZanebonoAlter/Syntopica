package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestProbeHealth_OK covers 四态之 ok：可达且有结果，并验证探针用的是固定词。
func TestProbeHealth_OK(t *testing.T) {
	var gotBody searchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "probe hit", "url": "https://example.com/probe", "summary": "ok", "rerank_score": 1.2},
			},
		})
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	health, err := ProbeHealth(context.Background())
	if err != nil {
		t.Fatalf("ProbeHealth: %v", err)
	}
	if health.Status != HealthStatusOK {
		t.Fatalf("Status = %q, want %q (detail: %s)", health.Status, HealthStatusOK, health.Detail)
	}
	if health.ErrorKind != "" {
		t.Fatalf("ErrorKind = %q, want empty on ok", health.ErrorKind)
	}
	if health.LatencyMS < 0 {
		t.Fatalf("LatencyMS = %d", health.LatencyMS)
	}
	if health.Detail == "" {
		t.Fatal("Detail must not be empty")
	}
	if gotBody.Query != HealthProbeQuery || gotBody.Count != DefaultResultCount || gotBody.NeedContent {
		t.Fatalf("probe body = %+v, want fixed query %q", gotBody, HealthProbeQuery)
	}
}

// TestProbeHealth_Degraded covers 四态之 degraded：HTTP 可达但 results 空。
func TestProbeHealth_Degraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	health, err := ProbeHealth(context.Background())
	if err != nil {
		t.Fatalf("ProbeHealth: %v", err)
	}
	if health.Status != HealthStatusDegraded {
		t.Fatalf("Status = %q, want %q", health.Status, HealthStatusDegraded)
	}
	if health.ErrorKind != "" {
		t.Fatalf("ErrorKind = %q, want empty on degraded", health.ErrorKind)
	}
}

// TestProbeHealth_Disabled covers 四态之 disabled：无可用供应商密钥（凭据解析
// 即止，不会发请求）。
func TestProbeHealth_Disabled(t *testing.T) {
	stubNotConfigured(t)

	health, err := ProbeHealth(context.Background())
	if err != nil {
		t.Fatalf("ProbeHealth: %v", err)
	}
	if health.Status != HealthStatusDisabled {
		t.Fatalf("Status = %q, want %q", health.Status, HealthStatusDisabled)
	}
	if health.ErrorKind != "" {
		t.Fatalf("ErrorKind = %q, want empty on disabled", health.ErrorKind)
	}
}

// TestProbeHealth_UnreachableByKind covers 四态之 unreachable：鉴权/额度/限速/
// 服务故障都归 unreachable，但 ErrorKind 必须给出具体类别（面板据此出文案）。
// 未知类别的 Detail 是固定文案：原始响应体（可能是 5xx 返回的 HTML）只进日志，
// 不得透到面板。
func TestProbeHealth_UnreachableByKind(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		body            string
		wantKind        ErrorKind
		wantNotContains string // 响应体片段：MUST NOT 出现在 Detail 里
	}{
		{"auth", http.StatusUnauthorized, `{"message":"invalid api key"}`, KindAuth, "invalid api key"},
		{"quota 402", http.StatusPaymentRequired, `{"message":"payment required"}`, KindQuota, "payment required"},
		{"quota 403 wording", http.StatusForbidden, `You do not have enough money or package quota`, KindQuota, "package quota"},
		{"rate limited", http.StatusTooManyRequests, `{"message":"too many requests"}`, KindRateLimited, "too many requests"},
		{"server error", http.StatusInternalServerError, `boom`, KindUnavailable, "boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			stubCredentials(t, srv.URL)

			health, err := ProbeHealth(context.Background())
			if err != nil {
				t.Fatalf("ProbeHealth: %v", err)
			}
			if health.Status != HealthStatusUnreachable {
				t.Fatalf("Status = %q, want %q", health.Status, HealthStatusUnreachable)
			}
			if health.ErrorKind != tc.wantKind {
				t.Fatalf("ErrorKind = %q, want %q", health.ErrorKind, tc.wantKind)
			}
			if health.Detail == "" {
				t.Fatal("Detail must not be empty on unreachable")
			}
			if strings.Contains(health.Detail, tc.wantNotContains) {
				t.Fatalf("Detail = %q, must not leak response body %q (log only)", health.Detail, tc.wantNotContains)
			}
		})
	}
}

// TestProbeHealth_Timeout covers 超时归 unreachable + timeout。
func TestProbeHealth_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	health, err := ProbeHealth(ctx)
	if err != nil {
		t.Fatalf("ProbeHealth: %v", err)
	}
	if health.Status != HealthStatusUnreachable {
		t.Fatalf("Status = %q, want %q", health.Status, HealthStatusUnreachable)
	}
	if health.ErrorKind != KindTimeout {
		t.Fatalf("ErrorKind = %q, want %q", health.ErrorKind, KindTimeout)
	}
}
