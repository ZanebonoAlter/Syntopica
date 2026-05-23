## Context

当前叙事系统每天重建 `narrative_boards`,Board 没有独立概念,只是 abstract tree 的 1:1 映射。用户期望的是类似 BBS 的持久化板块概念,由 LLM + embedding 驱动的智能标签分发。

## Goals / Non-Goals

**Goals:**
- `board_concepts` 表作为持久化板块概念,跨天存在
- LLM 冷启动:扫描 abstract tags 建议初始板块概念
- embedding 匹配:将小 abstract tree 和未分类 event tags 自动分发到概念板
- 大 abstract tree(≥6 节点)自动成为"每日热点板",支持跨天延续
- embedding 匹配阈值可通过 `ai_settings` 配置
- 用户可手动词板概念

**Non-Goals:**
- 不改变 narrative 本身的生成逻辑(LLM prompt 不变)
- 不改变 NarrativeBoardCanvas 的 p5.js 渲染核心
- 不引入新的 AI capability(复用 CapabilityTopicTagging)

## Decisions

### D1: 双轨制 Board 创建

**决定**: 每日生成时,大 abstract tree(≥6 节点)走"每日热点板"轨道,小 tree 和未分类 event tags 走"embedding 匹配概念板"轨道。

**替代方案**: 所有内容统一走 embedding 匹配。
**为什么不选**: 大 abstract tree 本身是一个有结构的话题簇,强行拆散到不同概念板会丢失语义完整性。用户也明确表示大 tree 是"每日动态热点"的素材。

### D2: board_concepts.embedding 的生成

**决定**: Board Concept 的 embedding 由 LLM 根据 concept 的 `name + description` 生成摘要文本,再调用 embedding 服务生成向量。不使用 tag embedding 的平均值。

**理由**: Concept 是比 tag 更高层的语义单元,直接用 concept 描述生成 embedding 能更好地捕获"板块概念"的语义边界,而非简单地取子 tag 的几何中心。

### D3: embedding 匹配流程

```
1. 每个待匹配项(小tree/unclassified event)取其 label + description → embedding
2. 与所有 active board_concepts.embedding 计算 cosine similarity
3. 取最高分,≥threshold 则匹配,<threshold 则进入"未归类"桶
4. "未归类"桶在当天定时任务结束后,如超过N个(默认5),
   触发 LLM 建议新的 board_concept 候选
```

### D4: 阈值配置存储

**决定**: 使用现有 `ai_settings` 表,key=`narrative_board_embedding_threshold`,value=`0.7`,通过现有 AI 设置 API 读写。

**替代方案**: 新建独立配置表或放在环境变量中。
**为什么不选**: `ai_settings` 已有 key-value 模式、API、前端 UI,直接复用成本最低。

### D5: 热点板跨天延续

**决定**: 每日热点板通过 `abstract_tag_id` 自动关联昨天的同名热点板(`prev_board_ids`)。热点板下的 narrative 通过 `parent_ids` 串联。

**这是现有逻辑(`matchPreviousBoard`)的复用**,不是新机制。

### D6: 板块概念冷启动

**决定**: LLM 一次性扫描所有 active abstract tags 的 name/description,建议初始板块概念列表。结果通过 API 返回,前端展示让用户审阅(接受/拒绝/修改)。用户确认后写入 `board_concepts` 表并生成 embedding。

### D7: board_concept 不替代 enum ScopeType

`scope_type` 和 `scope_category_id` 继续控制在哪个 scope(global/某 category)下生效。`board_concept` 是 scope 内的"子板块",不与 scope 正交。

## Risks / Trade-offs

- **[R] LLM 冷启动建议质量不稳定** → 用户审阅环节作为安全阀;后续可通过用户修正数据微调
- **[R] embedding 匹配不准确** → 阈值可配置;未归类桶提供可见性,用户可手动分配
- **[R] board_concepts 表增长** → 用户可标记 is_active=false 停用;定期清理长时间无匹配的概念板
- **[R] 热点板数量波动** → 每日 ≤28 个(N=6),LLM 调用量可控;如 actual abstract tree 规模变化巨大,阈值 N 可调整
