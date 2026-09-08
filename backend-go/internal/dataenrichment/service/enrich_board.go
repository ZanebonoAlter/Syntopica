package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/logging"
)

// ── EnrichBoard：版块简报编排入口（board-level-deep-analysis tasks 3.3，D2）───
//
// 默认触发只走简报链：enrichment_enabled 门槛 → 活跃泳道 → D10 补全门 →
// 态势卡装配 → board_brief 单次 LLM（坏 JSON 重试一次→机械降级，永不失败）
// → 持久化不可变 board_brief 快照（tool_calls 恒空数组；review judge 留待
// 3.5 按 kind 隔离接入）。
//
// 旧论文链（board_interpret → 研究方向 agent loop → board analyze →
// legacy 持久化）已整体退场（tasks 6.1）：生产不再有任何编排入口能跑通
// 该链。board_interpret / runBoardAgentLoop / board analyze prompt 的单步
// 实现保留在各自文件中仅供历史单测与阅读（*ForTest 包装），不再有落库
// 调用方；历史 legacy 结果只读兼容见 handler 的 list/detail 路由。

// BoardEnrichmentOutput mirrors EnrichmentOutput for the board scope.
type BoardEnrichmentOutput struct {
	Result    *repository.TopicEnrichmentResult `json:"result"`
	Review    *repository.TopicEnrichmentReview `json:"review,omitempty"`
	Freshness *FreshnessGateReport              `json:"freshness_report,omitempty"`
}

// laneRef is one board-sector lane citation (shared by the brief payload;
// legacy reports carry the same shape inside sectors jsonb).
type laneRef struct {
	LaneID uint `json:"lane_id"`
	// BoardID carries the owning board for cross-board references
	// (add-evidence-backed-cross-board-relations); 0/omitted = this board.
	BoardID uint   `json:"board_id,omitempty"`
	Note    string `json:"note,omitempty"`
}

// BoardConfigResolver reads a board's enrichment config by board ID (the
// existing BoardConfigReader resolves by topic; EnrichBoard starts from the
// board).
type BoardConfigResolver interface {
	GetBoardConfigByBoardID(ctx context.Context, boardID uint) (*BoardEnrichmentConfig, error)
}

// ginBoardConfigResolver adapts via a raw query (avoids importing topicgraph;
// mirrors board_config_impl.go's raw-table style).
type dbBoardConfigResolver struct {
	db *gorm.DB
}

// NewDBBoardConfigResolver creates the production resolver.
func NewDBBoardConfigResolver(db *gorm.DB) BoardConfigResolver { return &dbBoardConfigResolver{db: db} }

func (r *dbBoardConfigResolver) GetBoardConfigByBoardID(ctx context.Context, boardID uint) (*BoardEnrichmentConfig, error) {
	var enabled bool
	var windowDays int
	var relationAuto bool
	if err := r.db.WithContext(ctx).
		Table("semantic_labels").
		Select("enrichment_enabled, COALESCE(window_days, 14), COALESCE(relation_auto_discovery_enabled, false)").
		Where("id = ? AND label_type = ?", boardID, "board").
		Row().Scan(&enabled, &windowDays, &relationAuto); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DefaultBoardConfig(), nil
		}
		return nil, fmt.Errorf("board config %d: %w", boardID, err)
	}
	cfg := DefaultBoardConfig()
	cfg.EnrichmentEnabled = enabled
	if windowDays > 0 {
		cfg.WindowDays = windowDays
	}
	cfg.RelationAutoDiscoveryEnabled = relationAuto
	return cfg, nil
}

// SetBoardConfigResolver wires the resolver post-construction (nil = default deny).
func (o *OrchestratorService) SetBoardConfigResolver(r BoardConfigResolver) { o.boardResolver = r }

