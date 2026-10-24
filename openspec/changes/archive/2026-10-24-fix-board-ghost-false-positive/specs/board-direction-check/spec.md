## MODIFIED Requirements

### Requirement: Direction check for non-direct_hit matches

对每个匹配成功的 tag-board 结果（composite_hit 除外），系统 SHALL 按以下次序执行三层方向判定，任一未豁免的门触发即标记 `direction_mismatch=true`：

0. **全库最优豁免**：若挂载版块为该 tag 对**全部 active 版块**（`semantic_labels` WHERE label_type='board' AND status='active'，不限本次匹配候选）identity cosine 的最大者（含并列），则豁免绝对门与相对门，`direction_mismatch=false`——「具体事件 × 宽泛类目」在 embedding 空间的天然低 cosine 不是跑题。若 DirectionFloor > 0 且该 cosine < DirectionFloor，豁免不生效（地板线优先）。
1. **绝对门**：未获豁免的挂载（非全库最优，或全库最优但被地板线收回豁免），cosine(tag identity embedding, board embedding) < DirectionSimThreshold（默认 0.5，ai_settings 现值 0.6）→ 标记。
2. **相对门**：未获豁免的挂载，gap = bestCos − cosine(tag identity embedding, board embedding) ≥ DirectionRelativeMargin（默认 0.15）→ 标记。bestCos 为该标签**本次匹配成功的全部版块**（含同轮其他候选）中 identity cosine 的最大值。

规则：
- composite_hit 豁免：不执行方向校验，`direction_mismatch` 恒为 false（组合命中即指向一致）。
- direct_hit 同样执行三层判定（与间接规则一致）。
- 全库最优参照系 MUST 覆盖本次匹配候选之外的 active 版块：串味挂载的真家往往未被本次匹配命中（单挂弱关联 tag 的更优版块在候选外），仅用同轮候选做参照会误豁免。
- 标签本次仅匹配上一个版块且该版块即全库最优时，豁免生效；若非全库最优，绝对门照常适用（相对门 gap 恒 0 自然通过）。
- 校验仅标记，不影响 score。
- tag identity embedding 缺失时跳过全部判定，`direction_mismatch=false`；board embedding 缺失的版块不进入全库最优参照系，挂载版块自身 embedding 缺失时跳过对应门，`direction_mismatch=false`。

#### Scenario: global-best mount exempted despite low absolute cosine
- **WHEN** tag「日本首相高市早苗任命武部新接任农林水产大臣」挂载版块「日本新闻」AND cosine(tag_identity, 日本新闻)=0.5826 为该 tag 对全部 active 版块 identity cosine 的最大值 AND DirectionSimThreshold=0.6
- **THEN** direction_mismatch=false（全库最优豁免，挂载计入日报）

#### Scenario: non-best mount below threshold still flagged
- **WHEN** tag 挂载版块 B，cosine(tag_identity, B)=0.55 < 0.6，且存在 active 版块 A（可能不在本次匹配候选内）cosine(tag_identity, A)=0.82 更高
- **THEN** 挂载 B 的 direction_mismatch=true（非全库最优，绝对门照常拦截）

#### Scenario: tied global best also exempted
- **WHEN** tag 挂载版块 A 与 B 的 identity cosine 并列为全部 active 版块中最大值（均 0.58 < threshold 0.6）
- **THEN** 两处挂载均 direction_mismatch=false（并列=无法区分归属，不算跑题）

#### Scenario: global best reference spans beyond this run's candidates
- **WHEN** tag 本次仅匹配上榜版块 B（唯一候选，相对门 gap=0），但全库 active 版块中 A 的 identity cosine 更高（A 因 aux 交集为空未进入本次候选）
- **THEN** B 非全库最优，不获豁免；若 cosine(tag_identity, B) < DirectionSimThreshold 则 direction_mismatch=true

#### Scenario: hit_rate match passes direction check
- **WHEN** tag matches board via hit_rate rule AND the mount is not the tag's global best board AND cosine(tag_identity_embedding, board_embedding) >= 0.5 AND gap < margin
- **THEN** direction_mismatch=false, match recorded normally

#### Scenario: hit_rate match fails direction check
- **WHEN** tag matches board via hit_rate rule AND the mount is not the tag's global best board AND cosine(tag_identity_embedding, board_embedding) < 0.5
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: weighted match fails direction check
- **WHEN** tag matches board via weighted rule AND the mount is not the tag's global best board AND cosine(tag_identity_embedding, board_embedding) < 0.5
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: direct_hit match
- **WHEN** tag matches board via direct_hit rule AND the mount is not the tag's global best board AND the mount triggers the absolute gate or the relative gate
- **THEN** direction_mismatch=true（direct_hit 与间接规则同样执行三层判定）

#### Scenario: relative gate catches multi-mount cross-domain leak
- **WHEN** tag matched boards A and B in the same run AND cosine(tag_identity, A)=0.81 (best in run and global best) AND cosine(tag_identity, B)=0.60 AND margin=0.15
- **THEN** mount on B has direction_mismatch=true (gap 0.21 >= 0.15), mount on A has direction_mismatch=false

#### Scenario: legal dual-mount within margin survives
- **WHEN** tag matched boards A and B AND cosine(tag_identity, A)=0.82 (global best) AND cosine(tag_identity, B)=0.70 AND margin=0.15
- **THEN** both mounts have direction_mismatch=false (B 虽非全库最优但绝对门 0.70 >= 0.5 通过且 gap 0.12 < 0.15)

#### Scenario: single-board mount exempt when globally best
- **WHEN** tag matched only board B AND cosine(tag_identity, B)=0.55 < DirectionSimThreshold=0.6 AND B is the tag's global best board
- **THEN** direction_mismatch=false（单挂且全库最优，豁免生效）

#### Scenario: single-board mount unaffected by relative gate
- **WHEN** tag matched only board B AND cosine(tag_identity, B) >= 0.5
- **THEN** direction_mismatch=false (gap=0, relative gate passes)

#### Scenario: tag or board embedding unavailable
- **WHEN** tag identity embedding is NULL OR board embedding is NULL
- **THEN** all gates are skipped, direction_mismatch=false

#### Scenario: composite_hit exempt
- **WHEN** tag matches board via composite_hit rule
- **THEN** direction check is NOT performed, direction_mismatch=false

## ADDED Requirements

### Requirement: Direction floor configurable

DirectionFloor SHALL 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_floor`，默认 0（关闭）。值 > 0 时作为全库最优豁免的地板线：豁免仅对 cosine ≥ DirectionFloor 的挂载生效；cosine < DirectionFloor 的全库最优挂载豁免不生效，SHALL 回退绝对门与相对门判定（floor 是豁免的下限保险丝，不是第二把绝对刀）——防御「哪都不像却硬挂」的未来形态（当前真库全库最优挂载最低 cosine 0.515 且成色正确，默认关闭无操作量）。与现有匹配参数一致的管理方式。

#### Scenario: floor disabled by default
- **WHEN** ai_settings does not contain `semantic_board_match_direction_floor`
- **THEN** floor=0，全库最优豁免无额外下限

#### Scenario: floor overrides exemption
- **WHEN** ai_settings contains `semantic_board_match_direction_floor` with value 0.55 AND a mount is the tag's global best AND cosine=0.515
- **THEN** direction_mismatch=true（0.515 < floor 0.55，豁免不生效，绝对门照常判定）
