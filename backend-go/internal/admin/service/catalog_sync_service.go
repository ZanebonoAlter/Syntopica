package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// ── RSSHub 路由目录同步（design D2/D3，rsshub-route-catalog spec）──
//
// 来源：自建 RSSHub 实例 GET {rsshub_base_url}/api/namespace。
// 实测响应结构：{namespace: {routes: {path: {path,name,url,maintainers,example,parameters,description}}}}
// 同步按 content_hash diff：新增/变更入库，消失的标 gone（不物理删除）。
//
// 3.4 修复的四个缺陷（spec C2「Upstream and Manual Metadata Isolation」、tasks 3.4）：
//  1. fetch 失败曾返回「空成功」，把「拉不到」等同「无变化」——现在拉取失败必须返回错误；
//  2. content_hash 曾漏掉 url/example，上游改了站点地址或示例 URL 也不会更新；
//  3. 同步曾只写 rsshub_routes，不维护统一候选实体 feed_candidates 的关联；
//  4. 解析不完整时曾静默跳过，导致未取得的路由被误标 gone——现在响应结构无法完整
//     解析即整轮失败（fail-closed），绝不把「没拉到」当成「上游删除」。
//
// 人工隔离铁律：候选已存在时同步只补 route_id 关联，绝不覆盖 manual_metadata /
// recommendation_enabled / access_scope / revision（人工说明与推荐启停以候选自身为准）。

// DefaultRSSHubBaseURL 是 dump-sanitizer 已知的自建实例（design D2）。
const DefaultRSSHubBaseURL = "https://rsshub.app"

// CatalogSyncService 同步 RSSHub 路由目录。
type CatalogSyncService struct {
	db      *gorm.DB
	baseURL string
	// fetch 可注入：默认 HTTP GET /api/namespace；测试可替换为固定数据。
	fetch func(ctx context.Context) (map[string]json.RawMessage, error)
}

// NewCatalogSyncService 构造。baseURL 空则读 rsshub_config，配置也缺省回落默认自建实例（design E）。
func NewCatalogSyncService(db *gorm.DB, baseURL string) *CatalogSyncService {
	if baseURL == "" {
		baseURL = resolveRSSHubBaseURL(db)
	}
	s := &CatalogSyncService{db: db, baseURL: baseURL}
	s.fetch = s.httpFetchNamespace
	return s
}

// CatalogSyncSummary 描述一次目录同步的产出。
//
// CandidatesCreated / CandidatesLinked 是 3.4 新增的候选联动计数：前者为本轮新建的
// RSSHub 候选，后者为已存在候选补上（或纠正）route_id 关联的条数；人工列永不计入。
type CatalogSyncSummary struct {
	Inserted          int // 新增路由
	Updated           int // content_hash 变更的路由
	Gone              int // 目录中消失、标记 gone 的路由
	Total             int // 目录总路由数
	NewToEmbed        int // 需生成 embedding 的新路由
	CandidatesCreated int // 本轮新建的候选实体
	CandidatesLinked  int // 本轮补齐 route_id 关联的既有候选
}

