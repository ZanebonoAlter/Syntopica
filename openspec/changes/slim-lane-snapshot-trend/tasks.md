# Tasks: slim-lane-snapshot-trend

## 1. 用例先行

- [x] 1.1 编写 `test-cases.md`：主链路表串 specs 两个 delta 的全部 Scenario（结算触发/量级注入/定位约束/截断/空守卫/失败不阻塞/存量兼容/同窗 + 14 天档渲染三态）；⓪ 契约变更表逐行处置旧测试资产（`bash scripts/harness/test-assets.sh board-lane-dynamics` 与 `lane-trend-overview` 反查：lane_snapshot_test.go 双版断言、LaneTrendOverview.test.ts 长版/回退用例）；变体走查（解析输入：空串/纯空白全角/超长截断 rune 边界；量级窗口：as_of 当日计入/无报告日补零/7 天窗跨期/单日多 section 聚合）；效果核对节（真实泳道结算产物 vs 演进叙事判据的人工抽查方案）。

## 2. 后端结算改造

- [x] 2.1 `repository/lane_snapshot_repository.go` 新增 `ListDailyThreadCounts(topicID, sinceDate)`：按 `period_date` 聚合该泳道锚定 section 的当日 thread 数与 section 数；testcontainer PG 用例（单日多 section 求和、窗口边界日计入、无数据空集）。
- [x] 2.2 `service/lane_snapshot.go` `buildLaneSnapshotUserPrompt` 追加量级信号段：每日线索量（缺日补零）+ 近 7 天 / 上个 7 天合计，窗口锚定 as_of（`MAX(period_date)`），并注明「无报告日不计入趋势判断」。
- [x] 2.3 重写 `laneSnapshotSystemPrompt`：演进叙事定位（阶段/驱动转折/落点/量级佐证）+ 显式禁止逐日罗列与未来预测 + 纯文本输出要求（无 JSON/围栏）。
- [x] 2.4 重写 `parseLaneSnapshotOutput`：trim → 空/纯空白 error → `truncateRunes(200)`；`rolling_detail` 固定写空串；删除 JSON 解析与 degraded 分支；MaxTokens 1536 → 500。

## 3. 前端收敛

- [x] 3.1 `LaneTrendOverview.vue`：`summaryText` 直取 `summary`；删 `detailMissing` 与「长版随下次日报结算生成」提示；`data-testid` 统一 `trend-summary`；快照缺失占位/月年档/逐日事件/tab 不动。

## 4. Review

- [x] 4.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账。

## 5. 测试

- [x] 5.1 重写 `backend-go/internal/topicgraph/service/lane_snapshot_test.go`：纯文本解析守卫（空/空白/超长 rune 截断）、prompt 构造含量级信号段与禁止条款、结算写入 detail 置空、失败不阻塞路径；按 test-cases.md「继承与调整」表逐项处置旧双版用例。
- [x] 5.2 改造 `front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts`：单版渲染全文、存量 detail 非空不渲染不提示、快照缺失占位保留；删长版回退用例。
- [x] 5.3 人工效果核对（效果依赖 LLM 行为）：下个日报日结算后抽 ≥5 条真实泳道产物，对照讨论判据（阶段带驱动语义/事件仅转折标记/无逐日罗列/无预测/量级引用准确）记录至 `effect-check.md`；不合格回改 prompt 而非放宽判据。**2026-10-28 首轮已跑（数据源 10-25/26/27 三个日报日的 38 条新结算产物）**：内容判据全过、量级抽查与真库计数一致；但 38/38 顶到 200 字上限、37/38 从字中间切断（读者看到半截话）→ 判不合格，处置见 5.4，复验见 5.5，本项保持未勾。**2026-10-31 第二轮已跑（数据源 10-29/10-30 两个日报日的 37 条新结算产物，见 effect-check.md §4）**：形态判据全过（句末收尾 97.3% ≥ 90%、最长 200 字、超 200 字 0 条）；量级引用判据**不通过**——复验发现近7/上7合计与打印标签错位一天（R-12），处置见 5.6，复验见 5.7，本项保持未勾
- [x] 5.4 按 5.3 不合格结论改「形态不达标」而非放宽判据：①`laneSnapshotSystemPrompt` 加篇幅硬约束（总长含标点 ≤200 字、必须以句号收尾、写不下压缩/合并阶段）；②解析端 `clampLaneSnapshotRunes` 改句末收口（窗口内最后一个 `。！？…`，标点过早 <60% 预算或无标点则退回 rune 硬切）；delta spec「机械截断保护」Scenario 同步。验证：`go test ./internal/topicgraph/service -run 'LaneSnapshot'` 全绿（新增 `TestParseLaneSnapshotOutput_ClampsAtSentenceBoundary`，含「190 字正文+句号+尾巴→191 收口」「句号过早→退回硬切」「无标点→硬切」三支）
- [ ] 5.5 下个日报日（2026-10-29）结算后重跑 `effect-check.md` §3 的只读 SQL：期望「以句末标点收尾」≥ 90% 且字数仍 ≤200；合格后勾 5.3 并归档本 change（复验记录追加至 `effect-check.md`）。**2026-10-31 已跑**（数据源 10-29/10-30 两个日报日 37 条新结算产物，见 effect-check.md §4.1）：句末收尾 36/37 = 97.3% ≥ 90% ✓、最长 200 字 ✓、超 200 字 0 条 ✓ → 形态判据合格；量级引用判据不合格（错窗，R-12）→ 处置见 5.6、复验见 5.7，**本项保持未勾**（待 5.7 闭环后与 5.3 一并勾选）

