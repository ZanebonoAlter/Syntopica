## MODIFIED Requirements

### Requirement: 简介导语段按实质内容呈现

文章导语 MUST 仅在有实质内容时以标题下导语段呈现（浅色、细左线、无边框）；为空、纯空白、纯图片、纯符号或与正文重复时 MUST NOT 渲染任何占位块。导语文本来源 MUST 按优先级取值：详情接口的 `description`（权威，由 `GET /api/articles/:id` 提供）优先；详情未就绪或详情 `description` 为空时 SHALL 回退列表项的 `excerpt`（纯文本，≤200 字符）。段落结构、guard 判定与去重逻辑 MUST 保持不变。

#### Scenario: 有实质内容

- **WHEN** 文章 description 含与正文不重复的实质文本
- **THEN** 标签行下呈现导语段，无边框卡片

#### Scenario: 无实质内容

- **WHEN** description 为空、纯图片标签、仅空白字符或与正文归一化后相同
- **THEN** 不渲染导语段，不留空白占位块

#### Scenario: 详情未就绪时的首帧导语

- **WHEN** 用户点选一篇文章，列表项不再携带完整 HTML `description`（只有 `excerpt`），且详情请求尚未返回
- **THEN** 导语段 SHALL 以 `excerpt` 文本呈现（不留空白占位、不产生布局跳动）；详情返回后 SHALL 切换为详情 `description`

#### Scenario: 详情返回后以详情为准

- **WHEN** 详情接口返回的 `description` 与列表 `excerpt` 内容不同
- **THEN** 导语段 SHALL 呈现详情 `description` 的去标签文本
