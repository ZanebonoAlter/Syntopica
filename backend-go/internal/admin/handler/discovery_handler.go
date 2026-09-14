package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/logging"
)

// ── discovery handler（preference-vector-feed-discovery + improve-discovery-recommendations）──
// catalog / recommendation / ask（run 化）/ run 详情 端点。

// newRecommendationService 构造推荐 service（注入 airouter 供精排/问答；未配置 route 时降级）。
func newRecommendationService() *service.RecommendationService {
	return service.NewRecommendationService(repository.Repo.DB(), airouter.NewRouter(), nil)
}

// newDiscoveryRunService 构造 run 编排 service（4.1：ask/refresh 走 run 原子发布）。
// 变量而非函数：handler 测试可替换以注入慢/provider-失败的 router（async-run-fix 回归）。
var newDiscoveryRunService = func() *service.DiscoveryRunService {
	return service.NewDiscoveryRunService(repository.Repo.DB(), airouter.NewRouter(), nil)
}

// ── catalog ──

// SyncCatalog POST /api/discovery/catalog/sync — 手动触发 RSSHub 目录同步。
func SyncCatalog(c *gin.Context) {
	svc := service.NewCatalogSyncService(repository.Repo.DB(), "")
	summary, err := svc.SyncAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	// 同步完成后异步生成新路由 embedding（design D8；best-effort，不阻塞 HTTP 响应）。
	go func() {
		if embedded, embErr := svc.EmbedPendingRoutes(context.Background(), airouter.NewRouter()); embErr != nil {
			logging.Infof("catalog sync: embed pending routes failed: %v", embErr)
		} else if embedded > 0 {
			logging.Infof("catalog sync: embedded %d routes", embedded)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// GetCatalogStatus GET /api/discovery/catalog/status — 目录状态统计。
func GetCatalogStatus(c *gin.Context) {
	svc := service.NewCatalogSyncService(repository.Repo.DB(), "")
	st, err := svc.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": st})
}

// ── recommendation ──

// GetRecommendations GET /api/discovery/recommendations?status=pending — 推荐卡片列表。
// ?scope=history → 历史聚合视图（4.4 / design D5：accepted/expired/snoozed/excluded，
// 字段名对齐前端 HistoryPayload）。scope 优先于 status。
func GetRecommendations(c *gin.Context) {
	svc := newRecommendationService()
	if c.Query("scope") == "history" {
		entries, err := svc.GetRecommendationHistory(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": entries})
		return
	}
	status := c.DefaultQuery("status", "pending")
	cards, err := svc.GetRecommendations(c.Request.Context(), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cards})
}

// RefreshRecommendations POST /api/discovery/recommendations/refresh — 换一批。
//
// 异步契约（E2E async-run-fix，同 ask）：受理即返回 {run_id, status}，粗筛/严格
// 精排/原子发布在后台推进，前端经 GET /discovery/runs/:id 轮询终态。旧同步契约
// 会被 apiClient 超时掐断请求 ctx → run 半途 failed（实证 74s）。
// 兼容：data 保留 summary 计数字段（全 0）——旧前端读 candidates/inserted 不报错，
// 但「本轮是否产出」以 run 终态/推荐列表为准（前端待按 run_id 轮询，不在本次范围）。
func RefreshRecommendations(c *gin.Context) {
	svc := newDiscoveryRunService()
	run, reused, err := svc.EnsureRefreshRun(c.Request.Context())
	if err != nil || run == nil {
		errMsg := "refresh run unavailable"
		if err != nil {
			errMsg = err.Error()
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": errMsg})
		return
	}
	if !reused {
		// 已有 running refresh run 时 reused=true：复用该轮，不重复执行（design D2）。
		svc.StartRefreshInBackground(run)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"run_id":           run.ID,
		"status":           run.Status,
		"candidates":       0,
		"inserted":         0,
		"skipped":          0,
		"cooldown_blocked": 0,
	}})
}

// acceptRequest 接受推荐的请求体。
type acceptRequest struct {
	CategoryID *uint             `json:"category_id"`
	Parameters map[string]string `json:"parameters"`
}

// AcceptRecommendation POST /api/discovery/recommendations/:id/accept — 接受（直订/填参验证后订阅）。
func AcceptRecommendation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	var req acceptRequest
	_ = c.ShouldBindJSON(&req) // body 可空（直订）
	svc := newRecommendationService()
	feed, err := svc.AcceptRecommendation(c.Request.Context(), uint(id), req.CategoryID, req.Parameters)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": feed, "message": "feed created"})
}

// DismissRecommendation POST /api/discovery/recommendations/:id/dismiss —「暂时不看」
// （4.4 语义升级：写 candidate_preferences.snoozed_until = now+snooze_days，返回实际
// 到期时刻供前端展示；相关 pending 卡由列表过滤退出默认列表，不改推荐行状态）。
func DismissRecommendation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	svc := newRecommendationService()
	until, err := svc.SnoozeRecommendation(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"snoozed_until": until}, "message": "snoozed"})
}

