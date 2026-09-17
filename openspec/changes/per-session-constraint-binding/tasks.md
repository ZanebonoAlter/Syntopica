# Tasks — per-session-constraint-binding

> 实现契约：`proposal.md` + `design.md`（D1-D6）+ `specs/constraint-injection/spec.md`（会话作用域状态隔离）；
> 用例账本：`test-cases.md`。改动集中在 `.pi/extensions/constraint-injection.ts` + 其 smoke 用例。

## 1. 前置实测（父会话识别手段，决定 D3 走哪条路）

- [x] 1.1 实测 （**结论：路径 1 走通** —— session header 带 `parentSession` = 父会话文件路径；`ReadonlySessionManager` 暴露 `getHeader()`；实测本会话归档下 6 个 fork/子线程会话文件 header 全部携带该字段，形如 `…/2026-09-16T17-11-04-584Z_01a0ab33-….jsonl`。子会话 id 与父 id 均可从文件名反解 → 无需回退方案） pi-subagents 子线程可见的上下文：在 `session_start` / `before_agent_start` / `tool_execution_start` 里打印 `ctx.sessionManager` 可用字段（sessionId、sessionFile/父路径、parentSessionId 等），确认能否从子线程反解父 sessionId（同目录 `forks/<ts>_<parentId>-*.jsonl` 命名）——结论写入 `apply-report.md` 的 design 决策节
- [x] 1.2 若 1.1 拿不到父标识 （**未启用**：1.1 走通，父子关系由 `parentSession` 证明；回退方案保留为将来 pi 变更时的备选）：启用「派发即登记」回退——`tool_execution_start` 且 `toolName === "subagent"` 时，把当前会话 state 登记为待继承槽位，子会话 `startup` 认领（仅认领一次）；两条路径二选一落定后再动手改状态容器

## 2. 会话状态容器与访问器

- [x] 2.1 新增 `type SessionState` （`SessionState` + `ChannelState` + `sessionStates` + `FALLBACK_SESSION_KEY`/`SESSION_STATE_LIMIT`）（字段见 design D2：mode / boundChange / turnBindLocked / recentInputs / jitDocHits / keywordDocHits / pinReadSeen / channel / lastUsedAt）+ `const states = new Map<string, SessionState>()` + `FALLBACK_KEY`
- [x] 2.2 `stateFor(ctx)` （唯一入口；含 `sessionKey`/`evictSessionStates`/`realSessionId`/`parentSessionId`/`inheritFromParent`/`resetSessionState`）：唯一读写入口；`sessionId` 缺失 → `FALLBACK_KEY`；每次取用刷新 `lastUsedAt`
- [x] 2.3 条目上限 + LRU （默认 32，`cfg.sessionStateLimit` 可覆盖——轮次 2 补实现 `resolveSessionStateLimit`，非法值回退默认；淘汰不做额外副作用。已知边界（review F2/F3）：上限触顶可能把父会话条目挤掉 → 子线程继承退化为未激活（方向安全），建议 ≥ 并发会话数；§8.6 观测需排除被 LRU 淘汰的会话（假阳性））；`session_start` 的 new/resume/fork/reload 只重置本会话条目
- [x] 2.4 把既有 `resetChannelState()` （`pinReadSeen`/快照/指纹/`pendingSnapshot` 收进 `SessionState`；`resetChannelState` 由 `resetSessionState` 取代） / `pinReadSeen` / 稳定层快照与指纹 / `pendingSnapshot` 收进 `SessionState.channel`

## 3. 读写点迁移（逐处改为走 `stateFor(ctx)`）

