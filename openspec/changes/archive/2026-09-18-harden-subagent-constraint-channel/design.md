# Design — harden-subagent-constraint-channel

> 2026-09-18 实现期修订：D3 按 V1/H1 探针实测重写（原「预期零实现」判断被推翻）。
> 研究底稿：`docs/research/subagent-constraint-gap/explore-findings.md`（根因证据链 + 通道对比）。

## Context

根因证据链（`docs/research/subagent-constraint-gap/explore-findings.md`，2026-09-18）：

- 本仓库子线程主力通道是 **pi-web Agent 工具**（转录 custom 标记 `pi-web:subagent`），其 agent profile 写死 `loadExtensions:false`（`load_extensions` 未声明时默认 false）→ 子会话 `noExtensions:true`，连 AGENTS.md 都不自动带（`noContextFiles` 恒真）。
- 实测样本：后台子线程 `01a0b314`（「实现后端聚合服务与端点」）在 events.db 零事件、转录零注入痕迹，其 WIP 代码欠账（lint 中间态、测试红）堆在共享工作树上由主会话 turn_end 门禁兜出。
- pi-subagents 通道不同：后台子线程（独立 runner 进程）默认加载 ambient 扩展，继承链路 2026-09-17 已实测；前台默认不加载但可 frontmatter `extensions:` 显式挂载。

**2026-09-18 V1/H1 探针补充实测（implementer 档上线后）**：子线程扩展加载后——继承链路 ✓（`mode.set source=inherit` 落库）、turn_end 通道 ✓（rollup / dev-process-guard steer 可达）、**before_agent_start 通道 ✗（零 `constraint.inject`、关键词转录零注入条目）**。根因：pi-web bundle 的 SDK 事件词汇表**没有 `before_agent_start` / `session_compact`**（编译码零命中；有 session_start / tool_execution_start / turn_end / input）。`logConstraintInjects` 在 before_agent_start return 之前执行，事件缺失 ⇒ 事件根本没 fire——稳定层 system prompt 与 turn 起点动态层从 SDK 层就到不了子模型。

## Goals / Non-Goals

**Goals:**

- 实现类子线程（§0.6 步骤3 派发的实现/验证任务）harness 约束可达：注入 + 继承 + 记账
- 子线程门禁降载：多子线程并发不叠加重命令负载（4 核树莓派红线）
- 通道行为矩阵文档化：已知限制显式登记，不再靠口口相传

**Non-Goals:**

- 不改 pi-web 包产物（npm 编译产物，改了升级即丢）
- 不解决 AGENTS.md 不自动进子线程的问题（pi-web `noContextFiles` 恒真，包为硬编码；靠派发任务文本兜底，矩阵登记）
- 不改 pi-subagents 通道行为（后台默认已加载，前台归编排纪律管）
- 不做嵌套派发治理（子线程派子线程当前无场景）
- 不在子线程恢复 keyword 命中通道（匹配逻辑在 before_agent_start 内，pi-web 子会话该事件不存在；JIT 通道 tool_execution_start 可用，够用）

## Decisions

### D1 通道裁决：路线乙（留 pi-web，用户拍板 2026-09-18）

关 pi-web 切 pi-subagents 的路线被否：quota-gate 与 harness-telemetry 均只拦字面名 `Agent` 的工具（`quota-gate.ts:94`、`harness-telemetry.ts:250/259`），切通道要改两处接线且派发记账断流；A+C 路线周边设施零回归。代价是依赖 pi-web profile 白名单字段——官方字段（`load_extensions` 在解析白名单中），升级漂移风险低，验证节实测兜底。

### D2 配置载体：项目侧 profile `.pi/agents/implementer.md`

- pi-web 从三个目录扫 profile：全局 `~/.pi/agent/agents/`、workspace `.agents/agents/`、**项目 `.pi/agents/`**；现有 `change-review-glm` 即被两通道共扫成功注册。新增项目侧文件即可，不动包。
- frontmatter 用 pi-web 解析白名单的 snake_case 字段：`description` / `tools` / `load_skills: false` / `load_extensions: true` / `inherit_context: false` / `enabled: true`。tools 取 pi-web 默认集（read/bash/edit/write/grep/find/ls）；model/thinking 留空，派发时按 §0.6 模型表传全称。
- **只读探索/计划任务继续用内置 explore/plan**（不带扩展，轻量现状不变）；`implementer` 只给实现/验证类派发用。
- 文件名 = profile 名（无包前缀的项目文件）：`implementer.md`。
- ✅ 已实测（2026-09-18 V1 探针）：`implementer` 档可被 Agent 工具解析派发，子线程扩展加载生效（constraint-injection / dev-process-guard 均在子线程活动）。

### D3 constraint-injection 侧：子线程投递通道 = session_start + sendMessage（实测修订）

**原判断「预期零实现」被实测推翻**。修订后机制：