// ExcludeRecommendation POST /api/discovery/recommendations/:id/exclude — 长期排除
// （4.4 / D5：写 candidate_preferences.excluded_at；跨 qa/refresh 全局生效，候选
// enabled 开关不得解除排除）。
func ExcludeRecommendation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	svc := newRecommendationService()
	if err := svc.ExcludeRecommendation(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "excluded"})
}

// RestoreRecommendation POST /api/discovery/recommendations/:id/restore — 恢复
// （4.4 / R5「恢复不是订阅」：清 excluded_at/snoozed_until，仅恢复推荐资格，
// 不建订阅、不生成新卡）。
func RestoreRecommendation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	svc := newRecommendationService()
	if err := svc.RestoreRecommendation(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "restored"})
}

// askRequest 问答请求体。request_key 可选：客户端幂等键（并发重复提交/失败重试
// 复用同一 run，design D2）；空则服务端生成。
type askRequest struct {
	Question   string `json:"question" binding:"required"`
	RequestKey string `json:"request_key"`
}

// Ask POST /api/discovery/ask — 发起手动查询，返回 {run_id, status}（design D2/D9 新契约：
// 前端轮询 GET /discovery/runs/:id 取结果，不再直接返回卡片）。
//
// 异步契约（E2E async-run-fix）：受理（ensure run）同步秒回，执行阶段进后台 goroutine——
// 本地网关慢时同步跑完整链要 74s，被前端 apiClient 超时掐断请求 ctx 后 run 半途 failed
// （error_code=embedding err=...context canceled）。后台执行必须脱离请求 ctx。
// run 终态可能是 failed（error_code 可定位：configuration/embedding/recall/rerank/publish/
// internal）——受理成功即 200，失败语义由 run 状态承载（S1 查询失败后恢复）。
func Ask(c *gin.Context) {
	var req askRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "question is required"})
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "question is required"})
		return
	}
	svc := newDiscoveryRunService()
	run, execute, err := svc.EnsureAskRun(c.Request.Context(), req.Question, req.RequestKey)
	if err != nil || run == nil {
		// 受理阶段失败（DB 写入失败等）；空 question 已在上面 400 拦截。
		errMsg := "failed to start discovery run"
		if err != nil {
			errMsg = err.Error()
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": errMsg})
		return
	}
	if execute {
		// 同 request_key 已有 running/succeeded 的 run 时 execute=false：复用旧 run，不重跑。
		svc.StartAskInBackground(run, req.Question)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"run_id":     run.ID,
		"status":     run.Status,
		"error_code": run.ErrorCode,
	}})
}

