<!-- complexity: simple -->
<!-- ui-impact: none -->

## Why

change 制品目前几乎全是文字。抽查最近 15 个归档 change：**14 个零 mermaid**，唯一命中的一处还是任务行里提到「重画 mermaid」，指的是 SRS 文档而非 change 制品。结果是三类最该被看见的信息散落在长段落里，评审和半年后回溯都要在脑内重建：

- **用户故事核心路径**：从触发到完成经过哪些判断与分支
- **状态流转**：备份档、迁移、审批这类对象的合法状态与非法跃迁
- **跨层顺序**：脚本 → 数据库 → 应用启动这类调用时序，谁先谁后、哪一步失败会导致什么

项目本身已具备图表达的全部前提：`docs/reference/flow/` 10 篇文档全带 mermaid，前端有 `markdown-mermaid-render` spec 与对应渲染组件，`docs/reference/database/` 也大量用图。缺的不是能力，是 **change 制品的图表达范式**——没有固定放哪、画哪几类、和文字怎么分工，于是每人都写字。

本 change 先补一个真实样板（已归档的灾备恢复 change），再把范式写进规范，作为后续 change 的参考标准。

## What Changes

- **样板**：给已归档的 `2026-11-20-fix-disaster-recovery-channel` 补 `design-diagram.md`，含三类图：
  - 用户故事核心路径（flowchart）：灾难发生 → 恢复脚本 → 应用启动 → 三条根因的失败分支与修复后路径
  - 状态流转（stateDiagram-v2）：备份档「含/不含迁移历史」两种状态在旧写法与新写法下的迁移重放结果
  - 顺序图（sequenceDiagram）：备份 cron / 恢复脚本 / 应用启动三段时序
- **规范**：`docs/reference/开发执行规范.md` 新增 §13「change 图表达（design-diagram.md）」，规定：
  - 固定文件名 `design-diagram.md`（change 根，非 schema 制品，与 `explore-findings.md` / `review.md` 同性质，不阻断任何门禁）
  - 三类图的判据与 mermaid 类型（flowchart / stateDiagram-v2 / sequenceDiagram），以及「什么时候不必画」
  - 图与文字的分工：图负责路径、状态、时序；文字负责约束、取舍、失败原因。图不得替代 Decision 的「结论/落点/取舍」三行
  - 归档 change 补图属于 §12 补档范畴，只增不改原制品
- **推广**：同一文档 §0.6 步骤 1（context 建立）与 §9 产出物清单各加一句指引（涉及多步流程/状态机/跨层调用时先画图）。

## Capabilities

### New Capabilities

- `change-diagram-convention`: change 制品图表达的契约——文件位置、三类图的适用判据、与文字的分工及归档补档规则

### Modified Capabilities

（无）

## Impact

- **新增文档**：`openspec/changes/archive/2026-11-20-fix-disaster-recovery-channel/design-diagram.md`（样板，3 图）。
- **改动文档**：`docs/reference/开发执行规范.md`（新增 §13 + §0.6 步骤 1 指引 + §9 产出物清单一行）。
- **不改**：schema 模板（`openspec/schemas/syntopica-ui/`）、业务代码、数据库、前端。本 change 刻意不动 `templates/design.md`——先用非 schema 制品跑通样板，避免一次性改动所有 change 的制品形状。
- **部署后用户可见变化**：无界面变化。后续 change 评审时多一份可选图文档。
- **旧数据降级**：不涉及。存量 change 不强制补图，只有涉及多步路径/状态流转/跨层顺序时才建议补。
