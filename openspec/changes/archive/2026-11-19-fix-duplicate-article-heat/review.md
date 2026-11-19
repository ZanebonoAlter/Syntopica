# review — fix-duplicate-article-heat

> 审查方：`change-review` 子线程（只读，模型 `minimax-cn/MiniMax-M3.1-Flash:max`），审查日期 2026-11-19。
> 审查对象：change 制品 + 已落在 HEAD 的代码现状（本 change 代码随基线提交 `96f83315` 入库，工作树无其 diff）。
> 总判：代码部分（A 日期穿透 / B 内容身份去重）实现与 spec 断言逐条对得上，问题集中在「事件线已硬停写」这个后续背景造成的口径过期。

### R-1 2026-11-19【High】`story-heat-evidence` delta 把已退役的事件线热度写成现行有效行为，与红线 43/49 互斥

- **状态**：已修复
- **证据**：原 delta 正文断言「参与者计数的所有消费面（`story_signals`、`participant_count`、`story_heat`、`lane_heat`、雷达来源数、矩阵颜色分档）SHALL 使用同一份内容身份判定」，Scenario 断言「热度 SHALL 按 0.70 计算」；而 flow 红线 43 已把事件线在线生产硬停写（不再产 `board_stories` / `story_signals` / `story_id`，也不得重算 `participant_count`），红线 49 明确内容身份口径**不再派生** `participant_count` / `story_heat`，现行承接方是 `lane-heat-metric`「线索计数的内容身份去重口径」。
- **处置**：delta 头部加「适用范围（2026-11-19 补注）」段——声明本 Requirement 只描述**历史口径**、只适用于存量只读数据与 `cmd/calibrate-heat --dry-run` 标定，现行承接方指向 `lane-heat-metric`，并写入归档顺序约束（本 change SHALL 先于 `retire-story-adopt-event-metrics`，后者 REMOVE 本 Requirement，反序会使本 MODIFIED 找不到目标而硬失败）。tasks §4 同步登记该顺序约束。

### R-2 2026-11-19【High】存量修复路径已被拒写，design/tasks 承诺的操作入口现在是死的

- **状态**：已修复
- **证据**：`design.md` D5 / Migration Plan 第 2 步与 `proposal.md`「存量修复」都写「复用 `cmd/backfill-stories --rebuild-signals`」；而 `cmd/backfill-stories/main.go` 的 `retiredWriteModes()` 已把 `--rebuild-signals` / `--backfill-content-keys` / `--reattribute` 等写模式列入拒绝集并非零退出（红线 43），只留 `--dry-run`。功能目标本身已达成（真库 1386 行 `story_signals` 全有 `content_key`、`participant_count` 与去重键计数全库一致）。
- **处置**：`proposal.md` 存量修复段、`design.md` D5 与 Migration Plan 第 2 步各补 2026-11-19 补注——写模式现被拒并非零退出（仅 `--dry-run` 可用）、回填已于 2026-11-10 完成并核对、后续不得再重算 `participant_count`。

### R-3 2026-11-19【Medium】`flow/daily-report.md` 内文自相矛盾：本 change 5.1 的产物与红线 49 打架

- **状态**：已修复
- **证据**：`:194` 参与者身份段原写「`participant_count`、来源数、`story_heat` 全部同源于这一口径…存量回填走 `--backfill-content-keys`」，与同文件 `:377` 红线 49「不再派生 `participant_count` / `story_heat`」直接矛盾；`:473` 溯源行也停在退役前口径。（审查方另提的 `:182` 管线图已由该节头部 `:162` 的 ⚠️「本节描述的事件线链路已硬停写」覆盖，无需再改。）
- **处置**：`:194` 段改为「历史口径 + 现行承接方」并补 `#!` 例外与「回填已完成、写模式已拒」；`:473` 溯源行补「story 侧参与者口径随事件线退役，现行由当天聚簇的簇内内容身份篇数承接」。

### R-4 2026-11-19【Medium】`flow/semantic-board.md:27 / :226` 残留退役前的参与者/来源数/颜色分档口径

- **状态**：已驳回（不以本 change 修复，理由与落地改法记录在此）
- **证据**：`:27`「来源数 = 独立参与者数…与 `board_stories.participant_count` 同源」、`:226`「格颜色 = 当日最热事件线的独立参与者数（`max_participant_count`）分档」，与同文件 `:31`「事件线量纲整体退役」及红线 28/29（矩阵与演进必须用当天线索合计、MUST NOT 用事件数加权）矛盾；代码侧 `board_radar_stories.go` 已删除。
- **驳回理由**：这两行的主体口径（事件线 Top 8 清单、矩阵事件线数/颜色分档）属 `retire-story-adopt-event-metrics` 自己的文档同步欠账（该 change 23/23 任务完成但未归档，未清偿这两行），本 change 的 5.2 增量只是其中「按内容身份去重」短语；且该文件当前被并行 change `archive-time-chart-range` 的在途未提交改动占用，本会话改它会把他人脏文件混进本次提交。
- **落地改法（供 retire 归档时的文档同步直接采用）**：`:27` 的「今日该读 = 跨版块 Top 8 事件线 + 来源数 = 独立参与者数」改为「今日该读 = 当天线索最多的泳道清单（`top_lanes`）」并把来源数一句标为历史口径；`:226` 的「格颜色 = 最热事件线独立参与者数分档」改为「矩阵日格 = 当日非综合稿线索合计，颜色通道随事件线量纲退役」。

