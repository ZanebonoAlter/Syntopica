
## 需求与产品决策：A 方案范围、为什么只对域型泳道生效

**现象**：域型泳道（`AGENT编程工具相关`、`LLM生态相关新闻`）把当天全部线索塞进一条 section。实例：board 3030 / 2026-10-23 / section #7149 = 18 tags + 17 threads（做游戏、插件、MCP、harness、Skill、安全、推理基础设施混在一起）。

**根因链**：① section 的产生单位是「话题 × 天」——`daily_report_lane.go::assembleLaneGroups` 按 topic 聚合出唯一 ClusterGroup；② 质心 = 近 30 条 section 等权均值，是多主题平均值（实测到自身质心 0.098 vs 到板块全局均值 0.120，相关系数 0.685）；③ L1 阈值 0.18 已失效（近 14 天 87%~100% 的 section 距离 <0.18）；④ auto_new 用当天 section 标题当候选话题 label。

**问题分层（复审用）**：问题只出现在域型泳道板块——近 14 天单条 section max tags：3030=25、1980=15、2165/2197=13、2272=9、15220=7、**1974=5**、其余 ≤4。叙事型板块每天本就散成 5-6 条小 section，永远够不到阈值。

**用户拍板的范围（2026-10-24 讨论）**：
- 做 A 方案（泳道内主题分桶，产出同话题多条 section）；**B 方案（子桶跨天身份 / parent_topic_id）暂时不做**，留待以后。
- 桶标题要显示（方向名 + 当日章节标题都要看到）。
- 泳道视图按现在来（`BoardThreadBrowser.vue` 不改）；这几块页面后续要统筹优化，另开 change。
- 要「重新分桶」入口。
- UI 声明为 minor（复用现有子卡渲染位，不新增页面/路由/主视图模式）。

**引用**：docs/research/lane-intra-bucketing/explore-findings.md、openspec/changes/add-intra-lane-theme-bucketing/design.md

<!-- pinned 2026-10-24T05:50:47Z -->

## 实现必读：单 section 结构在哪、下游已支持到哪、哪些护栏必须加

实现本 change 必须知道的既有事实（全部已核实）：

1. **一个话题一天只能出一条 section 是 `assembleLaneGroups` 造成的**（`map[topicID]*acc`），不是数据模型限制。
2. **下游早已支持「同话题同日多条」**：库内已有 86 个实例（来自手动编排与 watch_sentence 物化，`topic_match_confidence=manual`）；`BoardThreadBrowser.vue::laneLayout` 按单天最大节点数撑高；`DailyReportTopicSection.vue:278` 只在 `group.sections.length > 1` 时渲染子标题；`dailyReportMagazine.ts::groupSectionsByTopic` 已把同 topic 的多条收进同一 group 的 `sections[]`（渲染顺序就是数组顺序，需按 bucket_index 重排）。
3. **命中计数不会被拆分灌水**：`assignAndUpdateTopics` 用 `hitTopicIDs` 集合，`planLifecycle` 对每个 topic 只 `hit_count+1 / consecutive_hits+1`，与 section 条数无关（不需要改代码，只需 spec 定死）。
4. **质心按 section 逐条计入**：`ComputeTopicCentroid` 取 `persistent_topic_id = ?` 的最近 `centroid_window` 条 section 均权平均，拆得越细质心越纯（正向副作用），不需要特殊处理。
5. **合并护栏必须加**：`daily_report_merge.go::sameAnchorClass` 允许同 topic 合并，拆分产物会被粘回去（当前 `daily_report_section_merge_enabled` 默认 false，但不能依赖默认值）。
6. **泳道动态排序现在是 section 条数**：`lane_snapshot_repository.go` 里 `a.count++`（每条 section）→ `SectionCount14d` → 排序键；拆分会让拆得多的泳道虚高，需改成 `len(a.dates)`（有锚定天数）。前端消费字段名是 `section_count_14d`（`front/app/api/laneDynamics.ts`）。
7. **重新分桶的牵连面**：section id 会变 → `topic_key_event_markers.source_section_id`（`ON DELETE SET NULL`）必须按 `source_thread_id` 重派生；`daily_report_section_relations` 需按组重建；`topic_key_event_markers`/关键事件推荐/泳道趋势都以 thread 或 topic 为锚，thread 不删即可保。
8. **`SaveReport` 同日重跑会整份删旧 sections/threads 再写**（`daily_report_repository.go` 约 L261），所以重新分桶必须自己走组级事务，不能复用整份重建。
9. **配置读取模式**：`DefaultPersistentTopicConfig()` 里已有 `LaneL1Threshold/LaneL2Threshold/...`，新配置项同族追加（键名当前未落 `embedding_config` 表，走代码默认值）。

**引用**：backend-go/internal/topicgraph/service/daily_report_lane.go:assembleLaneGroups、backend-go/internal/topicgraph/repository/daily_report_assignment.go:planLifecycle、backend-go/internal/topicgraph/service/daily_report_merge.go:sameAnchorClass、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:LaneDynamicsLane、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:group.sections、front/app/features/tags/components/daily-report/dailyReportMagazine.ts:groupSectionsByTopic

<!-- pinned 2026-10-24T05:50:47Z -->

## 实验证据：单次分桶丢线索、两阶段稳定、阈值与 k 规则的由来

实验数据（12 次真实调用，样本 section #7149 / #6766 / #6736 / #7010 / #7102 / #7103；工具 `go run ./cmd/verify-lane-bucketing -section <id> [-k N] [-fixed "a,b,c"] [-temp 0]`）：

- **单次自由分组不可用**：同一输入 8 次运行，桶数 6~9 漂移；2 次静默丢线索（coverage 14/17、15/17）；多次出现单例桶；`-k N` 指令无效（k=4→6 桶，k=6→9 桶）。
- **两阶段稳定**：固定组名 + 只做归属，7149 三次全部 17/17 覆盖、桶数恒 6（temp=0 下两次逐条一致）；6766 两次逐条完全一致（9/9）。
- **输入粒度**：threads 优于 tags——tags 变体产出 9 桶 + 4 个单例桶。
- **k 规则**：`k = clamp(round(lines/2.5), 2, 6)` 在 4 个样本全中甜点（17→6、9→4、7→3、10→4）；按 tag 数算 k 是错的。
- **阈值保护**：threads≥6 或 tags≥10 → 近 14 天 51/621 条 section（8.2%，约 3.6 条/天）触发，集中在 3030(15) 与 1980(18)；1974 #7102（4 线索）强行分桶会切成 3 组 2 单例 → 必须靠阈值挡住，不能交给模型判断。
- **质量上限**：最好一次输出「零基础做产品 / 工具调用与编排 / Agent 落地实践 / Skill 与规范生态 / 安全与风控 / 推理与决策基础设施」，与用户直觉（做游戏、做工具、harness、skill）吻合。
- **成本**：每次 1~3s、约 2.2k tokens；两阶段 ≈ 7-8 次调用/天。
- **组名漂移**：「Skill 与规范生态」/「Skill 与插件生态」/「Skill 与安全规范」同义不同名 → A 方案可接受（天级桶），B 阶段做跨天身份前必须先解决命名归一。

**引用**：backend-go/cmd/verify-lane-bucketing/main.go、openspec/changes/add-intra-lane-theme-bucketing/design.md

<!-- pinned 2026-10-24T05:50:52Z -->
