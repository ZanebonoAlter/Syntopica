## MODIFIED Requirements

### Requirement: 独立设置工作区承载全局设置

全局设置 SHALL 由独立工作区（`SettingsWorkspace`）承载，导航按四个分组组织（内容管理 / AI 配置 / 数据源与网络 / 运行状态，共 7 个导航项），SHALL NOT 回退为单一超长全局对话框，SHALL NOT 恢复无分组的平铺导航。参考角色与分析方法卡相关的设置入口 SHALL NOT 存在。

#### Scenario: 设置入口进入工作区

- **WHEN** 用户从入口打开设置
- **THEN** 进入独立设置工作区，按四个分组导航展示 7 个设置项（非全局弹窗、非平铺列表）

#### Scenario: URL 定位设置分区

- **WHEN** 设置工作区处于某领域分区
- **THEN** 该状态 SHALL 可通过 URL 定位（刷新/分享后还原到同一分区；复合 section 的子 tab 由 `tab` 参数承载）

## ADDED Requirements

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
