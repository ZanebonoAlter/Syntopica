package handler

import (
	"testing"
)

func TestMatchTier(t *testing.T) {
	// downgraded 标记已于 2026-11-12 退役：max_sim 统一 tier 2。
	cases := []struct {
		reason string
		want   int
	}{
		// direct_hit → tier 0 (best)
		{"direct_hit", 0},
		// hit_rate → tier 1
		{"hit_rate", 1},
		// max_sim → tier 2
		{"max_sim", 2},
		// weighted → tier 3 (weakest)
		{"weighted", 3},
		// unknown reason → tier 3 (safe fallback)
		{"unknown", 3},
		{"", 3},
	}
	for _, c := range cases {
		got := MatchTier(c.reason)
		if got != c.want {
			t.Errorf("MatchTier(%q) = %d, want %d", c.reason, got, c.want)
		}
	}
}
