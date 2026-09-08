
## database 文档切分的关键事实（design 输入）

1. harness 消费链路：api/ 与 database/ 目录**不在** constraint-injection 扫描范围（constraint-injection.ts walkDocs 只走 docs/reference/flow 深度1 + standard 深度2），无 doc-impact-applies 标签；doc-impact.sh 归档预勾选是域级粒度，与文件切分无关。切分对 harness 零影响（刻意保持，文档是代码投影，注入有漂移风险）。
2. DATABASE_FIELDS.md（1433 行/99.5KB）域清单：§1 内容 / §2 调度配置 / §3 AI 路由 / §4 主题标签 / §5 语义标签·板块 / §6 向量 / §7 任务队列 / §9 日报·持久话题·Watch（跳 §8）/ §10 数据增强 / §11 偏好发现 / §12 追踪 / §13 废弃预留 / §14 框架表。**§14 名不副实**：1214 行起 schema_migrations 仅 8 行，其余为「文章三字段说明」（归 content 域）+ 按域分节的「索引与约束总览」+「向量维度规则」（全局/分域内容，需分流）。
3. ER_DIAGRAM.md 七面与字段域**多对多交叉**：board_persistent_topics 出现于 DataEnrichment + DailyReport 两面、semantic_labels 三面、topic_tags 三面，Core 面横跨内容/队列/偏好三域——无法一比一搬运，design D2 决策按域重绘 bounded-context 子图（域实体展开、外域邻居仅名节点），边集必须 ⊆ 原七面边集 ∪ FK 引用矩阵行。
4. 外部引用点（死链清零范围）：architecture/overview.md:224-226（3 链接）、flow/data-enrichment.md:356 溯源行（DATABASE_FIELDS.md §16，§ 编号已失配）、openspec/specs/database-docs/spec.md（现存 spec，两个 Requirement 锚定旧文件名，本 change delta 已 REMOVED+ADDED 承接）。openspec/changes/archive/ 内历史链接不改。
5. 验收锚点：表数守恒 50 业务+5 废弃+1 框架各出现一次；新域文档全部 ≤300 行；全仓库 grep DATABASE_FIELDS|ER_DIAGRAM 除归档外零命中。旧文档「更新日志」历史节不迁移（doc-authoring.md 只强制 flow 域变更溯源）。

<!-- pinned 2026-09-06T03:02:25Z -->

## 真实 DB 外键 = 6 条（原文档 1/2/3 三版本全错）

原文档三处自相矛盾（"真实 DB FK"曾写 1/2/3 条三个版本），2026-09 逐条核对 backend-go/internal/platform/database/postgres_migrations.go 后确认真相为 **6 条**（已统一写入 tables/_conventions.md「DB 级外键（全库共 6 条，权威清单）」）：
1. topic_tags_merged_into_id_fkey：topic_tags.merged_into_id → topic_tags(id) CASCADE，迁移 20260601_0001（先 DROP 再幂等重建）
2. fk_topic_watch_hits_watch：topic_watch_hits.watch_id → board_topic_watches(id) CASCADE，20260801_0002
3. fk_topic_tag_embeddings_tag：topic_tag_embeddings.topic_tag_id → topic_tags(id) CASCADE，20260820_0001
4. fk_board_topic_watches_topic：board_topic_watches.persistent_topic_id → board_persistent_topics(id) SET NULL，20260825_0001
5. fk_topic_enrichment_result_parent_board：复合 FK (parent_result_id, semantic_board_id) → topic_enrichment_result(id, semantic_board_id) RESTRICT，20260828_0001
6. fk_composite_components_composite：composite_components.composite_id → semantic_labels(id) CASCADE，20260902_0001
其余全部为 GORM 逻辑关联（DisableForeignKeyConstraintWhenMigrating: true，AutoMigrate 不建 FK）。重绘域 ER 子图时，真实 FK 边（6 条）应与逻辑关联边区分标注。

<!-- pinned 2026-09-06T03:13:53Z -->
