## 1. 扩展实现

- [x] 1.1 新增 `.pi/extensions/sql-safety-guard.ts`：`classifySqlCommand(command, cwd)` 纯函数（design D2 判定顺序：双键粗筛 → 逃生注释 → BEGIN/ROLLBACK → 一次性容器 → `-f` 文件扫描）+ `export default` 注册 `tool_call`、三通道取文本（bash / ctx_execute shell / ctx_batch_execute）（验证：`npx tsc --noEmit` 或既有扩展编译链通过；smoke 2.x 断言纯函数行为）
- [x] 1.2 模式与记账：`SQL_SAFETY_GUARD=hard(默认)/soft/off`，非法值回退 hard；hard=block+reason（三条出路：`BEGIN;…;ROLLBACK;` / `# allow-truncate-drop` / `SQL_SAFETY_GUARD=off`），soft=notify+warn，off=粗筛前返回；block/warn 经 `lib/policy-decision` 记 `sql-safety`，reason 统一 `[sql-safety-guard]` 前缀（验证：smoke 3.x 断言三模式行为与记账参数）

## 2. 金样例回归

- [x] 2.1 新增 `.pi/extensions/tests/sql-safety-guard.smoke.cjs`（仿 `test-scope-guard` 同名 smoke 形态）：**事故原文 fixture 必须 block**（`docker exec syntopica-postgres psql -c "DO $$ … TRUNCATE TABLE categories … $$;"`）+ DROP/大小写/heredoc/嵌套 `bash -c`/ctx_batch_execute 通道各一例（验证：`node .pi/extensions/tests/sql-safety-guard.smoke.cjs` 退出码 0）
- [x] 2.2 反例（必须放行）：`grep -rn TRUNCATE docs/`、只读 `psql -c "select 1"`、`DELETE FROM` 不拦、`BEGIN+ROLLBACK` 包裹、`# allow-truncate-drop` 后缀、只读 `-f` 文件；边界（必须拦）：`-f` 含 DROP、`-f` 文件不可读保守拦、只有 BEGIN 无 ROLLBACK 不放行、语境豁免不存在（验证：同上 smoke 全绿）

## 3. 文档

<!-- doc-impact: standard -->
- [x] 3.1 `docs/reference/harness/pi-extensions.md` 全景表登记 `sql-safety-guard`（拦截面/逃生口/模式/记账口径一行）（验证：`grep -c sql-safety-guard docs/reference/harness/pi-extensions.md` ≥ 1）
- [x] 3.2 红线文档随本 change 归档（工作区已写，核对归属）：`AGENTS.md` AI Behavior Rules「验证含副作用的 SQL 一律不碰真库」行 + `docs/reference/standard/backend/testing.md` §🛑 按副作用扩写（含事故记录与 `pg_restore -t` 静默零匹配坑）（验证：两处 `grep 'psql -c'` 命中；`<!-- doc-impact: standard -->` 与 testing.md 更新对账）

## 4. 测试

- [x] 4.1 运行 `node .pi/extensions/tests/sql-safety-guard.smoke.cjs` → 期望退出码 0、全部断言通过（金样例 block + 反例放行 + 三模式 + 记账参数）
- [x] 4.2 运行既有扩展 smoke 无回归：`node .pi/extensions/tests/test-scope-guard.smoke.cjs`（若存在）与 `policy-decision.smoke.cjs` → 期望退出码 0（共享 `lib/policy-decision` 未被破坏）

## 5. 文档

- [x] 5.1 三处文档一致（proposal Impact / pi-extensions.md 全景表 / testing.md §🛑）：拦截范围均为「仅 TRUNCATE/DROP」、逃生口均为 `# allow-truncate-drop`，无相互矛盾表述（人工：逐处比对三者措辞）

## 6. 验证

| Scenario | 测试文件 |
| --- | --- |
| 事故金样例被拦截 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| DROP 命中 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 只有 psql 无动词放行 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 只有动词无 psql 入口放行 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| DELETE/ALTER 不在拦截范围 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 后缀注释放行 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 会话语境不构成豁免 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| BEGIN/ROLLBACK 包裹放行 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 只有 BEGIN 不放行 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 只读 SQL 文件放行 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 文件含 DROP 拦截 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 文件不可读保守拦截 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| soft 模式只提醒 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| off 模式零开销 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 非法配置回退默认 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| ctx_batch_execute 逐条生效 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |
| 嵌套 bash -c 原文命中 | .pi/extensions/tests/sql-safety-guard.smoke.cjs |

落地后回检（test-cases 效果核对②，人工）：新会话手工执行事故原文 → 期望被 block 且 events.db 出现 `policy.decision(block, sql-safety)`（`bash scripts/harness/harness-retro.sh` / skill harness-facts 查询）。

- [x] 6.1 `node .pi/extensions/tests/sql-safety-guard.smoke.cjs` → 退出码 0
- [x] 6.2 `openspec validate add-sql-safety-guard` → `is valid`
- [x] 6.3 `grep -rn 'allow-truncate-drop' .pi/extensions/sql-safety-guard.ts .pi/extensions/tests/sql-safety-guard.smoke.cjs docs/reference/harness/pi-extensions.md` → 三处均命中（逃生口名称一致）
- [x] 6.4 事故原文回归：smoke 内 `TRUNCATE TABLE categories` fixture 断言 `block===true` 通过（命令原文出自 2026-09-22 事故记录，不可删改语义）
- [x] 6.5 `grep -c 'sql-safety' .pi/extensions/sql-safety-guard.ts` → ≥ 1（记账 reasonCode 落地）
