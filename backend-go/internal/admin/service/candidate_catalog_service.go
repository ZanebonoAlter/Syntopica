package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 候选源库 CRUD（improve-discovery-recommendations，design D1/D9）──
//
// feed_candidates 表达「可被推荐的订阅候选」而非订阅本身：本服务所有写路径都
// MUST NOT 创建 Feed、抓取文章或发任何网络请求（spec C1：入库 ≠ 订阅）。
// RSSHub 候选由目录同步/迁移产生，CreateCandidate 只接受 kind=rss 手动新增。
// 推荐启停（recommendation_enabled）与长期排除（candidate_preferences）正交：
// 本服务不触碰 candidate_preferences——排除/冷却是另一权威（design D2）。

// 候选库类型化错误 code（design D9：错误响应统一可识别 code）。
const (
	CandidateErrorCodeValidation = "validation"
	CandidateErrorCodeConflict   = "conflict"
	CandidateErrorCodeNotFound   = "not_found"
)

// CandidateError 候选库类型化错误：handler 按 Code 映射 HTTP 状态；
// conflict 携带已存在条目 ExistingID，供前端展示「已存在入口」。
type CandidateError struct {
	Code       string
	Message    string
	ExistingID uint
}

func (e *CandidateError) Error() string { return e.Message }

func newCandidateValidationError(msg string) error {
	return &CandidateError{Code: CandidateErrorCodeValidation, Message: msg}
}

// asCandidateValidationError 把纯函数校验错误包装为 validation 类型错误。
func asCandidateValidationError(err error) error {
	return &CandidateError{Code: CandidateErrorCodeValidation, Message: err.Error()}
}

func newCandidateNotFoundError(id uint) error {
	return &CandidateError{Code: CandidateErrorCodeNotFound, Message: fmt.Sprintf("candidate %d not found", id)}
}

func newCandidateConflictError(existingID uint, msg string) error {
	return &CandidateError{Code: CandidateErrorCodeConflict, Message: msg, ExistingID: existingID}
}

// 候选库列表分页与关键词约束（design D9：分页默认 30、上限 100，query 限 500 runes）。
const (
	CandidateListDefaultPageSize = 30
	CandidateListMaxPageSize     = 100
	CandidateKeywordMaxRunes     = 500
)

// CandidateCatalogService 候选源库数据访问与业务逻辑。
type CandidateCatalogService struct {
	db *gorm.DB
}

// NewCandidateCatalogService 构造。
func NewCandidateCatalogService(db *gorm.DB) *CandidateCatalogService {
	return &CandidateCatalogService{db: db}
}

// CandidateCreateInput 手动新增候选请求体（仅 kind=rss；rsshub 候选由目录同步产生）。
//
// RecommendationEnabled 指针语义：nil（缺省）= 参与推荐 true（新建默认入库即可推荐）；
// 显式 false = 建条目但暂不参与推荐（与编辑路径 CandidateUpdateInput 同名 tag 对齐，
// 供表单「参与推荐」开关在新建时就能落库）。
type CandidateCreateInput struct {
	Kind                  string `json:"kind"`                   // 可选，空 = rss；仅接受 rss
	Name                  string `json:"name"`                   // 必填，非纯空白，≤200 runes
	FeedURL               string `json:"feed_url"`               // 必填，http/https，≤500 runes
	Description           string `json:"description"`            // 可选，≤4000 runes
	Language              string `json:"language"`               // 可选，≤50 runes
	Region                string `json:"region"`                 // 可选，≤50 runes
	RecommendationEnabled *bool  `json:"recommendation_enabled"` // 可选，nil = true
}

