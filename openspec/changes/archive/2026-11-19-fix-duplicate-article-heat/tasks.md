# tasks — fix-duplicate-article-heat

> design 决策见 `design.md`（D1–D7）；spec 断言见 `specs/` 三个 delta。

## 1. A · 关键字物化轨日期穿透

- [x] 1.1 `MaterializeKeywordWatches` 增加 `date time.Time` 入参并透传给 `scanBoardDayArticles`；删除 `scanBoardDayArticles` 的 `date.IsZero() → time.Now()` 兜底，零值改为返回明确错误（`backend-go/internal/topicgraph/service/watch_materialize_keyword.go`）。验证：`cd backend-go && go build ./internal/topicgraph/...` 通过
- [x] 1.2 生产调用点 `daily_report_orchestrator.go` Step 7.5 传入报告 `date`（与 sentence 轨同源）。验证：`cd backend-go && go vet ./internal/topicgraph/service/` 通过，且 `grep -n "MaterializeKeywordWatches(ctx" backend-go/internal/topicgraph/service/daily_report_orchestrator.go` 行内含 `date`
- [x] 1.3 更新 `watch_materialize_integration_test.go` 全部 7 处调用点（补传日期；实际行号 121/245/255/286/403/441/513），并新增「补档日 ≠ 执行日只取报告日文章」用例与「零值日期返回错误」用例。验证：`cd backend-go && go test -run 'TestMaterializeKeyword' ./internal/topicgraph/service/` 全绿

## 2. B · 参与者内容身份（规范化链接）

