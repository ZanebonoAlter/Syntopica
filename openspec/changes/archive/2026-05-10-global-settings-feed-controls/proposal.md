## Why

Global Settings 的"订阅源配置"tab 交互效率低：分类不可折叠导致页面过长、feed 卡片缺少处理管线开关（tag / firecrawl / 补全）导致用户无法精细控制 AI 开销、最大文章数"无限制"实际是 9999 会误删文章。

## What Changes

- Feed 卡片新增 3 个 toggle：Firecrawl 全文抓取、打标签、内容补全，与现有 AI 摘要 toggle 并列展示处理管线全貌
- 后端 Feed 模型新增 `tagging_enabled` 字段（默认 true），`enqueueArticleProcessing` 据此决定是否入 tag 队列
- 分类标题改为可折叠，默认展开，点击切换收起/展开
- 最大文章数"无限制"改为 `0`（前端显示"无限制"），后端 `CleanupOldArticles` 将 `0` 视为不限制
- Firecrawl 完成后的回调（content completion）也需检查 feed 的 `tagging_enabled`

## Capabilities

### New Capabilities

- `feed-tagging-control`: Feed 级打标签开关，控制文章是否进入 tag 处理管线
- `feed-settings-ui`: 全局设置订阅源配置的交互改进（折叠分类、管线 toggle 矩阵、最大文章数修正）

### Modified Capabilities

## Impact

- 后端：Feed 模型加字段 + 1 次数据库迁移；`enqueueArticleProcessing` 逻辑变更；`CleanupOldArticles` 兼容 `max_articles=0`；Firecrawl 回调路径需检查 tagging 开关
- 前端：`GlobalSettingsDialog.vue` feed 卡片重构（加 toggle、加折叠、修正 select 选项）；`updateFeedSetting` 扩展支持的 setting key
- API：Feed update API 需接受新字段 `tagging_enabled`、`firecrawl_enabled`、`completion_on_refresh`
