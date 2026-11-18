# Review — widen-highlight-cluster-candidates

| 项 | 值 |
| --- | --- |
| change | `widen-highlight-cluster-candidates` |
| 审查日期 | 2026-11-15 |
| 审查人 | 只读审查子线程（type A：openspec change 制品审查） |
| 审查对象 | `git diff` 可见的实现 diff + 新增测试 + `docs/reference/flow/daily-report.md` 本 change 改动段（200-223 / 346 / 439）+ change 目录全部制品 |
| 审查结论 | **需修复后归档**（H1 必须修；M2/M3 强烈建议；M4/M5/L1/L2/L3 建议同轮修掉） |

---

## 一、总评

**行为实现正确、制品质量总体高**，但**文档层留了一条与新行为直接冲突的活红线**，本 change 的立意（还清事件线时代文档债）因此只完成了一半。

**做得好的地方**：

1. **改动面收敛得干净**。设计 D1「一处改动」落地为真的「一处改动」：`ListReportEventThreads` 删一行 `Where("COALESCE(scope,'') <> ?", ThreadScopeComposite)`，另两个文件只改注释。`git diff` 中**没有任何超出 D1-D4 的实质代码改动**（越界检查通过）。
2. **入口唯一性已核实**。全仓 grep `ListReportEventThreads` 只有 **1 个调用方**（`service/daily_report_event_cluster.go:114` `buildDailyReportEventClusters`），确认聚簇输入扩大**只影响今日重点链路**，不会外溢到 lane_heat / story 归属 / 泳道快照 / 跨期去重（这四处各自有独立的 composite 过滤，未被触及）。
3. **「scope 语义不被误伤」核实通过**。scope 推导（`repository/story_repository.go:658-670`，event 标签数 ≥2 → composite）未改动；「不计事件数 / 不建事件线」链路（`daily_report_repository.go:688` 的 event 标签口径、`daily_report_story_dedup.go:69`、`story_reattribute.go:773`）均未受影响。
4. **测试设计有判别力**。service 侧 `TestDailyReportEventClusterCompositeBigThreadRanksFirst` 刻意「小簇先建（标签 id 最小）」再断言大簇仍排首位——这**能隔离出排序键是文章数而非标签 id**，是本 change 最关键的断言，做得对。
5. **测试分层合规**。新增的 `repository/report_event_clusters_test.go` 用 `testutil.SetupTestDB`（testcontainer PG），符合 `docs/reference/standard/backend/testing.md` 的「repository 包禁内存 SQLite」硬约束；stub embedder / stub chat 保证不烧真实模型。
6. **effect-check 诚实**。重建路径**未执行**（说明了三项越权副作用），改走只读 SQL + 服务层测试路径，并把「tier-1 现算向量未烧」如实标注为口径限制，而不是假装验过。
7. **制品完备度高**。test-cases.md 五问句齐（⓪契约盘点与逐行处置表 / 主链路 7 步 / 变体走查 12 组 + 不适用划除留痕 / 效果核对 / 白盒分支表 B1-B8 + 边界值清单 4 项）；effect-check.md 四要素齐（触发原因 / 方法 / 量化结果 / 结论）；test-cases 引用的 9 个既有测试锚点**逐一 grep 确认全部存在**。

**主要问题**：红线层 42 与新行为正面冲突（High），主 spec 与 flow 文档各留一段退役旧口径未清（Medium ×2），effect-check 的量化口径未写清限定词（Medium）。

---

## 二、发现清单

