package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/tagmanagement/service/related"
)

// 用例来源：openspec/changes/enable-story-cross-day-continuation/test-cases.md
// 故事 S3（CLI 逻辑单测：刷新/试验两种模式明确分离、参数拒绝、抽样规则确定）。
// 真实 provider 调用与效果核对是人工步骤（不在这层伪造数据）。

func TestParseOptionsSeparatesModes(t *testing.T) {
	t.Run("--refresh", func(t *testing.T) {
		opts, err := parseOptions([]string{"--refresh", "--board", "1980", "--generation", " g2 "})
		require.NoError(t, err)
		require.Equal(t, ModeRefresh, opts.Mode)
		require.Equal(t, uint(1980), opts.BoardID)
		require.Equal(t, "g2", opts.Generation)
	})

	t.Run("--experiment 未指定阈值 → 保留哨兵，运行期读 ai_settings（R-5）", func(t *testing.T) {
		opts, err := parseOptions([]string{"--experiment", "--board", "1980"})
		require.NoError(t, err)
		require.Equal(t, ModeExperiment, opts.Mode)
		require.Less(t, opts.Threshold, 0.0, "未指定阈值不在解析期定值，交给 resolveThreshold 读当前 ai_settings")
		require.Equal(t, related.WindowDays, opts.WindowDays)
		require.Equal(t, 50, opts.SampleLimit)
		require.Equal(t, 10, opts.NearLimit)
	})

	t.Run("--experiment 显式负数（非哨兵）拒绝", func(t *testing.T) {
		_, err := parseOptions([]string{"--experiment", "--board", "1980", "--threshold", "-0.5"})
		require.Error(t, err)
	})

	t.Run("--experiment 自定义阈值/窗口/样本", func(t *testing.T) {
		opts, err := parseOptions([]string{"--experiment", "--board", "1980", "--threshold", "0.9", "--window-days", "14", "--samples", "20", "--near", "5", "--out", "/tmp/x.json"})
		require.NoError(t, err)
		require.InDelta(t, 0.9, opts.Threshold, 1e-9)
		require.Equal(t, 14, opts.WindowDays)
		require.Equal(t, 20, opts.SampleLimit)
		require.Equal(t, 5, opts.NearLimit)
		require.Equal(t, "/tmp/x.json", opts.OutPath)
	})

	invalid := []struct {
		name string
		args []string
	}{
		{name: "两种模式互斥", args: []string{"--refresh", "--experiment", "--board", "1980"}},
		{name: "必须选一种模式", args: []string{"--board", "1980"}},
		{name: "缺 board", args: []string{"--refresh"}},
		{name: "阈值范围外", args: []string{"--experiment", "--board", "1980", "--threshold", "1.5"}},
		{name: "窗口非正", args: []string{"--experiment", "--board", "1980", "--window-days", "0"}},
		{name: "样本数非正", args: []string{"--experiment", "--board", "1980", "--samples", "0"}},
		{name: "多余位置参数", args: []string{"--refresh", "--board", "1980", "extra"}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOptions(tc.args)
			require.Error(t, err)
		})
	}
}

// R-5：缺省阈值必须走当前生效值（ai_settings），显式值优先且不再读库。
func TestResolveThresholdUsesCurrentSettingOnlyWhenUnset(t *testing.T) {
	called := 0
	loader := func() float64 { called++; return 0.86 }

	unset := &Options{Threshold: -1}
	resolveThreshold(unset, loader)
	require.InDelta(t, 0.86, unset.Threshold, 1e-9)
	require.Equal(t, 1, called)

	explicit := &Options{Threshold: 0.9}
	resolveThreshold(explicit, loader)
	require.InDelta(t, 0.9, explicit.Threshold, 1e-9)
	require.Equal(t, 1, called, "显式 --threshold 不得再读 ai_settings")
}

func TestSelectExperimentPairsIsDeterministic(t *testing.T) {
	pairs := []related.ExperimentPair{
		{AID: 1, BID: 5, Score: 0.91},
		{AID: 1, BID: 3, Score: 0.91},
		{AID: 2, BID: 4, Score: 0.86},
		{AID: 2, BID: 9, Score: 0.82},
		{AID: 3, BID: 6, Score: 0.815},
		{AID: 4, BID: 7, Score: 0.79},
		{AID: 5, BID: 8, Score: 0.7},
	}

	direct, near := related.SelectExperimentPairs(pairs, 0.82, 2, 2)

	require.Len(t, direct, 2, "达标对只取样本上限")
	// 分层抽样（每 k 个取一个）覆盖整个达标区间，而不是只取最高分的「漂亮例子」：
	// 4 条达标对取 2 条 → 第 0 与第 2 条（先按 score desc、再按 a_id/b_id 升序）。
	require.Equal(t, uint(1), direct[0].AID)
	require.Equal(t, uint(3), direct[0].BID, "同分按 a_id/b_id 升序，抽样确定")
	require.Equal(t, uint(2), direct[1].AID)
	require.Equal(t, uint(4), direct[1].BID)

	require.Len(t, near, 2, "阈值下样本单独取最接近阈值的两条")
	require.InDelta(t, 0.815, near[0].Score, 1e-9)
	require.InDelta(t, 0.79, near[1].Score, 1e-9)

	// 恰好等于阈值算达标（边界包含）。
	directAtBoundary, _ := related.SelectExperimentPairs([]related.ExperimentPair{{AID: 1, BID: 2, Score: 0.82}}, 0.82, 10, 10)
	require.Len(t, directAtBoundary, 1)

	// 空样本可解释：两条空列表，不报错、不伪造。
	emptyDirect, emptyNear := related.SelectExperimentPairs(nil, 0.82, 10, 10)
	require.Empty(t, emptyDirect)
	require.Empty(t, emptyNear)
}
