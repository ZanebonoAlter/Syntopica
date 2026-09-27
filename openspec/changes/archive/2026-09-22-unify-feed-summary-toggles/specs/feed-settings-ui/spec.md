## MODIFIED Requirements

### Requirement: Feed 卡片展示 firecrawl 和补全 toggle

Feed 详情编辑器 SHALL 展示统一词汇表的处理管线开关集合：「AI 总结」（`article_summary_enabled`，说明文案"为文章生成 AI 整理稿；开启后文章页可手动生成"）、「刷新后自动总结」（`completion_on_refresh`，说明文案"新文章自动排队总结；关闭后仅手动生成"）、「全文抓取」（`firecrawl_enabled`，说明文案"用 Firecrawl 抓取完整正文供阅读，与总结独立"）、「AI 打标签」（`tagging_enabled`，说明文案"自动为文章生成主题标签"）。MUST NOT 再出现"内容补全"等旧名。各 toggle 切换 SHALL 即时调用 PATCH /api/feeds/:id 更新对应单字段。Feed 详情编辑器 SHALL 同时提供最大重试次数（`max_completion_retries`）与 RSS 地址编辑。

#### Scenario: 切换 Firecrawl toggle
- **WHEN** 用户点击「全文抓取」（Firecrawl）toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 firecrawl_enabled

#### Scenario: 切换内容补全 toggle
- **WHEN** 用户点击「刷新后自动总结」toggle（原"内容补全"改名）
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 completion_on_refresh

#### Scenario: 切换 AI 总结主开关
- **WHEN** 用户点击「AI 总结」toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 article_summary_enabled

#### Scenario: 编辑 RSS 地址
- **WHEN** 用户在 feed 详情编辑器修改 RSS 地址并保存
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 url，保存成功后列表与详情反映新地址

## ADDED Requirements

### Requirement: 主界面编辑入口统一跳转设置工作区

主界面（阅读页）的"编辑订阅源"操作 SHALL 跳转到设置工作区订阅源 section 并自动定位到目标 feed 详情（深链 `/settings?feed=<id>&section=feeds`），MUST NOT 再弹出独立编辑对话框。`EditFeedDialog` 组件 SHALL 移除；其删除订阅源能力由设置工作区 feed 详情的既有删除入口承接，功能不回退。

#### Scenario: 主界面编辑跳转定位
- **GIVEN** 用户在主界面某 feed 的菜单中点击"编辑订阅源"
- **WHEN** 跳转发生
- **THEN** 打开设置页订阅源 section，且目标 feed 在列表中被选中、详情编辑器展开

#### Scenario: 深链目标不存在时优雅降级
- **WHEN** 深链携带的 feed id 在列表中不存在（如已删除）
- **THEN** 落到订阅源管理默认视图（无 feed 选中），不报错

#### Scenario: 删除订阅源能力保留
- **WHEN** 用户在设置工作区 feed 详情点击删除订阅源并确认
- **THEN** feed 及其文章按既有级联规则删除（与原主界面弹窗行为一致）

### Requirement: 订阅源列表展示总结开启状态

设置工作区订阅源列表项 SHALL 对 `article_summary_enabled = true` 的 feed 展示可辨识的"AI 总结"标识，使用户无需逐个打开详情即可看出哪些 feed 开了总结；未开启的 feed MUST NOT 展示该标识。

#### Scenario: 开总结的 feed 列表可见标识
- **GIVEN** 某 feed `article_summary_enabled = true`
- **WHEN** 用户查看设置工作区订阅源列表
- **THEN** 该 feed 列表项展示"AI 总结"标识

#### Scenario: 未开的 feed 不展示
- **GIVEN** 某 feed `article_summary_enabled = false`
- **WHEN** 用户查看设置工作区订阅源列表
- **THEN** 该 feed 列表项无"AI 总结"标识
