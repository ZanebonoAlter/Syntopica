package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 辅助标签约束（topic-lane-auxiliary-label-constraint）─────────────────────────
//
// 本文件承载 tasks.md 第 12 组映射表里落在 service 的 Scenario：
//   短词计算锤向量并记录模型标识 / 不要求用户补充描述 / 计算失败不落库 /
//   双侧 required 被不同标签分腿覆盖 / 单侧 required 未覆盖判违反 /
//   纯单侧内容不因相对偏好误判为双侧 / preferred 未覆盖不拦截 /
//   跨向量空间的标签被跳过 / 无标签 section 跳过约束检查 /
//   单个标签不得独自满足多条 required 约束 / 单约束话题按绝对上限判定
// 外加 tasks 3.1（margin 边界不含）与 3.4（两条退化路径）。
//
// 判定部分全部是纯函数（不碰 DB / 网络 / 时钟）；只有「计算失败不落库」与
// 「不要求用户补充描述」两条落在写入工作流，用 testcontainer PG 验真实落库。

// --- fixtures ---------------------------------------------------------------

// 单位向量构造：距离 = 1 - cos，所以二维单位向量能让期望距离精确可控。
func unitVec(deg float64) []float64 {
	r := deg * math.Pi / 180
	return []float64{math.Cos(r), math.Sin(r)}
}

func laneConstraintFixture(term, mode string, vec []float64, margin, maxDistance *float64) repository.TopicLaneConstraint {
	return repository.TopicLaneConstraint{
		Term:           term,
		Mode:           mode,
		TermEmbedding:  repository.FloatsToPgVector(vec),
		EmbeddingModel: "qwen3-embedding",
		Margin:         margin,
		MaxDistance:    maxDistance,
	}
}

func floatPtr(v float64) *float64 { return &v }

func labelFixture(tagID uint, model string, vec []float64) repository.LaneLabelVector {
	return repository.LaneLabelVector{TagID: tagID, Model: model, Vector: vec}
}

// constraintCfg is the default knob set (margin 0.05 / max_distance 0.55) —
// the values the migration seeds.
func constraintCfg() repository.PersistentTopicConfig {
	return repository.DefaultPersistentTopicConfig()
}

// 两个正交 required 锤：以色列 = (1,0)，黎巴嫩 = (0,1)。两个锤之间的余弦距离是
// 1（真实库实测 0.27~0.31，但「一个标签的锤距不可能同时对两个锤都占 margin 以上
// 的优势」是结构性质，与两锤间距无关——所以正交构造能让期望值精确可验）。
func bilateralConstraints() []repository.TopicLaneConstraint {
	return []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, nil),
		laneConstraintFixture("黎巴嫩", repository.LaneConstraintModeRequired, []float64{0, 1}, nil, nil),
	}
}

// --- 3.1 相对偏好判定（表驱动） ---------------------------------------------