- [x] 2.1 实现 `NormalizeArticleLink(link string) string`（TrimSpace → 斩 `#` 后段 → 斩 `?` 后段 → scheme/host 小写、path 原样；空输入返回空串）（`backend-go/internal/platform/textutil/normalize.go`）。验证：`cd backend-go && go test ./internal/platform/textutil/` 全绿，覆盖空串 / 纯空白 / 纯 fragment / query 差异 / 大小写 / 超长 URL
- [x] 2.2 迁移新增 `story_signals.content_key text`（可空）+ `(story_id, content_key)` 索引，可重入、不改历史行（`backend-go/internal/platform/database/postgres_migrations.go`）。验证：`cd backend-go && go test ./internal/platform/database/` 全绿；`docker exec syntopica-postgres psql -U postgres -d syntopica -c "\d story_signals"` 出现 `content_key` 列与索引
- [x] 2.3 `RecomputeSignalsForStory` 写入 signal 时同源产出 `content_key`（复用 2.1 函数，`participant_key` 保留）（`backend-go/internal/topicgraph/repository/story_signals_repository.go`）。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run TestStorySignals` 全绿，用例断言同 link 两副本的 `content_key` 相等
- [x] 2.4 参与者去重键统一为 `COALESCE(NULLIF(content_key,''), participant_key)`：`story_heat.go`（`loadBoardHeatEvidence` + `compute`）、`story_signals_repository.go`（`recomputeStoryParticipantsWith`）、`report_story_highlights.go`、`board_radar_stories.go`、`lane_snapshot_repository.go` 五处消费面同源改造。验证：`cd backend-go && go test ./internal/topicgraph/repository/ ./internal/topicgraph/service/` 全绿；新增用例断言同 link 两副本时 `participant_count=1`、`story_heat` 为单参与者值
  > 2026-11-19 补注（review R-7）：`board_radar_stories.go` 已随 `retire-story-adopt-event-metrics` 删除；`report_story_highlights.go` / `lane_snapshot_repository.go` 现也不再消费 participant 键。现行内容身份消费面只剩 `story_heat.go` + `story_signals_repository.go`（只读保留）与 `report_event_clusters.go`（当天聚簇簇内篇数，复用 `heatParticipantKey`）。
- [x] 2.5 前端零改动确认：`grep -rn "participants" front/app/features/tags/components/console/` 显示消费字段未变（仅数值语义变化）。验证：`cd front && pnpm exec nuxi typecheck` 通过

## 3. 存量重算（不重建报告正文）

- [x] 3.1 `cmd/backfill-stories` 增加 `content_key` 回填（按 `articles.link` 规范化，分批、幂等）并与 `--rebuild-signals` 串联重算 participant_count（`backend-go/cmd/backfill-stories/main.go`）。验证：`cd backend-go && go test ./cmd/backfill-stories/` 全绿
- [x] 3.2 真库只读核对（不写库）：事件线 5204 的 `participant_count` 由 2 回落为 1；全库 `count(DISTINCT participant_key) > count(DISTINCT content_key)` 的事件线归零。验证：执行 `tasks.md` §7 的核对 SQL（**判据已修正，见 6.5**——原 SQL 会把已正确去重也算入分母）
- [x] 3.3 确认无报告正文被改写：核对前后 `daily_report_threads` / `daily_report_sections` 行数与 `related_article_ids` 校验和不变。验证：§7 的只读计数 SQL 前后相等

## 4. 审查

- [x] 4.1 派 `change-review` 子线程聚焦审查（制品 + 说明变更 + 代码现状），意见按 High/Medium/Low 处置后 `review.md` 建账（9 条：R-1/R-2 High、R-3/R-4/R-5/R-9 Medium、R-6/R-7/R-8 Low）。处置见 `review.md` 状态行
- [x] 4.2 归档顺序约束：本 change SHALL 先于 `retire-story-adopt-event-metrics` 归档（后者 REMOVE「参与者身份与去重」Requirement，反序会使本 MODIFIED 找不到目标而硬失败）

## 5. 测试

- [x] 4.1 单测：`NormalizeArticleLink` 全变体 + 零值日期报错（`backend-go/internal/platform/textutil/normalize_test.go`、`backend-go/internal/topicgraph/service/watch_materialize_keyword_test.go`）
- [x] 4.2 仓测：同 link 双副本只计 1 参与者、不同 link 各计 1、空 link 退化按 feed（`backend-go/internal/topicgraph/repository/story_heat_test.go`、`story_signals_test.go`）
- [x] 4.3 集成：补档日期穿透（`backend-go/internal/topicgraph/service/watch_materialize_integration_test.go`）
- [x] 4.4 回填幂等：重跑 `--rebuild-signals` 两次结果一致（实现在 `backend-go/cmd/backfill-stories/rebuild_signals_test.go:TestRebuildStorySignalsRangeDryRunAndIdempotence` 与 `backfill_content_keys_test.go`；`main_test.go` 只覆盖 `TestParseOptions*`）
- [x] 4.5 用例文档：`test-cases.md` 主链路 / 继承与调整 / 变体走查 / 白盒附加四节齐备并与本 tasks 的验证节一致

## 6. 文档

<!-- doc-impact: daily-report, semantic-board, database -->
<!-- doc-impact-excuse: configuration=backend-go/configs/config.yaml 的 PG 连接池改动属并行 change fix-radar-overview-false-zero（T1.2，其 edit.map 归属被会话绑定串味），本 change 不动 configuration 语义 -->

- [x] 5.1 `docs/reference/flow/daily-report.md`：热度证据节补「参与者身份 = 内容身份优先、feed 兜底」与规范化链接口径；Step7.5 节补「物化扫描窗口 = `period_date`，补档同口径」；变更溯源表加一行
- [x] 5.2 `docs/reference/flow/semantic-board.md`：雷达「N 个来源」与矩阵颜色分档的计数口径指向内容身份去重
- [x] 5.3 `docs/reference/database/tables/`（含 `story_signals` 的表文档）：补 `content_key` 列与索引说明
- [x] 5.4 运行 `bash scripts/harness/doc-impact.sh verify --change fix-duplicate-article-heat` 对账通过

## 7. 验证

- [x] 6.1 `cd backend-go && go build ./...` → 退出码 0
- [x] 6.2 `cd backend-go && golangci-lint run ./internal/topicgraph/... ./internal/platform/textutil/... ./internal/platform/database/... ./cmd/backfill-stories/...` → 无新增告警（退出码 0）
- [x] 6.3 `cd backend-go && go test -short ./internal/topicgraph/... ./internal/platform/textutil/... ./internal/platform/database/... ./cmd/backfill-stories/...` → 全绿
- [x] 6.4 只读核对 1：`docker exec syntopica-postgres psql -U postgres -d syntopica -c "SELECT id, participant_count FROM board_stories WHERE id = 5204;"` → `participant_count = 1`
- [x] 6.5 只读核对 2（判据修正：原 SQL 把「已正确去重」也计入分母，高位是有意的）：真库核对 `SELECT count(*) FILTER (WHERE content_key IS NULL OR content_key='') FROM story_signals;` → `0`（1386 行全有内容身份键）；`SELECT count(*) FROM board_stories st WHERE st.participant_count <> (SELECT count(DISTINCT COALESCE(NULLIF(s.content_key,''), s.participant_key)) FROM story_signals s WHERE s.story_id=st.id);` → `0`（participant_count 与去重键计数全库一致）。仍是同 link 双副本的 5204 / 5259 / 5319 三条被原 SQL 命中，`participant_count` 已回落为 1
- [x] 6.6 样本核对（补档不再跨天）：**改用真库只读核对，不造测试日期日报**（红线：验证含副作用的 SQL 不碰真库）。同一条 SQL 把 `created_at > now() - interval '1 hour'` 换成 `period_date >= DATE '2026-11-10'`（修复落地日之后，含 11-16 22:00 那批补档的 11-13/11-14/11-15 报告）→ **0 行**；另核「同一篇文章出现在多天日报」`count(*)=0`（11-16 起）。唯一 1 条 `pub_date <> period_date` 的 l2_llm 线索是文章行自身 `pub_date` 事后被 feed 修正（article 176894：入库 11-16、pub_date 现为 11-18），与物化轨日期穿透无关。
- [x] 6.7 `openspec validate fix-duplicate-article-heat --strict` → 通过
