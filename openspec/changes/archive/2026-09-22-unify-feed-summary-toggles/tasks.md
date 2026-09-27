## 1. 后端闸门语义修复

- [x] 1.1 `buildArticleFromEntry`（feed_service.go）两个置总结状态分支加 `&& feed.CompletionOnRefresh`；补/改单测覆盖双开、仅主开、全关三种组合（firecrawl 与非 firecrawl 各一组），`go test ./internal/reader/...` 通过
- [x] 1.2 `job_firecrawl.go` 两处置 `summary_status="incomplete"`（成功完成处 + 终态失败降级处）加 `&& feed.CompletionOnRefresh`；更新 job_firecrawl_test.go 组合场景断言，`go test ./internal/admin/scheduler/...` 通过
- [x] 1.3 `ListReadyArticles`（content_completion_service.go）JOIN feeds 条件追加 `feeds.completion_on_refresh = true`；content_completion 相关单测补闸门关时积压文章不被捞起的断言
- [x] 1.4 审查次要路径：`ListArticlesIncomplete`（repository.go:457）查明调用方并按 D3 处理（加闸门或随死代码删除）；`job_blocked_article_recovery.go:51` 恢复查询加 `AND completion_on_refresh = true`；对应包测试通过
- [x] 1.5 删除死代码 `ListArticlesForCompletion`（repository.go:473）及其关联类型引用；`go build ./...` 与受影响包测试通过

## 2. 默认值与迁移

- [x] 2.1 `models/feed.go` 的 `CompletionOnRefresh` gorm tag `default:true` → `default:false`；全仓 grep 确认无其他 `default:true` 残留引用该字段
- [x] 2.2 新增幂等 Migration（postgres_migrations.go 既有模式）：①`UPDATE feeds SET completion_on_refresh=false WHERE completion_on_refresh=true`；②`UPDATE articles SET summary_status='complete' WHERE summary_status IN ('pending','incomplete')`；本地起库跑通、重复执行无副作用
- [x] 2.3 迁移后验证：`GetCompletionOverview` 待处理计数归零；新建 feed（不传开关）`completion_on_refresh=false`；`go test ./internal/platform/database/...` 通过

## 3. 前端 settings 补齐与词汇表统一

- [x] 3.1 FeedDetailEditor 新增「AI 总结」toggle 行（article_summary_enabled）与「刷新后自动总结」改名+新文案（词汇表见 proposal），"内容补全"旧名移除；更新 update-feed 事件类型与 SettingsSectionFeeds 透传
- [x] 3.2 FeedDetailEditor 增加最大重试次数（max_completion_retries）与 RSS 地址（url）编辑，保存走现有 PATCH 单字段/表单模式；组件测试覆盖渲染与事件
- [x] 3.3 订阅源列表（FeedMasterList/列表项）对 `article_summary_enabled=true` 展示「AI 总结」标识；组件测试覆盖开/关两态
- [x] 3.4 store/composable 映射统一：`completionOnRefresh` 回退一律 `?? false`；stores/api.test.ts 补默认值断言；`pnpm test:unit` 受影响文件通过

## 4. 主界面编辑入口统一

- [x] 4.1 SettingsSectionFeeds 支持 `?feed=<id>&section=feeds` 深链：列表就绪后选中定位、目标不存在优雅降级；组件测试覆盖两分支
- [x] 4.2 FeedLayoutShell 编辑入口改 `navigateTo` 深链跳转；删除 `EditFeedDialog.vue` 及引用、相关测试与类型残留；全仓 grep 无 EditFeedDialog 残留
- [x] 4.3 前端整体验证：`pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <受影响文件> --maxWorkers=2` 全绿

## 5. 集成验证与收尾

- [x] 5.1 端到端手工验证（start-dev.sh 起服务）：双开 feed 刷新自动总结；仅主开 feed 刷新不总结但文章页手动生成可用；主开关关的 feed 无手动入口且全文抓取正常；主界面编辑跳转定位成功
- [x] 5.2 部署验证：静态托管部署后迁移生效，settings 各 toggle 文案与词汇表一致，完成汇报含「部署后影响 + 需要的操作 + 旧数据降级」三段

## 6. 测试

- [x] 6.1 后端影响包（change-scope 判定）：`go test ./internal/models ./internal/reader/repository ./internal/reader/service ./internal/admin/scheduler ./internal/platform/database`，预期全绿，不顺手全量。
- [x] 6.2 前端受影响文件：`pnpm test:unit FeedDetailEditor FeedMasterList SettingsSectionFeeds stores/api.test --maxWorkers=2`，预期全绿。
- [x] 6.3 修复 `composite_components_migration_test.go` 测试卫生：金 schema 已建时跑全量 `RunAutoMigrate` 会按 GORM tag 剩掉迁移物化的 NOT NULL/DEFAULT（实测剥掉 scheduler_tasks.check_interval / semantic_labels.ref_count），导致 TestModelTagConstraints 在新测试改变 golden 构建顺序后必红；改为私有干净 schema + 收窄 AutoMigrate + 结束 ReimportTestDB 还原（归档前影响包红必须修，⌀11.4）。

## 7. 文档

<!-- doc-impact: flow, database -->