// CreateCandidate 手动新增原生 RSS 候选：校验 → 规范化 → 冲突检查 → 落库。
// 冲突 = stable_key 相同，或 kind=rss 且 canonical_key 相同（design D1：同一直接地址
// 重复保存报告重复，不建第二条）。本方法不创建 Feed、不抓取、不发网络请求。
func (s *CandidateCatalogService) CreateCandidate(ctx context.Context, in CandidateCreateInput) (*CandidateView, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = "rss"
	}
	if kind != "rss" {
		return nil, newCandidateValidationError(
			fmt.Sprintf("kind %q not supported for manual create: only rss is accepted (rsshub candidates come from catalog sync)", kind))
	}

	// 字段校验（candidate_identity 纯函数，rune 计数限长）。
	if err := ValidateManualField(ManualFieldName, in.Name); err != nil {
		return nil, asCandidateValidationError(err)
	}
	for field, val := range map[string]string{
		ManualFieldDescription: in.Description,
		ManualFieldLanguage:    in.Language,
		ManualFieldRegion:      in.Region,
	} {
		if val == "" {
			continue // 可选字段空 = 不写人工覆盖键
		}
		if err := ValidateManualField(field, val); err != nil {
			return nil, asCandidateValidationError(err)
		}
	}

	// 规范化 URL + 稳定身份。
	normalized, err := NormalizeRSSURL(in.FeedURL)
	if err != nil {
		return nil, asCandidateValidationError(err)
	}
	stableKey, err := BuildRSSStableKey(in.FeedURL)
	if err != nil {
		return nil, asCandidateValidationError(err)
	}

	// 冲突检查：stable_key 相同，或 kind=rss 且 canonical_key 相同。
	var existing models.FeedCandidate
	err = s.db.WithContext(ctx).
		Where("stable_key = ?", stableKey).
		Or("kind = ? AND canonical_key = ?", "rss", *normalized).
		First(&existing).Error
	if err == nil {
		return nil, newCandidateConflictError(existing.ID,
			fmt.Sprintf("candidate already exists for feed url %s (id=%d)", *normalized, existing.ID))
	}
	if !isNotFound(err) {
		return nil, err
	}

	// 参与推荐默认开启（nil/缺省 true），显式 false 则建条目但不进推荐池。
	enabled := true
	if in.RecommendationEnabled != nil {
		enabled = *in.RecommendationEnabled
	}
	cand := models.FeedCandidate{
		StableKey:             stableKey,
		Kind:                  "rss",
		RouteID:               nil,
		FeedURL:               normalized,
		CanonicalKey:          *normalized,
		ManualMetadata:        buildManualMetadata(in.Name, in.Description, in.Language, in.Region),
		RecommendationEnabled: &enabled,
		AccessScope:           "public",
		Revision:              1,
	}
	if err := s.db.WithContext(ctx).Create(&cand).Error; err != nil {
		return nil, err
	}
	return s.buildView(ctx, cand)
}

// CandidateUpdateInput 候选编辑请求体。指针语义：nil = 不修改；非 nil 空串 = 清空覆盖
// （删键，回退上游值）；非空 = 覆盖并校验。Revision 为乐观锁：传客户端所见版本，
// 与库内不一致返回 conflict；缺省 0 = 不校验版本（仍自增）。
//
// FeedURL 仅对 kind=rss 生效（spec feed-candidate-catalog：手动新增和编辑原生 RSS）：
// RSSHub 的路由地址来自上游 rsshub_routes，属只读资料，传了即 validation。
type CandidateUpdateInput struct {
	Name                  *string `json:"name"`
	Description           *string `json:"description"`
	Language              *string `json:"language"`
	Region                *string `json:"region"`
	FeedURL               *string `json:"feed_url"`
	RecommendationEnabled *bool   `json:"recommendation_enabled"`
	Revision              uint    `json:"revision"`
}

