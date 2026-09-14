package service

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── Medium 7：私网授权接线（design D9 / spec C5）──
//
// 1) ConfirmAccess 状态机：private_pending → private_allowed（确认）；confirm=false 拒绝
//    （403 forbidden，状态不变）；非 pending → 409 conflict。
// 2) 放行范围只含候选端点解析到的 IP（/32 或 /128），其他私网仍被拒（不是全局关闭 SSRF）。

// TestAllowedIPsForEndpointScopesToHost：IP 字面量直接 /32；主机名解析为全部地址的 /32|/128。
func TestAllowedIPsForEndpointScopesToHost(t *testing.T) {
	ips, err := allowedIPsForEndpoint(context.Background(), "http://127.0.0.1:8080/feed.xml")
	require.NoError(t, err)
	require.Len(t, ips, 1)
	require.True(t, safefetch.IsAllowed(net.ParseIP("127.0.0.1"), ips), "授权端点自身放行")
	require.False(t, safefetch.IsAllowed(net.ParseIP("10.1.2.3"), ips), "同私网段其他地址仍拒")
	require.False(t, safefetch.IsAllowed(net.ParseIP("192.168.1.50"), ips), "其他私网仍拒")

	// 主机名（localhost）解析：whitelist 至少含回环，非回环私网仍拒。
	local, err := allowedIPsForEndpoint(context.Background(), "http://localhost:9/feed")
	require.NoError(t, err)
	require.NotEmpty(t, local)
	require.False(t, safefetch.IsAllowed(net.ParseIP("10.1.2.3"), local))
}

// TestCheckCandidatePrivateAllowedScopedAllowedIPs：private_allowed 候选检查时把端点解析
// 到的 IP 作为 AllowedIPs 传入（不是空 Options，也不是全局放行）。
func TestCheckCandidatePrivateAllowedScopedAllowedIPs(t *testing.T) {
	db := setupCandidateCheckTestDB(t)
	svc := newCandidateCheckTestService(t, db)
	var captured safefetch.Options
	restore := SetCandidateCheckFetcher(func(_ context.Context, _ string, opts safefetch.Options) (*CandidateFetchResult, error) {
		captured = opts
		return okFeedResult(), nil
	})
	t.Cleanup(restore)

	cand := seedRSSCandidate(t, db, "http://127.0.0.1:18080/feed.xml", "private_allowed", true)
	_, err := svc.CheckCandidate(context.Background(), cand.ID, safefetch.Options{})
	require.NoError(t, err)
	require.NotEmpty(t, captured.AllowedIPs, "private_allowed 必须携带端点级 AllowedIPs")
	require.True(t, safefetch.IsAllowed(net.ParseIP("127.0.0.1"), captured.AllowedIPs))
	require.False(t, safefetch.IsAllowed(net.ParseIP("10.1.2.3"), captured.AllowedIPs), "只放行该端点，不放行其他私网")
}

// TestConfirmAccessStateMachine：确认转换 / 拒绝转换 / 非 pending 冲突。
func TestConfirmAccessStateMachine(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	cand := models.FeedCandidate{
		StableKey: "rss:http://10.0.0.9/feed", Kind: "rss",
		ManualMetadata: models.MetadataMap{ManualFieldName: "内网源"},
		AccessScope:    "private_pending", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)

	// confirm=false → 403 forbidden，状态不变。
	_, err := svc.ConfirmAccess(context.Background(), cand.ID, false)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeForbidden, ce.Code)
	var unchanged models.FeedCandidate
	require.NoError(t, db.First(&unchanged, cand.ID).Error)
	require.Equal(t, "private_pending", unchanged.AccessScope)

	// confirm=true → private_allowed，revision 自增。
	view, err := svc.ConfirmAccess(context.Background(), cand.ID, true)
	require.NoError(t, err)
	require.Equal(t, "private_allowed", view.AccessScope)
	require.Equal(t, uint(2), view.Revision)

	// 再次确认（已 private_allowed）→ 409 conflict。
	_, err = svc.ConfirmAccess(context.Background(), cand.ID, true)
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeConflict, ce.Code)

	// 不存在 → not_found。
	_, err = svc.ConfirmAccess(context.Background(), 999999, true)
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeNotFound, ce.Code)

	// public 候选确认 → conflict（无可确认项）。
	pub := models.FeedCandidate{StableKey: "rss:http://public.example/f", Kind: "rss", AccessScope: "public", Revision: 1}
	require.NoError(t, db.Create(&pub).Error)
	_, err = svc.ConfirmAccess(context.Background(), pub.ID, true)
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeConflict, ce.Code)
}
