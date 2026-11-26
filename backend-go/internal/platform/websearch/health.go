package websearch

import (
	"context"
	"fmt"
	"time"

	"syntopica-backend/internal/platform/logging"
)

// ── 连通性探测（design D5）────────────────────────────────────────────────
//
// 探测 = 真实探针搜索，不是进程探活：坏形态是「服务可达但搜不到东西」
// （HTTP 200 + results:[]）或「密钥/余额失效」，探活端点都看不出来。探测与
// 页边注问答共享同一凭据解析（ResolveCredentials，现读现生效）与同一搜索客户
// 端（Search），同源判活，结构上不可能「面板绿了链路还红」。
//
// 四态（枚举被前端 healthStatusMeta 与 spec 固定，不因本次换后端扩态）：
//
//	disabled     未配置（AI 供应商注册表无可用 DeepSeek 密钥）
//	unreachable  鉴权 / 额度 / 限速 / 超时 / 网络与协议失败（具体类别见 ErrorKind）
//	degraded     服务可达但探针无结果
//	ok           可达且有结果
//
// 探测不走 airouter、不写 ai_call_logs（非 AI 调用）；按需触发、不轮询。日志
// 前缀与页边注问答降级 WARN 同族（margin-notes:），排障时可一起 grep。

// 面板四态枚举。
const (
	HealthStatusDisabled    = "disabled"
	HealthStatusUnreachable = "unreachable"
	HealthStatusDegraded    = "degraded"
	HealthStatusOK          = "ok"
)

// HealthProbeQuery 是固定探针词（无输入面，不随调用方变化）。
const HealthProbeQuery = "connectivity probe"

// Health 是探测结果（直接可序列化给前端）。ErrorKind 仅在 unreachable 时非空，
// 供面板展示「鉴权失败 / 余额或额度不足 / 请求超时 / 服务暂不可用」的针对性
// 文案；其余状态为空。
type Health struct {
	Status    string    `json:"status"`
	LatencyMS int64     `json:"latency_ms"`
	Detail    string    `json:"detail"`
	ErrorKind ErrorKind `json:"error_kind,omitempty"`
}

// ProbeHealth 用固定探针词跑一次真实搜索并映射四态。所有失败形态都编码进
// Health（面板不会因探测本身报错而空白），故 error 目前恒为 nil——保留返回位
// 以便探测自身不可执行时（如未来新增前置依赖）如实透出，不吞成假状态。
func ProbeHealth(ctx context.Context) (Health, error) {
	start := time.Now()
	results, err := Search(ctx, HealthProbeQuery)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		kind := KindOf(err)
		if kind == KindNotConfigured {
			logging.Warnf("margin-notes: health probe skipped: web search not configured (%v)", err)
			return Health{
				Status: HealthStatusDisabled,
				Detail: "未关联可用的 DeepSeek 供应商（联网参考已关闭，问答不受影响）",
			}, nil
		}
		logging.Warnf("margin-notes: health probe unreachable (kind %s): %v", kind, err)
		return Health{
			Status:    HealthStatusUnreachable,
			LatencyMS: latency,
			Detail:    unreachableDetail(kind),
			ErrorKind: kind,
		}, nil
	}

	if len(results) == 0 {
		// 事故形态：HTTP 可达但结果为空（服务商侧无匹配或上游失效）。探活
		// 端点无法发现，只有真实探针搜索能暴露。
		logging.Warnf("margin-notes: health probe degraded: reachable but no results")
		return Health{
			Status:    HealthStatusDegraded,
			LatencyMS: latency,
			Detail:    "服务可达但搜索无结果（搜索服务侧可能异常）",
		}, nil
	}

	logging.Infof("margin-notes: health probe ok: %d results in %dms", len(results), latency)
	return Health{
		Status:    HealthStatusOK,
		LatencyMS: latency,
		Detail:    fmt.Sprintf("搜索正常，返回 %d 条结果", len(results)),
	}, nil
}

// unreachableDetail 给出按类别的人话原因（面板直接展示）。未知类别走固定文案：
// 原始错误（可能含响应体片段，如 5xx 返回的 HTML）只进日志，不进面板。
func unreachableDetail(kind ErrorKind) string {
	switch kind {
	case KindAuth:
		return "鉴权失败：DeepSeek 密钥无效或无权访问搜索接口"
	case KindQuota:
		return "余额或额度不足：DeepSeek 账户余额或套餐额度已耗尽"
	case KindRateLimited:
		return "请求过于频繁：搜索服务限速，请稍后重试"
	case KindTimeout:
		return "请求超时：搜索服务未在时限内响应"
	default:
		return "搜索服务暂不可用：网络不可达或服务端异常（详细原因见后端日志）"
	}
}
