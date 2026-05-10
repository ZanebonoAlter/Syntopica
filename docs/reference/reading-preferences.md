# 阅读偏好指南

## 功能说明

系统会记录阅读行为，用来生成偏好分数，并为 AI 总结和排序提供参考。

## 记录内容

- 文章打开和关闭
- 滚动深度
- 阅读时长
- 收藏行为

## 数据链路

```text
ArticleContent
  -> reading behavior API
  -> reading_behaviors table
  -> preference aggregation
  -> user_preferences table
  -> settings panel display
```

## 偏好分数计算

偏好分数由 `PreferenceService.calculatePreferenceScore()` 计算，综合以下维度：

| 维度 | 权重 | 说明 |
|------|------|------|
| 滚动深度 | 40% | `avgScrollDepth / 100 * 0.4` |
| 阅读时长 | 30% | `min(avgTime / 180, 1) * 0.3` |
| 交互次数 | 30% | `min(totalEvents / 50, 1) * 0.3` |

最终分数会施加时间衰减：`baseScore * exp(-daysSinceInteraction / 30)`，30 天半衰期。分数范围 0–1。

## 前端位置

- 阅读追踪：`front/app/composables/useReadingTracker.ts`
- 偏好状态：`front/app/stores/preferences.ts`

## 后端位置

- 行为接口：`backend-go/internal/domain/preferences/handler.go`
- 偏好服务：`backend-go/internal/domain/preferences/service.go`

## 偏好更新调度器

`PreferenceUpdateScheduler`（间隔 1800 秒）定时执行 `UpdateAllPreferences()`，完整流程：

1. 修复孤立的阅读行为记录（关联已删除的分类）
2. 删除不可恢复的孤立记录
3. 删除孤立的偏好记录（关联已删除的 feed/分类）
4. 清空 `user_preferences` 表
5. 基于 `reading_behaviors` 重新计算所有 feed 偏好
6. 基于 `reading_behaviors` 重新计算所有分类偏好

## 相关接口

- `POST /api/reading-behavior/track` — 记录单条阅读行为
- `POST /api/reading-behavior/track-batch` — 批量记录阅读行为
- `GET /api/reading-behavior/stats` — 获取阅读统计
- `GET /api/user-preferences` — 获取用户偏好（支持 `?type=feed` 或 `?type=category` 过滤）
- `POST /api/user-preferences/update` — 手动触发偏好重新计算
