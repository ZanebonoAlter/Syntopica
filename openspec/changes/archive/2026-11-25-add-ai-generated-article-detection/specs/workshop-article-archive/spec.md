## ADDED Requirements

### Requirement: 归档视图文章行标注疑似 AI 生成

版块归档视图的每篇文章行 SHALL 支持在标签带末尾呈现「疑似 AI 生成」chip。该 chip SHALL 位于文章行既有标签带内，SHALL NOT 占用「入板」chip 的位置或步号语义，SHALL NOT 使用标签规则色边框体系（规则色仅表达匹配规则）。

归档视图 SHALL NOT 因此新增筛选维度——既有三个筛选（全部／未纳入日报／弱证据）保持不变；该 chip SHALL 不参与「最强匹配」表达，也 SHALL NOT 改变「入板 chip」步号与「进日报」标记口径。

标记仅在该文章判定状态为疑似时呈现；未判定与已确认非 AI 的文章 SHALL 不呈现任何标记。

#### Scenario: 疑似文章归档行显示 chip

- **WHEN** 归档视图某篇文章行的判定状态为疑似 AI 生成
- **THEN** 该行标签带末尾 SHALL 呈现「疑似 AI 生成」chip

#### Scenario: 未判定文章归档行不显示 chip

- **WHEN** 归档视图某篇文章行的判定状态为未判定
- **THEN** 该行 SHALL NOT 呈现疑似 AI chip，SHALL NOT 出现占位文案

#### Scenario: chip 不影响入板 chip 与筛选维度

- **WHEN** 归档视图某篇文章同时命中版块标签且判定状态为疑似
- **THEN** 该行 SHALL 同时呈现「入板」chip 与「疑似 AI 生成」chip，且归档视图的筛选维度 SHALL 仍为既有三个

#### Scenario: chip 不改变归档排序口径

- **WHEN** 归档视图按既有质量序或时间序渲染文章
- **THEN** 排序结果 SHALL NOT 因疑似 AI 标记而改变
