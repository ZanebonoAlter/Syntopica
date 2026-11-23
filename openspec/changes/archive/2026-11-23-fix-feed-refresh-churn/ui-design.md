# UI Design: fix-feed-refresh-churn

<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯后端/数据层修复**：

- 改动对象为刷新判变比较、归档冻结、`tag_jobs` 部分唯一索引迁移与入队 SQL
  （`backend-go/internal/reader/service/feed_service.go`、
  `backend-go/internal/tagmanagement/repository/tag_job_queue.go`、
  `backend-go/internal/platform/database/tag_job_unique_migration.go`）
- 不触碰 `front/`，不新增页面、组件、样式或交互
- 用户可见界面与接口契约不变（文章内容/标签结果只是不再被无意义地重复清理重打）

`ui-impact: none` 与实际改动面一致。
