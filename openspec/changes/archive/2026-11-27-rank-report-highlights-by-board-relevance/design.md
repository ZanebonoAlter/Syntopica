<!-- quant-claims: none -->

## Context

动机见 `proposal.md`「Why」，行为契约见 `specs/daily-report-system/spec.md`「今日重点聚簇候选」与「当天同事件聚簇先筛重点候选」。这里只列约束实现方式的现状（事实来源：本 change 的 `explore-findings.md`）：

- 现有链路：`GenerateAndSaveReport` → `SaveReport`（正文板块连同泳道档 `lane_tier`、话题当期状态 `topic_status_at_report` 在同一事务内落库）→ `finalizeDailyReportHighlights` → 聚簇 → `RankEventClusterCandidates`（按簇内文章数取 Top 8）→ LLM 挑 ≤3 条 → `parseClusterHighlightsResponse`（按候选序输出）→ `FinalizeReportHighlights`（单次条件 UPDATE 写 highlights / title / summary，title 与 summary 取首条）。
- 为什么模型先挑：2026-11-20 `retire-story-adopt-event-metrics` D13 否决过「不用 LLM、按文章数直接取前 3」——重点的「为什么值得看」是语义判断，纯排序会退化成条数榜。本 change 保留模型挑选，只在挑选之前加一道贴题门槛。同一决策也定了「候选排序不用 section 数或泳道热度替代篇数」，本 change 不动它。
- 重点收尾时内存里只有 `report`（ID 等）与 `inputs.tags` / `inputs.clusters`（泳道分组）；`report.Sections` 为空。`inputs.clusters` 不含关注物化板块，也没有 `topic_status_at_report` 快照。
- 泳道档是「板块 embedding 到版块已成立话题质心的余弦距离分带 + 话题状态门」的离散结果（近 <0.18 且话题在跟 → 近档；≤0.30 → 中档；其余/无 embedding → 远档），阈值可经 `ai_settings` 覆盖。同泳道分桶产生的兄弟板块共享泳道档，标签各落一个板块。
- `lane_tier` 取值：`l1_direct` / `l2_llm` / `l3_new` / `watch_keyword` / `watch_sentence`；空串落 NULL。`cluster_tag_ids` 是 jsonb **数字**数组。
- 硬约束：顺序由服务端决定、模型不排（flow 红线 46）；锚定快照同事务写入、历史不回填（红线 5）；前端按接口数组顺序渲染（`daily-report-interface` 契约）；highlights 单点失败不得拖垮整份日报（红线 41）。

## Goals / Non-Goals

**Goals:**

- 候选池先筛：贴题簇（近档 + 手动关注命中）够 3 个时模型只在贴题簇里挑；不足时补位到 3 个；一个都没有时维持现状。
- 挑完再排：模型选出的重点按同一个贴题档稳定排序，补位的永远在后。
- 读板块信号失败时整体安全退回现状（候选池与展示序都用旧口径 + Warn），不改变重点的成员、文案与快照。
- 用表驱动单测把 delta spec 的候选池与排序场景钉死，并用「中东地缘政治与美伊关系」11-27 / 11-26 的真实形态做回归夹具。

**Non-Goals:**

- 不改模型提示词、不新增落库字段、不做新的向量计算、不改前端。
- 不改既有候选序比较器的三个排序键（文章数 → 标签数 → 代表标签 id）；实现上把比较器函数体抽成 `OrderEventClusterCandidates` 供组池复用完整序，`RankEventClusterCandidates` 只做截断委托。不让 section 数或泳道热度参与任何排序。
- 不改标签回退模式，不回填或重排历史日报。

## Decisions

### D1. 贴题信号 = 已落库正文板块的 `lane_tier` + `topic_status_at_report`

