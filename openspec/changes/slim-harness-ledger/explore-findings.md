
## harness 账本新记账口径（slim-harness-ledger D1–D5）

改动落地后的 harness 事实库口径（`.pi/harness/events.db`，代码在 `.pi/extensions/`，生效需 `/reload`）：
- D1 `quality-gate.ts` turn_end 第 0 步：子线程降载仍整体放行，但 `policy.decision{quality-gate,bypass,child-session}` 每会话至多一条；判重 `hasChildSessionDecision` 读账本（`queryBySession`），模块级 `Set<sessionId>` 只是快路径。改前占 policy.decision 96.6%（48,527/50,244）。
- D2 `lib/edit-map.ts` `syncEditMap`：`merged` 与 base 逐项全等（`samePaths`，`mergePaths` 字典序）直接 return false 不落库；append-only 不变，消费方仍取每 change 最新一条。副作用：retro「返工波次」= 贡献过新增路径的 session 数（不再含被动携带）。
- D3 `lib/session-rollup.ts` `shouldWriteSnapshot`：turn%20==0 或 token 增量 >50%（严格大于）；`final=true` 与 session_start 回填 prev 不变。
- D4 `subagent.complete` 已退役：`harness-log.ts` 的 `HarnessEventKind` 16 类；`LEGACY_RETENTION_DAYS{"subagent.complete":30}` 仅为清扫存量；`harness-telemetry.ts` 删 `get_subagent_result` 分支/`completedAgents`/`agentChangeBindings`；`subagent.dispatch.status` = 派发时刻快照（后台恒 running）。retro ⑦ 子线程 token 占比 = `ru_child_sum`（`ru_t.parent != ''` 的 rollup 终值 tokens.total 合计）/ `ru_sum`（全部 rollup 终值），键 `m7.sub_tokens` / `m7.sub_samples`。
- D5 `dev-process-guard.ts` turn_end：仅 `kills.length>0`（本会话制造）才 steer + `orphan-warn`；去重键 `warnedPgids` = `pgid:starttime`；payload 含 `inWindow`/`historicalOnly`（窗口外残留数）/`deduped`（同 pgid 被滤数）；`session_shutdown` 清理与 `orphan-killed` 不变。
- 验证入口：`bash .pi/extensions/tests/run-harness-smoke.sh`（唯一既有红 `spec-gate-smoke:block-target-only-names` 属 add-ui-pseudocode-chain-gate）、`bash scripts/harness/harness-retro.smoke.sh`。

**引用**：.pi/extensions/quality-gate.ts:hasChildSessionDecision、.pi/extensions/lib/edit-map.ts:syncEditMap、.pi/extensions/lib/session-rollup.ts:shouldWriteSnapshot、.pi/extensions/lib/harness-log.ts:LEGACY_RETENTION_DAYS、.pi/extensions/harness-telemetry.ts、.pi/extensions/dev-process-guard.ts、scripts/harness/harness-retro.sh:ru_child_sum

<!-- pinned 2026-11-27T15:54:03Z -->
