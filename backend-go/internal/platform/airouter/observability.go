package airouter

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/models"
)

// 降级观测（ai-provider-model-overhaul D6 / specs/ai-route-observability）：
// 对既有 ai_call_logs 的只读聚合，不写库、不改 schema。窗口受 7 天清理约束
// （7d 按现存日志计算）。事件还原在 Go 内存按 trace_id 分组进行——同一
// Router.Chat 的全部尝试行共享一个 trace_id（Store.LogCall 从 ctx span 取），
// 单用户行量可控。

// DegradationTypeStructured 结构化输出降级（json_schema→json_object 等运行时降档）。
const DegradationTypeStructured = "structured"

// DegradationTypeFallback 供应商失败顺位迁移；from/to 均为 nil 时表达全链失败。
const DegradationTypeFallback = "fallback"

// DegradationEvent 单条降级事件：From/To 为组内顺位（0-based）迁移；全链失败
// 与 structured 事件两者为 nil。Model 为降级后实际命中的模型（全链失败=最后
// 尝试的模型）。Code：structured 为降级方向（如 "json_schema→json_object"），
// fallback 为底层错误码（全链失败=最后错误码）。
type DegradationEvent struct {
	OccurredAt time.Time `json:"occurred_at"`
	Capability string    `json:"capability"`
	From       *int      `json:"from"`
	To         *int      `json:"to"`
	Model      string    `json:"model"`
	Type       string    `json:"type"` // "structured" | "fallback"
	Code       string    `json:"code"`
}

// DegradationStats 窗口统计：Calls=调用组数（distinct trace），Degradations=
// 降级事件数（structured+fallback，不含全链失败），Failures=全链失败组数。
type DegradationStats struct {
	Calls        int64 `json:"calls"`
	Degradations int64 `json:"degradations"`
	Failures     int64 `json:"failures"`
}

// DegradationReport 观测查询结果：Events 按 OccurredAt 降序。
type DegradationReport struct {
	Window string             `json:"window"`
	Stats  DegradationStats   `json:"stats"`
	Events []DegradationEvent `json:"events"`
}