### R-1 2026-11-15【High】flow 红线 42 仍禁止 composite 进簇，与新行为正面冲突（原 H1）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`docs/reference/flow/daily-report.md:342`（红线 42）
- **问题**：该红线原文至今未改，仍写着：

  > 42. **事件身份列退为只读历史列：新生成 / 重建的日报 `story_id` 恒 NULL、`story_manual` 恒 false、`display_tier` 恒 body；历史值只读保留、对外不透出；`scope` 仍由 `tag_ids` 中 event 标签数推导（≥2=composite），composite 不进任何聚簇、不计簇内篇数**（…）：…；综合稿是多事件混合，算进任一簇都会虚高，故 SHALL NOT 进簇、SHALL NOT 计簇内篇数。

  而本 change 的新行为恰恰相反：composite 线索的 event 标签与证据文章**同权进入聚簇**，且其证据文章**计入簇内内容身份去重文章数**（`service/daily_report_event_cluster.go:114-127` 的 `articleByTag[tagID] = thread.ArticleIDs`；`service/daily_report_event_cluster_test.go` 的旧断言 `second.ArticleCount` 由 0 改为 1 正是这一行为翻转的证据）。

  本 change 只更新了红线 46（`:346` 加了「composite 线索的标签与证据同权计入」），**红线层因此自相矛盾**：46 说进簇、42 说不得进簇。

  危害有三层：
  1. **约束注入层面**：constraint-injection 会把红线 42 原样注入后续会话的 system prompt，未来任何基于该文档的工作都会被要求「composite SHALL NOT 进簇」，与本仓现行代码相反——这是典型的红线债反向作用。
  2. **审计层面**：同一份文档里两条红线对同一事实给出相反的 SHALL/ SHALL NOT，谁优先无从判定。
  3. **立意层面**：本 change 的 proposal/design 明确把「主 spec 旧口径文档债」列为要还的债，而红线 42 正是同一条债在 flow 侧的残留，却只还了 spec 侧。

- **处置建议**：**必须修**。建议改法（红线 42 内部最小改动，不动 `story_id`/`story_manual`/`display_tier` 停写部分）：

  > …；`scope` 仍由 `tag_ids` 中 event 标签数推导（≥2=composite）；**composite 线索的 event 标签与证据文章自 widen-highlight-cluster-candidates 2026-11-15 起同权参与「今日重点」聚簇并计入簇内内容身份去重文章数**（跨事件混编标签由聚簇相似度门槛隔离自成小簇）；「composite 不计事件数、不建事件线」的语义不变。

  同时在 delta spec 的 ADDED requirement「今日重点聚簇候选」里补一句显式取代声明（例如「本 requirement 取代原红线 42 的『composite 不进任何聚簇、不计簇内篇数』条款」），使 spec 与 flow 的取代关系双向可追。注意：delta spec 现有的「composite『SHALL NOT 计入事件数、SHALL NOT 建立事件线』的既有语义 SHALL 不变」措辞虽然严谨（只保事件数/事件线），但**只说了「不变」的一半**，没有说「聚簇口径被取代」的那一半，建议一并补上。

---

### R-2 2026-11-15【Medium】主 spec 残留旧口径「Call A：热度 top 8 事件线 / story_id」（原 M2）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`openspec/specs/daily-report-system/spec.md:200`（Requirement「日报分段并行生成」正文第一条）
- **问题**：该行仍是事件线时代的描述：

  > - **Call A（今日重点）**：输入热度 top 8 **事件线**及其本期标签事实，输出含 **story_id**/title/reason/tag_ids 的重点；归属失败或无可用线时 SHALL 选择既有标签聚类回退模式，**story_id 为空**

  本 change 的 delta REMOVED「事件线热度先筛重点候选」并 ADDED「今日重点聚簇候选」，但**没有动这一条**。归档后主 spec 将同时存在：
  - 新 requirement：候选 = 当天 event 标签聚簇 Top 8，模型不输出数字，零候选走标签回退；
  - 旧 requirement 内的 Call A：候选 = **热度 top 8 事件线**，输出含 **story_id**。

  且 Call A 与现行实现直接冲突：`story_id` 已被红线 42/43 定死为「恒 NULL、不再产生」，事件线归属链已硬停写，「归属失败」这一分支不存在。

- **处置建议**：**建议修**（成本极低，一行措辞）。改为现行口径即可，例如：

  > - **Call A（今日重点）**：输入当天同事件聚簇 Top 8 候选（簇内去重文章数降序 → 簇内 event 标签数降序 → 代表标签 id 升序）及其簇内标签事实，输出含 cluster_id/title/reason/tag_ids 的重点；聚簇零候选时 SHALL 选择既有标签聚类回退模式，cluster_id 为空、story_id 恒为空

  若决定不在本 change 内改，请在 `tasks.md` / `effect-check.md` 的遗留观察项里显式登记（否则它会随归档一起被「洗白」成现行 spec）。

