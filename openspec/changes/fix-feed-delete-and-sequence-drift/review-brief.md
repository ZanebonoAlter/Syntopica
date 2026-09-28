# 审查 brief：fix-feed-delete-and-sequence-drift（全量增量，2026-09-28）

只读审查。你只输出报告，禁止修改任何文件、禁止执行改变状态的命令（允许 read/grep/find/ls 与只读验证如 `go vet` / `go build`；优先 grep 与精读）。

## 一、审查对象（本轮增量，未 commit，全在工作树）

**背景**：两个 2026-09-22 清库事故（`TRUNCATE ... RESTART IDENTITY CASCADE` 被误执行 + 带 `-t` 表过滤的 `pg_restore` 恢复）的余波缺陷（实测输出见 change `proposal.md` Why 节）：

- **① 序列漂移**：5 张表 `<table>_id_seq` 落后于表内 `max(id)`（`feeds` 1/34、`categories` 1/5、`articles` 4501/133738、`article_topic_tags` 15681/225551、`user_preferences` 1/15920）→ 新增订阅源/分类必然报 `duplicate key value violates unique constraint "<t>_pkey"`。
- **② 删除被 NO ACTION 外键挡住**：`DeleteFeedCascade` / `DeleteCategoryCascade` 不清理 `user_preferences`（已废弃但永久残留的表，Go 零引用、DROP 迁移被 `MIGRATIONS_ALLOW_DESTRUCTIVE` 跳过且记 applied）→ 删除事务整体回滚，用户视角「点了删除、报了错、记录还在」。24 个订阅源中 18 个删不掉。

**本轮改动清单**：

| 任务 | 辖区代码 |
| --- | --- |
| 1.1–1.4 删除路径引用清理 | `backend-go/internal/reader/repository/repository.go`：新增 `deleteReferencingRowsIfPresent` / `deleteFeedReferenceRows` / `deleteCategoryReferenceRows`；`DeleteFeedCascade` / `DeleteCategoryCascade` 接入；两处过时注释改写 |
| 1.5 集成测试 | `backend-go/internal/reader/repository/feed_delete_refs_test.go`（新增，PG testcontainer：测试内重建 `user_preferences` 表与历史 NO ACTION FK） |
| 2.1–2.4 序列自愈实现 | `backend-go/internal/platform/database/sequence_heal*.go`（新增：纯函数判定 + 候选表枚举 + setval + WARN 日志）、`backend-go/internal/app/runtime.go`（`StartRuntime` 接线） |
| 2.5–2.6 序列自愈测试 | `backend-go/internal/platform/database/sequence_heal_test.go`（纯函数边界）、`sequence_heal_pg_test.go`（testcontainer PG 真序列） |
| 3.x 文档 | `docs/reference/flow/reading.md`（约束 #6）、`docs/reference/deployment.md`（新「序列漂移与启动自愈」节 + 破坏性迁移副作用段）、`docs/reference/database/{DATA_LIFECYCLE.md,tables/_conventions.md,tables/content.md}`、`backend-go/DATABASE.md` / `DATABASE_SUMMARY.md` |

设计权威源：`openspec/changes/fix-feed-delete-and-sequence-drift/design.md`（决策 D1–D7 + Non-Goals + Risks）。
验收契约：`specs/feed-delete-referential-integrity/spec.md`、`specs/sequence-drift-self-heal/spec.md`。
Scenario 映射：`tasks.md` §5「验证」节映射表。

## 二、审查维度（按开发执行规范 §0.6 步骤 4）

