# Tasks: split-database-docs-by-domain

## 1. 全局骨架

- [x] 1.1 建 `docs/reference/database/tables/_conventions.md`：迁移「阅读约定 5 条 + FK 真相 + DB FK 清单 + pgvector/全文检索/性能/唯一/CHECK 全局索引 + 向量维度规则总述」（源：DATABASE_FIELDS.md §阅读约定 + §14 尾部全局小节 + ER_DIAGRAM.md「FK 引用矩阵 Part A/B + 关系模式说明」）。验证：文件存在且覆盖上述各块，`grep -c "## " tables/_conventions.md` ≥ 5。
- [x] 1.2 重写 `docs/reference/database/_index.md` 为纯导航：域文档链接表（含 flow 域对照列）+ 完整表清单（50+5+1，按域分组即归属速查表）+ 全局概览 ASCII 图（源：ER_DIAGRAM.md「全局域级概览」）。验证：表清单行数 = 56 张表行；含 flow 对照列。

## 2. 域文档字段字典搬运（12 文件）

每文件 = 头部一句话域定位 + 本域各表字段字典小节（源：DATABASE_FIELDS.md 对应 §，表内文字原样搬运）。验证统一标准：文件 ≤300 行、每表一个 `### 表名` 小节。

- [x] 2.1 `tables/content.md`（§1 + §14「文章三个内容字段」说明块并入）——验证：含 articles/feeds/categories 三表。
- [x] 2.2 `tables/scheduling-config.md`（§2）——验证：含 scheduler_tasks/ai_settings。
- [x] 2.3 `tables/ai-routing.md`（§3）——验证：含 ai_providers/ai_routes/ai_route_providers/ai_call_logs/ai_embedding_cache 五表。
- [x] 2.4 `tables/topic-tags.md`（§4）——验证：含 topic_tags 等 7 表。
- [x] 2.5 `tables/semantic-labels.md`（§5）——验证：含 semantic_labels 及中间表 6 张。
- [x] 2.6 `tables/embeddings.md`（§6）+ `tables/job-queues.md`（§7）——验证：两文件合计覆盖 embedding_config/embedding_queues/merge_reembedding_queues/firecrawl_jobs/tag_jobs。
- [x] 2.7 `tables/daily-report-watch.md`（§9）——验证：含 7 表（board_daily_reports 等）。
- [x] 2.8 `tables/data-enrichment.md`（§10）——验证：含 §10 全部表（board_data_sources 等 10+ 张）。
- [x] 2.9 `tables/preference-discovery.md`（§11）+ `tables/tracing.md`（§12）——验证：含 user_preferences/reading_behaviors 等 + otel_spans。
- [x] 2.10 `tables/deprecated-framework.md`（§13 全部废弃/预留表 + §14 的 schema_migrations + AI Summaries 废弃面简化 ER 图）——验证：5 废弃表 + 1 框架表齐全。

## 3. 域 ER 子图重绘（diagram-design）

- [x] 3.1 用 diagram-design skill 按 bounded-context 视图重绘 11 张域子图（deprecated 域已含简化图）：本域实体展开（字段列表可精简至 PK/FK/关键状态列以控行数），外域直接邻居仅实体名节点；Mermaid `erDiagram` 语法。**纪律：每张子图边集 ⊆（原七面边集 ∪ FK 引用矩阵行），禁止凭记忆新造边**。验证：11 张子图逐一嵌入域文档，`grep -c '```mermaid' tables/*.md` = 12。
- [x] 3.2 逐图核对边集：对照 ER_DIAGRAM.md 原图与 `postgres_migrations.go` 真实 FK/索引抽查每图 ≥2 条边。验证：核对结论记入本任务描述（每图 2 条边的出处）。
  - 核对结论：(a) 边集守恒双验证——自写提取器与 diagram-design `mermaid_extract.py` 交叉提取原七面边集，均 49 条去重边，新 12 图边并集 49，遗漏 0 / 凭空 0；(b) 逻辑边抽查——categories→feeds（category.go:17 foreignKey:CategoryID）、feeds→articles（feed.go:29）、articles→firecrawl_jobs 与 articles→tag_jobs（job_queue.go:28/52，跨域边）；(c) 真实 FK 边 6 条全部从 postgres_migrations.go ADD CONSTRAINT 逐条验证（见 change pin「真实 DB 外键 = 6 条」）；(d) tracing 域无边（独立表），文档已注明。

## 4. 索引拆分 + 旧文件移除 + 引用修复