// TestConstraint_RelativePreference covers tasks 3.1：约束 i 被覆盖 ⟺ 存在 tag t
// 使 d(t,A_i)+margin_i < d(t,A_j)（∀ 其它 required j）；恰好等于 margin 的边界
// 不含（严格不等式）。
func TestConstraint_RelativePreference(t *testing.T) {
	onlyIsrael := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, nil),
	}
	cfg := constraintCfg()

	tests := []struct {
		name   string
		tagDeg float64
		want   LaneConstraintVerdict
	}{
		// 单锤退化（无其它锤可比）→ 只看 max_distance（默认 0.55）。
		{"well inside absolute ceiling", 18, LaneConstraintSatisfied},
		// 1 - cos(58°) ≈ 0.470 < 0.55 → 仍覆盖（退化判定只看绝对上限）。
		{"still inside absolute ceiling", 45, LaneConstraintSatisfied},
		// 1 - cos(0.001°)≈0 → 无条件覆盖（距离塌到 0）。
		{"degenerate near zero distance", 0.001, LaneConstraintSatisfied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cov := EvaluateLaneConstraintCoverage(onlyIsrael, []repository.LaneLabelVector{
				labelFixture(1, "qwen3-embedding", unitVec(tc.tagDeg)),
			}, cfg)
			assert.Equal(t, tc.want, cov.Verdict)
		})
	}

	// margin 边界：absolute-value 判据在双锤下才有 margin 参与。
	bilateral := bilateralConstraints()
	t.Run("margin boundary is exclusive (diff below margin is not covered)", func(t *testing.T) {
		// φ=43.5° → d(以色列)=0.274626、d(黎巴嫩)=0.311645，差 0.037020 < 0.05
		// → 以色列 未被覆盖 → 违反。
		cov := EvaluateLaneConstraintCoverage(bilateral, []repository.LaneLabelVector{
			labelFixture(1, "qwen3-embedding", unitVec(43.5)),
		}, cfg)
		assert.Equal(t, LaneConstraintViolated, cov.Verdict)
		// 锤距几乎相等 → 两条腿都拿不到 margin 优势，都不算覆盖。
		assert.ElementsMatch(t, []string{"以色列", "黎巴嫩"}, cov.Uncovered)
	})
	t.Run("diff above margin is covered", func(t *testing.T) {
		// φ=42.5° → 差 0.061687 > 0.05 → 以色列 被覆盖（黎巴嫩仍未被覆盖）。
		cov := EvaluateLaneConstraintCoverage(bilateral, []repository.LaneLabelVector{
			labelFixture(1, "qwen3-embedding", unitVec(42.5)),
		}, cfg)
		assert.Equal(t, LaneConstraintViolated, cov.Verdict)
		assert.Equal(t, []string{"以色列"}, cov.Covered)
		assert.Equal(t, []string{"黎巴嫩"}, cov.Uncovered)
	})
}

// --- 3.2 分腿覆盖 -----------------------------------------------------------

// TestConstraint_SplitLegCoverage covers Scenario「双侧 required 被不同标签分腿
// 覆盖」：以色列侧的标签满足以色列锤、黎巴嫩侧的标签满足黎巴嫩锤 → 满足。
func TestConstraint_SplitLegCoverage(t *testing.T) {
	cov := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(18)), // d以色列=0.049, d黎巴嫩=0.691
		labelFixture(2, "qwen3-embedding", unitVec(72)), // d黎巴嫩=0.049, d以色列=0.691
	}, constraintCfg())
	assert.Equal(t, LaneConstraintSatisfied, cov.Verdict)
	assert.ElementsMatch(t, []string{"以色列", "黎巴嫩"}, cov.Covered)
	assert.Empty(t, cov.Uncovered)
	assert.False(t, cov.Blocks())
}

// TestConstraint_SingleSidedViolates covers Scenario「单侧 required 未覆盖判违反」
// and 「纯单侧内容不因相对偏好误判为双侧」: every tag is pure one-side content, so
// the other leg stays uncovered.
func TestConstraint_SingleSidedViolates(t *testing.T) {
	cfg := constraintCfg()

	t.Run("only the israel leg present", func(t *testing.T) {
		cov := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
			labelFixture(1, "qwen3-embedding", unitVec(18)),
		}, cfg)
		assert.Equal(t, LaneConstraintViolated, cov.Verdict)
		assert.Equal(t, []string{"黎巴嫩"}, cov.Uncovered)
	})
	t.Run("a whole set of pure israel tags", func(t *testing.T) {
		cov := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
			labelFixture(1, "qwen3-embedding", unitVec(10)),
			labelFixture(2, "qwen3-embedding", unitVec(20)),
			labelFixture(3, "qwen3-embedding", unitVec(30)),
		}, cfg)
		assert.Equal(t, LaneConstraintViolated, cov.Verdict)
		assert.Equal(t, []string{"黎巴嫩"}, cov.Uncovered)
	})
}

// TestConstraint_SingleTagCannotSatisfyBothRequired covers Scenario
// 「单个标签不得独自满足多条 required 约束」——几何上分腿覆盖必须由不同标签实现。
func TestConstraint_SingleTagCannotSatisfyBothRequired(t *testing.T) {
	cov := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(18)),
	}, constraintCfg())
	assert.Equal(t, LaneConstraintViolated, cov.Verdict)
	assert.NotContains(t, cov.Covered, "黎巴嫩")
}

