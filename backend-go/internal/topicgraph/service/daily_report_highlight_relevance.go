package service

// 「今日重点」的版块贴题档、候选池与展示序
// （rank-report-highlights-by-board-relevance）。
//
// 既有链路只按热度挑重点：候选池 = 候选序（簇内文章数 → 簇内标签数 → 代表标签
// id）前 8，展示序 = 候选序。于是「当天文章最多的跑题大新闻」必然进池并居首。
// 本文件在候选序之上加一把「贴题档」（0–5，越小越贴本期正文板块）：
//
//   - 先筛：贴题簇（0–1）≥3 个时只有贴题簇进候选池；1–2 个时按贴题档补位到 3
//     个；一个都没有时维持既有口径（前 8）。模型仍从池子里挑，提示词不变。
//   - 后排：模型挑完的 ≤3 条按同一把贴题档稳定排序后落库——顺序始终由服务端
//     决定，模型返回的顺序不作数（flow 红线 46）。
//
// 贴题档只读服务端已落库的板块快照（lane_tier + topic_status_at_report，见
// repository.ListReportSectionSignals）：不新算向量、不写任何快照、不回填历史。
// 读信号失败时调用方一律退回既有口径（design D9）。

import (
	"sort"
	"strings"

	"syntopica-backend/internal/topicgraph/repository"
)

// 贴题档（0–5，越小越贴本期正文板块）。先筛与后排共用这一个键（design D3），
// 免得两套口径在边界上打架。
const (
	// relevanceRankDirect: lane_tier = l1_direct —— 标签落在版块已成立话题的质心
	// 近档且话题在跟。
	relevanceRankDirect = 0
	// relevanceRankWatch: lane_tier = watch_* —— 命中用户手动关注（正文把关注板块
	// 放末位是版面考虑，不代表贴题度低，故排在近档之后、中档之前，design D4）。
	relevanceRankWatch = 1
	// relevanceRankFollowed: lane_tier = l2_llm 且话题当期在跟；话题状态缺失
	// （历史快照 NULL）同样落这一档——缺失不惩罚。
	relevanceRankFollowed = 2
	// relevanceRankCandidateTopic: lane_tier = l2_llm 且话题当期仍是候选（观察期，
	// topic_status_at_report = candidate）。
	relevanceRankCandidateTopic = 3
	// relevanceRankNew: lane_tier = l3_new —— 当天新起的话题。
	relevanceRankNew = 4
	// relevanceRankNoSection: 本期正文没有任何板块含该簇标签，或板块没有泳道档。
	relevanceRankNoSection = 5

	// themeRelevanceRankMax is the highest rank still counted as 贴题（近档 + 手动关注）.
	themeRelevanceRankMax = relevanceRankWatch
)

// eventClusterRelevanceRank resolves one cluster's 贴题档 by comparing its COMPLETE
// member tags (plus the representative tag) against every section's tag set and
// keeping the smallest rank: a cluster whose members land in several sections takes
// the best of them (design D2 — the representative tag alone is the smallest member
// id and would misjudge cross-section clusters).
func eventClusterRelevanceRank(cluster repository.EventCluster, signals []repository.ReportSectionSignal) int {
	best := relevanceRankNoSection
	for _, signal := range signals {
		if !clusterMatchesSection(cluster, signal.TagIDs) {
			continue
		}
		if rank := sectionRelevanceRank(signal); rank < best {
			best = rank
		}
	}
	return best
}

// clusterMatchesSection reports whether any of the section's tags is a member of
// the cluster (or its representative tag).
func clusterMatchesSection(cluster repository.EventCluster, sectionTagIDs []uint) bool {
	for _, tagID := range sectionTagIDs {
		if tagID == cluster.ClusterID {
			return true
		}
		for _, member := range cluster.TagIDs {
			if member == tagID {
				return true
			}
		}
	}
	return false
}

// sectionRelevanceRank maps one section's persisted lane snapshot to its rank. The
// l2_llm band splits on the topic's status at report time; a missing status counts
// as "not a candidate" (never punished).
func sectionRelevanceRank(signal repository.ReportSectionSignal) int {
	switch {
	case signal.LaneTier == laneL1Direct:
		return relevanceRankDirect
	case strings.HasPrefix(signal.LaneTier, "watch_"):
		return relevanceRankWatch
	case signal.LaneTier == laneL2LLM:
		if signal.TopicStatusAtReport == repository.TopicStatusCandidate {
			return relevanceRankCandidateTopic
		}
		return relevanceRankFollowed
	case signal.LaneTier == laneL3New:
		return relevanceRankNew
	default:
		return relevanceRankNoSection
	}
}

