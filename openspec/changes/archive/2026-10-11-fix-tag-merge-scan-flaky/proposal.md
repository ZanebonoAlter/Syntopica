<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

2026-10-11 测试巡检（patrol-20261011）跑出 be-tagmanagement 分片红：`internal/tagmanagement/service/merge` 包测试进程整体崩溃。根因是 `TestStartFullScan_RejectsConcurrent` 调用了真实的 `StartFullScan()`，起的后台 goroutine `runFullScan` 在测试断言结束后仍存活，走到 `tag_merge_suggest.go:110` 的 `repository.Repo.DB()`——单测进程里全局 `repository.Repo` 是 nil 指针，直接 panic 崩掉整个测试二进制（09-24 巡检该分片 total_fails=1 与本次同源，flaky：goroutine 与测试退出赛跑）。该命令也是 quality-gate 高频路径（retro 7 天窗口 `go test ./internal/tagmanagement/...` 失败 40 条，Top 2），不修会持续污染门禁失败统计。

同轮巡检还确认台账 9 条历史 open 欠账均为存量误挂：8 条 BoardSignal/BoardLevelAnalysisScope 迁移测试现已全绿（当时环境/代码问题已修复但未销账，且被两个 change 重复登记），1 条 `TestInvestigateBoardQuestion_PersistsFullInvestigation` 是需要 testcontainer Postgres 的集成测试（`-short` 模式设计如此 SKIP，属误登记）。

## What Changes

- **修 flaky（主修复）**：`TestStartFullScan_RejectsConcurrent` 不再真启动扫描 goroutine——同包测试直接持锁把 `scanState.running` 置 true 模拟「扫描进行中」态，只测互斥拒绝路径（第二次 `StartFullScan()` 返回 false），测试结束持锁恢复状态。真启动路径依赖 DB + embedding，单测不覆盖（注释本就写明 "can't easily start a real scan in unit tests"），由生产环境与集成路径覆盖。
- **产品侧防御**：`runFullScan` 在访问 `repository.Repo` 前加 nil 检查——Repo 未初始化时向 progress channel 发 error 并 return，而不是 nil 指针解引用 panic 崩掉宿主进程（生产启动顺序异常或未来其它测试路径的兜底）。
- **台账销账（附带，非代码）**：`test-patrol.sh --resolve` 销 8 条已转绿的迁移测试欠账；`--waive` 销 1 条设计性 SKIP 的集成测试欠账；flaky 修复验证后 resolve 今天新登记的 1 条。

## Capabilities

### New Capabilities

（无——标签合并建议全量扫描行为契约不变，仅修测试稳定性与防御）

### Modified Capabilities

（无——不修改任何既有 spec 行为；如 spec 中存在全量扫描相关 requirement，本 change 不触碰其行为面）

## Impact

- 代码：`backend-go/internal/tagmanagement/service/merge/tag_merge_suggest.go`（nil 防御一处）、`tag_merge_suggest_test.go`（重写并发拒绝测试）。
- 影响包：`internal/tagmanagement/service/merge`；验证命令 `go test -short -count=1 ./internal/tagmanagement/...`（连跑多次验证不再 flaky）+ `golangci-lint run ./internal/tagmanagement/...`。
- 部署影响：无——不改任何生产行为路径（防御分支正常启动永不触发），无需迁移/配置，用户无感。