---

### R-3 2026-11-15【Medium】flow 实现索引段指向 6 个已删退役符号（原 M3）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`docs/reference/flow/daily-report.md:368`（同段 367「后端事件线与热度」一并属退役口径）
- **问题**：该段整条 bullet 描述的是 `surface-story-heat-in-report-top` 时代的实现落点，引用了以下符号，实测在 `backend-go/` 中**全部 0 命中**（已删除）：

  | 符号 | 实测 |
  | --- | --- |
  | `boundedReportAttribution` | 仅剩 `daily_report_story_highlights.go:163` 一句「已移除」的注释 |
  | `storyHighlightCandidates` | 0 |
  | `GenerateStoryHighlights` | 0 |
  | `ReportStoryCandidates` | 0 |
  | `rankReportStoryCandidates` | 0 |
  | `YesterdayHighlightStoryIDs` | 0 |
  | `resolveReportHighlightIdentities` | 0 |

  `repository/report_story_highlights.go` 文件仍在，但现只剩 `resolveReportHighlightClusters` / `FinalizeReportHighlights` / `EventHighlight` 三个函数；同段提到的 `promptVersion 7.0` 路径亦已不在现行链路上。读者按此条定位不到任何现行实现。

  与 tasks 4.1 的声明有出入：任务写「顺手将 200-224 行过时段（`rankReportStoryCandidates`/`ReportStoryCandidates`）替换为现行聚簇口径」——同族的**实现索引段（367-368）被漏掉了**，200-224 行确实清干净了（该段已由本 change 重写为现行聚簇口径），但同一份文档里留下了指向死符号的孤儿条目。

  补充说明：该行**不在本 change 的 diff 块内**（属上一 change `retire-story-adopt-event-metrics` 的遗留债），所以严格说不是本 change 引入的回归；但本 change 的立意就是「文档债一并还清」，且 tasks 4.1 已在同一目标下动过邻近行，留着它属于「同一笔债还了一半」。

- **处置建议**：**建议修**。把 367-368 两段压缩/改写为现行聚簇口径的落点索引（指向 `service/daily_report_event_cluster.go` / `service/daily_report_story_highlights.go` / `repository/report_event_clusters.go` 的现行函数），或明确删除该段并指向 `:200` 的「今日重点的选题与热度」节。若要留给后续 change 处理，请在 tasks.md 里落一条**有主的**遗留项（谁改、什么时候改），不要只靠「反正历史段也没人看」。

---

### R-4 2026-11-15【Medium】effect-check 代理口径缺显式限定词（原 M4）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`openspec/changes/widen-highlight-cluster-candidates/effect-check.md:16, 28`
- **问题**：effect-check 用 `topic_tag_embeddings`（`embedding_type='semantic'`）的存量向量做 0.86 阈值边图 union-connectivity，得出「15 节点 67 条边、min pair 0.770 / avg 0.881、**连通分量 = 1**」。方法节已注明「tier-2 兜底层同款向量；tier-1 现算 related-event-v1 向量需真实 AI 调用，未烧」——但**量化结果表**里那一行只写「语义向量 0.86 边图 | 15 节点 67 条边…」，读者极易把它当成生产口径结论。

  口径差异是实质性的：tier-1 的比较对象是**现算**的 related-event-v1 向量，表示 = 名称 + 描述 + **aux 标签文本**（`daily_report_event_cluster.go` 的 `tagging.BuildRelatedEventRepresentation(tag.Label, tag.Description, auxLabels[tag.ID])`），与 tier-2 的存量 semantic 向量不是同一空间；67 条边 / 连通分量=1 对 tier-1 不必然成立。

  **补充一个能显著提升该结论可信度的论证（建议补进 effect-check）**：17828 的 20 篇证据是**挂在整条线索上**的——`articleByTag[tagID] = thread.ArticleIDs` 对该线索的每个标签都挂同一份证据集。因此**即使 tier-1 把这 15 个标签切成 k 个子簇**，每个子簇的簇内去重文章数都仍是 20；tier-1 与 tier-2 的差异只影响「簇的个数」，**不影响「首位候选 = 20 篇」**这一 D3 锚点（次名候选 2 篇，20 ≫ 2）。这条不敏感性论证能把结论从「代理口径下成立」升级为「口径无关地成立」，是当前 effect-check 缺的最关键一块。

