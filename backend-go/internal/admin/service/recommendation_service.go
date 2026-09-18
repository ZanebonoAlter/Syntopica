package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/safefetch"
	"syntopica-backend/internal/platform/tracing"
	readersvc "syntopica-backend/internal/reader/service"
)

// ── 订阅源推荐（design D5/D6，feed-discovery spec）──
//
// 粗筛（pgvector route_embeddings <=> preference_vectors）保留在本文件，供
// DiscoveryRunService 的召回批次器复用；精排/落库自 4.1 起改走 run 原子发布路径
// （discovery_run_service.go：严格精排 + 单短事务发布，不再全候选落库）。
// recommendation_hash = route_id+board_id（不含 source）：qa 与 manual_refresh
// 共享幂等池。排除规则（D5/B）：broken / 已 accepted 的 route /
// candidate_preferences 冷却或长期排除（跨 source 权威，4.4 起取代旧 dismiss hash 池）/
// usable_directly 且 feeds.url 已存在。

// RecommendationSource 枚举 feed_recommendations.source。
const (
	RecommendationSourceManualRefresh = "manual_refresh"
	RecommendationSourceQA            = "qa"
)

// RecommendationService 实现推荐列表查询、状态机与订阅；生成编排在
// DiscoveryRunService（4.1 起 ask/refresh 都走 run 原子发布）。
type RecommendationService struct {
	db        *gorm.DB
	router    *airouter.Router // 传递给 run 编排（精排 LLM + 问答 embedding）
	prefSvc   *PreferenceProfileService
	paramOpts *RouteParamOptionService // 路由参数可选值字典（注入 recommendation 响应）
	svcMu     sync.Mutex               // 保护 runSvc 惰性构造（Medium 6：并发首调不双重构造/竞争）
	runSvc    *DiscoveryRunService     // 惰性构造，避免与 NewDiscoveryRunService 循环建
}

// NewRecommendationService 构造。prefSvc 为 nil 时内部按需创建。
func NewRecommendationService(db *gorm.DB, router *airouter.Router, prefSvc *PreferenceProfileService) *RecommendationService {
	if prefSvc == nil {
		prefSvc = NewPreferenceProfileService(db)
	}
	return &RecommendationService{db: db, router: router, prefSvc: prefSvc, paramOpts: NewRouteParamOptionService(db)}
}

// runService 惰性返回 run 编排服务（首调构造）。并发正确性靠「锁内 check-and-set」：
// 构造、赋值（发布）、读取全在同一把 mu 临界区内完成，不存在双重构造，也不存在
// 「半初始化对象逃逸」（NewDiscoveryRunService 返回前已完全构造，指针只在锁内发布）。
// 注意：此处**不能**改成 lock-free 的 double-checked（先无锁读 s.runSvc 再进锁）——
// 普通指针字段的无锁读与锁内写本身即数据竞争，要 DCL 必须把 runSvc 换成
// atomic.Pointer；当前调用只在 ask/refresh 等重路径，锁开销可忽略，保持互斥最简。
func (s *RecommendationService) runService() *DiscoveryRunService {
	s.svcMu.Lock()
	defer s.svcMu.Unlock()
	if s.runSvc == nil {
		s.runSvc = NewDiscoveryRunService(s.db, s.router, s.prefSvc)
	}
	return s.runSvc
}

// RefreshSummary 描述一轮推荐刷新的产出。
type RefreshSummary struct {
	RunID           uint `json:"run_id"` // 4.1：本轮 run 账本行（前端轮询 run 详情用）
	Candidates      int  `json:"candidates"`
	Inserted        int  `json:"inserted"`
	Skipped         int  `json:"skipped"`          // hash 已 pending（本轮更新而非新建）
	CooldownBlocked int  `json:"cooldown_blocked"` // dismiss 冷却期内
}

// RefreshRecommendations 手动刷新：一次刷新 = 一个 run（粗筛 → 分批严格精排 → 原子发布）。
func (s *RecommendationService) RefreshRecommendations(ctx context.Context) (*RefreshSummary, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.RefreshRecommendations")
	defer span.End()
	return s.runService().Refresh(ctx)
}

// candidateRow 粗筛候选（统一候选身份：原生 rss 与 rsshub 共用）。
// CandidateID 是精排协议 id / 跨路去重 / 幂等 hash 的身份（feed_candidates.id）；
// RouteID 仅 rsshub 候选有值（原生 rss 为 0，其订阅地址在 Example/FeedURL）。
// Kind 用于资格过滤分流：rsshub 看 route status，原生看 candidate_availability。
type candidateRow struct {
	CandidateID        uint
	RouteID            uint
	Kind               string
	Namespace          string
	Path               string
	Name               string
	Description        string
	Example            string
	UsableDirectly     bool
	RequiresParameters bool
	Parameters         string
	BoardID            *uint
	Distance           float64
}

