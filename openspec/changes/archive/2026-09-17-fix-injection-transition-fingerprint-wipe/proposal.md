<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

constraint-injection 动态层指纹 diff 失效：同一约束内容在**同一会话代内**（无档位/绑定切换、无 compact、文件零修改）被原样重复投递。实证（`.pi/harness/events.db`，2026-09-17，session 01a0ae99）：

1. `docs/reference/standard/shared/test-design.md` mtime 停在 2026-09-15，之后零修改；
2. 事件 29645（09:32:36，JIT 即时投递路径）与 29647（09:45:02，快照路径）注入同一节、同 reason=jit-path、同 3110 字节；
3. 触发时间线（29643-29652）：09:31:52 `mode.set(requirements, boundChange=null, source=skill)`（tool_execution_start 钩子的 skill 文件读取分支，**turn 中途**激活）→ 09:32:36 同 turn JIT 即时投递 test-design → 09:45:02 下一 turn 的 before_agent_start 快照路径重建 stableSnapshot（mode-base 重新记账 = 快照重建实证）并**清空 sentFingerprint** → test-design 被当作增量原样重投；
4. 用户复验 SQL（代边界 = session.start + mode.set，切掉绑定切换/compact 合法重投）在 9/16-9/17 命中 **~19 组代内重复**，波及多个会话（01a0acae / 01a0ad82 / 01a0af29 等），非孤例。

根因不是会话状态实例分裂（所有事件都记在同一真实 session_id 下，stateFor 单 Map 按 sessionId 键控），而是**延迟快照重建的指纹清空把「新 regime 下已投递」的指纹一并清掉**（详见 design.md 根因节）。

## What Changes

- `ChannelState` 新增 `fingerprintRegime`（投递指纹时所处的 `stableSnapshotKey(mode, boundChange)` 快照键），`deliverDynamic` 写指纹时同步记录；
- `before_agent_start` 快照重建分支的指纹清空条件从「快照 key 变化即清空」收紧为「**指纹 regime ≠ 新快照 key** 才清空」——turn 中途档位/绑定切换后同 turn 已投递的内容不再被下一 turn 的快照重建误清；
- 真实 regime 切换（如绑定修正 edit-dir → 下一 turn）仍清空指纹全量重投（既有设计行为不变）；compact 后 force 重发不受影响；
- `resetSessionState` / `newSessionState` / `inheritFromParent` 同步维护该字段（fork/子线程继承时指纹 regime 随 channel 一起拷贝，修复 fork 场景继承的陈旧快照 key 同样触发的重投变体）；
- smoke 测试补双钩子序列回归：stub 会话构造「未激活 turn → mid-turn skill 读取激活 → 同 turn JIT 投递 → 下一 turn 快照重建」，断言第二次投递零发生；另补「真实绑定切换仍全量重投」反向护栏。

## Impact

- 代码：`.pi/extensions/constraint-injection.ts`（约 5 处小改：ChannelState 结构 / newSessionState / resetSessionState / inheritFromParent / deliverDynamic / before_agent_start 清空条件）+ `.pi/extensions/tests/constraint-injection.smoke.cjs`（新增两个断言场景）。
- 纯工具链（pi 扩展），不触前后端业务代码，无 UI 影响，无数据库迁移。
- 不破坏 per-session-constraint-binding 隔离不变式（头注释 160-166 行）：本修复只动**本会话** channel 内部字段，不读不写他会话状态。
- fail-safe 性质保持：改动全部是进程内 Map/字符串操作，无新 I/O、无新异常面；指纹清空条件收窄只会减少投递，不会让注入崩溃。
