## 1. 数据层与统计口径

- [x] 1.1 新建迁移：`board_health_snapshots` 表（字段见 design D2）+ GORM 模型（tagmanagement/models），验证：迁移在 testcontainer PG 跑通（`go test ./internal/tagmanagement/models -run TestMigration` 期望 PASS，或仓库既有迁移测试模式）
- [x] 1.2 新建 `internal/tagmanagement/service/boardhealth/` 包：版块级统计（matched_tags / single_aux_tag_ratio / reason_dist / ghost_matches / vector_missing），口径唯一实现，验证：函数单测覆盖各指标计算（`go test ./internal/tagmanagement/service/boardhealth -run TestBoardMetrics` 期望 PASS）
- [x] 1.3 标签级纯净样本归因：只取单 aux tag 匹配行、内存 cosine 重算（`CosineSimilarity`）、分布统计（mean/median/p25/p75/low_sim_ratio）、样本不足标 insufficient，验证：单测覆盖归因/跳过多 aux/向量缺失计数三路径（期望 PASS）

## 2. 画像 diff 与 LLM 评分

- [x] 2.1 事实画像 diff：uncaptured_candidates 聚合查询（三表 join、排除已挂载与 disabled、top 50），验证：testcontainer PG repository 测试（禁 SQLite）覆盖候选聚合与排除规则（期望 PASS）
- [x] 2.2 LLM 综合评分：合并 prompt（score/comment/notify 单次产出，design D5）+ 坏 JSON/超时降级（score=NULL 不重试），验证：单测 mock LLM 覆盖正常/降级两路（期望 PASS）
- [x] 2.3 异常通知：score 低于阈值且 notify=true 时落通知（LinkType=board）+ 冷却查询（notification 表近一条），验证：单测覆盖通知/静默/冷却三路（期望 PASS）

## 3. 轮次编排与挂载

- [x] 3.1 体检轮次编排 `RunCheckup`：遍历活跃版块 → 统计 → LLM → 快照落库 → 通知；单版块失败记日志继续兄弟版块，验证：单测覆盖轮次顺序与单版块失败隔离（期望 PASS）
- [x] 3.2 `DailyReportJob` 成功路径尾步接入：日报成功裁决后同步调 RunCheckup，体检 error/panic 仅日志不影响日报 job 返回值；手动路径（TriggerNowWithDate）同触发，验证：单测覆盖成功触发/体检失败不污染日报结果（期望 PASS）
- [x] 3.3 配置项 4 个落 ai_settings（design D8，均可缺省带默认值），验证：单测覆盖缺省回退（期望 PASS）

## 4. 只读端点

- [x] 4.1 `GET /api/semantic-boards/:id/health-checkup?trend=N`（N 白名单 1-90 默认 7）：latest+trend、404/空态分支，验证：handler 测试覆盖三态（期望 PASS）

## 5. 扩充召回接入

- [x] 5.1 `recallExpandAuxCandidates` 前置 member-derived 主路：最新快照候选 top N（排除已挂载保险滤）+ days>0 近期文章过滤 + 无快照/空清单回退双路，验证：单测覆盖主路/回退/days 过滤三路（期望 PASS）
- [x] 5.2 board-upgrade-expand 既有测试资产处置：跑 `bash scripts/harness/test-assets.sh board-upgrade-expand` 反查旧资产，按 test-cases.md「继承与调整」表逐行处置（改断言/保留），验证：受影响测试文件全部跑绿（期望 PASS）

## 6. 前端（形态 A：体检治理面板吸收构成管理，BoardCompositionPanel 退役）

- [x] 6.1 types（BoardHealthCheckup DTO）+ api client 方法，验证：`pnpm exec nuxi typecheck` 期望无错误
- [x] 6.2 `BoardHealthPanel.vue`：状态矩阵七态（loading/empty/error/success/insufficient/警告条/操作失败）+ 评分色档 + 评语 + 指标 chips + 质量表（行内移除，confirm 文案不变；**折叠式长列表：默认 top 10 + 展开全部**）+ 组合标签区（平移含创建入口，超长同样折叠）+ 趋势 + 未收编候选（收编快捷动作 + toast，默认 top 5 展开）；**双主题硬约束：仅 Layer 2 语义 token，AppButton/AppInput/AppSectionHeader 统一组件**，验证：`pnpm test:unit BoardHealthPanel --maxWorkers=2` 期望 PASS；`grep -E -- '--raw-|#rrggba?\(' BoardHealthPanel.vue` 期望零命中（无原始色/硬编码色）
- [x] 6.3 picker 复用 `AuxiliaryLabelPicker`（搜索/分页/双模式：版块推荐 + 手动指定均保留）与 CompositeCreateDialog 挂载迁入，验证：组件测试覆盖添加全流程含手动指定路径（期望 PASS）
- [x] 6.4 TagsPage 集成：治理面板替换 BoardCompositionPanel（退役删除）、LaneDynamicsPanel 迁为直接渲染、低分警告条，验证：`pnpm test:unit TagsPage --maxWorkers=2` 期望 PASS；全局 grep 确认无残留引用（`grep -rn BoardCompositionPanel app/` 期望仅测试文件或零命中）
- [x] 6.5 无快照降级：徽章「等待首轮体检」+ 质量列「待体检」+ 构成管理照常可用，验证：组件测试无快照分支（期望 PASS）

