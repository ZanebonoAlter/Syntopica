<!-- ui-impact: none -->
<!-- diagrams: full(3 族的归位与"日报全链路"是两条不同轴，文字说"生成→阅读→运维"读者以为按时间排；画出来才看出族是按 spec 契约切的) -->
<!-- constraint-domains: daily-report -->

# daily-report 链路设计按 3 族重排

## Why

`docs/reference/flow/daily-report.md` 555 行，链路设计 **322 行（58%）/ 14 块**。和刚做完的 semantic-board 同一个病：

| 现状 | 具体 |
|---|---|
| 按 change 切片 | 标题带 `add-daily-report-gap-backfill` / `add-event-story-heat` / `retire-story-adopt-event-metrics` / `widen-highlight-cluster-candidates` / `unify-lane-workbench` / `rework-workshop-into-radar-and-desk` / `thread-relations` / `replace-lane-lifeline-with-topic-events` |
| 按技术层排 | 9 块「后端：…」+ 4 块「前端：…」——技术层不是功能族 |
| 信息密度不均 | 「日报生成管线」37 行、「泳道态势结算」4 行、「话题关键事件注记」4 行 |

**具体痛点**：想查「日报的今日重点怎么选题」要在 322 行里找 `story highlights` 那块；想查「缺口补填为什么不能覆盖已有报告」要翻到 `gap backfill` 块再读一段引用。

## What Changes

### 1. 按 spec 的 3 族重组（`daily-report` 的 spec 本来就是三族）

| 族 | spec | 收哪些块 |
|---|---|---|
| **一、生成与匹配** | `daily-report-system`（23 req） | 生成管线 / Section↔话题锚定 / 事件线归属与跨期去重 / 今日重点选题 |
| **二、对外契约与阅读** | `daily-report-interface`（18 req） | 日报阅读台 / 线索相关推荐 / 话题盯盘 / Section 可视化与三套匹配血缘 |
| **三、运维与补档** | `daily-report-ops`（7 req） | 运维 backfill 端点 / 缺口与补填 / 泳道态势结算 / 话题关键事件注记 / 候选读取 |

**族顺序按「读日报的人的认知顺序」**：先怎么生成（一）→ 怎么读（二）→ 出问题怎么修（三），而不是原文的「后端 / 前端」。

### 2. 每族两层，与 semantic-board 同一套规则

- **当前规则**：现在必须遵守的规则与阈值。「**不得做 X**」类禁令属现行约束，留在这层
- **决策记录**：change 名 + 日期 + 为什么这么做 / 踩过什么坑

判据沿用上一轮 review 校准过的：**换一种做法会不会错**——会错就是现行规则，不管它是不是在讲历史。

### 3. 红线节与其余节原样搬

`## 业务约束与不变量`（**73 条红线**，全仓最多）节名与内容逐字不动；`## 代码入口`、`## 变更溯源` 同样不动。

## 不做的事

- **不补 `daily-report-margin-notes` 那一族**：spec 有 8 条 Requirement（报纸页边注：划选批注 + AI 作答），但 flow 里**没有对应的链路块**——只在阅读台布局里提到 `MarginNotesRail.vue`。这是 flow 的覆盖缺口，补它属于「写新内容」，不在重排范围，记入已知残留
- **不改任何文字**：只搬位置与层级。上一轮 review 的 R-1 教训——「现行约束被当历史搬走」是最大风险，这次逐段核对
- **不合并到 spec**：族是阅读入口，spec 是契约
- **不动其他 9 个 flow 文档**

## Impact

- **改动**：`docs/reference/flow/daily-report.md` 链路设计节
- **零代码 / 零前端 / 零后端**
- **链接安全**：先查有无 `daily-report.md#锚点` 引用
- **风险**：322 行搬迁丢内容。四道对账：正文行零丢失 / 20 术语 grep 一致 / 73 条红线注入逐字相同 / 四节逐字相同

## 已知残留

- **`daily-report-margin-notes` 无链路块**：spec 有 8 条 Requirement，flow 只在布局里提一句
- **`daily-report-interface` 与 `lane-*` 的边界模糊**：候选读取、阅读台都涉及泳道侧组件，族归属按「这段讲的是日报还是泳道」判
- **过期文本原样保留**：搬迁前旧债，不在本 change 范围
