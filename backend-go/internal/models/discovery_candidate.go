package models

import "time"

// ── improve-discovery-recommendations：统一候选实体（design D1）──
//
// feed_candidates 表达「可被推荐的订阅候选」而非订阅本身：RSSHub 路由以可空 route_id 关联，
// 原生 RSS 以规范化后的 feed_url 定位。RSSHub 原始资料（名称/描述/参数字典）仍以 rsshub_routes
// 为准；manual_metadata 只存人工覆盖（键：name/description/language/region），有效展示字段按
// 「人工非空覆盖 → 上游 → 缺省」计算（admin/service.EffectiveMetadata），清空覆盖表示恢复上游。
//
// 候选 ≠ 订阅：本表不得外键级联创建或删除 Feed，订阅与否由 feeds 独立表达。

// FeedCandidate 是统一订阅候选（RSSHub 路由或原生 RSS，design D1）。
// StableKey 为可移植稳定身份：RSSHub = 小写 namespace/path；RSS = "rss:"+sha256(规范化 URL)，
// 不含数据库 ID、source 请求类型或时间。CanonicalKey 保存 RSS 候选的规范化地址
// （RSSHub 候选为空），用于「同一直接地址重复保存」的解析去重。
// Revision 供向量生成/导入应用等异步写入做乐观锁：写入前复查 revision，防旧请求覆盖新资料（D6/D8）。
type FeedCandidate struct {
	ID                    uint        `gorm:"primaryKey" json:"id"`
	StableKey             string      `gorm:"size:128;uniqueIndex:idx_feed_candidates_stable_key" json:"stable_key"`
	Kind                  string      `gorm:"size:20" json:"kind"`                 // rsshub | rss
	RouteID               *uint       `gorm:"index" json:"route_id,omitempty"`     // RSSHub 路由引用（rss 候选为 NULL）
	FeedURL               *string     `gorm:"type:text" json:"feed_url,omitempty"` // 原生 RSS 地址（rsshub 候选为 NULL）
	CanonicalKey          string      `gorm:"size:512;index" json:"canonical_key"`
	ManualMetadata        MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"manual_metadata"` // 键：name/description/language/region
	RecommendationEnabled *bool       `gorm:"default:true" json:"recommendation_enabled,omitempty"`           // 可空区分未设置；推荐资格与订阅状态正交
	AccessScope           string      `gorm:"size:20;default:'public'" json:"access_scope"`                   // public | private_pending | private_allowed
	Revision              uint        `gorm:"default:1" json:"revision"`                                      // 乐观锁版本
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`
}

func (FeedCandidate) TableName() string { return "feed_candidates" }
