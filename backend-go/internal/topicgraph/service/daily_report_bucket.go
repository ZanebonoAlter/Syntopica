package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// 泳道内主题分桶（add-intra-lane-theme-bucketing）
//
// 问题：section 的产生单位曾被写死成「话题 × 天」（assembleLaneGroups 按
// topicID 聚合），于是域型泳道把当天全部内容塞进一条 section（实测 #7149：
// 18 tags / 17 threads 平铺）。本文件在 lane 归属之后、section 组装之前，对
// **单个聚类内部**做主题方向分桶，产出一条聚类下的多条同归属 section。
//
// 两阶段（design D3）：
//  1. 代码算组数 k = clamp(round(线索数/2.5), 2, 6) → LLM 只给 k 个方向名 +
//     每组当日章节标题；
//  2. 方向名固定 → LLM 只做逐条归属，禁止增删改名合并拆分，温度 0。
//
// 单次自由分组实测不可用（8 次运行 2 次静默丢线索、组数 6~9 漂移、-k 指令
// 形同虚设）；两阶段 5/5 次 100% 覆盖、逐条一致。分级输入也实测过：tags 当
// 输入产出 9 桶 + 4 个单例桶，故分组单位固定为当日 thread 线索。
//
// 校验器是确定性的（design D5）：覆盖 100% / 每组 ≥2 / 组数 ≤6 / 空组丢弃；
// 任一阶段失败或修复后只剩 1 组 → 调用方保持单 section，日报不失败。

const (
	// bucketNamesOperation / bucketAssignOperation are the two ai_call_logs
	// operation names (design D10: both run under CapabilityDigestPolish).
	bucketNamesOperation  = "daily_report.bucket_theme_names"
	bucketAssignOperation = "daily_report.bucket_assign"

	// bucketMinThemeGroups / bucketMaxThemeGroups bound the direction count.
	// Both ends are hard: a 1-group split is the current single-section state
	// (degenerate), and >6 cards in one lane stops being scannable.
	bucketMinThemeGroups = 2
	bucketMaxThemeGroups = 6

	// bucketLinesPerGroup is the measured sweet spot (17→6, 9→4, 7→3, 10→4).
	// Deliberately computed from thread lines, NOT tag count.
	bucketLinesPerGroup = 2.5

	// Prompt budgets (runes). The stored title column allows 200 — overlong
	// model output is stored as-is and ellipsized by the frontend.
	bucketTitleRunes   = 12
	bucketSectionRunes = 30
	bucketSummaryRunes = 120
)

