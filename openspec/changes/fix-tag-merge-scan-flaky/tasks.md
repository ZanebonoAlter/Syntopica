# Tasks — fix-tag-merge-scan-flaky

<!-- doc-impact: none -->

（纯测试稳定性修复 + 内部防御分支 + 巡检台账销账，不改任何业务行为/API/数据模型/配置，无 docs/reference 文档需同步）

- [x] T1 产品侧防御：`backend-go/internal/tagmanagement/service/merge/tag_merge_suggest.go` 的 `runFullScan` 在 `repository.Repo.DB()` 调用前加 nil 检查——Repo 为 nil 时 `scanState.progress <- ScanProgress{Status: "error", Error: "..."}` 后 return
  - 验收：`cd backend-go && go build ./...` 成功；`go vet ./internal/tagmanagement/...` 无告警
- [x] T2 重写 `TestStartFullScan_RejectsConcurrent`：持锁置 `scanState.running=true` + progress channel 模拟扫描进行中，断言 `StartFullScan()` 返回 false，defer 持锁恢复状态；不再真启动 goroutine
  - 验收：`cd backend-go && go test -short -count=1 ./internal/tagmanagement/service/merge/ -run TestStartFullScan -v` 显示单测通过且无 goroutine panic 输出
- [x] T3 新增 `TestRunFullScan_NilRepo_NoPanic`：同包直调 `runFullScan`，断言 progress channel 收到 `Status:"error"` 后关闭、进程不崩溃（覆盖 T1 防御分支）
  - 验收：`cd backend-go && go test -short -count=1 ./internal/tagmanagement/service/merge/ -run TestRunFullScan -v` 通过
- [x] T4 稳定性验证：连跑 10 次证明 flaky 根治 + 全包回归 + lint
  - 验收：`cd backend-go && go test -short -count=10 ./internal/tagmanagement/service/merge/` 全绿（0.006s）；`go test -short -count=1 ./internal/tagmanagement/...` 全绿；`golangci-lint run ./internal/tagmanagement/...` 0 issues
- [x] T5 台账销账（附带，非代码）：
  - resolve 8 条迁移测试欠账（TestBoardSignalMigrationIdempotent / LegacyRowsKeepNullColumns / ShapeAndFKRejectIllegalRows / TestBoardLevelAnalysisScopeMigration 各两轮登记），`--by fix-tag-merge-scan-flaky`
  - waive 1 条 `TestInvestigateBoardQuestion_PersistsFullInvestigation`（testcontainer 集成测试，-short 设计性 SKIP，误登记）
  - resolve 1 条 `tagmanagement/service/merge.TestStartFullScan_RejectsConcurrent`（本次修复，context=patrol-20261011）
  - 验收：`bash scripts/harness/test-patrol.sh --report` 显示 open 0 / fixed 170 / waived 1
