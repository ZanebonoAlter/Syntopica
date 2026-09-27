# apply-report — per-session-constraint-binding

范围：tasks.md §1-§7（`constraint-injection.ts` 会话作用域重构 + smoke 用例 + 模块注释）与 §8.1-§8.5 可本地执行项。
**未做**：§8.6（部署后真实多会话事实库口径核对）、§8.7（完工汇报）——属主线程部署后动作；未 commit；未碰 `docs/`、AGENTS.md、其它 change 脏文件、其它扩展。

改动文件（2 个，均在本 change 范围内）：

| 文件 | 规模 |
| --- | --- |
| `.pi/extensions/constraint-injection.ts` | +319 / −145（净含全局状态容器化重构） |
| `.pi/extensions/tests/constraint-injection.smoke.cjs` | +147（第 19 组会话隔离用例） |

---

## 1. design 决策：父会话识别（tasks §1，**路径 1 走通，无需回退**）

**结论：靠 session header 的 `parentSession` 判定「父子关系可证」。**

实测证据（三条，均可复现）：

1. **类型层**：`@earendil-works/pi-coding-agent` 的 `dist/core/session-manager.d.ts` —
   `SessionHeader { type, version?, id, timestamp, cwd, parentSession? }`；
   `ReadonlySessionManager`（扩展可见的 `ctx.sessionManager`）的 Pick 列表**包含 `getHeader`**；
   `SessionManager.forkFrom(sourcePath, …)` 与 `NewSessionOptions.parentSession` 说明 fork 会记录父会话。
2. **盘上事实**：本会话归档目录 `…/2026-09-16T16-02-56-624Z_01a0aaf4-…/forks/` 下 6 个 fork/子线程会话文件，
   header 第一行**全部**带 `"parentSession":"/home/…/2026-09-16T16-02-56-624Z_01a0aaf4-….jsonl"`。
   即：pi-subagents 的子线程会话**就是 fork 会话**，父会话路径可证。
3. **反解规则**：父会话文件名形如 `<ISO时间戳>_<sessionId>.jsonl` → 取第一个 `_` 之后、去掉 `.jsonl` 的部分即父 sessionId
   （`parentSessionId()` 实现；时间戳内不含 `_`，规则稳定）。

因此 **§1.2 的「派发即登记」回退未启用**（保留为将来 pi 变更时的备选）。
`ExtCtx` 类型相应补了 `getHeader?: () => { parentSession?: string } | null`（可选链，烟测 stub 无该方法时安全降级为「不可证」）。

---

## 2. 逐节点改动（文件:符号）

### 2.1 状态容器（§2）

`constraint-injection.ts`：

- **新增** `type ChannelState`（`stableSnapshot` / `sentFingerprint` / `pendingSnapshot`）与
  `type SessionState`（`sessionId` / `mode` / `boundChange` / `turnBindLocked` / `recentInputs` /
  `jitDocHits` / `keywordDocHits` / `pinReadSeen` / `channel` / `lastUsedAt`）——字段注释保留原全局变量的语义说明。
- **删除** 6 个模块级会话语义全局（`currentMode` / `modeBoundChange` / `recentInputs` / `jitDocHits` /
  `keywordDocHits` / `pinReadSeen`）+ 4 个混合通道全局（`stableSnapshot` / `sentFingerprint` /
  `pendingSnapshot` / `turnBindLocked`）+ `resetChannelState()`。
- **新增访问器**：`newSessionState` / `sessionKey` / `evictSessionStates` / `stateFor`（唯一读写入口）/
  `realSessionId`（兜底槽 → undefined，避免 stub 会话写事实库）/ `parentSessionId` / `inheritFromParent` /
  `resetSessionState`；常量 `FALLBACK_SESSION_KEY`、`SESSION_STATE_LIMIT = 32`。
- **新增烟测观测面**（导出，不参与注入逻辑）：`sessionStateKeysForTest()`、
  `SESSION_STATE_LIMIT_FOR_TEST`（有界性可断言；内部 `sessionStates` 不外泄可变引用）。
- **函数签名改造**：`planInjection(cwd, cfg, state, fallbackChange)`（原 `(cwd, cfg, mode, boundChangeName, fallbackChange)`；
  内部 `mode`/`boundChangeName` 取自 state，命中集/输入窗读 `state.*`）；
  `matchKeywordDocs(text, cfg, state)`；`deliverDynamic(pi, ctx, cfg, state, changeName, items, label, force)`
  （原第 4 参 `sessionId: string | undefined`，指纹读写改为 `state.channel.sentFingerprint`）。
  `stableSnapshotKey(mode, boundChange)` **未改**（隔离由存储层保证，导出签名不动以保烟测兼容）。

