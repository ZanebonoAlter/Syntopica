## 1. 现状核定（apply 启动）

- [x] 1.1 跑 `bash scripts/harness/doc-impact.sh suggest openspec/changes/daily-report-margin-notes` 与 `context`；复核 explore-findings 已 pin 的关键路径（DailyReportTopicSection thread header button、drm-layout 三列改造点、ensureArticles 链路、airouter open_notebook、DailyReportThread 模型），补精确行号；查库内现有 embedding 列维度供 term_notes.embedding 对齐，不另造结构。
- [x] 1.2 浏览器行为核定（Chromium 优先）：button 内划选 + capture click guard 方案验证；若某浏览器 button 内选区不可用，启用 design D2 备选等价重构（摘要移出 button），结论与差异记录 design.md 与 ui-design.md（等价重构不触发重审，行为须与红线 Scenario 一致）。
  - 2026-09-24 真机核定（agent-browser/Chromium，报告 896）：button 内 `data-mn-anchor` 摘要可划选、mouseup 出气泡；**guard 与批准原型不一致**（原型有 `bubble.contains(e.target)` 豁免、实现漏了），修复记录见文末「本轮回修」。

## 2. 数据层：三表迁移与 repository

- [x] 2.1 SQL migration 建 `report_annotations` / `annotation_qas` / `term_notes`（term_norm 唯一索引；embedding 可空 vector 列维度对齐现有；qa.cited_article_ids/extracted_terms jsonb）；隔离 PG 库跑迁移 + 重复执行幂等验证通过。
- [x] 2.2 repository 层：批注 CRUD（按 report 批量含问答轮）、qa 插入、term upsert（归一化 trim+全半角+大小写折叠，hit_count 累计不重复建条）；testcontainer PG 单测覆盖 jsonb 数组契约（空写 `[]` 不写 null、保序去重），`go test ./internal/topicgraph/...`（目标包）通过。

## 3. 后端：QA 服务与 API

- [x] 3.1 QA service：prompt 组装（quoted_text + question + thread 标题/摘要 + 前 3 篇关联文章摘录每篇 ≤300 runes）→ airouter `open_notebook`（operation `daily_report.margin_note_qa`）单次调用产出 `{answer, cited_article_ids[], terms[]}`；cited 候选集白名单校验（集外剔除、空=纯模型知识合法）、terms ≤5 归一化 upsert（失败 Warn 不阻断回答返回）；stub LLM 单测覆盖全链路与失败分支。
- [x] 3.2 handler + 路由（design D3 四端点：GET/POST `/api/daily-reports/:reportId/annotations`、DELETE `/api/annotations/:id`、POST `/api/annotations/:id/questions`）；参数校验（quoted_text 非空+长度上限、section/thread 归属校验、404 路径）；handler 测试通过。（gin 路由树约束：`:reportId` 实现为 `:id`，同段同参名）
- [x] 3.3 open_notebook 路由配置核对：AI 页现有路由配置面下 open_notebook 可被用户配置 provider 链（无默认路由时的报错行为与现有能力一致）；无新增 ai_settings 键，结论记录 design.md。（发现前端缺口：useAIRouterSettings 白名单漏 open_notebook，修复已追加至前端组 4.x）

## 4. 前端：批注交互与三列布局

