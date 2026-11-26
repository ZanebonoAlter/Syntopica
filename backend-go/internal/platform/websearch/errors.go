package websearch

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// ErrorKind 是搜索失败的封闭枚举（web-search-provider spec「错误分类」）。调用
// 方按类别决定降级与提示：探测面板要能区分「密钥错」和「余额没」，所以不同类
// 别必须是可判定的结构，不能退化成一句字符串（那样只会剩「连接失败」）。
type ErrorKind string

const (
	// KindNotConfigured 未配置：AI 供应商注册表中没有可用的 DeepSeek 供应商。
	KindNotConfigured ErrorKind = "not_configured"
	// KindInvalidQuery 搜索词非法：本地拒绝，未发起网络请求。
	KindInvalidQuery ErrorKind = "invalid_query"
	// KindAuth 鉴权失败：401，或 403 且响应体不是额度语义。
	KindAuth ErrorKind = "auth"
	// KindQuota 余额/额度不足：402，或 403 且响应体含额度语义。
	KindQuota ErrorKind = "quota"
	// KindRateLimited 被限速：429。
	KindRateLimited ErrorKind = "rate_limited"
	// KindTimeout 超过调用截止时间（context deadline 或网络超时）。
	KindTimeout ErrorKind = "timeout"
	// KindUnavailable 服务不可用：网络不可达、5xx、响应体无法解析、读体失败。
	KindUnavailable ErrorKind = "unavailable"
)

// ErrNotConfigured 是「无可用供应商」哨兵：ResolveCredentials 无命中时返回它，
// 调用方用 errors.Is 判定并走静默降级（未配置不是故障）。KindOf 也把它映射回
// KindNotConfigured，故无论直接返回还是被包装都判得出类别。
var ErrNotConfigured = errors.New("websearch: deepseek search vendor is not configured")

// Error 是带类别的搜索错误。Status 为 HTTP 状态码（本地拒绝与非 HTTP 失败为
// 0）；Err 保留底层 cause 以支持 errors.Is/As 链式判定。
type Error struct {
	Kind   ErrorKind
	Status int
	Msg    string
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Status != 0 {
		return fmt.Sprintf("websearch %s (status %d): %s", e.Kind, e.Status, e.Msg)
	}
	return fmt.Sprintf("websearch %s: %s", e.Kind, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf 归一化任意错误到枚举类别：nil → 空；未配置哨兵 → not_configured；
// *Error → 自带 Kind；裸 context deadline / net timeout → timeout；其余 →
// unavailable。调用方（含健康探测）只依赖这一个函数即可分流。
func KindOf(err error) ErrorKind {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrNotConfigured) {
		return KindNotConfigured
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	if isTimeoutError(err) {
		return KindTimeout
	}
	return KindUnavailable
}

// isTimeoutError 判定裸错误是否为超时形态（json/io 层包装不改变语义）。
func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
