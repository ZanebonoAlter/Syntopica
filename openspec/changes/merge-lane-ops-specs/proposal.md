<!-- complexity: medium -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, daily-report, topic-graph -->

# Proposal: 泳道操作/工坊视图两族合并（6 → 2）

## Why

泳道/画布 9 个 spec 共 3392 行，但只有两组是真碎片，其余三个本身已经是内聚单元：

| spec | 行数 | 判断 |
|---|---:|---|
| workshop-board-console | 877 | 单页内聚——矩阵与关键事件视图共享观察窗、日聚焦、日期轴，拆开会互指 |
| lane-correction-surface | 728 | 单工作面内聚——174 行那条就是「修正工作面是主战场」的总纲 |
| workshop-lane-compose | 572 | 编排画布完整契约 |
| **lane-manual-correction** | 318 | 泳道操作四件套之一 |
| **board-lane-dynamics** | 248 | 泳道动态视图 |
| **lane-trend-overview** | 108 | 日报里的泳道趋势区块 |
| **lane-heat-metric** | 104 | 热度量纲（矩阵排序的唯一口径出处） |
| **workshop-radar** | 219 | 工坊首屏雷达 |
| **workshop-article-archive** | 246 | 文章档案视图 |

## What Changes

**6 → 2**：

| 族 | 吸收 | 行数 | Requirement |
|---|---|---|---:|
| `lane-ops`（新建） | lane-manual-correction + board-lane-dynamics + lane-trend-overview + lane-heat-metric | 778 | 23 |
| `workshop-board-console`（就地扩容） | workshop-article-archive | 1106 | 33（25 原有 + 8 新增） |

判据：

- **`lane-ops`**：四者都是「泳道这条线怎么被看、被改、怎么量」——人工重设首义与归属变更（改）、滚动 14 天动态视图与趋势区块（看）、热度量纲（量，且是矩阵排序与雷达分档的唯一口径）。改「泳道怎么排序」时，`lane-heat-metric` 与 `board-lane-dynamics` 必然一起动
- **`workshop-board-console` ← `workshop-article-archive`**：archive 的原文写着「版块态势页 SHALL 提供『归档』视图，**与『矩阵』同层切换**」——它是 board-console 页面**内**的第三个视图，不是独立页面。board-console 的矩阵条款也明确「『查看归档』动作保留可达」

**为什么 `workshop-radar` 保持独立（R-5 更正）**：最初把 radar 与 archive 归成一个「工坊视图」族，理由写的是「平级视图 + 共享入口跳转」。实测两条都不成立——radar 是 `/tags` **未选中版块时**的跨版块首屏，archive 在版块**之内**；两者双向 grep 零命中。真实拓扑是 `radar ← board-console → archive`，把拓扑两端捆一起而把中间留外面，新族内部没有任何共享条款。

## 不做的事

- **不拆 `workshop-board-console`**（吸收 archive 后 1106 行）：矩阵、关键事件、归档三个视图共享「观察窗与自由日期显示」「日聚焦卡片列」「日报日驱动的日期轴」「面包屑回 `/tags`」——拆开会让改一次日期轴要开多个文件。1106 行在 1200 预警线下
- **不并 `lane-correction-surface`（728）与 `workshop-lane-compose`（572）**：合并后 1300 行超 1200 预警线。两者确实共享「吸顶标签列」「画布高度贴合容器」「缩放与滚动外壳同源」，但共享的是**外壳**（约 3 条 80 行），把两个完整工作面捆一起不划算。`workshop-lane-compose` 已有一条 Requirement 明确「吸顶标签列是两支画布共用的单元」，这就是当前的跨族约束表达方式
- 不拆 877 / 728 / 572 这三个（均 < 1200 预警线，且内部围绕单一工作面）
- 不改 spec 内容

## Impact

- **spec 数量**：150 → 146（退役 5 个 + 新建 1 个；`workshop-board-console` 就地扩容）
- **历史追溯**：指向 5 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段
- **代码**：零改动
- **用户可见行为**：无
- **回退**：`git revert`

## 后续（不在本 change）

`workshop-board-console`（1106）与 `lane-correction-surface`（728）是下一批拆分候选。前者天然分界是「矩阵 + 归档」vs「关键事件视图」，后者是「画布布局」vs「修正交互」。