- **处置建议**：**建议修**（两处小改）：① 量化结果表该行加口径限定，例如「代理口径 = tier-2 存量 semantic 向量（生产 tier-1 需部署后重建观察）」；② 结论节补上述「证据挂在整条线索上 ⇒ 首位判定对 tier-1/tier-2 不敏感」的论证。

---

### R-5 2026-11-15【Medium】Scenario「第 9 簇标签数最多」未字面覆盖（原 M5）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`openspec/changes/widen-highlight-cluster-candidates/specs/daily-report-system/spec.md`（Scenario「候选按簇内文章数截断」）↔ `backend-go/internal/topicgraph/repository/report_story_highlights_test.go:16-42`
- **问题**：Scenario 的 WHEN 是「本期聚出 10 个簇且去重文章数排序明确，**第 9 簇的标签数最多**」，THEN 是「第 9 簇 SHALL NOT 因标签数进入候选」。test-cases 把该 Scenario 的锚点指向既有 `TestRankEventClusterCandidatesOrderAndLimit`，但该用例的 10 簇夹具里**每簇都只有 1 个标签**（`TagIDs: []uint{uint(i)}`）——「标签数最多也不进」这一**条件本身**没有任何用例字面覆盖；被覆盖的只是「第 9 簇不进白名单」的结论。
- **处置建议**：**建议修**（1 行夹具）。在既有 `TestRankEventClusterCandidatesOrderAndLimit` 的 10 簇夹具里，给第 9 簇多挂几个 `TagIDs`（如 `[]uint{9, 91, 92, 93}`）并保持其 `ArticleCount` 最低，再断言它不进 top 8 —— 即字面兑现 Scenario 的 WHEN。

---

### R-6 2026-11-15【Low】代码注释日期与制品不一致（原 L1）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`backend-go/internal/topicgraph/repository/report_event_clusters.go:21`、`backend-go/internal/topicgraph/repository/report_event_clusters_test.go:3`、`backend-go/internal/topicgraph/service/daily_report_event_cluster_composite_test.go:3`
- **问题**：三处均写「widen-highlight-cluster-candidates（**2026-11-20**）」，而 `.openspec.yaml created: 2026-11-15`、flow 溯源行（`daily-report.md:439`）与 flow 正文口径（`:217`、`:221`、`:346`）均为 **2026-11-15**，本 review 日期亦为 2026-11-15。`2026-11-20` 疑为实现方 brief 的假定日期（effect-check.md 亦提到「brief 假设的 11-20」）。注释日期是变更溯源/审计锚点，同一 change 三处日期不同会削弱审计链。
- **处置建议**：**建议修**（改成 2026-11-15）。

---

### R-7 2026-11-15【Low】tasks 验证命令形式错误（原 L2）
- **状态**：已修复（主会话 2026-11-15 本轮修复）

- **位置**：`openspec/changes/widen-highlight-cluster-candidates/tasks.md:18`
- **问题**：任务写「验证：`bash scripts/harness/doc-impact.sh verify widen-highlight-cluster-candidates` 通过」。实测该命令**报错退出**：「FAIL: change 目录无效或无 tasks.md」——`verify` 子命令收的是 change **目录**。用正确形式 `bash scripts/harness/doc-impact.sh verify openspec/changes/widen-highlight-cluster-candidates` 执行**确实通过**（输出「声明: daily-report  文件:1 个  启发式:ownership」）。
  即：结论无误，只是任务里记的命令形式错了。
- **处置建议**：**建议修**（任务文本改目录形式）。不修也不影响本次审查结论。

---

### R-8 2026-11-15【Low】repository 层排序断言不具判别力（原 L3）
- **状态**：已采纳（留账观察，不阻塞归档）