// SyncAll 拉取全量目录并 diff 入库（D2）。
// 拉取失败、或响应结构无法完整解析时返回错误且不写库：既有路由与候选保持原样，
// 绝不把「没拉到」当成「上游删除」（spec C2：只有明确成功且缺席才算上游删除）。
func (s *CatalogSyncService) SyncAll(ctx context.Context) (*CatalogSyncSummary, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "CatalogSyncService.SyncAll")
	defer span.End()
	raw, err := s.fetch(ctx)
	if err != nil {
		logging.Warnf("rsshub catalog sync: fetch failed, keep existing catalog: %v", err)
		return nil, fmt.Errorf("rsshub catalog sync: fetch /api/namespace: %w", err)
	}

	records, err := flattenNamespace(raw)
	if err != nil {
		logging.Warnf("rsshub catalog sync: response not fully parseable, keep existing catalog: %v", err)
		return nil, fmt.Errorf("rsshub catalog sync: parse /api/namespace: %w", err)
	}
	// HTTP 200 但全量 namespace 展开为 0 条路由 = 非法/截断的目录载荷（Medium 9 修复）。
	// 若继续按「未出现在本轮列表」推导，会把既有全部路由误标 gone（合法目录不会整轮为空）。
	// 视同解析失败整轮放弃，绝不标 gone。
	if len(records) == 0 {
		logging.Warnf("rsshub catalog sync: /api/namespace expanded to zero routes, keep existing catalog")
		return nil, fmt.Errorf("rsshub catalog sync: empty namespace payload")
	}
	summary := &CatalogSyncSummary{Total: len(records)}

	// 取现有全部路由的 content_hash（namespace+path → {id, hash, status}）。
	existing, err := s.loadExistingRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("load existing routes: %w", err)
	}
	// 预载本轮路由对应的既有候选（一次 IN 查询，禁 N+1）：新增/更新路由要保证关联，
	// 内容未变的路由也要补齐导入条目留空的 route_id（design D8：导入的 rsshub 条目
	// 「待目录同步按 namespace/path 绑定本地上游」）。
	candidates, err := s.loadCandidatesByStableKey(ctx, records)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(records))
	for _, r := range records {
		key := r.Namespace + "|" + r.Path
		seen[key] = struct{}{}
		hash := r.contentHash()
		usable, requires := ParseRouteParameters(r.Path)
		row := models.RSSHubRoute{
			Namespace:          r.Namespace,
			Path:               r.Path,
			Name:               r.Name,
			URL:                r.URL,
			Description:        r.Description,
			Parameters:         r.ParametersJSON(),
			Example:            r.Example,
			RequiresParameters: requires,
			UsableDirectly:     usable,
			ContentHash:        hash,
			Status:             "unknown",
		}
		switch cur, ok := existing[key]; {
		case !ok:
			// 新增。
			if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
				return nil, fmt.Errorf("insert route %s: %w", key, err)
			}
			summary.Inserted++
			summary.NewToEmbed++
		case cur.hash == hash && cur.status != "gone":
			// 无变化：不写路由行，但仍参与候选关联补齐（只读 row 的 id/namespace/path）。
			row.ID = cur.id
		default:
			// 内容变更：保留既有可用性结论（Low 17：Save 全量写回不得把 broken/unknown
			// 重置为初始零值）；仅从 gone 复现时清 gone 重新评估。
			row.Status = cur.status
			if cur.status == "gone" {
				row.Status = "unknown" // 复现：清除 gone
			}
			row.ID = cur.id
			if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
				return nil, fmt.Errorf("update route %s: %w", key, err)
			}
			summary.Updated++
		}
		if err := s.syncRouteCandidate(ctx, row, candidates, summary); err != nil {
			return nil, err
		}
	}

	// 消失的路由标 gone（走到这里意味着本轮拉取与解析完全成功：只有明确成功且
	// 缺席才标 gone；对应候选保留不删——订阅与人工资料照旧，route 关联仍在）。
	for key, cur := range existing {
		if _, ok := seen[key]; ok {
			continue
		}
		if cur.status == "gone" {
			continue
		}
		if err := s.db.WithContext(ctx).Model(&models.RSSHubRoute{}).
			Where("id = ?", cur.id).
			Updates(map[string]any{"status": "gone", "updated_at": time.Now()}).Error; err != nil {
			return nil, fmt.Errorf("mark gone %s: %w", key, err)
		}
		summary.Gone++
	}

	logging.Infof("rsshub catalog sync: total=%d inserted=%d updated=%d gone=%d newEmbed=%d candidatesCreated=%d candidatesLinked=%d",
		summary.Total, summary.Inserted, summary.Updated, summary.Gone, summary.NewToEmbed,
		summary.CandidatesCreated, summary.CandidatesLinked)
	return summary, nil
}

