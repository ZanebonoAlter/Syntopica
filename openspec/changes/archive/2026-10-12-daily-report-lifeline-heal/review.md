# Review: daily-report-lifeline-heal

- 审查人：change-review-glm 子线程（zai-coding-cn/glm-5.3 high，只读）
- 日期：2026-10-12
- 范围：change 全部制品（proposal/design/specs/tasks/test-cases）+ 后端实现 diff（lifeline_context.go / lifeline_heal_tolerant_test.go / scheduler_jobs.go / job_daily_report.go 及其测试）+ 两份 flow 文档
- 结论：0 High / 2 Medium / 6 Low；决策①-⑤一致、5 个 Scenario 全落点、四条红线守住、无伪断言（R-1 伪覆盖声明除外，已修复）

### R-1 2026-10-12【Medium】test-cases.md 伪覆盖声明：三行变体声称「task 1.2 补充用例」但用例不存在，步 4 落点映射偏乐观

- **状态**：已修复

（补 `TestHealMissingTolerant_TopicNotListedNotHealed` / `TestHealMissingTolerant_NoMaterialNoHeal` / `TestHealMissingTolerant_MonthBoundaryDates` 三用例；test-cases.md 三行落点改为实际测试名并注明已落地，步 4 落点改「job 层触发断言 + 函数层缺行即补组合覆盖」。）

### R-2 2026-10-12【Medium】spec「week SHALL NOT 启用每日补齐」无任何断言守护（lifelineHealGranularities 零测试经过）

- **状态**：已修复

（补白盒用例 `TestLifelineHealGranularitiesExcludeWeek`，断言粒度集合恰为 `[]string{month, year}`，未来误加 week/误删 year 即红。）

### R-3 2026-10-12【Low】手动 HTTP 生成路径（generateAllBoards/generateSingleBoard）不经 DailyReportJob 不触发 heal，叠加定时路径 already_exists 跳过时该日缺口顺延窗口退化为两天

- **状态**：已驳回

（spec 字面即「挂在日报 job 尾部」，handler 手动路径本就不在契约内；TriggerNowWithDate 路径同触发；顺延不丢语义兜底最坏晚一天。若要收紧在 handler 成功终态复用 launchLifelineHealAsync，属独立小改动，另开 change。）

### R-4 2026-10-12【Low】全空版面日（reportCount==0 且 failedCount==0）仍触发 heal，「日报成功=模型健康」前提不成立

- **状态**：已驳回

（口径与体检尾步先例一致（failedCount==0，空版面算成功侧）；空版面日本就零新增素材，误触发仅空转扫描、零 LLM 调用、失败顺延，无害。收紧为 reportCount>0 会与先例口径分叉，不值。）

### R-5 2026-10-12【Low】HealMissingTolerant 与 HealMissing 约 40 行 gap-scan 逻辑重复，双份维护有漂移风险

- **状态**：已驳回

（本 change 设计明示「新函数、HealMissing 原行为不动」，回归由既有 scheduler_jobs 测试守护；抽共享 scanMissingPeriods helper 属重构，留后续 change。）

### R-6 2026-10-12【Low】panic 隔离用例收尾为 50ms sleep 启发式，recover 分支本身无可观测断言

- **状态**：已驳回

（主断言有效：缺 recover 时 panic 展开即崩测试进程、用例必红；recover 路径的可观测化（logging seam/二次 channel）为日志细节，成本不值。）

### R-7 2026-10-12【Low】heal 轮无防重入（单轮卡 >24h 时次日叠轮）

- **状态**：已驳回

（两轮幂等——只补缺行 + Upsert 不重写；最坏重复少量本地 LLM 调用；单用户系统概率极低，登记备查。）

### R-8 2026-10-12【Low】heal goroutine 与 backfill 补档流水线当晚并发争用本地 LLM

- **状态**：误报丢弃

（design.md Risks 首条已明示接受：本地 4B 单实例天然排队串行，最坏拉长补齐耗时；核实无遗漏，非问题。）