// UpdateCandidate 编辑候选：manual 元数据四字段 + 原生 RSS 地址（仅 rss）+ 推荐启停。
// 乐观锁 UPDATE ... WHERE revision=旧值，影响行数 0 返回 conflict（旧请求不得覆盖新资料）。
// 本方法不触碰 candidate_preferences（排除/冷却是另一权威，design D2）。
func (s *CandidateCatalogService) UpdateCandidate(ctx context.Context, id uint, in CandidateUpdateInput) (*CandidateView, error) {
	var cand models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&cand, id).Error; err != nil {
		if isNotFound(err) {
			return nil, newCandidateNotFoundError(id)
		}
		return nil, err
	}

	// 校验 manual 字段并构造新 manual map（空串 = 删键回退上游）。
	manual := models.MetadataMap{}
	for k, v := range cand.ManualMetadata {
		manual[k] = v
	}
	touched := false
	if in.Name != nil {
		touched = true
		if *in.Name == "" {
			// rss 候选没有上游名称可回退，清空名称会产生无名条目，拒绝。
			if cand.Kind == "rss" {
				return nil, newCandidateValidationError("name cannot be cleared for an rss candidate (no upstream name to fall back to)")
			}
			delete(manual, ManualFieldName)
		} else {
			if err := ValidateManualField(ManualFieldName, *in.Name); err != nil {
				return nil, asCandidateValidationError(err)
			}
			manual[ManualFieldName] = *in.Name
		}
	}
	for field, ptr := range map[string]*string{
		ManualFieldDescription: in.Description,
		ManualFieldLanguage:    in.Language,
		ManualFieldRegion:      in.Region,
	} {
		if ptr == nil {
			continue
		}
		touched = true
		if *ptr == "" {
			delete(manual, field)
			continue
		}
		if err := ValidateManualField(field, *ptr); err != nil {
			return nil, asCandidateValidationError(err)
		}
		manual[field] = *ptr
	}

	updates := map[string]any{"manual_metadata": manual}

	// 原生 RSS 地址编辑：rsshub 路由地址只读（上游表为准，不允许本地改写）。
	if in.FeedURL != nil {
		if cand.Kind != "rss" {
			return nil, newCandidateValidationError(
				"route address is read-only: feed_url can only be edited for rss candidates")
		}
		normalized, nerr := NormalizeRSSURL(*in.FeedURL)
		if nerr != nil {
			return nil, asCandidateValidationError(nerr)
		}
		stableKey, kerr := BuildRSSStableKey(*in.FeedURL)
		if kerr != nil {
			return nil, asCandidateValidationError(kerr)
		}
		// 改地址后身份必须仍唯一：撞其它条目的 stable_key，或撞另一条同规范化地址的 rss 候选
		// → conflict 携 ExistingID（与 CreateCandidate 同口径，不静默合并、不建重影）。
		var clash models.FeedCandidate
		cerr := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).
			Where("id <> ?", id).
			Where("stable_key = ? OR (kind = ? AND canonical_key = ?)", stableKey, "rss", *normalized).
			First(&clash).Error
		if cerr == nil {
			return nil, newCandidateConflictError(clash.ID,
				fmt.Sprintf("candidate already exists for feed url %s (id=%d)", *normalized, clash.ID))
		}
		if !isNotFound(cerr) {
			return nil, cerr
		}
		touched = true
		updates["stable_key"] = stableKey
		updates["feed_url"] = *normalized
		updates["canonical_key"] = *normalized
	}

	if in.RecommendationEnabled != nil {
		updates["recommendation_enabled"] = *in.RecommendationEnabled
	}
	if !touched && in.RecommendationEnabled == nil {
		// 空补丁：不产生写放大，直接返回当前视图。
		return s.buildView(ctx, cand)
	}

	updates["revision"] = gorm.Expr("revision + 1")
	q := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).Where("id = ?", id)
	if in.Revision != 0 {
		q = q.Where("revision = ?", in.Revision)
	}
	res := q.Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// 不存在或版本不一致：复查区分 not_found 与 revision conflict。
		var cnt int64
		if err := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).Where("id = ?", id).Count(&cnt).Error; err != nil {
			return nil, err
		}
		if cnt == 0 {
			return nil, newCandidateNotFoundError(id)
		}
		return nil, newCandidateConflictError(id,
			fmt.Sprintf("candidate %d was modified concurrently (revision mismatch, expected %d)", id, in.Revision))
	}

	var updated models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&updated, id).Error; err != nil {
		return nil, err
	}
	return s.buildView(ctx, updated)
}

// CandidateListQuery 列表查询参数（design D9：分页默认 30、上限 100，query 限 500 runes）。
type CandidateListQuery struct {
	Page                  int
	PageSize              int
	Keyword               string // 命中 name/description/feed_url（不区分 ASCII 大小写）
	Kind                  string // 空 = 全部；rsshub | rss
	RecommendationEnabled *bool  // 参与推荐状态筛选
}

// CandidateListResult 分页列表结果。
type CandidateListResult struct {
	Items []CandidateView `json:"items"`
	Total int64           `json:"total"`
}