- [x] 5.6 按 5.3 复验发现的量级窗口错位改「口径不达标」而非放宽判据（review R-12）：`renderLaneMagnitudeSignal` 的近7/上7 rollup 比较从 **instant** 改为**日历日槽位**（与上方每日行同一套 `byDay` 日期串），使「标签区间 = 计入区间」由构造保证；根因＝`anchor` 归一化到正午 UTC 而真库 PG `date` 列扫出午夜 UTC，窗首日 `00:00 < 12:00` 被判进前窗（标称近7天实算 6 天、上7天整体错位一天）；补 PG 形态（midnight 行）回归用例 `TestRenderLaneMagnitudeSignal_PGDateColumnMidnightShape` 锁死窗首/窗末日归属（原用例夹具用 noon 时点、与真库形态不一致才漏网）。验证：`go test ./internal/topicgraph/... -count=1` 全绿；真库复验见 effect-check.md §4.2（修复后 6/6 与独立 SQL 回算一致，修复前 5/6 不一致）

- [ ] 5.7 R-12 修复后的真库复验：下个日报日结算后重跑 effect-check.md §4 的只读核对——期望「量级引用准确」判据转绿（文中引用的近7/上7数字 == 独立 SQL 回算标称窗口），且形态判据不回退（句末收尾 ≥ 90%、≤200 字）；合格后勾 5.3 与 5.5 并归档本 change（复验记录追加至 effect-check.md）

## 6. 文档

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=新增 ListDailyThreadCounts 只读聚合查询，无 schema/迁移/模型列变更，表结构文档无需更新 -->

- [x] 6.1 更新 `docs/reference/flow/daily-report.md` 泳道态势结算节：单版演进叙事产物、量级信号注入、detail 置空与存量自愈、消费方口径；归档后按 §12.2 补 archive 永久链接。人工：链接可打开。
- [x] 6.2 核对 `docs/reference/api/` lane-dynamics 相关文档：`rolling_detail` 字段语义改为恒空（或移除出响应示例），与实现一致。人工：响应字段与文档逐项一致。

## 7. 验证

以下为实现后门禁。新增测试路径须在实现期创建；scenario-trace、doc-impact、review-trace 只在实际实现与文档更新后才应全绿。

- [x] 7.1 `openspec validate slim-lane-snapshot-trend --strict` → 退出码 0。
- [x] 7.2 `cd backend-go && golangci-lint run ./internal/topicgraph/... && go vet ./internal/topicgraph/... && go build ./...` → 零错误（只跑影响包/模块，不跑全量）。
- [x] 7.3 `cd backend-go && go test ./internal/topicgraph/ -run 'LaneSnapshot|DailyThreadCount'` → 全绿。
- [x] 7.4 `cd front && pnpm lint && pnpm exec nuxi typecheck` → 零错误。
- [x] 7.5 `cd front && pnpm test:unit LaneTrendOverview --maxWorkers=2` → 全绿（参数不带 `--`）。
- [x] 7.6 `bash scripts/harness/scenario-trace.sh openspec/changes/slim-lane-snapshot-trend && bash scripts/harness/doc-impact.sh verify openspec/changes/slim-lane-snapshot-trend && bash scripts/harness/check-standards.sh --change slim-lane-snapshot-trend && bash scripts/harness/review-trace.sh openspec/changes/slim-lane-snapshot-trend` → 全部退出码 0。
- [x] 7.7 `bash scripts/harness/test-patrol.sh --report` → 确认无未还欠账；非本 change 红按规范 register 登记。
- [x] 7.8 `bash scripts/dev/deploy-frontend.sh` → 验收通过后构建铺盘重启后端、健康检查 200。

### Scenario → 测试文件映射

| Scenario | 测试文件 |
|---|---|
| 日报完成后结算 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 量级信号随素材注入 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 演进叙事定位约束 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 机械截断保护 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 空输出守卫 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 结算失败不阻塞 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 存量快照兼容 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 态势句与时间线同窗 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 态势句就绪 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 存量长版不消费 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 快照缺失 | front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts |
| 单请求聚合 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 存量长版透出 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 无态势快照降级 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
