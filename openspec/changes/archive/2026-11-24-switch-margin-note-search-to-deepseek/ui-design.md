<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约 — 页边注联网切换 DeepSeek Search（minor）

ui-impact=minor 依据：不新建页面/弹窗/导航；仅①在既有「设置 → 数据源与网络」分区的 tab 列表内把 SearXNG tab 替换为「网页搜索」tab（同一 tab 容器、同一表单布局），②在既有 `MarginNoteCard` 问答轮元信息行增加一个状态标记。无新交互模式、无信息架构变更，故不需可视原型。

## 1. 入口与入口变更

| 入口 | 现状 | 变更后 |
| --- | --- | --- |
| 设置 → 数据源与网络 → tab「SearXNG」 | 打开 `SearxngConfigPanel`（endpoint + enabled 开关 + 测试连接） | **移除**该 tab；同一位置新增 tab「网页搜索」，打开 `WebSearchConfigPanel` |
| 日报阅读台 → 页边注卡 → 问答轮元信息行 | 仅 `web_search_status=empty` 渲染「联网搜索无结果」 | 三态可见：`empty`→「联网搜索无结果」（沿用文案与样式）；`ok` 且本轮有 `web_sources`→「已联网」；`ok` 且本轮无 `web_sources`→「已联网 · 未采用」；`disabled`/NULL 不渲染（不变） |
| 旧深链 `?section=datasources-network&tab=searxng`（及更旧的 `?section=searxng`） | 定位 SearXNG tab | 重定向到 `tab=web-search`（`SettingsWorkspace.vue` 两级重定向：旧 section 键与旧 tab 键都覆盖；分区容器留同值兜底映射） |

## 2. 受影响状态

### 2.1 `WebSearchConfigPanel`（替换 `SearxngConfigPanel`；**无表单、无可编辑字段**——密钥由「AI 模型」分区（`section=ai-providers`）的供应商配置统一管理，见 design D6）

| 状态 | 表现 |
| --- | --- |
| loading | 读取 `GET /api/settings/web-search` 期间展示文字「正在读取关联状态…」，「测试连接」禁用；点击「测试连接」后按钮 loading 态防重复提交 |
| empty | `vendor_configured=false`：面板显示中性提示「网页搜索使用已配置的 DeepSeek 供应商密钥，请先在「AI 模型」分区添加 DeepSeek 供应商」及前往指引；「测试连接」仍可点（探测会返回 `disabled` 中性态） |
| success | `vendor_configured=true`：显示「已关联供应商：<vendor_name>」；「测试连接」返回 `{status, latency_ms, detail, error_kind}` 按四态渲染徽标（`ok` 绿 / `degraded` 黄 / `unreachable` 红 / `disabled` 灰），四态色点样式从旧 SearXNG 面板原样搬运（`degraded` 用主题变量 `--color-warning`）；`vendor_name` 为空时显示「（未命名）」 |
| error | `GET settings` 失败：行内错误 + 重试入口；探测 `unreachable` 时按 `error_kind` 展示针对性主文案（见 §5），后端 `detail` 作次级说明行；`error_kind` 为 `auth` / `quota` 时另给一行去向提示「请到「AI 模型」分区的 DeepSeek 供应商检查密钥 / 额度」（纯文字，不新造跨分区跳转控件） |

### 2.2 `MarginNoteCard` 问答轮标记

| 状态 | 表现 |
| --- | --- |
| `web_search_status=empty` | 「联网搜索无结果」弱化标记（**不变**，线上既有 `data-testid="mn-web-search-empty"` 保持） |
| `web_search_status=ok` 且 `cited_web_sources.length>0` | 「已联网」弱化标记，`data-testid="mn-web-ok"` |
| `web_search_status=ok` 且 `cited_web_sources.length===0` | 「已联网 · 未采用」弱化标记，`data-testid="mn-web-unused"`，title 提示「已搜索到结果，但模型本轮未引用」 |
| `disabled` / NULL | 不渲染（**不变**） |

标记为纯展示：不改变回答内容、不改变 `web_sources` 校验、不改变静默降级。

## 3. 复用组件与布局模式

- 设置面板：复用 `SettingsSectionDatasourcesNetwork.vue` 的 tab 容器（`AppSectionHeader` + tab list），不新增容器；`WebSearchConfigPanel` 只用 `AppButton` 与既有面板容器样式，不含输入框 / 开关 / 表单（D6），dialog 模式不涉及（面板内嵌于分区，非弹窗）。
- 页边注卡：复用 `MarginNoteCard.vue`「联网搜索无结果」既有弱化标记规则（原 `.mn-qa__search-empty` 原位更名为 `.mn-qa__web-status` 以承载三态，规则体不变：`inline-block` / 0.62rem / `--text-muted`），不新增 CSS 类族；与「纯模型知识」（`.mn-qa__pure`，`<p>`）保持既有两行结构。
- 页面壳：`AppPageShell` workspace 模式（设置）与 reader 模式（日报）均不变。

## 4. 验收映射

