<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: reading -->

## Why

两个独立故障，共同性质是「库内状态与代码假设脱节 → 用户操作静默失败」，且都是 2026-09-22 TRUNCATE 清库事故的余波（事故档：`docs/experience/2026-09-22-psql-truncate-cascade-incident.md`）。

### ① 新增订阅源 / 分类必然失败（sequence 漂移）

实测复现（事务内回滚，2026-09-28）：

```
BEGIN; INSERT INTO feeds (title, url, ...) VALUES ('probe', 'https://probe.invalid/feed.xml'); ROLLBACK;
ERROR: duplicate key value violates unique constraint "feeds_pkey"
DETAIL:  Key (id)=(2) already exists.
```

成因链：事故清场脚本 `demo/entrypoint.sh:73` 用 `TRUNCATE TABLE %I RESTART IDENTITY CASCADE`（`RESTART IDENTITY` 把序列重置为 1）；恢复用 `pg_restore --data-only --disable-triggers -t <表>`，`-t` 只匹配表对象、TOC 里的 SEQUENCE SET 条目被一并过滤掉 → 数据回来（COPY 携带原 id，不推进 sequence），计数器停在 1。备份本身无缺陷（`scripts/db/backup-key-tables.sh` 走 `pg_dump -Fc -n public` 全 schema）；缺陷在恢复方式漏了一步，而 `setval` **不受事务回滚保护**，漏了就静默留着。

漂移实测（`seq last_value` / `max(id)`）：`feeds` 1/34（**立即失败**）、`categories` 1/5（**立即失败**）、`articles` 4501/133738（下一次 `nextval`=4502，约 1252 篇后撞上 5754）、`article_topic_tags` 15681/225551、`user_preferences` 1/15920。2026-09-28 已手工对齐这 5 张表恢复可用；本 change 补自动检测，防止下一次恢复/迁移再次漂移。

### ② 删除订阅源 / 分类静默失败（NO ACTION 外键挡住整个删除事务）

实测复现（事务内回滚，模拟 `DeleteFeedCascade` 完整路径）：

```
BEGIN;
DELETE FROM reading_behaviors WHERE feed_id=2;  -- DELETE 14
DELETE FROM articles WHERE feed_id=2;           -- DELETE 17
DELETE FROM feeds WHERE id=2;                   -- ERROR: fk_user_preferences_feed ... Key (id)=(2) is still referenced
ROLLBACK;
```

`DeleteFeedCascade` 只清 `reading_behaviors` + `articles`(+从属行)，`DeleteCategoryCascade` 只清 `articles` —— 都不碰 `user_preferences`。而 `fk_user_preferences_feed` / `fk_user_preferences_category` 是 `NO ACTION`，只要源/分类在表里有引用行，删除事务整体回滚，源与文章原样留在库里；用户视角就是「点了删除、报了错、记录还在」。

`user_preferences` 本体是 discovery v2 已废弃的表（`preference-vector-feed-discovery` D9，Go 代码零引用，仅 `dump-sanitizer` 表清单还留着）。DROP 迁移 `20260725_0001` 被 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫跳过，且按 `db-migration-safety` spec 要求**记入 `schema_migrations` 为已应用**（避免每次启动重复告警）→ 表在开发环境与生产环境都**永久残留**。删除逻辑不能指望它消失，必须自己容忍。

影响面实测：24 个订阅源里 18 个（id 1,2,3,6,9,11,12,16,17,19,20,21,22,24,25,26,28,29）在 `user_preferences` 有引用 → 删不掉；删分类同样必然失败（还多一层 `reading_behaviors` 的 `article_id`/`feed_id` 双 NO ACTION FK），而前端侧边栏就有删除分类入口（`AppSidebarView.vue:232`）。

### ③ 既有行为台账要改

`docs/reference/flow/reading.md` 约束 #6 把「有 `reading_behaviors` / `user_preferences` 子行时该删除仍会因 NO ACTION 外键失败」记为**既有行为**（2026-09-17 决策遗留：当时只改前端文案、保持删除语义）。本 change 明确废止这条行为：删除必须真的可用（2026-09-28 用户报障 + 决策）。

## What Changes