- **结论**：贴题与否用板块泳道档（近 / 手动关注 / 中 / 远 / 无对应板块），中档内再按话题当期是否为候选细分。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_story_highlights.go:finalizeDailyReportHighlights`
- **取舍**：被否 ① 板块 `avg_score` / `best_tier`——量纲混用（标签↔版块挂载分与 `1-余弦距离` 混算，关注板块恒为 0/4），跨板块不可比；② 标签↔版块挂载的 `score` / `match_reason`——该版块 188 个绑定里 131 个同为 weighted 0.60–0.73，贴题的伊朗新闻与中美关税同档，无区分力；③ 新算「簇向量 vs 版块主题向量」余弦——要新增向量计算、落库与阈值配置，收益不抵成本。

### D2. 簇 → 板块的映射与 1:N 取最好

- **结论**：以聚簇的**完整成员标签**（并上代表标签）对照本期各板块的 `cluster_tag_ids`；命中的板块里取贴题档最好者，同档取话题非候选者。
- **落点**：Scenario「跨板块取最好一档」
- **取舍**：被否「只用代表标签 `cluster_id` 定位一个板块」——代表标签是簇内最小 id，不代表最贴题的成员（11-14 一簇同落近档与远档两个板块），只看它会误判。

### D3. 统一「贴题档」0–5，先筛与后排共用一把键

- **结论**：0 = `l1_direct`；1 = `watch_` 前缀；2 = `l2_llm` 且话题非候选；3 = `l2_llm` 且候选；4 = `l3_new`；5 = 正文无对应板块 / 无档。候选池的「贴题 / 补位顺序」和落库前的展示序都只看这一个数。
- **落点**：Scenario「贴题簇充足时不贴题的大簇不进候选」
- **取舍**：被否「先筛用一套阈值、后排用另一套」——两套口径会在边界上打架（某簇被当成贴题放进池子却排到最后，或反之）。

### D4. 贴题池 = 近档 + 手动关注；手动关注排在近档之后（用户 2026-11-27 拍板）

- **结论**：贴题档 0 与 1 算贴题簇；手动关注命中排在近档之后、中档之前。
- **落点**：Scenario「手动关注命中排在近档之后」
- **取舍**：被否「把中档里挂在在跟话题下的也算贴题池」——用户没选；近 15 期中档在跟话题很少出现，两种选法绝大多数日子结果一样，取更严的；被否「手动关注排最后」——正文把关注板块放末位是版面叠加的考虑，不代表贴题度低，用户明确关注的内容不该排在二档的跑题新闻后面；被否「排最前」——会压过版块主叙事。

### D5. 先筛后挑：改的是候选池的组成，模型仍然挑

- **结论**：在既有候选序之上按贴题档组候选池，再交给模型挑 ≤3 条；提示词、校验、重试、快照不变。既有候选序的三个排序键不动——实现时把比较器函数体抽成 `OrderEventClusterCandidates`（完整序，不截断）供组池复用，`RankEventClusterCandidates` 只做截断委托、保留给既有测试与外部调用方。
- **落点**：`backend-go/internal/topicgraph/service/daily_report_story_highlights.go:highlightCandidatePool`
- **取舍**：被否「只改输出序」——挑出来就那 3 条，跑题的挪到后面也赶不走（11-26 三条里两条关税），用户当面否掉；被否「规则直接定 3 条、模型只写文案」——2026-11-20 D13 已否决，会退化成条数榜，而且像 11-26 两个「胡塞袭沙特炼油厂」讲的是同一件事，模型能分辨、规则不能。

### D6. 贴题簇不足补位到 3 个；贴题簇为空维持前 8

- **结论**：贴题簇 1–2 个 → 全部保留，并按（贴题档升序、既有候选序）从其余聚簇补位到池内共 3 个；贴题簇为 0 → 池子 = 既有候选序前 8。
- **落点**：Scenario「贴题簇为空时维持原候选口径」
- **取舍**：被否「一律补位到 8 个」——补位的多了，看不到贴题档的模型可能挑了补位、丢下贴题的；被否「贴题簇为空也只给 3 个」——新板子还没有已成立话题时全是新簇，等于把模型的编辑空间从 8 砍到 3。

### D7. 同档内只比文章数，section 数与泳道热度不参与

- **结论**：贴题档只决定进不进池子与先后梯队，梯队内部完全沿用既有候选序。
- **落点**：Scenario「同分与精度不反转」
- **取舍**：无备选：2026-11-20 已定「候选排序不用 section 数或泳道热度替代篇数」，本 change 只叠加相关度门槛，不动这条。

### D8. 信号来源：落库后新开一个轻量只读查询

- **结论**：新增按 report id 读板块信号的投影查询（字段：id / cluster_index / bucket_index / cluster_tag_ids / lane_tier / topic_status_at_report），写法仿 `ListReportEventThreads`。
- **落点**：`backend-go/internal/topicgraph/repository/report_section_signals.go:ListReportSectionSignals`（写法仿 `report_event_clusters.go` 的 `ListReportEventThreads`）
- **取舍**：被否 ① 复用内存 `inputs.clusters`——不含关注物化板块、没有 `topic_status_at_report` 快照，与落库口径会分叉；② 用 `GetReportByID`——会 Preload threads、折叠副本并当场改写 `report.Highlights`，副作用不可接受。

### D9. 查询失败整体退回现状，不阻断

- **结论**：读板块信号出错时，候选池取既有候选序前 8、重点按候选序落库，记一条含 report id 与原因的 Warn 日志。
- **落点**：Scenario「读取板块信号失败退回原口径」
- **取舍**：无备选：既有「highlights 单点失败不阻断日报」的契约（Requirement「日报 highlights 生成健壮性」）不允许因排序失败丢重点。

### D10. 不新增落库字段

- **结论**：候选池与排序的结果只体现在重点的成员与顺序上，不在重点上存贴题档快照。
- **落点**：无代码落点：`proposal.md`「What Changes」的不做清单与 delta spec Scenario「旧重点快照读取兼容」，因为这是限定实现不得扩张范围的约束，本身不产生代码。
- **取舍**：被否「在每条重点上存贴题档快照」——当前没有任何消费方展示它；板块快照已持久化，需要解释顺序时可按 report 回查。

### D11. 标签回退模式不接候选池规则

- **结论**：零候选走标签回退时维持现状（模型数组原序）。
- **落点**：Scenario「零候选走标签回退」
- **取舍**：被否「回退模式也按板块信号组池」——回退重点没有 `cluster_id`，标签与板块多对多，映射规则要另定；零候选是罕见路径，YAGNI。

### D12. 不改模型提示词

- **结论**：贴题门槛只作用在服务端组池，不把贴题档写进给模型的事实。
- **落点**：无代码落点：`proposal.md`「What Changes」的不做清单与 delta spec Scenario「模型只从白名单挑且不输出数字」，因为提示词与响应契约保持原样是实现不得扩张范围的约束，本身不产生代码。
- **取舍**：被否「把贴题档塞进提示词让模型自己往后排」——模型排序不受控（红线 46 要求服务端定序），而先筛已经保证跑题的不进池，没必要再多一层。

### D13. flow 文档同步：今日重点节与红线 46 改写

- **结论**：红线 46 的候选口径改为「贴题簇优先、不足补位到 3、为空维持前 8」，顺序表述改为「按贴题档稳定排序，同档保持候选序，仍非模型序」；「今日重点的选题与热度」节补贴题档、候选池、回退与回放结论。
- **落点**：5.1
- **取舍**：无备选：§12 文档流转要求 `docs/reference/flow/` 反映新代码状态。

## 核心逻辑（伪代码）

```go
// ── 仓储：轻量只读投影（新）──
type SectionSignal struct {
    ID                  int64
    ClusterIndex        int
    BucketIndex         int
    ClusterTagIDs       []int64 // jsonb 数字数组 → []int64；勿用 jsonb `?` 文本比较
    LaneTier            string  // l1_direct | l2_llm | l3_new | watch_* | ""
    TopicStatusAtReport string  // active | candidate | ""
}

