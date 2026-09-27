# settings-workspace Specification

## Purpose

独立设置工作区，替代超长 `GlobalSettingsDialog`：领域导航组织设置模块，支持 URL 定位、主从编辑、长列表治理和响应式布局。设计细节见下方 Capability 节与 `front/app/features/settings/` 实现。

## Requirements

### Requirement: 独立设置工作区承载全局设置

全局设置 SHALL 由独立工作区（`SettingsWorkspace`）承载，导航按四个分组组织（内容管理 / AI 配置 / 数据源与网络 / 运行状态，共 7 个导航项），SHALL NOT 回退为单一超长全局对话框，SHALL NOT 恢复无分组的平铺导航。参考角色与分析方法卡相关的设置入口 SHALL NOT 存在。

#### Scenario: 设置入口进入工作区

- **WHEN** 用户从入口打开设置
- **THEN** 进入独立设置工作区，按四个分组导航展示 7 个设置项（非全局弹窗、非平铺列表）

#### Scenario: URL 定位设置分区

- **WHEN** 设置工作区处于某领域分区
- **THEN** 该状态 SHALL 可通过 URL 定位（刷新/分享后还原到同一分区；复合 section 的子 tab 由 `tab` 参数承载）

### Requirement: 设置导航分组与复合 section

设置导航 SHALL 由四个分组标题与组内导航项构成：内容管理（订阅源/兴趣画像/页边注）、AI 配置（AI 模型/能力路由）、数据源与网络（1 个复合 section）、运行状态（1 个复合 section）。分组标题 SHALL 不可点击。

#### Scenario: 分组导航渲染

- **WHEN** 用户打开设置工作区
- **THEN** 侧栏按「内容管理 / AI 配置 / 数据源与网络 / 运行状态」四个分组标题渲染全部 7 个导航项

#### Scenario: 复合 section 子 tab 切换与 URL 承载

- **WHEN** 用户在「数据源与网络」内从 Firecrawl 切到出站代理子 tab
- **THEN** URL 查询参数更新为 `section=datasources-network&tab=proxy`，刷新后仍停留在该子 tab

#### Scenario: 旧 section 键深链重定向

- **GIVEN** URL 指向旧键 `?section=proxy`（或 firecrawl/bocha/searxng/rsshub/datasources/ai-health/queues/schedulers 任一）
- **WHEN** 页面加载
- **THEN** 工作区 SHALL 重定向到对应新键（`datasources-network` 或 `runtime-status`）并携带 `tab=<旧键>`，不出现 404 或空白

#### Scenario: 非法 tab 值回退

- **GIVEN** URL 含 `section=datasources-network&tab=不存在值`
- **WHEN** 页面加载
- **THEN** 回退到该复合 section 的默认子 tab（Firecrawl）并清除非法 tab 参数

#### Scenario: 复合 section 进入默认子 tab

- **WHEN** 用户分别进入「数据源与网络」与「运行状态」
- **THEN** 默认分别落在 Firecrawl 与 AI 健康子 tab，不跨 section 记忆上次子 tab

### Requirement: 日报生成时刻设置可用

定时任务设置中的日报生成时刻 SHALL 可读取真实配置并保存成功（读写走 `/api/ai/settings` 端点），保存后调度器 SHALL 在下一轮配置读取（分钟级）生效。

#### Scenario: 打开定时任务设置显示真实值

- **WHEN** 用户打开运行状态 section 的定时任务子 tab
- **THEN** 日报时刻输入框显示库中 `daily_report_time` 的真实值（非硬编码默认值）

#### Scenario: 保存后生效

- **WHEN** 用户修改日报时刻并保存成功
- **THEN** 出现成功反馈，且后端日志无 404；下一轮调度窗口读取使用新时刻

## Capability

独立设置工作区，替代超长 `GlobalSettingsDialog`。工作区通过领域导航组织设置模块，支持 URL 定位、主从编辑、长列表治理和响应式布局。

## Navigation

设置入口 SHALL 导航到 `/settings`。当前 section SHALL 通过 URL 查询参数或子路由保存。

### Sections

