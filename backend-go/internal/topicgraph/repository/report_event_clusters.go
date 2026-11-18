package repository

import (
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/textutil"
)

// ── 日报同事件聚簇的数据读取（retire-story-adopt-event-metrics D11/D12）────
//
// 日报生成时对「该版块当天命中的 event 标签」内联现算 related-event-v1 向量、
// 内存聚簇。本文件只提供只读投影：候选线程、标签事实、内容身份键、语义向量
// 兜底。聚簇算法与模型调用在 service 层（daily_report_event_cluster.go），
// 结果不落任何表。

// ReportEventThread is one report thread's event-tag / evidence projection.
// Composite (综合稿) threads are included since
// widen-highlight-cluster-candidates (2026-11-15): a single event voiced by
// many near-duplicate tags aggregates into one composite thread, and its
// event tags SHALL cluster with equal weight — the old entry-level scope
// exclusion starved the day's biggest cluster. Cross-event mixed tags are
// isolated by the clustering similarity threshold (own small cluster), not
// by this entry filter (spec: 今日重点聚簇候选).
type ReportEventThread struct {
	TagIDs     []uint
	ArticleIDs []uint
}

// ListReportEventThreads returns the report's threads (composite included)
// with their event tags and related article ids. Order is thread id ASC
// (deterministic).
func (r *TopicGraphRepository) ListReportEventThreads(reportID uint) ([]ReportEventThread, error) {
	var rows []struct {
		TagIDs            JSON
		RelatedArticleIDs JSON
	}
	if err := r.db.Table("daily_report_threads").
		Select("tag_ids, related_article_ids").
		Where("report_id = ?", reportID).
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("event cluster: report %d threads: %w", reportID, err)
	}
	out := make([]ReportEventThread, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReportEventThread{
			TagIDs:     jsonUintIDs(row.TagIDs),
			ArticleIDs: jsonUintIDs(row.RelatedArticleIDs),
		})
	}
	return out, nil
}

// ListEventTagsByIDs loads the given tags' fact rows (label / description /
// aliases / metadata) for the event tags among them, ordered by id ASC. The
// daily-report clustering needs the label+description+aux-label text (the
// related-event-v1 representation) and the event fingerprint (the last-resort
// subject approximation); both come from these rows.
func (r *TopicGraphRepository) ListEventTagsByIDs(ids []uint) ([]models.TopicTag, error) {
	if len(ids) == 0 {
		return []models.TopicTag{}, nil
	}
	var tags []models.TopicTag
	if err := r.db.
		Where("id IN ? AND category = ?", ids, models.TagCategoryEvent).
		Order("id ASC").
		Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("event cluster: event tags: %w", err)
	}
	return tags, nil
}

// ArticleContentIdentities resolves each article id to its content-identity key:
// the normalized link (content identity) with feed granularity as the fallback
// — the SAME 口径 as story_signals.content_key / heatParticipantKey (spec: 内容
// 身份去重的唯一实现点). An article row that cannot be found keeps a
// per-article key instead of being silently dropped.
func (r *TopicGraphRepository) ArticleContentIdentities(articleIDs []uint) (map[uint]string, error) {
	out := make(map[uint]string, len(articleIDs))
	if len(articleIDs) == 0 {
		return out, nil
	}
	var arts []struct {
		ID     uint
		FeedID uint
		Link   string
	}
	if err := r.db.Table("articles").
		Select("id, feed_id, link").
		Where("id IN ?", articleIDs).
		Scan(&arts).Error; err != nil {
		return nil, fmt.Errorf("event cluster: article identities: %w", err)
	}
	for _, a := range arts {
		out[a.ID] = heatParticipantKey(
			textutil.NormalizeArticleLink(a.Link),
			"feed:"+fmt.Sprint(a.FeedID))
	}
	for _, id := range articleIDs {
		if _, seen := out[id]; !seen {
			// Missing article row: content identity is undecidable, so the id
			// itself is the fallback key (still one solid identity).
			out[id] = fmt.Sprintf("article:%d", id)
		}
	}
	return out, nil
}

