<!-- stage: implement -->

# Tasks

## 1. 准备

- [x] 1.1 统计：persistent-topic 族 26 req / 120 sc / 885 行；topic-watch 族 13 req / 42 sc / 344 行
- [x] 1.2 反查创建 change：persistent-topic ← 2026-07-05-manual-topic-lane；key-event-markers ← 2026-10-15-add-topic-key-event-markers；topic-watch ← 2026-07-23-topic-watchlist-observability；watch-materialized-topic ← 2026-08-25-watch-materialized-topic
- [x] 1.3 确认 `related-events`（122 行）与 `data-enrichment`（684 行）不参与（前者是阅读页事件检索，后者是数据增强族）

## 2. 生成合并稿

- [x] 2.1 复用 `scripts/harness/merge-spec-family.sh`；**本轮补了「PURPOSE 缺失即报错」**（前两族踩过两次 TBD 占位）
- [x] 2.2 生成两族初稿
- [x] 2.3 守恒：ADDED 15 = REMOVED 15；39 = 15 搬移 + 24 保留（两族均就地扩容）

## 3. 人工校对

- [x] 3.1 `persistent-topic` 25 business / 1 impl
- [x] 3.2 `topic-watch` 13 business / 0 impl
- [x] 3.3 跨 spec 重复检查：合并后发现 **3 处同义重述**（R-8）——关键事件标记的「条目标记交互」与 persistent-topic 的「泳道节点可读性与就地详情」、物化轨「并排展示」与 topic-watch 的「日报顶部独立栏位」、watch 隔离条款在 topic-watch 与物化轨各一次。登记不修（改正文会破守恒）
- [x] 3.4 内容问题登记（本次不修）：
  - [x] 3.4a ~~红线 1 与 spec 冲突~~ **误报，已撤回**：原先引用的「不得出现无主 section」在 `docs/reference/flow/topic-graph.md` 中并不存在（`grep -c 无主` = 0）。真实红线的第 1 条是「planLifecycle 只更新命中计数、不得自动变更任何 status」，与 spec 的两处 NULL/unassigned 例外不冲突。**根因：本轮派发 brief 里的 5 条「红线」是手写的，未走 `constraint-digest.sh --brief --domains topic-graph`**——后续派发一律取脚本输出原样粘贴
  - [x] 3.4b 「历史数据回刷」**已改判 business**（R-3/R-4）：它是可重复调用的回刷端点（不是一次性脚本），且「在 SaveReport 之后、RebuildBoardRelations 之前」是长期顺序不变量。改判后本族 0 impl，39/0
  - [x] 3.4c `watch-materialized-topic` 被 `topic-watch` 的 Purpose 引用「见 watch-materialized-topic」，合并后自指

## 4. delta 与切换

- [x] 4.0 写 `.openspec.yaml`（`retire_capabilities: true`）
- [x] 4.1 生成 2 个族的 `## ADDED Requirements`
- [x] 4.2 生成 2 个旧 capability 的 `## REMOVED Requirements`
- [x] 4.3 归档前摘掉 **2 个**被退役 spec 的 `<!-- domain: -->`（topic-key-event-markers、watch-materialized-topic）；与 8.2 同一提交
- [x] 4.4 **归档后**断言 2 个目录已自动退役
- [x] 4.5 **归档后** `openspec validate --specs` → 150 全过（152 − 2 退役 + 0 新建）

## 5. 验证

- [x] 5.1 `domain-history.sh topic-graph` → 第 6 段列出指向 2 个旧 capability 的孤儿 delta
- [x] 5.2 `check-standards.sh` → 150 个 spec marker 全合法
- [x] 5.3 抽查 6 条来源注释，grep archive 确认属实
- [x] 5.4 确认 `related-events` / `data-enrichment` 未被触碰

## 6. review

- [x] 6.1 派 review（`minimax-cn/MiniMax-M3.1-Flash:max`，GLM 额度 13:17 重置前不可用）
- [x] 6.2 处置 review 发现

## 7. 文档

- [x] 7.1 `docs/reference/flow/topic-graph.md` 变更溯源表加本 change 一行
- [x] 7.2 `docs/research/process-arch-rework/issues.md` 回填 ISS-04（第四族）
- [x] 7.3 旧 capability 名引用扫描结果（R-6）：`topic-watch` 正文有 **3 处**指向 watch-materialized-topic（What Changes 段 + Purpose 自指），`daily-report-system` 有 **2 处**指向 topic-key-event-markers。全部登记到 tasks 7.4 的清扫批次
- [x] 7.4 **登记到「跨族引用清扫」change**（若 7.3 发现改 main spec 才能修的引用）

## 8. 归档

- [x] 8.1 `archive-readiness.sh merge-topic-specs` 六项全绿
- [x] 8.2 `openspec archive merge-topic-specs`
- [x] 8.3 归档后：
  - [x] 8.3a `persistent-topic` **手改 Purpose**（就地扩容时 delta 被忽略）
  - [x] 8.3b `topic-watch` **手改 Purpose**，并去掉「见 watch-materialized-topic」自指
  - [x] 8.3c 两族 marker 均保留原值（3 域），核验无丢域

## 9. 测试

零代码改动，验证即结构与守恒：

| 验证点 | 命令 | 期望 |
| --- | --- | --- |
| delta 结构 | `openspec validate merge-topic-specs` | valid |
| 全仓 spec | `openspec validate --specs` | 150 passed |
| 条数守恒 | ADDED 15 = REMOVED 15；39 = 15 + 24 | 恒等 |
| Scenario 守恒 | 120 + 42 = 162 | 恒等 |
| REMOVED 标题匹配 | 2 个 delta vs main spec 逐字 | 15/15 |
| 归档退役 | 归档后 spec 目录数 | 150，无空壳 |
| 未触碰项 | `git diff` on related-events / data-enrichment | 空 |

## 10. 文档

<!-- doc-impact: flow, research -->

- `docs/reference/flow/topic-graph.md`：变更溯源表加一行（7.1）
- `docs/research/process-arch-rework/issues.md`：ISS-04 进度回填（7.2）

## 11. 验收

- [x] A `persistent-topic` 26 req（19 保留 + 7 新增）、`topic-watch` 13 req（5 保留 + 8 新增）
- [x] B 15 条新搬入的 Requirement 带 `<!-- from: -->`，可 grep archive 验证
- [x] C 2 个旧 capability 已退役；`related-events` / `data-enrichment` 原样保留