- **删除路径补齐引用清理（同事务、容错）**：
  - `DeleteFeedCascade` 新增按 `feed_id` 清理 `user_preferences`（原只清 `reading_behaviors`）。
  - `DeleteCategoryCascade` 新增清理 `reading_behaviors`（按分类下 feed ids）与 `user_preferences`（按分类下 feed ids + `category_id`），原只清 `articles` 与其从属行。
  - 两张表的存在性探测沿用既有 `existingArticleDependents` 模式：表不存在（窄部署 / 已执行 DROP 的环境）则跳过该步、不报错；表存在但列缺失时 DELETE 自身报错并回滚（fail-closed），与既有注释语义一致。
  - `user_preferences` 无 Go 模型（已随功能删除），清理走带参数的 `Exec`/`Table` 语句而非模型删除。
- **sequence 漂移启动自愈**：`StartRuntime` 启动期（与 `resetStaleStates` 同时点、调度器与 HTTP 未起前）扫描「同时存在 `id` 列与 `<table>_id_seq` 序列」的表，判定「下一次 `nextval` 将分配的值 ≤ 表内 `max(id)`」即为漂移，执行 `setval(seq, max(id)+1, false)` 对齐；WARN 日志记录 `table / seq / max(id) / 新目标值 / 漂移根因提示`；单表失败只记日志、不阻塞启动；逻辑幂等（无漂移时不产生任何写入）。判定核心抽纯函数（`last_value` / `is_called` / `max(id)` → 是否漂移 + 目标值）以便单测覆盖边界。
- **文档**：`flow/reading.md` 约束 #6 从「既有失败行为」改写为「引用行同事务清理」的新语义；`deployment.md` 恢复流程补「恢复后启动自愈会自动对齐 sequence，及手工核对方式」；`docs/experience/` 记录迁移「跳过即记 applied」的权衡与后续 change 入口。

## Capabilities

### New Capabilities

- `feed-delete-referential-integrity`: 订阅源 / 分类删除时，指向它们的 `NO ACTION` 引用行（`reading_behaviors` / `user_preferences`）必须在同一事务内显式清理；引用表缺失时跳过而不失败；任一步失败整体回滚。
- `sequence-drift-self-heal`: 启动期检测「序列落后于表内 `max(id)`」并自动对齐，含漂移判据、对齐目标、日志与失败不阻塞启动的语义。

### Modified Capabilities

（无——`article-reference-integrity` 的 jsonb 引用维护与文章从属行清理语义不变；`db-migration-safety` 的守卫语义本 change 不动。）

## Impact

**代码**
- `backend-go/internal/reader/repository/repository.go`：`DeleteFeedCascade` / `DeleteCategoryCascade` 补引用清理（+ 抽取引用表容错 helper）
- `backend-go/internal/platform/database/`：新增序列漂移检测与对齐实现（含纯函数判定）
- `backend-go/internal/app/runtime.go`：`StartRuntime` 接线启动自愈

**测试**
- `repository` 单测（内存 SQLite）：删源/删分类在「有引用行」「无引用行」「引用表缺失」三态下的行为
- 漂移判定纯函数单测：`last_value/is_called/max(id)` 组合边界（未调用过的序列、空表、已对齐、超前）
- testcontainer PG 集成测试：真序列漂移检测与对齐、删除端到端不留孤儿引用

**数据**
- 本地库 5 张表 sequence 已于 2026-09-28 手工对齐（急救，非本 change 产物）；启动自愈负责此后的漂移
- 本地库 `user_preferences` 残留表（22 行）不自本 change 清理：迁移已记 applied，需另开 change 处置（见 Non-Goals）

**文档**
- `docs/reference/flow/reading.md`（约束 #6）、`docs/reference/deployment.md`（恢复流程）、`docs/experience/`（迁移语义权衡）

**Non-Goals**
- 不修迁移框架「破坏性迁移被跳过却记 applied」的语义（`db-migration-safety` 明文要求记 applied 以避免重复告警）；删除逻辑改为不依赖该表消失，语义缺陷另开 change 评估
- 不 DROP / 不迁移本地残留的 `user_preferences` 表
- 不改前端任何文案、组件、交互（现有确认文案与目标语义一致，ui-impact: none）
