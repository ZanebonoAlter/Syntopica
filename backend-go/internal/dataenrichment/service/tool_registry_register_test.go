package service

import (
	"context"
	"strings"
	"testing"
)

// TestRegisterDoesNotExtendExistingToolSurfaces (change
// integrate-research-data-sources spec「注入后现有工具面不变」): registering
// externally-provided research tools into the shared Registry must NOT change
// the tool surface of any existing agent flow — allowedTools lists (e.g.
// explorationToolNames) are flow-owned constants and the advertised tool
// description built from them excludes uninvited tools.
func TestRegisterDoesNotExtendExistingToolSurfaces(t *testing.T) {
	r := NewRegistry(NewDefaultHTTPFetcher())

	// Simulate the research tools being wired in (names only; behavior is
	// irrelevant to the surface invariant).
	for _, name := range []string{"eia_wpsr_table1", "jodi_oil_primary", "wb_wdi", "un_comtrade_trade"} {
		n := name
		r.Register(&Tool{
			Name:        n,
			Description: "research data source tool (test)",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Execute: func(ctx context.Context, args map[string]any) (string, error) {
				return "{}", nil
			},
		})
	}

	// Every existing flow's advertised description keeps excluding the
	// research tools.
	desc := buildToolsDesc(r, explorationToolNames)
	for _, name := range []string{"eia_wpsr_table1", "jodi_oil_primary", "wb_wdi", "un_comtrade_trade"} {
		if strings.Contains(desc, name) {
			t.Fatalf("exploration tool surface must not include %s after Register", name)
		}
	}
	// And the tools remain executable by name for future research flows.
	if _, ok := r.Tools()["eia_wpsr_table1"]; !ok {
		t.Fatal("registered research tool must be callable via registry")
	}
}
