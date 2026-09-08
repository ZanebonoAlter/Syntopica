# Design: split-database-docs-by-domain

## Context

现状事实（见 proposal Why）补充三点设计输入：

1. **`DATABASE_FIELDS.md` 的 § 域清单**（去编号后 12+2 组）：§1 内容域 / §2 调度与配置 / §3 AI 路由 / §4 主题标签 / §5 语义标签·板块 / §6 向量 / §7 任务队列 / §9 日报·持久话题·Watch / §10 数据增强 / §11 用户行为与偏好发现 / §12 链路追踪 / §13 废弃·预留表 / §14 框架表——**§14 名不副实**：其 220 行中 schema_migrations 仅 8 行，其余为"文章三字段说明"（属内容域）与按域分节的"索引与约束总览 + 向量维度规则"（全局/分域内容）。
2. **`ER_DIAGRAM.md` 七个面与字段域是多对多交叉**：`board_persistent_topics` 出现在 2 个面、`semantic_labels` 3 个面、`topic_tags` 3 个面；Core 面横跨内容/队列/偏好三域。不存在一比一搬运路径。
3. **api/ 目录先例**：`_index.md`（导航）+ `_conventions.md`（公共约定）+ 按领域文件，已验证的组织模式。

## Goals / Non-Goals

**Goals**

- 任一域的"表字段 + 域 ER 图 + 域级索引"单文件查全（单次 read，≤300 行/文件）。
- 全局事实（FK 真相、DB FK 清单、CHECK、向量维度规则、pgvector/全文检索索引）单点集中。
- 旧文件移除后全仓库无死链；表数守恒（50 业务 + 5 废弃 + 1 框架各出现且仅出现一次）。

**Non-Goals**

- 不改 api/ 目录、DATA_LIFECYCLE.md、任何代码与 harness 行为。
- 不迁移两份旧文档的"更新日志"历史节（flow 域变更溯源才是正主，database 文档日志价值衰减，doc-authoring.md 亦未强制）。
- 不重写字段字典内容本身（搬运为准，仅修搬运中发现的失配锚点）。

## Decisions

### D1: 切分粒度 = 12 个域文档，一比一映射 §1~§12（+deprecated）

去 § 编号，文件名用表语义自命名（不强行对齐 flow 域名——flow 是行为域、表是数据域，边界天然错位，强行对齐会造出怪名）：

`tables/` 下：`content` / `scheduling-config` / `ai-routing` / `topic-tags` / `semantic-labels` / `embeddings` / `job-queues` / `daily-report-watch` / `data-enrichment` / `preference-discovery` / `tracing` / `deprecated-framework`（§13 废弃预留 + §14 的 schema_migrations）。

*Alternative 否决*：合并小域（§6+§7、§2+§12）成 ~8 文件——省 4 个文件但引入二次归位决策，且小域独立后与 flow 域（如 scheduler→§2）对照更直。

`_index.md` 提供「flow 域 ↔ 域文档」对照列弥合两套命名的错位。

### D2: ER 图按域重绘子图，原七面废弃

每域文档内嵌一张 Mermaid `erDiagram` 子图：**本域实体完整展开字段，外域直接邻居仅画实体名节点**（bounded-context 视图惯例）。原七面已接受跨面重复实体（board_persistent_topics×2、semantic_labels×3），域图重复邻居节点不是新增债，是既有模式延伸。

- 丢弃 `ER_DIAGRAM.md` 文件；其"全局概览 ASCII 图"→ `_index.md`，"FK 引用矩阵 Part A/B + 关系模式说明"→ `tables/_conventions.md`，"AI Summaries（已废弃）面"简化后随废弃表进 `deprecated-framework.md`。
- **重绘纪律**：每张子图的关系边集合必须 ⊆（原七面边集 ∪ FK 矩阵行）；重绘用 diagram-design skill 起稿/校验（Mermaid 源级重绘），禁止凭记忆新造边。

*Alternative 否决*：七面按"主域"整体搬运 + 跨域链接——正是当前 FIELDS/ER 双文件跳转痛味的翻版，且一个面横跨 3 域时链接网更乱。

### D3: 全局内容收拢 = `_index.md`（导航+全局概览）+ `tables/_conventions.md`（约定+全局约束）

对齐 api/ 目录 `_conventions.md` 先例。分配：

| 内容块 | 去处 |
| --- | --- |
| 阅读约定 5 条（表名/FK 真相/向量维度/字段列含义/枚举） | `tables/_conventions.md` |
| 完整表清单（50+5+1）+ flow 域对照列 | `_index.md` |
| 索引与约束总览中**按域分节**的部分（语义标签域索引/叙事域索引/偏好发现域索引/日报域索引…） | 拆入对应域文档尾部「索引」节 |
| 索引总览中**全局**的部分（pgvector 扩展/全文检索/性能索引/唯一约束/CHECK/DB FK 清单） | `tables/_conventions.md` |
| 向量维度规则总述 | `tables/_conventions.md` |
| 文章三个内容字段说明 | `tables/content.md`（属内容域） |
| 旧 `_index.md` 的迁移历史杂项 | 不迁移（Non-Goal） |

### D4: 引用修复清单（死链清零范围）

`architecture/overview.md`（3 处链接）、`flow/data-enrichment.md` 溯源表（`DATABASE_FIELDS.md §16` → 指向 `tables/data-enrichment.md`，溯源行文本只改链接目标不动历史叙述）、`openspec/specs/database-docs/spec.md` 主 spec（归档时由 delta 合并更新）。openspec/changes/archive/ 内历史链接**不改**（归档不可变）。

### D5: 迁移方式

一次性 `write` 新文件集 + `git rm` 两旧文件，不留壳不留 redirect（死链清零是验收项而非兼容目标）。回滚 = `git revert` 单提交。

## Risks / Trade-offs

- **[重绘 ER 子图引入漂移]** → D2 重绘纪律（边集 ⊆ 原边集 ∪ FK 矩阵）+ tasks 中逐图核对步骤（对照 postgres_migrations.go 抽查）。
- **[切分漏表/漏内容]** → 表数守恒硬检查：新 `_index.md` 表清单与 12 域文档小节标题逐一对应，50+5+1 各出现一次（spec Scenario 3 同向）。
- **[维护成本微增]**（新表需先判域归属） → `_index.md` 表清单按域分组即归属速查表。
- **[域图重复邻居节点与原图漂移共存]** → 邻居节点只标实体名无字段，字段差异天然不可见；FK 真相以 `_conventions.md` 为唯一权威。

## Migration Plan

docs-only，无部署/数据影响。合并即生效；纯新增文件无旧数据降级问题。

## Open Questions

（无——域命名与映射均已定案，实施中如遇 § 未列出的游离内容块按"字段字典→所属域、跨域→_conventions"兜底规则归位，不改变方案结构。）
