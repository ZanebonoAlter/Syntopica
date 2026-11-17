## ADDED Requirements

### Requirement: 时间视窗可折叠为一行摘要

时间视窗控件 SHALL 提供折叠/展开能力：

- 标题行 SHALL 是可点击的折叠开关，带 `aria-expanded` 与方向指示（▾ 展开 / ▸ 收起）。
- 折叠时 SHALL 只渲染一行摘要，内容为「时间视窗 · <当前档位> · <起始 MM-DD>~<结束 MM-DD>」；
  档位按钮、日期表单、图注、趋势曲线、操作说明、反馈行 SHALL 全部隐藏。
- 默认态 SHALL 为：**窄视口（<768px）收起、宽屏展开**。
- 用户手动切换后 SHALL 记住该选择（`localStorage`），且手动选择 SHALL 优先于窄屏默认。
- 折叠 SHALL NOT 改变受控语义：`range` / `sourceDays` / `series` 仍由上层给定，
  折叠只是视觉收起，SHALL NOT 触发任何 `preview` / `commit` / `source-change` 事件。

#### Scenario: 折叠后只留一行摘要

- **WHEN** 用户在宽屏点击标题行收起
- **THEN** 控件高度 SHALL 从约 260–300px 降为单行（约 32–36px）
- **AND** 该行 SHALL 显示当前档位与起止日（如「时间视窗 · 近 30 天 · 10-19~11-17」）
- **AND** 档位按钮、日期表单、趋势曲线 SHALL 不可见且不可 Tab 聚焦

#### Scenario: 窄屏默认收起、宽屏默认展开

- **WHEN** 视口宽度 < 768px 且用户未做过手动选择
- **THEN** 控件 SHALL 默认处于收起态
- **WHEN** 视口宽度 ≥ 768px 且用户未做过手动选择
- **THEN** 控件 SHALL 默认处于展开态

#### Scenario: 手动选择覆盖默认并持久化

- **WHEN** 用户在窄屏手动展开控件
- **THEN** 刷新页面后 SHALL 仍保持展开（手动选择优先于窄屏默认）
- **AND** 切到宽屏后 SHALL 同样保持该选择

#### Scenario: 折叠不触发范围变更

- **WHEN** 用户点击标题行切换折叠态
- **THEN** SHALL NOT 触发 `preview` / `commit` / `source-change`
- **AND** 当前 `range` 与 `sourceDays` SHALL 保持不变
