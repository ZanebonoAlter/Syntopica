## Context

`design-diagram.md` 在项目里已有同类先例（`explore-findings.md`、`review.md`、`human-feedback.md`、`stage.md` 都是非 schema 制品，不参与 DAG、不阻断门禁）。前端 mermaid 渲染链路已存在（`markdown-mermaid-render` spec + `reading-article-pane`），`docs/reference/flow/` 与 `database/` 已有大量图。本 change 只补 change 制品这一层的范式与样板。

## Goals / Non-Goals

**Goals:**
- 让「用户故事核心路径 / 状态流转 / 顺序图」有一个固定、可预期的落点。
- 用真实复杂 change 做样板，证明三类图能替代哪部分文字、不能替代哪部分。

**Non-Goals:**
- 不改 schema 模板（`openspec/schemas/syntopica-ui/templates/design.md`）。
- 不引入自动门禁或 mermaid lint 工具链。
- 不要求存量 change 补图。

## Decisions

### D1. 用 change 根的非 schema 制品，而不是改 schema 模板

- **结论**：固定文件名 `design-diagram.md`，与 `review.md` / `explore-findings.md` 同性质。
- **落点**：`openspec/changes/add-change-diagram-convention/specs/change-diagram-convention/spec.md`
- **取舍**：给 `templates/design.md` 加可选图节（新 change 直接看到图位、约束更强，但一次性改变所有 change 的制品形状，且与 major UI 已占用的 ui-design 审批链叠加，改动面与收益不成比例）

### D2. 样板选灾备恢复 change 而非 UI change

- **结论**：选 `2026-11-20-fix-disaster-recovery-channel`。
- **落点**：`openspec/changes/archive/2026-11-20-fix-disaster-recovery-channel/design-diagram.md`
- **取舍**：选 UI 重做类 change（用户路径更直观，但状态机与跨层顺序弱，三类图凑不齐；灾备 change 同时具备分支、状态集合与三段时序，能一次示范三类图的边界）

### D3. 只写规范与样板，不做自动校验

- **结论**：不新增脚本、不加归档门禁。
- **落点**：`docs/reference/standard/shared/doc-authoring.md`
- **取舍**：加 mermaid 语法校验（能抓语法错，但「图画得对不对」无法自动判，且要为图引入新依赖，违反「不主动加工具」约束；语法错在评审渲染时一眼可见）

### D4. 图不替代 Decision 三行

- **结论**：`design.md` 的「结论 / 落点 / 取舍」保持文字，图只承担路径、状态、时序。
- **落点**：`openspec/changes/add-change-diagram-convention/specs/change-diagram-convention/spec.md`
- **取舍**：允许用图替代长段落决策（更短，但「被否方案与不选理由」恰恰是最难画成图、也最需要在归档后回看的部分）

## Risks / Trade-offs

- [图会随实现漂移，代码改了图没改] → 归档 review 时把「图与实现是否一致」列为检查项；图只画稳定骨架（模块与状态），不画易变的字段名。
- [mermaid 语法版本差异导致本地能渲染、CI/其他渲染器报错] → 只用三类图的稳定子集（flowchart 的节点与边、stateDiagram-v2 的状态与转移、sequenceDiagram 的参与者与消息），不用新特性。
- [样板选的是已归档 change，补档可能被视为改历史] → 只新增文件、不改任何原制品；规范里写明属 §12 补档范畴。

## Migration Plan

无数据迁移。规范落地后，新 change 按需使用；存量 change 不动。

## Open Questions

（无）
