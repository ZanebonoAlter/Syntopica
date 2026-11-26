package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 用例来源：board-health-triage-advice tasks 1.1 —— metrics jsonb 扩展
// （labels[].magnet/neighbors/desc_flags/description + 顶层 triage +
// board.magnet_flow）对历史快照（缺全部新字段）向后兼容：反序列化零值、
// 再序列化不报错；降级轮 triage 序列化为 null（design D4：Go 零值 nil →
// JSON null → 消费方判缺省）。
func TestBoardHealthMetricsJSONCompat(t *testing.T) {
	// 2026-10 之前的历史快照形态：只有 board + labels 分布字段。
	historical := `{"board":{"matched_tags":3,"reason_dist":{"direct_hit":3},"ghost_matches":0,"vector_missing":0},"labels":[{"aux_id":7,"label":"旧标签","samples":3,"cosine_mean":null,"cosine_median":null,"cosine_p25":null,"cosine_p75":null,"low_sim_ratio":null,"insufficient":true}]}`

	var metrics tagmodels.BoardHealthMetrics
	require.NoError(t, json.Unmarshal([]byte(historical), &metrics))
	require.Equal(t, 3, metrics.Board.MatchedTags)
	require.Nil(t, metrics.Board.MagnetFlow)
	require.Len(t, metrics.Labels, 1)
	require.Nil(t, metrics.Labels[0].Magnet)
	require.Nil(t, metrics.Labels[0].Neighbors)
	require.Nil(t, metrics.Labels[0].DescFlags)
	require.Empty(t, metrics.Labels[0].Description)
	require.Nil(t, metrics.Triage)

	// 历史形态再序列化：triage/magnet 显式 null（旧值不丢、不报错）。
	remarshaled, err := json.Marshal(&metrics)
	require.NoError(t, err)
	require.Contains(t, string(remarshaled), `"triage":null`)
	require.Contains(t, string(remarshaled), `"magnet_flow":null`)
	require.Contains(t, string(remarshaled), `"magnet":null`)
	require.Contains(t, string(remarshaled), `"neighbors":null`)
	require.Contains(t, string(remarshaled), `"cosine_mean":null`)

	// 新形态往返：triage/信号字段完整保留。
	allMean, failedMean := 0.68, 0.52
	topBoard := "对手板"
	roundtrip := tagmodels.BoardHealthMetrics{
		Board: tagmodels.BoardHealthBoardMetrics{
			MatchedTags: 11, ReasonDist: map[string]int{"direct_hit": 6}, GhostMatches: 5,
			MagnetFlow: &tagmodels.BoardHealthMagnetFlow{FailedTags: 5, FlowTags: 5, TopBoard: &topBoard, TopTags: 5},
		},
		Labels: []tagmodels.BoardHealthLabelMetrics{{
			AuxID: 42, Label: "上海", Samples: 400,
			Description: "上海知名商业综合体",
			Magnet:      &tagmodels.BoardHealthMagnetSignal{AllSamples: 400, FailedSamples: 40, Suspicious: true, AllMean: &allMean, FailedMean: &failedMean},
			Neighbors:   []tagmodels.BoardHealthNeighbor{{AuxID: 9, Label: "举办地点", Cosine: 0.796}},
			DescFlags:   []string{"此次"},
		}},
		Triage: []tagmodels.BoardHealthTriageItem{{
			AuxID: 42, Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "中国东部直辖市",
		}},
	}
	encoded, err := json.Marshal(&roundtrip)
	require.NoError(t, err)
	var decoded tagmodels.BoardHealthMetrics
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, roundtrip, decoded)

	// keep 类条目不带 suggestion 时 omitempty（契约：仅 rewrite 必填）。
	keepOnly := tagmodels.BoardHealthTriageItem{AuxID: 43, Category: "keep", Evidence: "cosine_mean=0.7"}
	itemJSON, err := json.Marshal(keepOnly)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(itemJSON), "suggestion"), "omitempty：非 rewrite 条目不携带空 suggestion 键")
}
