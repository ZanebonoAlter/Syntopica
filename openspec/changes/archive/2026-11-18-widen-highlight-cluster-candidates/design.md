# Design — widen-highlight-cluster-candidates

## Context

现行链路（探索+真库核验，详见 `docs/research/daily-report-topic-ranking/explore-findings.md`）：今日重点候选 = `ListReportEventThreads`（`WHERE COALESCE(scope,'') <> 'composite'`）取线索 → 按 related-event 向量聚簇（三级降级）→ 簇内内容身份去重文章数排序 → Top 8 → LLM 挑 ≤3。`scope` 由「线索内 event 标签数 ≥2」推导，本意抓跨事件混编综合稿，但单事件多表述标签（2026-11-12 黄仁勋 17 标签 20 篇）同样触发 ≥2 → 误伤出局。当日候选池仅剩 1 篇小簇，头条被「豆包接入新模型」（1 篇）占据。聚簇输入单位是标签、相似度门槛天然防混编，入口级 scope 排除属于重复防御且有系统性盲区。正文分区墙（active → candidate）为产品语义，本 change 不动（2026-11-15 用户拍板）。

## Goals / Non-Goals

**Goals:**
- 单事件多标签的大热点簇能进今日重点候选白名单并凭文章数排首位。
- 混编标签不污染单事件簇（由聚簇相似度门槛保证，且需测试佐证）。
- spec「事件线热度先筛重点候选」文档债一并还清（对齐现行聚簇口径）。

**Non-Goals:**
- 不动 `scope` 列、推导规则及「不计事件数/不进事件线」语义。
- 不动正文排序、质量分区、话题组投影（原 rank-topics 方向已废弃，2026-11-15 dry-run 证明用户案例卡在重点候选层与分区墙，分区墙保留）。
- 不动聚簇三级降级链、候选排序键、Top 8 截断、LLM 白名单约束。
- 不回填已落库 highlights 快照。

## Decisions

### D1. 放宽位置在 ListReportEventThreads 的 scope 过滤，一处改动
- **结论**: 删除 `ListReportEventThreads` 查询中的 `COALESCE(scope,'') <> 'composite'` 条件，聚簇输入自然扩大到全部线索的 event 标签；下游聚簇、排序、快照零改动。
- **落点**: backend-go/internal/topicgraph/repository/report_event_clusters.go:ListReportEventThreads
- **取舍**: 在聚簇 service 层重新拉全部线索再过滤（绕过 repository 现成入口、重复查询）被否；新增独立查询函数（同库两份近似查询，漂移风险）被否。

### D2. 防混编依赖聚簇相似度门槛，不引入标签级 scope 判定
- **结论**: 跨事件混编标签（「今日科技要闻」类）与任一单事件标签向量距离超阈值 → 自成小簇，不并入单事件簇；其簇文章数按自身关联文章去重计算，即使数值偏大也只是白名单普通候选，由 LLM 事实锚约束兜底。不新增「标签级混编识别」。
- **落点**: backend-go/internal/topicgraph/service/daily_report_event_cluster.go:clusterByVectors
- **取舍**: 引入标签级混编识别（新概念新阈值，无真库证据表明必要——先观察）被否；按线索 scope 白名单放行「同事件」类 composite（需判定标签同事件性，等价于把聚簇职责搬到入口，复杂度更高）被否。

### D3. 效果核对锚定「候选白名单第一簇」，不强绑 LLM 头条选择
- **结论**: 效果核对断言服务端可确定的部分：重建/重算 2026-11-12 后候选白名单第一簇 = 黄仁勋簇（去重文章数 ~24）。LLM 从 ≤8 白名单挑 ≤3 条存在选择波动，验证时以其选中黄仁勋、且头条（第一条）为大热点簇为主验收，不达则记录重试，不把 LLM 单次输出定为硬门槛。
- **落点**: Scenario「单事件多标签大簇进候选」
- **取舍**: 断言「头条必为黄仁勋」（把 LLM 行为写死进验收，波动即假红）被否。

### D4. 全量 composite 日报的回退语义随入口放宽自然变化
- **结论**: 标签回退触发条件从「无 single 线索候选」收敛为「聚簇零候选」（线索均无 event 标签或聚簇失败）；`GenerateHighlights` 回退分支本身不改，仅触发面缩小。
- **落点**: backend-go/internal/topicgraph/service/daily_report_story_highlights.go:GenerateClusterHighlights
- **取舍**: 保留旧回退触发（全 composite 仍回退——直接违背本 change 目标，全 composite 当天大热点将被埋）被否。

## Risks / Trade-offs

- [混编标签若向量恰好落入单事件簇阈值内，会把跨事件文章并入大簇导致 article_count 虚高] → 聚簇阈值是既有调参（未动）；变体测试覆盖「混编标签超阈值自成小簇」；上线后观察重点条目是否出现跨事件文案混编（LLM 事实锚也会拒写混编因果）。
- [重建历史日报的重点会变化（候选池扩大）] → 预期内行为，spec Scenario「旧重点快照读取兼容」已写明；不重建则旧快照原样。
- [composite 线索参与聚簇后候选标签总量上升，聚簇计算量微增] → 单版块当天 event 标签量级为几十，向量聚簇为内存计算，成本可忽略。
- [LLM 仍可能不选大簇当头条] → 候选序即展示序、白名单首位为大簇；D3 效果核对记录 LLM 实际选择，若系统性跳过大簇再议服务端强制首位（超出本 change）。

## Migration Plan

- 部署：后端发版即可，无迁移。新报告即时生效；历史报告需重建（现有 backfill/generate 端点）才按新口径重算重点。
- 验证路径：本地对 2026-11-12 触发该板重建 → 断言候选白名单首位为黄仁勋簇 → 观察 LLM 头条选择。
- 回滚：恢复 `ListReportEventThreads` 的 scope 过滤即可；已重建报告的重点快照若需还原，按原口径再重建一次（幂等覆盖）。

## Open Questions

- 无（分区墙/正文排序方向已于 2026-11-15 经 dry-run 与用户对齐排除；LLM 头条选择波动按 D3 处理）。
