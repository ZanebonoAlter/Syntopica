package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/tracing"
)

// ── 推荐生命周期用户动作（improve-discovery-recommendations 4.4 / design D5）──
//
// 冷却与长期排除的权威是 candidate_preferences（候选级，跨 qa/refresh 生效）；
// 旧的 route+dismissed_at 冷却 hash 池已退役（dismissed_at 历史行只读，迁移已把
// 剩余冷却回填为 snoozed_until）。写路径只写 candidate_preferences，pending 推荐
// 行状态不变——卡片退出/回归默认列表由列表与召回过滤派生，恢复只清字段
// （仅恢复推荐资格，不建订阅、不立即出卡）。

// candidatePreferenceBlocked 判断候选当前是否被长期排除或冷却阻断：
// excluded_at 非空，或 now 早于 snoozed_until 即阻断；无偏好行 = 不阻断。
// 列表过滤、发布前复查共用此判据（召回 SQL 内联同一语义）。
func candidatePreferenceBlocked(db *gorm.DB, candidateID uint, now time.Time) (bool, error) {
	if candidateID == 0 {
		return false, nil
	}
	var pref models.CandidatePreference
	err := db.Where("candidate_id = ?", candidateID).First(&pref).Error
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if pref.ExcludedAt != nil {
		return true, nil
	}
	return pref.SnoozedUntil != nil && now.Before(*pref.SnoozedUntil), nil
}

