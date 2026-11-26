<!-- complexity: medium -->
<!-- ui-impact: none -->
<!-- constraint-domains: topic-graph, semantic-board, daily-report -->

# Proposal: 话题/watch 两族 spec 合并（4 → 2）

## Why

`topic-graph` 域里 4 个 spec 共 1229 行讲的是两套紧耦合的功能，却被切成 4 个文件：

| spec | 行数 | 实际 |
|---|---:|---|
| persistent-topic | 717 | 持久叙事话题层的模型/归属/升级/生命线 API/泳道节点 |
| topic-key-event-markers | 168 | 用户在泳道上标记「值得长期记住的事」 |
| topic-watch | 142 | 用户声明「我在追踪什么」——提示轨 |
| watch-materialized-topic | 202 | 同一份 watch 的物化轨（关键字轨 / 一句话轨） |

`topic-watch` 的 Purpose 自己就写着「每期日报生成时以**两种形态**兑现：提示轨……物化轨（见 watch-materialized-topic）」——**它自己承认这两半是一件事**，却分成两个 spec。同理 `topic-key-event-markers` 的「条目标记交互」「已确认节点的来源语义」直接长在 `persistent-topic` 的泳道节点上。

## What Changes

**4 → 2**，两组各自就地扩容：

| 族 | 吸收 | 行数 | Requirement |
|---|---|---|---:|
| `persistent-topic`（就地扩容） | topic-key-event-markers | 885 | 26 |
| `topic-watch`（就地扩容） | watch-materialized-topic | 344 | 13 |

判据：

- 改**话题模型 / section 归属 / 升级生命周期 / 泳道节点呈现 / 关键事件标记** → 只碰 `persistent-topic`
- 改**watch 的命中判定 / 物化轨 / 日报栏位** → 只碰 `topic-watch`

## 不做的事

- **不并 `related-events`**（122 行）：它是「为阅读事件标签的用户推荐相关事件」，消费方在阅读页，不涉及话题建模。并进来会让「改话题模型」的人无端读到事件检索阈值
- **不并 `data-enrichment`**（684 行）：它是数据增强/认知工作台（agent loop、见解分级、报告追问），域声明本来就是 `data-enrichment` 打头，与话题建模不同层
- **不拆 `persistent-topic` 的 717 行**：D5 阈值是 1500 硬限 / 1200 预警，717 在健康区间。真正的巨型是 `daily-report-system` 1417 与 `workshop-board-console` 862
- 不改 spec 内容（矛盾/过时条款记入 `review.md`）

## Impact

- **spec 数量**：152 → 150（退役 2 个 + 就地扩容 2 个，不新增 capability）
- **历史追溯**：指向 2 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段
- **代码**：零改动
- **UI**：`ui-impact: minor`——`topic-key-event-markers` 与 `watch-materialized-topic` 描述的节点标记交互、日报栏位契约原样搬运，不改字段不增删控件，但涉及用户可见契约故标 minor 而非 none
- **用户可见行为**：无变化（纯文件重组）
- **回退**：`git revert`
