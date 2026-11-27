<!-- quant-claims: declared -->

# slim-harness-ledger 设计

## 上下文

账本现状（2026-11-27 采样，`events.db` 83.3MB / 101,883 条）：

| 指标 | 观测值 | 来源 |
| --- | --- | --- |
| `policy.decision` 中 child-session 条数 | 48,527 / 50,244（96.6%）；近 14 天 27,181 | `GROUP BY json_extract(payload,'$.reasonCode')` |
| `edit.map` 行数 / 体积 | 1,595 行 / 31MB / 平均 20,766B | `SUM(LENGTH(payload))` |
| `session.rollup` 行数 / 体积 | 29,043 行 / 13MB（≈738 行/天，保留 90 天） | 同上 |
| `subagent.complete` 行数 | 0（全库），background 派发 419 条恒为 `running` | `kind='subagent.complete'` |
| dev-process-guard warn | 541 条，其中 139 条仅历史残留（无本会话制造） | `payload.inWindow` |

全表扫描 5ms、retro 秒级——本次改动**不以性能为由**，目标是信噪比、可读性与采集正确性。

## Decisions

### D1 child-session 降载记账降为每会话一条

**决策**：`quality-gate` 的 turn_end 降载判定（`lib/child-session`）保持不变；记账从「每回合一条」改为「每会话至多一条」——首个降载回合写，后续回合照常降载但不写。判重 MUST 以事实库既有记录为准（`queryBySession` 查该 session 是否已有 `child-session` 行），不依赖进程内 `Set`，因此 `/reload`、pi 重启后不会重复记。无 `sessionId` 的 stub 语境照常放行且不记账（与既有记账门控一致）。记账失败 fail-open，不影响降载与放行。

**取舍**：失去「某子线程降载了多少回合」这一直接可查量（可由该 session 的 `session.start`/其它事件近似推断）。换来 policy.decision 量级从 27,181/14 天降到「子线程会话数量级」。相比「每 20 回合采样」方案，实现更简单且没有会话内计数状态；相比「session_shutdown 一条带 turnCount」，不会因进程被杀而丢记录。

**落点**：`.pi/extensions/quality-gate.ts` 的 `hasChildSessionDecision` 与 turn_end 第 0 步（`isChildSession` 分支；进程内 Set 仅为快路径，账本为准）；`.pi/extensions/tests/quality-gate.behavior.smoke.cjs` J2/J3；`openspec/changes/slim-harness-ledger/specs/subagent-harness-coverage/spec.md`「子线程门禁降载」。

### D2 `edit.map` 路径集合未变化时不写

**决策**：`syncEditMap` 在 `merged` 与 `editMapBase` 逐项全等时直接 `return false`，不落库。首次绑定/档位切换的 base 读取、工具自管路径剔除、跨 session 并集语义全部不变。append-only 约定不变（不做 UPDATE/DELETE）。

**取舍**：放弃「每回合都有快照」的冗余备份；保留「最新一条即完整集合」。未采用「覆盖旧行」方案——那会破坏账本 append-only 与历史考古能力，收益（再省几 MB）不值得。**已知副作用**：retro「返工波次」口径收窄为「为该文件贡献过新增路径的 session 数」——仅重编已有文件的 session 不再计入，新数据上波次会系统性偏低；口径已在 `harness-retro-loop` delta 与报告文案中声明。

**落点**：`.pi/extensions/lib/edit-map.ts` 的 `syncEditMap`（`samePaths` 全等短路）；`.pi/extensions/tests/quality-gate.smoke.cjs` Q 系未变化不落库断言；`openspec/changes/slim-harness-ledger/specs/concurrent-change-coordination/spec.md` 场景「路径集合未变化时不追加快照」；回检 `edit.map` 行数/体积。

### D3 `session.rollup` 节流抬到 20 回合 / 50%

**决策**：`shouldWriteSnapshot` 的阈值从 `turn % 5 == 0` / `tokenDelta > 20%` 改为 `turn % 20 == 0` / `tokenDelta > 50%`；`final=true` 终值（`session_shutdown`）与 `session_start` 回填 prev 终值的路径不变。规范里「最后一条快照与终值偏差有界」的约束保留——20 回合属于有界范围，且终值快照兜底。

**取舍**：最坏情况下崩溃丢掉 19 回合的中间进度（此前丢 4 回合）。收益是行数降到约 1/4、90 天稳态体积从约 30MB 降到约 8MB。

**落点**：`.pi/extensions/lib/session-rollup.ts` 的 `shouldWriteSnapshot` 与 `harness-telemetry.ts` turn_end 调用处；`.pi/extensions/tests/session-rollup.smoke.cjs` 节流边界用例；`openspec/changes/slim-harness-ledger/specs/harness-fact-log/spec.md`「session 效能汇总记账」。

### D4 删除 `subagent.complete`，子线程成本改由子会话 rollup 归父