// CandidateView 候选对外视图：行数据 + 有效展示字段（人工非空覆盖 → 上游 → 缺省）+ 订阅状态。
type CandidateView struct {
	ID                    uint               `json:"id"`
	StableKey             string             `json:"stable_key"`
	Kind                  string             `json:"kind"`
	RouteID               *uint              `json:"route_id,omitempty"`
	FeedURL               *string            `json:"feed_url,omitempty"`
	CanonicalKey          string             `json:"canonical_key"`
	ManualMetadata        models.MetadataMap `json:"manual_metadata"`
	RecommendationEnabled *bool              `json:"recommendation_enabled,omitempty"`
	AccessScope           string             `json:"access_scope"`
	Revision              uint               `json:"revision"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`

	// 有效展示字段（EffectiveMetadata 计算；两者皆缺不编造资料）。
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"`
	Region      string `json:"region"`

	// Subscribed = feed_url 与 feeds.url 精确匹配存在（只读判断，不建关联）。
	Subscribed bool `json:"subscribed"`

	// 可用性（3.3 / design D7）：candidate_availability 的当前状态；无记录 = unknown
	// 「未验证」（未验证不得当作失效，也不进推荐硬过滤）。LastCheckedAt 为最近一次检查时刻，
	// 无检查记录时为 null（不伪造时间）。字段名对齐前端 types/discovery.ts。
	Availability  string     `json:"availability"`
	LastCheckedAt *time.Time `json:"last_checked_at"`

	// Route：rsshub 候选的上游原始资料（rss 候选为 nil）。
	Route *models.RSSHubRoute `json:"route,omitempty"`
}

// ListCandidates 分页列出候选：关键词命中 name/description/feed_url（含 rsshub 上游
// 名称/描述，经 LEFT JOIN rsshub_routes），筛选 kind 与参与推荐状态，按 updated_at 倒序。
func (s *CandidateCatalogService) ListCandidates(ctx context.Context, q CandidateListQuery) (*CandidateListResult, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = CandidateListDefaultPageSize
	}
	if q.PageSize > CandidateListMaxPageSize {
		q.PageSize = CandidateListMaxPageSize
	}
	if q.Kind != "" && q.Kind != "rss" && q.Kind != "rsshub" {
		return nil, newCandidateValidationError(fmt.Sprintf("invalid kind filter %q: must be rsshub or rss", q.Kind))
	}

	// 关键词截断到 500 runes（design D9）。
	keyword := strings.TrimSpace(q.Keyword)
	if utf8.RuneCountInString(keyword) > CandidateKeywordMaxRunes {
		keyword = string([]rune(keyword)[:CandidateKeywordMaxRunes])
	}

	// 过滤器构造器：Count 与 Find 各建一次，避免 GORM 语句复用污染。
	newFiltered := func() *gorm.DB {
		dq := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).
			Joins("LEFT JOIN rsshub_routes ON rsshub_routes.id = feed_candidates.route_id")
		if q.Kind != "" {
			dq = dq.Where("feed_candidates.kind = ?", q.Kind)
		}
		if q.RecommendationEnabled != nil {
			dq = dq.Where("feed_candidates.recommendation_enabled = ?", *q.RecommendationEnabled)
		}
		if keyword != "" {
			pat := "%" + escapeLikePattern(strings.ToLower(keyword)) + "%"
			dq = dq.Where(
				"(LOWER(COALESCE(feed_candidates.feed_url, '')) LIKE ? ESCAPE '\\'"+
					" OR LOWER(CAST(feed_candidates.manual_metadata AS TEXT)) LIKE ? ESCAPE '\\'"+
					" OR LOWER(COALESCE(rsshub_routes.name, '')) LIKE ? ESCAPE '\\'"+
					" OR LOWER(COALESCE(rsshub_routes.description, '')) LIKE ? ESCAPE '\\')",
				pat, pat, pat, pat)
		}
		return dq
	}

	var total int64
	if err := newFiltered().Count(&total).Error; err != nil {
		return nil, err
	}
	var cands []models.FeedCandidate
	if err := newFiltered().
		Select("feed_candidates.*").
		Order("feed_candidates.updated_at DESC, feed_candidates.id DESC").
		Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).
		Find(&cands).Error; err != nil {
		return nil, err
	}
	views, err := s.buildViews(ctx, cands)
	if err != nil {
		return nil, err
	}
	return &CandidateListResult{Items: views, Total: total}, nil
}

// GetCandidate 取单条候选；rsshub 候选附带上游 Route 原始资料。
func (s *CandidateCatalogService) GetCandidate(ctx context.Context, id uint) (*CandidateView, error) {
	var cand models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&cand, id).Error; err != nil {
		if isNotFound(err) {
			return nil, newCandidateNotFoundError(id)
		}
		return nil, err
	}
	return s.buildView(ctx, cand)
}

