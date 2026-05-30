# article-sort-priority

## Summary

`getBoardArticles` 返回的文章按匹配质量分层排序，替代纯时间线排序。排序在 Go 端内存中完成。

## Behavior

### 排序规则

排序 key: `(tier ASC, score DESC, publish_time DESC)`

| Tier | match_reason | 条件 |
|------|-------------|------|
| 0 | direct_hit | — |
| 1 | hit_rate | — |
| 2 | max_sim | !downgraded |
| 3 | max_sim | downgraded |
| 3 | weighted | — |

- 文章有多个 tag 时，取 tier 最高的那个作为排序依据
- 同 tier 同 score 时按发布时间倒序

### 实现方式

- `filtered_tags` 已在内存中携带 `match_reason`/`score`/`downgraded`
- 在 Go 端遍历每篇文章的 tags，计算 best tier，然后 `sort.Slice` 排序
- 不使用 SQL ORDER BY（需要窗口函数，复杂度高）

### 不变项

- 文章内容、分页逻辑不变
- `direction_mismatch` 过滤逻辑不变（默认隐藏，`show_direction_mismatch=true` 显示）
- 如果文章的 tag 全部是 `direction_mismatch=true` 且未开启 `show_direction_mismatch`，该文章不出现在列表中

### 排序模式切换（P6 补充）

`getBoardArticles` 新增 `sort` 查询参数：
- `quality`（默认）: 上述 tier + score + pub_date 排序
- `time`: DB 直接按 `pub_date DESC, id DESC` 排序，跳过内存质量排序

前端文章列表 header 新增「质量/时间」切换按钮组。

## Test Cases

- direct_hit 文章排在 hit_rate 文章前面
- 同 tier 内 score 高的排前面
- 同 tier 同 score 时时间新的排前面
- 多 tag 文章取最高 tier 排序