// loadCandidatesByStableKey 一次 IN 查询预载本轮路由对应的既有候选（stable_key → 行），
// 避免逐路由查询（N+1）。同一路由的 stable_key 去重后再查。
func (s *CatalogSyncService) loadCandidatesByStableKey(ctx context.Context, records []routeRecord) (map[string]models.FeedCandidate, error) {
	out := make(map[string]models.FeedCandidate)
	if len(records) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, r := range records {
		stableKey, err := BuildRSSHubStableKey(r.Namespace, r.Path)
		if err != nil {
			return nil, fmt.Errorf("build candidate stable key for route %s|%s: %w", r.Namespace, r.Path, err)
		}
		if _, dup := seen[stableKey]; dup {
			continue
		}
		seen[stableKey] = struct{}{}
		keys = append(keys, stableKey)
	}
	var rows []models.FeedCandidate
	if err := s.db.WithContext(ctx).Where("stable_key IN ?", keys).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load candidates by stable key: %w", err)
	}
	for _, c := range rows {
		out[c.StableKey] = c
	}
	return out, nil
}

// syncRouteCandidate 让本轮路由与统一候选实体（feed_candidates）保持关联
// （design D1 / spec C2 / tasks 3.4）。existing 是本轮预算的 stable_key → 候选行映射，
// 本方法只在必要时写库并把结果回填进映射（本轮内新建的候选不会重复创建）。
//
// 人工隔离铁律：候选已存在时只补 route_id 关联（导入的 rsshub 条目 RouteID 为空，
// 待同步按 namespace/path 绑定本地上游），**绝不触碰** manual_metadata /
// recommendation_enabled / access_scope / revision——人工说明、推荐启停与访问授权
// 始终以候选自身为准；用户刚停用（enabled=false）后同步不得复活，人工说明不得被
// 上游资料冲掉。并发插入用 stable_key 唯一冲突兜底（OnConflict Do Nothing + 复查），
// 重复同步幂等：已绑定同一 route_id 时不产生任何写入。
func (s *CatalogSyncService) syncRouteCandidate(ctx context.Context, route models.RSSHubRoute, existing map[string]models.FeedCandidate, summary *CatalogSyncSummary) error {
	stableKey, err := BuildRSSHubStableKey(route.Namespace, route.Path)
	if err != nil {
		return fmt.Errorf("build candidate stable key for route %s|%s: %w", route.Namespace, route.Path, err)
	}

	if cand, ok := existing[stableKey]; ok {
		if cand.RouteID != nil && *cand.RouteID == route.ID {
			return nil // 已绑定本路由：不产生写放大
		}
		if err := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).
			Where("id = ?", cand.ID).
			Update("route_id", route.ID).Error; err != nil {
			return fmt.Errorf("link candidate %s to route %d: %w", stableKey, route.ID, err)
		}
		cand.RouteID = &route.ID
		existing[stableKey] = cand
		summary.CandidatesLinked++
		return nil
	}

	enabled := true
	routeID := route.ID
	cand := models.FeedCandidate{
		StableKey:             stableKey,
		Kind:                  "rsshub",
		RouteID:               &routeID,
		CanonicalKey:          "",
		ManualMetadata:        models.MetadataMap{},
		RecommendationEnabled: &enabled,
		AccessScope:           "public",
		Revision:              1,
	}
	// 并发插入容错：stable_key 唯一冲突时保留先插入者（可能是导入或人工建档），
	// 随后只补缺失的 route_id 关联，人工列不碰。
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&cand)
	if res.Error != nil {
		return fmt.Errorf("create candidate %s: %w", stableKey, res.Error)
	}
	if res.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).
			Where("stable_key = ? AND (route_id IS NULL OR route_id <> ?)", stableKey, route.ID).
			Update("route_id", route.ID).Error; err != nil {
			return fmt.Errorf("link concurrently created candidate %s: %w", stableKey, err)
		}
		if live, loadErr := s.findCandidateByStableKey(ctx, stableKey); loadErr == nil {
			existing[stableKey] = *live
		}
		summary.CandidatesLinked++
		return nil
	}
	existing[stableKey] = cand
	summary.CandidatesCreated++
	return nil
}

