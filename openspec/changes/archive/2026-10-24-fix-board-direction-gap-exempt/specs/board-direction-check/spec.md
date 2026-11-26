# board-direction-check Delta

## MODIFIED Requirements

### Requirement: Direction check for non-direct_hit matches

对每个匹配成功的 tag-board 结果（composite_hit 除外），系统 SHALL 按以下次序执行方向判定，未获豁免的挂载任一门触发即标记 `direction_mismatch=true`：

0. **全库 gap 豁免**：若挂载版块的 identity cosine 与该 tag 对**全部 active 版块**（`semantic_labels` WHERE label_type='board' AND status='active'，不限本次匹配候选）identity cosine 最大值的差值 < DirectionGapExemptMargin（默认 0.10；差值 < 1e-9 的并列最优恒在此集合内），则豁免绝对门与相对门，`direction_mismatch=false`——长事件 tag × 短板名 embedding 的 cosine 全挤窄带（实证 0.55-0.65）时全库第一名常被语义重叠的磁铁板（如「中国国内新闻」）抢走，真家排 2-5 名的窄带差距是排序噪声不是跑题。若 DirectionFloor > 0 且该 cosine < DirectionFloor，豁免不生效（地板线优先）。
1. **绝对门**：未获豁免的挂载，cosine(tag identity embedding, board embedding) < DirectionSimThreshold（默认 0.5，ai_settings 现值 0.6）→ 标记。
2. **相对门**：未获豁免的挂载，gap = bestCos − cosine(tag identity embedding, board embedding) ≥ DirectionRelativeMargin（默认 0.15）→ 标记。bestCos 为该标签**本次匹配成功的全部版块**（含同轮其他候选，含将被标 mismatch 的候选）中 identity cosine 的最大值。

规则：
- composite_hit 豁免：不执行方向校验，`direction_mismatch` 恒为 false（组合命中即指向一致）。
- direct_hit 同样执行上述判定（与间接规则一致）。
- 全库豁免参照系 MUST 覆盖本次匹配候选之外的 active 版块（垃圾单挂的真家常在候选外）。
- 豁免与各门仅消费 identity cosine，不影响 score；DirectionGapExemptMargin ≤ 0 时回退为「差值 < 1e-9 的并列最优」豁免（即本 change 前行为）。
- tag identity embedding 缺失时跳过全部判定，`direction_mismatch=false`；board embedding 缺失的版块不进入全库参照系，挂载版块自身 embedding 缺失时跳过对应门。

#### Scenario: near-best mount within gap margin exempted
- **WHEN** tag「高市早苗在众议院预算委员会发表个人家务诉苦言论」挂载版块「日本新闻」cosine=0.581，全库最优为「中国国内新闻」cosine=0.647（gap 0.066 < DirectionGapExemptMargin=0.10）AND DirectionSimThreshold=0.6
- **THEN** direction_mismatch=false（gap 豁免——窄带内磁铁板压制的真家挂载不算跑题）

#### Scenario: mount beyond gap margin still flagged
- **WHEN** tag「Anthropic推出Claude Opus 5.5模型」挂载版块「全球科技巨头动态」cosine=0.597，全库最优为「生成式AI与大模型厂商」cosine=0.811（gap 0.214 ≥ 0.10）AND cosine < 0.6
- **THEN** direction_mismatch=true（真家明显更近的次优挂载，绝对门拦截）

#### Scenario: rank-1 mount always within gap margin
- **WHEN** 挂载版块即全库 cosine 最大者（差值 0）
- **THEN** direction_mismatch=false（恒在豁免集合内，与 margin 取值无关）

#### Scenario: gap margin disabled falls back to tie-only exemption
- **WHEN** DirectionGapExemptMargin ≤ 0 AND 挂载版块非并列最优 AND cosine < 0.6
- **THEN** 回退本 change 前行为：绝对门照常标记（只有差值 < 1e-9 的并列最优获豁免）