## 7. 测试整合

- [x] 7.1 后端影响包全绿：`cd backend-go && go test ./internal/tagmanagement/... ./internal/admin/scheduler/...` 期望 PASS（范围判定 `bash scripts/harness/change-scope.sh`）
- [x] 7.2 前端影响面全绿：`cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit --maxWorkers=2` 期望全 PASS（归档前停其它 pi 会话）
- [x] 7.3 test-cases.md 故事走查：主链路各步落点齐、变体走查留痕，验证：`openspec validate add-board-health-checkup` 期望 valid

## 8. 文档

<!-- doc-impact: flow, api, database, architecture -->

- [x] 8.1 `docs/reference/flow/semantic-board.md`：新增「版块体检」节（链路/指标口径/自动化边界）+ 红线句新增（体检纯读不落建议 / member-derived 主路回退语义 / 快照口径唯一实现），验证：`bash scripts/harness/doc-impact.sh verify` 期望对账通过
- [x] 8.2 `docs/reference/flow/daily-report.md`：日报 job 成功路径尾步挂体检的交叉引用一行，验证：grep 「体检」有命中
- [x] 8.3 `docs/reference/architecture/map.md`：业务域→代码入口索引补 boardhealth 包入口，验证：grep boardhealth 有命中

## 9. 验证

- [x] 9.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/... ./internal/admin/scheduler/...` 期望零 issue
- [x] 9.2 `cd backend-go && go vet ./internal/tagmanagement/... ./internal/admin/scheduler/...` 期望无报告
- [x] 9.3 `cd backend-go && go build ./...` 期望成功
- [x] 9.4 `cd front && pnpm build` 期望成功
- [x] 9.5 端到端人工验证：手动触发日报生成 → 体检快照产生 → `curl --noproxy '*' http://localhost:5100/api/semantic-boards/<id>/health-checkup` 返回 latest 非空 + trend 数组 → 板块内容 tab 底部见体检卡，验证：人工按上述步骤执行（记录于会话）
- [x] 9.6 域外红已记账（归档验证补跑 `go test ./internal/platform/database/` 发现）：4 个 board_signal/board_level_analysis 迁移测试在全包跑时被**其他活跃 change `add-searxng-health-indicator` 的新测试文件** `annotation_qas_web_search_status_migration_test.go` 污染（其 import topicgraph 注册更多模型，改变整个测试二进制的 AutoMigrate 面，暴露 `prepareBoardSignalMigration` 清理列表漏删 `fk_board_signal_research_progress_candidate` 的既有缺口）；已实验定因（移走该文件全包绿）并经 `test-patrol.sh --register` 登记台账（4 条，context=add-board-health-checkup），修复责任归 searxng change
- [x] 9.7 归档前浏览器端到端验收（ui-design Acceptance「opencli 主链路断言」落地；工具 agent-browser/CDP headless chromium，对静态托管 ：5100 实测，2026-10-09）：
  - 主链路断言（1440×900）：/tags 选「开发者工具与开源生态」→ 板块内容 tab → `board-health-panel` 渲染 + 评分徽章 58（红档）+ LLM 评语 + 指标 chips（挂载 tags 439/幽灵匹配 31/向量缺失 0）+ 质量表默认折 10 行「展开全部 (96)」点击后 96 行 + 组合区 + 双列区（趋势/候选）+ 候选 5 行含「收编」按钮（如 OpenAI 81 tags · 97 篇）→ **低分警告条真实渲染**（`board-health-alert-bar`，58<60）
  - 双视口：1440×900 与 1920×1080 两档均面板全量渲染无折行破损（截图 /tmp/bhc-1440x900.png、/tmp/bhc-1920x1080.png）；1920 档断言 panel/score/96 rows/twoCol/alertBar/laneDynamics 全真
  - 与 ui-design Acceptance 差异说明④一致：指标 chips 无「单 aux 占比」条（口径修正退役）

### Scenario → 测试资产映射

