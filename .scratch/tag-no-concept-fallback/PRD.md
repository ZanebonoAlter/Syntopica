# PRD: 标签 × 阅读偏好联动

> **状态**: 待讨论
> **优先级**: 中（用户体验提升方向）
> **关联**: `docs/reference/reading-preferences.md`, 关注标签功能

## 背景

当前阅读偏好系统完全基于 **feed 级** 和 **category 级** 行为（滚动深度、阅读时间、交互次数）。标签与阅读偏好之间**零联动**：

```
当前偏好维度:
  feed × 滚动深度 → feed 偏好分
  category × 阅读时间 → category 偏好分

缺失维度:
  tag × ??? → tag 偏好分  ← 空白
```

用户的唯一标签级过滤手段是**手动关注标签**（watched tags），这不反映阅读行为。

## 潜在价值

1. **自动标签偏好** — 用户频繁阅读 AI 相关文章 → 自动提升 AI 标签权重 → 信息流排序优化
2. **关注标签推荐** — 基于阅读行为推荐应该关注的标签，而非纯手动
3. **冷启动优化** — 新用户无手动关注时，可基于标签偏好做文章推荐

## 需要讨论的方案

### 方案 A: 标签偏好分
在 `reading_preferences` 表中新增 tag 维度：
- 每次文章阅读完成时，提取文章的 `article_topic_tags`
- 对每个标签累加阅读行为（时间、滚动、交互）
- 计算 `tag_preference_score`

```
score(tag) = 0.4 × read_time_pct + 0.3 × scroll_depth_pct + 0.3 × interaction_count_pct
```

### 方案 B: 标签偏好作为关注信号
不做独立偏好分，而是将阅读行为转化为关注建议：
- 连续 3 天阅读同一标签 ≥ 5 篇 → 推荐"关注此标签"
- 用户确认后添加到 watched tags

### 方案 C: 标签偏好融入现有排序
在文章排序（relevance sort）中，将标签偏好作为权重因子：
- 文章标签偏好分之和 × 权重 → 影响排序位置
- 不引入新 UI，纯后端排序优化

## 待决策问题

1. 标签偏好是否值得独立存在？还是只作为关注标签的推荐信号？
2. 性能影响：每篇文章 5 个标签，偏好计算频率可能很高
3. 是否需要标签偏好 UI（偏好标签列表、权重调节）？
4. 与现有 feed/category 偏好的关系——独立还是融合？

## 影响范围

- `backend-go/internal/domain/preferences/` — 偏好计算
- `backend-go/internal/domain/article/handler.go` — 文章排序
- `backend-go/internal/jobs/preference_update.go` — 偏好调度
- 前端关注标签推荐 UI
- 新增 `tag_reading_preferences` 表（如果独立存储）
