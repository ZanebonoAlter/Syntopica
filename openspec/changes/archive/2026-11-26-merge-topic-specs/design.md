# Design: 话题/watch 两族合并

## D1 为什么是 4 → 2 而不是 4 → 1

两组各自内部承认自己是一件事：

**watch 组**——`topic-watch` 的 Purpose 原话：「每期日报生成时以**两种形态**兑现：提示轨（label / keyword）做只读命中判定并在日报顶部『我在追踪』栏集中展示……物化轨（keyword_topic / sentence_topic，见 watch-materialized-topic）」。同一个 watch 实体、同一条日报管线、同一批 API，拆成两个 spec 只因为物化轨是后来加的。

**topic 组**——`topic-key-event-markers` 的「人工选择关键节点」「已确认节点的来源语义」「条目标记交互」都长在 `persistent-topic` 的泳道节点上（后者的「当前泳道关键节点高亮」「泳道节点可读性与就地详情」是同一个界面）。改节点呈现必然两边一起动。

两组之间几乎无连带：watch 是「用户声明关注什么」，topic 是「系统把当天聚类持久化成话题」。故不并成一族。

## D2 为什么不并 related-events / data-enrichment

**`related-events`（122 行）**：Purpose 是「为阅读事件标签的用户提供基于名称、描述与辅助标签语义向量的直接相关事件推荐」，消费者在阅读页，且明确「不要求统一主体身份，不归并事件，不推断阶段或发酵」——是**事件检索**，与话题建模正交。

**但须收紧口径（R-9）**：它的 4 条里有 1 条是「日报同事件聚簇」，属日报生成期契约，与 `daily-report` 域有交叉。所以排除理由不是「完全不碰日报」，而是「它的主体是阅读页检索，只有 1 条涉及日报且是读数不写数」。并进来会让改话题模型的人读到在线阈值调整与历史样本验证，代价大于收益。

**`data-enrichment`（684 行）**：29 条 Requirement 全是 agent loop / 分层汇总 / 见解确定性分级 / 报告追问 / 素材新鲜度门，域声明是 `data-enrichment` 打头。它是**认知工作台**，属数据增强族，未来与 `board-research` 同批考虑。

判据统一为：**改这族时是否需要理解另一个领域的概念**。并了会导致「改话题归属要读 agent loop」，否定。

**同口径适用 `data-enrichment`（R-12）**：它的 29 条里没有任何一条约束话题身份/归属/生命周期，全是分析产出形态。排除理由不是「域声明不同」（那是结果不是理由），而是「改它不需要理解话题模型，改话题模型也不需要理解它」。

## D3 为什么不拆 persistent-topic 的 717 行

D5（doc-authoring 族级约定 + 前两族 design）定的阈值：

- 单族 > 1500 行 → 必须拆
- 单族 > 1200 行且含 ≥3 个不相干功能 → 考虑拆

717 行在健康区间。它的 19 条 Requirement 也确实围绕一件事（持久话题的生命周期），不是「3 个不相干功能塞一起」。

真正需要先拆的是 `daily-report-system`（1417）与 `workshop-board-console`（862）——那两族留到后面。

**合并后 885 行**（717 + key-event-markers 168 的结构开销），仍在 1200 预警线下，但已接近。

**更正（R-7）**：原先预判的拆分点「话题模型 vs 泳道呈现」与合并后实际内容不匹配——`persistent-topic` 的 19 条原有 Requirement 里，泳道呈现（高亮/可读性/边界/计数）只占 4 条（17-19 + 部分），新建的 key-event-markers 7 条才真正是泳道侧。若将来要拆，实际分界是**「话题身份与归属（模型/升级/API）」vs「话题在界面上的呈现（泳道节点 + 关键事件标记）」**，后者约 11 条。

## D4 就地扩容的两个代价

两族都是保留原名、只追加兄弟成员的 Requirement：

1. **Purpose 不会被 delta 覆盖**——openspec 对已存在的 spec 忽略 delta 的 `## Purpose`。归档后必须手工改写为新族级文案（tasks 8.3），否则读者看到的是旧文案，不知道新并入的关键事件标记/物化轨在哪
2. **原有条目留在原位、新增条目追加末尾**——排序不如新族整齐。`persistent-topic` 的 19 条在前、7 条标记在末尾；`topic-watch` 的 5 条在前、8 条物化在后。接受：这两族的原有条目本来就是核心模型，排前面符合阅读习惯

## D5 排序与分类

沿用已验证的两段式：business 在前（保持来源顺序）、impl 在后（标 `<!-- kind: impl -->`）。

**但本次两族都是就地扩容，两段式不成立（R-5）**：原有条目留在原位、只有新增条目进 delta，所以「impl 排到末尾并标记」只对新增条目生效；如果 impl 落在原有的保留条目里，它既不会被重排也不会带标记。分类结果为 **39 / 0**（本族无 impl），因此这个问题本次不触发——但要点明：**就地扩容族不具备两段式的落地条件**，后续这类族要么接受原样、要么改为新建 capability 才能重排。

**本族预判的 impl 候选**（review 重点看）：

- `历史数据回刷`（persistent-topic）——一次性数据迁移
- `回刷重建持久化话题（单成员不建）`——同上，但含「单成员不建」的持续规则，属混合
- `现有 API 向后兼容扩展`——business（对外承诺）
- `配置项`——business（阈值默认值影响归属结果）
- `PersistentTopic 质心表示`——含向量计算口径，可能 mixed

## D6 归档机制

1. `.openspec.yaml` 声明 `retire_capabilities: true`
2. 归档前摘掉 **2 个**被退役 spec 的 `<!-- domain: -->`（topic-key-event-markers、watch-materialized-topic）
3. 归档后补 marker：
   - `persistent-topic` 原 marker `topic-graph, semantic-board, daily-report`（3 域，保留不动）
   - `topic-watch` 原 marker `semantic-board, daily-report, topic-graph`（3 域，保留不动）
4. 归档后手改两族 Purpose（D4）

## D7 已知损失

指向 2 个被删 capability 的 archived delta 落 `domain-history.sh` 第 6 段；只碰过它们的 change 会从反查消失（与既往三族同一损失模式，别名表仍待做）。

**跨族引用**：`watch-materialized-topic` 被 `topic-watch` 的 Purpose 引用（「见 watch-materialized-topic」），合并后该句变自指。其他 main spec 对这两个 capability 的引用已扫描（见 tasks 7.3），登记到「跨族引用清扫」批次。
