package service_test

import (
	"testing"

	"syntopica-backend/internal/dataenrichment/service"
)

// ── evidence 两级分类（tasks 3.5 / M3）──────────────────────────────────────

func ec(st, ref, kind string) service.EvidenceChainItem {
	return service.EvidenceChainItem{SourceType: st, Ref: ref, Kind: kind}
}

func find(chain []service.EvidenceChainItem, st, ref string) *service.EvidenceChainItem {
	for i := range chain {
		if chain[i].SourceType == st && chain[i].Ref == ref {
			return &chain[i]
		}
	}
	return nil
}

// M3.1 旧组合零回归 + M3.2 kind 透传。
func TestEvidenceChain_LegacyAndKind(t *testing.T) {
	lanes := map[uint]bool{1: true, 2: true}
	chain := []service.EvidenceChainItem{
		ec("news", "n1", ""),     // M3.1 legacy
		ec("web", "u1", "quote"), // M3.2
		ec("page", "u2", "series"),
		ec("web", "u3", "chart"),
	}
	out := service.SanitizeEvidenceChainForTest(chain, lanes)
	if len(out) != 4 {
		t.Fatalf("want 4, got %d", len(out))
	}
	for _, c := range []struct{ st, ref, kind string }{
		{"news", "n1", ""}, {"web", "u1", "quote"}, {"page", "u2", "series"}, {"web", "u3", "chart"},
	} {
		got := find(out, c.st, c.ref)
		if got == nil {
			t.Fatalf("%s/%s dropped", c.st, c.ref)
		}
		if got.Kind != c.kind {
			t.Fatalf("%s/%s kind: want %q got %q", c.st, c.ref, c.kind, got.Kind)
		}
	}
}

// M3.3/M3.4 lane 证据合法组合。
func TestEvidenceChain_LaneValid(t *testing.T) {
	lanes := map[uint]bool{7: true}
	chain := []service.EvidenceChainItem{
		ec("lane", "7", ""),      // M3.3 lane 无 kind
		ec("lane", "7", "quote"), // M3.4 lane + kind
	}
	out := service.SanitizeEvidenceChainForTest(chain, lanes)
	if len(out) != 2 {
		t.Fatalf("want 2 lane entries, got %d", len(out))
	}
}

// M3.5 幽灵 lane_id：只拒绝该条，不牵连其余。
func TestEvidenceChain_GhostLane(t *testing.T) {
	lanes := map[uint]bool{1: true}
	chain := []service.EvidenceChainItem{
		ec("lane", "999", ""), // foreign lane
		ec("news", "n1", ""),  // must survive
		ec("lane", "1", ""),   // must survive
	}
	out := service.SanitizeEvidenceChainForTest(chain, lanes)
	if len(out) != 2 {
		t.Fatalf("ghost must drop alone: want 2, got %d", len(out))
	}
	if find(out, "news", "n1") == nil || find(out, "lane", "1") == nil {
		t.Fatal("neighbouring evidence must survive ghost drop")
	}
}

// M3.6 lane 缺 lane_id（ref 非数字/空）→ 拒绝。
func TestEvidenceChain_LaneMissingID(t *testing.T) {
	lanes := map[uint]bool{1: true}
	chain := []service.EvidenceChainItem{
		{SourceType: "lane", Kind: ""}, // no ref at all
		ec("lane", "abc", ""),          // non-numeric ref
		ec("news", "n1", ""),
	}
	out := service.SanitizeEvidenceChainForTest(chain, lanes)
	if len(out) != 1 || find(out, "news", "n1") == nil {
		t.Fatalf("lane entries without parseable lane_id must drop: %+v", out)
	}
}

// M3.7 非法 kind → 按空处理，证据保留。
func TestEvidenceChain_IllegalKindDegrades(t *testing.T) {
	lanes := map[uint]bool{1: true}
	chain := []service.EvidenceChainItem{
		ec("web", "u1", "image"),
		ec("news", "n1", "video"),
	}
	out := service.SanitizeEvidenceChainForTest(chain, lanes)
	if len(out) != 2 {
		t.Fatalf("illegal kind must not drop evidence: %d", len(out))
	}
	for _, e := range out {
		if e.Kind != "" {
			t.Fatalf("illegal kind must normalize to empty, got %q", e.Kind)
		}
	}
}

// M3.8 source_type 非法 → 整条拒绝（旧行为）。
func TestEvidenceChain_IllegalSourceType(t *testing.T) {
	chain := []service.EvidenceChainItem{
		ec("oracle", "x", ""),
		ec("news", "n1", ""),
	}
	out := service.SanitizeEvidenceChainForTest(chain, map[uint]bool{})
	if len(out) != 1 || find(out, "news", "n1") == nil {
		t.Fatalf("illegal source_type must drop whole entry: %+v", out)
	}
}

// 单泳道 scope：activeLanes=nil 时所有 lane 证据拒绝。
func TestEvidenceChain_LaneRejectedInTopicScope(t *testing.T) {
	chain := []service.EvidenceChainItem{ec("lane", "1", ""), ec("news", "n1", "")}
	out := service.SanitizeEvidenceChainForTest(chain, nil)
	if len(out) != 1 || out[0].SourceType != "news" {
		t.Fatalf("lane evidence must be board-scope only: %+v", out)
	}
}
