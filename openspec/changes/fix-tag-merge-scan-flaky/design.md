# Design — fix-tag-merge-scan-flaky

simple 档，仅记关键决策与取舍，无替代方案长文。

## 根因链（实测确认）

1. `TestStartFullScan_RejectsConcurrent` 调真 `StartFullScan()` → 起 goroutine `runFullScan(ctx)`
2. goroutine 内 `core.GetEmbeddingService()` 是**懒初始化**（`tagger.go:24-26`，nil 则 `NewEmbeddingService()`），首次调用永远返回非 nil → 不会走「embedding unavailable」安全分支
3. 继续执行 `repository.Repo.DB()`（`tag_merge_suggest.go:110`）——单测进程全局 `var Repo *TagManagementRepository`（`repository.go:9`）为 nil 指针，方法内 `return r.db` 解引用 nil → **panic，崩掉整个测试二进制**
4. 测试本身几毫秒结束；goroutine 起步与测试退出赛跑 → 赛输则安然无恙（09-24 巡检侥幸全绿记录 total_fails=1 说明也炸过一次），赛赢则崩（本次复现）→ flaky

## 决策

| # | 决策 | 理由 |
| --- | --- | --- |
| D1 | 测试改为**同包直接持锁置 `scanState.running=true`** 模拟扫描进行中，只测互斥拒绝路径（第二次调用返回 false），defer 持锁恢复 | 测试本意就只是测并发拒绝（源码注释自认单测起不了真扫描）；同包（`package merge`）可直接访问包级私有 `scanState`，无需为测试重构产品代码暴露缝；彻底消除 goroutine 泄漏源 |
| D2 | 「首次启动返回 true」路径单测不覆盖 | 该路径必然真起 goroutine（依赖 DB + embedding + 全表扫描），属集成/生产路径；强行单测就会重新引入本 bug |
| D3 | `runFullScan` 在 `repository.Repo.DB()` 前加 nil 检查，发 `error` 进度后 return | 防御兜底：goroutine 内 panic 会崩宿主进程（生产启动顺序异常、未来其它单测路径），一行 nil 检查成本换「任何环境下最坏是 error 进度而非进程崩溃」；错误文案沿用既有 `Status: "error"` 语义 |
| D4 | 不重构全局单例为依赖注入 | 超出 simple 修复范围；D1+D3 已根治 flaky 与崩溃面 |
| D5 | 台账 9 条存量 open 一并销账（8 resolve + 1 waive），非代码改动不入 tasks 验收门 | 巡检确认迁移测试已全绿（历史问题已修未销账+重复登记）；`TestInvestigateBoardQuestion_PersistsFullInvestigation` 为 testcontainer 集成测试，`-short` 设计性 SKIP，属误登记 → waive 而非 resolve（未「修好」，是本就不该挂账） |

## 风险

- 极低：D1 改后测试不再覆盖「首次 StartFullScan 返回 true」，该行为失去单测（D2 已声明取舍）；D3 防御分支正常路径永不触发。
- 状态恢复：测试 defer 持锁清 `scanState`（running=false、progress=nil），不影响同包其它测试（`TestIsScanRunning_Initially` 依赖初始 false）。
