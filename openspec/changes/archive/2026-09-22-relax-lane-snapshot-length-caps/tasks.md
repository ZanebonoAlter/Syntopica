# Tasks: relax-lane-snapshot-length-caps

<!-- ui-impact: none —— 纯后端生成参数放宽，前端展示层无任何变更 -->

## 1. 后端实现

- [x] 1.1 `backend-go/internal/topicgraph/service/lane_snapshot.go`：`laneSnapshotMaxRunes` 100→200、`laneSnapshotDetailMaxRunes` 500→1000、`laneSnapshotMaxTokens` 768→1536；常量注释同步。
- [x] 1.2 同文件 `laneSnapshotSystemPrompt()`：字数要求文案 ≤100字→≤200字、≤500字→≤1000字。

## 2. 测试更新

- [x] 2.1 `lane_snapshot_test.go`：pin `laneSnapshotMaxTokens=768` 的断言改为 1536（SN-9）；pin 100/500 截断上限的用例改用常量或新数值。

## 3. 注释一致性

- [x] 3.1 `backend-go/internal/topicgraph/repository/daily_report_models.go`：`RollingSummary`/`RollingDetail` 字段注释 ≤100字/≤500字 → ≤200字/≤1000字。
- [x] 3.2 `front/app/features/tags/components/daily-report/LaneTrendOverview.vue`：grep 无 100/500 字 pin，无需改动。

## 4. 测试

- [ ] 只跑影响包：`go test ./internal/topicgraph/service -run 'LaneSnapshot' -short`（lanes 结算相关全部通过）。

## 5. 文档

<!-- doc-impact: topic-graph, daily-report -->

- [x] `docs/reference/flow/daily-report.md` §泳道态势结算正文与业务约束 #22 的 100/500/768 → 200/1000/1536；§变更溯源表归档时补行（历史行不改写）。`flow/topic-graph.md` 无字数 pin。

## 6. 验证

- [x] `cd backend-go && go test ./internal/topicgraph/service -run 'LaneSnapshot' -short` → 全部 PASS
- [x] `cd backend-go && golangci-lint run ./internal/topicgraph/...` → 本 change 三文件 0 issue（剩余 1 条 gocritic 在他人会话新文件 margin_notes_repository.go，非本 change 归属）
- [x] `cd backend-go && go vet ./internal/topicgraph/...` → 无输出
- [x] `cd backend-go && go build ./...` → 无输出

### Scenario 映射（delta: board-lane-dynamics「日报后滚动态势结算」）

以下用例均在 backend-go/internal/topicgraph/service/lane_snapshot_test.go：结算触发/窗口锚定（WindowAnchorAndUpsertIdempotent）、两版单次同素材（TwoVersionJSONUpsert、TwoVersionContractAndMaxTokens）、机械截断（ClampBothVersionsIndependent、DetailExactBoundary、NonJSONDegrades）、失败不阻塞（LLMFailureContinuesSiblings、LLMErrorKeepsOldSnapshotContinuesSiblings、PanicDoesNotAffectReport）、存量兼容（LegacyRowDetailHeals）。

| Scenario | 测试文件 |
| --- | --- |
| 日报完成后结算 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 两版同窗同素材单次生成 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 机械截断保护 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 结算失败不阻塞 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 存量快照兼容 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
| 态势句与时间线同窗 | backend-go/internal/topicgraph/service/lane_snapshot_test.go |
- [ ] `bash scripts/harness/doc-impact.sh verify relax-lane-snapshot-length-caps` → 对账通过
