
## 子线程投递通道实测：before_agent_start 在 pi-web SDK 不存在，改走 session_start+steer

## V1/H1 探针实测结论（2026-09-18，实现期设计修订依据）

**子线程扩展加载 ≠ 约束可达**：implementer 档（load_extensions:true）派发的子线程里，扩展确实运行（session_start 继承 ✓、turn_end rollup/steer ✓、dev-process-guard 提醒可达 ✓），但 before_agent_start 通道完全不存在——零 constraint.inject、关键词探针转录零注入条目。

**根因**：pi-web bundle（.next/server/chunks 全量 grep）的 SDK 事件词汇表**没有 before_agent_start / session_compact**；有 session_start / tool_execution_start / turn_end / input。`logConstraintInjects` 在 before_agent_start return 之前执行，记账缺失 ⇒ 事件未 fire，非「return 被忽略」。

**已验证可用通道（子线程）**：session_start（mode.set inherit 实证）、turn_end（rollup + policy.decision 落点 + dev-process-guard steer 实证）、tool_execution_start（JIT，pi-web 词汇表内）。**不可用**：before_agent_start（稳定层 + keyword turn 起点投递）、session_compact。

**修订设计（design.md D3 rev）**：子线程稳定层改走 session_start 末尾 pi.sendMessage({customType:"constraint-injection"}, {deliverAs:"steer", triggerTurn:false})（compact 重发/JIT 同款机制），记 constraint.inject source=child-init + 置 stableSnapshot；before_agent_start 见快照已存在则子会话不再返回 systemPrompt 块（防双投）。

**生效前提**：pi-web server（pid 139853，2026-09-17 21:47 启动）持有旧扩展快照——证据：探针子线程在子代理编辑（18:31）之后派发，但新 quality-gate 降载未生效（零 policy.decision child-session）而旧能力（inherit）正常 ⇒ 扩展模块按 server 启动缓存。重启后新代码生效；重启终止其托管的所有会话（含本会话与并发 change 子线程 01a0b314），时机用户拍板。

**附带实测**：quality-gate 降载代码已落地且行为 smoke J/K 全绿（.pi/extensions/lib/child-session.ts + quality-gate.ts:235 入口短路）；SMOKE 全量 exit=0。lib/policy-decision.ts 的 reasonCode 校验是 kebab-case 形状级（无 per-policy 枚举数组），child-session 直接合法通过；词表登记落 SKILL.md 文档层。

<!-- pinned 2026-09-18T11:03:36Z -->
