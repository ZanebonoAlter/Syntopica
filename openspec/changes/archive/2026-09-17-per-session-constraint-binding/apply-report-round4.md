# apply-report — per-session-constraint-binding 轮次 4：跨进程子线程继承

范围：只改 `.pi/extensions/constraint-injection.ts` + `.pi/extensions/tests/constraint-injection.smoke.cjs`。
未动 `docs/` / tasks.md / design.md / specs / 其它 change 脏文件；未 commit。

## 1. 改动点（文件:行）

### `.pi/extensions/constraint-injection.ts`

| 行 | 改动 |
| --- | --- |
| `279` | 新增 `sessionIdFromSessionFile(file)`：`<ts>_<sessionId>.jsonl` → sessionId（原 `parentSessionId` 内联逻辑抽出，两条来源共用） |
| `289` | 新增 `parentSessionIdFromForkFile(file)`：路径含 `/forks/`（或 `\forks\`）时取**父目录名** `<ts>_<父id>` 解父会话 id；非 fork 路径 → `null` |
| `304` | `parentSessionId(ctx)` 改双来源：① `getHeader()?.parentSession` → ② `getSessionFile()` 的 forks 路径兜底；两条都无 → `null`（不可证 → 不继承） |
| `317` | `inheritFromParent`（内存路径）**语义不变**，仅补注释：真实链路极少命中（pi-subagents 子线程是独立 node 进程，父内存态不可见） |
| `344` | **新增 `inheritFromParentHistory(state, ctx)`**：解析父 id → `recoverMode(ctx.cwd, pid)`（复用既有「同 id 单段 + 绑定 change 目录存在性校验」）→ 只采纳 `mode` + `boundChange`（不编造命中集/快照/输入窗）；异常 `try/catch` 吞掉并 `console.error`，不阻断 `session_start` |
| `1772` | startup 分支：`inheritFromParent(...) \|\| inheritFromParentHistory(...)`（内存优先，跨进程回退），继承成功且 `state.mode` 非空 → 记 `source=inherit`；父不可证/无记录 → 继续按自身 sessionId `recover` → 都没有则未激活 |
| `1804` | `fork` 分支同款双路继承 + 记账 |
| 注释 | `session_start` 上方 @bugfix 注释补实测修正：子线程是**独立 node 进程**，「不重置」只保住主会话，子线程靠显式继承拿约束块 |

### `.pi/extensions/tests/constraint-injection.smoke.cjs`

| 行 | 改动 |
| --- | --- |
| `657` | 新增 `readModeSetFull(dir)`（含 `session_id`，供「记在谁名下」断言） |
| `667` | 新增 `seedModeSet(dir, sessionId, mode, boundChange)`——直接往 smoke 临时事实库插一条父会话 `mode.set`，模拟「父会话不在本进程内存，只有事实库记录」 |
| `843-875` | **19.10** 跨进程继承（事实库回退）：3 条断言 |
| `877-895` | **19.11** 路径兜底解析（`getHeader` 为 null，靠 `getSessionFile()` 的 forks 路径解父 id）：1 条断言 |
| `897-912` | **19.12** 两条父子证据都无（非 fork 路径）→ 不继承：1 条断言 |

## 2. 命令与原始结果

| 命令 | 结果 |
| --- | --- |
| `bash .pi/extensions/tests/run-smoke.sh` | **SMOKE OK**，189 项检查（本轮新增 5 项：19.10×3 / 19.11×1 / 19.12×1），`grep -c '^❌'` = **0** |
| `bash .pi/extensions/tests/run-harness-smoke.sh` | **SMOKE OK**（12 套件全绿） |
| `grep -nE '^let (currentMode\|modeBoundChange\|turnBindLocked\|recentInputs\|jitDocHits\|keywordDocHits)' .pi/extensions/constraint-injection.ts` | **零命中**（隔离不变量保持） |
| 新增断言原文（绿） | `✅ 跨进程: 子线程据父会话事实库历史继承档位与绑定` / `✅ 跨进程: 子线程带父会话声明域红线层（kwdom）` / `✅ 跨进程: 继承显式记账 source=inherit 且记在子会话名下` / `✅ 跨进程: getHeader 缺失时按 fork 路径解父 id 并继承` / `✅ 跨进程: 无父子证据（非 fork 路径）→ 不继承（未激活）` |

### 19.10 / 19.11 断言形态

- **19.10**：`seedModeSet(tmp, 'ci-smoke-parent-xproc', 'implementation', 'smoke-fixture-budget')`（父会话**从不作为 ctx 出现** → 内存必 miss）→ 子会话 ctx 的 `getHeader().parentSession` 指向 `/…/2026-09-17T00-00-00-000Z_ci-smoke-parent-xproc.jsonl` → `emit('session_start', {reason:'startup'})` → 断言：
  1. 稳定层 `/档位：实现/` + `活跃变更：smoke-fixture-budget`（= 采纳了父会话历史）；
  2. 声明域红线层 `### kwdom.md` 在（= 约束块真的带上了）；
  3. 增量区恰有 1 条 `source === 'inherit'` 且 `boundChange === 'smoke-fixture-budget'` 且 **`session_id === 'ci-smoke-child-xproc'`**（记在子会话名下，不是父会话名下）。
