<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: content-enrichment, ai-summary, reading, scheduler -->

## Why

`completion_on_refresh`（"刷新后自动总结"）已名存实亡：其唯一消费点 `ListArticlesForCompletion` 是零调用方的死代码，7 个开总结的 feed 全部是 firecrawl feed，其总结触发只看 `article_summary_enabled`（`job_firecrawl.go:226` 与 `ListReadyArticles` 均不检查 `completion_on_refresh`）——用户在界面上关掉"刷新后自动总结"根本不生效，每天约 46 篇 AI 总结持续消耗树莓派与 AI 配额资源（多为排行榜 feed 重复文章）。同时主界面 `EditFeedDialog` 与 settings `FeedDetailEditor` 双编辑入口能力互补残缺（前者有 URL/总结开关但无刷新间隔/打标签，后者相反）、同一字段两套文案互相矛盾，编辑体验割裂。

## What Changes

- **`completion_on_refresh` 语义修复为真正生效的"刷新后自动总结"闸门**：`buildArticleFromEntry` 两个分支、`job_firecrawl.go` 抓取完成置 `incomplete` 处、`ListReadyArticles` 调度扫描处，三处统一加 `article_summary_enabled && completion_on_refresh` 条件。
- **默认值统一为 `false`**：gorm tag `default:true` 改 `default:false`；修复 `EditFeedDialog` 同文件内 `?? false` / `?? true` 回退值不一致 bug；新建 feed 路径（AddFeedDialog 不传字段 + Go 零值 false）保持不变。
- **存量数据迁移**：一次性 `UPDATE feeds SET completion_on_refresh = false`（当前 24/24 全为 true 的历史遗产），部署后自动总结全部关闭。
- **组合语义明确（方案 A）**：`article_summary_enabled`（主开关）+ `completion_on_refresh`（自动闸门）四档组合——on+off 为手动模式：刷新不自动总结，文章页手动生成按钮保留（手动按钮显示仅依赖主开关，后端手动入口本就不检查 feed 开关）。
- **死代码清理**：删除 `ListArticlesForCompletion`（repository.go:473，零调用方）。
- **settings 可视化补齐**：`FeedDetailEditor` 新增"AI 总结"主开关（`article_summary_enabled`）与最大重试次数（`max_completion_retries`）与 URL 编辑（自 EditFeedDialog 迁移）；误导性命名"内容补全/刷新时自动补全文章正文"改为"刷新后自动总结"；feed 列表可见总结状态。
- **主界面编辑统一到 settings**：移除 `EditFeedDialog`（其删除能力 settings 已有），主界面"编辑订阅源"入口改为跳转 settings feed 详情深链（`/settings?feed=<id>&section=feeds`）；settings 支持该深链定位。
- **语义词汇表统一**（两处 UI 共用同一套文案）：
  | 字段 | 统一名称 | 说明文案 |
  |---|---|---|
  | `article_summary_enabled` | AI 总结 | 为文章生成 AI 整理稿；开启后文章页可手动生成 |
  | `completion_on_refresh` | 刷新后自动总结 | 新文章自动排队总结；关闭后仅手动生成 |
  | `firecrawl_enabled` | 全文抓取 | 用 Firecrawl 抓取完整正文供阅读，与总结独立 |
  | `tagging_enabled` | AI 打标签 | 自动为文章生成主题标签 |

## Capabilities

### New Capabilities

- `feed-summary-trigger`: feed 总结触发链路的字段语义契约——`article_summary_enabled`（能力主开关，含手动总结可用性）与 `completion_on_refresh`（自动触发闸门）的组合行为、默认值、存量迁移、firecrawl/非 firecrawl 双路径的触发条件。

### Modified Capabilities

- `feed-settings-ui`: feed 编辑的单一入口契约——settings 详情编辑器为唯一编辑界面（补齐 AI 总结开关/最大重试/URL/删除），主界面编辑入口改为深链跳转；toggle 文案与词汇表统一；默认值回退统一 false；移除 EditFeedDialog 双入口。

## Impact

- **后端** `backend-go`：`internal/models/feed.go`（gorm 默认值）、`internal/reader/service/feed_service.go`（buildArticleFromEntry）、`internal/admin/scheduler/job_firecrawl.go`（置 incomplete 条件）、`internal/reader/service/content_completion_service.go`（ListReadyArticles 查询条件）、`internal/reader/repository/repository.go`（删死代码）、数据迁移（存量 completion_on_refresh 置 false）。
- **前端** `front/app`：`features/settings/components/FeedDetailEditor.vue`（补开关/字段）、`SettingsSectionFeeds.vue`（深链定位）、`features/shell/components/FeedLayoutShell.vue`（编辑入口改跳转）、`components/dialog/EditFeedDialog.vue`（删除）、相关 store/composable 类型。
- **API**：无路由变更；`feeds` 资源字段语义收紧（`completion_on_refresh` 从死字段变为生效闸门）。
- **部署后用户可见行为**：所有 feed 刷新后不再自动生成 AI 总结；文章页手动总结按钮不受影响；主界面"编辑订阅源"跳转 settings。
- **需要的操作**：部署后自动总结静默关闭（迁移自动执行）；想恢复某个 feed 的自动总结需到 settings 手动打开"刷新后自动总结"（建议主开关也开着）。
