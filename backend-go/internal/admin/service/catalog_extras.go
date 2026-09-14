package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// ── 路由目录可用性校验（D4）+ 路由向量生成（spec 路由向量）──
//
// 路由向量（route_embeddings）自 4.3 起是遗留输入/回滚资料：新召回走
// candidate_embeddings × feed_candidates（discovery_recall.go:32），route_embeddings
// 没有消费方。3.4 据此不做无消费方的重嵌：EmbedPendingRoutes 保持现状（只补
// 「无向量」路由，不因 hash 变化重嵌）——内容变化的向量回补由候选侧
// CandidateEmbeddingService 按有效介绍指纹节流完成。
//
// 可用性校验：对带 example 的路由异步限流 GET，标记 ok/broken；无 example 保持 unknown。
// 校验是后台异步、不阻塞同步主流程（design D4）。本实现为同步批量版（job 内顺序执行 + 限流），
// 满足 spec「不阻塞同步主流程」——sync 与 check 分属不同方法，handler 可分别触发。
//
// 路由向量：新路由 / text_hash 变更时生成 embedding（namespace+name+description 摘要）。

// AvailabilitySummary 描述一轮可用性校验的产出。
type AvailabilitySummary struct {
	Checked int `json:"checked"`
	Ok      int `json:"ok"`
	Broken  int `json:"broken"`
}

// CheckAvailability 对带 example 的路由以可配速率（默认 2 req/s）发起 GET 校验。
// 超时/非 200/空响应 → broken，否则 ok；校验 broken 的路由在推荐粗筛中被硬排除（D4）。
func (s *CatalogSyncService) CheckAvailability(ctx context.Context, ratePerSec int) (*AvailabilitySummary, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "CatalogSyncService.CheckAvailability")
	defer span.End()
	if ratePerSec <= 0 {
		ratePerSec = 2
	}
	var routes []models.RSSHubRoute
	if err := s.db.WithContext(ctx).
		Where("status <> 'gone' AND example <> ''").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	interval := time.Second / time.Duration(ratePerSec)
	client := newAvailabilityClient()
	summary := &AvailabilitySummary{}
	base := strings.TrimRight(s.baseURL, "/")
	for _, r := range routes {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		status := checkOne(ctx, client, base+r.Example)
		now := time.Now()
		if err := s.db.WithContext(ctx).Model(&models.RSSHubRoute{}).
			Where("id = ?", r.ID).
			Updates(map[string]any{"status": status, "last_checked_at": now, "updated_at": now}).Error; err != nil {
			logging.Warnf("availability update route %d: %v", r.ID, err)
		}
		summary.Checked++
		if status == "ok" {
			summary.Ok++
		} else {
			summary.Broken++
		}
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		case <-time.After(interval):
		}
	}
	return summary, nil
}

// EmbedPendingRoutes 为尚无 embedding 的路由生成向量（D4 路由向量）。
// 文本取 namespace + name + description 摘要；向量维度/模型来自 airouter 返回。
// router 为 nil 时跳过（外网/embedding route 未配置时不阻塞）。
//
// 遗留兼容（3.4 判定）：route_embeddings 已无消费方（召回改用 candidate_embeddings），
// 本方法只为旧数据/回滚资料保留，因此刻意不处理 text_hash 变化（不做无消费方的重嵌）；
// 目录同步也不依赖它——新路由的候选资料由 feed_candidates 承载，向量回补由
// CandidateEmbeddingService.DirtyCandidateEmbeddings 按指纹增量完成。
func (s *CatalogSyncService) EmbedPendingRoutes(ctx context.Context, router *airouter.Router) (int, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "CatalogSyncService.EmbedPendingRoutes")
	defer span.End()
	if router == nil {
		return 0, nil
	}
	// 取无 route_embeddings 的非 gone 路由。
	var routes []models.RSSHubRoute
	err := s.db.WithContext(ctx).
		Raw(`SELECT r.* FROM rsshub_routes r
			WHERE r.status <> 'gone'
			  AND NOT EXISTS (SELECT 1 FROM route_embeddings e WHERE e.route_id = r.id)`).
		Scan(&routes).Error
	if err != nil {
		return 0, err
	}
	count := 0
	for _, r := range routes {
		text := buildRouteEmbeddingText(r)
		result, embErr := router.Embed(ctx, airouter.EmbeddingRequest{
			Input:     []string{text},
			Operation: "discovery.route_embedding",
			Metadata:  map[string]any{"route_id": r.ID, "namespace": r.Namespace},
		}, airouter.CapabilityEmbedding)
		if embErr != nil {
			logging.Warnf("embed route %d failed: %v", r.ID, embErr)
			continue
		}
		if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
			continue
		}
		emb := models.RouteEmbedding{
			RouteID:      r.ID,
			EmbeddingVec: floatsToPgVector(result.Embeddings[0]),
			Dimension:    result.Dimensions,
			Model:        result.Model,
			TextHash:     hashRouteText(text),
		}
		if err := s.db.WithContext(ctx).Create(&emb).Error; err != nil {
			logging.Warnf("persist route embedding %d: %v", r.ID, err)
			continue
		}
		count++
	}
	return count, nil
}

// buildRouteEmbeddingText 构造路由的 embedding 文本（namespace + name + description 摘要）。
func buildRouteEmbeddingText(r models.RSSHubRoute) string {
	desc := r.Description
	if len(desc) > 200 {
		desc = desc[:200]
	}
	return r.Namespace + " " + r.Name + " " + desc
}

// hashRouteText 文本指纹（用于 text_hash 变更检测，沿用 helpers 风格）。
func hashRouteText(text string) string {
	return sha256Hex32(text)
}

// newAvailabilityClient 构造可用性校验 HTTP 客户端（15s 超时）。
func newAvailabilityClient() *http.Client {
	return httpclient.New(httpclient.WithTimeout(15 * time.Second))
}

// checkOne 对单个 example URL 发起 GET：200 + 非空响应体 → "ok"，否则 "broken"。
func checkOne(ctx context.Context, client *http.Client, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "broken"
	}
	req.Header.Set("User-Agent", "syntopica-availability-check")
	resp, err := client.Do(req)
	if err != nil {
		return "broken"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "broken"
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return "broken"
	}
	return "ok"
}

// sha256Hex32 返回 sha256 前 32 hex 字符（与 ComputeRecommendationHash 同风格）。
func sha256Hex32(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:32]
}