- **19.11**：`getHeader: () => null` + `getSessionFile: () => '/…/2026-09-17T00-00-00-000Z_ci-smoke-parent-xproc/forks/2026-09-17T00-01-00-000Z_ci-smoke-child-path.jsonl'` → 断言仍能继承同样两条（档位 + kwdom 红线层）。
- **19.12**（负向）：`getHeader: () => null` + `getSessionFile()` 为**非 fork** 路径（`…/<ts>_ci-smoke-child-nosrc.jsonl`）→ 断言 `/档位：未激活/`（不借用任意他会话状态）。

## 3. 反向验证（新增用例非空性）

探针：在 `inheritFromParentHistory` 函数体首行插入 `return false; // PROBE: short-circuit history fallback`。

| 步骤 | 原始结果 |
| --- | --- |
| 探针前 sha256 | `24b50bf0294b878269c3a2a0bd0dc0f91523e38fe850e365126af7c0360e9d16` |
| 探针后跑 `run-smoke.sh` | **4 项失败**，恰好是 19.10 的三条 + 19.11 的一条：`❌ 跨进程: 子线程据父会话事实库历史继承档位与绑定` / `❌ 跨进程: 子线程带父会话声明域红线层（kwdom）` / `❌ 跨进程: 继承显式记账 source=inherit 且记在子会话名下` / `❌ 跨进程: getHeader 缺失时按 fork 路径解父 id 并继承`；**19.12（负向）保持 ✅**（它断言的就是「不继承」，符合预期，说明正负向断言各自有判别力） |
| 恢复后 sha256 | `24b50bf0…60e9d16`（与探针前**逐字节一致**） |
| 恢复后双门禁 | `run-smoke.sh` → **SMOKE OK**；`run-harness-smoke.sh` → **SMOKE OK** |
| 探针残留 | `grep -n 'PROBE'` → 无残留 |

## 4. 残留风险 / 待办（交主线程）

1. **真实链路未被本进程观测**：runner 进程里的扩展是 00:53 重启时加载的旧代码，本轮改动要**重启 pi（或触发扩展重载）后**才能观测到真实子线程继承。建议主线程在重启后跑一次「主会话绑 change → 派发现成子线程 → 查 `mode.set source=inherit`」的探针（判据：该子会话名下出现 `source=inherit` 且 `boundChange` = 主会话绑定）。
2. **`getSessionFile()` 契约依赖**：19.11 依赖 pi 的 fork 文件布局（`<父会话目录>/forks/<ts>_<子id>.jsonl`）。已实测盘上 3 个 fork 文件符合该布局；若 pi 改布局，回退路径静默失效（方向安全：退化为未激活，不借用他人），但会丢掉子线程继承——建议在 pi 升级后复跑一次真实探针。
3. **内存路径与事实库路径的优先级**：父会话在本进程内存且当前**未激活**（如刚 /new）时，内存路径胜出 → 不回落到父会话的历史绑定（有意：内存态是「现在」，历史绑定可能是陈旧的）。跨进程子线程不存在这种歧义。
4. **子线程继承只带 mode + boundChange**：父会话的 JIT/关键词命中集、稳定层快照不跨进程复制（拿不到不编造）→ 子线程会按自己的输入/编辑重新命中，动态层内容可能与父会话不同。这是有意的取舍（宁可少注入，不伪造状态）。
5. **事实库写入污染（非本 change 代码问题，供主线程清理）**：轮次 3 的探针把 smoke 事件以 `session_id='GLOBAL'` 写进了**真库** `.pi/harness/events.db`（31 行：`constraint.inject` 16 / `pin.write` 6 / `pin.read` 5 / `mode.set` 4，时间戳均为 2026-09-17T01:07:13）。建议主线程按 `session_id='GLOBAL'` 清掉这批探针行，避免污染后续归因查询。
