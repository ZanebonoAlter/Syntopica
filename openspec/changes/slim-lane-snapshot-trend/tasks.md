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
- [ ] 5.3 人工效果核对（效果依赖 LLM 行为）：下个日报日结算后抽 ≥5 条真实泳道产物，对照讨论判据（阶段带驱动语义/事件仅转折标记/无逐日罗列/无预测/量级引用准确）记录至 `effect-check.md`；不合格回改 prompt 而非放宽判据。

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
