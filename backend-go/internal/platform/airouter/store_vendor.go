package airouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/tracing"
)

// vendor/model 两层数据面（ai-provider-model-overhaul design D3）：绑定链从
// provider 换 model。旧 provider 链方法（UpsertRoute/LoadRouteWithProviders/
// ResolvePrimaryProvider 等）已随本 change 的 admin API 任务（3.x）删除，
// 本文件是新链路的唯一 Store 实现。

const (
	ModelKindLLM       = "llm"
	ModelKindEmbedding = "embedding"
	ModelStatusOK      = "ok"
	ModelStatusStale   = "stale"
)

var (
	ErrModelNotFound = errors.New("ai model not found")
)

// RouteModelBinding 是路由↔模型绑定的读取侧视图：模型连同其 vendor 一并装载，
// ThinkingLevel 为该绑定的场景覆盖档（空=继承模型默认档）。
type RouteModelBinding struct {
	Model         models.AIModel
	Vendor        models.AIVendor
	ThinkingLevel string // 绑定覆盖档，空=继承模型默认
}

// RouteModelBindingInput 是路由保存时的绑定写入侧输入（design D7：绑定项=[{model_id, thinking_level?}]）。
type RouteModelBindingInput struct {
	ModelID       uint
	ThinkingLevel string // 空=继承
}

// RefreshResult 是模型清单刷新的对账结果（W7）：Added=本次新增模型名、
// Stale=本次标记 stale 的名单（不删）、Visible=上游清单全部。
type RefreshResult struct {
	Added   []string
	Stale   []string
	Visible []string
}

// ListRoutesWithModels 返回全部路由，绑定链为 RouteModels → Model → Vendor
// （nested preload）。旧 provider 链已删，这是路由查询的唯一入口。
func (s *Store) ListRoutesWithModels() ([]models.AIRoute, error) {
	var routes []models.AIRoute
	if err := s.db.Preload("RouteModels", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("priority ASC")
	}).Preload("RouteModels.Model.Vendor").Order("capability ASC, name ASC").Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

// LoadRouteWithModels 语义对齐 LoadRouteWithProviders：按 capability 取路由
// （default 名优先、enabled 过滤），返回按 priority 有序的可用绑定列表。
// 绑定过滤：link.Enabled && model.Enabled && model.Status=ok && vendor.Enabled。
// 路由不存在 → ErrRouteNotFound；过滤后无可用绑定 → ErrNoProviders（被摘空的
// 线路允许存在，W8-8）。
func (s *Store) LoadRouteWithModels(capability Capability) (*models.AIRoute, []RouteModelBinding, error) {
	var route models.AIRoute
	err := s.db.Where("capability = ? AND enabled = ?", string(capability), true).
		Order("CASE WHEN name = 'default' THEN 0 ELSE 1 END").
		Order("id ASC").
		Preload("RouteModels", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("enabled = ?", true).Order("priority ASC")
		}).
		Preload("RouteModels.Model.Vendor").
		First(&route).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrRouteNotFound
		}
		return nil, nil, err
	}

	bindings := make([]RouteModelBinding, 0, len(route.RouteModels))
	for _, link := range route.RouteModels {
		if !link.Enabled || !link.Model.Enabled || link.Model.Status != ModelStatusOK || !link.Model.Vendor.Enabled {
			continue
		}
		level := ""
		if link.ThinkingLevel != nil {
			level = *link.ThinkingLevel
		}
		bindings = append(bindings, RouteModelBinding{
			Model:         link.Model,
			Vendor:        link.Model.Vendor,
			ThinkingLevel: level,
		})
	}
	if len(bindings) == 0 {
		return &route, nil, ErrNoProviders
	}
	return &route, bindings, nil
}

