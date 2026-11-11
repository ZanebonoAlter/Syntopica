# Tasks：description-repair-batch-matching

> 设计依据 design.md D1-D6；用例锚点 specs delta（3 个 MODIFIED Requirement / 15 个 Scenario）；探索事实 explore-findings.md（A 前端链路 / B 主 spec / C 接线与测试 / D 合并依据）。

## 1. 用例先行

- [ ] 1.1 撰写 `test-cases.md`：主链路表串节拍（步/动作/来源 Scenario/期望/层/落点），覆盖 specs delta 全部 15 个 Scenario（含 9 个继承原样、6 个新增：重启后批自动重建继续 / 同批任务合并为一次匹配 / 批内失败不连坐 / 攒批期间新增确认并入或顺延 / 排队任务数可见 / 排队信息缺失静默降级）；变体走查五组逐项给答案或划除留痕；**⓪ 继承与调整表**：跑 `bash scripts/harness/test-assets.sh auxiliary-label-description-repair` 反查旧测试资产，逐行列「旧 Scenario × 处置 × 旧测试 × 动作」；白盒附加节出分支表（executor 两段调度 / heartbeat 批续租 / 批量结算边界 K 不整除、尾批）。验证：文件存在且 `grep -c "Scenario" test-cases.md` ≥ 15。

## 2. 后端 repository 层

- [ ] 2.1 `repository/auxiliary_description_repair_executor.go`：新增 `ClaimMatchingBatch`（单事务 FOR UPDATE SKIP LOCKED 领取全部 matching 态可领 job，上限 BatchMaxJobs）；`ExecutorConfig` 增 `BatchMaxJobs`/`BatchSettleSize` 并入 `Validate()`。验证：`go test ./internal/tagmanagement/repository -run TestClaimMatchingBatch -count=1` 通过（并发双执行器仅一方领到、上限截断、SKIP LOCKED 不阻塞）。
- [ ] 2.2 `repository/auxiliary_description_repair_matching.go`：`ListRepairScopeItems` 扩展跨 job 去重装载（返回 tag 执行集 + 批内 job 归属）；`RecordMatchingItemResult` 改批量结算（每 K 条一事务：批量 UPDATE item + 一次 `aggregateRepairCounters` + 各 job 计数一次更新，尾批 flush）。验证：`go test ./internal/tagmanagement/repository -run TestBatchSettle -count=1` 通过（四类计数之和恒等 total、K 不整除尾批、崩溃后恢复不重结算）。
- [ ] 2.3 `repository/auxiliary_description_repair.go`：新增 `CountUnfinishedRepairJobs(ctx, excludeJobID)`（status NOT IN 终态，排除当前 job）；`service/auxrepair/auxrepair.go:JobView` 增 `queued_jobs` 字段并在查询组装层拼接。验证：`go test ./internal/tagmanagement/service/auxrepair -run TestJobViewQueued -count=1` 通过（含 0 个其它任务时 queued_jobs=0、终态不计入）。

## 3. 后端 service 层（调度与合并执行）

- [ ] 3.1 `service/auxrepair/executor.go`：runJob 拆两段——第一段 claim 未提交 job 只跑 embedding+saving，跑完留 matching 态不就地进行匹配，继续排干 pending；第二段无 pending 可领时 `ClaimMatchingBatch` 成批，heartbeat 遍历批内 job 逐个续租（任一失败取消整个 attempt）；批内 embedding/saving 失败的 job 标显式失败态并剔除，不连坐。验证：`go test ./internal/tagmanagement/service/auxrepair -run TestBatchSchedule -count=1` 通过（两 job 只发生一遍全量匹配的调度形态、失败剔除、批中途 panic 后各 job 留 matching 态可重建）。
- [ ] 3.2 `service/auxrepair/matching.go`：合并匹配——批内 tag 去重执行集，每 tag 恰好一次 `GetScopeTagState` + 一次引擎调用，结果广播写入批内各 job 的 item 行；逐 job `FinishMatchingPhase` 结算。验证：`go test ./internal/tagmanagement/service/auxrepair -run TestMergedMatching -count=1` 通过（stub matcher 计数 = 去重后 tag 数而非各 job 之和；两 job 分别计数恒等各自 total；批内一 job 部分失败另一 job 正常 completed）。

## 4. 前端

