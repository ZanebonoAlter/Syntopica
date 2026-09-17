# Design — fix-injection-transition-fingerprint-wipe

## 1. 根因（证据闭合，与初始嫌疑的差异说明）

**结论：不是 SessionState 实例分裂**。三条反证：

1. `stateFor`（constraint-injection.ts:247）经 `sessionKey`（:216-219）以 `ctx.sessionManager.getSessionId()` 为唯一键，`sessionStates` 是单 Map；pi 的 `ExtensionContext.sessionManager` 是 runner 上的 getter（同 runner 同实例），且 `logModeSet`（:1363）/`logConstraintInjects`（:1673）对 `realSessionId === undefined` 一律跳过——涉案全部事件（29644/29645/29646/29647）都记在同一真实 session_id 下，证明各钩子解析到的是**同一个** state 条目；
2. 排除「input 钩子先设档、当回合快照应重建却没建」：mode.set 29644 的 `source=skill` **只能**产自 `tool_execution_start` 钩子的 skill 分支（:1967-1982，input 钩子只有 `matchCommand` → source=command）——即档位激活发生在 **turn 中途**（agent read skill 文件），该 turn 的 `before_agent_start` 早已以 mode=null 跑完（快照 key `none|none` 命中，零记账，与事件空洞吻合）；
3. 排除 compact-resend：29647 的 payload 无 `source` 字段（force 重发必带 `compact-resend`）。

**真实机制（延迟快照重建 × 无条件指纹清空）**：

- 会话启动时 `before_agent_start`（:2021 起）以未激活档建快照 key=`none|none`（29643 index 记账）；
- turn N 中途 09:31:52：`tool_execution_start` skill 分支设 `state.mode=requirements`（快照**不重建**——重建只发生在 turn 起点钩子）；
- 同 turn 09:32:36：JIT 分支（:1974-1990）`deliverDynamic`（:1703）投递 test-design 并写 `sentFingerprint`——**此时指纹已是新 regime（`requirements|none`）下的事实**；
- turn N+1 09:45:02：`before_agent_start` 算得 key=`requirements|none` ≠ 快照 key=`none|none` → `isTransition=true`（:2039-2041）→ **无条件 `sentFingerprint = new Map()`（:2044）** → 该 turn 已投递的 test-design 指纹被清 → `computeDynamicDiff`（:1535）判为增量 → 原样重投（29646 mode-base 重建记账 + 29647 重投，间隔 9ms，与「先 stable 记账后 dynamic 投递」的代码顺序吻合）。

即：**指纹清空以「快照 key 变化」为触发，但档位/绑定可在 turn 中途变化、快照重建延迟到下一 turn 起点——中间窗口内以新 regime 投递的指纹被延迟重建误判为旧 regime 残留而清空**。:2038 注释的「首次建快照（如 JIT 已在快照前投递过）不重置」护栏只覆盖快照为 null 的情形，覆盖不了「快照存在但 key 陈旧」的本案。

**同族变体（fork/子线程，01a0acae 实证）**：`inheritFromParent`（:318-337）拷贝父会话 channel——若父会话此刻快照 key 陈旧（mid-turn 切档后未及重建）而指纹已是新 regime，继承方首个 turn 的快照重建同样触发无条件清空 → 父会话已投递条目重发，违背「父会话已投递过的动态层条目不该重发」（:332 注释）的 fork 语义。

影响面：用户复验 SQL（代边界切掉 session.start/mode.set）在 9/16-9/17 命中 ~19 组代内重复；9/16 前数千组为混合通道改造前的旧行为，不计入。

## 2. 修法：指纹按 regime 标记，清空条件收紧

单一改动轴：**指纹的清空判定从「快照 key 变了」改为「指纹投递时 regime ≠ 新快照 key」**。

1. `ChannelState`（:165-171）新增 `fingerprintRegime: string | null`——当前 `sentFingerprint` 各条目投递时的 `stableSnapshotKey(state.mode, state.boundChange)`（:1500）；
2. `newSessionState`（:210）/`resetSessionState`（:364）初始化为 `null`；
3. `deliverDynamic`（:1703）写指纹时同步 `state.channel.fingerprintRegime = stableSnapshotKey(state.mode, state.boundChange)`（仅在确有投递时写，零投递不动）；
4. `before_agent_start` 清空条件（:2044）改为 `if (isTransition && state.channel.fingerprintRegime !== key)`；isTransition 本身与快照重建逻辑不变（稳定层仍按新档位重建、mode-base 一次性重记账——这是正确行为，稳定层内容确实变了）；
5. `inheritFromParent`（:333）拷贝 channel 时带上 `fingerprintRegime`。

