## ADDED Requirements

### Requirement: 文章行可标注疑似 AI 生成

阅读页文章列表 SHALL 支持在文章行内呈现「疑似 AI 生成」标记。标记 SHALL 以封面缩略图右上角的紧凑角标形态呈现（两个字符的短标「AI」，中性底色 + 描边），完整文案「疑似 AI 生成」SHALL 经 `title` 与无障碍名（`aria-label`）提供；SHALL NOT 以大面积色块或整行底色呈现，SHALL NOT 改变列表行的固定行高与虚拟列表的行高假设。

角标 SHALL NOT 占用 meta 行（时间 / 来源名 / 处理状态图标）的横向空间，SHALL NOT 被任何祖先的 `overflow` 裁切。同一行内时间、来源名与处理状态图标的可见宽度 SHALL 与无角标的行一致——来源名 MUST NOT 因角标被压到零宽。

标记仅在该文章判定状态为疑似时呈现；未判定与已确认非 AI 的文章 SHALL 不呈现任何标记，SHALL NOT 出现占位或「未检测」文案。

#### Scenario: 疑似文章行显示角标

- **WHEN** 列表中某篇文章的判定状态为疑似 AI 生成
- **THEN** 该行封面缩略图右上角 SHALL 呈现短标「AI」角标
- **AND** 该角标的 `aria-label` 与 `title` SHALL 为完整文案「疑似 AI 生成」

#### Scenario: 角标不吃 meta 行宽度

- **WHEN** 列表中某篇文章带疑似 AI 角标
- **THEN** 该行时间 / 来源名 / 处理状态图标的可见宽度 SHALL 与同内容的无角标行一致
- **AND** 角标 SHALL 完整可见，其矩形 SHALL NOT 超出封面的可视区域（不被 `overflow` 裁切）

#### Scenario: 未判定文章行不显示角标

- **WHEN** 列表中某篇文章的判定状态为未判定
- **THEN** 该行 SHALL NOT 呈现疑似 AI 角标，也 SHALL NOT 出现占位文案

#### Scenario: 角标不破坏行高

- **WHEN** 列表中出现带疑似 AI 角标的文章行
- **THEN** 该行高度 SHALL 与无角标行一致，虚拟列表的行高假设 SHALL 保持不变

#### Scenario: 列表不提供按疑似状态过滤

- **WHEN** 用户在阅读页列表头部查看筛选入口
- **THEN** SHALL NOT 出现「隐藏疑似 AI 文章」开关或按疑似状态过滤列表的入口