| Scenario | 测试文件 |
| --- | --- |
| 构成标签贴合度分布全覆盖 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 样本不足显示数据不足 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 低分线按板块内相对分位 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| cosine 重算进入分布 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 规则分数不作为相似度统计输入 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 向量缺失跳过且计数 | backend-go/internal/tagmanagement/service/boardhealth/metrics_test.go |
| 高频未收编标签进入候选清单 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| 已挂载构成标签不进候选 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 体检轮次不落扩充建议 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 未收编候选可一键收编 | front/app/features/tags/components/BoardHealthPanel.test.ts |
| LLM 产出评分与评语 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| LLM 失败降级不丢统计 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 每轮落一条快照 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| 历史快照不被覆盖 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| 查询趋势序列 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go backend-go/internal/tagmanagement/handler/board_health_handler_test.go |
| 异常且裁决值得则通知 | backend-go/internal/tagmanagement/service/boardhealth/notify_test.go |
| 裁决不值得则静默 | backend-go/internal/tagmanagement/service/boardhealth/notify_test.go |
| 冷却期内不重复通知 | backend-go/internal/tagmanagement/service/boardhealth/notify_test.go |
| 体检在日报后自动执行 | backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 体检不改匹配数据 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |
| 扩充建议仍需手动触发 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go backend-go/internal/admin/scheduler/job_daily_report_test.go |
| 查询最新快照 | backend-go/internal/tagmanagement/handler/board_health_handler_test.go |
| 无快照返回空态 | backend-go/internal/tagmanagement/handler/board_health_handler_test.go |
| 版块不存在 404 | backend-go/internal/tagmanagement/handler/board_health_handler_test.go |
| member-derived 候选被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 无体检快照回退全库召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 相似标签被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 共现标签被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 已挂载标签不召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 无关标签不被召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 时间窗收紧 member-derived 召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 时间窗收紧相似路召回 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 时间窗收紧共现窗口 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 组合路候选相关性过滤 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 召回为空 | backend-go/internal/tagmanagement/service/board/semantic_board_expand_test.go |
| 确认执行保持逐条勾选 | backend-go/internal/tagmanagement/service/board/semantic_board_upgrade_test.go |

说明：人工留痕两项——双主题视觉核验（test-cases 步 14f，9.5 人工步骤内补）与端到端主链路人工验证（任务 9.5：手动触发日报 → 快照 → 端点 → 面板，实测 2026-10-07 21:33 轮达标）；组件层用例（含收编/无快照降级/长列表折叠）见 BoardHealthPanel.test.ts 与 TagsPage.test.ts。

## 10. 口径修正：标签级归因改贴合度分布（2026-10-08 实测数据驱动）

> 背景：旧「单 aux 纯净样本」口径真库实测失效（单 aux 占比仅 2%-11% 且每日新增 ≈ 0，样本池永不增长，板块 3030 仅 6/96 标签有分布）；新贴合度口径实测验证通过（96 标签全覆盖 ×444 样本，排序语义可验证）。决策与数据见 design D4 修正记录。旧测试资产处置见 test-cases.md ⓪ 口径修正表。

- [x] 10.1 后端 `computeLabelAttribution` 重写：构成标签 embedding × 全部有向量挂载 tags identity embedding 内存 cosine 分布；删 `SingleAuxTagRatio` 字段与计算；低分线改板块内 cosine_mean P25 相对线（移除 direction threshold 引用）；构成标签缺 embedding → 该标签 insufficient。验证：`cd backend-go && go test ./internal/tagmanagement/service/boardhealth ./internal/tagmanagement/models` 改口径后 PASS
- [x] 10.2 前端删「单 aux 占比」指标条 + `BoardHealthCheckup DTO` 与 `BoardHealthBoardMetrics` 类型的 `single_aux_tag_ratio` 字段清理。验证：`cd front && pnpm exec nuxi typecheck` 无错误；`pnpm test:unit BoardHealthPanel --maxWorkers=2` PASS（参数不带 `--`）
- [x] 10.3 迁移测试同步口径（`board_health_migration_test` 的 `SingleAuxTagRatio` 字段断言清理）+ 补 `TestLoadCompositionLabelsEmbeddingMapping` 列映射防回归测试（2026-10-07 真库回归：字段缺 column 标签致全标签 insufficient）。验证：随 10.1 一并 PASS
- [x] 10.4 范围验证：`bash scripts/harness/change-scope.sh` 判定影响包后全部跑绿 + `golangci-lint run ./internal/tagmanagement/...` 零 issue
- [x] 10.5 人工重跑一轮体检（手动触发日报）→ 快照 labels 有分布占比 ≥90%（板块 3030 抽查）→ 前端质量表无大面积「待体检/数据不足」。实测（2026-10-07 21:33 轮）：11/12 板块 100% 标签有分布（3030：96/96 × 449 样本，旧口径 6/96 × 17）；唯一例外刚果疫情（挂载量 1 < min=5）全 insufficient 属正确行为；3030 排序与 SQL 预验一致（top DeepSeek Harness 0.730 / bottom kprobe 0.544，低分占比 4%-8% vs 82%-99%）；已部署（deploy-frontend.sh 全栈重启）
- [x] 10.6 口径修正文档同步（归档验证补）：`flow/semantic-board.md`（§版块体检 口径 bullet + 红线 22 改贴合度口径）、`api/semantic-boards.md`（响应示例删 `single_aux_tag_ratio` + 说明改贴合度）、`database/tables/semantic-labels.md`（metrics 字段描述）、`flow/diagrams/board-health-checkup.html`（口径②节点 + 方法学卡片）。验证：`rg "single_aux_tag_ratio|纯净样本" docs/reference/` 期望零命中
