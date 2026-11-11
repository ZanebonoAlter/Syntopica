package service

// 日报当天同事件聚簇（retire-story-adopt-event-metrics D11/D12/D16）。
//
// 日报生成时对「该版块当天命中的 event 标签」内联现算 related-event-v1 向量，
// 在内存里两两比余弦并按相关事件阈值聚簇；结果只用于当天的「今日重点」候选，
// **不落库、不做身份、不跨天**。
//
// 口径与降级链：
//  1. 向量聚簇（主口径）：airouter Embed，capability=embedding，operation 独立
//     （与索引刷新的 tagmanagement.related_event.embedding 区分），每批 32、
//     单批 30s，与索引刷新同口径；每次调用尝试都由 airouter 落 ai_call_logs。
//  2. 语义向量兜底：调用失败 / 无路由 / 批次不完整时，读 topic_tag_embeddings
//     当天已有的 semantic 向量；覆盖不全则继续降级（不混用口径）。
//  3. 主体近似兜底：按事件指纹的 subject_key/action/object 机械分组；指纹缺失
//     的标签各自成簇（subject_key 已知漂移，宁可分开也不假合并）。
//
// 每一级都把真实口径写进结果（ClusterMode），调用方 SHALL 原样落到 highlights
// 的 cluster_mode 快照，绝不把降级结果呈现为向量聚簇。

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"
)

const (
	// dailyReportClusterBatchSize caps one provider call (same 32-item ceiling
	// as the related-event index refresh).
	dailyReportClusterBatchSize = 32
	// dailyReportClusterCallTimeout bounds a single provider call.
	dailyReportClusterCallTimeout = 30 * time.Second
	// dailyReportClusterMaxTags bounds the candidate set: a busy board can hit
	// hundreds of event tags a day; O(n²) comparison is cheap (n=500 → 125k
	// pairs) but the batched provider calls are not. Over budget the set is
	// truncated to the day's most-hit tags and marked as truncated.
	dailyReportClusterMaxTags = 500
	// dailyReportClusterThresholdEpsilon keeps the documented inclusive
	// threshold boundary robust against float64 rounding (same discipline as
	// the related-events boundary epsilon).
	dailyReportClusterThresholdEpsilon = 1e-6
	// dailyReportSemanticEmbeddingType is the stored embedding used by the
	// semantic-vector fallback tier (topic_tag_embeddings.embedding_type).
	dailyReportSemanticEmbeddingType = "semantic"

	// DailyReportClusterOperation is the airouter operation name of the daily
	// report's event-cluster embeddings. It is deliberately distinct from the
	// index-refresh operation and absent from the embedding-cache allowlist:
	// these are one-day, one-board vectors, never a reusable cache entry.
	DailyReportClusterOperation = "topicgraph.daily_report.event_cluster.embedding"
)

// dailyReportClusterEmbed is the swappable embedding hook (tests stub it so no
// test ever calls a real provider).
var dailyReportClusterEmbed = func(ctx context.Context, req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	return airouter.NewRouter().Embed(ctx, req, airouter.CapabilityEmbedding)
}

// dailyReportClusterNow stamps the research instant (研究时点). Swappable for
// deterministic tests.
var dailyReportClusterNow = time.Now

// EventClusterSet is the auditable outcome of one report's clustering: the
// clusters plus the honest 口径 accounting (mode / threshold / coverage /
// truncation). It is never persisted.
type EventClusterSet struct {
	Clusters []repository.EventCluster
	// Mode is the ACTUAL 口径 of this run (vector / semantic_fallback /
	// subject_approx) — never the requested one.
	Mode       string
	Threshold  float64
	TagCount   int
	Embedded   int
	FailedTags int
	Truncated  bool
	// DataAsOf is the report day (数据截至日); ResearchAt is the generation
	// instant (研究时点). Both are server-computed snapshots.
	DataAsOf   time.Time
	ResearchAt time.Time
}

// clusterCandidate is one event tag's clustering input (facts + evidence).
type clusterCandidate struct {
	tag         models.TopicTag
	text        string
	articleIDs  []uint
	hitCount    int
	reprFailure bool
}