// buildView 单条视图构造。
func (s *CandidateCatalogService) buildView(ctx context.Context, cand models.FeedCandidate) (*CandidateView, error) {
	views, err := s.buildViews(ctx, []models.FeedCandidate{cand})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// buildViews 批量视图构造：一次 IN 查询预载 rsshub_routes（禁 N+1），一次 IN 查询
// feeds.url 判订阅，有效字段经 EffectiveMetadata 计算。
func (s *CandidateCatalogService) buildViews(ctx context.Context, cands []models.FeedCandidate) ([]CandidateView, error) {
	if len(cands) == 0 {
		return []CandidateView{}, nil
	}

	// 预载 rsshub 上游资料。
	routeIDs := make([]uint, 0, len(cands))
	for _, c := range cands {
		if c.Kind == "rsshub" && c.RouteID != nil {
			routeIDs = append(routeIDs, *c.RouteID)
		}
	}
	routes := map[uint]models.RSSHubRoute{}
	if len(routeIDs) > 0 {
		var rows []models.RSSHubRoute
		if err := s.db.WithContext(ctx).Where("id IN ?", routeIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			routes[r.ID] = r
		}
	}

	// 订阅状态：feed_url 与 feeds.url 精确匹配（只读）。
	feedURLs := make([]string, 0, len(cands))
	for _, c := range cands {
		if c.FeedURL != nil {
			feedURLs = append(feedURLs, *c.FeedURL)
		}
	}
	subscribedSet := map[string]struct{}{}
	if len(feedURLs) > 0 {
		var urls []string
		if err := s.db.WithContext(ctx).Model(&models.Feed{}).Where("url IN ?", feedURLs).Pluck("url", &urls).Error; err != nil {
			return nil, err
		}
		for _, u := range urls {
			subscribedSet[u] = struct{}{}
		}
	}

	// 可用性状态：一次 IN 查询预载（禁 N+1）；无记录保持 unknown（未验证）。
	availabilityByCandidate := map[uint]models.CandidateAvailability{}
	candidateIDs := make([]uint, 0, len(cands))
	for _, c := range cands {
		candidateIDs = append(candidateIDs, c.ID)
	}
	var availabilityRows []models.CandidateAvailability
	if err := s.db.WithContext(ctx).Where("candidate_id IN ?", candidateIDs).Find(&availabilityRows).Error; err != nil {
		return nil, err
	}
	for _, r := range availabilityRows {
		availabilityByCandidate[r.CandidateID] = r
	}

	views := make([]CandidateView, 0, len(cands))
	for _, c := range cands {
		v := CandidateView{
			ID: c.ID, StableKey: c.StableKey, Kind: c.Kind, RouteID: c.RouteID,
			FeedURL: c.FeedURL, CanonicalKey: c.CanonicalKey,
			ManualMetadata: c.ManualMetadata, RecommendationEnabled: c.RecommendationEnabled,
			AccessScope: c.AccessScope, Revision: c.Revision,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		}
		var upstreamName, upstreamDesc string
		if c.RouteID != nil {
			if r, ok := routes[*c.RouteID]; ok {
				upstreamName = r.Name
				upstreamDesc = r.Description
				rc := r
				v.Route = &rc
			}
		}
		eff := EffectiveMetadata(c.ManualMetadata, upstreamName, upstreamDesc, "", "")
		v.Name, v.Description, v.Language, v.Region = eff.Name, eff.Description, eff.Language, eff.Region
		if c.FeedURL != nil {
			if _, ok := subscribedSet[*c.FeedURL]; ok {
				v.Subscribed = true
			}
		}
		v.Availability = AvailabilityStatusUnknown
		if row, ok := availabilityByCandidate[c.ID]; ok {
			if row.Status != "" {
				v.Availability = row.Status
			}
			v.LastCheckedAt = row.LastAttemptAt
		}
		views = append(views, v)
	}
	return views, nil
}

// buildManualMetadata 由 create 输入构造 manual map：只写非空字段（name 必填必写）。
func buildManualMetadata(name, description, language, region string) models.MetadataMap {
	m := models.MetadataMap{ManualFieldName: name}
	if description != "" {
		m[ManualFieldDescription] = description
	}
	if language != "" {
		m[ManualFieldLanguage] = language
	}
	if region != "" {
		m[ManualFieldRegion] = region
	}
	return m
}

// escapeLikePattern 转义 LIKE 通配符（%/_/反斜杠），配合 ESCAPE '\' 子句在
// PostgreSQL 与 SQLite 上行为一致。
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// ensure iface usage guard: CandidateError 实现 error（编译期断言）。
var _ error = (*CandidateError)(nil)
