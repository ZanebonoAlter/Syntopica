## 1. 后端：Feed 模型与 API

- [x] 1.1 Feed 模型新增 `TaggingEnabled bool gorm:"default:true"` 字段，更新 `ToDict()` 输出
- [x] 1.2 Feed handler 的 `CreateFeedRequest` 和 `UpdateFeedRequest` 新增 `tagging_enabled`、`firecrawl_enabled`、`completion_on_refresh` 字段
- [x] 1.3 Feed handler 的 update 路由处理新字段写入（参考现有 `article_summary_enabled` 模式）
- [x] 1.4 数据库迁移：确保 `tagging_enabled` 列被 auto-migrate 创建

## 2. 后端：处理管线逻辑

- [x] 2.1 `enqueueArticleProcessing` 新增 `tagging_enabled` 检查：firecrawl 关闭 + tagging 关闭时跳过所有队列
- [x] 2.2 `enqueueArticleProcessing` 处理 firecrawl 开启 + tagging 关闭：入 Firecrawl 队列但标记不触发 tag
- [x] 2.3 Firecrawl 回调（content completion service）完成时检查 feed 的 `tagging_enabled`，关闭则跳过 tag 入队
- [x] 2.4 `CleanupOldArticles` 兼容 `max_articles <= 0` 和 `max_articles >= 9999` 均视为不限制

## 3. 后端：单元测试

- [x] 3.1 `service_test.go` 补充 enqueueArticleProcessing 的 4 种组合场景测试（firecrawl×tagging 的 2×2 矩阵）
- [x] 3.2 `CleanupOldArticles` 测试 max_articles=0 不删除、max_articles=9999 不删除

## 4. 前端：Feed 卡片管线 toggle

- [x] 4.1 `updateFeedSetting` 函数扩展 setting 参数类型，支持 `tagging_enabled`、`firecrawl_enabled`、`completion_on_refresh`
- [x] 4.2 Feed 卡片底部重构：将单个"AI 总结"toggle 替换为 4 个 toggle 行（Firecrawl、打标签、AI 摘要、内容补全）
- [x] 4.3 每个 toggle 使用对应的 feed 字段绑定（`firecrawlEnabled`、`taggingEnabled`、`aiSummaryEnabled`、`completionOnRefresh`）

## 5. 前端：分类折叠

- [x] 5.1 新增 `collapsedCategories` ref（`Record<string, boolean>`），默认全部展开
- [x] 5.2 分类标题添加点击事件切换折叠状态，显示 ▼/▶ 图标
- [x] 5.3 分类下 feed 列表根据折叠状态显示/隐藏（`v-show`）

## 6. 前端：最大文章数修正

- [x] 6.1 `maxArticlesOptions` 的"无限制"值从 9999 改为 0
- [x] 6.2 `formatMaxArticles` 兼容 0 和 >=9999 都显示"无限制"

## 7. 验证

- [x] 7.1 后端 `go test ./internal/domain/feeds/... -v` 通过
- [x] 7.2 前端 `pnpm exec nuxi typecheck` 通过
- [x] 7.3 前端 `pnpm build` 通过