// bucketChatFn is the swappable LLM hook for the two bucketing stages. Tests
// replace it with a fake; the default issues the real airouter call with
// temperature 0 (design D3: both stages deterministic).
var bucketChatFn = func(ctx context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
	temperature := 0.0
	maxTokens := 4096
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  operation,
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema:  schema,
		Metadata:    map[string]any{"operation": operation},
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// themeBucket is one theme direction of a cluster: the direction name shown as
// the section badge, the bucket's own daily section title, and the threads it
// owns. LineIndexes are indexes into the caller's thread slice (stable identity
// across the two LLM stages and across the rebucket write path).
type themeBucket struct {
	Title        string
	SectionTitle string
	LineIndexes  []int
	Threads      []repository.Thread
	TagIDs       []uint
}

// bucketThemeName is a phase-1 direction: name + daily section title + the
// rough line count the model attached to it. The line count exists ONLY to make
// the >k convergence deterministic (merge the two smallest); phase 2 re-assigns
// every line from scratch.
type bucketThemeName struct {
	Title        string
	SectionTitle string
	LineCount    int
}

// planBucketCount decides whether and how many buckets to produce.
//
//	k = clamp(round(lines / 2.5), 2, 6)
//
// Trigger: global switch on AND (lines ≥ min_threads OR tags ≥ min_tags). The
// OR-threshold doubles as the misfire guard — narrative lanes (≤5 tags/section)
// never reach it, so only domain-type lanes get split (measured 8.2% of
// sections over 14 days).
func planBucketCount(lines, tags int, cfg repository.PersistentTopicConfig) (int, bool) {
	defaults := repository.DefaultPersistentTopicConfig()
	minThreads := cfg.IntraBucketMinThreads
	if minThreads <= 0 {
		minThreads = defaults.IntraBucketMinThreads
	}
	minTags := cfg.IntraBucketMinTags
	if minTags <= 0 {
		minTags = defaults.IntraBucketMinTags
	}

	if !cfg.IntraBucketEnabled || lines <= 0 {
		return 1, false
	}
	if lines < minThreads && tags < minTags {
		return 1, false
	}

	k := int(math.Round(float64(lines) / bucketLinesPerGroup))
	if k < bucketMinThemeGroups {
		k = bucketMinThemeGroups
	}
	if k > bucketMaxThemeGroups {
		k = bucketMaxThemeGroups
	}
	// A single line cannot be split into two directions; treat as no trigger
	// (reachable when tags ≥ min_tags but the day produced one line).
	if k > lines {
		k = lines
	}
	if k < bucketMinThemeGroups {
		return 1, false
	}
	return k, true
}

// BucketCluster splits one cluster's daily threads into 2-6 theme directions.
//
// Returns (nil, nil) when the cluster does not trigger bucketing or the repair
// converges back to a single group — the caller keeps its single section
// (spec: 未达阈值不触发 / 全组收敛为一组时回退). A non-nil error means the
// caller must degrade to a single section (spec: 分桶失败不阻断流水线); it never
// returns a half-bucketed result.
//
// tags is used for label lookups in the prompts only — the grouping unit stays
// the thread line (spec 输入粒度).
func BucketCluster(ctx context.Context, cluster repository.ClusterGroup, threads []repository.Thread, tags []repository.TagInput, laneLabel string, cfg repository.PersistentTopicConfig) ([]themeBucket, error) {
	k, trigger := planBucketCount(len(threads), len(cluster.TagIDs), cfg)
	if !trigger {
		return nil, nil
	}

	ctx, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.intra_lane_bucket")
	defer span.End()

	if strings.TrimSpace(laneLabel) == "" {
		laneLabel = cluster.GroupName
	}
	names, err := generateBucketThemeNames(ctx, cluster, threads, tags, laneLabel, k)
	if err != nil {
		return nil, fmt.Errorf("bucket theme names: %w", err)
	}
	raw, err := assignThreadsToBuckets(ctx, cluster, threads, tags, laneLabel, names)
	if err != nil {
		return nil, fmt.Errorf("bucket assign: %w", err)
	}
	lineTags := make([][]uint, len(threads))
	for i := range threads {
		lineTags[i] = threads[i].TagIDs
	}
	names, groups, degenerate := repairBuckets(names, raw, lineTags)
	if degenerate {
		logging.Infof("daily-report: intra-lane bucket for cluster '%s' converged to a single group (keeping single section)", cluster.GroupName)
		return nil, nil
	}

	buckets := make([]themeBucket, 0, len(groups))
	for gi, lineIdx := range groups {
		bucket := themeBucket{Title: names[gi].Title, LineIndexes: lineIdx}
		var tagIDs []uint
		seen := make(map[uint]bool)
		for _, li := range lineIdx {
			thread := threads[li]
			bucket.Threads = append(bucket.Threads, thread)
			for _, tid := range thread.TagIDs {
				if seen[tid] {
					continue
				}
				seen[tid] = true
				tagIDs = append(tagIDs, tid)
			}
		}
		if len(tagIDs) == 0 {
			// Degradation guard: a bucket whose threads carry no tag ids would
			// otherwise become an unassignable, embedding-less section. Keep the
			// cluster's tag set so the section stays anchored; the line split is
			// still correct.
			logging.Warnf("daily-report: intra-lane bucket '%s' of cluster '%s' has no thread tags (falling back to cluster tags)", bucket.Title, cluster.GroupName)
			tagIDs = append(tagIDs, cluster.TagIDs...)
		}
		bucket.TagIDs = tagIDs
		bucket.SectionTitle = resolveBucketSectionTitle(names[gi].SectionTitle, bucket.Threads, laneLabel)
		buckets = append(buckets, bucket)
	}

	logging.Infof("daily-report: intra-lane bucket split cluster '%s' (%d threads / %d tags) into %d directions",
		cluster.GroupName, len(threads), len(cluster.TagIDs), len(buckets))
	return buckets, nil
}

// resolveBucketSectionTitle applies the bucket-local display-title fallback
// chain (spec 产出段): phase-1 daily section title → the bucket's FIRST thread
// title → the lane label. It intentionally never borrows another bucket's
// content (the whole point of the split).
func resolveBucketSectionTitle(sectionTitle string, threads []repository.Thread, laneLabel string) string {
	label := strings.TrimSpace(sectionTitle)
	if label == "" {
		for _, th := range threads {
			if t := strings.TrimSpace(th.Title); t != "" {
				label = t
				break
			}
		}
	}
	if label == "" {
		label = strings.TrimSpace(laneLabel)
	}
	return truncateRunesMax(label, 200)
}

// generateBucketThemeNames runs phase 1 (direction names + per-direction daily
// section titles) and converges the returned group count to k.
func generateBucketThemeNames(ctx context.Context, cluster repository.ClusterGroup, threads []repository.Thread, tags []repository.TagInput, laneLabel string, k int) ([]bucketThemeName, error) {
	system, user := buildBucketNamesPrompt(cluster, threads, tags, laneLabel, k)
	content, err := bucketChatFn(ctx, system, user, bucketNamesSchema(), bucketNamesOperation)
	if err != nil {
		return nil, err
	}
	names, err := parseBucketNames(content, len(threads))
	if err != nil {
		return nil, err
	}
	converged := convergeBucketNames(names, k)
	if len(converged) < bucketMinThemeGroups {
		return nil, fmt.Errorf("phase 1 returned %d usable direction(s), need ≥%d", len(converged), bucketMinThemeGroups)
	}
	return converged, nil
}

// buildBucketNamesPrompt renders the phase-1 prompt. Threads are the grouping
// unit; tag labels are attached as fact anchors only.
func buildBucketNamesPrompt(cluster repository.ClusterGroup, threads []repository.Thread, tags []repository.TagInput, laneLabel string, k int) (system, user string) {
	system = fmt.Sprintf(`你是一名专业的新闻叙事分析师。下面给出某个话题泳道的当日线索（已事实锚定的短文本）。请把它们归纳为 %d 个互不重叠的主题方向。

要求：
1. 恰好给出 %d 个主题方向；每个方向一个名称（中文名词短语，不超过%d字，不得重复）
2. 每个方向给出一个当日章节标题（中文，不超过%d字）：一句话概括该方向今天的实际内容；必须仅基于所列线索中的事实，禁止编造事件/数字/情绪/因果；不得复述话题名
3. line_ids 列出大致属于该方向的线索编号（可先粗分，之后会重新逐条归属）
4. 只返回合法 JSON，不要 Markdown 代码块或解释文字：{"groups":[{"bucket_title":"...","section_title":"...","line_ids":[0,1]}]}`,
		k, k, bucketTitleRunes, bucketSectionRunes)

	var sb strings.Builder
	fmt.Fprintf(&sb, "## 话题泳道：%s\n\n", laneLabel)
	fmt.Fprintf(&sb, "## 当日线索（共 %d 条）\n", len(threads))
	sb.WriteString(renderBucketLines(threads, tags))
	fmt.Fprintf(&sb, "\n请归纳为 %d 个主题方向。\n", k)
	return system, sb.String()
}

// renderBucketLines renders the thread lines shared by both stages.
func renderBucketLines(threads []repository.Thread, tags []repository.TagInput) string {
	labelByID := make(map[uint]string, len(tags))
	for _, t := range tags {
		labelByID[t.ID] = t.Label
	}
	var sb strings.Builder
	for i, th := range threads {
		fmt.Fprintf(&sb, "- [line:%d] %s", i, strings.TrimSpace(th.Title))
		if summary := strings.TrimSpace(th.Summary); summary != "" {
			fmt.Fprintf(&sb, "；摘要：%s", truncateRunesMax(summary, bucketSummaryRunes))
		}
		var labels []string
		for _, tid := range th.TagIDs {
			if l, ok := labelByID[tid]; ok && strings.TrimSpace(l) != "" {
				labels = append(labels, l)
			}
		}
		if len(labels) > 0 {
			fmt.Fprintf(&sb, "；标签：%s", strings.Join(labels, " / "))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// bucketNamesSchema is the JSON schema for the phase-1 response.
func bucketNamesSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"groups": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"bucket_title":  {Type: "string", Description: "主题方向名，不超过12字"},
						"section_title": {Type: "string", Description: "该方向当日章节标题，不超过30字；须基于所列线索事实"},
						"line_ids":      {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
					},
					Required: []string{"bucket_title"},
				},
			},
		},
		Required: []string{"groups"},
	}
}

