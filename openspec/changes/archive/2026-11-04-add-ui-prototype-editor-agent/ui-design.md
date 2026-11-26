# UI Design: add-ui-prototype-editor-agent

<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为**纯 agent 工具链**调整，不触及任何产品界面：新增的是子线程档位定义（`.pi/agents/ui-design.md`）与门禁判定收窄（`.pi/extensions/`），无前端组件、无信息架构、无交互模式、无布局模式变更。

产品侧的用户可见行为完全不变——UI 合同未就绪时对**前端实现写入**的阻断行为照旧（`ui-design-gate` 的 `mutation` 判定未放宽）。变化只在 agent 侧：主会话在合同未就绪期间可派只读子线程勘察现状、可派原型编辑档调整 `ui-prototype/` 原型。

## 复用契约

- 不新增任何页面、组件或布局模式；不引入新的前端依赖。
- 原型编辑档的产出物落在 `openspec/changes/<change>/ui-prototype/`（既有目录约定），与 major 档既有原型资产（如 `board-health-triage-advice/ui-prototype/`）同构。
- `ui-design.md` 八节结构与审批 marker 语义不变（`ui-approval: approved` 仍只能由用户在对话中明确确认后写入）。
