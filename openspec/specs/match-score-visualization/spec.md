## Purpose

匹配得分可视化功能，通过颜色编码和分数显示直观展示 tag-board 匹配质量和匹配方式。

## Requirements

### Requirement: Tag chip 匹配方式颜色编码
Board 文章列表中每个 tag chip SHALL 根据 `match_reason` 字段用颜色区分匹配方式：
- `direct_hit` → 绿色(#22c55e)
- `hit_rate` → 蓝色(#3b82f6)
- `max_sim` → 橙色(#f59e0b)
- `weighted` → 灰色(#94a3b8)

颜色 SHALL 应用于 chip 的 border 或 background。数据来自 `topic_tag_board_labels` 表的 `match_reason` 和 `score` 字段。

#### Scenario: 文章有三种匹配方式的标签
- **WHEN** 文章 #101 的 filtered_tags 含 GPT-5发布(match_reason="max_sim")、AI芯片(direct_hit)、AI竞赛(weighted)
- **THEN** tag chips SHALL 分别显示为 橙色、绿色、灰色

#### Scenario: 所有标签同一种匹配方式
- **WHEN** 文章 #102 的 filtered_tags 全部为 match_reason="hit_rate"
- **THEN** 所有 tag chips SHALL 显示为 蓝色

### Requirement: Tag chip 分数文字显示
每个 tag chip 内 SHALL 显示分数文字，chip 格式为 `[标签名 分数]`（如 `[GPT-5发布 0.85]`）。分数 SHALL 保留两位小数。direct_hit 的 score 固定为 1.00。

#### Scenario: 显示分数
- **WHEN** tag chip "GPT-5发布" 的 score=0.85
- **THEN** chip 文字 SHALL 显示为 `GPT-5发布 0.85`

#### Scenario: direct_hit 分数
- **WHEN** tag chip "AI芯片" 的 match_reason="direct_hit", score=1.0
- **THEN** chip 文字 SHALL 显示为 `AI芯片 1.00`

### Requirement: 文章行最强匹配信息
每篇文章行右侧 end 处 SHALL 显示该文章在当前 board 中的最强匹配信息：匹配方式中文名（直接命中/命中率/相似度/综合）+ 最高分数。系统 SHALL 从该文章所有 filtered_tags 中选取 score 最高的 tag 作为最强匹配。

#### Scenario: 多标签取最高分
- **WHEN** 文章 #101 的 filtered_tags 为 [GPT-5发布(max_sim, 0.85), AI竞赛(hit_rate, 0.92), AI芯片(direct_hit, 1.00)]
- **THEN** 文章行右侧 SHALL 显示"直接命中 1.00"

#### Scenario: 仅一个标签
- **WHEN** 文章 #102 的 filtered_tags 仅有一个 tag (max_sim, 0.78)
- **THEN** 文章行右侧 SHALL 显示"相似度 0.78"

### Requirement: 匹配工具函数
前端 SHALL 提供 `matchReasonColor(reason: string): string` 和 `matchInfoLabel(tag: BoardArticleTag): string` 工具函数。`matchReasonColor` 返回对应颜色 HEX 值，`matchInfoLabel` 返回"匹配方式中文名 + 分数"字符串。

#### Scenario: matchReasonColor
- **WHEN** 调用 `matchReasonColor("max_sim")`
- **THEN** SHALL 返回 `"#f59e0b"`

#### Scenario: matchInfoLabel
- **WHEN** 调用 `matchInfoLabel({match_reason: "hit_rate", score: 0.75})`
- **THEN** SHALL 返回 `"命中率 0.75"`
