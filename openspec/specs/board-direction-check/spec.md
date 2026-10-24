## Purpose

方向性校验：对匹配结果（direct_hit 除外）计算 tag identity embedding 与 board embedding 的 cosine 相似度，低于阈值时标记 direction_mismatch=true。direct_hit 因精确重叠已足够可靠，跳过方向校验。

## Requirements

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

#### Scenario: global-best mount exempted despite low absolute cosine
- **WHEN** tag「日本首相高市早苗任命武部新接任农林水产大臣」挂载版块「日本新闻」AND cosine(tag_identity, 日本新闻)=0.5826 为该 tag 对全部 active 版块 identity cosine 的最大值 AND DirectionSimThreshold=0.6
- **THEN** direction_mismatch=false（全库最优挂载恒在 gap 豁免集合内，挂载计入日报）

#### Scenario: non-best mount below threshold still flagged
- **WHEN** tag 挂载版块 B，cosine(tag_identity, B)=0.55 < 0.6，且存在 active 版块 A（可能不在本次匹配候选内）cosine(tag_identity, A)=0.82 更高（gap 0.27 ≥ DirectionGapExemptMargin=0.10）
- **THEN** 挂载 B 的 direction_mismatch=true（未获 gap 豁免，绝对门照常拦截）

#### Scenario: tied global best also exempted
- **WHEN** tag 挂载版块 A 与 B 的 identity cosine 并列为全部 active 版块中最大值（均 0.58 < threshold 0.6）
- **THEN** 两处挂载均 direction_mismatch=false（并列最优差值 0 恒在豁免集合内，不算跑题）

#### Scenario: global best reference spans beyond this run's candidates
- **WHEN** tag 本次仅匹配上榜版块 B（唯一候选，相对门 gap=0），但全库 active 版块中 A 的 identity cosine 更高（A 因 aux 交集为空未进入本次候选）且差值 ≥ DirectionGapExemptMargin
- **THEN** B 未获全库 gap 豁免；若 cosine(tag_identity, B) < DirectionSimThreshold 则 direction_mismatch=true

#### Scenario: single-board mount exempt when globally best
- **WHEN** tag matched only board B AND cosine(tag_identity, B)=0.55 < DirectionSimThreshold=0.6 AND B is the tag's global best board
- **THEN** direction_mismatch=false（单挂且全库最优，豁免生效）

### Requirement: Direction check data loading

MatchTopicTag 加载 tag identity embedding（`topic_tag_embeddings` WHERE embedding_type='identity'）和所有活跃 board embedding（`semantic_labels` WHERE label_type='board'），传入 evaluateSemanticBoardMatches。embedding 缺失时不阻塞匹配流程。

#### Scenario: tag has no identity embedding
- **WHEN** tag has no identity embedding in topic_tag_embeddings
- **THEN** tagEmbedding passed as nil, direction check skipped for all max_sim matches

#### Scenario: board has no embedding
- **WHEN** a board's semantic_labels.embedding is NULL
- **THEN** that board is excluded from boardEmbeddings map, direction check skipped for that board

### Requirement: DirectionSimThreshold configurable

DirectionSimThreshold 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_sim_threshold`，默认 0.5。与现有 12 个匹配参数一致的管理方式。

#### Scenario: custom threshold
- **WHEN** ai_settings contains `semantic_board_match_direction_sim_threshold` with value 0.6
- **THEN** DirectionSimThreshold=0.6 used for direction check

#### Scenario: no custom threshold
- **WHEN** ai_settings does not contain the key
- **THEN** default 0.5 used

### Requirement: direction_mismatch persisted

topic_tag_board_labels 新增 `direction_mismatch BOOLEAN NOT NULL DEFAULT false` 列。replaceTopicTagBoardLabels 写入时包含此字段。

#### Scenario: persist direction mismatch
- **WHEN** evaluateSemanticBoardMatches returns a match with DirectionMismatch=true
- **THEN** topic_tag_board_labels.direction_mismatch=true for that record

### Requirement: Daily report excludes direction_mismatch tags

collectBoardTags 查询排除 direction_mismatch=true 的标签，包括主查询和 fallback 补算。

#### Scenario: direction_mismatch tag excluded from daily report
- **WHEN** tag has direction_mismatch=true in topic_tag_board_labels for target board
- **THEN** tag is NOT included in daily report for that board

### Requirement: Board articles API direction_mismatch filtering

getBoardArticles 的 filtered_tags 查询默认排除 direction_mismatch=true 的标签。通过 query param `?show_direction_mismatch=true` 可包含。

#### Scenario: default request
- **WHEN** getBoardArticles called without show_direction_mismatch param
- **THEN** filtered_tags excludes direction_mismatch=true tags

#### Scenario: explicit include
- **WHEN** getBoardArticles called with show_direction_mismatch=true
- **THEN** filtered_tags includes all tags including direction_mismatch=true

### Requirement: Match detail API returns direction_sim

getTagMatchDetail 响应新增 `direction_sim` 字段（float64），为 tag identity embedding 与 board embedding 的 cosine 值。无 embedding 时返回 null。

#### Scenario: direction_sim available
- **WHEN** both tag identity embedding and board embedding exist
- **THEN** response includes direction_sim value

#### Scenario: direction_sim unavailable
- **WHEN** tag or board embedding is NULL
- **THEN** response direction_sim is null/omitted

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

### Requirement: Direction floor configurable

DirectionFloor SHALL 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_floor`，默认 0（关闭）。值 > 0 时作为全库最优豁免的地板线：豁免仅对 cosine ≥ DirectionFloor 的挂载生效；cosine < DirectionFloor 的全库最优挂载豁免不生效，SHALL 回退绝对门与相对门判定（floor 是豁免的下限保险丝，不是第二把绝对刀）——防御「哪都不像却硬挂」的未来形态（当前真库全库最优挂载最低 cosine 0.515 且成色正确，默认关闭无操作量）。与现有匹配参数一致的管理方式。

#### Scenario: floor disabled by default
- **WHEN** ai_settings does not contain `semantic_board_match_direction_floor`
- **THEN** floor=0，全库最优豁免无额外下限

#### Scenario: floor overrides exemption
- **WHEN** ai_settings contains `semantic_board_match_direction_floor` with value 0.55 AND a mount is the tag's global best AND cosine=0.515
- **THEN** direction_mismatch=true（0.515 < floor 0.55，豁免不生效，绝对门照常判定）

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
