package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── NormalizeTerm / NormalizeTerms 纯函数单测（-short 可跑，无 DB）────────
// 对齐 test-cases.md WB-5/6；DB 侧行为（hit_count 累计）见
// margin_notes_repository_test.go 集成测试。

func TestNormalizeTerm_Unit(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  逆回购  ", "逆回购"},   // 首尾半角空白
		{"　逆回购　", "逆回购"},     // 首尾全角空格（U+3000）
		{"ＦＥＤ", "fed"},       // 全角字母 → 半角 + 小写
		{" FED ", "fed"},     // 大小写折叠
		{"Fed", "fed"},       // 混合大小写
		{"　央行逆回购　", "央行逆回购"}, // 全角空格夹中文
		{"流动性对冲", "流动性对冲"},   // 原样
		{"流动性对冲！", "流动性对冲!"}, // 全角标点折叠为半角标点（P1 不剥标点）
		{"", ""},               // 空串
		{"　", ""},              // 纯全角空白
		{"\t\n逆回购\r\n", "逆回购"}, // 控制空白
		{"利率招标　7 天期", "利率招标 7 天期"}, // 混合全半角空格
	}
	for _, c := range cases {
		assert.Equal(t, c.want, NormalizeTerm(c.in), "input %q", c.in)
	}
}

func TestNormalizeTerms_Unit(t *testing.T) {
	assert.Equal(t, []string{"逆回购", "fed"}, NormalizeTerms([]string{"逆回购", " FED ", "ＦＥＤ", ""}),
		"归一化 + 空串过滤 + 保序去重")
	assert.Empty(t, NormalizeTerms([]string{"", "　", "   "}), "WB-6 全空 → 空列表")
	assert.Empty(t, NormalizeTerms(nil))
}
