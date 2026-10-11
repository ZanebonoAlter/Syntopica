## Context

- **事故来源**：2026-09-22 清库事故（`demo/entrypoint.sh` 的 DO 块 `TRUNCATE TABLE %I RESTART IDENTITY CASCADE` 被误执行）后，恢复走 `pg_restore --data-only --disable-triggers -t <表>`。`-t` 只匹配表对象，TOC 里的 SEQUENCE SET 条目被过滤 → 5 张表序列停在 1（或远低于 `max(id)`）。
- **删除被挡的来源**：`user_preferences` 是 discovery v2 已废弃的表（Go 零引用），DROP 迁移 `20260725_0001` 被 `MIGRATIONS_ALLOW_DESTRUCTIVE` 跳过，并按 `openspec/specs/db-migration-safety` 明文要求记入 `schema_migrations` 为已应用（避免每次启动重复告警）→ 表在开发与生产环境都永久残留，其 `NO ACTION` FK 长期挡住删源/删分类事务。
- **既有代码结构**：`DeleteFeedCascade` / `DeleteCategoryCascade` 都在 `r.db.Transaction` 内，已有「表不存在则跳过」的容错先例（`existingArticleDependents` / `articleDependents`），注释里明确写了 `reading_behaviors` 被 NO ACTION FK 挡住是「已知且有意保留」。
- **既有文档台账**：`flow/reading.md` 约束 #6 把「有 reading_behaviors / user_preferences 子行时删除因 NO ACTION 外键失败」记为既有行为（2026-09-17 决策遗留），本 change 要改这条台账。
- **测试环境约束**：单元测试用内存 SQLite（无 sequence / `pg_sequences`），集成测试才有 testcontainer PostgreSQL。

## Goals / Non-Goals

**Goals:**

- 删源 / 删分类在「目标表存在且有引用行」与「目标表不存在」两种环境下都能真正完成，且与前端确认文案语义一致。
- 序列漂移在启动期被自动发现并修复，不再依赖「恢复流程记得跑 setval」这种人工纪律。
- 漂移判定逻辑可在 SQLite 单测环境覆盖（不需要 PG 也能验证边界）。
- 修复过程可观测（日志留痕）且不阻塞启动。

**Non-Goals:**

- 不修迁移框架「破坏性迁移被跳过却记 applied」的语义（这是 `db-migration-safety` 明文要求的权衡，改动面涉及迁移框架核心，另开 change 评估）。
- 不 DROP / 不迁移本地残留的 `user_preferences` 表（迁移版本已 applied，需要独立的处置 change）。
- 不改前端任何组件、文案、交互。
- 不为序列漂移做「定时巡检」（启动期一次即覆盖事故场景；运行期不产生漂移）。

## Decisions

### D1：删除路径显式清理引用行，而不是依赖表被 DROP

`DeleteFeedCascade` 增 `user_preferences`（按 `feed_id`）；`DeleteCategoryCascade` 增 `reading_behaviors`（按分类下 `feed_id` 集合）与 `user_preferences`（`feed_id ∈ 分类下订阅源` 或 `category_id = 分类`）。

- **理由**：生产环境永远不会有 DROP 的机会（`MIGRATIONS_ALLOW_DESTRUCTIVE` 不设为纪律），删除逻辑必须自己健壮；这也是既有 `articleDependent` 模式的同族扩展。
- **替代方案（否决）**：靠 DROP 表消除 FK —— 生产不可达；靠前端提示「删不掉」—— 功能不可用。
- **顺序约束**：引用行清理一律排在「删 `articles` 行」之前 —— `reading_behaviors.article_id` 的 NO ACTION FK 会在删文章时先炸（这也是 `DeleteFeedCascade` 现有顺序的既有理由）。

### D2：引用表清理走「表存在性探测 + 跳过」，探测失败 fail-open