// UpsertRouteModels 保存能力路由的模型绑定链（取代旧 UpsertRoute 的 provider 直挂）：
// 校验在事务外先查后写（拒绝时不动任何已存绑定），写入在单事务内全删全建——
// 顺位调整与 thinking_level 覆盖是同一次保存的原子操作（W9-6）。
func (s *Store) UpsertRouteModels(route *models.AIRoute, bindings []RouteModelBindingInput) error {
	if route == nil {
		return fmt.Errorf("route is nil")
	}
	if strings.TrimSpace(route.Capability) == "" || strings.TrimSpace(route.Name) == "" {
		return fmt.Errorf("route fields are incomplete")
	}

	// 绑定校验（真实 DB 查询）：存在性、stale/未启用、capability 类型匹配、
	// thinking_level 覆盖合法性。空 bindings=清空绑定，不校验。
	if len(bindings) > 0 {
		expectedKind := ModelKindLLM
		if route.Capability == string(CapabilityEmbedding) {
			expectedKind = ModelKindEmbedding
		}
		for _, binding := range bindings {
			var m models.AIModel
			if err := s.db.Select("id, name, model_kind, enabled, status").First(&m, binding.ModelID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrModelNotFound
				}
				return err
			}
			if !m.Enabled || m.Status != ModelStatusOK {
				return fmt.Errorf("模型「%s」当前不可绑定（stale 或未启用）", m.Name)
			}
			if m.ModelKind != expectedKind {
				if expectedKind == ModelKindEmbedding {
					return fmt.Errorf("embedding 路由不能挂 llm 模型「%s」", m.Name)
				}
				return fmt.Errorf("llm 路由不能挂 embedding 模型「%s」", m.Name)
			}
			if expectedKind == ModelKindEmbedding && binding.ThinkingLevel != "" {
				return fmt.Errorf("embedding 绑定不能携带思考档覆盖")
			}
			if binding.ThinkingLevel != "" && !IsValidThinkingLevel(binding.ThinkingLevel) {
				return fmt.Errorf("thinking_level 非法: %q（合法值: %s）", binding.ThinkingLevel, strings.Join(ValidThinkingLevels, "/"))
			}
		}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "capability"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "description", "max_concurrency", "updated_at"}),
		}).Create(route).Error; err != nil {
			return err
		}

		if err := tx.Where("capability = ? AND name = ?", route.Capability, route.Name).First(route).Error; err != nil {
			return err
		}

		if err := tx.Where("route_id = ?", route.ID).Delete(&models.AIRouteModel{}).Error; err != nil {
			return err
		}

		for idx, binding := range bindings {
			link := models.AIRouteModel{
				RouteID:  route.ID,
				ModelID:  binding.ModelID,
				Priority: idx + 1,
				Enabled:  true,
			}
			// 空覆盖档存 NULL（继承模型默认档），非空存七档原值
			if binding.ThinkingLevel != "" {
				level := binding.ThinkingLevel
				link.ThinkingLevel = &level
			}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ResolvePrimaryModel 返回 capability 首位可用绑定（aihealth 探测用），
// 取代旧 ResolvePrimaryProvider。
func (s *Store) ResolvePrimaryModel(capability Capability) (*models.AIModel, *models.AIVendor, *models.AIRoute, error) {
	route, bindings, err := s.LoadRouteWithModels(capability)
	if err != nil {
		return nil, nil, nil, err
	}
	binding := bindings[0]
	model, vendor := binding.Model, binding.Vendor
	return &model, &vendor, route, nil
}

// ─── Vendor CRUD ────────────────────────────────────────────────────────────

// normalizeVendor 归一化 vendor 字段并补默认值（Create/Update 共用）。
// api_key 允许空（llama.cpp/vLLM 等无认证本地服务），不做非空校验。
func normalizeVendor(v *models.AIVendor) error {
	if v == nil {
		return fmt.Errorf("vendor is nil")
	}
	v.Name = strings.TrimSpace(v.Name)
	v.ProviderType = strings.TrimSpace(v.ProviderType)
	v.BaseURL = strings.TrimRight(strings.TrimSpace(v.BaseURL), "/")
	v.APIKey = strings.TrimSpace(v.APIKey)
	v.StartCommand = strings.TrimSpace(v.StartCommand)
	v.ThinkingDialect = strings.TrimSpace(v.ThinkingDialect)
	v.StructuredOutput = strings.TrimSpace(v.StructuredOutput)
	if v.Name == "" || v.BaseURL == "" {
		return fmt.Errorf("vendor fields are incomplete")
	}
	if v.ProviderType == "" {
		v.ProviderType = ProviderTypeOpenAICompatible
	}
	if v.TimeoutSeconds <= 0 {
		// FB-10（2026-09-29 用户反馈）：默认 120s 对长思考链路过短，提到 600s
		v.TimeoutSeconds = 600
	}
	if v.ThinkingDialect == "" {
		v.ThinkingDialect = "none"
	}
	if v.StructuredOutput == "" {
		v.StructuredOutput = "json_schema"
	}
	return nil
}

// checkVendorNameAvailable 校验 vendor.name 全局唯一：excludeID 非 0 时为更新路径
// （改名只拒绝「别人占的」名字，排除自身行）；0 为创建路径（任何撞名拒绝）。
func (s *Store) checkVendorNameAvailable(name string, excludeID uint) error {
	var conflicting models.AIVendor
	nameQuery := s.db.Where("name = ?", name)
	if excludeID != 0 {
		nameQuery = nameQuery.Where("id <> ?", excludeID)
	}
	err := nameQuery.First(&conflicting).Error
	if err == nil {
		return fmt.Errorf("vendor name %q already exists", name)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func (s *Store) ListVendors() ([]models.AIVendor, error) {
	var vendors []models.AIVendor
	if err := s.db.Order("enabled DESC, name ASC").Find(&vendors).Error; err != nil {
		return nil, err
	}
	return vendors, nil
}

func (s *Store) CreateVendor(v *models.AIVendor) error {
	if err := normalizeVendor(v); err != nil {
		return err
	}
	if err := s.checkVendorNameAvailable(v.Name, 0); err != nil {
		return err
	}
	return s.db.Create(v).Error
}

// UpdateVendor 覆盖更新 vendor。clearAPIKey=true → key 置空（W8-2）；
// 否则请求 key 为空串时沿用已存值（W8-3，key 不丢）。
func (s *Store) UpdateVendor(v *models.AIVendor, clearAPIKey bool) error {
	if err := normalizeVendor(v); err != nil {
		return err
	}
	if v.ID == 0 {
		return fmt.Errorf("vendor id is required")
	}
	if clearAPIKey {
		v.APIKey = ""
	} else if v.APIKey == "" {
		var stored models.AIVendor
		if err := s.db.Select("api_key").First(&stored, v.ID).Error; err != nil {
			return err
		}
		v.APIKey = stored.APIKey
	}
	if err := s.checkVendorNameAvailable(v.Name, v.ID); err != nil {
		return err
	}
	return s.db.Save(v).Error
}

// DeleteVendor 单事务级联删除 vendor：先取两个级联计数（删除响应/确认弹窗告知
// 「删 N 模型、解 M 绑定」，W8-7/W8-8），再解绑 → 删模型 → 删 vendor。
// 不因仍被路由引用而拒绝。
func (s *Store) DeleteVendor(id uint) (deletedModels int, detachedBindings int, err error) {
	var modelCount, bindingCount int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var modelIDs []uint
		if err := tx.Model(&models.AIModel{}).Where("vendor_id = ?", id).Pluck("id", &modelIDs).Error; err != nil {
			return err
		}
		modelCount = int64(len(modelIDs))
		// 级联计数在删除前取
		if len(modelIDs) > 0 {
			if err := tx.Model(&models.AIRouteModel{}).Where("model_id IN ?", modelIDs).Count(&bindingCount).Error; err != nil {
				return err
			}
			if err := tx.Where("model_id IN ?", modelIDs).Delete(&models.AIRouteModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("vendor_id = ?", id).Delete(&models.AIModel{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&models.AIVendor{}, id).Error
	})
	if err != nil {
		return 0, 0, err
	}
	return int(modelCount), int(bindingCount), nil
}

// ─── 模型 ───────────────────────────────────────────────────────────────────

// ListModels 返回全部模型（vendor 信息由调用方经关联字段处理），按 vendor_id, name 排序。
func (s *Store) ListModels() ([]models.AIModel, error) {
	var list []models.AIModel
	if err := s.db.Order("vendor_id ASC, name ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpsertModel 创建/更新模型。约束（specs/ai-vendor-model「模型配置约束」）：
// model_kind 缺省 llm 且只接受 llm/embedding；thinking_level 必须是合法七档且
// embedding 模型不得携带；云端 vendor（base_url 非本机/内网）不得配 embedding；
// unique(vendor_id, name) 撞名拒绝（带 ID 的改名排除自身，同 UpsertProvider 语义）。
func (s *Store) UpsertModel(m *models.AIModel) error {
	if m == nil {
		return fmt.Errorf("model is nil")
	}
	m.Name = strings.TrimSpace(m.Name)
	switch kind := strings.TrimSpace(m.ModelKind); kind {
	case "":
		m.ModelKind = ModelKindLLM
	case ModelKindLLM, ModelKindEmbedding:
		m.ModelKind = kind
	default:
		return fmt.Errorf("invalid model_kind: %q (want llm or embedding)", kind)
	}
	if m.Name == "" || m.VendorID == 0 {
		return fmt.Errorf("model fields are incomplete")
	}
	if m.ThinkingLevel != nil && *m.ThinkingLevel != "" {
		if !IsValidThinkingLevel(*m.ThinkingLevel) {
			return fmt.Errorf("thinking_level 非法: %q（合法值: %s）", *m.ThinkingLevel, strings.Join(ValidThinkingLevels, "/"))
		}
		if m.ModelKind == ModelKindEmbedding {
			return fmt.Errorf("embedding 模型不能配置思考档")
		}
	} else {
		m.ThinkingLevel = nil // 空串统一归一为 NULL
	}

	var vendor models.AIVendor
	if err := s.db.First(&vendor, m.VendorID).Error; err != nil {
		return err
	}
	// 云端 vendor 不能配 embedding（W8-4）：本地判定=hostIsLocal(base_url)
	if m.ModelKind == ModelKindEmbedding && !hostIsLocal(vendor.BaseURL) {
		return fmt.Errorf("云端 vendor 不能配置 embedding 模型「%s」", m.Name)
	}

	var conflicting models.AIModel
	nameQuery := s.db.Where("vendor_id = ? AND name = ?", m.VendorID, m.Name)
	if m.ID != 0 {
		nameQuery = nameQuery.Where("id <> ?", m.ID)
	}
	err := nameQuery.First(&conflicting).Error
	if err == nil {
		return fmt.Errorf("model name %q already exists under vendor %d", m.Name, m.VendorID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if m.ID != 0 {
		return s.db.Save(m).Error
	}
	return s.db.Create(m).Error
}

// RefreshVendorModels 按 vendor 触发模型清单刷新（W7）：fetch 返回上游 /models
// 清单，与库内对账——新出现的 INSERT（默认未启用、kind=llm、status=ok）；已存在
// 且可见的更新 last_seen_at、stale 复活为 ok；库内有但清单无的标 stale（不删，
// 路由绑定保留）。fetch 失败 → 返回错误、库内零改动（W7-3）。
func (s *Store) RefreshVendorModels(ctx context.Context, vendorID uint, fetch func(ctx context.Context, vendor models.AIVendor) ([]string, error)) (RefreshResult, error) {
	var vendor models.AIVendor
	if err := s.db.First(&vendor, vendorID).Error; err != nil {
		return RefreshResult{}, err
	}
	names, err := fetch(ctx, vendor)
	if err != nil {
		return RefreshResult{}, err
	}

	// 清单去重保序，空名忽略
	visible := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		visible = append(visible, name)
	}

	result := RefreshResult{Visible: visible}
	now := time.Now()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing []models.AIModel
		if err := tx.Where("vendor_id = ?", vendorID).Find(&existing).Error; err != nil {
			return err
		}
		existingByName := make(map[string]models.AIModel, len(existing))
		for _, m := range existing {
			existingByName[m.Name] = m
		}

		for _, name := range visible {
			if _, ok := existingByName[name]; !ok {
				// 清单新出现 → 新增，默认未启用（W7-1），kind 一律 llm（W7-5）
				m := models.AIModel{
					VendorID: vendorID, Name: name, ModelKind: ModelKindLLM,
					Enabled: false, Status: ModelStatusOK, LastSeenAt: &now,
				}
				if err := tx.Create(&m).Error; err != nil {
					return err
				}
				result.Added = append(result.Added, name)
				continue
			}
			// 已存在且可见 → 更新 last_seen_at；stale 复活为 ok（W7-4）
			updates := map[string]any{"last_seen_at": now, "updated_at": now}
			if existingByName[name].Status == ModelStatusStale {
				updates["status"] = ModelStatusOK
			}
			if err := tx.Model(&models.AIModel{}).Where("id = ?", existingByName[name].ID).Updates(updates).Error; err != nil {
				return err
			}
		}

		// 库内有、清单无 → 标 stale 不删（W7-2），路由绑定保留
		for _, m := range existing {
			if !seen[m.Name] {
				if err := tx.Model(&models.AIModel{}).Where("id = ?", m.ID).
					Updates(map[string]any{"status": ModelStatusStale, "updated_at": now}).Error; err != nil {
					return err
				}
				result.Stale = append(result.Stale, m.Name)
			}
		}
		return nil
	})
	if err != nil {
		return RefreshResult{}, err
	}
	return result, nil
}

// ─── 连通性探测与本地判定 ────────────────────────────────────────────────────

// TestVendorConnection 用 GET {base_url}/models 探测 vendor 是否可达，返回可用
// 模型 id 列表（与旧 TestConnection 同一探测语义，承载实体换成 vendor）。
// api_key 为空时不带 Authorization 头（W8-1 后半的 client 层语义）。
func TestVendorConnection(ctx context.Context, vendor models.AIVendor) ([]string, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "airouter.TestVendorConnection")
	defer span.End()
	timeout := time.Duration(vendor.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	endpoint := strings.TrimRight(vendor.BaseURL, "/") + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	if vendor.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+vendor.APIKey)
	}

	resp, err := httpclient.New(httpclient.WithTimeout(timeout)).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("无法连接 %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp)
		msg := strings.TrimSpace(string(body))
		if errResp.Error != nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("供应商返回 HTTP %d: %s", resp.StatusCode, msg)
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// hostIsLocal 判定 base_url 是否指向本机/内网（loopback 或 RFC1918 私网，
// W8-4）。解析失败（含裸域名、空串）一律视为云端——云端 vendor 因此被禁止
// 配置 embedding 模型，误判方向是收紧而非放行。
func hostIsLocal(baseURL string) bool {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return false
	}
	host := raw
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	} else {
		// 无 scheme 的裸 host（如 "10.0.0.5:8080"、"localhost"）：剥离端口与路径
		if idx := strings.Index(host, "://"); idx >= 0 {
			host = host[idx+3:]
		}
		if idx := strings.Index(host, "/"); idx >= 0 {
			host = host[:idx]
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() { // 127.0.0.0/8 与 ::1
		return true
	}
	if v4 := ip.To4(); v4 != nil { // RFC1918：10/8、172.16/12、192.168/16
		return v4[0] == 10 ||
			(v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31) ||
			(v4[0] == 192 && v4[1] == 168)
	}
	return false
}
