package service

import "time"

// 可用性状态机（design.md D7 / test-cases.md S14）：纯函数、无 DB、无网络。
// 上层负责发起安全检查（safefetch + RSS 解析判定），把结果折叠为
// CheckOutcome 后调用 Evaluate 推进状态；持久化由调用方自理。

// 可用性状态取值。
const (
	AvailabilityStatusUnknown        = "unknown"             // 从未检查或检查已失效（端点变化）
	AvailabilityStatusOK             = "ok"                  // 最近一次检查成功
	AvailabilityStatusBroken         = "broken"              // 确定失效（连续失败达标或 410）
	AvailabilityStatusRequiresParams = "requires_parameters" // 模板需参数且无实例，不发请求
)

// CheckOutcome.Kind 取值（一次检查的归类结果）。
const (
	OutcomeHTTPOkContentValid   = "http_ok_content_valid"   // HTTP 成功且内容是可解析 RSS/Atom
	OutcomeHTTPOkContentInvalid = "http_ok_content_invalid" // HTTP 成功但内容不是 RSS/Atom
	OutcomeHTTP410              = "http_410"                // Gone：确定失效
	OutcomeHTTP429              = "http_429"                // 限流：推迟复查，不计确定失效
	OutcomeNetworkError         = "network_error"           // 超时/连接失败等
	OutcomeHTTPError            = "http_error"              // 其他 HTTP 错误（5xx 等）
)

// AvailabilityConfig 是状态机的可调参数，零值取默认。
type AvailabilityConfig struct {
	// BrokenThreshold 是升级 broken 所需的连续失败次数（默认 3）。
	BrokenThreshold int
	// BrokenWindow 是升级 broken 要求的失败跨度下限（默认 24h）：
	// 连续失败必须既达次数又跨时间窗，短暂抖动不判死。
	BrokenWindow time.Duration
	// DefaultInterval 是常规复查间隔（默认 7 天）。
	DefaultInterval time.Duration
	// MaxRetryAfter 是 429 Retry-After 的封顶（默认 7 天）。
	MaxRetryAfter time.Duration
}

// 默认参数（design.md D7：默认检查周期 7 天；连续 3 次失败且跨度至少 24 小时
// 才升级 broken；429 按 Retry-After 有界推迟）。
const (
	DefaultBrokenThreshold = 3
	DefaultBrokenWindow    = 24 * time.Hour
	DefaultCheckInterval   = 7 * 24 * time.Hour
	DefaultMaxRetryAfter   = 7 * 24 * time.Hour
)

func (c AvailabilityConfig) withDefaults() AvailabilityConfig {
	if c.BrokenThreshold <= 0 {
		c.BrokenThreshold = DefaultBrokenThreshold
	}
	if c.BrokenWindow <= 0 {
		c.BrokenWindow = DefaultBrokenWindow
	}
	if c.DefaultInterval <= 0 {
		c.DefaultInterval = DefaultCheckInterval
	}
	if c.MaxRetryAfter <= 0 {
		c.MaxRetryAfter = DefaultMaxRetryAfter
	}
	return c
}

// AvailabilityState 是可用性检查的持久化状态（调用方负责落库，
// 字段语义与 discovery 可用性表对齐，本包不做 GORM 映射）。
type AvailabilityState struct {
	Status              string
	ConsecutiveFailures int
	FirstFailureAt      *time.Time
	LastAttemptAt       *time.Time
	LastSuccessAt       *time.Time
	NextCheckAt         *time.Time
	LastErrorCode       string
}

// CheckOutcome 是一次检查的归类结果。At 为检查发生时刻；
// RetryAfterSeconds 仅对 http_429 有意义（无 Retry-After 头时为 0）。
type CheckOutcome struct {
	Kind              string
	RetryAfterSeconds int
	At                time.Time
}

func ptrTime(t time.Time) *time.Time { return &t }

// Evaluate 用一次检查结果推进可用性状态。规则（design.md D7）：
//
//   - 成功（http_ok_content_valid）：状态转 ok、清失败计数与首次失败时间、
//     记录成功时刻、按默认周期安排下次检查；
//   - 410：立即 broken（确定失效，不等待连续失败门槛）；
//   - 429：不是确定失效——不累加失败计数、不改变状态，按 min(Retry-After,
//     MaxRetryAfter) 推迟复查，无 Retry-After 用默认周期；
//   - network_error / http_error / content_invalid：短暂故障——计数 +1、首次
//     失败记时间、记错误码；ConsecutiveFailures >= BrokenThreshold 且
//     At-FirstFailureAt >= BrokenWindow 同时满足才升级 broken，否则保持原
//     状态（unknown/ok），按默认周期复查。
//
// requires_parameters 不经 Evaluate：模板缺必需参数实例时上层在发请求前
// 调 MarkRequiresParameters 设置，不做任何网络访问。
func Evaluate(prev AvailabilityState, outcome CheckOutcome, cfg AvailabilityConfig) AvailabilityState {
	cfg = cfg.withDefaults()
	next := prev

	at := outcome.At
	next.LastAttemptAt = ptrTime(at)

	switch outcome.Kind {
	case OutcomeHTTPOkContentValid:
		next.Status = AvailabilityStatusOK
		next.ConsecutiveFailures = 0
		next.FirstFailureAt = nil
		next.LastErrorCode = ""
		next.LastSuccessAt = ptrTime(at)
		next.NextCheckAt = ptrTime(at.Add(cfg.DefaultInterval))

	case OutcomeHTTP410:
		next.ConsecutiveFailures++
		if next.FirstFailureAt == nil {
			next.FirstFailureAt = ptrTime(at)
		}
		next.LastErrorCode = outcome.Kind
		next.Status = AvailabilityStatusBroken
		next.NextCheckAt = ptrTime(at.Add(cfg.DefaultInterval))

	case OutcomeHTTP429:
		next.LastErrorCode = outcome.Kind
		delay := cfg.DefaultInterval
		if outcome.RetryAfterSeconds > 0 {
			delay = time.Duration(outcome.RetryAfterSeconds) * time.Second
			if delay > cfg.MaxRetryAfter {
				delay = cfg.MaxRetryAfter
			}
		}
		next.NextCheckAt = ptrTime(at.Add(delay))

	case OutcomeNetworkError, OutcomeHTTPOkContentInvalid, OutcomeHTTPError:
		next.ConsecutiveFailures++
		if next.FirstFailureAt == nil {
			next.FirstFailureAt = ptrTime(at)
		}
		next.LastErrorCode = outcome.Kind
		if next.ConsecutiveFailures >= cfg.BrokenThreshold && at.Sub(*next.FirstFailureAt) >= cfg.BrokenWindow {
			next.Status = AvailabilityStatusBroken
		}
		next.NextCheckAt = ptrTime(at.Add(cfg.DefaultInterval))
	}

	return next
}

// MarkRequiresParameters 把状态置为 requires_parameters：模板带必填参数且
// 尚无实例时不发请求、无复查计划；上层拿到填参实例（端点变化）后应以
// unknown 重新开始。at 为标记时刻。
func MarkRequiresParameters(prev AvailabilityState, at time.Time) AvailabilityState {
	next := prev
	next.Status = AvailabilityStatusRequiresParams
	next.ConsecutiveFailures = 0
	next.FirstFailureAt = nil
	next.LastErrorCode = ""
	next.LastAttemptAt = ptrTime(at)
	next.NextCheckAt = nil
	return next
}