// （4.3 起：旧 route_embeddings 粗筛（coarseFilterByVector/coarseFilterByVectorBoard）
// 已由 discovery_recall.go 的 searchCandidates（candidate_embeddings × feed_candidates）
// 取代并删除；route_embeddings 仅作迁移输入/回滚资料，不再参与新召回。已订阅
// feeds.url 去重也已内联进 searchCandidates 的资格过滤 SQL（先于 top-N 截断）。）

// RecommendationCard 是推荐卡片视图（含路由元数据）。
type RecommendationCard struct {
	models.FeedRecommendation
	RouteNamespace     string `json:"route_namespace"`
	RoutePath          string `json:"route_path"`
	RouteName          string `json:"route_name"`
	RouteExample       string `json:"route_example"`
	UsableDirectly     bool   `json:"usable_directly"`
	RequiresParameters bool   `json:"requires_parameters"`
	Parameters         string `json:"parameters"`
	RouteStatus        string `json:"route_status"`
	BoardLabel         string `json:"board_label"`
	// ParamOptions 按参数名分组的可选值字典（feed-param-options D7）。必为非 nil map：
	// 无字典数据时序列化为 {}（向后兼容），永不为 null。
	ParamOptions map[string][]ParamOption `json:"param_options"`
}

// GetRecommendations 返回推荐卡片列表（默认 pending）。pending 列表只返回未到期
// （expires_at 为空或 now < expires_at，D5 到期判据 now >= expires_at 排他）且未被
// candidate_preferences 冷却/长期排除的卡；到期卡自动转入历史视图，**不写
// dismissed_at**（自动过期 ≠ 拒绝）。
func (s *RecommendationService) GetRecommendations(ctx context.Context, status string) ([]RecommendationCard, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.GetRecommendations")
	defer span.End()
	if status == "" {
		status = "pending"
	}
	var recs []models.FeedRecommendation
	q := s.db.WithContext(ctx).
		Preload("Route").Preload("Board").
		Where("status = ?", status)
	if status == "pending" {
		now := time.Now()
		q = q.Where("(expires_at IS NULL OR expires_at > ?)", now).
			Where(`NOT EXISTS (
				SELECT 1 FROM candidate_preferences cp
				WHERE cp.candidate_id = feed_recommendations.candidate_id
				  AND (cp.excluded_at IS NOT NULL
				       OR (cp.snoozed_until IS NOT NULL AND cp.snoozed_until > ?)))`, now)
	}
	err := q.Order("created_at DESC").Find(&recs).Error
	if err != nil {
		return nil, err
	}
	cards := make([]RecommendationCard, 0, len(recs))
	for _, r := range recs {
		card := RecommendationCard{FeedRecommendation: r}
		if r.Route != nil {
			card.RouteNamespace = r.Route.Namespace
			card.RoutePath = r.Route.Path
			card.RouteName = r.Route.Name
			card.RouteExample = r.Route.Example
			card.UsableDirectly = r.Route.UsableDirectly
			card.RequiresParameters = r.Route.RequiresParameters
			card.Parameters = r.Route.Parameters
			card.RouteStatus = r.Route.Status
		}
		if r.Board != nil {
			card.BoardLabel = r.Board.Label
		}
		cards = append(cards, card)
	}
	s.attachParamOptions(ctx, cards)
	return cards, nil
}

// attachParamOptions 批量注入 param_options 到卡片（一次 IN 查询，禁 N+1，design T3）。
// 每张卡 ParamOptions 必为非 nil map：无字典数据时为空 map（JSON 序列化为 {}，向后兼容）。
func (s *RecommendationService) attachParamOptions(ctx context.Context, cards []RecommendationCard) {
	for i := range cards {
		cards[i].ParamOptions = map[string][]ParamOption{} // 兜底空 map
	}
	if len(cards) == 0 {
		return
	}
	ids := make([]uint, 0, len(cards))
	for _, c := range cards {
		if c.RouteID != 0 {
			ids = append(ids, c.RouteID)
		}
	}
	opts, err := s.paramOpts.ListByRouteIDs(ctx, ids)
	if err != nil {
		logging.Warnf("attachParamOptions: load route param options failed: %v", err)
		return // 查询失败时保留兜底空 map，不破坏响应
	}
	grouped := GroupByRouteAndParam(opts)
	for i := range cards {
		if inner, ok := grouped[cards[i].RouteID]; ok {
			cards[i].ParamOptions = inner
		}
	}
}