// degradationRow 是 ai_call_logs 的观测投影行（只取还原所需的列）。
type degradationRow struct {
	ID           uint64    `gorm:"column:id"`
	TraceID      string    `gorm:"column:trace_id"`
	Capability   string    `gorm:"column:capability"`
	ProviderName string    `gorm:"column:provider_name"`
	Model        string    `gorm:"column:model"`
	Success      bool      `gorm:"column:success"`
	IsFallback   bool      `gorm:"column:is_fallback"`
	ErrorCode    string    `gorm:"column:error_code"`
	RequestMeta  string    `gorm:"column:request_meta"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

// structuredStageOf 从 request_meta（JSON 串）解析 structured_stage；缺失或
// 解析失败一律返回 ok=false（观测还原静默降级，不影响查询）。
func structuredStageOf(meta string) (string, bool) {
	if meta == "" {
		return "", false
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		return "", false
	}
	stage, ok := parsed["structured_stage"].(string)
	return stage, ok && stage != ""
}

// restoreGroup 还原一组的降级事件：
//   - 全失败 → 全链失败事件（from/to=nil，model=最后一行，全链失败标记）；
//   - 组内含成功行：逐相邻对扫描（含末失败行→成功行）——**同一模型**
//     （同 provider+model，D6「同 provider 相邻两行」）的两行都带 structured_stage
//     且档位不同 → structured 事件（code="X→Y"）；三档连降产生两条（每次降档
//     一条，spec「每次降级 SHALL 产生降级事件」）；跨模型的相邻对不算
//     structured（那是 vendor fallback，交给 fallback 事件）；
//   - 组内含成功行且无任何 structured 事件 → fallback 事件（from=最后一个失败
//     行序号、to=成功行序号，T2 形态）；
//   - 首行即成功 → 无事件。
//
// 返回 (事件列表, 是否有事件, 是否全链失败)。
func restoreGroup(rows []degradationRow) ([]DegradationEvent, bool, bool) {
	firstSuccess := -1
	for i, r := range rows {
		if r.Success {
			firstSuccess = i
			break
		}
	}

	if firstSuccess < 0 {
		last := rows[len(rows)-1]
		return []DegradationEvent{{
			OccurredAt: last.CreatedAt,
			Capability: last.Capability,
			Model:      last.Model,
			Type:       DegradationTypeFallback,
			Code:       last.ErrorCode,
		}}, true, true
	}

	if firstSuccess == 0 {
		return nil, false, false
	}

	// 同一模型的相邻行对都带不同 structured_stage → 一次结构化降档（每次降档
	// 一条事件；三档连降 = 两条）。跨模型对不在此列（vendor fallback 形态）。
	var events []DegradationEvent
	for i := 0; i < firstSuccess; i++ {
		a, b := rows[i], rows[i+1]
		if a.ProviderName != b.ProviderName || a.Model != b.Model {
			continue
		}
		aStage, okA := structuredStageOf(a.RequestMeta)
		bStage, okB := structuredStageOf(b.RequestMeta)
		if !okA || !okB || aStage == bStage {
			continue
		}
		events = append(events, DegradationEvent{
			OccurredAt: b.CreatedAt,
			Capability: b.Capability,
			Model:      b.Model,
			Type:       DegradationTypeStructured,
			Code:       aStage + "→" + bStage,
		})
	}
	if len(events) > 0 {
		return events, true, false
	}

	from := firstSuccess - 1
	to := firstSuccess
	success := rows[firstSuccess]
	lastFail := rows[from]
	return []DegradationEvent{{
		OccurredAt: success.CreatedAt,
		Capability: success.Capability,
		From:       &from,
		To:         &to,
		Model:      success.Model,
		Type:       DegradationTypeFallback,
		Code:       lastFail.ErrorCode,
	}}, true, false
}

// windowLabel 把查询窗口归一为响应里的 window 字段（24h/7d 两档之外按
// time.Duration 原样格式化，兜底可读）。
func windowLabel(window time.Duration) string {
	switch window {
	case 24 * time.Hour:
		return "24h"
	case 7 * 24 * time.Hour:
		return "7d"
	default:
		return window.String()
	}
}

// QueryDegradations 聚合窗口内 ai_call_logs 的降级统计与事件流（D6）：
//   - 窗口内行按 created_at ASC, id ASC 拉取，trace_id 非空按其分组，
//     空 trace_id 每行自成组（无 trace 的历史行不丢）；
//   - capability 过滤只作用于 events（保留精确匹配），stats 保持全窗口口径
//     （specs/ai-route-observability「按 capability 过滤事件流」）；
//   - events 按 occurred_at 降序。
func (s *Store) QueryDegradations(window time.Duration, capability string) (*DegradationReport, error) {
	var rows []degradationRow
	err := s.db.Model(&models.AICallLog{}).
		Select("id, trace_id, capability, provider_name, model, success, is_fallback, error_code, request_meta, created_at").
		Where("created_at >= ?", time.Now().Add(-window)).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	// 分组：非空 trace_id 共组；空 trace_id 每行自成组（合成键 "#row-<id>"
	// 与 64 位 hex trace_id 天然不撞，行不丢）。
	groups := make(map[string][]degradationRow)
	order := make([]string, 0, len(rows))
	for _, r := range rows {
		key := r.TraceID
		if key == "" {
			key = fmt.Sprintf("#row-%d", r.ID)
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}

	report := &DegradationReport{Window: windowLabel(window), Events: []DegradationEvent{}}
	report.Stats.Calls = int64(len(groups))
	for _, key := range order {
		events, hasEvent, allFailed := restoreGroup(groups[key])
		if allFailed {
			report.Stats.Failures++
		}
		if !hasEvent {
			continue
		}
		// Degradations 只计 structured+fallback 迁移（W10-1：全链失败单独算
		// Failures，不重复计入）；且在 capability 过滤之前——stats 恒为全窗口
		// 口径，过滤只裁剪 events（specs/ai-route-observability）。
		if !allFailed {
			report.Stats.Degradations += int64(len(events))
		}
		for _, event := range events {
			if capability != "" && event.Capability != capability {
				continue
			}
			report.Events = append(report.Events, event)
		}
	}

	sort.SliceStable(report.Events, func(i, j int) bool {
		return report.Events[i].OccurredAt.After(report.Events[j].OccurredAt)
	})
	return report, nil
}
