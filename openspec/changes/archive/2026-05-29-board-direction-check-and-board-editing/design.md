## Context

当前 tag-to-board 匹配管线中，max_sim 规则仅依赖辅助标签之间的 pairwise cosine 相似度。实际数据已验证：日经225指数因辅助标签"日本股市"与"美国政治与经济动态"板块的辅助标签"标普500指数"cosine=0.80 而匹配成功——辅助标签层面相似（都是股指），但标签整体语义方向与板块方向不符。

方向性校验需要使用 tag identity embedding（`topic_tag_embeddings` 表，embedding_type='identity'）与 board embedding（`semantic_labels` 表，label_type='board'）的 cosine 相似度。但实际数据发现：

1. **全部 10 个现有板块 embedding 为 NULL**——因为 LLM 升级建议创建板块时（`semantic_board_upgrade.go`）未调用 embedder
2. **质心方案不可行**——用辅助标签 embedding 质心代替 board embedding 时，大板块（20+ 辅助标签）方向信号被稀释，区分度差（min~0.40, median~0.74，无明显分界）
3. **前端缺少板块编辑功能**——`updateBoard` API 已存在但从未被前端调用

## Goals / Non-Goals

**Goals:**
- 修复 LLM 升级建议创建板块时缺失 embedding 的 bug
- 为现有 NULL embedding 板块一次性 backfill
- 前端新增板块编辑功能（label、description 修改）
- 为 max_sim 规则增加方向性校验，标记 direction_mismatch
- 日报排除 direction_mismatch 标签
- 前端默认隐藏方向不符标签，提供显示开关

**Non-Goals:**
- 不对 hit_rate/weighted/direct_hit 规则施加方向校验（这些规则本身有多样性去重机制）
- 不修改匹配评分算法（方向校验是布尔门控，不影响 score）
- 不自动删除或拒绝方向不符的匹配（仍记录，仅标记）

## Decisions

### D1: 方向性校验数据源

**选择**: tag identity embedding × board embedding（`semantic_labels.embedding`）

**排除方案**:
- 辅助标签质心：大板块区分度差，median 0.74 vs min 0.40 无清晰分界
- tag semantic embedding：信息冗余（semantic embedding 本身基于 description + aliases，与辅助标签语义重叠）

**原因**: board embedding 代表"板块文字定义"（label + description），tag identity embedding 代表"标签文字定义"（label + aliases + category），两者的 cosine 直接反映"话题方向一致性"

### D2: DirectionSimThreshold 默认值

**选择**: 0.5（初始默认）

**原因**: 基于质心的数据验证显示 max_sim 方向相似度 min=0.40, P10=0.66, median=0.74。使用 board embedding（而非质心）后，预期区分度更好——板块描述如"聚焦美国核心政治人物（特朗普、沃什）..."与"日经225指数"的文字差异远大于质心方式。0.5 作为保守起点，后续根据实际 board embedding 数据调优。

配置 key: `semantic_board_match_direction_sim_threshold`（走 `ai_settings` 表，与现有 12 个参数一致）

### D3: direction_mismatch 语义

**选择**: `direction_mismatch=true` 仍写入 `topic_tag_board_labels`，不影响 score，仅标记

**原因**: 保留匹配记录用于回溯分析和阈值调优，同时允许前端/日报灵活过滤

### D4: 前端过滤策略

**选择**: 后端过滤（query param `?show_direction_mismatch=true`，默认排除）

**原因**: 减少不必要数据传输，日报等场景也需要后端过滤

### D5: 板块 embedding 生成时机与输入文本

统一 embedding 输入文本为 `label + ". " + description`（description 为空时仅用 label）。

生成时机：
- LLM 升级建议 create_new 时生成（bug fix）
- 前端编辑 label 或 description 时刷新
- 现有 NULL 数据通过一次性 API 批量补生成

**注意**: LLM 升级 create_new 时 LLM 也会生成 description，因此统一使用 `label + ". " + description` 而非仅 `label`，确保所有场景下 board embedding 语义一致。

### D5.1: updateBoard 后不自动重新匹配

**选择**: updateBoard 后**不**触发相关 tag 的重新匹配

**原因**: 重新匹配所有关联 tag 成本高且实现复杂（需追踪哪些 tag 与该 board 相关）。方向校验结果会在以下时机自然刷新：
- collectBoardTags fallback 时重新调用 MatchTopicTag
- 手动触发 remtach（通过 Task 3.5 的 API）

如需立即刷新，用户可调用 `POST /api/semantic-boards/rematch-all`。

### D6: 板块编辑 UI 交互

**选择**: TagsPage 板块列表中加编辑按钮 → 弹出编辑对话框（label、description 可修改）

**排除方案**: 独立板块管理页面——当前只有 10 个板块，不需要独立页面

## Risks

- **阈值可能需要多次调优**: 0.5 是初始值，实际 board embedding 生成后的数据分布可能与质心预估不同。建议 backfill 后先跑一次数据验证再正式启用
- **embedding backfill 产生 API 调用**: 10 个板块 × 1 次 embedding 调用，成本可忽略
- **direction_mismatch 影响已有数据**: backfill 板块 embedding 后需要重新跑一次匹配才能刷新 direction_mismatch 标记（Task 3.5 提供 rematch API）