- **位置**：`backend-go/internal/topicgraph/repository/report_event_clusters_test.go:72, 100-106`
- **问题**：该用例里小簇 `ClusterID=2000`、大簇 `ClusterID=1000`，因此「按去重文章数 DESC」与「按代表标签 id ASC」两种排序给出**同一结果**。注释里写的「去重文章数 20 > 1 → composite 大簇排候选白名单首位」在本层并未真正隔离出排序键；真正具备判别力的是 service 侧 `TestDailyReportEventClusterCompositeBigThreadRanksFirst`（小簇先建 → id 更小，仍断言大簇首位）。
- **处置建议**：**建议修**（二选一，成本极低）：把本用例的 `smallTagID` 改成小于 `bigTagIDs[0]`（如 500），或把 repo 层的排序断言删掉、只保留「composite 线索同权返回 + 内容身份去重 20 篇」，把排序判别完全交给 service 侧用例。避免注释宣称的内容强于断言实际覆盖。

---

### R-9 2026-11-15【Low】topicgraph/repository 包 SQLite 存量债（原 L4）
- **状态**：已驳回（存量债非本 change 引入，不并入本次）

- **位置**：`backend-go/internal/topicgraph/repository/story_repository_test.go:8,25,632`、`.../daily_report_centroid_test.go:11,121`
- **问题**：`docs/reference/standard/backend/testing.md` 的验收守卫是 `grep -rnE 'glebarez/sqlite|sqlite\.Open' backend-go/internal/<pkg>/repository/` **→ 零命中**，实测该包**非零命中**。本 change 新增的 `report_event_clusters_test.go` 用的是 testcontainer PG，**未引入新违规**；上述命中全部来自别的文件/别的 change 的存量。
- **处置建议**：**本 change 不修**（越界，且会污染本 change 的 diff 边界）。建议单独立项清理 topicgraph/repository 包的内存 SQLite 遗留。

---

### R-10 2026-11-15【Low】混编标签观测项缺登记（原 L5）
- **状态**：已采纳（留账观察，不阻塞归档）

- **位置**：`openspec/changes/widen-highlight-cluster-candidates/design.md`（D2 与 Risks 第 1 条）
- **问题**：composite 线索的证据挂在**整条线索**上（`articleByTag[tagID] = thread.ArticleIDs`），故该线索的**每一个**标签都继承同一份证据集。design D2 已承认「其簇文章数……即使数值偏大也只是白名单普通候选，由 LLM 事实锚约束兜底」，Risks 也登记了「混编标签若向量恰好落入单事件簇阈值内，会把跨事件文章并入大簇导致 article_count 虚高」。但制品里**没有对应的观测项**（上线后看什么、看哪儿）。
- **处置建议**：**可不修**（design 已登记、风险有 LLM 事实锚兜底）。建议在 effect-check.md「遗留观察项」补一条：上线后抽查重点条目的 `article_count` 与其簇内标签的实际关联文章是否一致、有无跨事件文案混编。

---

## 三、核实记录（只读验证命令与结论）

### 3.1 行为正确性

| 核实项 | 方法 | 结果 |
| --- | --- | --- |
| `ListReportEventThreads` 调用方全集 | `grep -rn "ListReportEventThreads"` 全仓 | 仅 2 处：定义 + `service/daily_report_event_cluster.go:114`（`buildDailyReportEventClusters`）→ **今日重点链路独占**，输入扩大不外溢 ✅ |
| scope 推导是否被改 | 读 `repository/story_repository.go:655-670` | 未改（event 标签数 ≥2 → composite）✅ |
| 其他 composite 过滤是否被波及 | grep `ThreadScopeComposite` | `lane_heat_repository.go:100`、`story_reattribute.go:773`、`lane_snapshot_repository.go:389`、`daily_report_story_dedup.go:69` 全是**各自独立查询**，未被触及 ✅ |
| 「不计事件数」口径 | 读 `repository/daily_report_repository.go:670-700` | 走 `topic_tags.status='active' AND category='event'`，与 scope 无关，未受影响 ✅ |
| 回退触发条件是否真收敛为「聚簇零候选」 | 读 `service/daily_report_story_highlights.go:186-216` | `candidates` 唯一来源 = `buildDailyReportEventClusters` → `len(candidates)==0` 才走 `GenerateHighlights`；全 composite 含 event 标签 ⇒ clusters 非空 ⇒ 走聚簇重点 ✅ 与 D4 一致 |
| 越界检查（diff 是否超出 D1-D4） | `git diff -U8` 三个代码文件 | 仅：删一行 `Where` + 三处注释同步。**无其他实质改动** ✅ |

