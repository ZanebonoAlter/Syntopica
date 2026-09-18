# Tasks — harden-subagent-constraint-channel

> 复杂度 simple（无状态机/算法/协议设计，判定逻辑为单点条件短路）；ui-impact none（纯工具链）。
> 研究底稿：`docs/research/subagent-constraint-gap/explore-findings.md`（根因证据链 + 通道对比）。

## 1. 配置层（A）

- [ ] T1 新增项目侧 profile `.pi/agents/implementer.md`：frontmatter 按 pi-web snake_case 白名单（`description` / `tools: read, bash, edit, write, grep, find, ls` / `load_skills: false` / `load_extensions: true` / `inherit_context: false` / `enabled: true`），正文提示词对齐 §0.6 实现类子线程角色（必读文件先行、改动最小化、受影响包测试）
- [ ] T2 实测 profile 注册：pi 会话内确认 Agent 工具的 agent 列表出现 `implementer`，字段被读取（非静默退化；design D2 风险点）

## 2. 扩展层（C）

- [ ] T3 constraint-injection：抽 `isChildSession()` 同源判定复用（parentSession header / fork 路径），smoke 补「pi-web 子会话特征」用例（独立 sessionId + startup + header parentSession → `mode.set source=inherit` + 注入带绑定 change）——**已完成（2026-09-18）**
- [x] T3-rev（D3 修订）：子线程稳定层投递改走 session_start 末尾 `sendMessage(steer)`（记 `constraint.inject source=child-init` + 置 stableSnapshot）；before_agent_start 冻结路径子会话不再返回 systemPrompt 块（防双投）+ smoke 用例；顺手补 SKILL.md 词表遗漏的 `toolchain-down` 登记项——**已完成（2026-09-18，SMOKE 全绿）**
- [ ] T4 quality-gate：turn_end 入口接 `isChildSession()` 短路——子线程零重命令放行，记 `policy.decision {policy:"quality-gate", action:"bypass", reasonCode:"child-session"}`；主会话路径零改动
- [ ] T5 词表同步：扩展内 reasonCode 白名单加 `child-session`；`.agents/skills/harness-facts/SKILL.md` 枚举同步；smoke 精确断言更新

## 3. 文档沉淀

- [ ] T6 `docs/reference/harness/pi-extensions.md` 补「子线程通道矩阵」小节：pi-web / pi-subagents × 前台 / 后台四象限（扩展加载 / 约束可达 / 门禁行为）+ 已知限制（AGENTS.md 不自动加载、前台盲区编排兜底、pi-web 升级漂移依赖点）
- [ ] T7 `docs/reference/开发执行规范.md` §0.6 派发要点补一句：实现/验证类子线程用 `implementer` 档（带扩展），只读探索用内置 explore/plan，前台派发属盲区须任务文本带红线

## 测试

- [ ] T8 smoke：`.pi/extensions/tests/` 补 `isChildSession` 判定矩阵 + quality-gate 子线程 bypass 行为（零命令 + 一条 bypass 记账）+ 主会话不受影响 + policy.decision 形状断言；`bash .pi/extensions/tests/run-smoke.sh` 全绿
- [ ] T9 既有 smoke 回归（constraint-injection 全量用例含新增 pi-web 继承用例）不红

## 文档

<!-- doc-impact: none(harness 与执行规范文档不在七域清单；本 change 文档产出为 harness/pi-extensions.md 与开发执行规范 §0.6，已列于上方 T6/T7 与验证节 V4/V5) -->

- [x] docs/research/subagent-constraint-gap/explore-findings.md（已完成，研究底稿）
- [ ] docs/reference/harness/pi-extensions.md（T6）
- [ ] docs/reference/开发执行规范.md（T7）
- [ ] .agents/skills/harness-facts/SKILL.md（T5 词表）

## 验证

Scenario→测试映射：

| Scenario | 测试文件 |
| --- | --- |
| subagent-harness-coverage·实现档带扩展派发后约束可达 | 人工：V1 实测（事件库三事件） |
| subagent-harness-coverage·只读探索档保持轻量 | 人工：V2（内置档现状不变） |
| subagent-harness-coverage·子线程 turn_end 降载并记账 | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs`（场景 J） |
| subagent-harness-coverage·主会话门禁行为不变 | `.pi/extensions/tests/quality-gate.behavior.smoke.cjs`（场景 K）+ `.pi/extensions/tests/quality-gate.smoke.cjs`（CS 判定矩阵） |
| subagent-harness-coverage·矩阵文档可检索 | V4 grep 命令 |
| constraint-injection·子线程显式继承父会话（pi-web 通道） | `.pi/extensions/tests/constraint-injection.smoke.cjs`（新增用例）+ 人工：V1 |
| constraint-injection·其余既有 Scenario | `.pi/extensions/tests/constraint-injection.smoke.cjs`（回归，T9） |

验证命令（每条「命令 + 期望」）：

- [ ] V1 `bash -c 'openspec/dev 派发 implementer 子线程做最小验证任务后，sqlite3 .pi/harness/events.db "SELECT kind, COUNT(*) FROM events WHERE session_id=(SELECT MAX(session_id) FROM events WHERE kind=\"session.start\" AND session_id != \"父\") GROUP BY kind;"'` → 期望：该子会话含 `session.start`、`constraint.inject`、`mode.set`（payload source=inherit）至少三条（人工执行，结果贴 change 目录 `apply-report.md`）
- [ ] V2 探索类任务继续用内置 explore 派发一次 → 期望：该子会话在 events.db 零 harness 事件（现状盲区保持，靠任务文本兜底）
- [ ] V3 `bash .pi/extensions/tests/run-smoke.sh` → 期望：全绿，含新增 `isChildSession` 矩阵、quality-gate bypass、constraint-injection pi-web 继承用例
- [ ] V4 `grep -n "子线程通道矩阵" docs/reference/harness/pi-extensions.md` → 期望：命中矩阵小节，四象限 + 已知限制条目齐全
- [ ] V5 `grep -n "implementer" docs/reference/开发执行规范.md` → 期望：§0.6 派发要点命中新档约定
- [ ] V6 `sqlite3 .pi/harness/events.db "SELECT COUNT(*) FROM events WHERE kind='policy.decision' AND json_extract(payload,'$.reasonCode')='child-session';"` → 期望：V1 派发的子线程 turn_end 后 ≥1
- [ ] V7 主会话完成任意一轮编辑 → 期望：turn_end 门禁照常执行，无 child-session 记账（V6 计数不因主会话增长）
