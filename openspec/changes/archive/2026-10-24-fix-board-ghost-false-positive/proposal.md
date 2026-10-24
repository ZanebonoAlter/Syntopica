<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

版块体检与日报被 `direction_mismatch` 假幽灵（false positive）系统性污染：绝对门阈值 0.6（ai_settings 覆盖值，代码默认 0.5）切在 qwen3-embedding「具体事件 × 宽泛类目」的天然 cosine 分布带（0.50-0.65）上，全库 5158 个挂载行中 1214 个（23.5%）被标 mismatch。真库验证（2026-10-16 会话）：

- **1214 个 mismatch 行按「挂载版块是否为该 tag 全库 active 版块 identity cosine 最高者」切分：671 行（55%）为全库最优挂载——全是冤案**（抽样 10/10 + 低分段 11/11 全部正确归属：Cursor→开发者工具 0.731、高市早苗任命→日本新闻 0.5826 全库第一、Tokenization→生成式AI 0.515）；543 行（45%）非最优——全是真串味（俄乌谈判挂美国新闻、刚果埃博拉挂卫生健康，均有 0.76-0.87 的明显真家）。
- 用户实测：绝对门降到 0.5 会放进一大批弱关联垃圾——垃圾与冤案同住 0.50-0.60 分布带，**任何绝对阈值都切不开**（fix-digest-tag-leak 归档时对 digest 串味已得出同结论）。
- 双害：① mismatch 挂载不计入日报（`daily_report_orchestrator.go:537` `NOT direction_mismatch` 过滤），日本新闻日报正漏掉高市早苗任命、央行加息等核心内容；② boardhealth ghost_matches 把假幽灵计为病征，日本新闻连续 8 天 55-72 分（ghost 40%），LLM 评语反复点名「贴合度垫底」的构成标签。

## What Changes

- **方向校验加「全库最优豁免」**：非 composite_hit 挂载，若挂载版块 = 该 tag 对全部 active 版块 identity cosine 最高者（含并列），豁免 mismatch 判定（绝对门与相对门都不再适用）——「具体事件 × 宽泛类目」的天然低 cosine 不是跑题。
- **绝对门作用域收窄**：`direction_sim_threshold`（现 0.6）保持不动，但只对「非全库最优」挂载生效；相对门（margin 0.15）逻辑不变。判定次序：composite_hit 免检 → 全库最优豁免 → 绝对门 → 相对门。
- **参照系用全库 active 版块**（不是同轮匹配候选）：垃圾单挂唯一候选时，其真家往往在候选之外（巴拿马地震 × 美国新闻 > × 日本新闻），候选内参照会误豁免。
- **存量重算**：rematch-all 重算 `topic_tag_board_labels.direction_mismatch`（顺带修复存量陈旧 flag——豁免组存在 Cursor 0.731 全库最优却标 mismatch 的 embedding 重生成后未重算行）。
- **地板线保险丝（可选配置，默认关闭）**：`semantic_board_match_direction_floor`——全库最优挂载 cos < floor 时豁免不生效、回退绝对门判定。真库验证全库最优挂载最低 cos 0.515 且成色正确，当前无操作量，纯防御未来「哪都不像却硬挂」形态。
- 消费方零改动：日报过滤、boardhealth ghost 统计、前端展示均读同一 flag，改源头全链路受益。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `board-direction-check`: 方向校验判定规则变化——新增全库最优豁免层，绝对门/相对门仅对非最优挂载生效；配套新增可选地板线配置。mismatch 的下游语义（仍记录、不计日报、前端默认隐藏）不变。
- `tag-to-board-matching`: 红线级匹配行为约束（五级优先 + 方向校验双门表述）需同步更新为三层判定（composite_hit 免检 → 全库最优豁免 → 双门）。

## Impact

- **后端**：`backend-go/internal/tagmanagement/service/board/semantic_board_matching.go`（`directionMismatch` 闭包重构为先算全库最优集合再逐版块判定；匹配循环已全量加载 `boardEmbeddings`，每 tag 对 ~14 个 active 版块的 cosine 为纯内存计算，成本可忽略）、配置加载（新增 floor 键）、`semantic_board_backfill.go`（rematch-all 复用现有机制）。
- **下游自动受益（无代码改动）**：`daily_report_orchestrator.go` 日报过滤、`boardhealth/metrics.go` ghost_matches 统计、`BoardHealthPanel.vue` 等前端展示。
- **数据**：存量 `direction_mismatch` 需 rematch-all 重算；预期 mismatch 1214 → ~543，日本新闻 ghost 33 → 个位数，体检评分自然回升。
- **不解决**（明确 out of scope）：口径②「多数暴政」问题（多元版块少数子主题构成标签必然垫底被点名，如靖国神社）——留给后续 change；体检可解释性下钻（幽灵明细/判定理由展示）另议。