### R-5 2026-11-19【Medium】`database/tables/daily-report-watch.md` 把已拒写的 CLI 当运维入口

- **状态**：已修复
- **证据**：同一段一边写「本表只读保留、不再有新写入」，一边把 `cmd/backfill-stories --backfill-content-keys` 当「存量回填入口」并宣称 `participant_count` / 来源数 / 热度同源。
- **处置**：该段改写为「去重键口径 + 历史口径标注 + 现行消费面是当天聚簇簇内篇数 + 回填已于 2026-11-10 终结、写模式被拒仅 `--dry-run` 可用」。

### R-6 2026-11-19【Low】`NormalizeArticleLink` 的 `#!` hashbang 例外没写进 spec 的规范化口径

- **状态**：已修复
- **证据**：`textutil/normalize.go` 复用 `StripURLFragment`，对 `#!` 保留 fragment（测试 `normalize_test.go:72` 显式固化），spec 口径只写「去掉 `#` 之后的 fragment」，漏了例外。
- **处置**：spec 规范化口径补「`#!` hashbang 除外，沿用 `StripURLFragment` 既有语义」；`flow/daily-report.md` 同句同步。

### R-7 2026-11-19【Low】tasks.md 落点 / 计数 / 文件指针与现状不符

- **状态**：已修复
- **证据**：2.4 列的 `board_radar_stories.go` 已删；1.3 写「5 处调用点」实为 7 处（121/245/255/286/403/441/513）；4.4 指向 `main_test.go`（实为 `rebuild_signals_test.go` / `backfill_content_keys_test.go`）；test-cases 写 1281 行而真库现值 1386 行、且引用已删的 `board_radar_stories_test.go`。
- **处置**：tasks 1.3 / 2.4（附退役落点说明）/ 4.4 与 test-cases 三处一并订正；1386 以真库 `SELECT count(*) FROM story_signals` 为准。

### R-8 2026-11-19【Low】design D5 说「复用 `--rebuild-signals`」，实现是新增独立开关 `--backfill-content-keys`

- **状态**：已修复
- **证据**：`backfill_content_keys.go:54` 明写与 `--rebuild-signals` 正交；`main.go:150` 新增独立 flag。
- **处置**：design D5 与 proposal 存量修复段补上独立开关与正交关系（落点加 `backfill_content_keys.go`）。

### R-9 2026-11-19【Medium】归档门禁两项红：review 痕迹零建账 + complex 档 tasks 缺 Review 行

- **状态**：已修复
- **证据**：`archive-readiness.sh` ③④ 红（本文件即 ③ 的产物，④ 需 tasks 含 review/审查任务行）。
- **处置**：本 `review.md` 建账；`tasks.md` 新增 `## 4. 审查`（4.1 审查与处置、4.2 归档顺序约束），后续节号顺延（测试 5 / 文档 6 / 验证 7），正文 `§6` 交叉引用同步改为 `§7`。

---

## 审查范围

- **制品**：本 change 的 `proposal.md` / `design.md` / `tasks.md` / `test-cases.md` / `ui-design.md` / 三个 spec delta；主 spec `story-heat-evidence`、`watch-materialized-topic`、`daily-report-system`；参照 `retire-story-adopt-event-metrics`（proposal/design/tasks/specs）；`docs/reference/flow/daily-report.md`、`flow/semantic-board.md`、`database/tables/daily-report-watch.md`、`standard/shared/doc-authoring.md`、`开发执行规范.md` §11/§12。
- **代码**：`service/watch_materialize_keyword.go`、`service/daily_report_orchestrator.go`（Step 7.5 调用点）、`service/watch_materialize_integration_test.go`、`service/watch_materialize_keyword_test.go`、`platform/textutil/normalize.go(+_test)`、`platform/database/postgres_migrations.go`（content_key 迁移）、`repository/story_signals_repository.go`、`story_heat.go`、`story_models.go`、`report_story_highlights.go`、`report_event_clusters.go`、`lane_snapshot_repository.go`、`cmd/backfill-stories/*`。
- **只读命令**：`git log/show/status/diff`、`openspec validate --strict`、`review-trace.sh`、`check-tasks-tail.sh`、`archive-readiness.sh`、`doc-impact.sh verify`、`go build`、`go test ./internal/platform/textutil/`、多次 `grep -rn` 消费面盘点。
- **未跑**：全量 `go test ./...`、前端全量、任何写库 SQL、任何 dev 服务、浏览器。
