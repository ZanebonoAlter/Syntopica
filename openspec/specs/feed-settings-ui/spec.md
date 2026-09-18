# Feed Settings UI

## Purpose

TBD
## Requirements
### Requirement: Feed 卡片展示完整处理管线 toggle
每个 feed 卡片 SHALL 展示 4 个 toggle 开关，按管线执行顺序排列：Firecrawl 全文抓取、打标签、AI 摘要、内容补全。

#### Scenario: 展示所有 toggle
- **WHEN** 用户打开全局设置的订阅源配置 tab
- **THEN** 每个 feed 卡片底部显示 4 个 toggle：Firecrawl、打标签、AI 摘要、内容补全

#### Scenario: Toggle 状态与后端数据一致
- **WHEN** feed 的 firecrawl_enabled = true, tagging_enabled = false, article_summary_enabled = true, completion_on_refresh = true
- **THEN** 对应 toggle 分别为 开/关/开/开

### Requirement: Toggle 变更即时保存
用户切换 toggle SHALL 立即调用 feed update API 保存到后端，无需额外提交按钮。

#### Scenario: 切换 toggle
- **WHEN** 用户点击某 feed 的"打标签" toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 tagging_enabled，并刷新 feed 列表

### Requirement: 分类标题可折叠
订阅源配置 tab 中的分类标题 SHALL 支持点击折叠/展开。默认展开。折叠状态在当前页面生命周期内保持。

#### Scenario: 折叠分类
- **WHEN** 用户点击某分类标题
- **THEN** 该分类下的 feed 列表收起，标题显示折叠图标（▶）

#### Scenario: 展开分类
- **WHEN** 用户点击已折叠的分类标题
- **THEN** 该分类下的 feed 列表展开，标题显示展开图标（▼）

#### Scenario: 刷新页面后恢复默认
- **WHEN** 用户关闭并重新打开全局设置对话框
- **THEN** 所有分类恢复为展开状态

### Requirement: 最大文章数"无限制"语义正确
最大文章数选项 SHALL 使用 `0` 表示无限制。后端 `CleanupOldArticles` SHALL 将 `maxArticles <= 0` 视为不限制。超出上限的行为语义为归档降级（保留原文、清除衍生数据），而非物理删除，详见 `article-retention` capability。

#### Scenario: 选择无限制
- **WHEN** 用户选择"无限制"选项
- **THEN** 前端发送 `max_articles: 0` 到后端

#### Scenario: 后端不清理 max_articles=0 的 feed
- **WHEN** feed.MaxArticles = 0 且文章数超过任意值
- **THEN** CleanupOldArticles 不归档任何文章

#### Scenario: 兼容旧的 9999 值
- **WHEN** feed.MaxArticles = 9999
- **THEN** 前端显示"无限制"，后端行为与 max_articles=0 一致（不归档）

### Requirement: Feed 卡片展示 firecrawl 和补全 toggle
Firecrawl toggle SHALL 控制 `firecrawl_enabled` 字段，内容补全 toggle SHALL 控制 `completion_on_refresh` 字段。

#### Scenario: 切换 Firecrawl toggle
- **WHEN** 用户点击 Firecrawl toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 firecrawl_enabled

#### Scenario: 切换内容补全 toggle
- **WHEN** 用户点击内容补全 toggle
- **THEN** 前端调用 PATCH /api/feeds/:id 更新 completion_on_refresh

### Requirement: 订阅源管理区聚合全部 feed 管理入口
设置工作区「订阅源」section SHALL 提供 4 个管理入口：添加订阅源、添加分类、导入 OPML、导出 OPML，入口与订阅源主列表同处一屏可见位置。首页顶栏 MUST NOT 再出现这 4 个按钮。

#### Scenario: 设置页可见全部管理入口
- **WHEN** 用户打开 `/settings?section=feeds`
- **THEN** 订阅源管理区显示「添加订阅源」「添加分类」「导入」「导出」4 个入口

#### Scenario: 添加订阅源
- **WHEN** 用户点击「添加订阅源」入口
- **THEN** 打开 `AddFeedDialog` 对话框，行为与迁移前首页顶栏一致

#### Scenario: 添加分类
- **WHEN** 用户点击「添加分类」入口
- **THEN** 打开 `AddCategoryDialog` 对话框，行为与迁移前首页顶栏一致