// --- 3.3 跨向量空间跳过 -----------------------------------------------------

// TestConstraint_CrossSpaceLabelsSkipped covers Scenario「跨向量空间的标签被跳过」：
// 模型标识与锤不一致的 tag 既不判满足也不判违反。
func TestConstraint_CrossSpaceLabelsSkipped(t *testing.T) {
	cfg := constraintCfg()

	// 全部标签在另一个空间：无法判定，但 SHALL NOT 判违反。
	cov := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
		labelFixture(1, "local-gguf-q4", unitVec(18)),
		labelFixture(2, "local-gguf-q4", unitVec(72)),
	}, cfg)
	assert.Equal(t, LaneConstraintUndecided, cov.Verdict)
	assert.False(t, cov.Blocks(), "cross-space tags SHALL NOT 判违反")

	// 跨空间标签不改变「同空间标签」的判定结果。
	withForeign := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(18)),
		labelFixture(2, "local-gguf-q4", unitVec(72)),
	}, cfg)
	withoutForeign := EvaluateLaneConstraintCoverage(bilateralConstraints(), []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(18)),
	}, cfg)
	assert.Equal(t, withoutForeign.Verdict, withForeign.Verdict)
	assert.Equal(t, withoutForeign.Uncovered, withForeign.Uncovered)
}

// --- 3.4 单约束退化 --------------------------------------------------------

// TestConstraint_SingleConstraintDegradesToAbsoluteCeiling covers Scenario
// 「单约束话题按绝对上限判定」.
func TestConstraint_SingleConstraintDegradesToAbsoluteCeiling(t *testing.T) {
	cfg := constraintCfg()
	constraints := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, floatPtr(0.45)),
	}
	// 距离 0.30 < 0.45 → 覆盖。
	cov := EvaluateLaneConstraintCoverage(constraints, []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", []float64{0.7, 0.714142843}),
	}, cfg)
	assert.Equal(t, LaneConstraintSatisfied, cov.Verdict)

	// 距离 0.60 > 0.45 → 违反。
	cov = EvaluateLaneConstraintCoverage(constraints, []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", []float64{0.4, 0.916515139}),
	}, cfg)
	assert.Equal(t, LaneConstraintViolated, cov.Verdict)
}

// TestConstraint_SingleConstraintWithoutCeilingIsUndecidable covers the second
// degenerate path of tasks 3.4：单约束 + 无 max_distance（行内为空且全局默认
// 缺失）→ 视为无法判定，SHALL NOT 参与拦截。
func TestConstraint_SingleConstraintWithoutCeilingIsUndecidable(t *testing.T) {
	cfg := constraintCfg()
	cfg.ConstraintMaxDistanceDefault = 0 // 模拟「绝对上限也为空」
	constraints := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, nil),
	}
	cov := EvaluateLaneConstraintCoverage(constraints, []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(60)),
	}, cfg)
	assert.Equal(t, LaneConstraintUndecided, cov.Verdict)
	assert.False(t, cov.Blocks())
}

// --- 3.5 preferred 不参与拦截 ----------------------------------------------

// TestConstraint_PreferredNeverBlocks covers Scenario「preferred 未覆盖不拦截」.
func TestConstraint_PreferredNeverBlocks(t *testing.T) {
	constraints := []repository.TopicLaneConstraint{
		laneConstraintFixture("真主党", repository.LaneConstraintModePreferred, []float64{0, 1}, nil, nil),
	}
	cov := EvaluateLaneConstraintCoverage(constraints, []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(18)), // 离真主党锤很远
	}, constraintCfg())
	assert.Equal(t, LaneConstraintSatisfied, cov.Verdict)
	assert.Equal(t, 0, cov.Required, "preferred 不计入 required 数")
}

