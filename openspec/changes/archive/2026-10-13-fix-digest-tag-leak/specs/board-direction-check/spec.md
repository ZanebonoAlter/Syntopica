## RENAMED Requirements

- FROM: `Direction check for non-direct_hit matches`
- TO: `Direction check for matched boards`

## MODIFIED Requirements

### Requirement: Direction check for non-direct_hit matches

对每个匹配成功的 tag-board 结果（composite_hit 除外），系统 SHALL 执行双条件方向校验，任一条件触发即标记 `direction_mismatch=true`：

1. **绝对门**：cosine(tag identity embedding, board embedding) < DirectionSimThreshold（默认 0.5）。
2. **相对门**：gap = bestCos − cosine(tag identity embedding, board embedding) ≥ DirectionRelativeMargin（默认 0.15）。bestCos 为该标签**本次匹配成功的全部版块**（含同轮其他候选）中 identity cosine 的最大值。

规则：
- composite_hit 豁免：不执行方向校验，`direction_mismatch` 恒为 false（组合命中即指向一致）。
- direct_hit 同样执行双条件校验（与间接规则一致）。
- 标签本次仅匹配上一个版块时 gap 恒为 0，相对门自然通过，仅绝对门生效。
- 校验仅标记，不影响 score。
- tag identity embedding 或 board embedding 缺失时，跳过对应门的校验，`direction_mismatch=false`。

#### Scenario: hit_rate match passes direction check
- **WHEN** tag matches board via hit_rate rule AND cosine(tag_identity_embedding, board_embedding) >= 0.5 AND gap < margin
- **THEN** direction_mismatch=false, match recorded normally

#### Scenario: hit_rate match fails direction check
- **WHEN** tag matches board via hit_rate rule AND cosine(tag_identity_embedding, board_embedding) < 0.5
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: weighted match fails direction check
- **WHEN** tag matches board via weighted rule AND cosine(tag_identity_embedding, board_embedding) < 0.5
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: tag or board embedding unavailable
- **WHEN** tag identity embedding is NULL OR board embedding is NULL
- **THEN** both gates are skipped, direction_mismatch=false

#### Scenario: direct_hit match
- **WHEN** tag matches board via direct_hit rule AND the mount triggers the absolute gate or the relative gate
- **THEN** direction_mismatch=true（修正旧 spec「跳过校验」的漂移，与现行实现及 semantic-board 红线一致：direct_hit 与间接规则同样校验）

#### Scenario: relative gate catches multi-mount cross-domain leak
- **WHEN** tag matched boards A and B in the same run AND cosine(tag_identity, A)=0.81 (best) AND cosine(tag_identity, B)=0.60 AND margin=0.15
- **THEN** mount on B has direction_mismatch=true (gap 0.21 >= 0.15), mount on A has direction_mismatch=false

#### Scenario: legal dual-mount within margin survives
- **WHEN** tag matched boards A and B AND cosine(tag_identity, A)=0.82 (best) AND cosine(tag_identity, B)=0.70 AND margin=0.15
- **THEN** both mounts have direction_mismatch=false (gap 0.12 < 0.15)

#### Scenario: single-board mount unaffected by relative gate
- **WHEN** tag matched only board B AND cosine(tag_identity, B) >= 0.5
- **THEN** direction_mismatch=false (gap=0, relative gate passes)

#### Scenario: composite_hit exempt
- **WHEN** tag matches board via composite_hit rule
- **THEN** direction check is NOT performed, direction_mismatch=false

## ADDED Requirements

### Requirement: DirectionRelativeMargin configurable

DirectionRelativeMargin SHALL 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_relative_margin`，默认 0.15。设为 0 时相对门 MUST 关闭，回退为仅绝对门校验。与现有匹配参数一致的管理方式。

#### Scenario: custom margin
- **WHEN** ai_settings contains `semantic_board_match_direction_relative_margin` with value 0.18
- **THEN** relative gate uses margin=0.18 (gap >= 0.18 flags mismatch)

#### Scenario: margin zero disables relative gate
- **WHEN** ai_settings contains `semantic_board_match_direction_relative_margin` with value 0
- **THEN** only the absolute gate applies

#### Scenario: no margin row
- **WHEN** ai_settings does not contain the key
- **THEN** default 0.15 is used
