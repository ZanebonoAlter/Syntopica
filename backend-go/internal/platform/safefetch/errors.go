package safefetch

import (
	"errors"
	"fmt"
	"net"
)

// 哨兵错误：调用方用 errors.Is 判定失败类别。PrivateAddressError 额外携带
// 被拒 IP（结构化），但其 Error() 输出不含完整原 URL——私网拒绝的错误会进入
// 日志与上层响应，不能借机回显内网完整地址（含 path/query）。
var (
	// ErrUnsupportedScheme 表示 URL（含重定向 Location 的目标）不是 http/https。
	ErrUnsupportedScheme = errors.New("safefetch: unsupported URL scheme (only http/https allowed)")

	// ErrPrivateAddress 表示解析出的目标 IP 落在默认拒绝网段（回环/RFC1918/
	// link-local/多播/保留段等）且未被 Options.AllowedIPs 显式授权。
	// 携带 IP 的具体类型为 *PrivateAddressError。
	ErrPrivateAddress = errors.New("safefetch: private or reserved address blocked")

	// ErrTooManyRedirects 表示重定向次数超过 Options.MaxRedirects。
	ErrTooManyRedirects = errors.New("safefetch: too many redirects")

	// ErrBodyTooLarge 表示响应体超过 Options.MaxBytes。
	ErrBodyTooLarge = errors.New("safefetch: response body exceeds size limit")

	// ErrTimeout 表示请求在 Options.Timeout 内未完成（连接/响应/读体任一阶段）。
	ErrTimeout = errors.New("safefetch: request timed out")
)

// PrivateAddressError 描述一次被拒绝的私网/保留地址访问。
// Error() 只输出被拒的 IP 与原因，不包含原 URL 的 scheme/host/path/query，
// 避免脱敏要求下（design.md D7「无权探测返回脱敏错误」）泄漏内网拓扑。
type PrivateAddressError struct {
	IP net.IP
}

func (e *PrivateAddressError) Error() string {
	return fmt.Sprintf("safefetch: resolved address %s is not allowed (private/reserved/loopback range)", e.IP)
}

func (e *PrivateAddressError) Unwrap() error { return ErrPrivateAddress }
