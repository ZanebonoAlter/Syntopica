# UI Design — split-retro-warn-classes

<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A（无 UI 影响）

本 change 是纯工具链改动（harness-retro shell 报告脚本的 ④ 段分类口径 + smoke fixture + 两份文档），产出为终端文本报告与 CLI JSON 输出，不涉及 front/ 任何页面、组件、布局契约。无原型、无双视口验收需求。

报告文本的可读性约定（非 UI 契约，实现自查）：④ 段沿用现有分段标题 + 「指标名 + 数值」行格式；新增情报型汇总行沿用既有缩进与「（无…）」占位样式；标题追加「（纠正型；…）」限定词不改分段编号与骨架。
