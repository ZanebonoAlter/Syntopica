# test-cases: widen-highlight-cluster-candidates

> ⓪ 契约盘点：本 change REMOVED 主 spec「事件线热度先筛重点候选」（事件线热度链路已退役的文档债）+ ADDED「今日重点聚簇候选」（现行聚簇口径 + composite 放宽）。旧资产反查：`bash scripts/harness/test-assets.sh daily-report-system`（2026-11-15 执行，输出见下「继承与调整」）。

## 故事 S1: 当天最大的同事件热点（哪怕被聚成一条 composite 线索）必须能进「今日重点」候选并凭文章数排首位（锚 Requirement: 今日重点聚簇候选）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 取当天线索（含 composite）做聚簇输入 | 单事件多标签大簇进候选 | composite 线索的 17 个 event 标签 + 证据文章同权返回；report 隔离、id ASC 不变 | testcontainer PG（repository 禁 SQLite） | `backend-go/internal/topicgraph/repository/report_event_clusters_test.go` `TestListReportEventThreadsIncludesComposite` |
| 2 | 同事件多表述标签聚为一簇；混编标签隔离 | 单事件多标签大簇进候选 / 混编标签自成小簇不污染 | 3 个同事件标签聚一簇（代表 = 最小 id）；混编标签自成单簇、不并入单事件簇，簇文章数按自身证据去重 | testcontainer PG + stub embedder（service） | `daily_report_event_cluster_composite_test.go` `TestDailyReportEventClusterCompositeBigThreadRanksFirst` / `TestDailyReportEventClusterMixedTagStaysOwnCluster` |
| 3 | 候选排序与截断（Top 8） | 候选按簇内文章数截断 | 去重文章数 DESC → 簇内标签数 DESC → 代表标签 id ASC；大簇（20 篇）排首位（标签 id 更小的 1 篇小簇不得占首）；第 9 簇不进 | 函数单测 + PG | `report_event_clusters_test.go`（排序链）+ 既有 `report_story_highlights_test.go` `TestRankEventClusterCandidatesOrderAndLimit` |
| 4 | LLM 从 ≤8 白名单挑 ≤3、不输出数字 | 模型只从白名单挑且不输出数字 | 集外 cluster_id / 越界 tag_ids / 空白文案判非法同参重试一次；合法 `[]` 不重试；展示序 = 服务端候选序 | 函数单测（stub chat） | 既有 `service/daily_report_highlights_cluster_test.go` `TestHighlightsClusterRejectsOutOfWhitelist` / `TestHighlightsClusterPromptAndSnapshot`（本 change 不改，回归网） |
| 5 | 服务端补快照 | 同上 | `cluster_id` / `article_count`（去重篇数）/ `cluster_mode` / `data_as_of` / `research_at` 全部服务端计算 | 函数单测 | 既有 `TestHighlightsClusterPromptAndSnapshot` + `TestEventHighlightSnapshot`；新测试断言大簇 `article_count=5/20` |
| 6 | 回退边界 | 全 composite 日报不再走标签回退 / 零候选走标签回退 | 全 composite 含 event 标签 → 聚簇重点（cluster_id 非空）；线索（含 composite）均无 event 标签 → 标签回退（cluster_id 空、不伪造快照） | testcontainer PG + stub chat | `TestHighlightsCompositeOnlyReportNotTagFallback` + 既有 `TestHighlightsClusterFinalizeAndTagFallback` |
| 7 | 旧快照读取兼容 | 旧重点快照读取兼容 | 旧 highlights 原样返回、不回填；重建按新口径重算 | 既有回归网 | `repository/daily_report_repository_test.go`（highlights 恒数组/归一）+ `front/.../DailyReportMasthead.test.ts`（缺字段不显示）；人工：真库 report 1473 现行 highlights 读取如常（effect-check 已核对） |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 输入·空集 | report 无线索 / tagHits 空 | 零候选 → 标签回退（`GenerateClusterHighlights(nil)` 不调模型直接空数组） | 函数单测 | 既有 `TestHighlightsClusterRejectsOutOfWhitelist` 零候选分支 |
| 输入·重复元素 | 同 link 多 feed 副本 | 簇内文章按内容身份去重只算 1 篇（红线 49） | PG | 既有 `TestDailyReportEventClusterEndToEnd`（3 行证据=3 篇）+ 新 repo 测试（21 行=20 篇） |
| 输入·越界引用 | 其它 report 的线索；已删除标签（无 topic_tags 行） | report 隔离不放松；删除标签静默缺席、存活标签照常参与 | PG / 真库 | 新 `TestListReportEventThreadsIncludesComposite`（隔离）；effect-check（17828 的 17 标签→15 存活） |
| 输入·部分满足前置 | 混编标签与部分单事件标签相似 | 相似度门槛承担防混编：超阈值自成小簇，落阈值内则并入（既有调参，观察项） | 函数单测 | `TestDailyReportEventClusterMixedTagStaysOwnCluster` |
| 输入·空串/纯空白/分隔符/大小写/特殊字符/超长/单 token | —— | 不适用（划除留痕）：输入是 DB 关系数据非自由文本；标签文本异常走既有表示构造失败分支（空名称显式失败、不静默截断） | 函数单测 | 既有 `TestDailyReportEventClusterBatchSplitsAndRepresentationFailure` |
| 时间窗口·跨窗口 | 昨日线索的标签 | 不进当天候选集（按 report_id 隔离，天然无「当天算不算」边界歧义——划除留痕） | PG | 既有 `TestDailyReportEventClusterEndToEnd`（yesterday 线索） |
| 时间窗口·归一化 | data_as_of | = NormalizeReportDate(报告期) | PG | 既有 `TestDailyReportEventClusterEndToEnd` 断言 |
| 幂等·重复执行 | 聚簇纯内存、确定性 union-find | 重复计算结果一致；业务表零写入 | PG | 既有 `TestDailyReportEventClusterOrderIsDeterministic` + EndToEnd 零写入断言 |
| 幂等·重建覆盖 | 同板同日重跑整份覆盖 | 约束 1 既有行为，本 change 不触碰 | PG | 既有 `daily_report_repository` 系（回归网）；迟到写入保护 `FinalizeReportHighlights` CAS（既有） |
| 可用性·错误态 | embedding 网关失败 / 覆盖不全 | 三级降级如实标注口径、不阻断日报 | PG | 既有 `TestDailyReportEventClusterDegradation` |
| 可用性·空态 | 零候选 | 标签回退 / `[]`，前端「本期暂无重点」占位（约束 41） | 既有回归网 | `TestHighlightsClusterFinalizeAndTagFallback` + `DailyReportMasthead.test.ts` |
| 可用性·误输入反馈 / 加载态 / 超长文本 / 重复提交 | —— | 不适用（划除留痕）：纯后端 change，无新 UI 交互；前端消费面零改动 |

