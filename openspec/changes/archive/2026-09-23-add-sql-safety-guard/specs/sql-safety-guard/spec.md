## Purpose

定义 agent 会话 shell 通道在**执行前**对破坏性 SQL（仅 `TRUNCATE` / `DROP`）的硬拦截契约：命中判定、后缀授权逃生、事务回滚与一次性容器的自动放行、`psql -f` 文件内容扫描、模式配置与记账。目标是把「误清真库」从软约束（文档红线）升级为工具边界的硬门禁（2026-09-22 TRUNCATE CASCADE 事故教训）。

## ADDED Requirements

### Requirement: TRUNCATE/DROP 前置硬拦截

扩展 SHALL 在 Bash 类工具（`bash` / `ctx_execute` language=shell / `ctx_batch_execute`）执行前检查命令原文：当同时满足 ①含 psql 执行入口（出现 `psql` token，覆盖 `docker exec … psql`、`psql -c`、`psql -f`、heredoc 管入）②原文命中 `\bTRUNCATE\b` 或 `\bDROP\b`（大小写不敏感）——且不满足任何放行条件时，默认（hard）模式 SHALL 阻断执行，reason SHALL 给出全部合法出路。拦截范围 SHALL 仅限这两个词：`DELETE`/`UPDATE`/`ALTER`/`INSERT` SHALL NOT 触发拦截。

#### Scenario: 事故金样例被拦截
- **WHEN** 执行 `docker exec syntopica-postgres psql -c "DO $$ … TRUNCATE TABLE categories … $$;"`
- **THEN** 工具调用被 block、命令未执行，reason 含 `[sql-safety-guard]` 前缀与出路指引

#### Scenario: DROP 命中
- **WHEN** 执行 `psql -c "DROP TABLE tmp_x"`
- **THEN** block，命令未执行

#### Scenario: 只有 psql 无动词放行
- **WHEN** 执行 `docker exec syntopica-postgres psql -c "select count(*) from articles"`
- **THEN** 放行执行

#### Scenario: 只有动词无 psql 入口放行
- **WHEN** 执行 `grep -rn TRUNCATE docs/`（无 `psql` token）
- **THEN** 放行执行

#### Scenario: DELETE/ALTER 不在拦截范围
- **WHEN** 执行 `psql -c "DELETE FROM tmp_queue"`（无逃生注释）
- **THEN** 放行执行（用户拍板只拦 TRUNCATE/DROP）

### Requirement: 后缀授权逃生

当命令**原始文本**（判定在任何掩蔽/截断之前）含 `# allow-truncate-drop` 注释时，扩展 SHALL 放行本次执行并记录 info。逃生 SHALL 只认命令文本内的后缀注释——SHALL NOT 提供基于近期会话语境、历史命令或状态文件的豁免通道。

#### Scenario: 后缀注释放行
- **WHEN** 执行 `docker exec syntopica-postgres psql -c "TRUNCATE TABLE sandbox" # allow-truncate-drop`
- **THEN** 放行执行并记 info

#### Scenario: 会话语境不构成豁免
- **WHEN** 命令不含后缀注释，但近期会话消息出现「我确认要清库」类表述
- **THEN** 照常拦截（不设语境豁免）

### Requirement: 事务回滚与一次性容器自动放行

同一命令文本内 `BEGIN` 与 `ROLLBACK` 同时出现（大小写不敏感）时 SHALL 放行——事务内验证是与逃生注释等价的硬证据。一次性环境（`docker run --rm` 拉起的 postgres 客户端/服务、临时库 `createdb`+`dropdb` 配对）SHALL 同样放行。

#### Scenario: BEGIN/ROLLBACK 包裹放行
- **WHEN** 执行 `psql -c "BEGIN; TRUNCATE TABLE sandbox; ROLLBACK;"`
- **THEN** 放行执行（无需逃生注释）

#### Scenario: 只有 BEGIN 不放行
- **WHEN** 执行 `psql -c "DO $$ BEGIN … END $$;"`（含 BEGIN 无 ROLLBACK）
- **THEN** 照常拦截

### Requirement: psql -f 文件内容扫描

对 `psql -f <file>`（或 `--file`）形态，扩展 SHALL 读取所指文件内容并按同样规则扫描 `TRUNCATE`/`DROP`；文件存在且不含两词 SHALL 放行；文件不可读或参数解析不出 SHALL 保守拦截（无法证明无害即拦）。

#### Scenario: 只读 SQL 文件放行
- **WHEN** 执行 `psql -f scripts/db/readonly-report.sql` 且文件不含两词
- **THEN** 放行执行

#### Scenario: 文件含 DROP 拦截
- **WHEN** 执行 `psql -f migration-drop.sql` 且文件含 `DROP INDEX …`
- **THEN** block

#### Scenario: 文件不可读保守拦截
- **WHEN** 执行 `psql -f /not/exist.sql`（且命令无逃生注释/ROLLBACK）
- **THEN** block（无法验证内容）

### Requirement: 模式配置与记账

`SQL_SAFETY_GUARD` SHALL 支持 `hard`（默认，block）/ `soft`（仅 `ctx.ui.notify` 提醒不阻断）/ `off`（不评估）三模式，非法值回退 `hard`。block SHALL 记 `policy.decision(block, sql-safety)`，soft 命中 SHALL 记 `warn`；off/未命中 SHALL NOT 记账。守卫自身输出 SHALL 统一带 `[sql-safety-guard]` 前缀（语境扫描过滤依据，防自噬）。

#### Scenario: soft 模式只提醒
- **WHEN** `SQL_SAFETY_GUARD=soft` 且命令命中拦截条件
- **THEN** 命令照常执行，仅 UI 提醒 + 记 `warn`，不 block

#### Scenario: off 模式零开销
- **WHEN** `SQL_SAFETY_GUARD=off`
- **THEN** 不评估不记账（粗筛前直接返回）

#### Scenario: 非法配置回退默认
- **WHEN** `SQL_SAFETY_GUARD=yes`（非法值）
- **THEN** 按 `hard` 行为执行

### Requirement: 通道覆盖与判定面

扩展 SHALL 覆盖 `bash`（`input.command`）、`ctx_execute`（仅 language=shell 的 `input.code`）、`ctx_batch_execute`（逐 `commands[].command`）三通道；判定 SHALL 基于命令原文整体（含引号内 SQL 正文与嵌套 `bash -c` payload——不掩蔽引号，因引号内正是被拦的 SQL），性能上 SHALL 先做「含 psql ∧ 含两词」双键粗筛再走细则。

#### Scenario: ctx_batch_execute 逐条生效
- **WHEN** `ctx_batch_execute` 的某条 command 为事故金样例
- **THEN** 该次工具调用被 block

#### Scenario: 嵌套 bash -c 原文命中
- **WHEN** 执行 `bash -c 'docker exec syntopica-postgres psql -c "DROP TABLE t"'`
- **THEN** block（原文双键粗筛天然覆盖嵌套 payload）