// loadRecommendation 按 id 取推荐行（任意状态；调用方按需自行判定状态）。
func (s *RecommendationService) loadRecommendation(ctx context.Context, id uint) (*models.FeedRecommendation, error) {
	var rec models.FeedRecommendation
	if err := s.db.WithContext(ctx).First(&rec, id).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// resolveRecommendationCandidate 解析推荐行对应的统一候选 id：已有 candidate_id
// 直接用；缺失（迁移前老行）按 route 兜底建档并回填，幂等。
func (s *RecommendationService) resolveRecommendationCandidate(ctx context.Context, rec *models.FeedRecommendation) (uint, error) {
	if rec.CandidateID != nil && *rec.CandidateID != 0 {
		return *rec.CandidateID, nil
	}
	var route models.RSSHubRoute
	if err := s.db.WithContext(ctx).First(&route, rec.RouteID).Error; err != nil {
		return 0, fmt.Errorf("load route %d: %w", rec.RouteID, err)
	}
	cand, err := ensureRouteCandidate(s.db.WithContext(ctx), candidateRow{
		RouteID: route.ID, Namespace: route.Namespace, Path: route.Path,
	})
	if err != nil {
		return 0, fmt.Errorf("ensure candidate for route %d: %w", rec.RouteID, err)
	}
	if err := s.db.WithContext(ctx).Model(&models.FeedRecommendation{}).Where("id = ?", rec.ID).
		Update("candidate_id", cand.ID).Error; err != nil {
		return 0, err
	}
	return cand.ID, nil
}

// upsertCandidatePreference 按候选 upsert 偏好行：存在 → 应用 updates；不存在 → 新建
// （newPref 附带初始字段）。excluded_at/snoozed_until 的「不清空」语义由调用方只传
// 需变更的键保证。
func (s *RecommendationService) upsertCandidatePreference(ctx context.Context, candidateID uint, updates map[string]any) error {
	now := time.Now()
	var pref models.CandidatePreference
	err := s.db.WithContext(ctx).Where("candidate_id = ?", candidateID).First(&pref).Error
	if err == nil {
		updates["updated_at"] = now
		return s.db.WithContext(ctx).Model(&models.CandidatePreference{}).
			Where("id = ?", pref.ID).Updates(updates).Error
	}
	if !isNotFound(err) {
		return err
	}
	created := models.CandidatePreference{CandidateID: candidateID}
	if v, ok := updates["excluded_at"]; ok {
		if t, ok2 := v.(time.Time); ok2 {
			created.ExcludedAt = &t
		}
	}
	if v, ok := updates["snoozed_until"]; ok {
		if t, ok2 := v.(time.Time); ok2 {
			created.SnoozedUntil = &t
		}
	}
	return s.db.WithContext(ctx).Create(&created).Error
}

// SnoozeRecommendation 「暂时不看」：写 candidate_preferences.snoozed_until =
// now + snooze_days（无行则建；已有长期排除不动 excluded_at）。返回实际到期时刻
// 供前端展示。仅 pending 卡可暂时不看（历史行不支持）。
func (s *RecommendationService) SnoozeRecommendation(ctx context.Context, id uint) (*time.Time, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.SnoozeRecommendation")
	defer span.End()
	rec, err := s.loadRecommendation(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.Status != "pending" {
		return nil, fmt.Errorf("recommendation %d not pending (status=%s)", id, rec.Status)
	}
	cfg, err := loadLifecycleConfig(s.db)
	if err != nil {
		return nil, err
	}
	candID, err := s.resolveRecommendationCandidate(ctx, rec)
	if err != nil {
		return nil, err
	}
	until := cfg.EndOfSnooze(time.Now())
	if err := s.upsertCandidatePreference(ctx, candID, map[string]any{"snoozed_until": until}); err != nil {
		return nil, err
	}
	return &until, nil
}

// ExcludeRecommendation 长期排除：写 candidate_preferences.excluded_at = now
// （无行则建；不动 snoozed_until）。跨 qa/refresh 全局生效，候选 enabled 开关
// （目录推荐启停）不得解除排除（写路径互不相通）。
func (s *RecommendationService) ExcludeRecommendation(ctx context.Context, id uint) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.ExcludeRecommendation")
	defer span.End()
	rec, err := s.loadRecommendation(ctx, id)
	if err != nil {
		return err
	}
	if rec.Status == "accepted" {
		return fmt.Errorf("recommendation %d already accepted", id)
	}
	candID, err := s.resolveRecommendationCandidate(ctx, rec)
	if err != nil {
		return err
	}
	return s.upsertCandidatePreference(ctx, candID, map[string]any{"excluded_at": time.Now()})
}

// RestoreRecommendation 恢复：清 excluded_at 与 snoozed_until，仅恢复推荐资格——
// 不建订阅、不生成新 pending 卡、不改变已订阅源。无偏好行时幂等成功。
func (s *RecommendationService) RestoreRecommendation(ctx context.Context, id uint) error {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.RestoreRecommendation")
	defer span.End()
	rec, err := s.loadRecommendation(ctx, id)
	if err != nil {
		return err
	}
	candID, err := s.resolveRecommendationCandidate(ctx, rec)
	if err != nil {
		return err
	}
	now := time.Now()
	res := s.db.WithContext(ctx).Model(&models.CandidatePreference{}).
		Where("candidate_id = ?", candID).
		Updates(map[string]any{"excluded_at": nil, "snoozed_until": nil, "updated_at": now})
	return res.Error
}

// ── 历史视图（GET /api/discovery/recommendations?scope=history）──

// RecommendationHistoryEntry 是历史区单条（字段名对齐前端 types/discovery.ts 的
// DiscoveryHistoryItem / api HistoryPayload：id/name/status/snoozed_until/
// last_selected_at/llm_reason）。status ∈ accepted|expired|snoozed|excluded；
// 自动过期（expired）不得携带「不感兴趣」语义（snoozed_until 仅 snoozed 有值）。
type RecommendationHistoryEntry struct {
	ID             uint       `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	SnoozedUntil   *time.Time `json:"snoozed_until"`
	LastSelectedAt *time.Time `json:"last_selected_at"`
	Reason         string     `json:"llm_reason"`
}

// 历史状态常量（与前端 HistoryEntryStatus 的四种后端下发值一致）。
const (
	HistoryStatusAccepted = "accepted"
	HistoryStatusExpired  = "expired"
	HistoryStatusSnoozed  = "snoozed"
	HistoryStatusExcluded = "excluded"
)

// GetRecommendationHistory 聚合推荐历史：订阅（accepted）、长期排除（excluded）、
// 冷却中（snoozed，带 snoozed_until）、自动过期（expired：pending 已到期 / dismissed
// 历史行 / 迁移 legacy 行）。活跃未到期 pending 卡不进历史（仍在默认列表）。候选级
// 的 snoozed/excluded 状态对同候选历史行去重（保留最新一行，避免重复条目）。
// 到期判据 now >= expires_at（排他）；自动过期绝不写 dismissed_at、不改变推荐资格。
func (s *RecommendationService) GetRecommendationHistory(ctx context.Context) ([]RecommendationHistoryEntry, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.GetRecommendationHistory")
	defer span.End()
	var recs []models.FeedRecommendation
	if err := s.db.WithContext(ctx).Preload("Route").Order("id DESC").Find(&recs).Error; err != nil {
		return nil, err
	}

	// 批量取候选偏好与候选资料（名称兜底），避免 N+1。
	candIDs := make([]uint, 0, len(recs))
	for _, r := range recs {
		if r.CandidateID != nil && *r.CandidateID != 0 {
			candIDs = append(candIDs, *r.CandidateID)
		}
	}
	prefByCand := map[uint]models.CandidatePreference{}
	candByID := map[uint]models.FeedCandidate{}
	if len(candIDs) > 0 {
		var prefs []models.CandidatePreference
		if err := s.db.WithContext(ctx).Where("candidate_id IN ?", candIDs).Find(&prefs).Error; err != nil {
			return nil, err
		}
		for _, p := range prefs {
			prefByCand[p.CandidateID] = p
		}
		var cands []models.FeedCandidate
		if err := s.db.WithContext(ctx).Where("id IN ?", candIDs).Find(&cands).Error; err != nil {
			return nil, err
		}
		for _, c := range cands {
			candByID[c.ID] = c
		}
	}

	now := time.Now()
	emittedPrefCand := map[uint]struct{}{} // 候选级 snoozed/excluded 去重
	out := make([]RecommendationHistoryEntry, 0, len(recs))
	for _, rec := range recs {
		var pref *models.CandidatePreference
		if rec.CandidateID != nil {
			if p, ok := prefByCand[*rec.CandidateID]; ok {
				pref = &p
			}
		}
		status, ok := classifyHistoryEntry(rec, pref, now)
		if !ok {
			continue
		}
		if (status == HistoryStatusSnoozed || status == HistoryStatusExcluded) && rec.CandidateID != nil {
			if _, dup := emittedPrefCand[*rec.CandidateID]; dup {
				continue
			}
			emittedPrefCand[*rec.CandidateID] = struct{}{}
		}
		var cand *models.FeedCandidate
		if rec.CandidateID != nil {
			if c, ok := candByID[*rec.CandidateID]; ok {
				cand = &c
			}
		}
		entry := RecommendationHistoryEntry{
			ID: rec.ID, Name: historyEntryName(rec, cand), Status: status,
			LastSelectedAt: rec.LastSelectedAt, Reason: rec.LLMReason,
		}
		if status == HistoryStatusSnoozed && pref != nil {
			entry.SnoozedUntil = pref.SnoozedUntil
		}
		out = append(out, entry)
	}
	// 稳定排序：最近入选优先，其次 id 降序（无入选时间的老行靠后但不丢）。
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].LastSelectedAt, out[j].LastSelectedAt
		switch {
		case ti == nil && tj == nil:
			return out[i].ID > out[j].ID
		case ti == nil:
			return false
		case tj == nil:
			return true
		default:
			if ti.Equal(*tj) {
				return out[i].ID > out[j].ID
			}
			return ti.After(*tj)
		}
	})
	return out, nil
}

// classifyHistoryEntry 判定推荐行的历史状态；ok=false 表示该行不进历史
// （活跃未到期 pending / 未知状态）。
func classifyHistoryEntry(rec models.FeedRecommendation, pref *models.CandidatePreference, now time.Time) (string, bool) {
	if rec.Status == "accepted" {
		return HistoryStatusAccepted, true
	}
	if pref != nil {
		if pref.ExcludedAt != nil {
			return HistoryStatusExcluded, true
		}
		if pref.SnoozedUntil != nil && now.Before(*pref.SnoozedUntil) {
			return HistoryStatusSnoozed, true
		}
	}
	switch rec.Status {
	case "dismissed", "legacy":
		return HistoryStatusExpired, true // 旧 dismissed / 迁移 legacy 行归自动过期（只读）
	case "pending":
		if rec.ExpiresAt != nil && !now.Before(*rec.ExpiresAt) {
			return HistoryStatusExpired, true
		}
		return "", false
	default:
		return "", false
	}
}

// historyEntryName 解析历史条目展示名（有效名称优先，缺失返回空串由前端兜底）。
func historyEntryName(rec models.FeedRecommendation, cand *models.FeedCandidate) string {
	if rec.Route != nil {
		if n := strings.TrimSpace(rec.Route.Name); n != "" {
			return n
		}
		if rec.Route.Path != "" {
			return rec.Route.Namespace + rec.Route.Path
		}
	}
	if cand != nil {
		if n, ok := cand.ManualMetadata[ManualFieldName].(string); ok && strings.TrimSpace(n) != "" {
			return strings.TrimSpace(n)
		}
		if cand.FeedURL != nil && strings.TrimSpace(*cand.FeedURL) != "" {
			return strings.TrimSpace(*cand.FeedURL)
		}
	}
	return ""
}