#### Scenario: 导入 OPML
- **WHEN** 用户点击「导入」入口
- **THEN** 打开 `ImportOpmlDialog` 对话框，行为与迁移前首页顶栏一致

#### Scenario: 导出 OPML
- **WHEN** 用户点击「导出」入口
- **THEN** 浏览器下载 `feeds-export-<date>.opml` 文件，行为与迁移前首页顶栏一致

#### Scenario: 首页顶栏不再出现管理按钮
- **WHEN** 用户打开首页 `/`
- **THEN** 顶栏仅显示：暂停分析、AI 健康、刷新、全部已读、设置、新手引导、主题切换共 7 个按钮
- **AND** 不存在添加订阅、添加分类、导入、导出按钮

#### Scenario: 空状态引导入口保留
- **GIVEN** 用户没有任何订阅源和分类
- **WHEN** 用户打开首页 `/`
- **THEN** `FeedEmptyGuide` 空状态引导仍提供「添加订阅」按钮并可正常打开 `AddFeedDialog`

### Requirement: 最大文章数控件文案反映归档语义
Feed 设置中最大文章数控件的辅助说明 SHALL 表达"超出后归档保留原文"语义，MUST NOT 出现"删除"措辞。

#### Scenario: 文案展示归档语义
- **WHEN** 用户查看 feed 卡片的最大文章数控件
- **THEN** 辅助说明表述为超出上限的文章将被归档（原文保留、不再出现在列表中）

### Requirement: AI 摘要开关状态字段名契约
前端读取 feed AI 摘要开关时 SHALL 使用后端 `ToDict` 返回的 `article_summary_enabled` 字段（snake_case），不得使用 `ai_summary_enabled`（后端从不返回该键）。字段缺省（undefined/null）时前端 SHALL 按后端 gorm 默认值 `false` 处理，不得回退为 `true`。`aiSummaryEnabled`/`ai_summary_enabled` 相关死字段与死 key SHALL 从前端类型定义中移除。

#### Scenario: feed 开关状态真实反映后端值
- **WHEN** 后端返回 feed 的 `article_summary_enabled: false`
- **THEN** 前端该 feed 的 AI 摘要开关状态为关闭（旧行为因读错键恒为 true）

#### Scenario: 字段缺省时按后端默认处理
- **WHEN** 后端返回的 feed 数据中 `article_summary_enabled` 缺失或为 null
- **THEN** 前端该 feed 的 AI 摘要开关状态为关闭（false），与 gorm default:false 一致

#### Scenario: 死字段不再存在于类型定义
- **WHEN** 检查 `front/app/types/feed.ts` 与 `front/app/stores/api.ts` 的响应/更新接口
- **THEN** 不存在 `aiSummaryEnabled` / `ai_summary_enabled` 字段定义

### Requirement: 订阅源列表展示来源质量指标

设置工作区（`/settings?section=feeds`）订阅源列表的每一项 SHALL 展示该源在当前统计窗口内的入板块率与篇数：

- 列表项 meta 行 SHALL 显示「N 天内 X 篇 · 入板块 Y」；
- 行右端 SHALL 显示入板块率指标 pill，三档配色：≥60% 高、30%–60% 中、<30% 低；
- 窗口内文章数 < 5 时，pill SHALL 显示「样本少」而不展示百分比（避免小样本误判）；
- 窗口内文章数为 0 时，pill SHALL 显示「无新文」；
- `tagging_enabled = false` 的源 SHALL 显示灰底「打标关闭」，MUST NOT 显示 0% 命中率（该 0% 是配置结果而非内容质量）。

#### Scenario: 高命中源显示高指标
- **WHEN** 某源近 7 天 67 篇、命中 56 篇
- **THEN** 列表项 SHALL 显示「7 天内 67 篇 · 入板块 56」与高指标 pill「84%」

#### Scenario: 高流量低命中源显示低指标
- **WHEN** 某源近 7 天 1662 篇、命中 437 篇
- **THEN** 列表项 SHALL 显示「7 天内 1662 篇 · 入板块 437」与低指标 pill「26%」

#### Scenario: 样本不足不显示百分比
- **WHEN** 某源近 7 天仅 3 篇
- **THEN** pill SHALL 显示「样本少」，MUST NOT 显示任何百分比

