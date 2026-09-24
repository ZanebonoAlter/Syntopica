## 1. 用例先行（复杂档）

- [x] 1.1 复核 `test-cases.md` §3「继承与调整」：动工前先跑 `cd backend-go && go test ./internal/reader/handler/ -run TestGetArticle` 与 `cd front && pnpm test:unit app/features/articles/components/ArticleContentPreviewPanel.test.ts --maxWorkers=2`，记录旧断言中依赖「列表带正文/完整 description」的用例清单。验证：命令输出中列出需按新契约调整的用例名，写入 `evidence/pre-change-tests.txt`
- [x] 1.2 把 `test-cases.md` §5 白盒分支表落到测试骨架（先写会失败的断言）。验证：`cd backend-go && go test ./internal/reader/handler/ -run TestGetArticles_Projection` 出现预期失败（红）而非编译错误

## 2. 后端：列表窄投影 + excerpt + per_page 可观测

- [x] 2.1 `article_handler.go` 的 `GetArticles` 四个 `Select` 分支统一改为显式窄列清单（id/feed_id/title/link/image_url/pub_date/author/read/favorite/summary_status/created_at/archived + `feeds.category_id` + relevance 分支的 `relevance_score`），移除 `articles.*` 与 `content`/`description`/`firecrawl_content`/`ai_content_summary`。验证：`go test ./internal/reader/handler/ -run TestGetArticles` 全绿且断言项内无 `content` 字段
- [x] 2.2 新增 `excerpt` 派生列（`description` 去标签折叠后截断 200 字符，空则回退 `content`），随列表项返回（源空时返回 `""`）。验证：`go test ./internal/reader/handler/ -run TestExcerpt` 覆盖 §5.2 边界值全绿
- [x] 2.3 `per_page` 超上限改为「clamp 到 100 + `logging.Warnf` 记录请求值与路径」。验证：`go test ./internal/reader/handler/ -run TestPerPageOverLimit` 断言 100 条与 WARN 各一条；`per_page=101` 同样命中
- [x] 2.4 复核列表不引入新的 N+1：`tag_count` 保持既有逐行子查询（`articles.tag_count` 列 97.5% NULL，不可用，design D8）。验证：`explain analyze` 粘贴到 `evidence/`，比较动工前后同一请求的 rows/循环特征无恶化
- [x] 2.5 真库体积量化（只读）：`curl -s --noproxy '*' -o /tmp/list.json -w '%{size_download}\n' 'http://127.0.0.1:5100/api/articles?per_page=20'`。验证：输出 < 102400（目标 100 KB 内），且 `grep -o '"content"' /tmp/list.json | wc -l` 输出 0
- [x] 2.6 **（实现中发现的设计补丁一，2026-09-24 用户确认）** 导语去重抑制：`buildExcerpt` 在「导语与正文重复」时返回 `""`（判据必须是前端 guard 规则的子集：归一化后完全相同，或导语归一化 ≥ 40 字符且被归一化正文包含）。理由：实测非归档文章 `description == content` 占 92%，下发这类 excerpt 会让点选时导语闪现再消失（布局跳动）。验证：`go test ./internal/reader/handler/ -run TestExcerpt` 覆盖「同源/子串/短导语/不重复」四类；浏览器点选首页文章断言 `.lede` 自始至终不存在
- [x] 2.7 **（设计补丁二）** 正文兜底导语抑制：导语来自 `content`（`description` 无实质文本）且该文章无 Firecrawl 正文时返回 `""`（**无 40 rune 门槛**，因为 guard 的相等规则无门槛；实测该类 198 条、其中带 Firecrawl 正文 0 条，浏览器实测 V2EX 短帖仍会闪现）。实现：excerpt 源查询加 `CASE WHEN firecrawl_content = '' THEN 1 ELSE 0 END`（零额外搬运）。验证：`go test ./internal/reader/handler/ -run TestExcerpt` 覆盖「兜底短帖无 firecrawl → 空 / 有 firecrawl → 保留」；真库扫描非空 excerpt 从 33 降到 ~18（均为 description 与正文不同的真导语）