// GetDiscoveryRun GET /api/discovery/runs/:id — run 详情（前端 getRun 的
// DiscoveryRun 形状：id/kind/query/status/started_at/finished_at/items，
// items 含 candidate_id/name/description/reason/recall_origins/availability）。
func GetDiscoveryRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	svc := newDiscoveryRunService()
	view, err := svc.GetRun(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// GetInterests GET /api/discovery/interests — 逐条问答兴趣记录（不合成平均画像）。
// 分页：page（默认 1）、page_size（默认 30、上限 100），created_at 降序；
// 响应 data 为数组（前端 getInterests 直接 map），分页元信息放顶层 pagination
// （沿用 reader 列表 handler 的惯例）。
func GetInterests(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(service.InterestListDefaultPageSize)))
	svc := newDiscoveryRunService()
	result, err := svc.ListInterestEntries(c.Request.Context(), service.InterestListQuery{Page: page, PageSize: pageSize})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	pages := int(result.Total) / result.Size
	if result.Size > 0 && int(result.Total)%result.Size > 0 {
		pages++
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result.Items,
		"pagination": gin.H{
			"page":     result.Page,
			"per_page": result.Size,
			"total":    result.Total,
			"pages":    pages,
		},
	})
}

// ── rsshub settings（design E）──

// GetRSSHubSettings GET /api/settings/rsshub — 读 RSSHub 实例地址（缺省回落 DefaultRSSHubBaseURL）
// 与官方文档基址 rsshub_doc_base（缺省回落 https://docs.rsshub.app，feed-param-options D4）。
func GetRSSHubSettings(c *gin.Context) {
	baseURL := service.DefaultRSSHubBaseURL
	configured := false
	if cfg, _, err := aisettings.LoadRSSHubConfig(); err == nil {
		if u, ok := cfg["rsshub_base_url"].(string); ok && strings.TrimSpace(u) != "" {
			baseURL = strings.TrimSpace(u)
			configured = true
		}
	}
	docBase, _ := aisettings.LoadRSSHubDocBaseConfig()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"rsshub_base_url":         baseURL,
			"configured":              configured,
			"default":                 service.DefaultRSSHubBaseURL,
			"rsshub_doc_base":         docBase,
			"rsshub_doc_base_default": aisettings.DefaultRSSHubDocBase(),
		},
	})
}

// saveRSSHubSettingsRequest 保存 RSSHub 实例地址请求体。
// RSSHubDocBase 为可选字段：非空时一并更新 rsshub_doc_base（feed-param-options D4）。
type saveRSSHubSettingsRequest struct {
	RSSHubBaseURL string `json:"rsshub_base_url"`
	RSSHubDocBase string `json:"rsshub_doc_base"`
}

// SaveRSSHubSettings POST /api/settings/rsshub — 写 RSSHub 实例地址（空串=恢复默认）。
// rsshub_doc_base 非空时一并写入（空串=不修改 doc_base）。
func SaveRSSHubSettings(c *gin.Context) {
	var req saveRSSHubSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}
	configJSON := map[string]interface{}{
		"rsshub_base_url": strings.TrimSpace(req.RSSHubBaseURL),
	}
	if err := aisettings.SaveRSSHubConfig(configJSON, "RSSHub instance configuration"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if docBase := strings.TrimSpace(req.RSSHubDocBase); docBase != "" {
		if err := aisettings.SaveRSSHubDocBaseConfig(docBase); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── bocha settings（数据增强 web_search 后端 key，界面可配 + 动态读）──

// defaultBochaEndpoint 是博查通搜默认 endpoint（与 config.yaml 默认一致）。
const defaultBochaEndpoint = "https://api.bochaai.com/v1/web-search"

// maskAPIKey 脱敏：返回 key 末 4 位（长度≤4 时返回空，仅标“已配置”）。GET 回显用。
func maskAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return ""
	}
	return key[len(key)-4:]
}