// EnrichBoard runs the default board cycle-B flow: a board_brief only
// (manual trigger; no auto investigation, no tools, no thesis chain).
func (o *OrchestratorService) EnrichBoard(ctx context.Context, boardID uint) (*BoardEnrichmentOutput, error) {
	// 0. Board config gate (M5.1).
	if o.boardResolver == nil {
		return nil, fmt.Errorf("enrich board %d: board config resolver not wired", boardID)
	}
	cfg, err := o.boardResolver.GetBoardConfigByBoardID(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("enrich board %d: board config: %w", boardID, err)
	}
	if !cfg.EnrichmentEnabled {
		return nil, fmt.Errorf("enrich board %d: enrichment not enabled for this board", boardID)
	}

	sessionID := generateBoardSessionID(boardID)

	// 1. Active lanes of this board (also the lane whitelist).
	laneIDs, err := o.boardActiveLaneIDs(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("enrich board %d: list lanes: %w", boardID, err)
	}

	// 2. D10 completeness gate — top up month/year lifelines BEFORE assembly.
	freshness := o.ensureLaneFreshness(ctx, laneIDs)

	// 3. Situation cards (sorted by quality; the material budget).
	cards, err := o.assembleSituationCards(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("enrich board %d: situation cards: %w", boardID, err)
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("enrich board %d: no active lanes to analyze", boardID)
	}
	cardsMD := RenderSituationCardsMarkdown(cards)
	allSparse := boardCardsAllSparse(cards)

	// 4. board_brief：一次 LLM（重试一次→机械降级），仅态势卡输入。
	//    3.5: 同 kind（board_brief）applied review digest 在生成【前】读取
	//    注入（advisory 偏差提醒，不当本次事实）；读取失败降级为空不阻塞。
	reviewDigest, digestErr := o.boardBriefReviewDigest(ctx, boardID)
	if digestErr != nil {
		logging.Warnf("enrich board %d: same-kind review digest unavailable (skipped): %v", boardID, digestErr)
	}
	// 4.5 Confirmed cross-board relation background (5.x): load active
	// confirmed relations touching this board, mechanically select within
	// budget (quality DESC, confirmed_at DESC, id ASC) and render the block.
	crossMD, crossRefs, crossTruncated := o.loadConfirmedRelationBackground(ctx, boardID)
	in := boardBriefInput{
		SessionID:        sessionID,
		CardsMD:          cardsMD,
		ReviewDigest:     reviewDigest,
		AllSparse:        allSparse,
		Cards:            cards,
		CrossRelationsMD: crossMD,
		CrossRelations:   crossRefs,
	}
	brief, meta := o.generateBoardBrief(ctx, in)
	// Mechanical field: the server assembles the consumed relations onto the
	// payload; the LLM never generates them (spec: 新简报消费确认关系).
	brief.CrossBoardRelations = crossRefs

	// 5. Persist the immutable brief (atomic; generation snapshot fixed).
	out, err := o.persistBoardBriefResult(ctx, boardID, sessionID, brief, meta,
		boardBriefPromptInput{CardsMD: cardsMD, ReviewDigest: reviewDigest, AllSparse: allSparse, CrossRelationsMD: crossMD, TruncatedCrossRelations: crossTruncated},
		cards, freshness)
	if err != nil {
		return nil, err
	}

	// 6. 3.5 review judge：仅与同 board 上一份 board_brief 比较（legacy/
	//    investigation/topic 永不参与）；全程 non-fatal，失败只记日志，
	//    已落库简报不回滚、lifeline 永不回写。
	out.Review = o.judgeBoardBriefAgainstPrev(ctx, boardID, sessionID, out.Result)

	// 7. Non-fatal auto relation discovery (7.1): board switch + budget +
	// sparse gates inside; never rolls the brief back, never blocks here.
	o.maybeAutoDiscoverRelations(cfg, boardID, out.Result.ID, brief)
	return out, nil
}

// EnrichBoardLegacyAnalysis（旧论文链入口）已删除（tasks 6.1）：生产不再有
// 任何 handler/service 公开路径能运行 board_interpret → 研究方向 agent loop
// → board analyze → legacy 持久化；历史 legacy 结果只读兼容走 handler 的
// list/detail 路由。

// boardCardsAllSparse reports whether every card signals thin/no material
// (no facts digest or a degraded sparse history) — the honest-decline input.
func boardCardsAllSparse(cards []LaneSituationCard) bool {
	for _, c := range cards {
		if c.FactsSource != "none" && c.Signals.SparseHistory < situationCardSparseDegraded {
			return false
		}
	}
	return true
}

