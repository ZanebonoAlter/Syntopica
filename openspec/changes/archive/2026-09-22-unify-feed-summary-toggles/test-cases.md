# Test Cases: unify-feed-summary-toggles

> 白盒用例（complex 档）。核心维度：`article_summary_enabled`（S，能力主开关）× `completion_on_refresh`（G，自动闸门）× `firecrawl_enabled`（F，全文抓取，独立正交）组合矩阵，贯穿标记侧（入库置状态）/ 扫描侧（调度捞取）/ 恢复侧（告警统计）/ 迁移（存量归位）四层。"无需总结"的存储值沿用既有 `complete`。层选择：纯逻辑单测（SQLite/内存）为主，前端组件 vitest；端到端走 tasks §5 手工验证。

## 一、标记侧：buildArticleFromEntry（feed_service.go，入库置 summary_status）

组合矩阵（S=主开关、G=自动闸门、F=firecrawl；预期列指 `summary_status` 初值）：

| # | S | G | F | 预期 | 断言判据 |
| --- | --- | --- | --- | --- | --- |
| BE-1 | T | T | T | `incomplete`（旧行为不变） | 等全文抓取后进总结流程 |
| BE-2 | T | F | T | `complete`（无需总结） | 闸门关，firecrawl 完成后也不置 incomplete |
| BE-3 | F | * | T | `complete`（不变） | 主开关关，无总结 |
| BE-4 | T | T | F | `pending`（旧行为不变） | 非 firecrawl 直接收 RSS 素材 |
| BE-5 | T | F | F | `complete`（无需总结） | 闸门关，非 firecrawl 路径也不进队 |
| BE-6 | F | * | F | `complete`（不变） | 主开关关 |
| BE-7 | F | T | F | `complete` | 仅闸门开、主开关关 → 仍无总结（G 不越过 S） |
| BE-8 | * | * | F | `firecrawl_status=completed` | 非 firecrawl 路径抓取状态不受两开关影响 |
| BE-9 | * | * | T | `firecrawl_status=pending` | firecrawl 队列不受总结开关影响（spec：关总结不影响抓取） |

## 二、标记侧：job_firecrawl.go 两处置 incomplete

抓取成功完成处 + 抓取终态失败降级 RSS 处，条件由 `feed.ArticleSummaryEnabled` 收紧为 `&& feed.CompletionOnRefresh`：

| # | S | G | 场景 | 预期 |
| --- | --- | --- | --- | --- |
| BF-1 | T | T | 抓取成功 | `summary_status` 更新为 `incomplete`，其余 updates（firecrawl_status=completed 等）正常 |
| BF-2 | T | F | 抓取成功 | `summary_status` 不在 updates 中，保持原值（BE-2 置的 `complete`） |
| BF-3 | F | T | 抓取成功 | 同 BF-2 |
| BF-4 | T | T | 终态失败（AttemptCount≥MaxAttempts） | `summary_status` 更新为 `incomplete`（降级 RSS 素材路径保留） |
| BF-5 | T | F | 终态失败 | 不 Update `summary_status`；tagging fallback（tag_jobs enqueue）不受影响仍执行 |
| BF-6 | T | F | 非终态失败 | 行为不变：MarkFailed + backoff，无 summary_status 写入（既有逻辑） |

## 三、扫描侧：ListReadyArticles（content_completion_service.go）

JOIN feeds 追加 `feeds.completion_on_refresh = true`：

| # | 场景 | 预期 |
| --- | --- | --- |
| BS-1 | S=T G=T，文章 incomplete/pending 且 firecrawl_status IN (completed,failed) | 被捞起（旧行为不变），limit 截断生效 |
| BS-2 | S=T G=F，同上文章（迁移前积压） | 不被捞起——闸门关后积压不消费 |
| BS-3 | S=F G=T，文章 incomplete | 不被捞起（主开关优先，既有条件） |
| BS-4 | S=T G=T，文章 summary_status=complete | 不被捞起（既有） |
| BS-5 | stale 租约（summary_processing_started_at ≤ staleBefore） | 捞起逻辑不变（既有，回归保护） |

## 四、恢复侧与死代码

| # | 场景 | 预期 |
| --- | --- | --- |
| BR-1 | STAT-05 告警计数：S=T G=F 的 incomplete 文章 | 不计入 blocked count（条件加 `completion_on_refresh = true`） |
| BR-2 | STAT-05：S=T G=T 的 incomplete 且 firecrawl_status≠completed | 照旧计入（回归保护） |
| BR-3 | 恢复循环（firecrawl_status IN waiting_for_firecrawl/blocked → pending） | 不受两开关影响，行为不变（只关 firecrawl 卡死） |
| BR-4 | `ListArticlesIncomplete`（repository.go:454） | 零调用方（生产+测试 grep 确认）→ 随死代码删除 |
| BR-5 | `ListArticlesForCompletion` + `ItemQuery`（repository.go:473/560） | 零调用方 → 删除；`go build ./...` 通过，全仓 grep 无残留 |

