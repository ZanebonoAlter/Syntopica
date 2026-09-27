
## 指纹重复投递根因：延迟快照重建的无条件指纹清空

constraint-injection 动态层重复投递根因（2026-09-17 events.db 实证，session 01a0ae99 事件 29643-29652）：

【排除嫌疑】非 SessionState 实例分裂：stateFor(constraint-injection.ts:247) 单 Map 按 ctx.sessionManager.getSessionId() 键控；logModeSet(:1363)/logConstraintInjects(:1673) 对 undefined sessionId 一律跳过，涉案事件全记在同一真实 session_id 下 → 各钩子同一 state。另排除 compact-resend（29647 payload 无 source 字段）。

【真实机制】延迟快照重建 × 无条件指纹清空：
1. 会话启动 before_agent_start(:2021起) 以未激活档建快照 key="none|none"（29643）；
2. turn 中途 09:31:52 tool_execution_start 的 skill 分支(:1967-1982, matchSkillPath) 设 mode=requirements 并记 mode.set source=skill——source=skill 只能产自该分支（input 钩子只有 matchCommand→source=command），即档位激活发生在 turn 中途，该 turn 的 before_agent_start 早已以 mode=null 跑完（快照命中，零记账）；
3. 同 turn 09:32:36 JIT 分支(:1974-1990)→deliverDynamic(:1703) 投递 test-design 并写 sentFingerprint（指纹此时已是新 regime "requirements|none" 下的事实）（29645）；
4. 下一 turn 09:45:02 before_agent_start：key="requirements|none"≠快照 key → isTransition(:2039) → 无条件 sentFingerprint=new Map()(:2044) → 已投递指纹被清 → computeDynamicDiff(:1535) 判增量 → 原样重投（29647；29646 mode-base 重建记账）。:2038 注释「首次建快照不重置」护栏只覆盖快照为 null 情形。
同族变体：inheritFromParent(:318-337) 拷贝父 channel，父快照 key 陈旧时 fork/子线程继承方首 turn 同样触发无条件清空（01a0acae 实证：00:04:51 首投 → 00:16:58 全量重投）。

【修复方案（已立项 change fix-injection-transition-fingerprint-wipe，planning 完成）】指纹按 regime 标记：ChannelState(:165-171) 增 fingerprintRegime（投递时 stableSnapshotKey(mode,boundChange)，:1500）；deliverDynamic 写指纹时同步记 regime；before_agent_start 清空条件收紧为 isTransition && fingerprintRegime!==key（:2044）；newSessionState/resetSessionState 初始化 null；inheritFromParent 拷贝带上。真实 regime 切换（edit-dir 绑定后下一 turn，09:48:14 案例）仍全量重投=设计行为保持；compact force 重发不看 regime 不变。

【验证要点】smoke stub 会话（ctx 无 sessionManager → FALLBACK 槽）复刻序列：before_agent_start(未激活) → tool_execution_start read skill 路径 → tool_execution_start edit 命中 doc-impact-applies → before_agent_start：断言稳定层重建但 dynamicText() 为空；反向护栏：edit-dir 绑定后下一 turn 仍全量重投。复验 SQL（代边界=session.start+mode.set，HAVING c>1）：9/16-9/17 基线 ~19 组重复，部署后应归零。

<!-- pinned 2026-09-17T12:27:01Z -->

## smoke 动态层断言姿势：systemPromptOnly + dynamicText，勿用 systemPrompt（会消费）

constraint-injection smoke 测试（.pi/extensions/tests/constraint-injection.smoke.cjs）两个易踩陷阱（fix-injection-transition-fingerprint-wipe apply 阶段实测）：

【陷阱1：systemPrompt() 会消费动态层消息】smoke 的 systemPrompt(c) helper 在返回 sp+dyn 拼接后立即 msgCursor=messages.length（消费）。之后断言 dynamicText() 恒为空串 → 「零重投」类断言恒绿、无判别力。正确姿势：断言动态层有/无内容时必须用 systemPromptOnly(c)（只返回 system prompt、不消费）+ dynamicText() 检查，检查完 markMessages()。已有 19.5 组就是这个写法。

【陷阱2：tmp 场景的档位选择与 mtime 兜底】tmp 场景（makeTempScenario）的 constraint-injection.json 默认无 skillSignals 字段 → tool_execution_start 的 matchSkillPath 永远返回 null，skill 路径读取无法激活档位；需在配置里显式加 skillSignals:{requirements:[...],implementation:[...]}（匹配规则 path.includes('/skills/<name>/')）。且 before_agent_start 对 implementation 档有 mtime 兜底绑定（plan.changeName → source=fallback 记账）→ tmp 下多个 fixture change 会让快照 key 变成 implementation|<mtime最新>，指纹 regime 场景断言会被污染；mid-turn 切档/指纹 regime 类场景必须用 requirements 档（无 mtime 兜底）。另：change 级文件（explore-findings/词汇表）仅在 implementation 档注入（planInjection 的 mode==="implementation" 分支），requirements 档断言 change-file 内容必挂。

【回归验证】本 change 的 regime 组断言判别力已做变异验证：把 before_agent_start 清空条件改回无条件 isTransition → 场景 A/C 红；改成永不清空 → 场景 B 红。

**引用**：.pi/extensions/tests/constraint-injection.smoke.cjs:systemPrompt、.pi/extensions/constraint-injection.ts:before_agent_start、.pi/extensions/constraint-injection.ts:planInjection

<!-- pinned 2026-09-17T14:16:37Z -->
