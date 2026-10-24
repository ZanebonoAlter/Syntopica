# Review — fix-daily-report-cross-process-gate

- 审查方式：change-review-glm 子线程只读审查（制品 + diff），2026-10-24
- 审查范围：proposal/design/spec delta/test-cases/tasks、generation_gate.go 及测试、SaveReport 锁段与并发测试、testutil.NewIsolatedDB、flow 文档同步

### R-1 2026-10-24【High】探测超时但服务端已获锁 → 锁漂移到归池会话，永久 already_running
- **状态**：已修复（T4 与 release 失败路径统一 killSession 自终结会话，锁随会话终结必释放；design Risks 补记双向窗口；test-cases 白盒表 T4/R2 更新）

### R-2 2026-10-24【Medium】proposal「What Changes」声称 ON CONFLICT 原子 upsert 与 D4 实现（xact 锁 + 既有 Updates 路径）矛盾
- **状态**：已修复（proposal 措辞改为「(board, date) 粒度事务级 advisory lock 串行化 + 既有覆盖重建路径」，指向 design D4 取舍）

### R-3 2026-10-24【Medium】design D5 / test-cases V1 声明的双参 classKey 常量在代码中不存在（且双参两槽 int32 本装不下 64 位 fnv，草案不可实现）
- **状态**：已修复（D5/V1 回写为单参 bigint 实现 + 值域不相交论证 + 偏离原因留痕）

### R-4 2026-10-24【Medium】持闸期间 PG 重启的「无声丢锁」窗口未在 Risks 覆盖
- **状态**：已采纳（design Risks 补记窗口与联合概率评估，留痕接受不做 liveness 自查——单用户部署「PG 重启 + 临时实例并存」概率极低，行为不劣于升级前）

### R-5 2026-10-24【Low】design Risks 声称的「ctx 取消向上传播」缓解不存在（GORM Transaction 未绑定管线 ctx）
- **状态**：已修复（措辞修正为无自动缓解 + 外层预算兜底说明）

### R-6 2026-10-24【Low】B2 测试未真正验证「跨板不阻塞」（分键退化为全局串行时照样 PASS）
- **状态**：已驳回（耗时上界断言在树莓派上易 flaky、pg_locks 探查测的是 PG 行为而非 SaveReport；分键退化是性能非正确性问题，B1 已覆盖正确性主链；断言收紧落在 L-4/R-7 的同 writer 一致性上）

### R-7 2026-10-24【Low】B1 断言未验证 title 与 section label 来自同一写入者（锁失效混写可逃逸）
- **状态**：已修复（survivingWriter 定位后断言 section label 与 title 同 writer）

### R-8 2026-10-24【Low】spec「获取闸失败时……并照常释放闸」措辞可误读为失败方调 Release（会误放他方锁）
- **状态**：已修复（delta spec 改为「闸由持闸管线在结束时释放（获取失败方无锁可释放，SHALL NOT 调用释放）」）
