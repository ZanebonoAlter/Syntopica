<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计 — N/A

## 影响等级

`none`。本 change 只修改 `docs/reference/standard/frontend/` 下的设计规范文本、`docs/reference/开发执行规范.md` 的编排纪律、以及 `openspec` 的 spec 基线，**不产生任何用户可见界面**，不新增/修改/删除页面、面板、弹窗、导航或多步流程。

## N/A 理由

- 无前端代码改动（`front/` 零 diff）。
- 无新增组件、无布局模式变化、无交互路径变化。
- 受本 change 约束的对象是「后续 change 的 `ui-design.md` 与原型制品形态」，不是产品界面本身。

## 验收方式

本 change 无法用 agent-browser 或视口截图验证（无可渲染对象），验收改为**文档一致性校验**：

1. `bash scripts/harness/check-standards.sh` 无阻断项；
2. `bash scripts/harness/doc-impact.sh verify openspec/changes/add-ui-contract-ladder` 无阻断项；
3. `openspec validate add-ui-contract-ladder` 通过；
4. 人工对读：`interaction.md` 的模式词表覆盖 `layout.md` 中出现的全部浮层/布局形态，无互斥矛盾。

## 原型

`ui-prototype: none`。本 change 不产出原型；`interaction.md` 中定义的「状态图 + 操作表」形态本身是产物规范，不是本 change 的原型交付物。
