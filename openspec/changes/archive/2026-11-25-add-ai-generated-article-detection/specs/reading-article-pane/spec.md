## ADDED Requirements

### Requirement: 文章详情呈现疑似 AI 标记

文章详情面板 SHALL 在既有标签区呈现「疑似 AI 生成」标记，SHALL NOT 把该标记做成卡片或改变阅读列版式（reader 版式去卡片化约束保持不变）。该标记 SHALL 为静态展示，SHALL NOT 复用带操作能力的标签组件。

标记仅在该文章判定状态为疑似时呈现；未判定与已确认非 AI 的文章 SHALL 不呈现任何标记。

#### Scenario: 疑似文章详情显示标记

- **WHEN** 用户打开一篇判定状态为疑似 AI 生成的文章详情
- **THEN** 详情标签区 SHALL 呈现「疑似 AI 生成」标记

#### Scenario: 未判定文章详情不显示标记

- **WHEN** 用户打开一篇判定状态为未判定的文章详情
- **THEN** 详情标签区 SHALL NOT 呈现疑似 AI 标记，也 SHALL NOT 出现占位文案

### Requirement: 文章详情可人工标记与取消疑似 AI

文章详情的「手动操作」菜单 SHALL 提供「标记为疑似 AI 生成」与「取消疑似 AI 标记」两项操作。操作 SHALL 就地生效并即时反映到界面，SHALL NOT 弹出二次确认弹窗；操作失败 SHALL 以既有菜单内联错误位呈现。

手动结论 SHALL 优先于自动结论：已人工标记或人工取消的文章，其后续自动判定 SHALL NOT 覆盖人工结论。

#### Scenario: 手动标记后界面即时反映

- **WHEN** 用户在文章详情菜单选择「标记为疑似 AI 生成」
- **THEN** 该文章 SHALL 立即呈现疑似 AI 标记，标记来源 SHALL 为人工标记

#### Scenario: 手动取消后界面即时反映

- **WHEN** 用户在文章详情菜单选择「取消疑似 AI 标记」
- **THEN** 该文章 SHALL 不再呈现疑似 AI 标记

#### Scenario: 操作失败内联报错

- **WHEN** 手动标记请求失败
- **THEN** 菜单 SHALL 内联展示错误信息，SHALL NOT 以弹窗或整页错误页呈现

#### Scenario: 手动结论不被自动判定覆盖

- **WHEN** 用户已手动取消疑似标记，随后该文章的自动判定完成且置信分高于阈值
- **THEN** 该文章 SHALL 仍不呈现疑似 AI 标记
