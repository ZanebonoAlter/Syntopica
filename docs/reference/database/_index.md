# 数据库文档索引

RSS Reader 数据库全景概览。

## 概览

| 指标 | 值 |
|------|-----|
| 总表数 | **38** |
| FK 约束数 | **35** |
| 业务域 | **6**（Core、Topic Tags、AI Summaries、Narrative、Hierarchy、AI Infrastructure） |
| 枢纽表 | `topic_tags`（12 条入边，10 张表引用） |
| 向量表 | `topic_tag_embeddings`、`board_concepts`（pgvector） |
| 预留表 | 4 张（`ai_summaries` 系 + `digest_configs`） |

## 文档导航

| 文档 | 描述 |
|------|------|
| [DATABASE_FIELDS.md](DATABASE_FIELDS.md) | 38 张表的完整字段字典，含类型、约束、用途说明 |
| [ER_DIAGRAM.md](ER_DIAGRAM.md) | 全局实体关系图（ASCII 全局概览 + 6 域 Mermaid ER 图 + FK 引用矩阵） |
| [DATA_LIFECYCLE.md](DATA_LIFECYCLE.md) | 6 条数据链路的状态字段流转（4 条核心 + 2 条预留） |

## 如何阅读

1. **先看本页概览**：了解数据库的规模和结构
2. **[ER_DIAGRAM.md](ER_DIAGRAM.md)**：快速了解表之间的关系，找到 FK 依赖
3. **[DATABASE_FIELDS.md](DATABASE_FIELDS.md)**：按章节查阅具体表的字段定义
4. **[DATA_LIFECYCLE.md](DATA_LIFECYCLE.md)**：理解数据在系统各阶段如何流转和状态变迁

## 相关文档

- [项目架构总览](../architecture/overview.md) — 系统组件和子系统总览
- [数据流](../architecture/data-flow.md) — 代码执行流（"代码怎么跑的"）
- [开发指南](../development.md) — 构建、测试、验证命令

---

## 更新日志

### 2026-05-14

- 初始版本：全景概览 + 文档导航