#### Scenario: 打标关闭的源不显示 0%
- **WHEN** 某源 `tagging_enabled=false` 且窗口内有 1 篇无标签文章
- **THEN** pill SHALL 显示「打标关闭」

### Requirement: 订阅源列表窗口选择与排序筛选

订阅源列表 SHALL 提供窗口分段控件（7 天默认 / 30 天 / 90 天）、排序选择器与「只看低命中」筛选：

- 窗口切换 SHALL 同步到订阅源详情内的来源质量块（二者共享同一窗口状态，任一处切换两处一致）；
- 排序选项 SHALL 包含：默认（分类内标题）、分类内 · 入板块率升序、分类内 · 篇数降序、分类内 · 杂音量降序（杂音量 = 窗口内文章数 − 命中数）；排序 SHALL 在既有分类分组内生效，MUST NOT 打乱分类结构；
- 「只看低命中」筛选条件 SHALL 为：窗口内 ≥ 5 篇 **且** 入板块率 < 30%；`tagging_enabled=false` 的源 MUST NOT 参与该筛选；筛选后无匹配项时 SHALL 显示空结果提示与复位入口（空分类不显示）；
- 无样本源（窗口内 0 篇）在按率排序时 SHALL 排在样本充足项之后。

#### Scenario: 排序在分类内生效
- **WHEN** 用户选择「分类内 · 入板块率升序」
- **THEN** 每个分类内部的订阅源 SHALL 按入板块率升序排列，分类顺序与分组结构保持不变

#### Scenario: 窗口切换双向同步
- **WHEN** 用户在列表工具栏切到 30 天，随后选中一个源查看详情
- **THEN** 详情来源质量块 SHALL 同样处于 30 天窗口

#### Scenario: 筛出杂音源
- **WHEN** 用户开启「只看低命中」，某源窗口内 1662 篇、命中 437 篇（26%）
- **THEN** 该源 SHALL 保留在列表中；窗口内 67 篇、命中 56 篇（84%）的源 SHALL 被筛除

#### Scenario: 打标关闭源不被判为杂音
- **WHEN** 用户开启「只看低命中」，某源 `tagging_enabled=false`
- **THEN** 该源 SHALL NOT 出现在筛选结果中

#### Scenario: 筛选无结果可复位
- **WHEN** 开启「只看低命中」后没有任何源满足条件
- **THEN** 列表区 SHALL 显示空结果提示并提供复位（清除筛选）入口

### Requirement: 订阅源详情展示来源质量块

订阅源详情（右栏）SHALL 在状态条之后、设置表单之前展示只读「来源质量」块，内容 SHALL 包含：窗口分段控件（与列表共享状态）、四分解堆叠条与图例（入板块 / 有标签无板块 / 打标排队中 / 已处理无标签，含各自篇数）、入板块率数值（含分子分母）、命中板块分布 chips（Top 6 + 「其他 N 个板块」）。

- 窗口内 0 篇时 SHALL 显示「近 N 天没有新文章」；< 5 篇时 SHALL 显示「样本不足（N 篇）」；
- `tagging_enabled=false` 时 SHALL 明示该源已关闭打标、不参与板块归属，MUST NOT 显示 0% 命中率；
- 统计加载失败时 SHALL 在块内显示失败提示与「重试」入口，MUST NOT 影响下方设置项的可用性；
- 块内 MUST NOT 承载任何写操作（配置项仍在既有设置表单中）。

#### Scenario: 展示三分解与率
- **WHEN** 用户选中某源，近 7 天 1662 篇：命中 437、有标签无板块 739、打标排队中 486
- **THEN** 块 SHALL 显示对应堆叠条与图例篇数，并显示「入板块率 26.3%（437 / 1662）」

#### Scenario: 展示板块分布
- **WHEN** 该源命中分布在 8 个板块
- **THEN** 块 SHALL 显示 Top 6 板块 chips（含篇数）与「其他 2 个板块」

#### Scenario: 打标关闭不等于零命中
- **WHEN** 某源关闭了 AI 打标
- **THEN** 块 SHALL 说明「该源已关闭打标，不参与板块归属」，MUST NOT 显示 0% 命中率

#### Scenario: 统计失败不阻断设置
- **WHEN** 来源质量聚合请求失败
- **THEN** 块内 SHALL 显示失败提示与「重试」，且分类 / 自动刷新 / 最大文章数 / 管线 toggle 等设置项 SHALL 仍可正常查看与修改