## 五、默认值与迁移（postgres_migrations.go 既有幂等模式）

| # | 场景 | 预期 |
| --- | --- | --- |
| MG-1 | gorm tag | `CompletionOnRefresh` `default:true` → `default:false`，全仓 grep 无该字段其他 default:true |
| MG-2 | 迁移① feeds | `UPDATE feeds SET completion_on_refresh=false WHERE completion_on_refresh=true`；执行后 0 行 true |
| MG-3 | 迁移② articles | `summary_status IN ('pending','incomplete')` → `'complete'`；**failed/stale/''/complete 不动**（failed 保留失败可观测性） |
| MG-4 | 幂等 | 同一迁移重复执行：第二次 RowsAffected=0，无副作用 |
| MG-5 | 新建 feed 不传开关 | `completion_on_refresh=false`（CreateFeedRequest Go 零值 + DB default 一致） |
| MG-6 | 迁移后 GetCompletionOverview | 待处理计数（incomplete+pending）归零 |
| MG-7 | 已生成整理稿 | AIContentSummary 数据不删不改（迁移只动状态位） |

## 六、手动总结路径（保持不动，方案 A 回归保护）

| # | 场景 | 预期 |
| --- | --- | --- |
| MA-1 | CompleteArticle（POST /api/content-completion/articles/:id/complete） | 只检查 `ArticleSummaryEnabled`，**不检查** `CompletionOnRefresh`；force 语义不变 |
| MA-2 | 前端手动按钮显示（useArticleContentView） | 仅依赖 `article_summary_enabled`，闸门关不影响手动入口 |
| MA-3 | S=F | 后端返回 "AI summary not enabled"，前端无手动按钮 |

## 七、前端 settings（组件 vitest）

| # | 场景 | 预期 |
| --- | --- | --- |
| FE-1 | FeedDetailEditor 渲染 | 四 toggle 行文案=词汇表：「AI 总结」「刷新后自动总结」「全文抓取」「AI 打标签」+各自说明文案；无「内容补全」旧名 |
| FE-2 | 切换「AI 总结」toggle | emit update-feed → PATCH article_summary_enabled |
| FE-3 | 切换「刷新后自动总结」toggle | emit update-feed → PATCH completion_on_refresh |
| FE-4 | 编辑最大重试次数 | number 输入渲染当前 max_completion_retries，保存 PATCH |
| FE-5 | 编辑 RSS 地址 | url 输入渲染当前值，保存 PATCH url；列表与详情反映新地址 |
| FE-6 | FeedMasterList/列表项 S=T | 展示「AI 总结」标识 |
| FE-7 | FeedMasterList/列表项 S=F | 无该标识 |
| FE-8 | store 映射 | feed 数据缺 completionOnRefresh/null → `?? false`；api.test.ts 补默认值断言 |

## 八、深链与入口收敛（组件 vitest + grep）

| # | 场景 | 预期 |
| --- | --- | --- |
| DL-1 | `?feed=<存在id>&section=feeds` | 列表就绪后自动选中该 feed、详情展开 |
| DL-2 | `?feed=<不存在id>` | 优雅降级默认视图，不报错 |
| DL-3 | 列表加载失败重试后 | URL 参数保留，重试成功仍能定位 |
| DL-4 | 主界面「编辑订阅源」菜单 | `navigateTo('/settings?feed=<id>&section=feeds')`，无弹窗 |
| DL-5 | 删除能力承接 | settings 详情删除按钮确认后走既有 deleteFeed，行为与原弹窗一致 |
| DL-6 | EditFeedDialog 清理 | 组件文件、引用、测试、类型全删，全仓 grep 无残留 |

## 九、既有不变量回归（改动面邻接，跑通即可）

| # | 场景 | 预期 |
| --- | --- | --- |
| RG-1 | content_completion 状态机 | incomplete → pending → complete/failed 流转、claim 乐观锁、32min stale 租约逻辑均不变 |
| RG-2 | tag_jobs | firecrawl 完成后 `TaggingEnabled` enqueue 不受闸门影响 |
| RG-3 | 快讯 upsert（同 link 未变内容） | 不触发处理链（既有约束 11） |
| RG-4 | auto_refresh 预埋状态位 | 走 buildArticleFromEntry 同一路径，随 BE 矩阵联动 |

## 测试文件映射（落点）

- §一/§九：`internal/reader/service/` feed_service 既有测试文件扩展（或新增 feed_service_test.go 用例）
- §二：`internal/admin/scheduler/job_firecrawl_test.go` 组合场景
- §三/§六：`internal/reader/service/content_completion_service_test.go`（既有）
- §四：`internal/reader/repository/` + `internal/admin/scheduler/job_blocked_article_recovery` 相关测试
- §五：`internal/platform/database/postgres_migrations_test.go`（幂等断言）；MG-6/MG-2/3 归 §5 手工验证
- §七/§八：`front/` 对应组件 `.spec.ts` / `api.test.ts` 扩展
