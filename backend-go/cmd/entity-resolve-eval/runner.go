package main

// Evaluation runner (design D11): walks the FROZEN samples through the real
// resolution chain behind the send-guard.
//
// Hard isolation guarantees, by construction:
//   - the runner NEVER reads gold: Run() accepts samples only; there is no
//     gold path/field anywhere in the runner API or body;
//   - the runner NEVER writes tag business tables: it writes the entity
//     layer (entity_* rows, SourceTagID=nil synthetic provenance) and the
//     returned records (eval artifacts). ai_call_logs rows come from the
//     router's normal audit, which is expected;
//   - without a registered send-guard the run pre-blocks BEFORE any request.

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/entity"
)

// Runner executes one bounded evaluation run.
type Runner struct {
	db        *gorm.DB
	resolver  *entity.Resolver
	guard     *SendGuard
	protocol  Protocol
	sessionID string
}

// NewRunner wires the resolver with guarded provider clients. inner is the
// real provider client in production (airouter.NewOpenAICompatibleClient)
// and a fake in tests; the guard wraps it and is REGISTERED ONTO A NEW
// ROUTER-OWNED CLIENT MAP via RegisterClient — production routing code is
// untouched (D11: 仅在评测入口注入).
func NewRunner(db *gorm.DB, protocol Protocol, sessionID string, allowedModels []string, inner airouter.ProviderClient) (*Runner, error) {
	if err := VerifyFrozen(protocol); err != nil {
		return nil, err
	}
	if inner == nil {
		return nil, fmt.Errorf("inner provider client 未提供：无法建立发送前计数（预先阻断）")
	}
	guard := NewSendGuard("eval", protocol.Budget, allowedModels, inner)
	router := airouter.NewRouterWithStore(airouter.NewStore(db))
	router.RegisterClient(airouter.ProviderTypeOpenAICompatible, guard)
	router.RegisterClient(airouter.ProviderTypeOllama, guard)

	chat := func(ctx context.Context, req airouter.ChatRequest) (string, error) {
		result, err := router.Chat(ctx, req)
		if err != nil {
			return "", err
		}
		return result.Content, nil
	}
	embed := func(ctx context.Context, text string) (entity.QueryVector, error) {
		result, err := router.Embed(ctx, airouter.EmbeddingRequest{
			Input:     []string{text},
			Operation: "entity_eval.query",
			Metadata:  map[string]any{"session_id": sessionID},
		}, airouter.CapabilityEmbedding)
		if err != nil {
			return entity.QueryVector{}, err
		}
		if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
			return entity.QueryVector{}, fmt.Errorf("empty embedding result")
		}
		vec := result.Embeddings[0]
		return entity.QueryVector{
			Vec:     vec,
			Triplet: entity.SpaceTriplet{Model: result.Model, Version: result.Model, Dim: len(vec)},
		}, nil
	}

	repo := repository.NewEntityRepository(db)
	resolver := entity.NewResolver(repo, chat, embed, entity.DefaultResolverConfig())
	return &Runner{db: db, resolver: resolver, guard: guard, protocol: protocol, sessionID: sessionID}, nil
}

// Run processes the frozen samples in order (cold-start semantics: the pool
// grows as the run creates entities — the scorer later annotates which gold
// entities were already in the pool at each point; the runner itself stays
// gold-blind). Returns per-mention records plus the actual-budget report.
func (r *Runner) Run(ctx context.Context, samples []EvalSample) ([]MentionRecord, BudgetReport, error) {
	// Pre-block when send-side counting could not be established.
	if err := EnsureGuardRegistered(r.guard != nil); err != nil {
		return nil, BudgetReport{}, err
	}
	if err := VerifyFrozen(r.protocol); err != nil {
		return nil, BudgetReport{}, err
	}
	if SamplesDigest(samples) != r.protocol.SampleDigest {
		return nil, BudgetReport{}, fmt.Errorf("样本清单与冻结协议不一致（%s ≠ %s）：冻结后不得改样本", SamplesDigest(samples), r.protocol.SampleDigest)
	}

	records := make([]MentionRecord, 0, len(samples))
	report := BudgetReport{Plan: r.protocol.Budget}
	secondRoundAttempts := 0 // Σ records.Attempts（真实二判尝试数，含有限重试）
	for _, s := range samples {
		rec := MentionRecord{
			SampleID:   s.SampleID,
			Mention:    s.Mention,
			SubjectKey: s.SubjectKey, // 既有身份对照侧（非金标）
		}
		if s.Disputed {
			// 争议项预列：不进入运行（不计身份分也不耗预算）。
			continue
		}
		resolution := r.resolver.ResolveMention(ctx, entity.MentionRequest{
			MentionText:    s.Mention,
			SubjectDesc:    s.SubjectDesc,
			ContextSnippet: s.ContextText,
			SourceTagID:    nil, // eval synthetic source: never writes business tables
			AuxLabelIDs:    nil,
			SessionID:      r.sessionID,
		})
		rec.EntityID = resolution.EntityID
		rec.Status = resolution.Status
		rec.TechFailure = resolution.Status == entity.StatusUnresolved
		// 实际尝试次数与失败原因随 records 落盘（错例清单可由产物复算，
		// 不从审计反推）。
		rec.Attempts = resolution.Attempts
		rec.Reason = resolution.Reason
		secondRoundAttempts += resolution.Attempts
		if resolution.Status == entity.StatusCreated {
			rec.EntityID = resolution.EntityID // created entities carry their new ID
		}
		records = append(records, rec)
	}
	// 预算实际值（口径见 BudgetReport 注释）：全部由 guard 的发送前计数与
	// records 的 Attempts 机械导出，不估算。
	report.ActualSecondRound = secondRoundAttempts
	report.ActualEmbedTexts = r.guard.EmbedTexts()
	report.ActualHTTP = r.guard.HTTPCount()
	report.Rejected = r.guard.Rejections()
	// ① 实际抽取调用数 = chat 尝试 − 二判尝试（二判之外只可能是文章抽取）。
	report.ActualExtraction = r.guard.ChatCount() - secondRoundAttempts
	if report.ActualExtraction < 0 {
		// 防御：二判计数不可能超过实际 chat 尝试；负数归零而非报假数据。
		report.ActualExtraction = 0
	}
	return records, report, nil
}

// EntityLayerStats summarizes the entity-layer footprint of a run (cleanup
// support: every eval row carries the run's session provenance).
func EntityLayerStats(ctx context.Context, db *gorm.DB) (entities int64, mentions int64, err error) {
	repo := repository.NewEntityRepository(db)
	if entities, err = repo.CountEntities(ctx); err != nil {
		return 0, 0, err
	}
	if err = db.WithContext(ctx).Model(&tagmodels.EntityMention{}).Count(&mentions).Error; err != nil {
		return entities, 0, err
	}
	return entities, mentions, nil
}