// TestConstraint_PreferredDoesNotJoinTheReferenceSet pins the reading of
// 「preferred SHALL NOT 参与拦截判定」: a preferred anchor must not become a
// competitor that tightens a required leg's relative preference.
func TestConstraint_PreferredDoesNotJoinTheReferenceSet(t *testing.T) {
	requiredOnly := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, nil),
	}
	withPreferred := append(requiredOnly,
		laneConstraintFixture("黎巴嫩", repository.LaneConstraintModePreferred, []float64{0, 1}, nil, nil))

	labels := []repository.LaneLabelVector{labelFixture(1, "qwen3-embedding", unitVec(18))}
	a := EvaluateLaneConstraintCoverage(requiredOnly, labels, constraintCfg())
	b := EvaluateLaneConstraintCoverage(withPreferred, labels, constraintCfg())
	assert.Equal(t, a.Verdict, b.Verdict, "adding a preferred constraint SHALL NOT change a required leg's verdict")
}

// --- 3.6 无标签 section ---------------------------------------------------

// TestConstraint_NoLabelsSkipsCheck covers Scenario「无标签 section 跳过约束检查」.
func TestConstraint_NoLabelsSkipsCheck(t *testing.T) {
	cov := EvaluateLaneConstraintCoverage(bilateralConstraints(), nil, constraintCfg())
	assert.Equal(t, LaneConstraintSatisfied, cov.Verdict)
	assert.False(t, cov.Blocks())
}

// --- 3.7 纯函数（无约束 → 满足，零开销） -----------------------------------

func TestConstraint_NoConstraintsIsSatisfied(t *testing.T) {
	cov := EvaluateLaneConstraintCoverage(nil, []repository.LaneLabelVector{
		labelFixture(1, "qwen3-embedding", unitVec(18)),
	}, constraintCfg())
	assert.Equal(t, LaneConstraintSatisfied, cov.Verdict)
}

// --- 2.1 锤文本编码 -------------------------------------------------------

func TestConstraint_AnchorTextMatchesTagEmbeddingTemplate(t *testing.T) {
	assert.Equal(t, "以色列 event", laneConstraintAnchorText("以色列"))
	assert.Equal(t, "黎巴嫩 event", laneConstraintAnchorText(" 黎巴嫩 "))
}

// --- 2.2 锤向量计算 -------------------------------------------------------

func stubEmbed(t *testing.T, wantText string, vec []float64, model string, calls *int) func() {
	t.Helper()
	prev := laneConstraintEmbedFn
	laneConstraintEmbedFn = func(_ context.Context, req airouter.EmbeddingRequest, capability airouter.Capability) (*airouter.EmbeddingResult, error) {
		*calls++
		assert.Equal(t, airouter.CapabilityEmbedding, capability)
		require.Len(t, req.Input, 1)
		assert.Equal(t, wantText, req.Input[0], "锚文本 SHALL 等于 term + \" event\"")
		assert.NotEmpty(t, req.Operation, "ai_call_logs 需要 operation")
		return &airouter.EmbeddingResult{
			Embeddings: [][]float64{vec},
			Model:      model,
			Dimensions: len(vec),
		}, nil
	}
	return func() { laneConstraintEmbedFn = prev }
}

// TestConstraint_ComputeAnchorPassesTextAndModel covers Scenario
// 「短词计算锤向量并记录模型标识」.
func TestConstraint_ComputeAnchorPassesTextAndModel(t *testing.T) {
	calls := 0
	restore := stubEmbed(t, "黎巴嫩 event", []float64{0.1, 0.2, 0.3}, "qwen3-embedding", &calls)
	defer restore()

	vec, model, err := ComputeLaneConstraintAnchor(context.Background(), "黎巴嫩")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, vec)
	assert.Equal(t, "qwen3-embedding", model, "provider 返回的模型名 SHALL 被透传为 embedding_model")
}

// --- 2.3 失败路径 ---------------------------------------------------------

