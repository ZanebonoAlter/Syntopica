## Context

当前 `hasDirectSemanticBoardHit` 只要求 tag 与 board 的辅助标签有 **1 个 ID 交集**即返回 true，导致 score=1.0 的 direct_hit。`max_sim` 和 `hit_rate` 规则中的 `DirectMaxSimMinHits` 等双因子保护完全被短路。

实际案例：事件「日菲加强安保合作旨在牵制中国」(tag 100154) 的 4 个辅助标签为 {特朗普(2478), 日菲安全合作(8609), 日下涉(10435), 马科斯(10440)}，其中「特朗普」与「美国政治与经济动态」(board 3640) 的 composition 产生交集，直接命中。而该事件与该板块在语义上不相关。

同时，匹配详情 API 在 direct_hit 场景下只返回精确匹配的辅助标签对（`direct_hit_auxiliaries`），不展示未命中标签的相似度，用户无法看到"只有 1 个交集而其他 3 个都不匹配"的全貌。

相关代码：
- 匹配核心：`backend-go/internal/domain/tagging/semantic_board_matching.go`
- 匹配详情 handler：`backend-go/internal/domain/tagging/semantic_board_handler.go` (`getTagMatchDetail`)
- LLM prompt：`backend-go/internal/domain/tagging/extractor_enhanced.go` (`buildEventPersonPrompt`)
- 前端面板：`front/app/features/tags/components/MatchDetailPanel.vue`
- 现有配置加载：`semantic_board_matching.go` 的 `loadConfig`

## Goals / Non-Goals

**Goals:**
- direct_hit 规则增加最小交集数要求，交集不足时退回到相似度匹配流程
- LLM 辅助标签提取 prompt 增加相关性约束，减少源头噪音
- 匹配详情 API 在 direct_hit 场景下也展示所有辅助标签的相似度匹配对（含未命中）

**Non-Goals:**
- 不修改其他三条匹配规则（hit_rate / max_sim / weighted）的逻辑
- 不修改辅助标签解析/入库流程
- 不修改前端面板布局（只新增数据展示）
- 不做历史数据自动迁移（用户可手动触发全量回填重算）

## Decisions

### D1: direct_hit 最小交集数

**决策**: 新增可配置参数 `DirectHitMinOverlap`（默认 2，对应 `ai_settings` 键 `semantic_board_match_direct_hit_min_overlap`）。`hasDirectSemanticBoardHit` 改为 `countDirectSemanticBoardHits`，返回交集数而非布尔值。`evaluateSemanticBoardMatches` 中只有交集数 ≥ `DirectHitMinOverlap` 时才走 direct_hit（score=1.0）路径；否则退回到相似度匹配流程。

**理由**: 与 `max_sim` 规则的 `DirectMaxSimMinHits` 思路一致——要求多个辅助标签共同支持匹配，而非单标签决定归属。默认 2 表示至少需要 2 个辅助标签同时出现在 board composition 中。

**边界行为**:
- 交集 = 1，阈值 = 2 → 不算 direct_hit，退回到相似度匹配
- 交集 = 2，阈值 = 2 → direct_hit，score = 1.0
- 交集 = 0 → 不变，走相似度匹配
- 阈值 = 1 → 退化为原行为（向后兼容）

**备选方案**: direct_hit 也用混合打分（交集比例）→ 被否决，因为 direct_hit 的语义就是"这些辅助标签完全一致"，不需要模糊化，问题只是阈值太低。

### D2: LLM prompt 辅助标签相关性约束

**决策**: 在 `buildEventPersonPrompt` 的「辅助标签要求」段落中新增约束：
- 辅助标签必须与事件核心主体直接相关，是理解事件不可或缺的要素
- 文章中仅为背景提及、一笔带过的人物或国家不应作为辅助标签
- 如果移除某个实体后事件描述仍然成立，则该实体不应成为辅助标签

**理由**: 从源头减少噪音辅助标签。以「日菲加强安保合作」为例，即使原文提及了特朗普的反应，但事件核心是日菲双边关系，特朗普并非事件核心主体。

**不增加硬性数量调整**: 保持 3-5 个范围不变，通过语义约束而非数量约束来提升质量。

### D3: direct_hit 场景的匹配详情展示完整辅助标签对

**决策**: `getTagMatchDetail` handler 在 direct_hit 场景下也调用 `computeMatchDetail` 计算所有 tag-board 辅助标签的相似度对。返回结构中 `direct_hit_auxiliaries` 保持不变（精确匹配列表），同时新增 `pairs` / `hits` / `hit_rate` / `max_similarity` 字段。

**返回结构变化**（direct_hit 场景）:
```jsonc
{
  "match_reason": "direct_hit",
  "score": 1.0,
  "direct_hit_auxiliaries": [
    // 精确匹配的辅助标签（不变）
    {"tag_auxiliary_id": 2478, "tag_label": "特朗普", "board_auxiliary_id": 2478, "board_label": "特朗普"}
  ],
  // 新增：所有辅助标签的相似度匹配对
  "pairs": [
    {"tag_auxiliary_label": "特朗普", "board_auxiliary_label": "特朗普", "similarity": 1.0, "is_hit": true},
    {"tag_auxiliary_label": "日菲安全合作", "board_auxiliary_label": "美国国防部", "similarity": 0.45, "is_hit": false},
    {"tag_auxiliary_label": "日下涉", "board_auxiliary_label": "纽约时报", "similarity": 0.31, "is_hit": false},
    {"tag_auxiliary_label": "马科斯", "board_auxiliary_label": "特朗普政府", "similarity": 0.38, "is_hit": false}
  ],
  "hits": 1,
  "hit_rate": 0.25,  // 1 / max(4, 3) = 0.25
  "max_similarity": 1.0
}
```

**理由**: 用户看到 direct_hit 后需要知道"其他标签怎么样"。当前只展示精确匹配的 1 个标签，用户无法判断匹配质量。加上 pairs 后，用户可以看到"1 个精确命中但另外 3 个完全不匹配"，这实际上会帮助用户理解为什么后续加了最小交集数后这个匹配可能不再成立。

**前端适配**: `MatchDetailPanel.vue` 在 direct_hit 场景下，`direct_hit_auxiliaries` 上方显示精确匹配，下方新增一个区域展示所有辅助标签对的相似度（与 hit_rate/max_sim/weighted 场景相同的 pairs 展示逻辑）。

### D4: 新增配置参数

**决策**: 新增 `DirectHitMinOverlap` 配置项，默认值 2，`ai_settings` 键名 `semantic_board_match_direct_hit_min_overlap`。加载逻辑同其他参数。

## Risks / Trade-offs

- **匹配结果变更**: 部分之前 direct_hit 的 tag（交集数 < 2）会退回到相似度匹配，可能不再匹配原 board 或以更低分数匹配。这是预期行为——这些匹配本身就是弱关联。
- **回填影响**: 用户修改阈值后需要触发全量回填才能让所有 tag 重新匹配。现有回填机制支持此操作。
- **LLM prompt 变更非确定性**: prompt 约束可能无法完全消除不相关辅助标签，但能显著降低频率。
- **direct_hit 场景 pairs 计算开销**: direct_hit 时额外调用 `computeMatchDetail`（< 1ms），按需触发，影响可忽略。