### 效果核对

见本目录 [`effect-check.md`](effect-check.md)（四要素齐备）。摘要：真库 report 1473（board 15220, 2026-11-12）黄仁勋簇 15 个存活 event 标签、去重 20 篇，次名候选 2 篇——新口径下白名单首位 = 黄仁勋簇（20 ≫ 2）；旧口径头条为 1 篇小簇（现行 highlights 三条 article_count 全 = 1 为证）。端点重建因 11-12 在窗口内（实际核对日 2026-11-15，brief 假设的 11-20 超窗被拒场景不成立）且伴随覆盖现存报告 + 真实 LLM 调用，未执行，留待部署新后端后重建观察 LLM 头条选择（D3）。

### 继承与调整（⓪，REMOVED「事件线热度先筛重点候选」逐行处置）

反查来源：`test-assets.sh daily-report-system` §1 主 spec 该 requirement 仍存（本 change delta 偿还文档债）；§2 archive `2026-11-06-surface-story-heat-in-report-top`（含 test-cases）与 `2026-11-12-retire-story-adopt-event-metrics`；§3 历史 Scenario→测试映射。

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 候选按热度而非桶数截断 | 废止（热度候选已随事件线退役；排序键改为簇内去重文章数） | `repository/report_story_highlights_test.go`（retire 时已改写为 `TestRankEventClusterCandidatesOrderAndLimit`） | 照跑（改写后版本守聚簇排序）+ 新增 `TestListReportEventThreadsIncludesComposite` 排序链锚点 |
| 同分与精度不反转 | 改语义（tie-break 换为标签数/代表 id；精度问题随浮点热度退役消失） | 同上（已改写） | 照跑 |
| 来源窗口与半衰期 | 继承但移出本故事（热度公式仍服务 lane_heat/雷达域） | `repository/story_heat_test.go` | 照跑（回归网，不属本 requirement） |
| 只可从候选事实写文案 | 改语义（白名单 = 聚簇候选，事实 = 簇内标签 facts） | `service/daily_report_highlights_cluster_test.go`（retire 时迁移为 `TestHighlightsClusterPromptAndSnapshot` 等） | 照跑 |
| 无事件线走标签回退 | 改语义（触发条件收敛为「聚簇零候选」；全 composite 含 event 标签不再回退） | 同上（场景二） | 照跑 + 新增 `TestHighlightsCompositeOnlyReportNotTagFallback` 双场景 |
| 少量候选与合法空返回 | 改语义（聚簇模式同款：合法 `[]` 不重试、两次非法报错） | 同上 `TestHighlightsClusterRejectsOutOfWhitelist` | 照跑 |
| 重点快照与旧数据兼容 | 继承（快照字段族换成 cluster 系后语义不变：旧快照原样、缺字段 `omitempty`） | `repository/daily_report_repository_test.go` + `front/.../DailyReportMasthead.test.ts` | 照跑（回归网） |