### 2.2 生命周期与继承（§3.1 + design D3）

`pi.on("session_start")`：

| reason | 新行为 |
| --- | --- |
| `startup` | 本会话 `mode === null` 时：**先** `inheritFromParent`（父子可证且父有状态 → 拷档位/绑定/命中集/快照/指纹，记 `mode.set source=inherit`）；**否则**按同 sessionId 第 1 段恢复（`source=recover`）；都没有 → 保持未激活。本会话已有状态 → 不动（@bugfix 语义保留）。 |
| `new` | `resetSessionState(state)` ——**只清本会话**（旧行为是清全局） |
| `fork` | 先重置本会话，再 `inheritFromParent`（可证则继承 + 记账）；不可证 → 未激活 |
| `resume` / `reload` | 重置本会话后按同 sessionId 恢复（`source=recover`），逻辑不变 |

继承为**深拷**（Map/Set/数组/快照 items 均新建），子会话后续改写不影响父会话；继承含 `sentFingerprint`，
因为子会话上下文是父会话的拷贝，父会话已投递的动态层条目不应重发。

### 2.3 其余读写点（§3.2-§3.6）

- `pi.on("input")`：输入入窗、命令绑定、`source=command` 记账、turn 锁全部走本会话 state。
- `pi.on("tool_execution_start")`：skill 绑定（`source=skill`）、JIT 命中集、绑定修正
  （`resolveBindAction` 的 `currentBound`/`boundExists` 与 `state.turnBindLocked` 取本会话，`source=edit-dir`）、
  JIT 即时投递（`planInjection`/`deliverDynamic` 传 state）。
- `pi.on("before_agent_start")`：turn 解锁只解本会话；绑定目录消失回落只对本会话；`planInjection(ctx.cwd, cfg, state, activeChange)`；
  mtime 兜底显式化（`source=fallback`）写本会话；稳定层快照/`isTransition` 指纹重置/pin.read 去重全部落 `state.channel` / `state.pinReadSeen`。
- `pi.on("session_compact")`：`stateFor(ctx).channel.pendingSnapshot = true`（他会话 compact 不再触发本会话重发）。
- `pin_finding` 工具：绑定解析读本会话 `state.mode`/`state.boundChange`；`pin.write` 记账用 `realSessionId(state)`。
- `pin.read` 去重 key 由 `${sessionId}|${title}` 简化为 `title`（集合已按会话隔离，前缀冗余）。

### 2.4 模块注释（§7.1）

文件头「职责」新增 **0. 会话作用域状态** 段：隔离不变量（归属只由本会话决定 / 不借用他会话 / 边界事件只重置本会话 / 有界）
+ 明确「只有父子可证才继承」+ 指向 smoke 第 19 组作为回归面。

---

## 3. smoke 用例（§4，第 19 组，21 条 check）

| # | 用例 | 断言要点 |
| --- | --- | --- |
| 19.1 | 两会话交叉绑定互不污染（4 条） | A 绑 budget 不被 B 改写；A 声明域（kwdom 红线层）在；B 归属 temp；B 无 A 的域 |
| 19.2 | 他会话绑定变化不刷新本会话稳定层 | A 稳定层字节前后完全相等 |
| 19.3 | 无自身绑定且无父子关系 → 不借他会话 | 未激活 + `活跃变更：无` + 无 kwdom + 本会话零动态投递 |
| 19.4 | 子线程显式继承（4 条） | 继承档位/绑定/声明域 + `mode.set source=inherit` 且 `boundChange=smoke-fixture-budget` + 主会话不被清零 |
| 19.4b | 父会话不可证（header 指向不存在会话）→ 不继承 | 未激活 |
| 19.5 | 命中集按会话隔离（4 条） | A 关键词命中投递全节（BIGKW-MARKER）；B 零动态投递；A JIT 命中 bigjit；B 零 JIT 投递 |
| 19.6 | turn 绑定锁按会话隔离（2 条） | B 新 turn 兜底绑不被 A 的锁拦住；A 保持原绑定 |
| 19.7 | 无 sessionId 兜底槽 + 同 sessionId 多视图（2 条） | 兜底槽自洽；同 id 多视图共享状态 |
| 19.8 | 会话条目有界（2 条） | 超建 4 个后不超上限；最久未用（记录首 key）被淘汰 |

---

## 4. 命令与原始结果