**决策**：下线 `subagent.complete` 整条链路——
- `harness-telemetry.ts`：删除 `tool_result(get_subagent_result)` 分支、`parseSubagentSummary` 相关调用、`completedAgents` 幂等 Set 与 `agentChangeBindings` 快照（dispatch 的 change 列仍按派发时刻判定，不再需要为回填暂存）；
- `lib/harness-log.ts`：事件类型词汇与 TTL 表移除该 kind；另设 legacy 保留期表只为清扫存量行（全库实测 0 条，但 spec 场景「历史行 31 天后被 TTL 清扫」必须可验证）；
- `scripts/harness/harness-retro.sh`：⑦ 子线程 token 占比改取**子会话 `session.rollup` 终值**（`payload.parentSessionId` 归父，与 B' 任务成本同源），不再 join `subagent.complete`；
- 测试：`harness-log.smoke.cjs`（可写可查/TTL 断言改「该 kind 已退役，存量行仍被清扫」）、`telemetry-dispatch.smoke.cjs`、`failure-classify.smoke.cjs` 中 complete 相关断言改为「不再产生该事件」或删除；
- 文档：`.agents/skills/harness-facts/SKILL.md` 事件表、`docs/reference/harness/pi-extensions.md`、`docs/reference/flow/README.md` 里程碑引用。

**理由**：419/419 后台派发没有一条回填证据——锚点（父线程主动调 `get_subagent_result`）在真实工作流里不会发生。子会话 rollup 已带 `parentSessionId` 且覆盖率 98%，B' 任务成本已经用这条路归父，子线程成本可以直接复用，不必再养一条解析路径。后台 dispatch 行的 `status` 明确为**派发时刻快照**（不承诺终态），该语义写进 spec。

**落点**：`.pi/extensions/harness-telemetry.ts` 的 `tool_result` 处理（`get_subagent_result` 分支已删）、`scripts/harness/harness-retro.sh` 的 `ru_child_sum` CTE 与 `effectiveness.subagent`；`openspec/changes/slim-harness-ledger/specs/harness-fact-log/spec.md`（REMOVED）、`openspec/changes/slim-harness-ledger/specs/harness-retro-loop/spec.md`（B 组口径）。

### D5 `dev-process-guard` turn_end 提醒收窄到「本会话制造」

**决策**：
- turn_end 只在 `kills.length > 0`（本会话窗口内、会话结束会被自动清理的泄漏）时提醒；纯历史残留不再 steer、不记 `orphan-warn`；
- 进程内按 pgid 去重，同一进程组只提醒一次（不同会话不再各报一次）；
- 保留「每会话至多一次」，`session_shutdown` 的清理逻辑与 `orphan-killed` 记账完全不变；
- `orphan-warn` payload 增加 `historicalOnly`（被本轮静默的历史残留条数），让"少提醒了多少"可回检。

**取舍**：跨会话历史残留的非浏览器 dev 进程不再有人提醒（浏览器栈由 `browser-leak-guard.sh` 定时回收）。这是有意让渡——那类提醒 139/541 是纯噪声，且收件人通常既不是制造者也无法判断该不该杀。

**落点**：`.pi/extensions/dev-process-guard.ts` 的 turn_end 处理（`kills.length === 0` 早退与 `warnedPgids` 去重）；`.pi/extensions/tests/dev-process-guard.smoke.cjs` I-03/I-04 与新增历史残留静默用例；`openspec/changes/slim-harness-ledger/specs/dev-process-guard/spec.md`「turn_end 软提醒」。

## 可量化断言

| 类别 | 断言 | 观测值 | 复算方式 | 观测日期 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 数据 | child-session 占 `policy.decision` 比例 | 96.6%（48,527/50,244） | `SELECT json_extract(payload,'$.reasonCode'), COUNT(*) ... GROUP BY 1` | 2026-11-27 | 改前基线 |
| 数据 | `edit.map` 行数与平均 payload | 1,595 行 / 20,766B | `SELECT COUNT(*), AVG(LENGTH(payload)) FROM events WHERE kind='edit.map'` | 2026-11-27 | 改前基线 |
| 数据 | `session.rollup` 日均行数 | 738 行/天 | 近 14 天 `COUNT(*)/14` | 2026-11-27 | 改前基线 |
| 数据 | background 派发的 complete 回填率 | 0/419 | `payload.background=1` 且存在 complete 的 agentId 数 | 2026-11-27 | 改前基线 |
| 数据 | `orphan-warn` 中历史残留占比 | 25.7%（139/541） | `payload.inWindow > 0` 分组 | 2026-11-27 | 改前基线 |
| 数据 | 改后 7 天回检：四个比例/量级是否按预期下降 | 待测 | `harness-retro.sh --days 7` + 上表同款 SQL | 部署后 7 天 | 待验证 |

## 边界（明确不改）

- 不改写任何历史事件、不做 DB schema 迁移；child-session 存量行随 30 天 TTL 自然消退。
- 不改 quality-gate 的**门禁降载行为本身**（只改记账粒度）。
- 不改 rollup 的 `final` 终值与 `session_start` 回填 prev 逻辑。
- 不改 `dev-process-guard` 的判定条件、清理动作与 `orphan-killed` 记账。
- 不新增注入通道，不动 constraint-injection。
- **观察项（review R-12）**：D2 后若将来给 `isToolManagedPath` 的排除黑名单加新模式，存量最新快照里可能带着「后来才被排除」的路径，在下一个真正新增路径出现前不会自愈（改前每回合重写所以会自愈）。当前所有排除规则已落地、现存最新行干净，不处理；下次扩黑名单时记得配合一次强制重写或清洗。

## 验证

- smoke：`bash .pi/extensions/tests/run-harness-smoke.sh`（含 harness-log / telemetry / dev-process-guard / quality-gate behavior）与 `bash .pi/extensions/tests/run-smoke.sh`；`bash scripts/harness/harness-retro.smoke.sh`。
- 账本回检：部署 7 天后按「可量化断言」表同款 SQL 复查；对照 `harness-retro.sh --days 7` 的 `soft.*` / `m7.*` 指标确认无回归。
