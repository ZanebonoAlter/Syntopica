package service

import (
	"math"
	"testing"
)

// TestArticleBehaviorLevel 验证 D1 权重分级：收藏 > 深读 > 普通打开。
func TestArticleBehaviorLevel(t *testing.T) {
	tests := []struct {
		name        string
		hasFavorite bool
		maxScroll   int
		maxTime     int
		want        float64
	}{
		{"收藏文章取最高档", true, 0, 0, BehaviorWeightFavorite},
		{"收藏优先于深读", true, 90, 200, BehaviorWeightFavorite},
		{"深读-滚动达标", false, 85, 60, BehaviorWeightDeepRead},
		{"深读-时长达标", false, 50, 130, BehaviorWeightDeepRead},
		{"深读-双达标仍 0.6", false, 95, 300, BehaviorWeightDeepRead},
		{"普通打开-均未达标", false, 50, 60, BehaviorWeightOpen},
		{"边界-滚动恰好80算深读", false, 80, 0, BehaviorWeightDeepRead},
		{"边界-时长恰好120算深读", false, 0, 120, BehaviorWeightDeepRead},
		{"边界-滚动79算普通", false, 79, 0, BehaviorWeightOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := articleBehaviorLevel(tt.hasFavorite, tt.maxScroll, tt.maxTime)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("articleBehaviorLevel(%v,%d,%d) = %v, want %v", tt.hasFavorite, tt.maxScroll, tt.maxTime, got, tt.want)
			}
		})
	}
}

// TestTimeDecay 验证 30 天指数衰减 exp(-days/30)。
func TestTimeDecay(t *testing.T) {
	tests := []struct {
		days float64
		want float64
	}{
		{0, 1.0},
		{30, math.Exp(-1)},   // ≈0.368
		{60, math.Exp(-2)},   // ≈0.135
		{15, math.Exp(-0.5)}, // 半衰减
	}
	for _, tt := range tests {
		got := timeDecay(tt.days)
		if math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("timeDecay(%v) = %v, want %v", tt.days, got, tt.want)
		}
	}
	if timeDecay(0) != 1.0 {
		t.Error("当天行为衰减应为 1.0（满权重）")
	}
}

// TestNormalizeVector 验证向量归一化为单位长度。
func TestNormalizeVector(t *testing.T) {
	t.Run("普通向量归一化", func(t *testing.T) {
		v := []float64{3, 4}
		got := normalizeVector(v)
		// |(3,4)|=5 → (0.6, 0.8)
		if math.Abs(got[0]-0.6) > 1e-9 || math.Abs(got[1]-0.8) > 1e-9 {
			t.Errorf("normalize(3,4) = %v, want [0.6 0.8]", got)
		}
		// 归一化后模长应为 1
		var norm float64
		for _, x := range got {
			norm += x * x
		}
		if math.Abs(math.Sqrt(norm)-1.0) > 1e-9 {
			t.Errorf("归一化后模长 %v ≠ 1", math.Sqrt(norm))
		}
	})
	t.Run("零向量保持原样避免除零", func(t *testing.T) {
		got := normalizeVector([]float64{0, 0, 0})
		if got != nil {
			t.Errorf("normalize(零向量) = %v, want nil", got)
		}
	})
}

// TestMergeSeedVectors 验证 D7/A 种子加权合并：normalize(α×incoming + (1−α)×existing)。
func TestMergeSeedVectors(t *testing.T) {
	alpha := 0.4
	t.Run("加权合并累积非覆盖", func(t *testing.T) {
		existing := []float64{1, 0}
		incoming := []float64{0, 1}
		got := mergeSeedVectors(incoming, existing, alpha)
		// 期望方向 = normalize(0.4*(0,1) + 0.6*(1,0)) = normalize(0.6, 0.4)
		want := normalizeVector([]float64{0.6, 0.4})
		if len(got) != len(want) {
			t.Fatalf("维度不符: got %d want %d", len(got), len(want))
		}
		for i := range got {
			if math.Abs(got[i]-want[i]) > 1e-9 {
				t.Errorf("merge[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})
	t.Run("existing 为 nil 时直接归一化 incoming", func(t *testing.T) {
		incoming := []float64{3, 4}
		got := mergeSeedVectors(incoming, nil, alpha)
		if math.Abs(got[0]-0.6) > 1e-9 || math.Abs(got[1]-0.8) > 1e-9 {
			t.Errorf("首次种子 = %v, want [0.6 0.8]", got)
		}
	})
	t.Run("incoming 为 nil 时保持 existing", func(t *testing.T) {
		existing := []float64{1, 0}
		got := mergeSeedVectors(nil, existing, alpha)
		if got == nil || math.Abs(got[0]-1.0) > 1e-9 {
			t.Errorf("incoming 空 = %v, want 保持 existing [1 0]", got)
		}
	})
}

// TestRecommendationHash 验证 D5/C：hash = route_id+board_id，不含 source。
func TestRecommendationHash(t *testing.T) {
	h1 := ComputeRecommendationHash(10, nil)
	h2 := ComputeRecommendationHash(10, nil)
	hOtherRoute := ComputeRecommendationHash(11, nil)
	hWithBoard := ComputeRecommendationHash(10, uintPtr(5))
	if h1 != h2 {
		t.Error("相同 route+board 应产出相同 hash")
	}
	if h1 == hOtherRoute {
		t.Error("不同 route 应产出不同 hash")
	}
	if h1 == hWithBoard {
		t.Error("不同 board 应产出不同 hash")
	}
	// hash 应为 32 位十六进制（沿用 board_upgrade ComputeSuggestionHash 风格）
	if len(h1) != 32 {
		t.Errorf("hash 长度 = %d, want 32", len(h1))
	}
}

// TestParseRouteParameters 验证 D3：path 参数段解析。
func TestParseRouteParameters(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		usable   bool
		requires bool
	}{
		{"零参数可直接订阅", "/36kr/newsflashes", true, false},
		{"必填参数路由", "/bilibili/user/dynamic/:uid", false, true},
		{"全可选参数可直接订阅", "/81rc/:category{.+}?", true, false},
		{"混合可选必填需填参", "/x/:a/:b?", false, true},
		{"纯前缀无参", "/github/trending", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usable, requires := ParseRouteParameters(tt.path)
			if usable != tt.usable || requires != tt.requires {
				t.Errorf("ParseRouteParameters(%q) = (usable=%v,requires=%v), want (%v,%v)", tt.path, usable, requires, tt.usable, tt.requires)
			}
		})
	}
}

// uintPtr 测试辅助。
func uintPtr(v uint) *uint { return &v }