// TestConstraint_AnchorFailuresReturnError covers Scenario「计算失败不落库」的
// 纯函数半边：embedding 报错 / 返回不可解析（空） / 模型名缺失 一律返回错误，
// SHALL NOT 返回零向量。
// 同时锁定「失败带可读原因」：每个分支必须是 *LaneConstraintAnchorError，
// Reason 指向具体现象（写端点把它当 503 文案展示给用户）。
func TestConstraint_AnchorFailuresReturnError(t *testing.T) {
	cases := []struct {
		name   string
		stub   func(context.Context, airouter.EmbeddingRequest, airouter.Capability) (*airouter.EmbeddingResult, error)
		reason string
	}{
		{
			name: "embedding error",
			stub: func(context.Context, airouter.EmbeddingRequest, airouter.Capability) (*airouter.EmbeddingResult, error) {
				return nil, errors.New("network_error")
			},
			reason: laneConstraintReasonEmbedFailed,
		},
		{
			name: "empty embedding",
			stub: func(context.Context, airouter.EmbeddingRequest, airouter.Capability) (*airouter.EmbeddingResult, error) {
				return &airouter.EmbeddingResult{Embeddings: [][]float64{{}}, Model: "qwen3-embedding"}, nil
			},
			reason: laneConstraintReasonEmptyVector,
		},
		{
			name: "missing model name",
			stub: func(context.Context, airouter.EmbeddingRequest, airouter.Capability) (*airouter.EmbeddingResult, error) {
				return &airouter.EmbeddingResult{Embeddings: [][]float64{{0.1, 0.2}}, Model: "  "}, nil
			},
			reason: laneConstraintReasonNoModel,
		},
		{
			name: "non-finite component",
			stub: func(context.Context, airouter.EmbeddingRequest, airouter.Capability) (*airouter.EmbeddingResult, error) {
				return &airouter.EmbeddingResult{Embeddings: [][]float64{{math.NaN()}}, Model: "qwen3-embedding"}, nil
			},
			reason: laneConstraintReasonNonFinite,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := laneConstraintEmbedFn
			laneConstraintEmbedFn = tc.stub
			defer func() { laneConstraintEmbedFn = prev }()

			vec, model, err := ComputeLaneConstraintAnchor(context.Background(), "黎巴嫩")
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrLaneConstraintAnchor)
			assert.Empty(t, vec, "SHALL NOT 返回零向量")
			assert.Empty(t, model)

			var anchorErr *LaneConstraintAnchorError
			require.ErrorAs(t, err, &anchorErr, "失败必须是结构化错误，携可读原因")
			assert.Equal(t, "黎巴嫩", anchorErr.Term)
			assert.Equal(t, tc.reason, anchorErr.Reason)
			msg := anchorErr.UserMessage()
			assert.Contains(t, msg, "黎巴嫩")
			assert.Contains(t, msg, tc.reason)
			assert.NotContains(t, msg, "http://", "客户端文案 SHALL NOT 带内部地址")
		})
	}
}

// --- 1.3/2.3 写入工作流（PG） --------------------------------------------

func setupLaneConstraintWriteTest(t *testing.T) (*repository.TopicGraphRepository, uint) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repo := repository.NewTopicGraphRepository(db)
	previous := repository.Repo
	repository.Repo = repo
	t.Cleanup(func() { repository.Repo = previous })

	board := models.SemanticLabel{Label: "constraint-board", Slug: "constraint-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	now := repository.NormalizeReportDate(time.Now())
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID,
		Label:           "以黎冲突升级：停火破裂与军事打击",
		Embedding:       repository.FloatsToPgVector([]float64{1, 0}),
		Status:          repository.TopicStatusActive,
		Source:          repository.TopicSourceAuto,
		FirstSeenDate:   now,
		LastSeenDate:    now,
		HitCount:        1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return repo, topic.ID
}

// TestConstraint_AnchorFailurePersistsNothing covers Scenario「计算失败不落库」的
// 落库半边：锤计算失败时接口返回错误且表内零新增行。
func TestConstraint_AnchorFailurePersistsNothing(t *testing.T) {
	repo, topicID := setupLaneConstraintWriteTest(t)

	prev := laneConstraintEmbedFn
	laneConstraintEmbedFn = func(context.Context, airouter.EmbeddingRequest, airouter.Capability) (*airouter.EmbeddingResult, error) {
		return nil, errors.New("network_error")
	}
	defer func() { laneConstraintEmbedFn = prev }()

	_, err := ReplaceTopicLaneConstraints(context.Background(), topicID, []LaneConstraintRequest{{Term: "以色列"}})
	require.Error(t, err)

	rows, err := repo.ListTopicLaneConstraints(topicID)
	require.NoError(t, err)
	assert.Empty(t, rows, "计算失败 SHALL NOT 落库")
}