- 子线程（isChildSession 为真且继承成功）在 **session_start 末尾**用 `pi.sendMessage({customType:"constraint-injection", content:稳定层快照}, {deliverAs:"steer", triggerTurn:false})` 投递稳定层——与 compact 重发 / JIT 同款机制，子线程已实测可达（dev-process-guard steer 先例）。随投递记 `constraint.inject`（payload 附 `source:"child-init"`）并置 `state.channel.stableSnapshot`。
- before_agent_start 若在完整 SDK 运行时（如 pi-subagents 后台 runner）触发：见到快照已存在走冻结路径不重复记账，且**子会话 SHOULD NOT 再返回 systemPrompt 块**（防双重投递——消息与 system prompt 二选一，session_start 已投递即由消息通道承载）。主会话投递路径不变。
- 动态层：JIT（tool_execution_start，pi-web 词汇表内有）照常；keyword 匹配在 before_agent_start 内 ⇒ 子线程不可用，登记已知限制。
- 事件词汇表证据：pi-web bundle 有 session_start / tool_execution_start / turn_end / input，**无 before_agent_start / session_compact**（编译码零命中）。

### D4 quality-gate 降载：子线程 turn_end 整体 bypass + 显式记账

- 判定：`isChildSession()` 纯函数（lib/child-session.ts，与 constraint-injection 同源证据），turn_end 入口最顶端短路（早于 touchedCode 早退）。
- 行为：子线程 MUST NOT 执行任何重命令，直接放行回合；记 `policy.decision {policy:"quality-gate", action:"bypass", reasonCode:"child-session"}` 一条；gate.check 零记账（没跑命令）。
- 正确性论据：共享工作树的增量门禁由主会话 turn_end 承担；子线程交付验收由 §0.6 步骤4/5 在主会话收口。
- 词表同步：SKILL.md reasonCode 枚举 + smoke 精确断言（lib/policy-decision.ts 的校验是 kebab-case 形状级，无 per-policy 枚举数组——`child-session` 形状合法直接通过，枚举登记落在 SKILL.md 文档层）。
- ✅ 代码已落地（2026-09-18），行为 smoke 场景 J/K 全绿；**生效依赖 server 重启（见 D7）**。

### D5 其余扩展子线程行为裁决表（全部维持现状）

| 扩展 | 子线程行为 | 理由 |
| --- | --- | --- |
| constraint-injection | 全量生效（本 change 目标，投递通道见 D3） | 注入+继承+记账 |
| quality-gate | bypass + 记账（D4） | 负载红线 |
| spec-gate | 现状 | 归档操作不应发生于子线程；误触发也无害 |
| quota-gate | 现状 | 子线程内无嵌套派发工具 |
| ui-design-gate / entry-gate / test-scope-guard | 现状 | 低频、上下文依赖主会话裁决 |
| tool-output-spill / harness-telemetry | 现状 | 无通道差异；telemetry 在子线程正常记账（rollup 实测可达） |
| dev-process-guard | 现状 | 实测子线程 turn_end 提醒可达；窗口归因在多会话并发下有「他线程进程落入本会话窗口」局限，登记矩阵 |

### D6 已知限制（矩阵文档登记，不在本 change 解决）

1. AGENTS.md 不自动进子线程（`noContextFiles` 恒真）→ 项目上下文靠派发任务文本（必读文件清单）。
2. 内置档与前台派发仍是盲区 → 编排纪律兜底（任务文本带红线摘要）。
3. keyword 命中通道在子线程不可用（匹配在 before_agent_start 内）→ JIT（编辑路径）通道可用。
4. pi-subagents 前台如需扩展 → agent 定义显式 `extensions:`，矩阵登记备用。

### D7 生效前提：pi-web server 重启（用户拍板时机）

pi-web server 进程（2026-09-17 21:47 启动）持有旧扩展快照：旧 SDK 无 before_agent_start（能力缺失，重启不改变）、旧 quality-gate 无降载（重启后生效）。**重启会终止其托管的全部会话**（含本会话与并发 change 的在跑子线程）——执行时机必须避开在跑实现子线程，由用户拍板。重启后 V1/V6 重测收口。

## Risk / Trade-offs

- **pi-web 升级漂移**（白名单字段改名/扫描目录变化/事件词汇表变化）：T2 实测 + 矩阵文档登记依赖点；漂移表现为「子线程退化无扩展」而非错误行为，可观测（事件库三事件消失）。
- **多子线程并发加载扩展**：每子线程独立加载 10 扩展——内存换约束；events.db 并发写由 WAL + busy_timeout 5000ms 既有机制覆盖。
- **降载掩盖子线程真回归**：主会话 turn_end 仍跑全量门禁（共享树），欠账不会消失只会晚一轮暴露；符合「主会话收口」模型。
- **session_start steer 时序**：steer 在首 turn 开始前入队，随首 turn 上下文送达——若实测发现时序异常（消息丢失/顺序错乱），回退方案为 input 事件时机投递（词汇表内有 input），实现复杂度相当。

## Test Strategy

- 纯函数 smoke：`isChildSession` 判定矩阵（header 有/无 parentSession × fork 路径 × stub 槽位）；policy.decision 形状精确断言（policy/action/reasonCode 白名单）。
- 行为 smoke：子线程 turn_end 零命令执行 + 一条 bypass 记账；主会话不受影响；session_start 投递路径（子线程 inherit 后 steer 发送 + 记账 + 快照置位；before_agent_start 冻结路径不双投）。
- 实测验证（人工，server 重启后）：V1 重派 `implementer` 探针 → 事件库验证 `session.start` + `constraint.inject(source=child-init)` + `mode.set source=inherit` + `policy.decision(child-session)` 四事件、转录含 constraint-injection custom_message、主会话门禁照常。
