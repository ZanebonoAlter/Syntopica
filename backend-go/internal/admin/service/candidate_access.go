package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
	"syntopica-backend/internal/platform/tracing"
)

// ── 私网授权接线（improve-discovery-recommendations Medium 7，design D9）──
//
// 导入/新增时标注为 private_pending 的候选（内网地址或含凭据）在用户显式确认前一律
// 不探测（CheckCandidate 拒绝、DueCandidateIDs 排除）。本文件补齐「用户主动放行」：
//
//   - POST /api/discovery/candidates/:id/access-confirm → ConfirmAccess：
//     private_pending → private_allowed（confirm=false 拒绝转换，403）；
//   - CheckCandidate 与 accept 订阅验证对 private_allowed 候选：解析其端点 host 的
//     **全部 IP**，构造只含这些 IP 的 AllowedIPs（/32 或 /128）传入 safefetch.Options。
//     这是按来源放行指定 host，**不是全局关闭 SSRF**：hook 之外的私网地址仍被拒；
//     safefetch 逐跳重解析并校验，DNS 变化/重定向越界即阻断（design D7/D9、spec C5）。

// ConfirmAccess 确认候选的私网访问授权：private_pending → private_allowed。
// confirm=false 表示用户拒绝授权 → 403 forbidden，状态不变；非 private_pending（已是
// public/private_allowed）→ 409 conflict（无可确认项，不重复写）。
func (s *CandidateCatalogService) ConfirmAccess(ctx context.Context, candidateID uint, confirm bool) (*CandidateView, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "CandidateCatalogService.ConfirmAccess")
	defer span.End()

	var cand models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&cand, candidateID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newCandidateNotFoundError(candidateID)
		}
		return nil, err
	}
	if !confirm {
		return nil, newCandidateForbiddenError(fmt.Sprintf(
			"candidate %d access confirmation declined by user (no state change)", candidateID))
	}
	if cand.AccessScope != "private_pending" {
		return nil, newCandidateConflictError(candidateID, fmt.Sprintf(
			"candidate %d is not private_pending (access_scope=%s): nothing to confirm", candidateID, cand.AccessScope))
	}
	// 条件更新（WHERE access_scope='private_pending'）防并发重复确认：影响行数 0 时复查，
	// 已被并发确认则按 conflict 处理，不覆盖 revision。
	res := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).
		Where("id = ? AND access_scope = ?", candidateID, "private_pending").
		Updates(map[string]any{
			"access_scope": "private_allowed",
			"revision":     gorm.Expr("revision + 1"),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, newCandidateConflictError(candidateID, fmt.Sprintf(
			"candidate %d access scope changed concurrently", candidateID))
	}
	var updated models.FeedCandidate
	if err := s.db.WithContext(ctx).First(&updated, candidateID).Error; err != nil {
		return nil, err
	}
	return s.buildView(ctx, updated)
}

// allowedIPsForEndpoint 解析 endpoint 的 host 当前解析到的全部 IP，返回 /32 或 /128 的
// AllowedIPs（仅放行这些地址）。host 为 IP 字面量时不查 DNS，直接放行该地址。
// 解析失败/零地址返回错误（调用方决定是否阻断；不得静默退化为空 whitelist 让私网被拒
// 而用户以为已授权）。
func allowedIPsForEndpoint(ctx context.Context, endpoint string) ([]*net.IPNet, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("endpoint has no host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return []*net.IPNet{hostIPNet(ip)}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve endpoint host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("endpoint host %q resolved to no address", host)
	}
	seen := make(map[string]struct{}, len(addrs))
	out := make([]*net.IPNet, 0, len(addrs))
	for _, a := range addrs {
		n := hostIPNet(a.IP)
		key := n.String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

// hostIPNet 把单个 IP 包装为 /32（IPv4）或 /128（IPv6）网段。
func hostIPNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}

// authorizePrivateEndpoint 为 private_allowed 候选在 opts 上补该端点的 AllowedIPs；
// 其他 scope 原样返回（public 不吃 whitelist，private_pending 已在调用前拒绝）。
func authorizePrivateEndpoint(ctx context.Context, scope, endpoint string, opts safefetch.Options) (safefetch.Options, error) {
	if scope != "private_allowed" {
		return opts, nil
	}
	ips, err := allowedIPsForEndpoint(ctx, endpoint)
	if err != nil {
		return opts, err
	}
	opts.AllowedIPs = ips
	return opts, nil
}
