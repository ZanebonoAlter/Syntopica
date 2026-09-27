package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	discmodels "syntopica-backend/internal/discovery/models"
)

// ── 路由参数可选值字典（feed-param-options）──
//
// 维护 RSSHub 路由参数的可选值枚举（人工 manual / 文档抓取 scraped）。
// spec 铁律 D5：参数可选值只来自字典真实数据，LLM 绝不生成 —— source 限定 manual/scraped，
// Create/Update 拒绝 source=llm。recommendation 响应附带 param_options 按 param_name 分组（D7）。

// ParamOption 是推荐卡片 param_options 数组元素的契约形状（design 契约锁死）。
// 刻意不含 id/route_id/param_name —— 这些由外层 card 与分组 key 承载。
type ParamOption struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Source string `json:"source"` // manual | scraped（never llm）
}

// RouteParamOptionService 字典数据访问（批量查询供 recommendation 注入 + admin CRUD）。
type RouteParamOptionService struct {
	db *gorm.DB
}

// NewRouteParamOptionService 构造。
func NewRouteParamOptionService(db *gorm.DB) *RouteParamOptionService {
	return &RouteParamOptionService{db: db}
}

// ListByRouteIDs 一次 WHERE route_id IN (?) 批量取字典（禁 N+1，design T2）。
// routeIDs 为空时直接返回 nil（不发 IN () 查询）。
func (s *RouteParamOptionService) ListByRouteIDs(ctx context.Context, routeIDs []uint) ([]discmodels.RouteParamOption, error) {
	if len(routeIDs) == 0 {
		return nil, nil
	}
	var opts []discmodels.RouteParamOption
	err := s.db.WithContext(ctx).
		Where("route_id IN ?", routeIDs).
		Order("route_id ASC, param_name ASC, id ASC").
		Find(&opts).Error
	return opts, err
}

// GroupByRouteAndParam 把扁平字典条目按 route_id → param_name → []ParamOption 分组（纯函数，无 DB）。
// 返回的 map 对每个出现的 route_id 都有对应内层 map（调用方对缺失 route_id 自行兜底空 map）。
func GroupByRouteAndParam(opts []discmodels.RouteParamOption) map[uint]map[string][]ParamOption {
	out := make(map[uint]map[string][]ParamOption, len(opts))
	for _, o := range opts {
		inner, ok := out[o.RouteID]
		if !ok {
			inner = make(map[string][]ParamOption)
			out[o.RouteID] = inner
		}
		inner[o.ParamName] = append(inner[o.ParamName], ParamOption{
			Value: o.Value, Label: o.Label, Source: o.Source,
		})
	}
	return out
}

// RouteParamOptionInput 是 admin CRUD 的请求体。
type RouteParamOptionInput struct {
	RouteID   uint   `json:"route_id"`
	ParamName string `json:"param_name"`
	Value     string `json:"value"`
	Label     string `json:"label"`
	Source    string `json:"source"`
}

// 字典 source 合法值（D5：永不接受 llm）。
func isValidParamOptionSource(s string) bool {
	return s == "" || s == "manual" || s == "scraped"
}

// List 字典列表。routeID 非 nil 时按 route_id 过滤。
func (s *RouteParamOptionService) List(ctx context.Context, routeID *uint) ([]discmodels.RouteParamOption, error) {
	q := s.db.WithContext(ctx).Order("route_id ASC, param_name ASC, id ASC")
	if routeID != nil {
		q = q.Where("route_id = ?", *routeID)
	}
	var opts []discmodels.RouteParamOption
	return opts, q.Find(&opts).Error
}

// Get 按 id 取单条。
func (s *RouteParamOptionService) Get(ctx context.Context, id uint) (*discmodels.RouteParamOption, error) {
	var opt discmodels.RouteParamOption
	if err := s.db.WithContext(ctx).First(&opt, id).Error; err != nil {
		return nil, err
	}
	return &opt, nil
}

// Create 新建字典条目。空 source 走 DB DEFAULT（'manual'）；拒绝非法 source。
func (s *RouteParamOptionService) Create(ctx context.Context, in RouteParamOptionInput) (*discmodels.RouteParamOption, error) {
	if !isValidParamOptionSource(in.Source) {
		return nil, fmt.Errorf("invalid source %q: only manual/scraped allowed (llm prohibited)", in.Source)
	}
	if in.RouteID == 0 || in.ParamName == "" || in.Value == "" {
		return nil, fmt.Errorf("route_id, param_name, value are required")
	}
	opt := discmodels.RouteParamOption{
		RouteID:   in.RouteID,
		ParamName: in.ParamName,
		Value:     in.Value,
		Label:     in.Label,
		Source:    in.Source, // 空 source → GORM 按 default:'manual' tag 省略，DB DEFAULT 生效
	}
	if err := s.db.WithContext(ctx).Create(&opt).Error; err != nil {
		return nil, err
	}
	return &opt, nil
}

// Update 更新字典条目（value/label/source 可改；route_id/param_name 保持不变，避免破坏唯一键语义）。
func (s *RouteParamOptionService) Update(ctx context.Context, id uint, in RouteParamOptionInput) (*discmodels.RouteParamOption, error) {
	if !isValidParamOptionSource(in.Source) {
		return nil, fmt.Errorf("invalid source %q: only manual/scraped allowed (llm prohibited)", in.Source)
	}
	var opt discmodels.RouteParamOption
	if err := s.db.WithContext(ctx).First(&opt, id).Error; err != nil {
		return nil, err
	}
	if in.Value != "" {
		opt.Value = in.Value
	}
	opt.Label = in.Label
	if in.Source != "" {
		opt.Source = in.Source
	}
	if err := s.db.WithContext(ctx).Save(&opt).Error; err != nil {
		return nil, err
	}
	return &opt, nil
}

// Delete 按 id 删除。
func (s *RouteParamOptionService) Delete(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&discmodels.RouteParamOption{}, id).Error
}
