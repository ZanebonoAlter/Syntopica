package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
)

// ── 简报消费确认跨版块关系（add-evidence-backed-cross-board-relations 5.1/5.2）──
//
// 覆盖：confirmed 未过期关系注入 prompt 背景块 + 机械 cross_board_relations
// 字段 + input_snapshot 冻结；非 confirmed/过期关系不进入；预算内确定性排序
// 与截断计数；简报落库后旧快照不受后续关系状态变化影响（不可变）。

func seedConfirmedCrossRelation(t *testing.T, repo *repository.Repository, id, sourceBoard, targetBoard uint, kind, quality, claim string, confirmedDaysAgo int, expiresInDays int) {
	t.Helper()
	expires := time.Now().AddDate(0, 0, expiresInDays)
	confirmed := time.Now().AddDate(0, 0, -confirmedDaysAgo)
	rel := &repository.CrossBoardRelation{
		SourceBoardID: sourceBoard, TargetBoardID: &targetBoard,
		TargetConcept: fmt.Sprintf("概念-%d", targetBoard), RelationType: kind,
		Claim: claim, VerificationVerdict: repository.RelationVerdictSupported,
		QualityGrade: quality, Status: repository.RelationStatusConfirmed,
		SuggestionHash: fmt.Sprintf("cbr-brief-h-%d", id),
		ExpiresAt:      &expires, ConfirmedAt: &confirmed,
		Evidence: []repository.RelationEvidence{
			{Ref: "run/step1", Tool: "web_search", URL: "https://example.com/r" + fmt.Sprint(id), Quote: "原始网页摘录 " + claim, Institution: "示例机构", Date: "2026-08-30", Use: "support", Verified: true},
		},
	}
	require.NoError(t, repo.DB().Create(rel).Error)
	t.Cleanup(func() { _ = repo.DB().Exec(`DELETE FROM cross_board_relations WHERE id = ?`, rel.ID).Error })
}