// AcceptRecommendation 接受推荐：原生 RSS 用候选规范化地址，RSSHub 填参拼最终地址；
// 两者都经共享建源服务（design D9）安全验证（私网/loopback/云元数据默认拒绝、重定向
// 逐跳校验、仅可解析 RSS/Atom 才算过），验证通过后在**同一短事务**内建/复用 Feed 并
// 标记 accepted + accepted_feed_id。验证失败不建源、不标 accepted，输入保留可重试；
// 重复接受同有效地址复用同一 Feed（幂等，不产生重复订阅）。RSSHub 最终 URL 同样受
// 500 rune 上限约束，超长就地拒绝。
func (s *RecommendationService) AcceptRecommendation(ctx context.Context, id uint, categoryID *uint, params map[string]string) (*models.Feed, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "RecommendationService.AcceptRecommendation")
	defer span.End()
	var rec models.FeedRecommendation
	if err := s.db.WithContext(ctx).Preload("Route").First(&rec, id).Error; err != nil {
		return nil, err
	}
	// 幂等：已接受的推荐直接返回既有订阅，不重复验证/建源/标记。
	if rec.Status == "accepted" {
		if rec.AcceptedFeedID == nil {
			return nil, fmt.Errorf("recommendation %d accepted without feed", id)
		}
		var feed models.Feed
		if err := s.db.WithContext(ctx).First(&feed, *rec.AcceptedFeedID).Error; err != nil {
			return nil, fmt.Errorf("recommendation %d accepted feed unavailable: %w", id, err)
		}
		return &feed, nil
	}
	if rec.Status != "pending" {
		return nil, fmt.Errorf("recommendation %d not pending (status=%s)", id, rec.Status)
	}

	rawURL, title, accessScope, err := s.resolveAcceptTarget(ctx, &rec, params)
	if err != nil {
		return nil, err
	}

	feedSvc := readersvc.NewFeedCreateService(s.db)
	// 私网授权接线（Medium 7）：private_allowed 候选只放行其端点解析到的 IP（/32 或 /128），
	// 不是全局关闭 SSRF；safefetch 仍逐跳重解析并校验。
	fetchOpts := safefetch.Options{}
	if accessScope == "private_allowed" {
		ips, aerr := allowedIPsForEndpoint(ctx, rawURL)
		if aerr != nil {
			return nil, fmt.Errorf("resolve private endpoint for accept: %w", aerr)
		}
		fetchOpts.AllowedIPs = ips
	}
	normalized, err := feedSvc.VerifySubscriptionURLWithOptions(ctx, rawURL, fetchOpts)
	if err != nil {
		return nil, err // 验证失败：不建源、不标 accepted（可重试）
	}

	var feed *models.Feed
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		f, _, txErr := feedSvc.CreateOrReuseFeed(tx, normalized, readersvc.FeedCreateOptions{
			Title: title, CategoryID: categoryID,
		})
		if txErr != nil {
			return fmt.Errorf("create feed: %w", txErr)
		}
		updated, txErr := markRecommendationAccepted(tx, rec.ID, f.ID)
		if txErr != nil {
			return txErr
		}
		if !updated {
			// 并发 accept 已抢先标记（WHERE status='pending' 影响行数 0）——幂等返回其已关联 feed，
			// 不重复标 accepted、不报错（Medium 11）。
			var fresh models.FeedRecommendation
			if err := tx.First(&fresh, rec.ID).Error; err != nil {
				return err
			}
			if fresh.Status != "accepted" || fresh.AcceptedFeedID == nil {
				return fmt.Errorf("recommendation %d concurrent accept left no accepted feed", rec.ID)
			}
			var existing models.Feed
			if err := tx.First(&existing, *fresh.AcceptedFeedID).Error; err != nil {
				return fmt.Errorf("recommendation %d accepted feed unavailable: %w", rec.ID, err)
			}
			feed = &existing
			return nil
		}
		feed = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return feed, nil
}

