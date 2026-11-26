# Design: 泳道操作/工坊视图两族合并

## D1 为什么只并两组

9 个 spec 里，6 个是真碎片，3 个本身内聚。判断依据仍是「改 A 时是否几乎必然要动 B」：

| 组合 | 连带 | 结论 |
|---|---|---|
| lane-manual-correction ↔ board-lane-dynamics ↔ lane-trend-overview ↔ lane-heat-metric | 强：改「泳道怎么排序/怎么量热度」，热度量纲（唯一口径）、动态视图的排序、矩阵排序同时动 | **同族** |

| **workshop-board-console ↔ workshop-article-archive** | 强：archive 原文「版块态势页 SHALL 提供『归档』视图，与『矩阵』同层切换」——同一页面内的两个视图，共享面包屑、观察窗、日期轴 | **同族（就地扩容）** |
| workshop-radar ↔ 任何 | 它是 `/tags` 未选中版块时的跨版块首屏，与 archive 双向 grep 零命中 | 保持独立（R-5 更正） |
| lane-correction-surface ↔ workshop-lane-compose | 只共享外壳（吸顶标签列、高度贴合、缩放滚动同源），约 3 条 80 行 | 保持两族 |

**结论：不是所有族都该合并。** 前三轮的「合并」是有目标的收敛，不是「把相邻的都塞一起」。

## D2 lane-heat-metric 为什么进 lane-ops 而不是 board-console

`lane-heat-metric` 说自己是「雷达关注度分档、泳道行序与矩阵排序的**唯一口径出处**」——它同时喂 `workshop-radar`（关注度分档）、`workshop-board-console`（矩阵排序）、`board-lane-dynamics`（泳道行序）。

它无法两全，必须选一个。选 `lane-ops` 的理由：

- 它是**量纲定义**，不是视图。放视图族会变成「矩阵族里的口径条款」，而 `board-lane-dynamics` 也在用这个口径
- `lane-ops` 里 3 个都是泳道展示（动态视图、趋势区块、热度量纲），放一起后「改泳道怎么算、怎么排、怎么显示」在一个文件里闭环
- `workshop-board-console` 引用它时，现状本来就有跨族指针（它的「泳道热度口径」那条），合并后指针不变

## D3 为什么不把两支画布合并

合并后 1300 行超 D5 的 1200 预警线。共享部分只有 3 条（吸顶标签列、高度贴合、缩放滚动外壳同源），约 80 行。

**当前约束表达方式**：`workshop-lane-compose` 有一条 Requirement 明确写「吸顶标签列是**两支画布共用**的单元」——跨族共享靠这句话维持，而不是靠同文件。

若未来两者继续长，正确做法是抽一个 `canvas-shell`（约 100 行），而不是把两个工作面捆一起。

## D4 排序与分类

沿用两段式（business 在前、impl 在后标 `<!-- kind: impl -->`）。两个族都是**新建** capability，所以两段式完整适用（不像就地扩容那样受「原有条目留在原位」限制）。

预判 impl 候选：
- `滚动窗口成员取数的单点实现`（lane-manual-correction）——若只讲「从哪取、怎么取」偏 impl
- `展示字段数据溯源`（lane-correction-surface，不在本族）——同类
- `泳道动态聚合端点`（board-lane-dynamics）——若是纯端点定义，可能偏 interface；但它在本族内作为数据来源说明，保留

## D5 归档机制

1. `.openspec.yaml` 声明 `retire_capabilities: true`
2. 归档前摘掉 **6 个**被退役 spec 的 `<!-- domain: -->`
3. 归档后补 marker（R-2 更正）：
   - `lane-ops` → **`semantic-board, daily-report, topic-graph`**（3 域，正好上限）。原方案漏了 `topic-graph`——`lane-manual-correction` 的 marker 是 `semantic-board, topic-graph`，且 lane-ops 里躺着 topic-graph 红线 23/24/25 的对应条款（质心刷新范围、手动建话题留痕、滚动窗口单点实现）。按 doc-authoring「按改这族必须读哪些 flow 红线取舍」的判据，topic-graph 不可省
   - `workshop-board-console` 保留原 marker `semantic-board`（archive 原 marker 也是 `semantic-board`）
4. 两个新族都是新建，Purpose 由 `merge-spec-family.sh` 的 PURPOSE 字典带入（缺则脚本报错退出）

## D6 已知损失

- 指向 6 个被退役 capability 的 delta 落 `domain-history.sh` 第 6 段
- `lane-heat-metric` 被 `workshop-board-console` 引用（「泳道热度口径」那条），合并后该引用仍指向 `lane-ops` 的对应条款，不会断
- `lane-correction-surface` 的 Purpose 目前是 `TBD - created by archiving change ...`（历史遗留），本 change 不动它，登记为内容问题
## D7 归档陷阱：表格与散段会让 retire 拒绝删除（R-1，实测）

`openspec archive` 的 retire 路径会对**整份原 spec** 做「哪些行能被合并流程点名」的审计，只有标题 / `## Purpose` / `## Requirements` 头 / 每个 requirement 的标题+首段+scenario 子弹可被点名。**markdown 表格行**与**首段之后的额外散文段**都算 unaccounted content → 拒绝删除整个 spec。

`lane-manual-correction` 两类都有（20 行表格 + 尾部散文），实测导致归档连续中止两次。清理后（内容已在 lane-ops 的 ADDED 块里逐字保留）才通过。

**操作要求**：
1. 归档前清掉该 spec 的表格行（`grep -c '^|'` 应为 0）
2. **不要**先把 spec 删空——`counts.removed = 0` 会让 `decideSpecOutcome` 返回 `write` 而非 `retire`，归档同样中止。必须保留 requirement 块，只删不可点名的行
3. 六项门禁（archive-readiness）**不含 dry-run**，实测六项全绿而归档仍中止 → 这类坑只会在最后一步暴露