### 3.2 spec ↔ 实现 ↔ 测试 一致性

| delta spec Scenario | 锚点 | 覆盖状态 |
| --- | --- | --- |
| 单事件多标签大簇进候选 | `TestDailyReportEventClusterCompositeBigThreadRanksFirst` | ✅ 真实覆盖（含「小簇 id 更小仍排第二」） |
| 混编标签自成小簇不污染 | `TestDailyReportEventClusterMixedTagStaysOwnCluster` | ✅ 真实覆盖（断言混编标签不入单事件簇、自身簇 article_count=2 按内容身份去重） |
| 全 composite 日报不再走标签回退 | `TestHighlightsCompositeOnlyReportNotTagFallback` 场景一 | ✅ 真实覆盖（chatMode=="cluster"、cluster_id 非空、cluster_mode=="向量聚簇"） |
| 候选按簇内文章数截断 | 既有 `TestRankEventClusterCandidatesOrderAndLimit` | ⚠️ 「第 9 簇不进」已覆盖，但 WHEN 里的「第 9 簇标签数最多」未字面覆盖 → 见 M5 |
| 模型只从白名单挑且不输出数字 | 既有 `TestHighlightsClusterRejectsOutOfWhitelist` / `TestHighlightsClusterPromptAndSnapshot` | ✅ 既有回归网覆盖 |
| 零候选走标签回退 | 新用例场景二 + 既有 `TestHighlightsClusterFinalizeAndTagFallback` | ✅ 真实覆盖 |
| 旧重点快照读取兼容 | 既有 `daily_report_repository_test.go` / `DailyReportMasthead.test.ts` | ✅ 既有回归网覆盖 |

- test-cases.md 引用的 9 个既有测试锚点**逐一 grep 确认全部存在**（`TestHighlightsClusterRejectsOutOfWhitelist` / `TestHighlightsClusterPromptAndSnapshot` / `TestEventHighlightSnapshot` / `TestRankEventClusterCandidatesOrderAndLimit` / `TestHighlightsClusterFinalizeAndTagFallback` / `TestDailyReportEventClusterThresholdBoundary` / `TestDailyReportEventClusterTruncatesOverBudget` / `TestDailyReportEventClusterBatchSplitsAndRepresentationFailure` / `TestDailyReportEventClusterDegradation`）✅
- 旧符号清理（全仓 grep `ReportStoryCandidates|rankReportStoryCandidates`）：代码侧 0 命中 ✅；文档侧唯一残留 = `docs/reference/flow/daily-report.md:368` → 见 M3。

### 3.3 测试质量

- **排序键断言**：service 层用例具备判别力 ✅；repository 层用例不具判别力 ⚠️（见 L3）。
- **testcontainer 合规**：新增 repository 测试用 `testutil.SetupTestDB`（pgvector testcontainer）✅，符合 `standard/backend/testing.md`「repository 禁 SQLite」硬约束；stub embedder / stub chat，不烧真实模型 ✅。
- **测试间状态泄漏**：`newClusterFixture` 在 `t.Cleanup` 里还原 `repository.Repo` / `dailyReportClusterEmbed` / `dailyReportClusterNow` 三个包级变量 ✅；`highlightsChatFn` 用 `defer` 还原 ✅；无用例调用 `t.Parallel()`（避免包级单例竞态）✅；用例内嵌 PG 隔离容器互不共享 ✅。
- **测试与实现的口径翻转点已留痕**：旧 `TestDailyReportEventClusterEndToEnd` 的断言从 `require.Zero(t, second.ArticleCount, "综合稿的 articleC 不计入…")` 改为 `require.Equal(t, 1, second.ArticleCount, "composite 线索的证据文章计入其标签的簇（同权）…")`——**这是本 change 行为翻转的直接证据**，与实现语义一致 ✅。

### 3.4 真库只读核实（localhost:5432/syntopica，只跑 SELECT）

