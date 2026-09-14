package models

import "time"

// ── improve-discovery-recommendations：候选可用性状态（design D7）──
//
// 可用性按「实际 resolved URL / RSSHub 实例+参数组合」保存，不把一个填参实例的失败
// 提升为整个模板 broken：LastEndpointKey 记录上次检查的实际端点（规范化地址），端点
// 一旦变化，旧检查结论作废、状态回 unknown 重新评估。
//
// 状态语义（admin/service/availability.go 的状态机产出）：
//   - unknown：从未检查，或端点变化使旧结论失效（未验证不得作为推荐硬过滤）；
//   - ok：最近一次检查拿到可解析 RSS/Atom（成功清失败计数，但不解除用户 disabled/excluded）；
//   - broken：确定失效（连续失败达次数且跨时间窗，或 HTTP 410）；
//   - requires_parameters：模板带必填参数且无实例，不发请求。
//
// CandidateID 唯一：本表按候选单行保存当前状态（同模板多实例的并行状态由端点 key 变更
// 重置实现，不按端点分多行）。检查结果与「推荐开关/订阅」正交：写本表不得触碰
// feed_candidates.recommendation_enabled 或 feeds（spec C6 修复后复查场景）。

// CandidateAvailability 是候选的真实端点可用性状态（design D7）。
type CandidateAvailability struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	CandidateID         uint       `gorm:"uniqueIndex:idx_candidate_availability_candidate" json:"candidate_id"`
	Status              string     `gorm:"size:20;default:'unknown'" json:"status"` // unknown | ok | broken | requires_parameters
	LastAttemptAt       *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	FirstFailureAt      *time.Time `json:"first_failure_at,omitempty"`
	NextCheckAt         *time.Time `gorm:"index" json:"next_check_at,omitempty"`
	LastErrorCode       string     `gorm:"size:50" json:"last_error_code"`
	// LastEndpointKey 是上次检查的实际端点（≤500 rune 的规范化地址，留 512 余量）；
	// 空表示无端点（requires_parameters 标记）。变化即作废旧状态。
	LastEndpointKey string    `gorm:"size:512" json:"last_endpoint_key"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (CandidateAvailability) TableName() string { return "candidate_availability" }
