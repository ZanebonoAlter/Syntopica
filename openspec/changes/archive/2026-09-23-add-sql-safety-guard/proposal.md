<!-- complexity: complex -->
<!-- ui-impact: none -->

## Why

2026-09-22 事故：验证 `demo/entrypoint.sh` 的 DO 块时，`psql -c "DO $$ TRUNCATE…$$"` 被直接执行（`-c` 无 dry-run），CASCADE 级联清空本地库 7 张表，靠 04:00 自动备份才恢复。事后补的 AGENTS.md/standard 文档红线是**软约束**——依赖执行者每次都记得，用户明确判定「软约束没有用」，要求在工具执行边界上**硬挡住**（仓库已有 `test-scope-guard` 同款 `tool_call` 前置拦截机制可复用）。

## What Changes

- **新增 `.pi/extensions/sql-safety-guard.ts`**：Bash 类工具执行前拦截——命令同时命中「psql 执行入口」∧「`\bTRUNCATE\b`/`\bDROP\b`」且无放行条件时，hard（默认）直接 block，reason 给出三条出路。**只拦这两个词**（用户拍板：DELETE/UPDATE/ALTER 不拦，db-cleanup 等日常操作零摩擦）。
- **后缀授权逃生**：命令原文尾部 `# allow-truncate-drop` 注释即放行（用户拍板）；另有 `BEGIN`+`ROLLBACK` 同现自动放行、一次性容器放行；**不设会话语境豁免**（防自噬回路）。
- **`psql -f` 读文件扫描**：对 `-f` 指向的文件读取内容扫两词；文件读不到按命中保守拦截。
- **通道覆盖**：bash / ctx_execute(language=shell) / ctx_batch_execute 三通道取命令原文（同 test-scope-guard 的通道矩阵）。
- **模式与记账**：`SQL_SAFETY_GUARD=hard(默认)|soft|off`；block 记 `policy.decision(block, sql-safety)` 进 events.db；reason 统一 `[sql-safety-guard]` 前缀。
- **金样例回归**：smoke 以事故命令原文为 fixture——必须 block；配套反例（文档 grep、只读 `-f`、逃生口、ROLLBACK 包裹）必须放行。
- **文档**：`docs/reference/harness/pi-extensions.md` 全景表登记新扩展；AGENTS.md 行为红线与 `standard/backend/testing.md` §🛑 红线（已在工作区写好，随本 change 归档）。

## Capabilities

### New Capabilities
- `sql-safety-guard`: agent 会话 shell 通道对破坏性 SQL（TRUNCATE/DROP）的前置硬拦截行为契约：命中判定、后缀逃生授权、事务回滚放行、`-f` 文件扫描、模式配置与记账。

### Modified Capabilities

（无——`subagent-harness-coverage` 对 pi-extensions.md 的约束仅限「子线程矩阵」小节，本 change 不触及；testing.md/AGENTS.md 红线属文档层，无 spec requirement 变化。）

## Impact

- **新增**：`.pi/extensions/sql-safety-guard.ts`、`.pi/extensions/tests/sql-safety-guard.smoke.cjs`、change 内 `test-cases.md`（complex 档要求）。
- **文档**：`docs/reference/harness/pi-extensions.md`（全景表 +1 行）、`AGENTS.md`（AI Behavior Rules 破坏性 SQL 红线，工作区已写）、`docs/reference/standard/backend/testing.md`（§🛑 按副作用扩写，工作区已写）。
- **事件账本**：新增 `policy.decision` reasonCode `sql-safety`（harness-log 既有通道，无 schema 变化）。
- **行为边界（如实声明）**：只覆盖 AI 会话的 shell 工具通道；用户终端手敲、`pi.exec` 内部调用不经过 `tool_call` 天然不拦；mysql/sqlite 不在范围（本仓库仅 PG）。
- **对既有工作流的影响**：正常 `psql` 只读操作、`grep`/`sed` 涉及 SQL 关键词的文档操作零影响（双条件合取）；`psql -f` 跑含 DROP 的迁移/清理脚本会被拦——需事务包裹或加逃生注释（有意的摩擦）。
