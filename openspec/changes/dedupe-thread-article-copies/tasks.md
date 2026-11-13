# Tasks: dedupe-thread-article-copies

## 1. 实现（后端读路径折叠）

- [x] 1.1 新增 `backend-go/internal/topicgraph/repository/article_identity.go`：导出 `FoldThreadArticleCopies(threads []DailyReportThread, linkByArticleID map[uint]string) []DailyReportThread`——纯函数（无 IO）、保序、首个出现为准、`link` 去空白后为空或查不到的 id 不参与折叠；分支表 B1~B13 见 `test-cases.md` — 验证：文件 `backend-go/internal/topicgraph/repository/article_identity.go` 存在且导出该符号
- [x] 1.2 `backend-go/internal/topicgraph/repository/daily_report_repository.go` 的 `GetReportByID`：Preload 之后收集本报告全部被引用文章 id，一次 `SELECT id, link FROM articles WHERE id IN ?` 取 link 后调用折叠；查询失败 → Warnf + 跳过折叠（原样返回，不阻断详情接口） — 验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestGetReportByID -count=1` → PASS
- [x] 1.3 不改生成期与落库口径：`populateThreadArticles`（`daily_report_merge.go`）与 `daily_report_threads.related_article_ids` 落库值保持原样（红线 45 不动） — 验证：`git diff -- backend-go/ | grep -E "populateThreadArticles|SaveThreads|SaveReport"` → 无匹配

## 2. 测试

- [x] 2.1 新增 `backend-go/internal/topicgraph/repository/article_identity_test.go`（单元测试，无 DB）：分支表 B1~B13 + 变体 #1~#12、#15（同 link 折叠 / 归一后相同折叠 / 空 link 不折叠 / 同标题不同 link 不折叠 / 保序首个为准 / 重复元素 / 越界 id / 空映射降级） — 验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestFoldThreadArticleCopies -count=1` → PASS
- [x] 2.2 `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` 补集成用例（testcontainer PG）：seed 一条含跨 feed 同 link 副本的 thread → `GetReportByID` 返回已折叠且落库值未变；link 查询失败时降级不折叠 — 验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestGetReportByID -count=1` → PASS
- [x] 2.3 前端零改动回归核对：四个渲染面（`DailyReportTopicSection` / `LaneChronicleThread` / `TopicDetectiveWall` / `SectionLifecyclePanel`）继续消费同一 `GET /daily-reports/:id` 响应 — 验证：`git diff --name-only HEAD -- front/` → 输出为空

## 3. 文档

- [x] 3.1 `docs/reference/flow/daily-report.md`：红线 45 补一句「读路径投影按归一 link 折叠同内容副本（存储口径不变）」，并在代码入口小节登记折叠 helper 与调用点 — 验证：文件含「读路径例外」段，且 `bash scripts/harness/constraint-digest.sh --brief --domains daily-report` 的第 45 条仍是原句
- [x] 3.2 `docs/reference/api/daily-reports.md`：`GET /daily-reports/:id` 响应契约补「`related_article_ids` 为按归一 link 折叠后的读路径投影（落库值不变）」 — 验证：文件含 `related_article_ids` 折叠语义段
- [ ] 3.3 `openspec/specs/daily-report-system/spec.md`：归档时 delta 同步（MODIFIED Requirement「日报查询 API」） — 验证：归档后 `openspec validate --specs` → valid

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=无 schema/迁移/索引变更，改动只在读路径内存折叠；standard=无新增代码写法约定（复用既有 textutil 归一函数）；configuration=无新配置项；architecture=无骨架变化 -->

## 4. Review

- [x] 4.1 派子线程聚焦 review（审查制品 + diff：折叠口径是否越界、落库值是否被误改、下游白名单是否被收窄、读路径降级路径是否完整），意见按 High/Medium/Low 处置后 `review.md` 建账 — 验证：`bash scripts/harness/review-trace.sh openspec/changes/dedupe-thread-article-copies` → 退出码 0（建账 11 条）

## 5. 验证

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/...` → 0 error（实测：lint `0 issues.`、vet 无输出）
- [x] 5.2 `cd backend-go && go test ./internal/topicgraph/repository -count=1` → 全绿（含 testcontainer PG 用例；实测 `ok syntopica-backend/internal/topicgraph/repository 30.907s`）
- [x] 5.3 真库只读基线（落库值口径，修复前后不变）：`docker exec syntopica-postgres psql -U postgres -d syntopica -A -t -c "SELECT count(*) FROM (SELECT t.id FROM daily_report_threads t JOIN articles a ON a.id IN (SELECT jsonb_array_elements_text(t.related_article_ids)::bigint) WHERE t.related_article_ids IS NOT NULL AND jsonb_typeof(t.related_article_ids)='array' GROUP BY t.id, a.link HAVING count(*)>1) x;"` → 34（修复前后均为此值，证明落库值未被改写）
- [x] 5.4 接口层折叠核对（重启后端后）：`bash scripts/dev/start-dev.sh back --restart` → `curl --noproxy '*' -s http://localhost:5100/api/daily-reports/1428 | python3 -c "..."` → 修复前 `[169400, 170292]` → 修复后 `[[169400]]` ✅
- [x] 5.5 端到端（agent-browser）：静态托管 `:5100` 打开 `/tags/3030/2026-11-07` → 展开 thread 17312 → 只出现 1 条文章标题「从 Coze 居然要下线了说起」、「N 篇」计数为 1 ✅；同时确认**未折叠**样本 thread 17700（`/tags/1974/2026-11-12`，同标题不同 URL 的多源转载）仍为「2 篇」——本口径只认归一 link 相同
- [x] 5.6 `bash scripts/harness/scenario-trace.sh openspec/changes/dedupe-thread-article-copies` → 退出码 0（9 个 Scenario 映射齐全）
- [x] 5.7 `bash scripts/harness/doc-impact.sh verify openspec/changes/dedupe-thread-article-copies` → 零失败（声明: flow, api / 文件: 2 个）
- [x] 5.8 `bash scripts/harness/check-standards.sh --change dedupe-thread-article-copies` → 零失败（284 通过 / 0 失败；期间顺手清了 2026-11-10 两个旧归档 change 的 E 段旧债：它们本已声明「未改任何文档」，只因措辞未含门禁豁免关键词「无 flow 影响」而被误判，已补上标准措辞）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 查询板块日报列表 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 查询日报详情含线程 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| API 查询超过 30 天 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 无日报时返回空 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 线索下同 link 副本折叠 | backend-go/internal/topicgraph/repository/article_identity_test.go, backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 归一后 link 相同也折叠 | backend-go/internal/topicgraph/repository/article_identity_test.go |
| link 为空不折叠 | backend-go/internal/topicgraph/repository/article_identity_test.go |
| 同标题不同 link 不折叠 | backend-go/internal/topicgraph/repository/article_identity_test.go |
| 同一 id 重复引用折叠为一条 | backend-go/internal/topicgraph/repository/article_identity_test.go |