- `feeds`
- `ai-providers`
- `capability-routes`
- `embedding`
- `queues`
- `preferences`
- `firecrawl`
- `schedulers`

### Scenario: Open settings

- **WHEN** 用户点击应用 Header 的设置按钮
- **THEN** 应用导航到设置工作区
- **AND** 默认打开 `feeds`
- **AND** 浏览器返回可回到之前页面

### Scenario: Restore section

- **GIVEN** URL 指向 `section=embedding`
- **WHEN** 页面刷新
- **THEN** 工作区仍显示 Embedding section

## Layout

桌面端 SHALL 使用侧栏导航和独立内容区。窄屏 SHALL 使用适合小宽度的导航，不得把全部 section 挤压为多行横向 tab。

### Scenario: Stable workspace height

- **WHEN** 用户在长内容 section 和 Firecrawl 短表单之间切换
- **THEN** 工作区外框位置和主要高度保持稳定
- **AND** 仅内容区独立滚动

### Scenario: Narrow viewport

- **GIVEN** viewport 宽度为 600px
- **WHEN** 用户打开设置
- **THEN** section 导航可完整访问
- **AND** 不出现水平溢出或被压缩到难以辨识的标签

## Feed Settings

订阅源设置 SHALL 使用主从编辑模式，不得默认同时挂载所有订阅源编辑表单。

### Scenario: Select a feed

- **GIVEN** 订阅源列表已加载
- **WHEN** 用户选择一个订阅源
- **THEN** 仅该订阅源的编辑器显示在详情区
- **AND** 其他订阅源不挂载完整表单

### Scenario: Find a feed

- **WHEN** 用户按名称搜索或展开分类
- **THEN** 列表即时缩小到相关订阅源
- **AND** 分类默认可折叠

## AI and Embedding Settings

AI 提供商、能力路由和 Embedding SHALL 是独立 section，不得继续堆叠在一个“通用设置”长页面中。

### Scenario: Edit provider

- **WHEN** 用户选择一个模型提供商
- **THEN** 工作区展示该提供商详情和局部保存/测试操作
- **AND** 能力路由与 Embedding 配置不同时渲染

### Scenario: Open AI providers section

- **WHEN** 用户进入 `AI 模型` section
- **THEN** 页面只渲染主模型和备用提供商管理
- **AND** 不渲染能力路由编辑器
- **AND** 不渲染板块匹配阈值

## Long Lists

队列、阅读偏好和其他长列表 SHALL 使用分页、窗口化或受限默认条数。

### Scenario: Queue history

- **WHEN** 用户进入队列 section
- **THEN** 首屏显示摘要和有限数量的最近记录
- **AND** 用户可分页或进入完整记录视图

### Scenario: Preference sources

- **WHEN** 阅读偏好包含大量来源
- **THEN** 用户可搜索、排序和分页
- **AND** 统计摘要保持在列表顶部

### Scenario: Bounded initial rendering

- **WHEN** 用户首次进入队列或阅读偏好 section
- **THEN** 页面只挂载当前活动视图和当前页数据
- **AND** 非活动队列列表不得同时保留在 DOM
- **AND** 不能以存在分页控件但仍渲染完整数据集的方式满足本要求

## Theme and Accessibility

工作区及所有 section SHALL 响应 `editorial` / `dark`，并使用语义 token。辅助文字不得通过重复 opacity 降低到难以辨认。

### Scenario: Theme switch

- **WHEN** 用户在设置工作区切换主题
- **THEN** 导航、表单、列表、状态和图表同时更新
- **AND** 当前 section 与未保存输入不丢失

### Scenario: Scheduler labels

- **WHEN** 用户进入定时任务 section
- **THEN** 已知任务和状态使用可读中文文案
- **AND** 技术标识作为次要信息保留
- **AND** 状态文字在 dark 下保持清晰可辨

## Migration

- 复用现有 API 和业务 composable。
- 将 `GlobalSettingsDialog` 入口替换为路由导航。
- 面板组件迁移为 section 组件后，删除 Dialog 专用导航和尺寸逻辑。
- `AppDialog` 继续承载短流程编辑和确认。