## 3. 前端：按新契约消费

- [x] 3.1 `ArticlePayload`/`Article` 类型的 `content`/`description`/`firecrawl_content` 转为可选，`excerpt` 加入类型；`normalizeArticle` 对缺失给空串兜底。验证：`pnpm exec nuxi typecheck` 0 error；`pnpm test:unit app/api/normalizers --maxWorkers=2`（若有）绿
- [x] 3.2 阅读页导语按优先级取值：详情 `description` 优先，未就绪回退列表 `excerpt`（`ArticleContentPreviewPanel` / `ArticleContentView` 的 `previewProps` 传参处）。验证：`pnpm test:unit app/features/articles/components/ArticleContentPreviewPanel.test.ts --maxWorkers=2` 新增「详情未就绪用 excerpt」「详情返回覆盖」两用例绿
- [x] 3.3 全仓反查列表来源的正文/描述消费点，确认无遗漏（`grep -rn '\.content\b\|\.description\b' front/app --include='*.vue' --include='*.ts' | grep -v test`）。验证：逐条判定并记录到 `evidence/consumers.md`，无可疑未处理项
- [x] 3.4 opencli/agent-browser 端到端（静态 `:5100`）：点选一篇文章，断言正文非空、导语先是 excerpt 后为详情文本、无控制台报错。验证：断言输出与截图存 `evidence/`（对应 test-cases 节拍 5）

## 4. 前端：刷新编排收敛

- [x] 4.1 `useAutoRefresh.ts` 删除 `fetchArticles({ per_page: 10000 })`，改为「按当前筛选 + 当前页重取」（复用 `useArticlePagination` 当前状态 / `FeedLayoutShell.loadArticles`）。验证：`pnpm test:unit app/features/feeds/composables/useAutoRefresh.test.ts --maxWorkers=2` 断言刷新完成后无 `per_page > 100` 请求
- [x] 4.2 每 feed 独立 `setInterval` 改为单一调度器（`{feedId, intervalMinutes, nextDueAt}` + 单 `setTimeout` 串行推进，刷新完成后重排）。验证：同测试文件用 fake timers 推进 60 分钟，断言同分钟至多 1 个刷新触发
- [x] 4.3 刷新失败不终止调度（记录并继续排下一个 due）。验证：同测试文件注入刷新 reject，断言后续 feed 仍被调度
- [x] 4.4 刷新后保持选中行与滚动位置（不整表替换 / 不重置分页）。验证：同测试文件断言刷新前后 `selectedArticle` 与 `page` 不变
- [x] 4.5 复核 `useRefreshPolling.ts` 的 `fetchFeeds({ per_page: 10000 })`（响应仅 14.6 KB，非本 change 痛点）保持不动并在 design/文档留痕说明。验证：`grep -n 'per_page: 10000' front/app/features/feeds/composables/useRefreshPolling.ts` 输出与 `evidence/consumers.md` 记录一致

## 5. 测试

- [x] 5.1 影响包全绿：`cd backend-go && go test ./internal/reader/...`，期望全 PASS
- [x] 5.2 前端受影响测试全绿：`cd front && pnpm test:unit app/features/articles app/features/feeds app/stores --maxWorkers=2`，期望全 PASS
- [x] 5.3 `test-cases.md` 主链路 9 个节拍逐行勾对落点测试通过证据（命令 + 结果），期望无缺漏落点、无未处理划除项

## 6. 文档

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=仅新增 models.Article.ToListDict 序列化方法（窄投影用），无表结构/迁移/AutoMigrate 变更，docs/reference/database/ 无需更新 -->