| 核实项 | SQL 事实 | 与制品对照 |
| --- | --- | --- |
| report 1473 线索构成 | 12 条：9 single（各 1 标签 1 文章）+ 3 composite（17828: 17 标签 20 文章 / 17829: 2 标签 2 文章 / 17830: 2 标签 2 文章） | 与 explore-findings「composite 3 条 24 篇 / single 9 条 9 篇」一致 ✅ |
| thread 17828 的 17 个 tag_id | **15 行存在**（全部 `status=active`、`category=event`）+ **2 行已不存在**（id 275069 / 275279，硬删） | effect-check「15 存活 / 2 已删除不参与」属实 ✅；test-cases B7「删除标签静默缺席（`ListEventTagsByIDs` 既有过滤）」属实 ✅（该函数只按 `category='event'` 过滤，行不存在即缺席） |
| 17828 证据文章去重 | 20 行 / 20 distinct id / **20 distinct 归一 link** | effect-check「黄仁勋簇并集去重文章数 20 篇」属实 ✅ |
| 28 个候选标签的 tier-2 向量 | 每个标签 `embedding_type='semantic'` 向量数 = 1 → tier-2 覆盖完整 | effect-check「15/15 有向量」属实 ✅ |
| report 1473 现行 highlights | 3 条，`article_count` **全 = 1**；`title` = 「豆包接入新模型」；`cluster_mode` = 向量聚簇 | effect-check「头条被 1 篇小簇占据」属实 ✅（旧口径误伤证据成立） |

### 3.5 制品完备性

| 制品 | 检查 | 结论 |
| --- | --- | --- |
| proposal.md | Why / What Changes / Impact 三段齐；显式声明「不动 scope 列及其推导、不计事件数/不建事件线语义不变、不动降级链/排序键/Top 8/白名单/不回填」 | ✅ |
| design.md | Context / Goals+Non-Goals / D1-D4（含取舍）/ Risks / Migration / Open Questions | ✅ D1-D4 与实现一一对应，无空决策 |
| specs/daily-report-system/spec.md | REMOVED（事件线热度先筛重点候选，附 Reason/Migration）+ ADDED（今日重点聚簇候选，7 个 Scenario） | ✅ 结构合规；M5 为唯一覆盖缺口 |
| test-cases.md | ⓪契约盘点（7 行「继承与调整」逐行处置）/ 主链路 7 步 / 变体走查 12 组 + 不适用划除留痕 / 效果核对 / 白盒分支表 B1-B8 / 边界值清单 4 项 | ✅ 五问句齐 |
| effect-check.md | 触发原因 / 方法 / 量化结果（6 行表）/ 结论（含遗留观察项） | ✅ 四要素齐；M4 为限定词缺口 |
| tasks.md | 1-5 组已勾、6.1（派 review）待本轮 | ✅ 勾选与实际完成相符（5.1-5.3 已复跑通过） |
| ui-design.md | `ui-impact: none` / `ui-approval: not-required` / `ui-prototype: none` + N/A Reason | ✅ 与「纯后端、无前端改动」相符（diff 中确无前端文件） |

### 3.6 复跑的验证命令

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && go vet ./internal/topicgraph/...` | ✅ 通过（无输出，exit 0） |
| `go test ./internal/topicgraph/repository/ -run TestListReportEventThreadsIncludesComposite -count=1 -v` | ✅ PASS（22.76s，testcontainer PG 真跑） |
| `go test ./internal/topicgraph/service/ -run 'TestDailyReportEventClusterCompositeBigThreadRanksFirst｜TestDailyReportEventClusterMixedTagStaysOwnCluster｜TestHighlightsCompositeOnlyReportNotTagFallback｜TestDailyReportEventClusterEndToEnd' -count=1 -v` | ✅ 4/4 PASS |
| `openspec validate widen-highlight-cluster-candidates --strict` | ✅ `Change 'widen-highlight-cluster-candidates' is valid` |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/widen-highlight-cluster-candidates` | ✅ 通过（声明: daily-report / 文件:1 个 / 启发式:ownership）。注：tasks.md 里记的同名不带路径形式**不可用**，见 L2 |
| `bash scripts/harness/change-scope.sh` | ✅ 正常输出；建议验证命令含 `cd backend-go && go test -short ./internal/topicgraph/...`（与 tasks 5.1 一致） |
| `grep -rnE 'glebarez/sqlite\|sqlite\.Open' backend-go/internal/topicgraph/repository/` | ⚠️ 非零命中（存量债，非本 change 引入）→ 见 L4 |