// parseBucketNames maps the phase-1 JSON onto direction names. Titles are
// trimmed, blanks skipped, duplicates merged (the fixed name set in phase 2 must
// be unambiguous), and out-of-range line ids ignored.
func parseBucketNames(content string, lineCount int) ([]bucketThemeName, error) {
	cleaned := jsonutil.SanitizeLLMJSON(content)
	var raw struct {
		Groups []struct {
			BucketTitle  string `json:"bucket_title"`
			SectionTitle string `json:"section_title"`
			LineIDs      []int  `json:"line_ids"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return nil, fmt.Errorf("parse bucket names JSON: %w", err)
	}

	var out []bucketThemeName
	byTitle := make(map[string]int, len(raw.Groups))
	for _, g := range raw.Groups {
		title := strings.TrimSpace(g.BucketTitle)
		if title == "" {
			continue
		}
		count := 0
		for _, id := range g.LineIDs {
			if id >= 0 && id < lineCount {
				count++
			}
		}
		if idx, ok := byTitle[title]; ok {
			out[idx].LineCount += count
			if out[idx].SectionTitle == "" {
				out[idx].SectionTitle = strings.TrimSpace(g.SectionTitle)
			}
			continue
		}
		byTitle[title] = len(out)
		out = append(out, bucketThemeName{
			Title:        title,
			SectionTitle: strings.TrimSpace(g.SectionTitle),
			LineCount:    count,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("phase 1 returned no usable direction name")
	}
	return out, nil
}

// convergeBucketNames reconciles the model's group count with k (spec: 组数指令
// 不被模型遵守时收敛). Too many groups → repeatedly merge the two smallest
// (tie: the later group loses) until k remain; too few → keep the actual count
// (no padding — fewer directions is a legal outcome as long as ≥2 survive).
func convergeBucketNames(names []bucketThemeName, k int) []bucketThemeName {
	target := k
	if target < bucketMinThemeGroups {
		target = bucketMinThemeGroups
	}
	if target > bucketMaxThemeGroups {
		target = bucketMaxThemeGroups
	}
	out := append([]bucketThemeName(nil), names...)
	for len(out) > target {
		out = mergeSmallestThemeGroup(out)
	}
	for len(out) > bucketMaxThemeGroups {
		out = mergeSmallestThemeGroup(out)
	}
	return out
}

// mergeSmallestThemeGroup merges the two smallest directions (by line count)
// into one, keeping the larger group's name. Phase 2 re-assigns every line, so
// dropping the smaller name loses no thread.
func mergeSmallestThemeGroup(names []bucketThemeName) []bucketThemeName {
	if len(names) < 2 {
		return names
	}
	order := make([]int, len(names))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ia, ib := order[a], order[b]
		if names[ia].LineCount != names[ib].LineCount {
			return names[ia].LineCount < names[ib].LineCount
		}
		return ia > ib // tie: the later group sorts first (it loses)
	})
	smallA, smallB := order[0], order[1]
	keep, drop := smallA, smallB
	if names[smallB].LineCount > names[smallA].LineCount ||
		(names[smallB].LineCount == names[smallA].LineCount && smallB < smallA) {
		keep, drop = smallB, smallA
	}

	out := make([]bucketThemeName, 0, len(names)-1)
	for i, name := range names {
		if i == drop {
			continue
		}
		if i == keep {
			name.LineCount += names[drop].LineCount
			if name.SectionTitle == "" {
				name.SectionTitle = names[drop].SectionTitle
			}
		}
		out = append(out, name)
	}
	return out
}

// assignThreadsToBuckets runs phase 2: with the direction names fixed, the
// model only assigns each line to one direction. Returns one line-index slice
// per direction (indexes aligned with names).
func assignThreadsToBuckets(ctx context.Context, cluster repository.ClusterGroup, threads []repository.Thread, tags []repository.TagInput, laneLabel string, names []bucketThemeName) ([][]int, error) {
	system, user := buildBucketAssignPrompt(cluster, threads, tags, laneLabel, names)
	content, err := bucketChatFn(ctx, system, user, bucketAssignSchema(), bucketAssignOperation)
	if err != nil {
		return nil, err
	}
	return parseBucketAssignments(content, len(names), len(threads))
}

func buildBucketAssignPrompt(cluster repository.ClusterGroup, threads []repository.Thread, tags []repository.TagInput, laneLabel string, names []bucketThemeName) (system, user string) {
	system = `你是一名专业的新闻叙事分析师。主题方向集合已经固定，你现在只做逐条归属。

要求：
1. 每条线索必须且只能归入一个方向，禁止遗漏、禁止重复
2. 禁止新增方向、修改方向名、合并或拆分方向
3. 只返回合法 JSON，不要 Markdown 代码块或解释文字：{"assignments":[{"line_id":0,"bucket_index":0}]}`

	var sb strings.Builder
	fmt.Fprintf(&sb, "## 话题泳道：%s\n\n## 固定主题方向\n", laneLabel)
	for i, name := range names {
		fmt.Fprintf(&sb, "- [%d] %s\n", i, name.Title)
	}
	fmt.Fprintf(&sb, "\n## 当日线索（共 %d 条）\n", len(threads))
	sb.WriteString(renderBucketLines(threads, tags))
	sb.WriteString("\n请逐条归属到上述方向。\n")
	return system, sb.String()
}

// bucketAssignSchema is the JSON schema for the phase-2 response.
func bucketAssignSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"assignments": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"line_id":      {Type: "integer"},
						"bucket_index": {Type: "integer"},
					},
					Required: []string{"line_id", "bucket_index"},
				},
			},
		},
		Required: []string{"assignments"},
	}
}

// parseBucketAssignments maps the phase-2 JSON onto per-direction line-index
// slices. A repeated line_id keeps its FIRST occurrence (spec 重复保留首处);
// out-of-range line ids / bucket indexes are dropped and later treated as
// uncovered by repairBuckets.
func parseBucketAssignments(content string, bucketCount, lineCount int) ([][]int, error) {
	cleaned := jsonutil.SanitizeLLMJSON(content)
	var raw struct {
		Assignments []struct {
			LineID      int `json:"line_id"`
			BucketIndex int `json:"bucket_index"`
		} `json:"assignments"`
	}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		return nil, fmt.Errorf("parse bucket assignments JSON: %w", err)
	}

	groups := make([][]int, bucketCount)
	seen := make([]bool, lineCount)
	for _, a := range raw.Assignments {
		if a.LineID < 0 || a.LineID >= lineCount || a.BucketIndex < 0 || a.BucketIndex >= bucketCount {
			continue
		}
		if seen[a.LineID] {
			continue // duplicate → keep first
		}
		seen[a.LineID] = true
		groups[a.BucketIndex] = append(groups[a.BucketIndex], a.LineID)
	}
	return groups, nil
}

// repairBuckets applies the deterministic repair rules (design D5) in order:
//
//  1. duplicates keep their first occurrence (handled by the builder below);
//  2. empty groups are dropped (they produce no section, spec 空组直接丢弃);
//  3. uncovered lines are merged into the most similar group — similarity is
//     the tag-set Jaccard overlap between the line and each group (a cheap,
//     deterministic proxy for "语义最近组" without embeddings; ties and
//     zero-overlap fall back to the smallest group, which is also the
//     documented no-embedding fallback);
//  4. singleton groups are merged into the most similar other group by the
//     same proxy, so every surviving group has ≥2 lines;
//  5. >6 groups are merged down to 6.
//
// degenerate=true when fewer than 2 groups survive — the caller then keeps its
// single section (spec 全组收敛为一组时回退).
func repairBuckets(names []bucketThemeName, raw [][]int, lineTags [][]uint) ([]bucketThemeName, [][]int, bool) {
	lineCount := len(lineTags)
	var groups []bucketGroup
	seen := make([]bool, lineCount)
	for gi := 0; gi < len(names) && gi < len(raw); gi++ {
		var lines []int
		for _, line := range raw[gi] {
			if line < 0 || line >= lineCount || seen[line] {
				continue
			}
			seen[line] = true
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			continue // empty group → dropped
		}
		sort.Ints(lines)
		var tags []uint
		for _, line := range lines {
			tags = unionTagIDs(tags, lineTags[line])
		}
		groups = append(groups, bucketGroup{name: names[gi], lines: lines, tagIDs: tags})
	}

	// No surviving group at all (model returned nothing usable): fall back to a
	// single group holding every line — degenerate, caller keeps one section.
	if len(groups) == 0 {
		all := make([]int, 0, lineCount)
		for i := 0; i < lineCount; i++ {
			all = append(all, i)
		}
		name := bucketThemeName{}
		if len(names) > 0 {
			name = names[0]
		}
		return []bucketThemeName{name}, [][]int{all}, true
	}

	// Uncovered lines → the most similar group (tag-overlap; tie → smallest).
	for line := 0; line < lineCount; line++ {
		if seen[line] {
			continue
		}
		target := nearestGroupIndex(groups, lineTags[line], -1)
		merged := bucketGroup{lines: []int{line}}
		mergeBucketGroups(&groups[target], merged)
		seen[line] = true
	}

	// Singleton groups → merged into the most similar other group.
	for {
		single := -1
		for i, g := range groups {
			if len(g.lines) == 1 {
				single = i
				break
			}
		}
		if single < 0 || len(groups) < 2 {
			break
		}
		target := nearestGroupIndex(groups, groups[single].tagIDs, single)
		mergeBucketGroups(&groups[target], groups[single])
		groups = append(groups[:single], groups[single+1:]...)
	}

	// Hard cap at 6 groups.
	for len(groups) > bucketMaxThemeGroups {
		groups = mergeSmallestGroupPair(groups)
	}

	outNames := make([]bucketThemeName, len(groups))
	outGroups := make([][]int, len(groups))
	for i, g := range groups {
		outNames[i] = g.name
		outGroups[i] = g.lines
	}
	return outNames, outGroups, len(groups) < bucketMinThemeGroups
}

// bucketGroup is one repaired direction: its name, the line indexes it owns,
// and the union of those lines' tag ids (the similarity proxy for repair).
type bucketGroup struct {
	name   bucketThemeName
	lines  []int
	tagIDs []uint
}

// unionTagIDs appends add's ids to dst, deduping and preserving order.
func unionTagIDs(dst []uint, add []uint) []uint {
	if len(add) == 0 {
		return dst
	}
	seen := make(map[uint]bool, len(dst)+len(add))
	for _, id := range dst {
		seen[id] = true
	}
	for _, id := range add {
		if seen[id] {
			continue
		}
		seen[id] = true
		dst = append(dst, id)
	}
	return dst
}

// nearestGroupIndex returns the group most similar to tags by Jaccard overlap,
// skipping skip (-1 = consider all). Ties prefer the smaller group, then the
// earlier index; zero overlap falls back to the smallest group (documented
// no-embedding fallback, same rule as before).
func nearestGroupIndex(groups []bucketGroup, tags []uint, skip int) int {
	best := -1
	bestScore := -1.0
	for i, g := range groups {
		if i == skip {
			continue
		}
		score := jaccardScore(g.tagIDs, tags)
		if score > bestScore ||
			(score == bestScore && best >= 0 && len(g.lines) < len(groups[best].lines)) {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return smallestGroupIndex(groups, skip)
	}
	return best
}

// jaccardScore is |A∩B| / |A∪B| (0 when either side is empty).
func jaccardScore(a, b []uint) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[uint]bool, len(a))
	for _, id := range a {
		set[id] = true
	}
	inter := 0
	union := len(a)
	for _, id := range b {
		if set[id] {
			inter++
			continue
		}
		union++
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// smallestGroupIndex returns the index of the group with the fewest lines,
// skipping skip (pass -1 to consider every group). Ties go to the earliest
// index.
func smallestGroupIndex(groups []bucketGroup, skip int) int {
	best := -1
	for i, g := range groups {
		if i == skip {
			continue
		}
		if best < 0 || len(g.lines) < len(groups[best].lines) {
			best = i
		}
	}
	return best
}

// mergeSmallestGroupPair merges the two smallest groups (tie: the later group is
// absorbed) and keeps the larger group's name.
func mergeSmallestGroupPair(groups []bucketGroup) []bucketGroup {
	if len(groups) < 2 {
		return groups
	}
	first := smallestGroupIndex(groups, -1)
	second := smallestGroupIndex(groups, first)
	if len(groups[second].lines) < len(groups[first].lines) {
		first, second = second, first
	}
	mergeBucketGroups(&groups[first], groups[second])
	return append(groups[:second], groups[second+1:]...)
}

// mergeBucketGroups absorbs src into dst (lines + tag union).
func mergeBucketGroups(dst *bucketGroup, src bucketGroup) {
	dst.lines = append(dst.lines, src.lines...)
	sort.Ints(dst.lines)
	dst.tagIDs = unionTagIDs(dst.tagIDs, src.tagIDs)
}