#### Scenario: floor revokes gap exemption
- **WHEN** 挂载 gap < margin 获豁免 AND DirectionFloor > 0 AND cosine < DirectionFloor
- **THEN** 豁免不生效，回退双门判定

#### Scenario: hit_rate match passes direction check
- **WHEN** tag matches board via hit_rate rule AND（gap 豁免生效 OR（cosine >= DirectionSimThreshold AND 相对门 gap < margin））
- **THEN** direction_mismatch=false, match recorded normally

#### Scenario: weighted match fails direction check
- **WHEN** tag matches board via weighted rule AND 未获豁免 AND cosine < DirectionSimThreshold
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: hit_rate match fails direction check
- **WHEN** tag matches board via hit_rate rule AND 未获豁免 AND cosine(tag_identity_embedding, board_embedding) < DirectionSimThreshold
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: relative gate catches multi-mount cross-domain leak
- **WHEN** tag 同轮匹配版块 A（cosine 0.81）与 B（cosine 0.60）AND 两处均未获全库 gap 豁免 AND DirectionRelativeMargin=0.15
- **THEN** B 的 direction_mismatch=true（相对门 gap 0.21 拦截），A 正常

#### Scenario: legal dual-mount within margin survives
- **WHEN** tag 同轮匹配版块 A（cosine 0.80）与 B（cosine 0.68），gap 0.12 < DirectionRelativeMargin
- **THEN** A、B 均 direction_mismatch=false（相对门内合法双挂）

#### Scenario: single-board mount unaffected by relative gate
- **WHEN** tag 本次仅匹配一个版块（未获全库豁免）AND cosine < DirectionSimThreshold
- **THEN** 绝对门照常标记；相对门 gap≡0 恒不触发

#### Scenario: composite_hit exempt
- **WHEN** tag matches board via composite_hit rule
- **THEN** 不执行方向校验，direction_mismatch=false（组合命中即指向一致）

#### Scenario: tag or board embedding unavailable
- **WHEN** tag identity embedding is NULL OR board embedding is NULL
- **THEN** 对应门与豁免跳过，direction_mismatch=false

#### Scenario: direct_hit match
- **WHEN** tag matches board via direct_hit rule AND（未获 gap 豁免 AND 任一门触发）
- **THEN** direction_mismatch=true（direct_hit 与间接规则同样校验）

## ADDED Requirements

### Requirement: DirectionGapExemptMargin configurable

系统 SHALL 支持通过 ai_settings 键 `semantic_board_match_direction_gap_exempt_margin` 配置全库 gap 豁免的差值门槛：

- 默认 0.10；取值 ≤ 0 表示关闭 gap 扩围（回退并列最优豁免）。豁免带有效带宽 = max(DirectionGapExemptMargin, 1e-9)——亚噪声 margin（0 < margin < 1e-9）实际生效 1e-9（与代码 math.Max 归一一致）。
- 配置加载进 `LoadConfig` 白名单，与 `direction_relative_margin` / `direction_floor` 同管道（DB-only 键，不经 matching-config API 白名单；解析语义复用 margin 类 parse：负值归 0 关闭，越界（> 1.0）或非法值拒绝并保持默认 0.10）。

#### Scenario: custom margin applied
- **WHEN** ai_settings 设置 `semantic_board_match_direction_gap_exempt_margin=0.15` AND 挂载 gap=0.12
- **THEN** 豁免生效（0.12 < 0.15）

#### Scenario: default margin without setting
- **WHEN** ai_settings 无该键
- **THEN** DirectionGapExemptMargin=0.10

#### Scenario: invalid margin rejected
- **WHEN** ai_settings 设置该键为 1.5 或非数值
- **THEN** 配置拒绝，保持默认 0.10（margin 类 parse 语义：fallback 恒指本次加载的默认值，与 relative_margin/floor 同继承语义）
