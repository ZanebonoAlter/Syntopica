package searxng

import (
	"context"
	"fmt"
	"strings"
	"time"

	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/logging"
)

// ── SearXNG 连通性探测（add-searxng-health-indicator design D1/D2/D6）─────
//
// 探测=真实探针搜索，非进程探活：事故形态是「进程活、引擎死」（HTTP 200 +
// results:[]），/healthz 类端点无法暴露；探测与页边注问答联网链路共享同一
// 配置解析（ResolveConfiguredEndpoint，DB > env > yaml，现读现生效）与同一
// 搜索客户端（Search），同源判活避免「面板绿了链路还红」。代价一次真实引
// 擎调用（~秒级，用户手动触发可接受）。
//
// 四态（面板档；问答落库三态的映射见 change design D4——落库不细分
// unreachable/degraded，对读者语义同为 empty）：
//
//	disabled    未配置/未启用（预检不过，与 QA 链路完全同一函数）
//	unreachable endpoint 连接失败/超时（含响应体解析失败——保守判定：
//	            不能确认可达即归此态）
//	degraded    HTTP 可达但 results 为空（引擎全挂，2026-10-01 事故形态）
//	ok          可达且结果非空
//
// 探测不走 airouter、不落 ai_call_logs（非 AI 调用）；按需触发不轮询（单用户
// 系统，轮询会持续打真实引擎）。日志前缀与 QA 降级链路同族（margin-notes:），
// 排障时可与 QA 搜索失败 WARN 一起 grep。

// 面板四态枚举（字符串常量，可读性/日志友好）。
const (
	HealthStatusDisabled    = "disabled"
	HealthStatusUnreachable = "unreachable"
	HealthStatusDegraded    = "degraded"
	HealthStatusOK          = "ok"
)

// HealthProbeQuery is the fixed probe query (design D1：固定简短 query，无输入
// 面，不随调用方变化)。
const HealthProbeQuery = "connectivity probe"

// HealthResult is the probe outcome ready for the handler: status 四态枚举 +
// 探测耗时 + 人话摘要（不透传原始 JSON）。
type HealthResult struct {
	Status    string
	LatencyMS int64
	Detail    string
}

// resolveConfiguredEndpoint is the swappable resolution hook (searchHTTPClient
// precedent)：默认实现即唯一事实源；同包测试替换它脚本化「已配置/未配置」
// 两态，不经真实 DB。
var resolveConfiguredEndpoint = func() (string, bool) {
	// 1. DB (UI) first. enabled 缺省视为启用（仅显式 false 才跳过 DB）。
	if cfg, _, err := aisettings.LoadSearxngConfig(); err == nil && cfg != nil {
		if v, ok := cfg["enabled"].(bool); !ok || v {
			if ep, _ := cfg["endpoint"].(string); strings.TrimSpace(ep) != "" {
				return strings.TrimSpace(ep), true
			}
		}
	}
	// 2. env / config.yaml 兑底（SEARXNG_URL 已在 config 加载时折入）。
	if c := config.AppConfig; c != nil {
		if ep := strings.TrimSpace(c.Searxng.Endpoint); ep != "" {
			return ep, true
		}
	}
	return "", false
}

// ResolveConfiguredEndpoint resolves the currently effective SearXNG endpoint
// (DB(界面) > env(SEARXNG_URL) > config.yaml > 空=禁用，每次现读，界面改即时
// 生效)。页边注问答链路（topicgraph/service marginNoteSearxngEndpoint）与本
// 包健康探测共享此函数——单一事实源，两侧判活口径必然一致。
func ResolveConfiguredEndpoint() (string, bool) {
	return resolveConfiguredEndpoint()
}

// ProbeHealth runs one real probe search against the currently resolved
// endpoint and maps the outcome onto the four panel states. 不重试：一次调用、
// 一次判定；探测结果只用于面板展示与日志，不影响问答链路行为。
func ProbeHealth(ctx context.Context) HealthResult {
	start := time.Now()
	endpoint, ok := resolveConfiguredEndpoint()
	if !ok {
		logging.Warnf("margin-notes: health probe skipped: searxng not configured or disabled")
		return HealthResult{
			Status: HealthStatusDisabled,
			Detail: "未配置或未启用（联网搜索已关闭，问答不受影响）",
		}
	}

	results, err := Search(ctx, endpoint, HealthProbeQuery)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		// HTTP 层错误/超时/非 2xx/响应体解析失败一律归 unreachable（D2 保守
		// 判定：不能确认可达）。detail 为错误摘要，不透传原始 JSON。
		logging.Warnf("margin-notes: health probe unreachable (endpoint %s): %v", endpoint, err)
		return HealthResult{
			Status:    HealthStatusUnreachable,
			LatencyMS: latency,
			Detail:    fmt.Sprintf("无法连接：%v", err),
		}
	}
	if len(results) == 0 {
		// 事故形态：进程活、引擎死（HTTP 200 + results 空）。进程探活无法
		// 发现，只有真实探针搜索能暴露。
		logging.Warnf("margin-notes: health probe degraded (endpoint %s): reachable but no results", endpoint)
		return HealthResult{
			Status:    HealthStatusDegraded,
			LatencyMS: latency,
			Detail:    "服务可达但搜索无结果（引擎可能故障或上游代理失效）",
		}
	}
	logging.Infof("margin-notes: health probe ok (endpoint %s): %d results in %dms", endpoint, len(results), latency)
	return HealthResult{
		Status:    HealthStatusOK,
		LatencyMS: latency,
		Detail:    fmt.Sprintf("搜索正常，返回 %d 条结果", len(results)),
	}
}