沿用 `existingArticleDependents` 的语义：`HasTable` 为假（含探测查询失败）时跳过该表的清理，其余步骤照常完成。**仅探测表存在性，不探测列**——列缺失时 DELETE 自身报错并回滚整个事务（fail-closed）。

- **理由**：删除是用户主动操作，宁可在极窄场景留下引用孤儿（无害：表已废弃）也不要因为元数据探测失败挡住用户删除；而列不匹配是真错误，必须回滚暴露。
- **代价**：探测查询失败时会静默跳过清理（注释已记录该边界）。

### D3：`user_preferences` 用参数化 SQL 清理（无 Go 模型）

该表的模型已随功能删除，清理用 `tx.Table("user_preferences").Where("feed_id = ?", id).Delete(nil)` / `Exec` 形式，占位符一律参数化，禁止字符串拼接 id。

### D4：序列自愈放在启动期的 `StartRuntime`，而不是独立脚本或定时任务

- **理由**（用户 2026-09-28 决策）：漂移只在「清库 + 恢复」后出现，启动是唯一必然经过的时点；脚本需要人记得跑（本次事故恰恰是人工流程漏了一步），定时任务对一次性问题过重。
- **时点**：与 `resetStaleStates` 同处 `StartRuntime` 开头——调度器与 HTTP 未起、无并发写，检测到的 `max(id)` 是稳定快照。
- **替代方案（否决）**：独立巡检脚本（靠人记得，与事故成因同类）；定时任务（一次性问题，长期空转）。

### D5：对齐目标是 `max(id) + 1` 且 `is_called = false`，绝不回退

- `setval(seq, max+1, false)` 使下一次 `nextval` 精确返回 `max+1`（与项目既有 `dump-sanitizer` / `testutil` 的写法一致）。
- 只有在「下一次分配值 ≤ `max(id)`」时才写；已领先的序列不写、不回退（避免与 `setval` 不可回滚的语义叠加出意外）。
- 空表（`max(id)` = 0）与从未调用过的序列不误修。

### D6：判定逻辑抽纯函数，SQLite 单测覆盖边界，PG 集成测试覆盖真实序列

- 单测环境没有序列语义，因此 `(lastValue, isCalled, maxID) → (needsHeal, target)` 必须是可单测的纯函数；表/序列枚举与「执行 setval」留在平台层，由 testcontainer 集成测试覆盖。

### D7：自愈失败不阻塞启动

与 `resetStaleStates` 同风格：单表失败只记日志继续；探测失败同样不阻塞。理由：启动可用性优先，漂移本身不影响读路径，失败会在日志与后续报障中暴露。

## Risks / Trade-offs

| 风险 | 说明 | 缓解 |
|---|---|---|
| 删除范围扩大引发误删 | 新增清理 `reading_behaviors` / `user_preferences` 扩大了删除的行集 | 限定在「被删实体直接指向」的行（`feed_id` / `category_id` 匹配），测试覆盖「分类外数据不受影响」；`topic_tags` 等仍交维护任务，不越权 |
| 表探测 fail-open 留孤儿 | 元数据查询失败时跳过清理，可能留下引用孤儿 | 表已废弃且无害；删除主路径不受影响；注释与 spec 明确该边界 |
| 自愈在错误时点写入 | 启动期执行 `setval` 若被并发写入打断 | 时点选在调度器/HTTP 启动前，无并发写；`setval` 本身原子 |
| 自愈掩盖真实问题 | 自动修复让「恢复流程漏 setval」不再显现 | 每次修复打 `WARN` 并在日志给出根因提示；恢复流程文档同步补说明 |
| 目录枚举范围误差 | 只覆盖「有 `id` 列 + `<table>_id_seq`」的表，复合主键表天然排除 | 复合主键表无单列序列，本就不适用；枚举条件写进 spec 与测试 |
| 迁移缺陷未修 | `user_preferences` 残留表仍在（生产同样残留） | 删除逻辑已容错，业务不再受影响；缺陷记录进 `docs/experience/` 并按用户决策另开 change |