// buildDailyReportEventClusters computes the report's same-day event clusters.
// Zero candidate event tags yields an empty (non-nil) set — the caller then
// chooses the tag-fallback highlight mode.
func buildDailyReportEventClusters(ctx context.Context, boardID, reportID uint, periodDate time.Time) (*EventClusterSet, error) {
	if repository.Repo == nil {
		return nil, fmt.Errorf("event cluster: repository is not initialized")
	}
	set := &EventClusterSet{
		Clusters:   []repository.EventCluster{},
		Mode:       repository.EventClusterModeVector,
		Threshold:  tagging.LoadRelatedEventThreshold(repository.Repo.DB()),
		DataAsOf:   repository.NormalizeReportDate(periodDate),
		ResearchAt: dailyReportClusterNow(),
	}

	threads, err := repository.Repo.ListReportEventThreads(reportID)
	if err != nil {
		return nil, err
	}
	// tagHits counts the day's non-composite threads carrying each event tag
	// (the truncation ranking); articleByTag unions the same threads' evidence.
	tagHits := make(map[uint]int)
	articleByTag := make(map[uint][]uint)
	for _, thread := range threads {
		for _, tagID := range thread.TagIDs {
			tagHits[tagID]++
			articleByTag[tagID] = appendUintSet(articleByTag[tagID], thread.ArticleIDs)
		}
	}
	if len(tagHits) == 0 {
		return set, nil
	}
	tagIDs := make([]uint, 0, len(tagHits))
	for id := range tagHits {
		tagIDs = append(tagIDs, id)
	}
	sort.Slice(tagIDs, func(i, j int) bool { return tagIDs[i] < tagIDs[j] })

	tags, err := repository.Repo.ListEventTagsByIDs(tagIDs)
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return set, nil
	}
	if len(tags) > dailyReportClusterMaxTags {
		// 超预算：按当天命中次数取前 N（tie: id asc），并如实标注截断。
		sort.SliceStable(tags, func(i, j int) bool {
			if tagHits[tags[i].ID] != tagHits[tags[j].ID] {
				return tagHits[tags[i].ID] > tagHits[tags[j].ID]
			}
			return tags[i].ID < tags[j].ID
		})
		tags = tags[:dailyReportClusterMaxTags]
		set.Truncated = true
		logging.Warnf("event cluster: report=%d candidate tags truncated to %d (most-hit first)", reportID, dailyReportClusterMaxTags)
		sort.Slice(tags, func(i, j int) bool { return tags[i].ID < tags[j].ID })
	}
	set.TagCount = len(tags)

	auxLabels, err := tagging.LoadRelatedEventAuxLabels(ctx, repository.Repo.DB(), tagIDsOf(tags))
	if err != nil {
		logging.Warnf("event cluster: report=%d auxiliary labels degraded: %v", reportID, err)
		auxLabels = map[uint][]string{}
	}

	candidates := make([]clusterCandidate, 0, len(tags))
	for _, tag := range tags {
		text, buildErr := tagging.BuildRelatedEventRepresentation(tag.Label, tag.Description, auxLabels[tag.ID])
		if buildErr != nil {
			// 显式失败：超预算 / 空名称的标签不参与向量聚簇，也不静默截断。
			set.FailedTags++
			logging.Warnf("event cluster: report=%d tag=%d representation failed: %v", reportID, tag.ID, buildErr)
			candidates = append(candidates, clusterCandidate{tag: tag, hitCount: tagHits[tag.ID], articleIDs: articleByTag[tag.ID], reprFailure: true})
			continue
		}
		candidates = append(candidates, clusterCandidate{
			tag: tag, text: text, hitCount: tagHits[tag.ID], articleIDs: articleByTag[tag.ID],
		})
	}

	identities := make(map[uint]string)
	allArticles := make([]uint, 0)
	for _, candidate := range candidates {
		allArticles = append(allArticles, candidate.articleIDs...)
	}
	if len(allArticles) > 0 {
		if identities, err = repository.Repo.ArticleContentIdentities(allArticles); err != nil {
			return nil, err
		}
	}

	groups, mode, embedded := clusterCandidateGroups(ctx, boardID, reportID, candidates, set.Threshold)
	set.Mode = mode
	set.Embedded = embedded
	set.Clusters = buildEventClusters(candidates, groups, identities, set.Mode, set.DataAsOf, set.ResearchAt)
	return set, nil
}