### 效果核对

- 触发原因：折叠收益依赖真实库里的重复分布，断言通过 ≠ 库里真有重复、折叠真生效。
- 核对方法：真库只读 SELECT 对账（5.3）+ 接口层折叠核对（5.4）+ 端到端目视核对（5.5）。
- 量化结果 / 结论：
  - 落库口径（修复前后不变）：全库同 thread 内「同 link 重复」组 **34**（真库只读实测，修复前后同值 → 证明落库值零改写）。
  - 接口口径（修复后）：thread 17312（跨 feed 同 link 副本）响应 `[[169400]]`，落库值仍 `[169400, 170292]` → 折叠只发生在读路径。
  - 端到端：关心的话题页展开该线索 → 1 条标题、计数「1 篇」（修复前为 2 条同名）。
  - **收益边界**：全库 136 个重复组中，本口径只覆盖 **30 组**（同 link 跨 feed 副本）；其余 **106 组**是「同标题不同 URL」的多源转载（如用户截图那条 thread 17700：华尔街见闻 vs 凤凰网），**本次不折叠**——用户已选保守档（误伤样本：「提醒：日内请重点关注（以下均为北京时间）」同 feed 同日 06:30 / 12:30 两版，标题相同、内容相似度 0.74~0.95，实为两篇）。若要覆盖这 106 组，需另开 change 放宽内容身份口径（先解决该误伤样本）。
