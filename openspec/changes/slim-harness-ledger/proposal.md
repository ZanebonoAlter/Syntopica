<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

harness 事实库（`.pi/harness/events.db`）当前 83MB / 101,883 条事件。四类记账口径已经影响可读性与采集价值（数字取自 2026-11-27 账本，近 14 天或全库，标注见逐条）：

1. **child-session 降载记账按回合写**：48,527 条 = 全部 `policy.decision` 的 **96.6%**（近 14 天 27,181 条）。任何 policy 频次查询都必须先过滤它，retro 报告也得为它开白名单——一条"每回合都一样"的事实把账本变成了流水账。
2. **`edit.map` 每回合写全量快照**：1,595 行 / 31MB（平均 20.8KB），占库体积 37%；而消费方（spec-gate 并发检查、`concurrency-status.sh`、doc-impact）只取每个 change 的**最新一条**。
3. **`session.rollup` 每 5 回合或 token 增量 >20% 写一条**：29,043 行 / 13MB，保留期 90 天，稳态还会继续涨。
4. **`subagent.complete` 从未触发**：全库 0 条；419 条 `background=true` 的派发永远停在 `status: running`，⑦ 效能看板的子线程 tokens/toolUses 实际为空（`samples=0`）。锚点选错了——后台完成由 pi 主动推送，没人会再手动调 `get_subagent_result`。
5. **`dev-process-guard` 的 turn_end 提醒含跨会话残留**：541 条 warn 中 139 条（25.7%）提醒的是**别的会话**留下的进程；同一 pid 会被不同会话各提醒一次（实测 53 秒内两条、不同 session）。

## What Changes

- **child-session 记账降为每会话一条**：首个降载回合写一条即止，判重以事实库既有记录为准（跨 reload 有效）。spec 从「每个 turn_end 必须记」改为「每会话 MUST NOT 静默（至少一条）」。
- **`edit.map` 路径集合未变化时不写**：`merged` 与 base 全等直接返回，append-only 与「最新一条 = 完整集合」语义不变。
- **`session.rollup` 节流抬到每 20 回合或 token 增量 >50%**：`final=true` 终值快照与 `session_start` 回填 prev 终值不变。
- **删除 `subagent.complete`**：事件词汇、代码路径（解析 + `agentChangeBindings` + 幂等 Set）、测试、文档一并下线；⑦ 子线程 token 占比改由**子会话 `session.rollup` 经 `parentSessionId` 归父**提供。
- **`dev-process-guard` 的 turn_end 提醒只覆盖本会话制造的泄漏**（`inWindow>0`），并按 pgid 做进程内跨会话去重；历史残留改由定时回收（浏览器栈）与人工处置承担。

## Impact

- 代码：`.pi/extensions/{quality-gate.ts, dev-process-guard.ts, harness-telemetry.ts, lib/edit-map.ts, lib/harness-log.ts, lib/session-rollup.ts}`、`scripts/harness/harness-retro.sh`。
- 规格：`subagent-harness-coverage`、`harness-fact-log`、`harness-retro-loop`、`concurrent-change-coordination`、`dev-process-guard` 五个 capability 的 delta。
- 测试：`.pi/extensions/tests/{harness-log, quality-gate, quality-gate.behavior, telemetry-dispatch, failure-classify, session-rollup, dev-process-guard}.smoke.cjs`、`scripts/harness/harness-retro.smoke.sh`。
- 数据：历史事件不改写、不做 schema 迁移；新口径自部署后生效；已写入的 child-session 行随 30 天 TTL 自然消退。

## 已知取舍（部署后影响，先写清楚）

- **部署后用户可见行为变化**：无 UI 变化。账本噪声与体积下降；retro ⑦ 的子线程成本维度从「空」变成「有值」（来源换成子会话 rollup）。
- **丢失的能力**：① 报告不再能直接查「某子线程被降载了多少回合」（可由该 session 其它事件近似推断）；② 后台 dispatch 行的 `status` 恒为派发时刻快照（`running`），终态与成本以子会话 rollup 为准，属有意声明而非缺陷；③ turn_end 不再提醒跨会话历史残留的 dev 进程（浏览器栈由定时回收兜底，非浏览器栈需人工排查）。
- **需要用户手动执行的操作**：无。不需要重建库、不需要迁移脚本。