- [x] 4.1 把 §14「索引与约束总览」中按域分节的小节（语义标签域/叙事域/偏好发现域/日报域/AI 调用日志域索引）拆入对应域文档尾部「索引」节。验证：各域文档含「索引」节且 `_conventions.md` 不再含按域小节。
- [x] 4.2 `git rm` `DATABASE_FIELDS.md` + `ER_DIAGRAM.md`。验证：两文件不存在于工作树。
- [x] 4.3 引用修复：`architecture/overview.md` 3 处链接改指 `database/_index.md` 或对应域文档；`flow/data-enrichment.md` 溯源行 `DATABASE_FIELDS.md §16` 改指 `tables/data-enrichment.md`（只改链接目标不动历史叙述）。验证：`grep -rn "DATABASE_FIELDS\|ER_DIAGRAM" docs/ openspec/specs/ scripts/ .pi/` 除 openspec/changes/ 外零命中。

## 5. 验证

Scenario 对账映射（归档门禁 scenario-trace 消费）：

| Scenario | 测试文件 |
| --- | --- |
| 域文档包含字段字典与 ER 图 | `docs/reference/database/tables/daily-report-watch.md` |
| 全局约定集中于入口文档 | `docs/reference/database/tables/_conventions.md` |
| 旧单文件移除且无死链 | 人工：全仓库检索 DATABASE_FIELDS 与 ER_DIAGRAM 字样，除 openspec/changes/ 归档历史外零命中（任务 5.4 复检） |
| topic_analysis_jobs mapping is corrected | 人工：核对 `tables/deprecated-framework.md` §13.2 小节（topic_analysis_jobs 标注已废弃无 migrator 注册）与 `_index.md` 废弃表分组行 |
| model path uses current location | 人工：核对 `tables/deprecated-framework.md` §13.1 小节 ai_summaries 的 model 引用为 `internal/models/`（无 `internal/domain/models/` 残留：`grep -c "internal/domain/models" tables/*.md` = 0） |

- [x] 5.1 表数守恒：新 `_index.md` 表清单 56 张表逐一对应 12 域文档小节标题，各出现且仅出现一次（脚本或逐域 grep 核对）。验证：输出守恒对照结果。
- [x] 5.2 行数上限：`wc -l docs/reference/database/tables/*.md docs/reference/database/_index.md` 全部 ≤300。验证：命令输出全绿。
- [x] 5.3 specs Scenario 覆盖核对（scenario-trace）：`openspec/changes/split-database-docs-by-domain` 四个 Scenario（域文档含字典+ER、全局约定集中、无死链、topic_analysis_jobs 映射修正）逐条指向上述任务产物。验证：scenario-trace 输出无 MISSING。
- [x] 5.4 归档门禁：`scripts/doc-impact.sh verify` + `scripts/check-standards.sh` + 全仓库死链复检通过后归档（含主 spec `database-docs` delta 合并）。

## 6. 测试

纯文档 change（无代码路径变更），以 grep 一致性校验代替代码测试：

- [x] 6.1 边集守恒：自写提取器 × diagram-design `mermaid_extract.py` 交叉提取原七面边集均 49 条，新 12 图边并集 49 —— 期望：遗漏 0 / 凭空 0（已执行，任务 3.2 记录）。
- [x] 6.2 表数守恒：`grep -c '^| `' docs/reference/database/_index.md` 计表行 58（52 业务 + 5 废弃 + 1 框架），逐一匹配域文档小节/正文 —— 期望：58/58 无缺失（已执行，任务 5.1 记录）。
- [x] 6.3 死链清零：`grep -rn "DATABASE_FIELDS.md\|ER_DIAGRAM.md" docs/reference/ docs/README.md` 过滤历史叙述后 —— 期望：零命中；全仓库残留仅 archive/v1.x/issues 历史归档（已执行，任务 5.4 复检）。
- [x] 6.4 行数上限：`wc -l docs/reference/database/tables/*.md docs/reference/database/_index.md` —— 期望：全部 ≤300（最大 263，已执行）。

## 7. 文档

<!-- doc-impact: database -->

<!-- 注：本 change 为纯文档结构切分，无 flow 影响（不触及任何业务链路行为），溯源豁免。 -->

- 域声明：无（纯文档结构 change，不涉及业务域行为）。
- database 域：`docs/reference/database/` 整体重组——DATABASE_FIELDS.md/ER_DIAGRAM.md 移除，新增 `tables/` 14 文件，`_index.md` 重写。
- 引用清零（非域内容变更）：architecture/overview.md、flow/data-enrichment.md 溯源行、DATA_LIFECYCLE.md 相关文档节、docs/README.md 导航。
- 顺手修正：真实 DB FK 三处矛盾（1/2/3 版本）统一为 6 条权威清单；表清单补录 cross_board_relation_runs/relations（52 业务表口径）。