// clusterCandidateGroups resolves the actual 口径 and the member groups.
func clusterCandidateGroups(ctx context.Context, boardID, reportID uint, candidates []clusterCandidate, threshold float64) ([][]uint, string, int) {
	allIDs := make([]uint, len(candidates))
	for i, candidate := range candidates {
		allIDs[i] = candidate.tag.ID
	}

	// Tier 1 — 当天现算 related-event-v1 向量。表示构造失败的标签不参与
	// 向量聚簇（它没有可比文本，也不得悄悄变成单元素簇）。
	if vectors, ok := embedClusterCandidates(ctx, boardID, reportID, candidates); ok {
		return clusterByVectors(vectors, threshold), repository.EventClusterModeVector, len(vectors)
	}

	// Tier 2 — 语义向量兜底：必须覆盖全部候选标签，否则口径不完整。
	if repository.Repo != nil {
		vectors, err := repository.Repo.TagEmbeddingVectors(allIDs, dailyReportSemanticEmbeddingType)
		switch {
		case err != nil:
			logging.Warnf("event cluster: report=%d semantic fallback degraded: %v", reportID, err)
		case len(vectors) == len(allIDs):
			return clusterByVectors(vectors, threshold), repository.EventClusterModeSemanticFallback, len(vectors)
		case len(vectors) > 0:
			logging.Warnf("event cluster: report=%d semantic fallback incomplete (%d/%d), degrading to subject approximation", reportID, len(vectors), len(allIDs))
		}
	}

	// Tier 3 — 主体近似兜底。
	return clusterBySubject(candidates), repository.EventClusterModeSubjectApprox, 0
}

// embedClusterCandidates batches the representation texts into provider calls
// (32 per batch, 30s per call). A single failed or incomplete batch fails the
// whole tier: mixing half-embedded sets would compare different spaces.
func embedClusterCandidates(ctx context.Context, boardID, reportID uint, candidates []clusterCandidate) (map[uint][]float64, bool) {
	pending := make([]clusterCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.reprFailure && candidate.text != "" {
			pending = append(pending, candidate)
		}
	}
	if len(pending) == 0 {
		return nil, false
	}
	vectors := make(map[uint][]float64, len(pending))
	for start := 0; start < len(pending); start += dailyReportClusterBatchSize {
		end := start + dailyReportClusterBatchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[start:end]
		texts := make([]string, len(batch))
		for i := range batch {
			texts[i] = batch[i].text
		}
		callCtx, cancel := context.WithTimeout(ctx, dailyReportClusterCallTimeout)
		result, err := dailyReportClusterEmbed(callCtx, airouter.EmbeddingRequest{
			Input:     texts,
			Operation: DailyReportClusterOperation,
			Metadata: map[string]any{
				"board_id":               boardID,
				"report_id":              reportID,
				"representation_version": tagging.RelatedEventRepresentationVersion,
				"batch_size":             len(texts),
			},
		})
		cancel()
		if err != nil {
			logging.Warnf("event cluster: report=%d embedding batch %d/%d failed: %v", reportID, start/dailyReportClusterBatchSize+1, (len(pending)+dailyReportClusterBatchSize-1)/dailyReportClusterBatchSize, err)
			return nil, false
		}
		if result == nil || len(result.Embeddings) != len(batch) {
			logging.Warnf("event cluster: report=%d embedding batch returned a mismatched batch", reportID)
			return nil, false
		}
		for i, candidate := range batch {
			vectors[candidate.tag.ID] = result.Embeddings[i]
		}
	}
	return vectors, true
}

