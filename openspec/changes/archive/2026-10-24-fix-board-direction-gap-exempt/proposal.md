<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

# Proposal: fix-board-direction-gap-exempt

## Why

fix-board-ghost-false-positive 的「全库最优豁免」只保护 rank-1 挂载，但真库 pgvector 还原判定现场证明该假设在窄带分布下不成立：长事件 tag（一句话描述）× 短板名（多数板 description 为空）的 identity cosine 全部挤在 0.55-0.65 窄带，**全库第一名经常被语义重叠的磁铁板抢走**——「中国国内新闻」抢走日本新闻 25 个 mismatch 中 19 个的全库第一（真家 cosine 均值 0.577 排第 3-5 名），「生成式AI与大模型厂商」抢走科技巨头 209 个中 158 个（OpenAI/Anthropic 双归属）。真家排 2-5 名拿不到豁免，绝对门 0.6 继续错杀：现存 682 个 direction_mismatch 大部分仍是冤案（美国新闻 216 个里 215 个真家非全库第一、日本新闻 25 个里 22 个，样本=高市早苗内阁丑闻/日本央行加息/靖国神社分祀），ghost 占比 40% 拖垮体检评分，4.2 验收「ghost 回落个位数」落空，日报继续漏核心内容。

改造效果已经 SQL 实证锚定（2026-10-21 真库模拟，模拟器对现状复现率 89%、差异为 embedding 重嵌的时间漂移）：**gap 豁免 0.10 + 保留绝对门 0.6** 下全库 ghost 553→166，日本新闻 21→1（验收达标），美国新闻 190→46；gap [0.05,0.10) 新增豁免抽样（高市早苗众议院发言 gap 0.066、李强访中亚 gap 0.078、沙特IFF gap 0.051）确认为被磁铁噪声压制的真家挂载；反向验证：纯 gap 门（去绝对门）会让 mounted_cos≥0.65 的强关联被误标（科技巨头 225 个），不可行。

## What Changes

- **豁免扩围（判定核心）**：`computeGlobalBestBoards` 的豁免集合从「cosine 与全库最大值差 < 1e-9（并列最优）」扩为「差 < DirectionGapExemptMargin（默认 0.10）」；获 gap 豁免的挂载同样豁免绝对门与相对门。新配置键 `semantic_board_match_direction_gap_exempt_margin`（默认 0.10，≤0 关闭回退现行为），进 `LoadConfig` 白名单。
- **其余门不动**：绝对门（DirectionSimThreshold，现值 0.6）、相对门（DirectionRelativeMargin 0.15，候选集内参照）、DirectionFloor 语义保持；composite_hit 豁免、embedding 缺失跳过等既有规则不变。
- **存量重算**：部署后手动 rematch-all 刷 direction_mismatch（顺带清除 82 个 tag 已无 identity embedding 的陈旧标记与 66 个时间漂移标记）。
- **文档**：`docs/reference/flow/semantic-board.md` 匹配管道与红线节更新为「gap 豁免」表述，变更溯源补本 change 行。
- **明确不做**：科技巨头板残留 ~96 个「真家在生成式AI」的次优挂载标记保留（gap>0.10 标记有语义道理，板构成治理另立 change）；不动匹配管道的挂载判定（hit_rate/weighted/max_sim 路径门槛）与板 embedding 生成文本源。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `board-direction-check`（`openspec/specs/board-direction-check/spec.md`）：Requirement「Direction check for non-direct_hit matches」的全库最优豁免条件从「rank-1（含并列）」扩围为「与全库最优 cosine 差 < gap 豁免 margin」。

## Impact

- **代码**：`backend-go/internal/tagmanagement/service/board/semantic_board_matching.go`（computeGlobalBestBoards 判定扩围 + directionMismatch 闭包 + applyDirectionRelativeGate 跳过集合）、`LoadConfig`（新键）；单测扩展 `semantic_board_matching_unit_test.go`。
- **下游（无代码改动，行为自动改善）**：boardhealth `ghost_matches`（metrics.go 计数口径不变，标记数下降）、日报收集过滤（daily_report_orchestrator.go:537）、体检 LLM 评语与评分。
- **数据**：一次性 rematch-all 重算（全量 3448 tag，参照上个 change 4.1 实测口径）。
- **风险**：margin 过大（≥0.15）会漏放小 gap 真串味（实证：「韩国总统李在明」挂美国新闻 gap 0.066 在 gap10 下被豁免——接受为代价，靠默认 0.10 + 可配置收敛）。