另：`service/daily_report_story_highlights_test.go` 中归属硬停写测试（`TestStoryAttributionHardStop` 等）不属本 requirement，不动。

### 白盒附加（复杂档：scope 过滤删除点的输入集合变化）

## 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| B1 | report 无线索 | 0 条 thread | 空输入 → 零候选 → 回退（旧行为不变） | 既有零候选分支 |
| B2 | 全 single 线索 | 仅 single | 全返回（旧行为不变） | `TestDailyReportEventClusterEndToEnd` |
| B3 | 含 composite 且含存活 event 标签 | 17828 型线索 | **同权进入**（本 change 唯一行为变化点） | `TestListReportEventThreadsIncludesComposite` / `TestDailyReportEventClusterCompositeBigThreadRanksFirst` |
| B4 | 全 composite 且含 event 标签 | 当天仅 composite | 全部进入、不走回退（旧：空 → 回退） | `TestHighlightsCompositeOnlyReportNotTagFallback` 场景一 |
| B5 | composite 携带非 event（keyword）标签 | tag_ids 无 event 类 | 进入投影但 `ListEventTagsByIDs` 过滤后零候选 → 回退（不变） | `TestHighlightsCompositeOnlyReportNotTagFallback` 场景二 |
| B6 | 跨 report 线索 | 邻日/邻报 | 排除（隔离不随放宽放松） | `TestListReportEventThreadsIncludesComposite` |
| B7 | composite 的 tag_ids 含已删除标签 | 17 中 2 个无行 | 存活标签照常参与，删除标签静默缺席（`ListEventTagsByIDs` 既有过滤） | 真库 effect-check（15/17 存活） |
| B8 | composite 证据与 single 线索共享文章 | 同文章多线索引用 | 簇内按内容身份去重，不重复计数 | `TestDailyReportEventClusterEndToEnd`（first=3 篇不变） |

## 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 聚簇相似度阈值 0.86 | −ε / 恰好 / +ε | 不成簇 / 成簇（边界含）/ 成簇 | 既有 `TestDailyReportEventClusterThresholdBoundary` |
| 候选上限 | 8 / 9 簇 | 第 9 簇不进（标签数再多也不进） | 既有 `TestRankEventClusterCandidatesOrderAndLimit` |
| 聚簇标签预算 | 501 标签 | 按命中数截前 500、如实标注 | 既有 `TestDailyReportEventClusterTruncatesOverBudget` |
| embedding 批大小 | 34 标签 | 32+2 两批 | 既有 `TestDailyReportEventClusterBatchSplitsAndRepresentationFailure` |

## 不适用划除（留痕）

- 输入变体的自由文本条目（空串/纯空白/纯分隔符/单 token/大小写/特殊字符/超长）：输入为 DB 关系数据，标签文本质量由打标域约束；聚簇侧仅保留「空名称表示构造失败」既有分支。
- 时间窗口「当天算不算」边界：候选按 report_id 圈定而非日期窗口比较，无跨窗歧义。
- 并发幂等：聚簇纯内存无共享态；落库并发（约束 1）由 SaveReport/FinalizeReportHighlights 既有 CAS 承担，非本 change 辖区。
- UI 可用性组：纯后端 change（ui-impact: none），前端消费面零改动。