// TestConstraint_PayloadOnlyNeedsTerm covers Scenario「不要求用户补充描述」:
// the payload carries nothing but the short term and the constraint is accepted
// and stored (with the computed vector + model).
func TestConstraint_PayloadOnlyNeedsTerm(t *testing.T) {
	_, topicID := setupLaneConstraintWriteTest(t)

	calls := 0
	restore := stubEmbed(t, "黎巴嫩 event", []float64{0.3, 0.4}, "qwen3-embedding", &calls)
	defer restore()

	var reqs struct {
		Constraints []LaneConstraintRequest `json:"constraints"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"constraints":[{"term":"黎巴嫩"}]}`), &reqs))

	rows, err := ReplaceTopicLaneConstraints(context.Background(), topicID, reqs.Constraints)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "黎巴嫩", rows[0].Term)
	assert.Equal(t, repository.LaneConstraintModeRequired, rows[0].Mode, "mode 缺省落 required")
	assert.Equal(t, "qwen3-embedding", rows[0].EmbeddingModel)
	assert.NotEmpty(t, rows[0].TermEmbedding)
}

// TestConstraint_UnchangedTermReusesStoredAnchor pins the write-path idempotence:
// re-submitting the same term SHALL NOT re-embed (the anchor is a pure function of
// the term, and re-using it keeps the vector space stable when the route degrades).
func TestConstraint_UnchangedTermReusesStoredAnchor(t *testing.T) {
	repo, topicID := setupLaneConstraintWriteTest(t)

	calls := 0
	restore := stubEmbed(t, "以色列 event", []float64{1, 0}, "qwen3-embedding", &calls)
	defer restore()

	_, err := ReplaceTopicLaneConstraints(context.Background(), topicID, []LaneConstraintRequest{{Term: "以色列"}})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	rows, err := ReplaceTopicLaneConstraints(context.Background(), topicID, []LaneConstraintRequest{
		{Term: "以色列", Mode: repository.LaneConstraintModePreferred},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "同短词重提 SHALL NOT 重算锤向量")
	require.Len(t, rows, 1)
	assert.Equal(t, repository.LaneConstraintModePreferred, rows[0].Mode)
	assert.Equal(t, "qwen3-embedding", rows[0].EmbeddingModel)

	// And the stored set is the caller's set (omission = deletion).
	rows, err = repo.ListTopicLaneConstraints(topicID)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

// TestConstraint_InvalidPayloadRejectedBeforeWrite covers the service-side
// validation contract（400 且 SHALL NOT 部分写入）。
func TestConstraint_InvalidPayloadRejectedBeforeWrite(t *testing.T) {
	repo, topicID := setupLaneConstraintWriteTest(t)
	bad := 1.5
	cases := []struct {
		name string
		reqs []LaneConstraintRequest
	}{
		{"duplicate term", []LaneConstraintRequest{{Term: "以色列"}, {Term: " 以色列 "}}},
		{"empty term", []LaneConstraintRequest{{Term: "  "}}},
		{"invalid mode", []LaneConstraintRequest{{Term: "以色列", Mode: "optional"}}},
		{"margin out of range", []LaneConstraintRequest{{Term: "以色列", Margin: &bad}}},
		{"max_distance out of range", []LaneConstraintRequest{{Term: "以色列", MaxDistance: &bad}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReplaceTopicLaneConstraints(context.Background(), topicID, tc.reqs)
			require.Error(t, err)
			var badRequest *LaneConstraintValidationError
			assert.ErrorAs(t, err, &badRequest, "客户端载荷问题 SHALL 归为 400 类错误")

			rows, listErr := repo.ListTopicLaneConstraints(topicID)
			require.NoError(t, listErr)
			assert.Empty(t, rows, "非法载荷 SHALL NOT 部分写入")
		})
	}
}
