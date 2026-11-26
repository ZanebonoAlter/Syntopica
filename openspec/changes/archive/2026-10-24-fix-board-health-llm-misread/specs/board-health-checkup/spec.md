# board-health-checkup Delta

## MODIFIED Requirements

### Requirement: LLM 综合评分与降级
体检 SHALL 将统计快照交 LLM 综合裁决，产出单一评分与一句评语；MUST NOT 使用固定权重公式合成评分。LLM 调用失败或返回非法时 SHALL 降级：纯统计部分照常持久化，评分与评语标记为空，体检轮次 MUST NOT 因此失败或重试阻塞。

评分判据 SHALL 防三类系统性误读（fix-board-health-llm-misread，2026-10-22 实证）：

1. **贴合度分化须双条件**：low_sim_ratio 是板内相对 P25 线的位次占比（P25 相对线的固有性质决定相当比例标签天然在线下），「分化明显」判据须同时满足相对垫底（low_sim_ratio 大面积偏高）与绝对不健康（多数达标标签 cosine_mean < 0.6）；identity 事件句 × 短名词标签的 cosine 天然带 0.51-0.76，mean ≥ 0.6 的标签 low_sim_ratio 高只是位次不是跑题证据。
2. **短实体标签不扣分**：人名/地名/技术名词等短标签语义聚焦，cosine_mean 低于板内泛概念标签属正常形态，不构成扣分依据。
3. **composite 常态保护**：composite_hit 仅在板挂载组合标签时可能产生（全库少数板）；无组合挂载的板 reason_dist 全为单标签路径是常态非病征，该病征仅在弱路径占比主导且 ghost 占比同时偏高时成立。

#### Scenario: LLM 产出评分与评语
- **WHEN** 版块「AI 前沿」体检统计完成并送 LLM 综合裁决
- **THEN** 快照 SHALL 含 LLM 评分与评语

#### Scenario: LLM 失败降级不丢统计
- **WHEN** 某版块的 LLM 综合评分调用超时
- **THEN** 该版块快照的统计部分 SHALL 照常持久化，评分/评语为空，体检轮次继续处理后续版块

#### Scenario: 相对垫底但绝对健康的标签不判分化
- **WHEN** 板块达标标签 cosine_mean 全部 ≥ 0.6（如俄乌板 0.632-0.700）且个别短实体标签 low_sim_ratio 0.5-0.7
- **THEN** 评分 SHALL NOT 因「贴合度分化」降档（绝对健康时相对位次不是病征）

#### Scenario: 无组合挂载的板不因 reason_dist 判病
- **WHEN** 板块 reason_dist 全为单标签路径（weighted/max_sim/hit_rate）且 composite_hit=0 且 ghost 占比低
- **THEN** 「弱命中路径主导」SHALL NOT 单独作为降档病征

#### Scenario: 真分化仍被拦截
- **WHEN** 板块多数达标标签 cosine_mean < 0.6 且 low_sim_ratio 大面积偏高
- **THEN** 评分 SHALL 按「贴合度分化明显」正常降档（双条件同时满足，防误读不等于放松真病征）