// resolveAcceptTarget 解析订阅目标地址、展示标题与候选授权范围：候选 kind=rss 用候选
// 规范化 feed_url；kind=rsshub 用 route + 填参按既有 buildFeedURL 拼装（参数可选值字典
// 优先由既有入参规则决定，此处不另起一套）；最终地址长度上限交由共享建源服务统一把关。
func (s *RecommendationService) resolveAcceptTarget(ctx context.Context, rec *models.FeedRecommendation, params map[string]string) (string, string, string, error) {
	candID, err := s.resolveRecommendationCandidate(ctx, rec)
	if err != nil {
		return "", "", "", err
	}
	var cand models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&cand, candID).Error; err != nil {
		return "", "", "", fmt.Errorf("load candidate %d: %w", candID, err)
	}
	if cand.Kind == "rss" {
		if cand.FeedURL == nil || strings.TrimSpace(*cand.FeedURL) == "" {
			return "", "", "", fmt.Errorf("candidate %d has no feed url", candID)
		}
		return *cand.FeedURL, EffectiveMetadata(cand.ManualMetadata, "", "", "", "").Name, cand.AccessScope, nil
	}
	if rec.Route == nil {
		return "", "", "", fmt.Errorf("recommendation %d has no route", rec.ID)
	}
	feedURL := buildFeedURL(rec.Route, params, resolveRSSHubBaseURL(s.db))
	if feedURL == "" {
		return "", "", "", fmt.Errorf("cannot resolve feed url for route %s", rec.Route.Path)
	}
	return feedURL, rec.Route.Name, cand.AccessScope, nil
}

// markRecommendationAccepted 在给定事务内标记推荐 accepted 并关联 feed_id；只更新
// status='pending' 的行（Medium 11：并发 accept 只有一个赢家，不覆盖别人的 accepted_feed_id）。
// 返回 updated=false 表示影响行数 0（已被并发 accept 抢先或已非 pending），调用方据 fresh
// 行幂等复用既有 feed。只更新必要三列而非 Save：避免覆盖迁移/并发写入的其它字段。
func markRecommendationAccepted(tx *gorm.DB, recID, feedID uint) (bool, error) {
	res := tx.Model(&models.FeedRecommendation{}).
		Where("id = ? AND status = ?", recID, "pending").
		Updates(map[string]any{
			"status": "accepted", "accepted_feed_id": feedID, "updated_at": time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// （旧的 DismissRecommendation（写 status=dismissed + 30 天 hash 冷却池）已于 4.4
// 退役：暂时不看/长期排除/恢复统一走 recommendation_lifecycle.go 的
// SnoozeRecommendation / ExcludeRecommendation / RestoreRecommendation，
// 写 candidate_preferences 权威，pending 行状态不变。）

// buildFeedURL 拼接受订阅的 feed URL。
// 用户填了任一参数值 → 一律模板填参（usable_directly 的 example 只是缺省形态，不能压过
// 用户显式填参，否则填参被无视、URL 恒为 example）；未填参时 usable_directly 用
// baseURL + example（或 namespace+path），requires_parameters 用 params 填 path 参数。
func buildFeedURL(r *models.RSSHubRoute, params map[string]string, baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	hasParams := false
	for _, val := range params {
		if strings.TrimSpace(val) != "" {
			hasParams = true
			break
		}
	}
	if !hasParams && r.UsableDirectly {
		if r.Example != "" {
			return base + r.Example
		}
		return base + "/" + r.Namespace + r.Path
	}
	// 模板填参：先剥 {regex} 约束（暴露参数名），再替换 :name?（可选标记跟随参数名一起
	// 替换，防值尾残留 `?`），后替换裸 :name；未提供的可选段 strip，剩 : 即未填必填。
	u := "/" + r.Namespace + r.Path
	u = stripBraceConstraints(u)
	for name, val := range params {
		if strings.TrimSpace(val) == "" {
			continue // 空值视作未提供：必填段残留报错、可选段 strip 丢弃
		}
		escaped := url.PathEscape(val)
		u = strings.ReplaceAll(u, ":"+name+"?", escaped)
		u = strings.ReplaceAll(u, ":"+name, escaped)
	}
	// 去掉剩余可选参数段 :x?。
	u = stripOptionalParams(u)
	if !strings.Contains(u, ":") {
		return base + u
	}
	return "" // 仍有未填必填参数
}

// stripOptionalParams 去掉 :param? 残留段（{regex} 已由 stripBraceConstraints 先行剥离）。
func stripOptionalParams(url string) string {
	// 去可选参数段 :xxx?（整段连同前导 /）
	out := []string{}
	for _, seg := range strings.Split(url, "/") {
		if strings.HasPrefix(seg, ":") && strings.HasSuffix(seg, "?") {
			continue
		}
		out = append(out, seg)
	}
	return strings.Join(out, "/")
}

// stripBraceConstraints 去掉路径中的 {regex} 正则约束片段（暴露参数名供替换；
// 与 stripOptionalParams 配对：先剥约束再替换/丢弃可选段）。
func stripBraceConstraints(url string) string {
	for strings.Contains(url, "{") {
		i := strings.Index(url, "{")
		j := strings.Index(url, "}")
		if j < i {
			break
		}
		url = url[:i] + url[j+1:]
	}
	return url
}