> 未复跑全量 `go test ./...`（按 brief 说明，全量已由实现方跑绿，不强制重跑）。

---

## 四、结论

**需修复后归档。**

- **归档前必须做**：**H1**（红线 42 与新行为正面冲突；本 change 只还了红线 46，红线层自相矛盾，且会被注入后续会话）。建议连同 **M2**（主 spec Call A 旧口径）与 **M3**（flow 实现索引段的死符号）一并处理——这三条正是本 change 自述要还的「同一笔文档债」的三个面，只还一个面会留下更隐蔽的债。
- **建议同轮修掉（成本极低）**：M4（effect-check 加口径限定词 + 补「首位判定对 tier-1/tier-2 不敏感」的论证）、M5（补 1 行夹具字面兑现 Scenario）、L1（注释日期改 2026-11-15）、L2（任务里的命令改目录形式）、L3（repo 层排序夹具或注释对齐）。
- **本 change 明确不需要做**：L4（SQLite 存量债另立项）、L5（可只补一条观测项）。
- **实现侧无阻塞问题**：改动面最小且语义正确，入口唯一性、scope 其它语义不受波及、真库事实与 effect-check 全部对得上，新测试有判别力且分层合规。**修完文档三处矛盾即可归档**。

## 四、处置记录（主会话 2026-11-15）

| 条目 | 处置 | 落点 |
| --- | --- | --- |
| R-1 | 已修复：红线 42 改为「composite 不进事件线、不计事件数」+ 聚簇入口放宽说明（2026-11-15 起 composite 同权、混编归聚簇门槛隔离） | docs/reference/flow/daily-report.md 红线 42 |
| R-2 | 已修复：Call A 行改为聚簇口径（Top 8 候选、composite 同权、零候选回退 cluster_id 为空）；Scenario「并行生成成功」的「落库、归属与去重后」改「落库后」（归属/去重已随事件线退役） | delta spec 新增 MODIFIED「日报分段并行生成」 |
| R-3 | 已修复：实现索引段重写为现行符号（GenerateClusterHighlights / buildEventClusters / ListReportEventThreads / ArticleContentIdentities / RankEventClusterCandidates / resolveReportHighlightClusters），6 个退役符号清零（红线 43 中「不再有 boundedReportAttribution」为正确历史声明，保留） | docs/reference/flow/daily-report.md 实现索引 |
| R-4 | 已修复：补「首位判定对降级层不敏感」论证（17 标签同挂线索 17828，任意降级层聚入同一候选簇，20 vs 2 篇断层不受影响；差异仅簇边界） | effect-check.md 核对方法 4 |
| R-5 | 已修复：TestRankEventClusterCandidatesOrderAndLimit 夹具第 9/10 簇改 4 标签（其余 1 标签），断言仍不进前 8——「标签数最多也不进」字面覆盖 | backend-go/internal/topicgraph/repository/report_story_highlights_test.go |
| R-6 | 已修复：3 处代码注释 2026-11-20 → 2026-11-15 | report_event_clusters.go + 2 个测试文件 |
| R-7 | 已修复：tasks 4.1 验证命令改目录形式 `doc-impact.sh verify openspec/changes/widen-highlight-cluster-candidates` | tasks.md |
| R-8 | 已采纳：留账不修（不阻塞归档），后续测试巡检时补强 repository 层排序断言的判别力 | — |
| R-9 | 已驳回：SQLite 存量债非本 change 引入，不并入本次（review 自证） | — |
| R-10 | 已采纳：观测项与 design Risks 第 1 条一致，部署后重建观察重点条目有无跨事件混编文案 | design.md Risks |

**本轮复跑**：`go test ./internal/topicgraph/repository -run TestRankEventClusterCandidatesOrderAndLimit -count=1` ok（0.042s）；`openspec validate --strict` Change is valid（含新增 MODIFIED）；`doc-impact.sh verify` 通过；`2026-11-20` 残留仅剩本文件问题陈述内的合理引用（2 处）。
