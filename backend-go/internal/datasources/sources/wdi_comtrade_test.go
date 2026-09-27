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

const wdiSampleResp = `[{"page":1,"pages":1,"per_page":30,"total":3,"sourceid":"2","lastupdated":"1783987200"},
[{"indicator":{"id":"NE.EXP.GNFS.ZS","value":"Exports of goods and services (% of GDP)"},"country":{"id":"CN","value":"China"},"countryiso3code":"CHN","date":"2024","value":20.44},
 {"indicator":{"id":"NE.EXP.GNFS.ZS","value":"Exports of goods and services (% of GDP)"},"country":{"id":"JP","value":"Japan"},"countryiso3code":"JPN","date":"2023","value":null},
 {"indicator":{"id":"NE.EXP.GNFS.ZS","value":"Exports of goods and services (% of GDP)"},"country":{"id":"KR","value":"Korea, Rep."},"countryiso3code":"KOR","date":"2024","value":42.1}]]`

func newWDIOverServer(t *testing.T, status int, body string) (*WDI, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.Contains(r.URL.RawQuery, "format=json") {
			t.Errorf("request must ask for JSON: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20})
	f.SetClient(testClient())
	w := NewWDI(f, datasources.NewTTLCache(time.Minute))
	w.baseURL = srv.URL + "/v2"
	return w, &calls
}

func TestWDIFetchMultiCountry(t *testing.T) {
	w, calls := newWDIOverServer(t, 200, wdiSampleResp)
	res, err := w.Fetch(context.Background(), "NE.EXP.GNFS.ZS", []string{"CHN", "JPN", "KOR"}, 2023, 2024)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if res["lastupdated"].(string) != "1783987200" {
		t.Fatalf("lastupdated passthrough: %v", res["lastupdated"])
	}
	obs := res["observations"].([]WDIObservation)
	if len(obs) != 3 {
		t.Fatalf("want 3 observations, got %d", len(obs))
	}
	if obs[0].Value == nil || *obs[0].Value != 20.44 {
		t.Fatalf("CHN 2024 value: %v", obs[0].Value)
	}
	if obs[1].Value != nil {
		t.Fatalf("null observation must stay null, got %v", *obs[1].Value)
	}
	// Case-normalized countries must dedupe (CHN + chn → one call).
	_, err = w.Fetch(context.Background(), "NE.EXP.GNFS.ZS", []string{"chn", "CHN"}, 2024, 2024)
	if err != nil {
		t.Fatalf("dedupe fetch: %v", err)
	}
	if *calls != 2 { // first fetch + dedupe fetch (different year range → new key)
		t.Fatalf("calls = %d", *calls)
	}
}

func TestWDIInvalidArgumentsNoNetwork(t *testing.T) {
	w, calls := newWDIOverServer(t, 200, wdiSampleResp)
	for _, args := range []struct {
		ind  string
		cs   []string
		f, e int
	}{
		{"ne.exp.gnfs.zs", []string{"CHN"}, 2020, 2024},   // lowercase → reject
		{"NOT_A_CODE", []string{"CHN"}, 2020, 2024},       // no dot separators
		{"NE.EXP.GNFS.ZS", []string{"CN"}, 2020, 2024},    // not ISO3
		{"NE.EXP.GNFS.ZS", []string{"china"}, 2020, 2024}, // not ISO3
		{"NE.EXP.GNFS.ZS", nil, 2020, 2024},               // empty countries
		{"NE.EXP.GNFS.ZS", []string{"CHN"}, 1950, 2024},   // before 1960
		{"NE.EXP.GNFS.ZS", []string{"CHN"}, 2024, 2020},   // from > to
	} {
		_, err := w.Fetch(context.Background(), args.ind, args.cs, args.f, args.e)
		wantInvalidArg(t, err)
	}
	if *calls != 0 {
		t.Fatalf("INVALID_ARGUMENT must not touch the network, calls = %d", *calls)
	}
}

func TestWDIErrorPayloadShape(t *testing.T) {
	// World Bank error responses are single-element arrays with message.
	w, _ := newWDIOverServer(t, 200, `[{"message":{"id":"120","key":"Invalid value"}}]`)
	_, err := w.Fetch(context.Background(), "NE.EXP.GNFS.ZS", []string{"CHN"}, 2024, 2024)
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("error payload: want SOURCE_UNAVAILABLE, got %v", err)
	}
}

const comtradeSampleResp = `{"elapsedTime":"0.1 secs","count":2,"data":[
 {"reporterCode":392,"partnerCode":0,"period":"202505","freqCode":"M","flowCode":"M","cmdCode":"2709","qtyUnitCode":8,"qty":9268800900.0,"netWgt":9268800900.0,"primaryValue":5208641182.0,"isAggregate":true},
 {"reporterCode":392,"partnerCode":682,"period":"202505","freqCode":"M","flowCode":"M","cmdCode":"2709","qtyUnitCode":8,"qty":46433650000.0,"netWgt":46433650000.0,"primaryValue":28747349248.9,"isAggregate":true}],"error":""}`

func newComtradeOverServer(t *testing.T, status int, body string, wantKey string) (*Comtrade, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if wantKey != "" && r.Header.Get("Ocp-Apim-Subscription-Key") != wantKey {
			t.Errorf("subscription key header mismatch")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	f := datasources.NewFetcher([]string{host}, datasources.FetchBudget{Timeout: 5 * time.Second, MaxBytes: 1 << 20})
	f.SetClient(testClient())
	c := NewComtrade(f, datasources.NewTTLCache(time.Minute), func() string { return wantKey })
	c.baseURL = srv.URL + "/data/v1"
	return c, &calls
}

func TestComtradeFetchBilateral(t *testing.T) {
	c, calls := newComtradeOverServer(t, 200, comtradeSampleResp, "test-key")
	saudi := 682
	res, err := c.Fetch(context.Background(), ComtradeParams{
		FreqCode: "M", Reporter: 392, Partner: &saudi, CmdCode: "2709", FlowCode: "M", Period: "202505",
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	obs := res["observations"].([]map[string]any)
	if len(obs) != 2 {
		t.Fatalf("want 2 observations, got %d", len(obs))
	}
	if obs[0]["partner_code"].(int) != 0 || obs[0]["is_aggregate"].(bool) != true {
		t.Fatalf("World aggregate row: %+v", obs[0])
	}
	if obs[1]["net_wgt_kg"].(*float64) == nil || *obs[1]["net_wgt_kg"].(*float64) != 46433650000 {
		t.Fatalf("partner row netWgt kg: %v", obs[1]["net_wgt_kg"])
	}
	// Same query again → cache hit, no extra upstream call.
	if _, err := c.Fetch(context.Background(), ComtradeParams{
		FreqCode: "M", Reporter: 392, Partner: &saudi, CmdCode: "2709", FlowCode: "M", Period: "202505",
	}); err != nil {
		t.Fatalf("cache fetch: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("cache must serve second call, calls = %d", *calls)
	}
}

func TestComtradeKeyMissing(t *testing.T) {
	c, calls := newComtradeOverServer(t, 200, comtradeSampleResp, "")
	_, err := c.Fetch(context.Background(), ComtradeParams{
		FreqCode: "A", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "2024",
	})
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable {
		t.Fatalf("want SOURCE_UNAVAILABLE, got %v", err)
	} else if !strings.Contains(err.Error(), "COMTRADE_API_KEY") {
		t.Fatalf("error must name the config key: %v", err)
	}
	if *calls != 0 {
		t.Fatalf("missing key must not touch the network, calls = %d", *calls)
	}
}

func TestComtradeInvalidArguments(t *testing.T) {
	c, _ := newComtradeOverServer(t, 200, comtradeSampleResp, "k")
	for _, p := range []ComtradeParams{
		{FreqCode: "W", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "2024"},
		{FreqCode: "A", Reporter: 156, CmdCode: "2709", FlowCode: "Z", Period: "2024"},
		{FreqCode: "A", Reporter: 9999, CmdCode: "2709", FlowCode: "M", Period: "2024"},
		{FreqCode: "A", Reporter: 156, CmdCode: "27o9", FlowCode: "M", Period: "2024"},
		{FreqCode: "M", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "2024"},   // M needs YYYYMM
		{FreqCode: "A", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "202405"}, // A needs YYYY
	} {
		_, err := c.Fetch(context.Background(), p)
		wantInvalidArg(t, err)
	}
}

func TestComtradeRateLimitAndNoData(t *testing.T) {
	// 429 → SOURCE_UNAVAILABLE carrying the upstream status (rate limit is
	// distinguishable via StatusCode).
	c, _ := newComtradeOverServer(t, 429, `{"statusCode":429,"message":"Rate limit"}`, "k")
	_, err := c.Fetch(context.Background(), ComtradeParams{
		FreqCode: "A", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "2024",
	})
	if se, ok := datasources.AsSourceError(err); !ok || se.Kind != datasources.ErrSourceUnavailable || se.StatusCode != 429 {
		t.Fatalf("429: want SOURCE_UNAVAILABLE status=429, got %+v", err)
	}

	// count=0 (monthly not yet published) is legitimate no_data, not an error.
	c2, _ := newComtradeOverServer(t, 200, `{"elapsedTime":"0.1 secs","count":0,"data":[],"error":""}`, "k")
	res, err := c2.Fetch(context.Background(), ComtradeParams{
		FreqCode: "M", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "202608",
	})
	if err != nil {
		t.Fatalf("no-data must not error: %v", err)
	}
	if res["no_data"] != true {
		t.Fatal("no_data flag expected")
	}
}

var _ = fmt.Sprintf
var _ = json.Marshal