- [ ] 4.1 `front/app/api/normalizers/auxiliaryRepair.ts` + `front/app/api/auxiliaryLabels.ts`：`AuxiliaryRepairJobView` 增 `queuedJobs`，normalize 透传，缺失/非有限数字容错为 null。验证：`cd front && pnpm test:unit composables/useAuxiliaryDescriptionRepair.test.ts --maxWorkers=2` 通过（新增透传与容错用例）。
- [ ] 4.2 `front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts:boardHealthProgressText`：matching 阶段且 queued_jobs≥1 时追加「· 另有 M 项排队」，M=0/null/缺失时文案与现状逐字一致。验证：`cd front && pnpm test:unit components/console/board-health/boardHealthWorkspace.model.test.ts --maxWorkers=2` 通过（追加/回归双形态断言）。
- [ ] 4.3 既有展示层回归：`BoardHealthWorkspace.repair.test.ts` / `BoardHealthWorkspace.test.ts` 不改断言全绿（确认文案追加不破坏既有形态）。验证：`cd front && pnpm test:unit components/console/board-health/ --maxWorkers=2` 通过。

## 5. Review

- [ ] 5.1 派 change-review 子线程聚焦 review（审查制品 specs/design/test-cases 与实现 diff），意见按 High/Medium/Low 处置后在 `review.md` 建账。验证：review.md 存在且每条意见有处置行。

## 6. 测试

- [ ] 6.1 后端影响包门禁：`bash scripts/harness/change-scope.sh` 机械判定后，对影响包跑 `go test <影响包> -count=1` 全 PASS、`go vet ./...` 0、`golangci-lint run <影响包>` 0 issues、`go build ./...` 退出 0；树莓派上不顺手跑全量。验证：命令与退出码记录在本节证据行。
- [ ] 6.2 前端门禁：`cd front && pnpm lint` 0 错误、`pnpm exec nuxi typecheck` 0 error、受影响单测文件（4.1-4.3 三文件）带 `--maxWorkers=2` 全 PASS。验证：命令与退出码记录在本节证据行。

## 7. 文档

<!-- doc-impact: semantic-board -->

- [ ] 7.1 `docs/reference/flow/semantic-board.md`：修复执行链小节更新——两段式攒批调度、按 tag 去重广播、一致快照语义、批内失败不连坐、批为执行期概念崩溃自动重建；业务约束节红线句同步（合并匹配各 job 分别结算、计数恒等不变量不变）。验证：`bash scripts/harness/doc-impact.sh context` 对应段核对一致。
- [ ] 7.2 `docs/reference/api/semantic-boards.md`：GET `description-repairs/:job_id` 响应字段表补 `queued_jobs`（int，同库未结束任务数不含当前；语义与前端展示规则）。验证：字段名与实现 JSON tag 逐字一致（grep 双端核对）。
- [ ] 7.3 补主 spec `openspec/specs/auxiliary-label-description-repair/spec.md` 顶部 Purpose（triage 归档遗留 TBD 占位，一句话概括能力）。验证：`grep -c "TBD" openspec/specs/auxiliary-label-description-repair/spec.md` = 0。

## 8. 验证

- [ ] 8.1 `bash scripts/dev/deploy-frontend.sh` 成功（构建/铺盘/重启/健康检查退出 0）；后端随 `start-dev.sh back --restart` 载入新调度。验证：脚本末尾健康检查通过。
- [ ] 8.2 agent-browser 真实页面核对（静态 :5100，落 `ui-verification/report.md` 建账）：连续确认两项描述修复 → 先行任务 matching 进度文案出现「· 另有 1 项排队」→ 批完成后文案回到现状形态；拍照留 1440×900 一档即可（minor UI 无新布局）。验证：report.md 有真实端点/落态依据与截图。
- [ ] 8.3 `bash scripts/harness/doc-impact.sh verify openspec/changes/description-repair-batch-matching && bash scripts/harness/check-standards.sh` 通过；`bash scripts/harness/test-patrol.sh --report` 无本 change 未还欠账。验证：命令退出码 0 或欠账已登记非本 change。
- [ ] 8.4 完工汇报节：部署后影响（同批确认总匹配耗时约除以任务数、进度文案新增排队段）/ 需要用户操作（无——零迁移零清理）/ 旧数据降级（无——job 状态语义不变，在跑任务由新调度自然接管）。验证：汇报含三要素。
