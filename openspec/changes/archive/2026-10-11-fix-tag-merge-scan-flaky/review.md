# Review — fix-tag-merge-scan-flaky

### R-1 2026-10-11【Low】自审：防御分支与测试重写的正确性
- **状态**：已修复（审毕确认，无需改动）

自审范围：`tag_merge_suggest.go` 防御分支、`tag_merge_suggest_test.go` 两个测试的重写/新增、test-cases.md 主链路。

- **防御分支不改变生产行为**：`repository.Repo` 在服务正常启动路径恒非 nil（`InitRepository` 在 wire 阶段调用），nil 分支只在单测进程/启动顺序异常时触发；触发时发 `Status:"error"` 进度后 return，与既有 embedding-unavailable 分支语义一致，progress channel buffer 32 不阻塞。
- **测试状态恢复无泄漏**：两个测试均持锁置位、defer 持锁恢复（running=false、progress=nil），同包顺序执行下不影响 `TestIsScanRunning_Initially` 等其它测试；不再有任何真 goroutine 被启动（flaky 根因消除，非概率缓解）。
- **`TestRunFullScan_NilRepo_NoPanic` 竞态面**：runFullScan 的 defer `close(progress)` 与测试 select 消费无竞态（channel 收发/关闭语义安全）；5s 超时兜底防挂死；`<-done` 确证 runFullScan 正常返回未 panic。
- **取舍声明复核**（design D2）：「首次启动返回 true」路径退出单测面——该路径必然真起 goroutine（DB+embedding+全表扫描），单测强行覆盖即重新引入本 bug，取舍成立。

- 审毕无发现（审查范围：上述两文件 diff + design.md 决策表 + test-cases.md 变体走查）
