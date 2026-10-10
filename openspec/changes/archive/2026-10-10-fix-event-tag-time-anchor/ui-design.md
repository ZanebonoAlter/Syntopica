<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为纯后端修复：替换标签抽取/描述生成 LLM prompt 中的污染示例、增加时间锚纪律约束，并一次性重生成存量病态 event 标签的 description。不新增或修改任何前端界面、交互或布局（proposal 已声明 ui-impact: none）。唯一用户可见影响是数据内容层面——标签描述与日报线索文案的时间锚恢复正确，无需 UI 设计介入。