func ListReportSectionSignals(ctx context.Context, reportID int64) ([]SectionSignal, error)
// SELECT id, cluster_index, bucket_index, cluster_tag_ids, lane_tier, topic_status_at_report
// FROM daily_report_sections WHERE report_id = ? ORDER BY cluster_index, bucket_index

// ── 服务：贴题档（先筛与后排共用）──
func relevanceRank(cluster EventCluster, signals []SectionSignal) int {
    best := 5
    for s := range signals {
        if !intersects(s.ClusterTagIDs, cluster.TagIDs) { continue } // 以候选簇完整成员标签为准
        r := 5
        switch {
        case s.LaneTier == "l1_direct":               r = 0
        case strings.HasPrefix(s.LaneTier, "watch_"): r = 1
        case s.LaneTier == "l2_llm":                  r = 2; if s.TopicStatusAtReport == "candidate" { r = 3 }
        case s.LaneTier == "l3_new":                  r = 4
        }
        if r < best { best = r }
    }
    return best
}

// ── 服务：候选池（先筛）──
// ordered = 既有候选序的完整聚簇列表（用既有比较器排序但不截断）
func selectCandidatePool(ordered []EventCluster, signals []SectionSignal, maxCandidates, minPool int) []EventCluster {
    ranks := make([]int, len(ordered))
    for i := range ordered { ranks[i] = relevanceRank(ordered[i], signals) }

    theme := indicesWhere(ordered, func(i int) bool { return ranks[i] <= 1 })
    if len(theme) == 0 { // 贴题簇为空：维持既有口径
        return head(ordered, maxCandidates)
    }
    pool := theme[:min(len(theme), maxCandidates)]
    if len(pool) < minPool { // 补位：其余聚簇按（贴题档升序，既有候选序）稳定取
        rest := sortByRankStable(indicesNotIn(theme), ranks)
        for len(pool) < minPool && len(rest) > 0 { pool = append(pool, ordered[rest[0]]); rest = rest[1:] }
    }
    return pool // 池内顺序：贴题簇（既有候选序）在前，补位簇在后
}

