package datasources

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestFetcher builds a fetcher whose client trusts the httptest TLS cert
// (same-package white-box: we swap the client, production never does).
func newTestFetcher(t *testing.T, allowlist []string, budget FetchBudget, srvURL string) *Fetcher {
	t.Helper()
	f := NewFetcher(allowlist, budget)
	f.client = &http.Client{
		Timeout: budget.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // test-only: trust httptest self-signed cert
	}
	return f
}

func TestFetcherAllowlist(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("a,b,c\n1,2,3\n"))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	f := newTestFetcher(t, []string{host}, FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20}, srv.URL)

	if _, err := f.Get(context.Background(), srv.URL+"/x.csv", nil); err != nil {
		t.Fatalf("allowlisted host should pass: %v", err)
	}
	// Not in allowlist → SOURCE_UNAVAILABLE with host detail.
	_, err := f.Get(context.Background(), "https://definitely.not.allowed/x.csv", nil)
	if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("non-allowlisted host: want SOURCE_UNAVAILABLE, got %v", err)
	} else if !strings.Contains(err.Error(), "白名单") {
		t.Fatalf("error should mention allowlist: %v", err)
	}
	// Subdomain of an allowlisted host is NOT auto-allowed (exact match only).
	_, err = f.Get(context.Background(), "https://sub."+host+"/x.csv", nil)
	if _, ok := AsSourceError(err); !ok {
		t.Fatalf("subdomain should be rejected, got %v", err)
	}
	// http (not https) is rejected even for an allowlisted host.
	_, err = f.Get(context.Background(), "http://"+host+"/x.csv", nil)
	if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("plain http should be SOURCE_UNAVAILABLE, got %v", err)
	}
}

func TestFetcherRedirects(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hop":
			w.Header().Set("Location", "/final.csv")
			w.WriteHeader(http.StatusFound)
		case "/cross-host":
			w.Header().Set("Location", "https://elsewhere.test/final.csv")
			w.WriteHeader(http.StatusFound)
		case "/downgrade":
			w.Header().Set("Location", "http://insecure.test/f.csv")
			w.WriteHeader(http.StatusFound)
		default:
			_, _ = w.Write([]byte("ok\n"))
		}
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	f := newTestFetcher(t, []string{host}, FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20, MaxRedirects: 3}, srv.URL)
	ctx := context.Background()

	if doc, err := f.Get(ctx, srv.URL+"/hop", nil); err != nil || string(doc.Payload) != "ok\n" {
		t.Fatalf("same-host redirect should follow: body=%q err=%v", doc.Payload, err)
	}
	if _, err := f.Get(ctx, srv.URL+"/cross-host", nil); err == nil {
		t.Fatal("cross-host redirect should be rejected")
	} else if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("cross-host redirect: want SOURCE_UNAVAILABLE, got %v", err)
	}
	if _, err := f.Get(ctx, srv.URL+"/downgrade", nil); err == nil {
		t.Fatal("http downgrade redirect should be rejected")
	} else if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("http downgrade redirect: want SOURCE_UNAVAILABLE, got %v", err)
	}
}

func TestFetcherBudgets(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")

	// Size budget: cap 100 bytes, server sends 4096 → abort with budget error.
	f := newTestFetcher(t, []string{host}, FetchBudget{Timeout: 5 * time.Second, MaxBytes: 100}, srv.URL)
	_, err := f.Get(context.Background(), srv.URL+"/big.csv", nil)
	if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("oversize body should be SOURCE_UNAVAILABLE, got %v", err)
	}
	if !strings.Contains(err.Error(), "大小预算") {
		t.Fatalf("error should mention size budget: %v", err)
	}

	// Non-200 → SOURCE_UNAVAILABLE with status in message.
	srv2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv2.Close()
	host2 := strings.TrimPrefix(srv2.URL, "https://")
	f2 := newTestFetcher(t, []string{host2}, FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20}, srv2.URL)
	_, err = f2.Get(context.Background(), srv2.URL+"/missing", nil)
	if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("404 should be SOURCE_UNAVAILABLE, got %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error should include status: %v", err)
	}
}

func TestFetcherHTMLMasquerade(t *testing.T) {
	// Content-Type: text/html → rejected even though status is 200.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><body>login page</body></html>"))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	f := newTestFetcher(t, []string{host}, FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20}, srv.URL)
	_, err := f.Get(context.Background(), srv.URL+"/fake.csv", nil)
	if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("html CT should be rejected, got %v", err)
	}

	// No content-type but body starts with <html → sniffed and rejected.
	srv2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body>maintenance</body></html>"))
	}))
	defer srv2.Close()
	host2 := strings.TrimPrefix(srv2.URL, "https://")
	f2 := newTestFetcher(t, []string{host2}, BudgetFor(host2), srv2.URL)
	_, err = f2.Get(context.Background(), srv2.URL+"/x", nil)
	if se, ok := AsSourceError(err); !ok || se.Kind != ErrSourceUnavailable {
		t.Fatalf("sniffed html should be rejected, got %v", err)
	}
}

// BudgetFor is a tiny helper so the sniff case uses the default budget.
func BudgetFor(_ string) FetchBudget { return FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20} }

func TestFetcherSuccessMeta(t *testing.T) {
	payload := "STUB_1,\"424.460\"\n"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, payload)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	f := newTestFetcher(t, []string{host}, BudgetFor(host), srv.URL)

	doc, err := f.Get(context.Background(), srv.URL+"/table1.csv", nil)
	if err != nil {
		t.Fatalf("success case failed: %v", err)
	}
	if string(doc.Payload) != payload {
		t.Fatalf("body mismatch: %q", doc.Payload)
	}
	if doc.RetrievedAt.IsZero() || doc.Bytes != int64(len(payload)) || doc.SHA256 == "" {
		t.Fatalf("doc metadata incomplete: %+v", doc)
	}
}
