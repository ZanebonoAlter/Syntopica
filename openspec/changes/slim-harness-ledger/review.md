# slim-harness-ledger 审查记录

审查日期 2026-11-27 · 审查范围：proposal / design D1–D5 / tasks / 5 份 delta spec / 代码 diff（`.pi/extensions`、`scripts/harness`）/ 文档 diff（`docs/reference`、`.agents/skills`）。
High 零条；Medium 4 条、Low 7 条（含 R-8b 观察项）。

### R-1 2026-11-27【Medium】D4 退役 kind 后历史 `subagent.complete` 行不再被 TTL 清扫，与 delta spec 矛盾
- **状态**：已修复（`lib/harness-log.ts` 新增 `LEGACY_RETENTION_DAYS` 只为清扫存量行、不参与写入校验；TTL 循环合并两表；`harness-log.smoke.cjs` 增「40 天前 subagent.complete 行被删除」断言）

### R-2 2026-11-27【Medium】D2 没有任何测试，tasks 也无测试任务
- **状态**：已修复（`quality-gate.smoke.cjs` Q 系增 `syncEditMap` 三步断言：首写 true / 子集未变化 false 且行数不变 / 新路径 true；tasks 1.2 补注）

### R-3 2026-11-27【Medium】D2 顺带改变了 retro「返工波次」的口径，spec/design/文案均未声明
- **状态**：已采纳（`harness-retro-loop` delta C 组补「波次 = 为该文件贡献过新增路径的 session 数」与新场景「被动携带不计波次」；`harness-retro.sh` 两处报告文案去掉「含被动携带」；design D2 取舍段补「已知副作用」）

### R-4 2026-11-27【Medium】`openspec validate --strict` 不通过，tasks 5.3 无法按字面勾选
- **状态**：已采纳（tasks 5.3 改为「零 ERROR」；4 条 >500 字 Requirement WARNING 是主 spec 继承的长文本，非本次写坏，另行治理不在本 change 范围）

### R-5 2026-11-27【Low】tasks 2.5 勾选了未改动的 `telemetry-archive.smoke.cjs`
- **状态**：已修复（tasks 2.5 / proposal Impact / design D4 测试清单删掉该文件；Impact 补 `quality-gate.smoke.cjs`）

### R-6 2026-11-27【Low】D5 `historicalOnly` 不覆盖「因 pgid 去重被静默」的进程
- **状态**：已修复（`orphan-warn` payload 增 `deduped = kills.length - warnList.length`；`historicalOnly` 语义不变；I-03 断言同步）

### R-7 2026-11-27【Low】`warnedPgids` 无界且不感知 pgid 复用
- **状态**：已修复（去重键改 `pgid:starttime`；新增 I-04d「同 pgid 不同 starttime 各提醒一次」）

### R-8 2026-11-27【Medium】D1 每回合查库可加进程内快路径
- **状态**：已修复（模块级 `Set<sessionId>` 快路径：命中跳过查库，miss 查库→写账后入 Set；`/reload` 后 Set 重建仍由账本判重兜底；随既有 session 重置清理）

### R-9 2026-11-27【Low】`failure-classify.smoke.cjs:218` 用已出联合类型的字面量调 `queryBySession`
- **状态**：已修复（改裸 SQL `SELECT COUNT(*) … WHERE kind='subagent.complete'`，与 telemetry-dispatch 同款）

### R-10 2026-11-27【Low】`harness-facts` SKILL 的 `orphan-warn` payload 字段表漏 `historicalOnly`
- **状态**：已修复（字段清单补 `historicalOnly` 与 `deduped` 及语义）

### R-11 2026-11-27【Low】design「落点」行号已漂移
- **状态**：已修复（D1–D5 落点全部改为符号锚：函数名 / CTE 名 / spec 场景名，不再写行号）


### R-12 2026-11-27【Low】D2 后「最新一条 = 完整集合」在排除规则新增时会滞后
- **状态**：已采纳（作为观察项记入 design「边界」：当前排除规则已落地、现存最新行干净，不处理；下次扩 `isToolManagedPath` 黑名单时配合一次强制重写）

- 审毕无发现（High 级：错误数据 / 红线破坏 / 主路径 spec↔实现不符）
- 审毕无发现（红线：append-only / fail-open / 无 schema 迁移——diff 新增行零 UPDATE/DELETE/DDL）