// GetBochaSettings GET /api/settings/bocha — 读博查配置（脱敏不回显完整 key）。
// 返回 {api_key_configured, api_key_hint(末4位), endpoint, enabled}。
func GetBochaSettings(c *gin.Context) {
	endpoint := defaultBochaEndpoint
	enabled := true
	apiKeyConfigured := false
	apiKeyHint := ""
	if cfg, _, err := aisettings.LoadBochaConfig(); err == nil && cfg != nil {
		if v, ok := cfg["api_key"].(string); ok && strings.TrimSpace(v) != "" {
			apiKeyConfigured = true
			apiKeyHint = maskAPIKey(v)
		}
		if v, ok := cfg["endpoint"].(string); ok && strings.TrimSpace(v) != "" {
			endpoint = strings.TrimSpace(v)
		}
		if v, ok := cfg["enabled"].(bool); ok {
			enabled = v
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"api_key_configured": apiKeyConfigured,
			"api_key_hint":       apiKeyHint,
			"endpoint":           endpoint,
			"enabled":            enabled,
		},
	})
}

// saveBochaSettingsRequest 保存博查配置请求体。
// Enabled 用指针：nil（未传）=保留现有值；非 nil=覆盖。避免前端漏传导致被设为 false。
// APIKey 空串=不改（保留原值）；非空才覆盖，防表单回填空覆盖掉已有 key。
type saveBochaSettingsRequest struct {
	APIKey   string `json:"api_key"`
	Endpoint string `json:"endpoint"`
	Enabled  *bool  `json:"enabled"`
}

// SaveBochaSettings POST /api/settings/bocha — 写博查配置。界面改即时生效（动态读）。
func SaveBochaSettings(c *gin.Context) {
	var req saveBochaSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}

	// 读现有配置，作为“不改”语义的缺省源。
	existing, _, _ := aisettings.LoadBochaConfig()
	if existing == nil {
		existing = map[string]interface{}{}
	}
	apiKey, _ := existing["api_key"].(string)
	endpoint, _ := existing["endpoint"].(string)
	enabled := true
	if v, ok := existing["enabled"].(bool); ok {
		enabled = v
	}

	// api_key：空串=不改，非空才覆盖。
	if k := strings.TrimSpace(req.APIKey); k != "" {
		apiKey = k
	}
	// endpoint：空串=不改（保留现有/default）；非空才覆盖。
	if ep := strings.TrimSpace(req.Endpoint); ep != "" {
		endpoint = ep
	}
	// enabled：指针 nil=不改；非 nil=覆盖。
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	configJSON := map[string]interface{}{
		"api_key":  apiKey,
		"endpoint": endpoint,
		"enabled":  enabled,
	}
	if err := aisettings.SaveBochaConfig(configJSON, "Bocha web-search configuration"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ── proxy settings（全局出站代理）──

// GetProxySettings GET /api/settings/proxy — 读全局出站代理地址（feed 抓取等所有外部请求）。
func GetProxySettings(c *gin.Context) {
	proxyURL := ""
	configured := false
	if cfg, _, err := aisettings.LoadProxyConfig(); err == nil {
		if u, ok := cfg["http_proxy_url"].(string); ok {
			proxyURL = strings.TrimSpace(u)
			configured = proxyURL != ""
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"http_proxy_url": proxyURL,
			"configured":     configured,
		},
	})
}

// saveProxySettingsRequest 保存全局出站代理地址请求体。
type saveProxySettingsRequest struct {
	HTTPProxyURL string `json:"http_proxy_url"`
}

// SaveProxySettings POST /api/settings/proxy — 写全局出站代理地址并即时生效。
// 空串=清除代理（恢复直连）。URL 需为 http/https/socks5，非法值返回 400。
func SaveProxySettings(c *gin.Context) {
	var req saveProxySettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}
	proxyURL := strings.TrimSpace(req.HTTPProxyURL)
	if err := httpclient.SetProxy(proxyURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	configJSON := map[string]interface{}{
		"http_proxy_url": proxyURL,
	}
	if err := aisettings.SaveProxyConfig(configJSON, "Global outbound proxy configuration"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
