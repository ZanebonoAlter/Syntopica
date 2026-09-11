# settings-workspace Specification

## Purpose

独立设置工作区，替代超长 `GlobalSettingsDialog`：领域导航组织设置模块，支持 URL 定位、主从编辑、长列表治理和响应式布局。设计细节见下方 Capability 节与 `front/app/features/settings/` 实现。

## Requirements

### Requirement: 独立设置工作区承载全局设置

全局设置 SHALL 由独立工作区（`SettingsWorkspace`）承载，按领域导航分区组织（订阅源/AI 与 embedding/主题与无障碍等），SHALL NOT 回退为单一超长全局对话框。

#### Scenario: 设置入口进入工作区

- **WHEN** 用户从入口打开设置
- **THEN** 进入独立设置工作区，按领域导航分区展示设置模块（非全局弹窗）

#### Scenario: URL 定位设置分区

- **WHEN** 设置工作区处于某领域分区
- **THEN** 该状态 SHALL 可通过 URL 定位（刷新/分享后还原到同一分区）

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
