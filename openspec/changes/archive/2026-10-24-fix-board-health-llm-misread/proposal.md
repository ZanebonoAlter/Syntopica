<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

# Proposal: fix-board-health-llm-misread

## Why

体检 LLM 评分判据与数据事实脱节，系统性压低健康版块分数（2026-10-22 重算后新一轮体检实证）：①「贴合度分化」判据无绝对锚——low_sim_ratio 是板内相对 P25 线的位次占比，P25 相对线的固有性质决定相当比例标签天然在线下，LLM 把「low_ratio 50-66%」直译「分化严重/跑题风险」，而俄乌板（55 分全库最低）的标签级 min_mean 0.632 实为全库最健康；②短实体标签（普京/乌军/SQLite 等 2-4 字名词）语义聚焦，与长事件句 identity embedding 的 cosine 天然比泛概念标签低 0.05-0.10，全库 11 板达标标签 mean 带一致落在 0.51-0.76；③「reason_dist 被弱命中路径主导且 composite 几乎为零」条款对 9/12 板天然命中（composite_hit 全库仅 58 行、3 板），俄乌板（weighted 69 主导 + composite 0 的正常形态）被判病征。

## What Changes

- `boardhealth/llm.go` 评分判据（boardHealthSystemPrompt）三处校准：
  1. **分化判据加绝对锚**：「板块内贴合度分化」须同时满足相对垫底（low_sim_ratio 大面积偏高）**且**绝对不健康（多数达标标签 cosine_mean < 0.6）；cosine_mean ≥ 0.6 的标签 low_sim_ratio 高只是板内位次，不是跑题证据。
  2. **短实体标签条款**：人名/地名/技术名词等短标签语义聚焦，cosine_mean 低于板内泛概念标签属正常形态，不构成扣分依据。
  3. **reason_dist 条款限定**：composite_hit 仅在板挂载组合标签时可能产生（全库少数板），无组合挂载的板 reason_dist 全为单标签路径是常态；该病征仅在弱路径占比畸高**且** ghost 占比同时偏高时成立。
- 单测：prompt 锚定条款防回归断言（字符串级）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `board-health-checkup`（`openspec/specs/board-health-checkup/spec.md`）：Requirement「LLM 综合评分与降级」补评分判据防误读条款（分化双条件 / 短实体标签 / composite 常态）。

## Impact

- **代码**：`backend-go/internal/tagmanagement/service/boardhealth/llm.go`（system prompt 常量）+ 单测文件；统计口径（metrics.go）与降级语义零改动——红线 22 口径唯一实现不动，只校准 LLM 消费口径的解读规则。
- **行为**：同样的统计快照评分预期上移（俄乌/中国国内/卫生健康/影视等「假低分」板回归真实档位）；评分漂移是本 change 目的本身。
- **数据**：无需重算任何匹配数据；下一轮体检自动生效。