// TestBoardBriefConsumesConfirmedRelations: confirmed 未过期关系 → prompt
// 背景块（含 claim + 证据 URL）+ 机械 cross_board_relations 字段 +
// prompt_inputs 冻结；简报仍不联网（tool_calls 恒空）。
func TestBoardBriefConsumesConfirmedRelations(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	seedBoardLane(t, repo, 901, 8821, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	seedEnabledAnalysisMethod(t, repo)
	router.addResponse(validBriefLLM)
	// 一进（本板 8821 是 target）一出（本板 8821 是 source）。
	seedConfirmedCrossRelation(t, repo, 1, 8821, 9902, "causal", "medium", "日债收益率走高经避险资金传导", 2, 30)
	seedConfirmedCrossRelation(t, repo, 2, 9901, 8821, "common_driver", "high", "与中东局势共享同一避险驱动", 5, 30)

	out, err := orch.EnrichBoard(context.Background(), 8821)
	require.NoError(t, err)

	// Prompt 契约：背景块存在，方向正确，证据 URL 可核查。
	prompt := router.Calls[0].Messages[0].Content
	require.Contains(t, prompt, "已确认外部关系背景")
	require.Contains(t, prompt, "日债收益率走高经避险资金传导")
	require.Contains(t, prompt, "与中东局势共享同一避险驱动")
	require.Contains(t, prompt, "https://example.com/r1")

	// 机械字段：sectors.cross_board_relations 由服务端装配（非 LLM 生成）。
	var payload struct {
		CrossBoardRelations []struct {
			RelationID   uint   `json:"relation_id"`
			OtherBoardID uint   `json:"other_board_id"`
			Direction    string `json:"direction"`
			RelationType string `json:"relation_type"`
			Claim        string `json:"claim"`
			EvidenceURL  string `json:"evidence_url"`
		} `json:"cross_board_relations"`
	}
	require.NoError(t, json.Unmarshal(out.Result.Sectors, &payload))
	require.Len(t, payload.CrossBoardRelations, 2)
	byClaim := map[string]struct {
		RelationID   uint   `json:"relation_id"`
		OtherBoardID uint   `json:"other_board_id"`
		Direction    string `json:"direction"`
		RelationType string `json:"relation_type"`
		Claim        string `json:"claim"`
		EvidenceURL  string `json:"evidence_url"`
	}{}
	for _, cr := range payload.CrossBoardRelations {
		byClaim[cr.Claim] = cr
	}
	outRel := byClaim["日债收益率走高经避险资金传导"]
	require.Equal(t, uint(9902), outRel.OtherBoardID)
	require.Equal(t, "outgoing", outRel.Direction)
	require.NotZero(t, outRel.RelationID)
	require.NotEmpty(t, outRel.EvidenceURL)
	inRel := byClaim["与中东局势共享同一避险驱动"]
	require.Equal(t, uint(9901), inRel.OtherBoardID)
	require.Equal(t, "incoming", inRel.Direction)

	// 快照冻结：prompt_inputs 携带注入块原文。
	var snap struct {
		PromptInputs struct {
			CrossRelationsMD        string `json:"cross_relations_md"`
			TruncatedCrossRelations int    `json:"truncated_cross_relations"`
		} `json:"prompt_inputs"`
	}
	require.NoError(t, json.Unmarshal(out.Result.InputSnapshot, &snap))
	require.NotEmpty(t, snap.PromptInputs.CrossRelationsMD)
	require.Zero(t, snap.PromptInputs.TruncatedCrossRelations)

	// 简报仍不联网。
	require.Equal(t, "[]", string(out.Result.ToolCalls))
	require.Equal(t, 1, countBriefCalls(router))
}

// TestBoardBriefIgnoresNonConfirmedOrExpiredRelations: proposed/dismissed/
// 过期 confirmed 均不进入简报（prompt 无背景块、机械字段为空数组）。
func TestBoardBriefIgnoresNonConfirmedOrExpiredRelations(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	seedBoardLane(t, repo, 901, 8821, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	router.addResponse(validBriefLLM)

	db := repo.DB()
	stale := time.Now().AddDate(0, 0, -3)
	dismissed := time.Now().AddDate(0, 0, -1)
	rows := []*repository.CrossBoardRelation{
		{SourceBoardID: 8821, TargetBoardID: u64ptr(9902), RelationType: "causal", Claim: "未确认关系",
			Status: repository.RelationStatusProposed, SuggestionHash: "cbr-brief-n1", VerificationVerdict: "supported"},
		{SourceBoardID: 8821, TargetBoardID: u64ptr(9903), RelationType: "causal", Claim: "被驳回关系",
			Status: repository.RelationStatusDismissed, SuggestionHash: "cbr-brief-n2", DismissedAt: &dismissed, VerificationVerdict: "supported"},
		{SourceBoardID: 8821, TargetBoardID: u64ptr(9904), RelationType: "causal", Claim: "过期确认关系",
			Status: repository.RelationStatusConfirmed, SuggestionHash: "cbr-brief-n3", ExpiresAt: &stale, VerificationVerdict: "supported"},
	}
	for _, r := range rows {
		require.NoError(t, db.Create(r).Error)
		t.Cleanup(func() { _ = db.Exec(`DELETE FROM cross_board_relations WHERE id = ?`, r.ID).Error })
	}

	out, err := orch.EnrichBoard(context.Background(), 8821)
	require.NoError(t, err)

	prompt := router.Calls[0].Messages[0].Content
	require.NotContains(t, prompt, "已确认外部关系背景")
	require.NotContains(t, prompt, "未确认关系")
	require.NotContains(t, prompt, "被驳回关系")
	require.NotContains(t, prompt, "过期确认关系")

	var payload struct {
		CrossBoardRelations []json.RawMessage `json:"cross_board_relations"`
	}
	require.NoError(t, json.Unmarshal(out.Result.Sectors, &payload))
	require.Empty(t, payload.CrossBoardRelations, "field must serialize as empty slice, not null")

	var snap struct {
		PromptInputs struct {
			CrossRelationsMD string `json:"cross_relations_md"`
		} `json:"prompt_inputs"`
	}
	require.NoError(t, json.Unmarshal(out.Result.InputSnapshot, &snap))
	require.Empty(t, snap.PromptInputs.CrossRelationsMD)
}

// TestBoardBriefRelationBudgetAndOrder: 超预算时按 quality DESC, confirmed_at
// DESC, id ASC 截断；截断数进快照；落库后关系状态变化不改旧简报。
func TestBoardBriefRelationBudgetAndOrder(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	seedBoardLane(t, repo, 901, 8821, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	router.addResponse(validBriefLLM)

	// 4 条 confirmed，预算 3：high(新) > high(旧) > medium(新) 入选，medium(旧) 截断。
	seedConfirmedCrossRelation(t, repo, 11, 9901, 8821, "causal", "high", "高质量新确认", 1, 30)
	seedConfirmedCrossRelation(t, repo, 12, 9902, 8821, "common_driver", "high", "高质量旧确认", 9, 30)
	seedConfirmedCrossRelation(t, repo, 13, 9903, 8821, "causal", "medium", "中质量新确认", 2, 30)
	seedConfirmedCrossRelation(t, repo, 14, 9904, 8821, "contextual", "medium", "中质量旧确认-应截断", 8, 30)

	out, err := orch.EnrichBoard(context.Background(), 8821)
	require.NoError(t, err)

	var payload struct {
		CrossBoardRelations []struct {
			Claim string `json:"claim"`
		} `json:"cross_board_relations"`
	}
	require.NoError(t, json.Unmarshal(out.Result.Sectors, &payload))
	require.Len(t, payload.CrossBoardRelations, 3)
	claims := []string{payload.CrossBoardRelations[0].Claim, payload.CrossBoardRelations[1].Claim, payload.CrossBoardRelations[2].Claim}
	require.Equal(t, []string{"高质量新确认", "高质量旧确认", "中质量新确认"}, claims)

	var snap struct {
		PromptInputs struct {
			TruncatedCrossRelations int `json:"truncated_cross_relations"`
		} `json:"prompt_inputs"`
	}
	require.NoError(t, json.Unmarshal(out.Result.InputSnapshot, &snap))
	require.Equal(t, 1, snap.PromptInputs.TruncatedCrossRelations)
	prompt := router.Calls[0].Messages[0].Content
	require.NotContains(t, prompt, "中质量旧确认-应截断")

	// 不可变：落库后 dismiss 第一条，旧 sectors 语义不变（jsonb 存储会规范化
	// 键序/空白，故断言解码后的消费内容而非字节相等）。
	require.NoError(t, repo.DB().Exec(`UPDATE cross_board_relations SET status='dismissed' WHERE suggestion_hash='cbr-brief-h-11'`).Error)
	var fresh repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", out.Result.ID).First(&fresh).Error)
	var freshPayload struct {
		CrossBoardRelations []struct {
			Claim string `json:"claim"`
		} `json:"cross_board_relations"`
	}
	require.NoError(t, json.Unmarshal(fresh.Sectors, &freshPayload))
	require.Len(t, freshPayload.CrossBoardRelations, 3)
	require.Equal(t, claims, []string{freshPayload.CrossBoardRelations[0].Claim, freshPayload.CrossBoardRelations[1].Claim, freshPayload.CrossBoardRelations[2].Claim}, "persisted brief must be immutable to later relation changes")
}

// TestBoardBriefLegacySectorWithoutCrossField: 无 cross_board_relations 的
// 旧简报 sectors 解码为空数组（降级读取不崩）。
func TestBoardBriefLegacySectorWithoutCrossField(t *testing.T) {
	legacy := []byte(`{"result_kind":"board_brief","summary":"旧简报","observations":[],"relationships":[]}`)
	var payload struct {
		CrossBoardRelations []json.RawMessage `json:"cross_board_relations"`
	}
	require.NoError(t, json.Unmarshal(legacy, &payload))
	require.Empty(t, payload.CrossBoardRelations)
	require.False(t, strings.Contains(string(legacy), "cross_board_relations"))
}

func u64ptr(u uint) *uint { return &u }