// clusterByVectors groups tag ids whose cosine similarity reaches the threshold
// (boundary inclusive). Only tags WITH a vector participate: a tag without one
// cannot be compared and must not surface as a singleton cluster. Deterministic:
// ids are visited ascending, union-find keeps the smallest id as the group's
// representative, groups are sorted by it.
func clusterByVectors(vectors map[uint][]float64, threshold float64) [][]uint {
	ordered := make([]uint, 0, len(vectors))
	for id := range vectors {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	parent := make(map[uint]uint, len(ordered))
	for _, id := range ordered {
		parent[id] = id
	}
	var find func(uint) uint
	find = func(id uint) uint {
		if parent[id] != id {
			parent[id] = find(parent[id])
		}
		return parent[id]
	}
	union := func(a, b uint) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}
	limit := threshold - dailyReportClusterThresholdEpsilon
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			a, b := ordered[i], ordered[j]
			va, okA := vectors[a]
			vb, okB := vectors[b]
			if !okA || !okB {
				continue
			}
			score, err := airouter.CosineSimilarity(va, vb)
			if err != nil {
				continue // 不同维度 / 零向量：不可比，不猜
			}
			if score >= limit {
				union(a, b)
			}
		}
	}
	byRoot := make(map[uint][]uint, len(ordered))
	for _, id := range ordered {
		root := find(id)
		byRoot[root] = append(byRoot[root], id)
	}
	groups := make([][]uint, 0, len(byRoot))
	for _, members := range byRoot {
		groups = append(groups, members)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	return groups
}

// clusterBySubject is the last-resort grouping: event fingerprints with the
// same (subject_key, action, object) group together; a tag whose fingerprint is
// missing/incomplete stays alone (subject_key drift makes it untrustworthy).
func clusterBySubject(candidates []clusterCandidate) [][]uint {
	type group struct {
		key     string
		members []uint
	}
	byKey := make(map[string]int)
	groups := make([]group, 0)
	for _, candidate := range candidates {
		fp, complete := ParseEventFingerprint(candidate.tag)
		key := fmt.Sprintf("tag:%d", candidate.tag.ID)
		if complete {
			key = "fp:" + fp.SubjectKey + "\x1f" + strings.ToLower(fp.Action) + "\x1f" + strings.ToLower(fp.Object)
		}
		index, ok := byKey[key]
		if !ok {
			index = len(groups)
			byKey[key] = index
			groups = append(groups, group{key: key})
		}
		groups[index].members = append(groups[index].members, candidate.tag.ID)
	}
	out := make([][]uint, 0, len(groups))
	for _, g := range groups {
		sort.Slice(g.members, func(i, j int) bool { return g.members[i] < g.members[j] })
		out = append(out, g.members)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// buildEventClusters materializes the transmission shape: representative tag
// (smallest member id), its facts, members and the content-identity-deduped
// article count (综合稿 threads never reach this function).
func buildEventClusters(candidates []clusterCandidate, groups [][]uint, identities map[uint]string, mode string, dataAsOf, researchAt time.Time) []repository.EventCluster {
	byID := make(map[uint]clusterCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.tag.ID] = candidate
	}
	clusters := make([]repository.EventCluster, 0, len(groups))
	for _, members := range groups {
		representative, ok := byID[members[0]]
		if !ok {
			continue
		}
		seen := make(map[string]bool)
		for _, id := range members {
			for _, articleID := range byID[id].articleIDs {
				if key, ok := identities[articleID]; ok {
					seen[key] = true
				}
			}
		}
		facts := make([]repository.EventTagFact, 0, len(members))
		for _, id := range members {
			member := byID[id]
			facts = append(facts, repository.EventTagFact{
				ID: member.tag.ID, Label: member.tag.Label, Description: member.tag.Description,
			})
		}
		clusters = append(clusters, repository.EventCluster{
			ClusterID:    members[0],
			TagIDs:       members,
			Label:        representative.tag.Label,
			Description:  representative.tag.Description,
			ArticleCount: len(seen),
			Facts:        facts,
			Mode:         mode,
			DataAsOf:     dataAsOf,
			ResearchAt:   researchAt,
		})
	}
	return clusters
}

// tagIDsOf projects tag rows to their ids.
func tagIDsOf(tags []models.TopicTag) []uint {
	ids := make([]uint, len(tags))
	for i := range tags {
		ids[i] = tags[i].ID
	}
	return ids
}