| 命令 | 结果 |
| --- | --- |
| `bash .pi/extensions/tests/run-smoke.sh` | **SMOKE OK**（152 条 check，含新增 21 条；既有 131 条全绿） |
| `bash .pi/extensions/tests/run-harness-smoke.sh` | **SMOKE OK**（12 个套件全绿：constraint-injection / harness-log / failure-classify / policy-decision / spec-gate / spill / test-case-gate / ui-design-gate / quality-gate ×2 等） |
| `grep -nE '^let (currentMode\|modeBoundChange\|turnBindLocked\|recentInputs\|jitDocHits\|keywordDocHits)' .pi/extensions/constraint-injection.ts` | 零命中 ✅ |
| `openspec validate per-session-constraint-binding` | valid |
| `bash scripts/scenario-trace.sh openspec/changes/per-session-constraint-binding` | 通过（8 个 Scenario 映射齐全，自动测试 8 / 人工 0） |
| `bash scripts/check-standards.sh` | 通过 161 / 失败 0 |
| `bash scripts/doc-impact.sh verify openspec/changes/per-session-constraint-binding` | 通过（声明 none、0 文件） |

调试过程说明（可追溯）：新增用例首轮 1 项失败（19.3），定位为**我用例断言写错**——未激活档头部是
「活跃变更：无」，被 `!includes('活跃变更：')` 误判；经临时 debug 打印实际注入内容确认实现语义正确（未激活 + 仅 idx.md），
改为 `includes('活跃变更：无')` 后全绿。实现代码未因此改动。

---

## 5. 与 design/tasks 的偏差

1. **§1.2 回退未启用**（1.1 走通）——见 §1。
2. **新增 2 个导出测试观测面**（`sessionStateKeysForTest` / `SESSION_STATE_LIMIT_FOR_TEST`）：design D2 的 API 清单未列。
   理由：有界性（上限 + LRU）是 spec Scenario「会话条目有界淘汰」的可断言面，而内部 Map 不应外泄可变引用 → 只导只读快照。
3. **`stableSnapshotKey` 保持不带会话维度**：design D3/“快照 key 含会话维度”一条由**存储层**（`state.channel`）实现，
   函数签名不动（它是既有导出纯函数，被烟测与语义检查引用）；效果等价——他会话绑定变化不再影响本会话快照（19.2 断言字节恒定）。
4. **`fork` 语义按 design D3 表实现**（可证则继承）；既有烟测无 fork 用例，故无回归冲突。
5. **`pin.read` 去重 key 简化**（去 sessionId 前缀）：集合已按会话隔离；既有断言（同会话 2 条、二次注入不重复）仍绿。
6. **`realSessionId(state)` 取代 `ctx.sessionManager?.getSessionId?.()`**：兜底槽（无 sessionId 语境）不再写事实库，
   与既有「REPO_ROOT 无 sessionManager → 不写真实事件库」的烟测约定一致（既有断言全绿）。

---

## 6. 未做 / 阻塞 / 残留风险

**未做（主线程跟进）**：§8.6 部署后事实库口径核对（观测窗口内 `constraint.inject.change` 必须等于该会话自身最近
`mode.set.boundChange`，无 mode.set 的会话必须为 null）；§8.7 完工汇报（重启 pi 会话使扩展生效；旧会话状态不迁移）。

**残留风险**：

1. **父会话识别依赖 pi 内部契约**：`parentSession` 若在将来 pi 版本改名/移除，子线程继承会静默退化为「未激活」
   （而非借用他人 —— 失败方向安全，但 apply 场景会缺约束块）。缓解：烟测 19.4 会在契约变化时…（不会失败，
   因为它 stub 了 header）；**建议主线程在部署后按 §8.6 观测子线程是否出现 `source=inherit`**，作为真实链路契约探针。
2. **`.pi/extensions` 无类型检查**：harness 只用 esbuild bundle（不做类型检查），本次改动的类型面（`ExtCtx.getHeader`、
   `SessionState` 字段）无机器校验；缓解：smoke 覆盖全部改动路径，且 `state.*` 访问在 bundle 阶段若拼错会抛运行期错误被烟测捕获。
3. **同 sessionId 多视图共享状态**是有意语义（19.7 断言）；若用户期望「同一会话两个 pane 各自独立」，需另立需求。
4. **LRU 淘汰可能淘汰仍在用的会话**（上限 32；仅淘汰最久未用）。极端场景（>32 个并发会话）下被淘汰会话下次事件会
   重建为空 → 若它当时是 implementation 档，会走 startup/recover 之外的路径（事件非 session_start 时不恢复）→
   表现为「注入掉回未激活」。风险低（>32 并发会话非常态），但已如实记录。
5. **本机 go 工具链在门禁中不可达**：本轮 harness 质量门禁报 `golangci-lint/go: command not found`（PATH/环境问题，非本
   change 引入——本 change 零 Go 改动）。已如实记录，未做处理。
