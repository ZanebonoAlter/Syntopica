# match-score-visualization — retire-downgraded-mount delta

## MODIFIED Requirements

### Requirement: Tag chip 匹配方式颜色编码
Board 文章列表中每个 tag chip SHALL 根据 `match_reason` 字段用颜色区分匹配方式：
- `direct_hit` → 绿色(#22c55e)
- `hit_rate` → 蓝色(#3b82f6)
- `max_sim` → 橙色(#f59e0b)
- `weighted` → 灰色(#94a3b8)

颜色 SHALL 应用于 chip 的 border 或 background。**【2026-11-12 本 change 起生效】前端 SHALL NOT 再因「降级匹配」降低视觉权重**（降级标记已退役）——所有 chip 一律使用规则色的完整亮度。数据来自 `topic_tag_board_labels` 表的 `match_reason` 与 `score` 字段。

#### Scenario: 文章有三种匹配方式的标签
- **WHEN** 文章 #101 的 filtered_tags 含 GPT-5发布(match_reason="max_sim")、AI芯片(direct_hit)、AI竞赛(weighted)
- **THEN** tag chips SHALL 分别显示为 橙色、绿色、灰色

#### Scenario: 所有标签同一种匹配方式
- **WHEN** 文章 #102 的 filtered_tags 全部为 match_reason="hit_rate"
- **THEN** 所有 tag chips SHALL 显示为 蓝色

### Requirement: Tag chip 分数文字显示
每个 tag chip 内 SHALL 显示分数文字，chip 格式为 `[标签名 分数]`（如 `[GPT-5发布 0.85]`）。分数 SHALL 保留两位小数。direct_hit 的 score 固定为 1.00。**【2026-11-12 本 change 起生效】chip SHALL NOT 再追加 "↓" 后缀**（降级标记已退役）。

#### Scenario: 正常匹配 tag chip 显示
- **WHEN** tag chip 的 match_reason="max_sim"，score=0.85
- **THEN** tag chip SHALL 使用对应规则色（#f59e0b）的完整亮度边框，显示 "相似度 0.85"

#### Scenario: 降级匹配 tag chip 显示
- **WHEN** tag chip 的 match_reason="max_sim"，score=0.85（历史 downgraded=true 的行现已无此字段）
- **THEN** tag chip SHALL 与其它 max_sim chip 一致：完整亮度规则色边框 + "相似度 0.85"，SHALL NOT 追加 "↓"、SHALL NOT 降低不透明度

#### Scenario: direct_hit 和其他规则不受降级影响
- **WHEN** tag chip 的 match_reason="direct_hit" 或 match_reason="hit_rate" 或 match_reason="weighted"
- **THEN** tag chip SHALL 正常显示

#### Scenario: 显示分数
- **WHEN** tag chip "GPT-5发布" 的 score=0.85
- **THEN** chip 文字 SHALL 显示为 `GPT-5发布 0.85`

#### Scenario: direct_hit 分数
- **WHEN** tag chip "AI芯片" 的 match_reason="direct_hit", score=1.0
- **THEN** chip 文字 SHALL 显示为 `AI芯片 1.00`

### Requirement: 匹配工具函数
前端 SHALL 提供 `matchReasonColor(reason: string): string` 和 `matchInfoLabel(tag: BoardArticleTag): string` 工具函数。`matchReasonColor` 返回对应颜色 HEX 值，`matchInfoLabel` 返回"匹配方式中文名 + 分数"字符串。**【2026-11-12 本 change 起生效】`matchReasonColor` SHALL 只接受 reason 单参**（不再有 downgraded 淡化分支），`matchInfoLabel` SHALL NOT 输出 "↓" 后缀。

#### Scenario: matchReasonColor
- **WHEN** 调用 `matchReasonColor("max_sim")`
- **THEN** SHALL 返回 `"#f59e0b"`

#### Scenario: matchInfoLabel
- **WHEN** 调用 `matchInfoLabel({match_reason: "hit_rate", score: 0.75})`
- **THEN** SHALL 返回 `"命中率 0.75"`

### Requirement: 匹配详情面板降级说明和方向校验展示
`MatchDetailPanel.vue` 的匹配流程步骤 SHALL 在 max_sim 命中时展示实际使用的 hits 门槛与方向校验结果。步骤 ④（max_sim）展示方向校验结果和 direction_sim 值。**【2026-11-12 本 change 起生效】步骤 SHALL NOT 再显示「⚠ 降级匹配（原阈值 X，因仅有 N 个辅助标签降为 M）」**（降级标记已退役）——minHits 的实际取值由前端自算门槛表达，不带降级语义。

#### Scenario: 降级匹配流程步骤
- **WHEN** tag 以 max_sim 匹配，有 1 个辅助标签，direct_max_sim_min_hits=2
- **THEN** 匹配流程步骤 ④ SHALL 显示命中数值与门槛（如 "✓Smax=0.85 ✓1≥1 命中 ✓R=0.33 → 满足！"），SHALL NOT 出现「⚠ 降级匹配」字样

#### Scenario: 正常匹配流程步骤不变
- **WHEN** tag 以 max_sim 匹配
- **THEN** 匹配流程步骤 SHALL 按原有方式显示，无降级提示

#### Scenario: 方向校验通过
- **WHEN** max_sim 匹配且 direction_sim >= threshold
- **THEN** 步骤 ④ 展示 "方向校验 ✓ sim=X≥Y"

#### Scenario: 方向校验未通过
- **WHEN** max_sim 匹配且 direction_sim < threshold
- **THEN** 步骤 ④ 展示方向校验未通过标记（沿用既有文案）