**行为矩阵**（修复后）：

| 场景 | 指纹 regime vs 新 key | 结果 | 与现行为关系 |
| --- | --- | --- | --- |
| mid-turn 切档 + 同 turn JIT 投递（本案 09:45:02） | 相同 | 不清空，零重投 | 修复目标 |
| fork 继承陈旧快照 key（01a0acae） | 相同 | 不清空，父已投递不重发 | 恢复 fork 语义 |
| 真实绑定切换（09:48:14，edit-dir 后下一 turn） | 不同 | 清空，全量重投 | 设计行为保持 |
| 未激活→激活（input 命令路径，同 turn 起点即重建） | 不同 | 清空，全量重投 | 行为不变 |
| 档位回落（change 归档 → mode=null） | 不同 | 清空 | 行为不变 |
| compact force 重发 | 不看 regime | 无视指纹重发 | 行为不变 |

## 3. 备选方案与否决理由

- **B1（mid-turn 切档时立刻清指纹 + 置快照为待重建）**：切档点（tool_execution_start skill/edit-dir 分支）就地 `sentFingerprint = new Map()` 并将 `stableSnapshot` 置空。否决：置空快照会让下一 turn 走「首次建快照」分支（isTransition=false）→ **绑定切换的全量重投语义被破坏**（09:48:14 设计内行为）；只清指纹不置空快照则与现状同样漏（清完之后同 turn 的 JIT 投递又会写入，下一 turn 仍被 isTransition 清掉）。
- **B2（isTransition 清空时逐条比对内容哈希，只清「内容已不在新计划」的条目）**：语义上最细，但把「regime 切换全量重投」降级为「内容级 diff」，改变了主 spec 明文「档位切换时动态层全量集合重置」的设计意图（新 change 语境下即使内容相同的条目也要重新出现在模型近期上下文），否决。
- **B3（selected：regime 标记）**：改动最小（5 处、纯内存），完整保留设计内重投语义，只消灭「regime 未变却被清」的错误清空。

## 4. fail-safe 与隔离不变式

- fail-safe 保持：改动全部是进程内字符串拼接与 Map 赋值，无新 I/O / 新异常面；清空条件收窄只可能**减少**投递（退化方向 = 少注入提示，与既有「指纹机制退化时宁可不投递」一致）。`stableSnapshotKey` 是纯字符串模板，不抛错。
- per-session-constraint-binding 隔离不变式（头注释 160-166）：本修复只读写**本会话** state 的 channel 字段；`inheritFromParent` 拷贝是既有显式继承路径（父子可证才走），不新增任何跨会话读取。`sessionStateKeysForTest` 观测面不变。
- 记账语义不变：`constraint.inject` 仍「送达时记」；修复后同代内不再出现同节同字节的重复记账（这是验收信号之一）。

## 5. 回检方案（可回检指标 + 观察窗口）

- **指标**：用户复验 SQL（`events.db`，kind∈{constraint.inject, session.start, mode.set}，ts≥部署日，代边界=session.start+mode.set，HAVING c>1）的代内重复组数。基线（2026-09-16~17）≈19 组/2天。
- **判据**：修复部署后观察窗口 ≥3 个工作日，代内重复组数应归零（绑定切换/compact 边界已被 SQL 切代逻辑排除；若仍有残留，逐组核对是否为未覆盖的第三变体）。
- **基线留档**：部署前跑 `bash scripts/harness-retro.sh --save-baseline` 可选；本指标也可直接用上述 SQL 手工回检（harness-retro 报告不直接聚合该维度）。
- **smoke 回归**：`.pi/extensions/tests/run-smoke.sh` 全绿（新增两个场景断言，见 test-cases.md）。

## 6. 测试策略（详见 test-cases.md）

- 主链路：stub 会话复刻 09:31:52→09:48:14 真实事件序列（未激活 turn → mid-turn skill 读取 → 同 turn JIT 投递 → 下一 turn 快照重建），断言第二次投递零发生；
- 反向护栏：绑定切换（regime 真变）仍全量重投，防止修复过窄变过宽；
- fork 继承变体：继承后首 turn 不因陈旧快照 key 重投。
