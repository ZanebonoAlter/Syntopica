<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计 — N/A

## 影响等级

`none`。本 change 只改 `docs/reference/standard/frontend/` 下的规范文本，不产生任何用户可见界面。

## N/A 理由

- 无前端代码改动（`front/` 零 diff）。
- 无新增组件、无布局模式变化、无交互路径变化。
- 被本 change 约束的对象是「后续 change 的原型内容质量」，不是产品界面本身。

## 验收方式

无可渲染对象，验收改为文档一致性校验：

1. `bash scripts/harness/check-standards.sh` 无阻断项；
2. `bash scripts/harness/doc-impact.sh verify openspec/changes/add-prototype-slop-filter` 无阻断项；
3. `openspec validate add-prototype-slop-filter` 通过；
4. `interaction.md` 行数 ≤220（本 change 自设预算），且新增三节的判据均可被人工逐条核对。

## 原型

`ui-prototype: none`。