1. **精读高风险文件**（至少覆盖）：
   - `repository.go` 三个新函数：
     - `deleteReferencingRowsIfPresent` 的 fail-open 语义（`HasTable` 探测失败读作 false → 跳过清理）：在**存量 FK 的库**上，探测失败会让引用行留着 → 后续删 `feeds` 撞 FK → 整个事务回滚。判断该权衡是否被注释与 design D2 诚实记录、失败时错误信息是否仍可诊断（不是"静默成功"）。
     - `deleteCategoryReferenceRows` 的 SQL：`category_id = ? OR feed_id IN ?` 的分支与空集合处理（`len(feedIDs)==0` 时只按 `category_id`）；表名常量拼接是否存在注入面（id 均为绑定参数）。
     - **事务顺序**：引用清理必须严格在 `deleteArticlesWithRefs` / `deleteArticlesOfFeedsWithRefs` **之前**（`reading_behaviors.article_id` 的 NO ACTION FK 会在删文章时先炸）。
     - **越权删除**：分类删除只应碰「分类下 feed 的引用行」与 `category_id` 指向该分类的偏好行；`topic_tags` 孤儿不应被删（仍归 `aux_label_cleanup`）。
     - `DeleteCascadeByFeed`（未接入新清理）是否真的无调用方、不构成遗漏。
   - 序列自愈（`sequence_heal*.go` + `runtime.go`）：
     - 判定逻辑与 `specs/sequence-drift-self-heal/spec.md`、design D5 逐条对齐：**仅当**「下一次 `nextval` 分配值 ≤ `max(id)`」才修（`is_called` 两种取值）、目标 `max(id)+1`、`is_called=false`、空表 `max=0` 不误修、领先时不回退。
     - **标识符与参数安全**：表名/序列名拼接是否正确引用（catalog 来源仍需引号或 `%I`），`setval` 目标值必须参数化，禁止字符串拼接数字。
     - 失败语义：单表失败继续、目录查询失败只记日志——是否存在 panic 路径或会中止启动的分支。
     - 接入时点：必须在 `StartRuntime`（`resetStaleStates` 之后、调度器/HTTP 之前）；`main.go` 在 `DEMO_READ_ONLY=1` 时不调用 `StartRuntime`，故那里天然不写只读快照——确认没有挪到 `main.go` 或加多余分支。
     - 幂等：连续两次执行第二次零写入、序列不倒退。
   - `feed_delete_refs_test.go`：测试是否真的能测出「FK 阻断」（测试内重建 `user_preferences` + 还原 `reading_behaviors` 两个历史 FK）？断言是否覆盖「引用行真被删」而不只是「没报错」？fixture 是否污染同进程后续用例（`t.Cleanup` 是否充分）？
2. **grep 全包验证关键不变量**（至少）：
   - `grep -rn "UserPreference" backend-go/internal/models/` → 空（表无 model，清理只走参数化 SQL）。
   - `grep -n "articleDependent\|deleteArticleDependents" backend-go/internal/reader/repository/repository.go` → 结构与被调用行为零变化（本 change 只加步骤）。
   - `grep -rn "localhost:5432" backend-go/internal/platform/database/*_test.go backend-go/internal/reader/repository/*_test.go` → 不应出现在测试连接串（禁连业务库红线）。
   - `git diff --stat backend-go/internal/platform/database/migrator.go backend-go/internal/platform/database/postgres_migrations.go demo/entrypoint.sh` → 应无改动（迁移框架与清场脚本不在范围）。
   - `sequence_heal` 是否 import `internal/reader`（依赖方向应有平台层 → 上层为零）。
3. **分级**：High（bug/安全/清晰错误，必修）/ Medium（合理但依赖上下文，选修）/ Low（误报，静默丢弃）。

## 三、输出要求（三段式，中文）

1. **总评**：改动是否达成 design/spec 意图、可否进入门禁收口（一句话结论 + 理由）。
2. **问题清单**：按 High/Medium/Low 分级，每条给 `file:line` + 复现/依据 + 建议修法；没有就明确写「无」。
3. **核实记录**：实际跑过的只读命令与退出码、读过的关键文件清单（防幻觉）。

## 已知且已裁决、**不要**当问题报的事项

- 迁移框架「破坏性迁移被跳过却记 applied」不在本 change 范围（`openspec/specs/db-migration-safety` 明文要求记 applied 以避免重复告警；另开 change 评估）。
- 不 DROP / 不迁移残留的 `user_preferences` 表本身（另一 change 处置）。
- 探测元数据失败时 fail-open 跳过清理（design D2 明示权衡）。
- 分类删除的 `reading_behaviors` 只按 `feed_id` 集合清理；`feed_id IS NULL` 且 `article_id` 属被删文章的假设场景未覆盖（与既有 feed 路径同形状；真库实测 562/562 行均有 `feed_id`）。
- 本 change 不修前端任何文件（ui-impact: none）。
- 工作树含其他 active change 脏文件（`.pi/agents/*.md`、`front/app/features/articles/components/ArticleStatusMenu.vue`、`openspec/changes/ai-provider-model-overhaul/**`）——超出本 change 辖区，一律不评。