| 验收点 | 测试文件 | 断言 |
| --- | --- | --- |
| 设置分区 tab 列表含「网页搜索」且不含「SearXNG」 | `front/app/features/settings/components/SettingsSectionDatasourcesNetwork.test.ts` | tabs 渲染断言更新 |
| 旧深链 `tab=searxng` 重定向 | `front/app/features/settings/components/SettingsWorkspace.test.ts` | 现有 legacy 重定向用例追加一条 |
| 新面板四态徽标 + `vendor_configured=false` 空态 | 新增 `front/app/components/dialog/WebSearchConfigPanel.test.ts` | 四态各一例 + 空态一例 + `error_kind` 文案一例 + 测试连接 loading 防重复 |
| 页边注卡三态标记 | `front/app/features/tags/components/daily-report/MarginNoteCard.test.ts` | `ok+有引用`→`mn-web-ok`；`ok+无引用`→`mn-web-unused`；`empty`→`mn-web-search-empty`；`disabled`/NULL→无标记 |
| API 层 | 新增 `front/app/api/webSearch.test.ts`（替换 `searxng` 相关断言） | GET settings / GET health 两接口形状（无 POST）与响应归一化 |
| D6 守卫：面板不含 `input` / 开关组件 / `form` | `front/app/components/dialog/WebSearchConfigPanel.test.ts` | `find('input')` / `findComponent(AppToggle)` / `find('form')` 均不存在 |
| 指引分区名与去向提示 | 同上 | 空态文案含「AI 模型」；`error_kind=auth` / `quota` 时含去向提示；`vendor_name` 为空显示「（未命名）」 |
| 旧 tab 键重定向映射单一来源 | `SettingsWorkspace.test.ts`、`SettingsSectionDatasourcesNetwork.test.ts` | 两处经 `front/app/features/settings/legacyRedirects.ts` 取同一映射；旧深链用例不变 |

浏览器验证不强制（minor）；实现后在 §6 补一行人工走查记录即可。

## 5. 界面文字

- 面板标题：「网页搜索」；说明句：「页边注问答的联网参考由 DeepSeek Search API 提供，复用「AI 模型」分区中已配置的 DeepSeek 供应商密钥。」
- 探测失败主文案（前端按 `error_kind` 映射固定短语，后端 `detail` 作次级说明行）：`鉴权失败（检查 DeepSeek 供应商 API Key）` / `余额或额度不足` / `请求超时` / `请求过于频繁，稍后再试` / `搜索服务暂不可用`；`degraded`→「服务可达但搜索无结果」；`disabled`→「未关联供应商，联网参考已关闭」。
- 页边注卡标记文案见 §2.2，不加句号、不加图标（对齐现有「联网搜索无结果」写法）。

## 6. 人工走查记录

**2026-11-24 走查（浏览器实走 + 组件测试，详见 `ui-verification/report.md`）**

| 项 | 路径 / 证据 | 结论 |
| --- | --- | --- |
| tab 更替 | `?section=datasources-network` → tabs「Firecrawl / 博查 / 网页搜索 / RSSHub」，无 SearXNG | pass |
| 旧深链 `tab=searxng` | 地址变为 `tab=web-search`，网页搜索 tab selected | pass |
| 更老旧键 `?section=searxng` | 地址变为 `?section=datasources-network&tab=web-search` | pass |
| 负向：`?section=ai-providers&tab=searxng` | 地址不变（不触发重定向） | pass |
| 测试连接 | 「连接正常 · 394ms」+「搜索正常，返回 10 条结果」 | pass |
| 无可编辑字段 | 快照无输入控件；组件测试守卫断言 | pass |
| 卡片三态标记 | 组件测试 13 例覆盖；浏览器实走 `/tags/2272/2026-11-06` 时为「已联网」+「网络来源」（`margin-note-web-status.png`） | pass |

## 7. Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | SearXNG 配置 tab 退役，原位换成「网页搜索」tab（只读关联状态 + 探测） | `panel` | `dialog`, `drawer`, `page` | `pinned`（变更来源：用户 2026-11-23「直接切 DeepSeek Search API，避免 SearXNG 部署难度」） |
| D2 | 「测试连接」为原地单步动作，结果以行内四态色点 + 耗时反馈 | `panel` | `dialog`, `drawer` | `pinned`（沿用旧面板既有的「点击 → 行内四态」形态） |
| D3 | 密钥修改不在本面板提供：只给文字指引到「AI 模型」分区（`section=ai-providers`），不新造跨分区跳转控件 | `page`（经既有侧栏导航到达） | `command`（词表标注「暂无实例；引入时须先登记」）, `dialog`, `inline-edit` | `pinned`（design D6：凭据单一来源，面板无可编辑字段） |

D2 / D3 的 pinned 依据：2026-11-23 实现前向用户展示的「关键决策」表（面板不放 API Key 输入框、只显示关联状态 + 测试连接），用户指示按此派发实现。页边注卡三态标记为纯展示、无用户操作，不进本表（红线 27：标记仅 UI 层）。

## 8. Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 「已关联供应商：<名称>」/「未关联」 | `api GET /api/settings/web-search`（`vendor_configured`、`vendor_name`） | 路由注册于 `backend-go/internal/discovery/routes.go`；名称来自 `ai_vendors.name` |
| 探测四态色点 | `api GET /api/settings/web-search/health`（`status`、`error_kind`） | 按需触发，不轮询 |
| 探测耗时「N ms」 | `api GET /api/settings/web-search/health`（`latency_ms`） | 同上 |
| 探测失败原因文案 | `派生 前端按 error_kind 映射固定短语` | 次级说明行取同一接口的 `detail` |
| 去向提示「请到「AI 模型」分区…」 | `派生 error_kind ∈ {auth, quota}` | 纯前端文案，无后端字段 |
| 页边注卡「已联网 / 已联网 · 未采用 / 联网搜索无结果」 | `api POST /annotations/:id/questions` 与批注列表响应的 `web_search_status`（表 `annotation_qas.web_search_status`） | 「已联网」与「未采用」的区分取同一响应的 `cited_web_sources` 是否为空 |