// ── 服务：展示序（后排兜底）──
func rankPickedHighlights(picked []Highlight, pool []EventCluster, signals []SectionSignal) []Highlight {
    rankOf := map[clusterID]int; for c := range pool { rankOf[c.ClusterID] = relevanceRank(c, signals) }
    sort.SliceStable(picked, func(i, j int) bool { return rankOf[picked[i].ClusterID] < rankOf[picked[j].ClusterID] })
    return picked
}

// ── 接线（改 finalizeDailyReportHighlights）──
func finalizeDailyReportHighlights(ctx context.Context, report *Report, inputs Inputs) error {
    clusters := buildClusters(...)                              // 不变
    if len(clusters) == 0 { return legacyTagFallback(ctx, report, inputs) } // 不变

    ordered := repository.OrderEventClusterCandidates(clusters)        // 既有比较器，完整序不截断
                                                // （勿用 RankEventClusterCandidates(…, n)：limit 上限仍是 8）
    signals, err := repo.ListReportSectionSignals(ctx, report.ID)
    var pool []EventCluster
    if err != nil {
        log.WarnContext(ctx, "report highlight relevance signals unavailable", "report_id", report.ID, "err", err)
        pool = head(ordered, maxEventClusterCandidates)         // 退回现状：前 8
    } else {
        pool = selectCandidatePool(ordered, signals, maxEventClusterCandidates, 3)
    }

    picked, err := generateAndValidate(ctx, report, pool)        // 不变：输出即池内顺序
    if err != nil { return err }
    if len(picked) > 1 && signals != nil {
        picked = rankPickedHighlights(picked, pool, signals)     // 后排兜底
    }
    return FinalizeReportHighlights(ctx, report.ID, picked)      // 不变：title/summary = 首条
}
```

边界：`ordered` 为空 → 走标签回退（不变）；贴题簇为空 → 前 8（现状）；贴题簇 1–2 个但当天再无其他聚簇 → 池子就是这 1–2 个；`picked` 为空或 1 条 → 后排无操作；板块信号为空数组（当天没有任何板块）→ 所有簇贴题档为 5 → 贴题簇为空 → 前 8（现状）；同一板块被多个聚簇命中 → 各自独立取档。

## Risks / Trade-offs

- [贴题簇多但都只有 1 篇，而当天有一条 10 篇的跑题大新闻 → 大新闻进不了重点] → 用户明确接受「跑题的别进来」；想看它在正文里仍在，只是不上刊头。
- [模型在池子里仍可能只挑 1 条甚至返回空] → 现状如此（「信息不足宁可不生成」），本 change 不改。
- [新板子或尚无已成立话题/关注的版块 → 全是远档 → 贴题池为空] → 维持现状前 8，不影响新板子的冷启动。
- [`topic_status_at_report` 为空的历史板块] → 贴题档把缺失当非候选，不惩罚（11-22 两期验证）。
- [同包有进行中的 change（`lane-first-recall`）在改 `daily_report_lane.go` / `daily_report_orchestrator.go` / `daily_report_lane_recall.go`] → 本 change 只碰 `daily_report_story_highlights.go` 与新仓储文件；派发前跑 `concurrency-status.sh`，影响包测试若因对方中间态变红按 [中间态] 处理，不混淆归属。
- [`cluster_tag_ids` 是 jsonb 数字数组] → Go 侧按 `[]int64` 解析；SQL 里别用 jsonb `?` 做文本比较（数字元素恒为 false），需要时用 `jsonb_array_elements_text`。
- [首条重点变化连带 `title` / `summary` 变化] → 既有派生关系不变，消费方无需改。

## Migration Plan

无迁移、无新配置。部署后新生成或重跑的日报即生效；历史日报原样不动。回滚：回退代码即可，读路径不改写已落库的顺序。