// TagEmbeddingVectors reads the stored vectors of the given tags for the
// semantic-fallback tier of the daily-report clustering (default
// embedding_type = semantic). Tags without a vector are absent from the map —
// the caller decides whether the tier is complete.
func (r *TopicGraphRepository) TagEmbeddingVectors(tagIDs []uint, embeddingType string) (map[uint][]float64, error) {
	out := make(map[uint][]float64, len(tagIDs))
	if len(tagIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		TopicTagID uint   `gorm:"column:topic_tag_id"`
		Embedding  string `gorm:"column:embedding"`
	}
	if err := r.db.Table("topic_tag_embeddings").
		Select("topic_tag_id, embedding").
		Where("topic_tag_id IN ? AND embedding_type = ?", tagIDs, embeddingType).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("event cluster: tag embedding vectors (%s): %w", embeddingType, err)
	}
	for _, row := range rows {
		vector, err := repoParsePgVector(row.Embedding)
		if err != nil || len(vector) == 0 {
			continue
		}
		out[row.TopicTagID] = vector
	}
	return out, nil
}

// ── 聚簇结果形状与候选排序（retire-story-adopt-event-metrics D13）──────────
//
// 聚簇只在内存里存在（不落库、不做身份）；本文件定义它的传输形状与「今日重点」
// 候选排序（簇内文章数降序 → 簇内标签数降序 → 代表标签 id 升序），供 service
// 的聚簇实现与最终落库的 highlights 快照共用。

// Event cluster mode values (wire) + their user-facing labels. The label is
// what `highlights[].cluster_mode` stores, so the UI can state the real 口径
// (never presenting a degraded result as vector clustering).
const (
	EventClusterModeVector           = "vector"            // 向量聚簇
	EventClusterModeSemanticFallback = "semantic_fallback" // 语义向量兜底
	EventClusterModeSubjectApprox    = "subject_approx"    // 主体近似
)

// EventClusterModeLabel maps a wire mode to its user-facing label.
func EventClusterModeLabel(mode string) string {
	switch mode {
	case EventClusterModeSemanticFallback:
		return "语义向量兜底"
	case EventClusterModeSubjectApprox:
		return "主体近似"
	default:
		return "向量聚簇"
	}
}

// maxEventClusterCandidates caps the 「今日重点」prompt whitelist (spec: 最多 8 条).
const maxEventClusterCandidates = 8

// EventCluster is one same-day event cluster. ClusterID is the representative
// tag's id (the smallest member id): stable, globally unique and resolvable
// with one hop in the same board, so a stored highlight can point at it without
// persisting any cluster table. TagIDs are the member event tags, Label /
// Description come from the representative tag.
type EventCluster struct {
	ClusterID    uint           `json:"cluster_id"`
	TagIDs       []uint         `json:"tag_ids"`
	Label        string         `json:"label"`
	Description  string         `json:"description"`
	ArticleCount int            `json:"article_count"`
	Facts        []EventTagFact `json:"facts"`
	Mode         string         `json:"cluster_mode"`
	DataAsOf     time.Time      `json:"-"`
	ResearchAt   time.Time      `json:"-"`
}

// EventTagFact is one member tag's prompt fact (label + description). The model
// may only quote these — no invented numbers or causes.
type EventTagFact struct {
	ID          uint   `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// RankEventClusterCandidates orders the day's clusters into the highlight
// candidate list: cluster article count DESC → member event-tag count DESC →
// representative tag id ASC (unrounded article counts, stable tie-break), then
// truncates to limit. Zero candidates yield an empty list (the caller switches
// to the tag-fallback mode).
func RankEventClusterCandidates(clusters []EventCluster, limit int) []EventCluster {
	if limit <= 0 || limit > maxEventClusterCandidates {
		limit = maxEventClusterCandidates
	}
	ordered := make([]EventCluster, len(clusters))
	copy(ordered, clusters)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.ArticleCount != b.ArticleCount {
			return a.ArticleCount > b.ArticleCount
		}
		if len(a.TagIDs) != len(b.TagIDs) {
			return len(a.TagIDs) > len(b.TagIDs)
		}
		return a.ClusterID < b.ClusterID
	})
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}
