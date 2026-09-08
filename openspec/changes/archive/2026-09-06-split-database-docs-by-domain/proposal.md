<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

`docs/reference/database/DATABASE_FIELDS.md` 已膨胀到 1433 行 / 99.5KB（50 张业务表 + 废弃表 + 框架表全在一个文件），超过 AI read 工具单次 50KB 上限——跨表对照需翻页 2-3 次并人工拼装；同时域级 ER 图在 `ER_DIAGRAM.md`、字段字典在 `DATABASE_FIELDS.md`，查一个域要跨两个大文件跳转。切分动机是 AI 检索体验与文档人体工学，**不是** harness 改进：api/ 与 database/ 均不在 constraint-injection 扫描范围，本 change 不改变任何 harness 行为（刻意保持，因文档是代码的投影，注入投影有漂移风险）。

## What Changes

- **切分 DATABASE_FIELDS.md → `docs/reference/database/tables/` 下 12 个域文档**：按现有 §1~§14 域边界一比一切分（去掉 § 编号历史包袱，含 §8 跳号），每文档 30~220 行，单次 read 无压力。
- **ER_DIAGRAM.md 的域级 ER 图并入对应域文档**：每域文档 = 字段字典 + 该域 ER 图，一个域一个文件查全。7 个 ER"面"与 12 个字段域的映射关系在 design.md 定。
- **全局内容收拢**：阅读约定（全局事实）、完整表清单、FK 真相、FK 引用矩阵、关系模式说明 → 精简后的 `_index.md`（或独立全局文件，design.md 定）；§13 废弃表 + §14 框架表 → 合并为 `tables/deprecated-framework.md`。
- **移除 `DATABASE_FIELDS.md` 与 `ER_DIAGRAM.md` 两个源文件**（内容全部迁移，不留壳）。
- **`_index.md` 重写**：从 230 行杂烩（表清单 + 索引 + 迁移历史）改为纯导航——域文档链接表 + 全局约定入口。
- **外部引用修复**：`architecture/overview.md` 3 个链接、`flow/data-enrichment.md` 溯源表中的 `DATABASE_FIELDS.md §16` 引用（该 § 编号已失配，顺手指向新位置）。
- **明确不做**：api/ 目录不动（已按领域切好）；DATA_LIFECYCLE.md 不动；不接入 constraint-injection / 不加 doc-impact-applies 标签；不改任何代码。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `database-docs`: 现存 spec 的两个 Requirement 均锚定 `DATABASE_FIELDS.md` / `DATA_LIFECYCLE.md` 文件名与章节结构；切分后 `DATABASE_FIELDS.md` 移除，需将准确性契约改写为锚定新域文档结构，并新增域文档组织契约（每域文档 = 字段字典 + ER 图，全局约定集中于入口文档）。

## Impact

- **纯文档 change，零代码影响**：不触及 backend-go/、front/、.pi/extensions/、scripts/。
- **harness 零变化**：api/ 与 database/ 本就不在 constraint-injection 扫描范围（无 doc-impact-applies 标签），quality-gate/spec-gate 不消费这些文档；doc-impact.sh 归档预勾选为域级粒度（"动没动 database 文档"），不受文件切分影响。
- **AI 协作收益**：单域文档 5-20KB 单次 read 完成；域粒度与 flow/ 文档域对齐（content / daily-report / data-enrichment / discovery…），为将来 flow↔表文档互链预留结构。
- **维护成本**：新增表时从"往 §N 追加"变为"打开对应域文件追加"，多一次域归属决策（_index.md 表清单兜底）。