// selectEventClusterCandidatePool filters the 「今日重点」candidate pool by 贴题档
// before the model picks (spec: 今日重点聚簇候选 / 候选池):
//
//   - 贴题簇（0–1）≥3 个 → 只有贴题簇进池，按候选序取至多 maxCandidates；不贴题
//     的簇不进候选、也不会被送给模型；
//   - 贴题簇 1–2 个 → 全部贴题簇留池，其余聚簇按（贴题档升序 → 候选序）补位到
//     minPool 个（补位永远排在贴题簇之后，挤不掉贴题的）；
//   - 贴题簇 0 个 → 维持既有口径（候选序前 maxCandidates，通常是新板子的冷启动）。
//
// ordered MUST be the full candidate order (repository.OrderEventClusterCandidates):
// the pool keeps that order inside each tier — never the model's, never section
// count / lane heat (design D7).
func selectEventClusterCandidatePool(ordered []repository.EventCluster, signals []repository.ReportSectionSignal, maxCandidates, minPool int) []repository.EventCluster {
	if len(ordered) == 0 {
		return []repository.EventCluster{}
	}
	if maxCandidates <= 0 {
		maxCandidates = maxHighlightCandidatePool
	}
	if minPool <= 0 || minPool > maxCandidates {
		minPool = maxCandidates
	}
	ranks := make([]int, len(ordered))
	theme := make([]int, 0, len(ordered))
	for i := range ordered {
		ranks[i] = eventClusterRelevanceRank(ordered[i], signals)
		if ranks[i] <= themeRelevanceRankMax {
			theme = append(theme, i)
		}
	}
	if len(theme) == 0 {
		return headEventClusters(ordered, maxCandidates)
	}
	pool := make([]repository.EventCluster, 0, maxCandidates)
	for _, index := range theme {
		if len(pool) == maxCandidates {
			return pool
		}
		pool = append(pool, ordered[index])
	}
	if len(pool) >= minPool {
		return pool
	}
	isTheme := make(map[int]bool, len(theme))
	for _, index := range theme {
		isTheme[index] = true
	}
	fill := make([]int, 0, len(ordered)-len(theme))
	for i := range ordered {
		if !isTheme[i] {
			fill = append(fill, i)
		}
	}
	// fill is already in candidate order, so a stable sort on the rank alone keeps
	// the documented tie-break (贴题档升序 → 候选序).
	sort.SliceStable(fill, func(a, b int) bool { return ranks[fill[a]] < ranks[fill[b]] })
	for _, index := range fill {
		if len(pool) >= minPool {
			break
		}
		pool = append(pool, ordered[index])
	}
	return pool
}

// rankPickedHighlights re-orders the model's picks with the same 贴题档 (挑完再排):
// stable, so equal ranks keep the candidate order the pool already carries. It never
// drops or limits a pick — a single pick is returned untouched, and a highlight whose
// cluster is not in the pool (tag-fallback rows carry no cluster_id) ranks lowest
// instead of failing.
func rankPickedHighlights(picked []repository.Highlight, pool []repository.EventCluster, signals []repository.ReportSectionSignal) []repository.Highlight {
	if len(picked) < 2 {
		return picked
	}
	ranks := make(map[uint]int, len(pool))
	for _, cluster := range pool {
		ranks[cluster.ClusterID] = eventClusterRelevanceRank(cluster, signals)
	}
	rankOf := func(highlight repository.Highlight) int {
		if highlight.ClusterID == nil {
			return relevanceRankNoSection
		}
		if rank, ok := ranks[*highlight.ClusterID]; ok {
			return rank
		}
		return relevanceRankNoSection
	}
	ordered := make([]repository.Highlight, len(picked))
	copy(ordered, picked)
	sort.SliceStable(ordered, func(i, j int) bool { return rankOf(ordered[i]) < rankOf(ordered[j]) })
	return ordered
}

// headEventClusters copies at most limit clusters (the unchanged 口径 fallback).
func headEventClusters(ordered []repository.EventCluster, limit int) []repository.EventCluster {
	count := len(ordered)
	if count > limit {
		count = limit
	}
	out := make([]repository.EventCluster, count)
	copy(out, ordered[:count])
	return out
}