// boardActiveLaneIDs lists active lane IDs for the board.
// BoardEnrichmentEnabled is the cheap synchronous pre-flight used by the
// async trigger handler: disabled boards are rejected with a client error
// before a background job is scheduled (M6.1 semantics preserved).
func (o *OrchestratorService) BoardEnrichmentEnabled(ctx context.Context, boardID uint) error {
	if o.boardResolver == nil {
		return fmt.Errorf("enrich board %d: board config resolver not wired", boardID)
	}
	cfg, err := o.boardResolver.GetBoardConfigByBoardID(ctx, boardID)
	if err != nil {
		return fmt.Errorf("enrich board %d: board config: %w", boardID, err)
	}
	if !cfg.EnrichmentEnabled {
		return fmt.Errorf("enrich board %d: enrichment not enabled for this board", boardID)
	}
	return nil
}

func (o *OrchestratorService) boardActiveLaneIDs(ctx context.Context, boardID uint) ([]uint, error) {
	var ids []uint
	err := o.repo.DB().WithContext(ctx).
		Table("board_persistent_topics").
		Where("semantic_board_id = ? AND status = ?", boardID, "active").
		Order("id ASC").
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// uintPtr returns a pointer to v (review FK columns are pointer-typed).
func uintPtr(v uint) *uint { return &v }

// loadConfirmedRelationBackground selects budgeted confirmed, unexpired
// cross-board relations for the board (either side) and renders the prompt
// block. Pure mechanical ordering (spec: 关系数量超过预算的确定性排序);
// failures degrade to empty (brief generation never blocks on this).
func (o *OrchestratorService) loadConfirmedRelationBackground(ctx context.Context, boardID uint) (string, []boardBriefCrossRelation, int) {
	if o.repo == nil {
		return "", nil, 0
	}
	rows, err := o.repo.ListActiveConfirmedRelationsForBoard(ctx, boardID)
	if err != nil || len(rows) == 0 {
		if err != nil {
			logging.Warnf("enrich board %d: confirmed relations unavailable (skipped): %v", boardID, err)
		}
		return "", nil, 0
	}
	cfg := config.EffectiveCrossBoardRelationConfig()
	refs := make([]boardBriefCrossRelation, 0, len(rows))
	for _, r := range rows {
		other := r.SourceBoardID
		direction := "incoming"
		if r.SourceBoardID == boardID {
			if r.TargetBoardID == nil {
				continue
			}
			other = *r.TargetBoardID
			direction = "outgoing"
		}
		ref := boardBriefCrossRelation{
			RelationID:   r.ID,
			OtherBoardID: other,
			Direction:    direction,
			RelationType: r.RelationType,
			Claim:        r.Claim,
			QualityGrade: r.QualityGrade,
		}
		if r.ConfirmedAt != nil {
			ref.ConfirmedAt = r.ConfirmedAt.Format("2006-01-02")
		}
		if r.ExpiresAt != nil {
			ref.ExpiresAt = r.ExpiresAt.Format("2006-01-02")
		}
		if len(r.Evidence) > 0 {
			ref.EvidenceURL = r.Evidence[0].URL
			ref.EvidenceQuote = truncateRunes(r.Evidence[0].Quote, 120)
		}
		refs = append(refs, ref)
	}
	truncated := 0
	if len(refs) > cfg.BriefMaxRelations {
		truncated = len(refs) - cfg.BriefMaxRelations
		refs = refs[:cfg.BriefMaxRelations]
	}
	if len(refs) == 0 {
		return "", nil, 0
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "共 %d 条有效确认关系", len(refs))
	if truncated > 0 {
		fmt.Fprintf(&sb, "（另有 %d 条超出预算未展示）", truncated)
	}
	sb.WriteString("：\n")
	var total int
	for i, ref := range refs {
		line := fmt.Sprintf("%d. [%s/%s] %s — 确认于 %s", i+1, ref.RelationType, ref.QualityGrade, ref.Claim, orDashDate(ref.ConfirmedAt))
		if ref.EvidenceURL != "" {
			line += "（证据: " + ref.EvidenceURL + "）"
		}
		line += "\n"
		if total+len([]rune(line)) > cfg.BriefMaxRelationRunes {
			// rune budget hit: drop this and everything after; count as truncated
			truncated += len(refs) - i
			refs = refs[:i]
			break
		}
		total += len([]rune(line))
		sb.WriteString(line)
	}
	return sb.String(), refs, truncated
}

func orDashDate(s string) string {
	if s == "" {
		return "（未知日期）"
	}
	return s
}