// findCandidateByStableKey 按 stable_key 读回候选（并发插入冲突后的复查）。
func (s *CatalogSyncService) findCandidateByStableKey(ctx context.Context, stableKey string) (*models.FeedCandidate, error) {
	var cand models.FeedCandidate
	if err := s.db.WithContext(ctx).Where("stable_key = ?", stableKey).First(&cand).Error; err != nil {
		return nil, err
	}
	return &cand, nil
}

// existingRoute 记录现有路由的 diff 元数据。
type existingRoute struct {
	id     uint
	hash   string
	status string
}

// loadExistingRoutes 取现有全部路由 namespace+path → existingRoute。
func (s *CatalogSyncService) loadExistingRoutes(ctx context.Context) (map[string]existingRoute, error) {
	type row struct {
		ID          uint
		Namespace   string
		Path        string
		ContentHash string
		Status      string
	}
	var rows []row
	err := s.db.WithContext(ctx).Model(&models.RSSHubRoute{}).
		Select("id, namespace, path, content_hash, status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]existingRoute, len(rows))
	for _, r := range rows {
		out[r.Namespace+"|"+r.Path] = existingRoute{id: r.ID, hash: r.ContentHash, status: r.Status}
	}
	return out, nil
}

// routeRecord 是从 /api/namespace 解析出的单条路由（中间结构）。
type routeRecord struct {
	Namespace   string
	Path        string
	Name        string
	URL         string
	Description string
	Example     string
	parameters  any // 原始 parameters（数组或对象）
}

// contentHash = sha256(namespace|path|name|url|description|example|parametersJSON)（D2 diff）。
// url（站点地址）与 example（示例 URL）是推荐卡片与订阅地址的事实来源，必须参与 diff：
// 3.4 之前漏掉这两项，上游改了站点或示例地址后本系统会一直用旧值（订阅 404、卡片链接失效）。
// hash 算法变更不需要迁移脚本：旧行 hash 必然不同 → 下一轮同步走 Update 幂等 upsert，
// 一次重同步即收敛（重复同步不再产生变更）。
func (r routeRecord) contentHash() string {
	raw := strings.Join([]string{
		r.Namespace, r.Path, r.Name, r.URL, r.Description, r.Example, r.ParametersJSON(),
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}

// ParametersJSON 序列化 parameters 为稳定 JSON 字符串；空值返回 "{}"（jsonb 列拒绝空串）。
func (r routeRecord) ParametersJSON() string {
	if r.parameters == nil {
		return "{}"
	}
	b, err := json.Marshal(r.parameters)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}

// flattenNamespace 把 {ns: {routes: {path: detail}}} 嵌套结构展平为 routeRecord 切片。
//
// 解析不完整返回错误（fail-closed，spec C2）：同步一旦拿到「部分目录」，其后按
// 「未出现在本轮列表」推导出的 gone 就会把没取得的路由误判成上游删除。因此任一
// namespace 体或任一条 route detail 无法解析都中止整轮——调用方在写库前放弃，
// 既不标 gone 也不做半截更新（磁盘上的目录保持原样，下一轮成功即收敛）。
func flattenNamespace(raw map[string]json.RawMessage) ([]routeRecord, error) {
	var out []routeRecord
	for ns, nsBody := range raw {
		// nsBody = {"routes": {path: detail}, "apiRoutes": {...}}
		var nsContainer struct {
			Routes    map[string]json.RawMessage `json:"routes"`
			APIRoutes map[string]json.RawMessage `json:"apiRoutes"`
		}
		if err := json.Unmarshal(nsBody, &nsContainer); err != nil {
			return nil, fmt.Errorf("namespace %q is not a routes container: %w", ns, err)
		}
		for _, group := range []map[string]json.RawMessage{nsContainer.Routes, nsContainer.APIRoutes} {
			for _, detailRaw := range group {
				rec, err := parseRouteDetail(ns, detailRaw)
				if err != nil {
					return nil, fmt.Errorf("namespace %q route detail: %w", ns, err)
				}
				if rec.Path == "" {
					// path 是 diff 键（namespace|path），拿不到就无法判断该路由是否仍然存在，
					// 更不能用「没见到」推导 gone——按解析失败处理。
					return nil, fmt.Errorf("namespace %q route detail without path", ns)
				}
				out = append(out, rec)
			}
		}
	}
	return out, nil
}

// parseRouteDetail 解析单条路由 detail；结构异常返回错误（由 flattenNamespace 升级为整轮失败）。
func parseRouteDetail(ns string, detailRaw json.RawMessage) (routeRecord, error) {
	var d struct {
		Path        string          `json:"path"`
		Name        string          `json:"name"`
		URL         string          `json:"url"`
		Example     string          `json:"example"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(detailRaw, &d); err != nil {
		return routeRecord{}, err
	}
	var params any
	if len(d.Parameters) > 0 {
		_ = json.Unmarshal(d.Parameters, &params)
	}
	return routeRecord{
		Namespace:   ns,
		Path:        d.Path,
		Name:        d.Name,
		URL:         d.URL,
		Description: d.Description,
		Example:     d.Example,
		parameters:  params,
	}, nil
}

// httpFetchNamespace 默认 fetcher：GET {baseURL}/api/namespace。
func (s *CatalogSyncService) httpFetchNamespace(ctx context.Context) (map[string]json.RawMessage, error) {
	url := strings.TrimRight(s.baseURL, "/") + "/api/namespace"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "syntopica-catalog-sync")
	client := httpclient.New(httpclient.WithTimeout(60 * time.Second))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rsshub /api/namespace returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// 响应顶层即 {namespace: {...}}（已验证）。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse namespace response: %w", err)
	}
	return raw, nil
}

// CatalogStatus 是 GET /api/discovery/catalog/status 的响应。
type CatalogStatus struct {
	Total    int64 `json:"total"`
	Ok       int64 `json:"ok"`
	Broken   int64 `json:"broken"`
	Unknown  int64 `json:"unknown"`
	Gone     int64 `json:"gone"`
	Embedded int64 `json:"embedded"`
}

// GetStatus 返回目录状态统计。
func (s *CatalogSyncService) GetStatus(ctx context.Context) (*CatalogStatus, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "CatalogSyncService.GetStatus")
	defer span.End()
	var st CatalogStatus
	if err := s.db.WithContext(ctx).Model(&models.RSSHubRoute{}).
		Select("COUNT(*) AS total").
		Scan(&st).Error; err != nil {
		return nil, err
	}
	st.Total = countStatus(s.db.WithContext(ctx), "")
	st.Ok = countStatus(s.db.WithContext(ctx), "ok")
	st.Broken = countStatus(s.db.WithContext(ctx), "broken")
	st.Unknown = countStatus(s.db.WithContext(ctx), "unknown")
	st.Gone = countStatus(s.db.WithContext(ctx), "gone")
	s.db.WithContext(ctx).Model(&models.RouteEmbedding{}).Count(&st.Embedded)
	return &st, nil
}

func countStatus(db *gorm.DB, status string) int64 {
	q := db.Model(&models.RSSHubRoute{})
	if status == "" {
		var c int64
		q.Count(&c)
		return c
	}
	var c int64
	q.Where("status = ?", status).Count(&c)
	return c
}