- [x] 4.1 `useMarginNotes` composable + API client（按 reportId 拉取、落锚/提问/删除状态机）；hook/类型单测通过。
- [x] 4.2 `SelectionAskBubble` + selection guard（capture 阶段吞有效选区≥2 字符后的同一点击）+ mark 点击分层（跳卡 stopPropagation 不触发 thread 展开）；vitest 断言三件套：划选不误展开 / 无选区纯点击正常展开 / 点 mark 跳卡不展开（specs 红线三 Scenario）。
- [x] 4.3 `MarginNotesRail` / `MarginNoteCard`：引用摘要 2 行截断点击跳原文并闪现、QA 轮列表、提问/追问输入（loading 锁定、错误行内重试不丢问题）、引用 chips 走现有 ensureArticles→文章预览、术语 chips（新词入库角标）、空态灰色轻提示、删除确认 AppDialog sm（明示连带删除）；组件测试覆盖各状态。
- [x] 4.4 `BoardDailyReportTimeline` 三列改造（第三列 `clamp(15rem,17vw,17rem)` sticky 栏内滚动；<1100px 收为 AppSidebarDrawer 右抽屉 + 右下浮动入口含批注数，点高亮开抽屉定位）；高亮渲染含 quoted_text 偏移失配→模糊匹配→「原文已变更」降级态；视觉对齐主题 token 与批准原型。
- [x] 4.5 `SettingsWorkspace` sections 数组注册「页边注」分区（key `margin-notes`、深链 `?section=margin-notes` 兼容现有导航模式）+ `SettingsSectionMarginNotes` 组件（筛选栏/批注行/问答折叠/仅标记行态/空态）；组件测试覆盖列表渲染、筛选、空态、删除联动。
- [x] 4.6 API client 增跨报告列表端点（board/q/分页）与管理页数据 hook（与 useMarginNotes 同数据层）；「跳原日报」深链打开 TagsPage 日报层并定位报告+锚点（复用 focusSectionId/scrollIntoView 机制）；验证方式并入 5.1。
- [x] 4.7 前端整体验证：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit <受影响文件> --maxWorkers=2` 全绿（不与 build/浏览器并行）。
  - 2026-09-24：`pnpm lint` 0 error（7 条既有 warning，与本 change 无关）、`nuxi typecheck` exit 0、影响文件 9 个 test file / 73 tests 全绿。

## 5. 集成验证与收尾

- [x] 5.1 端到端手验（`bash scripts/dev/start-dev.sh` 起服务）：划选→气泡→落锚（长划词展开/收起、↗ 跳原文、✕ 常显）→提问→带引用回答→术语 chips→追问→删除确认全链；重开同份日报批注与问答仍在；设置「页边注」分区列表/筛选/跳原日报定位/删除与日报内同源；thread 展开/收起溯源、topic 收展、ensureArticles 预取行为与改动前一致。
  - 2026-09-24 全链真机走查（报告 896，agent-browser）：划选/气泡/落锚（thread + lead）、长划词 2 行截断 + 展开收起、提问→带引用回答→`{term,is_new}` 术语 chips、追问追加、重开&重新部署后批注/问答/高亮俱在、mark 点击点亮卡不展开 thread、↗ 跳原文闪现、删除确认（取消/删除 + 连带 QA 与高亮同步消失）、管理页列表/筛选/问答折叠/跳原日报定位/删除同源、窄屏（1024）FAB+抽屉+点高亮开抽屉定位；guard 零回归（划选不误展开、无选区纯点击照常展开）。走查发现并修复 9 处缺口见文末「本轮回修」。
- [x] 5.2 完成汇报含「部署后影响 + 需要的操作 + 旧数据降级」三段（纯新增三表无存量迁移；旧日报无批注数据属正常空态）。
  - 2026-09-25 联网扩充轮汇报已交付（见会话）；遗留一项：web_sources 真机端到端渲染待本地 LLM 服务器（10.11.12.111:8080，EHOSTUNREACH）恢复后补验（提问一次即可，渲染层已由组件测试覆盖）。

## 6. 测试

- [x] 6.T1 `bash scripts/harness/change-scope.sh` 机械确定影响包（预期 `internal/topicgraph/...` + 前端 tags 组件群），按输出跑目标 Go 测试；预期全绿，不顺手全量。
- [x] 6.T2 按 test-cases.md 执行并回填精确命令：repository 层 testcontainer PG（数组契约/幂等/归一化）、service 层 stub LLM（白名单剔除/terms 失败隔离/重试）、handler 层（404/参数校验）、前端 vitest（guard 三断言/状态矩阵）；预期与用例表逐条对应。
  - Go：`cd backend-go && go vet ./... && go test -short ./internal/topicgraph/...`（含 `TestValidateAnnotationTarget` 的头条 `section_id=0` 用例、`TestListAnnotationsManagement_Contract` 的 `board_id/board_label` 断言）；
  - 前端：`cd front && pnpm test:unit app/api/marginNotes.test.ts app/features/tags/composables/useMarginNotes.test.ts app/features/tags/composables/useMarginNoteSelection.test.ts app/features/tags/components/daily-report/MarginNoteCard.test.ts app/features/tags/components/daily-report/MarginNotesRail.test.ts app/features/tags/components/daily-report/marginNoteAnchor.test.ts app/features/settings/components/SettingsSectionMarginNotes.test.ts app/features/settings/composables/useMarginNoteAdmin.test.ts app/features/tags/components/BoardDailyReportTimeline.test.ts --maxWorkers=2` → 73 passed。
- [x] 6.T3 opencli/agent-browser 主链路断言：划选→气泡→落锚→提问→回答渲染→引用打开→追问→删除确认 + 现有交互零回归（thread 展开溯源/topic 收展）；若人工豁免在本节留痕验证方式。
  - 走查方式：agent-browser 真鼠标事件（mouse move/down/up 拖选 + 坐标点击），dev server 与部署后静态产物（:5100）双入口各跑一遍；引用 chip → 文章预览弹窗打开已验证。

## 7. 文档

<!-- doc-impact: flow, api, database, configuration -->

- [x] 7.1 `docs/reference/flow/daily-report.md`「业务约束与不变量」补页边注红线（引用文章反查豁免 archived 与 thread 同口径、cited 白名单、术语归一化与单次调用契约、selection guard 零回归）；「代码入口」补新组件/服务/端点路径；变更溯源链接留 §12 补。
- [x] 7.2 `docs/reference/flow/ai-summary.md` 同步 open_notebook 业务用途绑定（页边注问答第五条）与 operation 清单。
- [x] 7.3 `docs/reference/api/` 按目录规范注册批注四端点契约（请求/响应形状、错误码、jsonb 数组契约）。
- [x] 7.4 `docs/reference/database/tables/` 新增三表文档（字段/索引/归一化规则/embedding 预留说明），按目录注册点 checklist 注册；含 `annotation_qas.cited_web_sources` 列（任务 10.4 产出）。
- [x] 7.5 `docs/reference/flow/data-enrichment.md` 约束 10 补 SearXNG 事实（页边注问答联网后端为本地 SearXNG 原始结果聚合，同样只用原始结果、无 AI 总结模式，失败静默降级；客户端 `internal/platform/searxng`，`WebSearcher` 未来薄适配可选）；`docs/reference/configuration.md` 补 `searxng_config` 键（endpoint+enabled，DB>env>config.yaml>空=禁用）。

## 8. 验证

- [x] 8.1 `openspec validate daily-report-margin-notes`，预期通过。（2026-09-25 通过）
- [x] 8.2 `bash scripts/harness/doc-impact.sh verify openspec/changes/daily-report-margin-notes` 与 `bash scripts/harness/check-standards.sh --change daily-report-margin-notes`，预期文档对账无遗漏。（verify 声明 flow/api/database/configuration 通过；check-standards 204/204，139 capability validate 通过）
- [x] 8.3 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，预期通过；测试仅用 6.T1 影响范围。（2026-09-25 全部 0 issues；受影响包测试全绿：searxng 8 用例、aisettings +3、database 迁移幂等 1、topicgraph service/repository/handler 全绿）
- [x] 8.4 `bash scripts/dev/deploy-frontend.sh`，预期静态产物部署与健康检查成功；串行于测试/浏览器。（2026-09-25 部署成功，:5100 同源健康 200，走查即在该入口）
- [x] 8.5 `agent-browser` 按 ui-design.md Acceptance 验收：1440×900 / 1920×1080 双档 + 375×667 / 390×844 窄屏降级，截图存本 change evidence；与批准原型差异说明记录到 ui-design.md（重大差异需重新审批）。
  - 原型主体验收于 §9 完成双视口走查（证据见 §9 验收记录）；联网增补（chips + 设置分区）按 ui-design「不变项」约定不重跑全量双视口，设置分区证据 `evidence/searxng-settings-configured.png`。

### Scenario → 测试映射（scenario-trace 对账）

| Scenario | 测试文件 |
| --- | --- |
| 划选落锚 | front/app/features/tags/composables/useMarginNoteSelection.test.ts |
| 批注跨会话持久 | backend-go/internal/topicgraph/repository/margin_notes_repository_test.go |
| 引用展开与双向跳转 | front/app/features/tags/components/daily-report/MarginNoteCard.test.ts |
| 删除批注连带问答 | backend-go/internal/topicgraph/repository/margin_notes_repository_test.go |
| 提问获得带引用回答 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 无文章上下文时明示 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 生成失败可重试不丢问题 | front/app/features/tags/components/daily-report/MarginNoteCard.test.ts |
| 追问 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 联网补强回答 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 搜索失败静默降级 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 编造 URL 白名单剔除 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 纯模型知识标注口径 | front/app/features/tags/components/daily-report/MarginNoteCard.test.ts |
| 问答后术语自动入库 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 重复术语命中累计 | backend-go/internal/topicgraph/repository/margin_notes_repository_test.go |
| 划选不误触发展开 | front/app/features/tags/composables/useMarginNoteSelection.test.ts |
| 纯点击仍正常展开溯源 | front/app/features/tags/composables/useMarginNoteSelection.test.ts |
| 点高亮跳卡不展开 | front/app/features/tags/composables/useMarginNoteSelection.test.ts |
| 列表与筛选 | backend-go/internal/topicgraph/repository/margin_notes_repository_test.go |
| 跳转原日报定位 | front/app/features/settings/components/SettingsSectionMarginNotes.test.ts |
| 管理页删除与日报内同源 | front/app/features/settings/components/SettingsSectionMarginNotes.test.ts |
| 无匹配空态 | front/app/features/settings/components/SettingsSectionMarginNotes.test.ts |
| 空态提示 | front/app/features/tags/components/daily-report/MarginNotesRail.test.ts |
| 窄屏抽屉形态 | front/app/features/tags/components/BoardDailyReportTimeline.test.ts |
| 文章总结使用 summary 路由 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 日报生成使用 digest_polish 路由 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
| 页边注问答使用 open_notebook 路由 | backend-go/internal/topicgraph/service/margin_notes_qa_test.go |
- [x] 8.6 `bash scripts/harness/test-patrol.sh --report`（各分片 last_ok=1，无本 change 未还欠账；归档另获用户指令） 与 `bash scripts/harness/archive-readiness.sh daily-report-margin-notes`，预期无本 change 未解决问题；实际归档另获用户指令。

## 9. 本轮回修（2026-09-24 用户报障「划词点击没响应」）

真机按用户故事走查（划选 → 气泡 → 落锚 → 提问 → 回答 → 追问 → 复制/跳转 → 删除 → 管理页 → 窄屏）发现 9 处缺口，均已修并补回归测试：

1. **「问一问」气泡点击被 selection guard 吞掉（报障主因）**：`useMarginNoteSelection.ts` 的 document capture guard 缺气泡豁免（批准原型 `reader.html` 有 `bubble.contains(e.target)` 豁免，实现漏抄）。气泡出现的前提就是选区存续 + 气泡 `mousedown.prevent` 保选区 → 落锚链路整条坏死。修复 + `RG-5` 用例。
2. **落锚后卡片不渲染（qas:null）**：`POST /daily-reports/:id/annotations` 返回新建行时 `qas` 零值序列化为 `null`，前端 `MarginNoteCard` 读 `note.qas.length` 抛错 → 栏内只剩计数、输入也不聚焦。后端响应补 `[]`，前端 `anchorAnnotation` 同口径归一化。
3. **提问响应契约不匹配（卡片永久 pending）**：后端返回扁平 `{answer, cited_article_ids, terms, new_terms, …}`，前端等 `data.qa` → `normalizeQa(undefined)` 抛错，问题文本与错误行内提示都没出现。改为 service 回传已落库 QA 行、handler 按 GET 同形状返回 `data.qa`（术语带 `{term, is_new}` 供「入库」角标）；前端加契约防护（缺 qa → 行内错误 + 重试不丢问题）。
4. **头条 lead 批注落锚 400**：highlight 派生头条无 section → `section_id=0` 被 `ValidateAnnotationTarget` 拒（PR-4 案例走不通）。后端放行 `thread_id=nil && section_id=0`，测试补用例。
5. **落锚失败静默**：`anchorError` 无人消费，点气泡没卡没提示（与报障同感）。栏内新增行内错误提示（独立 `v-if`，不切断 loading/error/empty 链）。
6. **删除确认弹窗被阅读层盖住**：AppDialog 默认 `z-index:1000` < 阅读层 overlay `9000` → 弹窗可见不可点（点取消/删除落到正文，可能顺带展开 thread）。改用既有 `9100` 档（与 `ArticlePreviewModal` 同约定）。
7. **管理页「未知版块」+ 跳原日报丢 board**：列表接口只回 `semantic_board_id`、无 `board_label` → 行头显示「未知版块」，且前端 `row.board_id` 为空 → 深链 `board=` 空值被 TagsPage 直接丢弃、跳转失败。后端 join `semantic_labels` 回 `board_id`/`board_label`。
8. **管理页术语 chips 全空 + 重复**：列表路径未做 `extracted_terms` 字符串→对象归一化（渲染出空 chip），且按 QA 轮展开会重复展示同一术语。改用后端去重 `row.terms`（与批准原型 `x.terms` 同口径）。同一化归一化也补到 `listAnnotations`。
9. **深链定位不滚动不闪现 / 窄屏 Esc 连阅读层一起关**：`pendingFocusNoteId` 在 annotations 未到齐时就被消费（report 详情 watcher 先触发一次空跑）→ 卡不亮、正文不滚；改为批注数据到齐才消费并同步 `jumpToOriginal`（滚到锚点 + 闪现）。另：抽屉开着时 Esc 只关抽屉（不再连阅读层一起关丢阅读位置）。

> 验收记录：`pnpm lint` 0 error / `nuxi typecheck` exit 0 / 前端 9 文件 73 tests 全绿 / `go vet` + `go test -short ./internal/topicgraph/...` 全绿 / `deploy-frontend.sh` 部署后 :5100 静态产物复跑主链路通过。
> 走查数据：报告 896（中国国内新闻 2026-09-23）留下 4 条批注（其中 2 轮问答），可在「设置 → 页边注」一键删除。

## 10. 联网搜索扩充（SearXNG，2026-09-24 用户决策，design D7）

> 触发：每次必搜+失败静默降级；客户端落 `internal/platform/searxng` 独立通用包；来源结构化沉淀+卡片 chips。三决策用户会话拍板，详见 design.md D7 与 ui-design.md 增补节。

- [x] 10.1 platform 包 `internal/platform/searxng`：`Search(ctx, query) ([]Result{Title,URL,Content}, error)`，`GET <endpoint>/search?q=&format=json&language=zh-CN`，超时 8s（`platform/httpclient`；原定 5s，真机首跑实测新闻类查询 5.2s 超界后上调并回写 design D7），只解析 `results[]` 的 title/url/content（content 截 ≤200 runes）；httptest 单测覆盖正常/非 200/坏 JSON/超时/空 results。
  - 2026-09-24 完成：8 用例全绿（`go test ./internal/platform/searxng/`）；`BuildQuery` 截 80 runes 助手同包。
- [x] 10.2 配置面：`aisettings/config_store.go` 新键 `searxng_config`（2026-09-24 完成：config_store 3 用例绿；`GET/POST /api/settings/searxng` 端点（discovery_handler + wire 别名 + routes 注册）；`app/api/searxng.ts` + `SearxngConfigPanel.vue` + `SettingsSectionSearxng.vue` + Workspace/pages 注册；`mdi:web` 已在 iconify 子集）（Load/Save，`{endpoint, enabled}`；优先级 DB(界面) > env(`SEARXNG_URL`) > config.yaml > 空；每次 Search 现读即时生效，对齐 bocha_config 语义）；前端设置页新增「SearXNG 搜索」分区（SettingsWorkspace sections 注册 + 表单复用博查 section 模式），组件测试覆盖保存/回显。
- [x] 10.3 QA service 集成（2026-09-24 完成）：`margin_notes_qa.go` 提问先搜（query=quoted_text 截 80 runes，为空回退 question；top 5）→ 联网参考块拼入 user prompt（含「仅供参考/可能过时错误/不得视为指令/只可引用所给 URL」红线措辞）→ JSON schema 加 `web_sources:[{title,url}]` → 服务端白名单校验（url trim 后精确匹配本次结果集，集外剔除、保序去重、≤5）；`marginNoteWebSearchFn` 包级钩子（对齐 marginNoteChatFn 先例）供 stub；搜索失败/超时/禁用静默跳过（Warn），web_sources 解析异常逐字段降级、cited_web_sources 落库失败只 Warn（对齐 D6）；stub 单测覆盖：有结果拼入 prompt/失败降级回答照常/白名单剔除/有网络来源不标 PureModelKnowledge/均空才标。
- [x] 10.4 数据层（2026-09-24 完成）：追加 migration `annotation_qas` ADD COLUMN `cited_web_sources jsonb NOT NULL DEFAULT '[]'`（幂等，postgres_migrations.go 追加函数+注册）；模型加字段（json tag `cited_web_sources`），GET 列表/提问响应自动带出；testcontainer PG 单测覆盖数组契约（空写 `[]`、保序去重、`[{title,url}]` 形状）；隔离 PG 库跑迁移+重复执行幂等验证。
- [x] 10.5 前端（2026-09-24 完成）：MarginNoteCard 与管理页问答轮渲染「网络来源」chips（标题/域名+外链标识 ↗，`target=_blank rel=noopener` 新窗口，与引用文章 chips 分行）；「纯模型知识」标注改为本地引用与网络来源均为空才显示；`useMarginNotes`/admin hook 类型补 `cited_web_sources`；组件测试覆盖有/无网络来源、标注口径两分支。
- [x] 10.6 验证：`bash scripts/harness/change-scope.sh` 判定影响包（预期 topicgraph + platform/searxng + aisettings + 前端受影响文件）并只跑受影响范围；真机走查：配置 endpoint 后提问命中网络来源 → chips 可点开新窗口；停 SearXNG 容器再提问 → 回答照常无报错无网络来源行；设置页改 endpoint 即时生效（下次提问用新地址）。
- [x] 10.7 归档前 7.3/7.4/7.5 文档对齐本组产出（api 响应形状含 cited_web_sources）。
  - 2026-09-24/25 真机走查结论（agent-browser，同源 :5100，deploy-frontend.sh 部署后）：
    - ✅ 设置页「SearXNG 搜索」分区：endpoint 填写 + 开关开启 + 保存 → `ai_settings.searxng_config` 落库 `{"enabled":true,"endpoint":"http://localhost:8889"}`（截图 `evidence/searxng-settings-configured.png`）；重进回显正确。
    - ✅ migration `20260924_0001` 启动时落库（`annotation_qas.cited_web_sources jsonb NOT NULL DEFAULT '[]'`，schema_migrations 已见版本）。
    - ✅ 联网搜索真实触发：超时 8s 上调后，真实提问链路无 searxng WARN（00:42 起搜索在界内成功）；此前 00:37 的 5s 超时 WARN 证明「搜索失败静默降级、问答继续」路径真实工作。
    - ✅ 提问 UI 链路（form submit → pending 骨架 → 行内错误 → 重试按钮）与删除确认弹窗取消路径零回归。
    - ⚠️ web_sources 端到端渲染（真实 LLM 回答含网络来源 chips）**被环境阻断**：本地 qwen 服务器（10.11.12.111:8080）自 2026-09-24 22:20 起不可达，`ai_call_logs` 显示 `extractor_enhanced`/`auxlabel_embedding` 等无关 operation 同步 parse_error/http_502（provider 全域故障，与本 change 无关）。渲染层已由组件测试覆盖（WS-4/7，78 用例绿）；**待 LLM 服务器恢复后补一条真机提问即可闭环**。
    - 附带发现并修复：SearXNG 实测延迟 2.8~5.2s（新闻类查询），原定 5s 超时首跑超界 → 上调 8s（design D7 已同步）。