- [x] 3.1 `session_start` （startup：本会话无状态时先试父会话继承 → 再按同 sessionId 恢复；new 只清本会话；fork 改为「父子可证则继承」）（`constraint-injection.ts:1544-1589`）：startup 恢复改读本会话条目；`new` 只清本会话；`resume`/`reload` 清本会话后按同 sessionId 恢复；**fork/子线程**走显式继承（`source=inherit` 记账）
- [x] 3.2 `input` （入窗/命令绑定/记账全走本会话 state）（`:1591-1624`）：`recentInputs` 入窗 + 命令绑定 + `mode.set source=command` 全部写本会话 state
- [x] 3.3 `tool_execution_start` （skill 绑定、JIT 命中集、绑定修正的 `currentBound`/锁、JIT 即时投递全走本会话 state）（`:1626-1701`）：skill 绑定、JIT 命中集、绑定修正条件化（`resolveBindAction` 的 `currentBound` 取本会话）、JIT 即时投递的 plan/指纹取本会话
- [x] 3.4 turn 收尾路径 （`state.turnBindLocked` 复位、绑定健康检查、mtime fallback 记账）（`:1704-1760`）：`turnBindLocked` 复位、绑定健康检查、mtime fallback（`source=fallback`）全部按会话
- [x] 3.5 `before_agent_start` （快照/指纹/pin 去重全走本会话 `channel`；`planInjection` 改为接 `SessionState`；`declaration` 域选择读本会话 `boundChange`） / 注入计划（`:1780-1810`）：稳定层快照/指纹/pin 去重收进本会话 `channel`（`stableSnapshotKey` 本身不含会话维度，隔离由 per-session 存储保证，见 apply-report 偏差 3）；`declaration` 域选择读本会话 `boundChange`
- [x] 3.6 `session_compact` （`stateFor(ctx).channel.pendingSnapshot = true`）：`pendingSnapshot` 落本会话 channel
- [x] 3.7 全文件清理 （grep `^let (currentMode|modeBoundChange|turnBindLocked|recentInputs|jitDocHits|keywordDocHits)` → 零命中；另同步 `matchKeywordDocs`/`deliverDynamic` 签名）：确认 `grep -nE '^let (currentMode|modeBoundChange|turnBindLocked|recentInputs|jitDocHits|keywordDocHits)'` **零命中**
- [x] 3.8 轮次 4（真实链路实测：pi-subagents 子线程是**独立 node 进程**，内存继承永不命中）：`parentSessionId` 双来源（`getHeader().parentSession` → `getSessionFile()` 的 `forks/` 路径反解父 id）；新增 `inheritFromParentHistory`——父 id 可证但内存无父条目时，从事实库查父会话最近一条可恢复 `mode.set` 采纳档位（记 `source=inherit`，只采纳 mode+boundChange，不伪造命中集/快照）；startup/fork 分支改 `inheritFromParent(state, ctx) || inheritFromParentHistory(state, ctx)`

## 4. smoke 用例（`.pi/extensions/tests/constraint-injection.smoke.cjs`）

- [x] 4.1 两会话交叉绑定互不污染 （smoke 19.1，4 条断言）（含「A 绑 → B 绑 → A 注入」精确交错序列）
- [x] 4.2 他会话绑定变化不刷新本会话稳定层 （smoke 19.2：快照字节恒定——等价于无稳定层重建/无新增稳定层记账）
- [x] 4.3 无自身绑定且无父子关系 （smoke 19.3 + 19.4b「父会话不可证」两条） → 仅索引、归因未绑定、无他人域约束
- [x] 4.4 子线程显式继承父会话 （smoke 19.4：继承档位/绑定/声明域 + `source=inherit` 记账 + 主会话不被清零）（完整约束块 + `mode.set source=inherit` + 主会话不被清零）
- [x] 4.5 命中集按会话隔离 （smoke 19.5：关键词 + JIT 各一例，断言他会话零投递）（JIT + 关键词各一例）
- [x] 4.6 turn 绑定锁按会话隔离 （smoke 19.6）
- [x] 4.7 LRU 淘汰 + 上限边界 （smoke 19.8：上限 32 + 超建 4 个，断言不超上限且最久未用被淘汰）（含上限=1）
- [x] 4.8 无 sessionId 兜底槽位 （smoke 19.7：兜底槽自洽 + 同 sessionId 多视图共享状态）行为等价 + 同 sessionId 多视图共享 state
- [x] 4.9 轮次 4 跨进程继承用例 （smoke 19.10 事实库回退继承×3 / 19.11 路径兜底×1 / 19.12 两证皆无→不继承；反向验证：短路回退分支恰 4 红，恢复后 sha256 一致）

## 5. 编码实现（子线程执行）

- [x] 5.1 按 §2-§4 落地改动 （单文件 `constraint-injection.ts`；会话作用域/不借用他会话的不变量写进模块注释与函数注释）（单文件为主 + smoke 用例），中文注释说明「会话作用域 + 不借用他会话」不变量
- [x] 5.2 `bash .pi/extensions/tests/run-harness-smoke.sh` 全绿 （实测 SMOKE OK，12 套件全绿）（含全部新增与既有用例）

## 6. 测试

- [x] 6.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → SMOKE OK（12 套件，多次独立复跑）
- [x] 6.2 `bash .pi/extensions/tests/run-smoke.sh` → SMOKE OK（最终 189 项 check；轮次 1 新增第 19 组隔离族 21 条，轮次 2 补判别力与上限用例，轮次 4 补 19.10-19.12 跨进程继承 5 条）
- [x] 6.3 `grep -nE '^let (currentMode → 零命中（实测 ✅）|modeBoundChange|turnBindLocked|recentInputs|jitDocHits|keywordDocHits)' .pi/extensions/constraint-injection.ts` → 零命中
- [x] 6.4 既有 `constraint-injection` 全部 Scenario （回归绿：子线程派发不清零、turn 锁定、reload/resume 恢复、稳定层恒定、legacy 通道等全部 ✅）（尤其子线程继承、turn 锁定、reload/resume 恢复、稳定层恒定）回归绿
- [x] 6.5 非空性实测（轮次 3）：探针 P1（`stateFor` 退回单例全局）→ **21 条变红**（19.1/19.2/19.3 各精确命中 + 同族连带）；探针 P2（`inheritFromParent` 短路）→ **4 条变红**（继承族）；恢复后 sha256 与 diff 字节级一致、双门禁全绿、无残留（apply-report-round3）

