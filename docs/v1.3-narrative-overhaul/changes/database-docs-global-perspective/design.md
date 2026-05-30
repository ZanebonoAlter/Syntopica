## Context

`docs/reference/database/` 目前只有一个 `DATABASE_FIELDS.md`（744 行），以表级字段字典形式描述了 29 张表。实际 PostgreSQL 有 38 张表、35 条 FK 约束。缺少全局关系视角和数据生命周期视角。`architecture/data-flow.md` 覆盖代码执行流，但未从数据状态变迁角度描述。

已有的 Mermaid 图例惯例在 `architecture/overview.md` 中建立。

## Goals / Non-Goals

**Goals:**
- 提供全局实体关系图，让读者一眼看出表之间的 FK 依赖和域归属
- 提供数据生命周期文档，从状态字段变迁角度描述核心业务链路
- 补齐 `DATABASE_FIELDS.md` 至 38 张表覆盖
- 建立数据库文档目录索引
- 在 `DATABASE_FIELDS.md`、`data-flow.md`、`overview.md` 之间建立清晰的交叉引用

**Non-Goals:**
- 不修改运行时代码
- 不修改数据库 schema 或迁移
- 不重写 `data-flow.md` 的内容，只加交叉引用
- 不生成自动化文档工具或脚本

## Decisions

### D1: 文件拆分方案

在 `docs/reference/database/` 下新建 3 个文件，保持 `DATABASE_FIELDS.md` 不重命名：

```
database/
├── _index.md              # 全景概览 + 导航
├── DATABASE_FIELDS.md     # 字段字典（补齐 + 迁移章节）
├── ER_DIAGRAM.md          # 全局实体关系图
└── DATA_LIFECYCLE.md      # 数据生命周期
```

**替代方案**：把 ER 图和生命周期合并到 `DATABASE_FIELDS.md`——文件已 744 行，合并后超过 1200 行，不可取。

### D2: ER 图表达方式——混合 ASCII + Mermaid

- 全局概览：ASCII 分组图（域间关系，~15 个节点），30+ 节点 Mermaid 不可读
- 每域 ER：Mermaid `erDiagram`（每图 5-10 实体）
- FK 引用矩阵：纯表格（35 行）
- 关系模式说明：文字描述桥接表、自引用、反规范化等模式

**替代方案**：全部 Mermaid——全局图不可读；全部 ASCII——域级图不如 Mermaid 美观。

### D3: 生命周期链路覆盖

4 条核心链路 + 2 条预留功能：

| 链路 | 类型 |
|---|---|
| 文章生命周期 | 核心 |
| 主题标签生命周期 | 核心 |
| 阅读反馈生命周期 | 核心 |
| 叙事生成生命周期 | 核心 |
| AI 批量摘要 | 预留（0 行数据，无 Go 代码） |
| Digest 推送 | 预留（0 行数据，无 Go 代码） |

每条链粒度：状态字段流（标注涉及表和状态字段变迁），约 30-50 行/链。

### D4: DATABASE_FIELDS.md 章节重组

迁移出去：
- "工作流程"（570-614 行）→ `DATA_LIFECYCLE.md`
- "状态流转图"（617-632 行）→ `DATA_LIFECYCLE.md`
- "配置要求"（662-677 行）→ `DATA_LIFECYCLE.md`

保留：
- "三个内容字段的区别"（636-660 行）
- "数据库索引清单"（680-706 行）
- 各文件各自维护"更新日志"和"相关文档"

新增章节：
- §6.5 "AI 摘要关联表"（`ai_summaries`、`ai_summary_feeds`、`ai_summary_topics`）
- §8 "层级关系相关表"（`hierarchy_config`、`hierarchy_config_versions`、`adopt_narrower_queues`、`multi_parent_resolve_queues`）
- §14 "已废弃/预留表"（4 张孤儿表：`ai_summaries` 等、`digest_configs`）
- 补 `otel_spans` 到 §13 其他表
- 补 `articles.feed_summary_id` 到 articles 表字段说明

### D5: 与 data-flow.md 的边界

```
data-flow.md       = "代码怎么跑的"（函数调用链、API 调用、前端 store 交互）
DATA_LIFECYCLE.md  = "数据怎么变的"（哪些表被写入、状态字段怎么流转、数据产出依赖）
```

### D6: 交叉引用更新范围

- `architecture/overview.md`：相关文档章节加 `database/` 下文件链接
- `architecture/data-flow.md`：头部加一句指向 `DATA_LIFECYCLE.md`
- `DATABASE_FIELDS.md`：修正"相关文档"路径（当前路径 `docs/architecture/` 错误，应为 `docs/reference/architecture/`），加新文件链接

### D7: _index.md 定位

轻量全景概览（~40 行）：38 张表、6 个业务域、35 条 FK 的数字概要 + 3 个文件的导航链接。

## Risks / Trade-offs

- **维护同步风险**：3 个文件 + 1 个索引需要在 schema 变更时同步更新 → 每个文件维护自己的更新日志，`_index.md` 引用各文件而非重复内容
- **Mermaid 渲染依赖**：域级 ER 图依赖 GitHub/VSCode 的 Mermaid 渲染 → ASCII 全局图作为 fallback
- **孤儿表判断可能不准确**：4 张表标记为"已废弃/预留"基于当前代码扫描，可能遗漏动态 SQL 或迁移逻辑 → 标注"基于当前代码分析"保留修正空间
