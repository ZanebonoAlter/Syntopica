# slim-harness-ledger 实施任务

## 1. 记账降载（quality-gate / edit-map）

- [x] 1.1 `quality-gate.ts`：child-session 降载记账改「每会话至多一条」——首个降载回合用账本判重（`queryBySession` 查该 session 是否已有 `child-session` 行），有则跳过；无 sessionId 的 stub 语境照常放行不记；判重/写账异常 fail-open，不影响放行。
- [x] 1.2 `lib/edit-map.ts`：`syncEditMap` 在 `merged` 与 base 逐项全等时直接返回 false，不落库；其余语义（base 读入、工具自管剔除、跨 session 并集）不变。含 `quality-gate.smoke.cjs` Q 系「未变化不落库」断言（review R-2）。
- [x] 1.3 `tests/quality-gate.behavior.smoke.cjs`：更新 J2/J3 断言（首回合一条、后续零新增），新增「reload 后不重复记账」（复用既有账本行）+「主会话零记账」对照用例。

## 2. 采集修复（harness-telemetry / retro）

- [x] 2.1 `harness-telemetry.ts`：删除 `tool_result(get_subagent_result)` 分支、`parseSubagentSummary` 调用、`completedAgents` 与 `agentChangeBindings` 快照；dispatch 的 change 列仍按派发时刻 `detectActiveChange` 判定。
- [x] 2.2 `lib/harness-log.ts`：事件词汇与 TTL 表移除 `subagent.complete`；注释同步。
- [x] 2.3 `scripts/harness/harness-retro.sh`：⑦ 子线程 token 占比改取子会话 `session.rollup` 终值（`parentSessionId` 归父），删除 `sub_pairs` 中对 `subagent.complete` 的 join；口径说明文字同步。
- [x] 2.4 `lib/session-rollup.ts`（`shouldWriteSnapshot`）+ `harness-telemetry.ts` 调用处：节流阈值改 20 回合 / token 增量 >50%；`final` 终值与 prev 回填逻辑不动。
- [x] 2.5 测试更新：`harness-log.smoke.cjs`（可写可查/TTL 断言改「该 kind 已退役，存量行仍被 legacy 表清扫」）、`telemetry-dispatch.smoke.cjs`、`failure-classify.smoke.cjs`（complete 断言删除）、`session-rollup.smoke.cjs`（节流边界 20/50%）、`scripts/harness/harness-retro.smoke.sh`（子线程占比 fixture 改为子会话 rollup）。

## 3. dev-process-guard

- [x] 3.1 `dev-process-guard.ts` turn_end：仅 `kills.length > 0` 时提醒；进程内按 pgid 去重（跨会话同 pgid 只提醒一次）；`orphan-warn` payload 增 `historicalOnly`（本轮被静默的历史残留条数）；`session_shutdown` 清理与 `orphan-killed` 记账不变。
- [x] 3.2 `tests/dev-process-guard.smoke.cjs`：更新 I-03（payload 含 `historicalOnly`）/I-04，新增「仅历史残留 → 零 steer 零记账」与「同 pgid 跨会话只提醒一次」用例。

## 4. 文档

<!-- doc-impact: none(纯内部 harness 记账口径，无对外行为变化；harness 机制文档与 skill 随代码同步更新) -->

- [x] 4.1 `docs/reference/harness/pi-extensions.md`：事件表/插件行为矩阵中 `subagent.complete`、child-session 记账粒度、rollup 节流、dev-process-guard 提醒范围的表述同步。
- [x] 4.2 `.agents/skills/harness-facts/SKILL.md`：事件词汇表、`subagent.complete` 行、查询配方与保留期说明同步；`.agents/skills/harness-retro/SKILL.md`：⑦ 子线程 token 占比口径同步。
- [x] 4.3 `docs/reference/flow/README.md`：里程碑/溯源中引用 `subagent.complete` 的历史表述按「历史事实不改写、当前口径以新 spec 为准」处理（仅在必要处补注）。

## 5. 验证

- [x] 5.1 `bash .pi/extensions/tests/run-harness-smoke.sh` 与 `bash .pi/extensions/tests/run-smoke.sh` 全绿（唯一红 = 既有多 change 欠账 `spec-gate-smoke:block-target-only-names`，台账 context `add-ui-pseudocode-chain-gate`，非本 change）。
- [x] 5.2 `bash scripts/harness/harness-retro.smoke.sh` 全绿（114/0）；`bash scripts/harness/harness-retro.sh --days 7 --json` 正常出报告（子线程 token 占比 40%，136 样本，源子会话 rollup）。
- [x] 5.3 `openspec validate slim-harness-ledger --strict` 零 ERROR（4 条 >500 字 Requirement WARNING 为主 spec 继承的长文本，非本次写坏，另行治理——review R-4）。
- [x] 5.4a 复跑 design「可量化断言」表的基线 SQL：历史口径未被改写（child-session 48,827 行 / subagent.complete 0 行 / edit.map 1,611 行 / 总 102,409 行，只增不改；旧口径仍在被未 `/reload` 的进程追加属预期）。
- [ ] 5.4b 部署（`/reload`）后 7 天回检四项指标：child-session 占 policy.decision 比例、edit.map 日均行数/体积、rollup 日均行数、orphan-warn 日均条数；结果补到 design「可量化断言」表「待验证」行。

## Review

- [x] 派子线程聚焦 review（审查制品 + diff，对照本 change 的 5 份 delta spec 与 design D1–D5），意见按 High/Medium/Low 处置后写 `review.md` 建账（High 0 / Medium 4 / Low 8，全部已修复或已采纳，`review-trace.sh` 通过）。