## 7. 文档

<!-- doc-impact: none(纯 harness 工具链：改动仅在 .pi/extensions/constraint-injection.ts + 其 smoke 用例；harness 规则文档 AGENTS.md 与 .agents/skills/harness-facts/SKILL.md 当刻被其他 active change 脏改（并发工作树），本 change 不动它们以避免冲突，改由扩展内模块注释 + 本 change 归档记录承载不变量) -->

- [x] 7.1 `constraint-injection.ts` 顶部模块注释 （新增「职责 0. 会话作用域状态」段：隔离不变量 + 继承规则 + 有界性 + 回归面指引）补充「会话作用域状态 + 父子继承 + 不借用他会话」不变量（源码即 harness 文档）
- [x] 7.2 `apply-report.md` 记录 design 决策 （见本 change 的 apply-report：§1 结论 + 逐节点改动 + 原始门禁结果）（父会话识别走 1.1 还是 1.2 路径）与偏移
- [x] 7.3 flow/standard/database 活文档**无变更** （实测 `doc-impact.sh verify` 通过：声明 none、0 文件）（本 change 不改业务行为与数据结构）；`bash scripts/doc-impact.sh verify <change>` 通过（如需 excuse 按门禁提示补 `<!-- doc-impact-excuse -->`）

## 8. 验证

| Scenario | 测试文件 |
| --- | --- |
| 两会话交叉绑定互不污染 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 他会话绑定变化不刷新本会话稳定层 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无自身绑定且无父子关系时不借用他会话 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 子线程显式继承父会话 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 命中集按会话隔离 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| turn 绑定锁按会话隔离 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 会话条目有界淘汰 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 无 sessionId 语境行为等价 | .pi/extensions/tests/constraint-injection.smoke.cjs |
| 人工：部署后真实多会话无串味（事实库口径） | 人工：观察窗口内 `constraint.inject.change` 与各会话自身最近 `mode.set.boundChange` 逐条一致（0 例不符） |
| 人工：真实链路子线程继承成立 | 人工：派子线程后查事实库出现 `mode.set source=inherit` 且 boundChange = 主会话 |

- [x] 8.1 `bash .pi/extensions/tests/run-harness-smoke.sh` → SMOKE OK → 0 失败
- [x] 8.2 `bash .pi/extensions/tests/run-smoke.sh` → SMOKE OK → 0 失败
- [x] 8.3 `openspec validate per-session-constraint-binding` → valid（实测） → valid
- [x] 8.4 `bash scripts/scenario-trace.sh → 8/8 映射齐全（实测退出码 0） openspec/changes/per-session-constraint-binding` → 退出码 0
- [x] 8.5 `bash scripts/check-standards.sh` → 161/161 通过；doc-impact verify 通过（实测） + `bash scripts/doc-impact.sh verify openspec/changes/per-session-constraint-binding` → 通过
- [x] 8.6 事实库口径核对（部署后）：**Before**（2026-09-17 重启前，近 3h）——注入归属与该会话自身最后 `mode.set.boundChange` 不符的会话 **5/5**（`01a0aaf4` 自身零绑定却被注入 `dedupe-rss-articles`×3 + `failover-dead-proxy-to-direct`×2；`01a0aca4` 自身绑 `failover-...` 却被注 `harness-retro-loop`）。**After**（重启后 00:53:41 起，扩展已重载）——本会话注入仅 `reason=index` + `change=None`（未绑定→仅索引，不再借用任何人）；`01a0aca4` 以 `source=recover` 恢复自己的 change；判据「归属≠自身绑定」计数 **0**（持续观察中）。口径注意（review F3）：被 LRU 淘汰后又重建的会话会出现假阳性，观测前先确认窗口内无淘汰；`source=inherit` 真实链路探针待下一次 fork 派发自然验证（fresh-context 子会话无 `parentSession`，按 spec 正确地不继承）
- [x] 8.7 完工汇报含「部署后影响 + 需要的操作」：扩展已随 pi 重启重载生效；旧会话状态不迁移（重启后按同 sessionId 的 `mode.set` 历史 recover，无历史则未激活）；后续真实多会话使用中可随时用 8.6 的判据复查