- [x] 7.1 `docs/reference/flow/content-enrichment.md`：总结触发链路同步双层闸门条件（buildArticleFromEntry 两分支 / job_firecrawl 置 incomplete 处 / ListReadyArticles 调度扫描）；变更溯源表补本 change 行（归档时补链接）。
- [x] 7.2 `docs/reference/database/tables/content.md`：feeds 表 `article_summary_enabled`（主开关语义）与 `completion_on_refresh`（自动闸门，默认 false，存量由迁移 20260920_0002 一次性关闭）两行语义改写。

## 8. 验证

| 命令 | 实测结果（归档前复跑） |
| --- | --- |
| `openspec validate unify-feed-summary-toggles` | ✓ valid |
| `bash scripts/harness/scenario-trace.sh openspec/changes/unify-feed-summary-toggles` | ✓ 20 个 Scenario 映射齐全（自动 18 / 人工 2） |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/unify-feed-summary-toggles` | ✓ 通过（声明 flow, database） |
| `bash scripts/harness/check-standards.sh --change unify-feed-summary-toggles` | ✓ A-D/F/G/H/I 全过；E 段 15 失败均为 2026-09-17/18/19 历史 archive 溯源欠账，与本 change 无关 |
| `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...` | ✓ 0 issues / 0 / 0 |
| `cd backend-go && go test -count=1 ./internal/models ./internal/reader/repository ./internal/reader/service ./internal/admin/scheduler ./internal/platform/database ./internal/topicgraph/service ./internal/topicgraph/repository` | ✓ 全绿（含 6.3 修复后 platform/database 全包） |
| `cd front && pnpm lint && pnpm exec nuxi typecheck` | ✓ 0 errors（7 warnings 存量）/ exit=0 |
| `cd front && NODE_ENV=test pnpm test:unit FeedDetailEditor FeedMasterList SettingsSectionFeeds stores/api.test --maxWorkers=2` | ✓ 4 文件全绿（与 lane 合跑 8 文件 84 用例全绿） |
| 端到端人工验证（tasks 5.1，本 change apply 会话已完成留痕）：双开自动总结 / 仅主开手动模式 / 主开关关无手动入口 / 主界面编辑深链跳转 | ✓ 全部符合（留痕见 5.1） |

| Scenario | 测试文件 |
| --- | --- |
| 手动模式 | backend-go/internal/reader/service/feed_service_unit_test.go |
| 关闭总结不影响抓取 | backend-go/internal/admin/scheduler/job_firecrawl_test.go |
| 完全关闭 | backend-go/internal/reader/service/feed_service_unit_test.go |
| 双开时抓取完成进入总结 | backend-go/internal/admin/scheduler/job_firecrawl_test.go |
| 自动闸门关闭时抓取完成不总结 | backend-go/internal/admin/scheduler/job_firecrawl_test.go |
| 非 firecrawl 双开时入库即待总结 | backend-go/internal/reader/service/feed_service_unit_test.go |
| 非 firecrawl 自动闸门关闭时入库不总结 | backend-go/internal/reader/service/feed_service_unit_test.go |
| 关闭自动闸门后积压文章不再消费 | backend-go/internal/reader/service/content_completion_service_test.go |
| 新建 feed 不传开关字段 | 人工端到端（tasks 2.3/5.1：新建 feed 缺省 completion_on_refresh=false） |
| 前端数据缺省回退 | front/app/stores/api.test.ts |
| 部署后存量 feed 自动总结全部关闭 | backend-go/internal/platform/database/completion_on_refresh_off_migration_test.go |
| 切换 Firecrawl toggle | front/app/features/settings/components/FeedDetailEditor.test.ts |
| 切换内容补全 toggle | front/app/features/settings/components/FeedDetailEditor.test.ts |
| 切换 AI 总结主开关 | front/app/features/settings/components/FeedDetailEditor.test.ts |
| 编辑 RSS 地址 | front/app/features/settings/components/FeedDetailEditor.test.ts |
| 主界面编辑跳转定位 | front/app/features/settings/components/SettingsSectionFeeds.test.ts |
| 深链目标不存在时优雅降级 | front/app/features/settings/components/SettingsSectionFeeds.test.ts |
| 删除订阅源能力保留 | 人工端到端（tasks 5.1：settings 详情删除入口承接原弹窗能力） |
| 开总结的 feed 列表可见标识 | front/app/features/settings/components/FeedMasterList.test.ts |
| 未开的 feed 不展示 | front/app/features/settings/components/FeedMasterList.test.ts |

映射注记：闸门组合矩阵在 `TestBuildArticleFromEntryTracksOnlyRunnableStates`（feed_service_unit_test.go）；firecrawl 闸门用例为 `TestFirecrawlJobCompletionGateOffSkipsSummaryMarking` / `TestFirecrawlJobTerminalFailureGateOffSkipsSummaryMarking`；调度闸门为 `TestListReadyArticlesRespectsCompletionGate`；迁移幂等/积压不消费为 `TestCompletionOnRefreshOffMigrationIdempotent` / `BacklogNotScanned`；前端用例号 FE-2/FE-3/FE-5/FE-6/FE-7/DL-1/DL-2 见各测试文件 describe。 |

UI 验收映射（ui-impact: minor）：toggle 词汇表/字段补齐 → FeedDetailEditor.test.ts 组件测试；列表标识 → FeedMasterList.test.ts；深链定位 → SettingsSectionFeeds.test.ts；端到端人工验证 → tasks 5.1 留痕。