- [x] 6.1 `docs/reference/api/articles.md`：`/api/articles` 列表响应字段清单更新（窄投影 + `excerpt` + `per_page` 上限语义），并注明 `/api/articles/:id` 仍返回全部正文类字段。验证：`grep -rn 'firecrawl_content' docs/reference/api/` 命中的列表接口章节已标注「详情接口专属」
- [x] 6.2 `docs/reference/flow/reading.md`：代码入口/数据流补「列表窄投影 + 导语来源优先级（详情 description → excerpt 过渡）」，业务约束节补「列表 MUST NOT 携带正文」红线句，变更溯源表补本 change 行。验证：`grep -n 'excerpt' docs/reference/flow/reading.md` 有命中
- [x] 6.3 `docs/reference/flow/content-enrichment.md`：注明正文/`description` 的消费边界（仅详情接口消费），溯源行补本 change。验证：`grep -n 'slim-article-list-payload' docs/reference/flow/reading.md docs/reference/flow/content-enrichment.md` 命中两处
- [x] 6.4 `bash scripts/harness/doc-impact.sh verify`，期望对账通过（无未声明域、无缺失溯源）

## 7. 验证

- [x] `cd backend-go && golangci-lint run ./...`，期望 0 issue
- [x] `cd backend-go && go vet ./... && go build ./...`，期望退出码 0
- [x] `cd backend-go && go test ./internal/reader/...`，期望全 PASS
- [x] `cd front && pnpm lint && pnpm exec nuxi typecheck`，期望 0 error
- [x] `cd front && pnpm test:unit app/features/articles app/features/feeds --maxWorkers=2`，期望全 PASS
- [x] `curl -s --noproxy '*' -o /tmp/list.json -w '%{size_download}\n' 'http://127.0.0.1:5100/api/articles?per_page=20'`，期望输出 < 102400；且 `grep -o '"content"' /tmp/list.json | wc -l` 期望 0；`grep -o '"excerpt"' /tmp/list.json | wc -l` 期望 20
- [x] `curl -s --noproxy '*' -o /dev/null 'http://127.0.0.1:5100/api/articles?per_page=10000'` 后 `grep -c 'per_page=10000' backend-go/logs/app.log`，期望 ≥1（超限 WARN 留痕）
- [x] `grep -n 'fetchArticles({ per_page: 10000' front/app/features/feeds/composables/useAutoRefresh.ts`，期望无输出
- [x] 人工：浏览器打开静态托管页面（`:5100`）点选一篇文章，期望正文与导语正常显示、列表行渲染与刷新前一致（opencli/agent-browser 断言 + 截图存 `evidence/`）

### Scenario → 测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
|---|---|
| 列表响应体积上限 | backend-go/internal/reader/handler/article_handler_test.go |
| 各筛选分支字段集一致 | backend-go/internal/reader/handler/article_handler_test.go |
| 大字段按需到详情取 | backend-go/internal/reader/handler/article_handler_test.go |
| 长 HTML 源被截断 | backend-go/internal/reader/handler/article_handler_test.go |
| 空源返回空串 | backend-go/internal/reader/handler/article_handler_test.go |
| 与正文重复的导语不下发 | backend-go/internal/reader/handler/article_handler_test.go, front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 正文兜底导语不下发 | backend-go/internal/reader/handler/article_handler_test.go |
| 客户端请求 10000 条 | backend-go/internal/reader/handler/article_handler_test.go |
| 合法值不产生告警 | backend-go/internal/reader/handler/article_handler_test.go |
| 单 feed 刷新完成 | front/app/features/feeds/composables/useAutoRefresh.test.ts |
| 不发起全量重拉 | front/app/features/feeds/composables/useAutoRefresh.test.ts |
| 多 feed 同周期 | front/app/features/feeds/composables/useAutoRefresh.test.ts |
| 刷新过程中用户切换视图 | front/app/features/feeds/composables/useAutoRefresh.test.ts |
| 刷新失败不终止调度 | front/app/features/feeds/composables/useAutoRefresh.test.ts |
| 有实质内容 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 无实质内容 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 详情未就绪时的首帧导语 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
| 详情返回后以详情为准 | front/app/features/articles/components/ArticleContentPreviewPanel.test.ts |
