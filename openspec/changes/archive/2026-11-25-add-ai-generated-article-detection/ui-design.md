<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: ui-prototype/index.html -->
<!-- ui-stage: data -->

<!--
  marker 契约（ui-design-gate 机器读取，缺失/非法/重复会被阻断）：
  - ui-impact 必须与 proposal 声明一致
  - minor → approval=not-required、prototype 可为 none，也允许给出可点原型引用
    （保留四节轻量契约：入口与入口变更 / 受影响状态 / 复用组件与布局模式 / 验收映射）
  - ui-stage 是原型生产阶段标记（l0|skeleton|interaction|data|approved），缺失视为 legacy，不阻断
-->

## 入口与入口变更

本变更**不新增任何页面、面板、弹窗、抽屉或导航入口**，只在四类既有视图内追加一个「疑似 AI 生成」标记与两个既有菜单动作：

| 视图 | 既有入口 | 变更 |
| --- | --- | --- |
| 版块态势页「归档」视图 | `/tags/:boardId` → 归档 tab（`BoardArchiveView.vue`） | 文章行标签带末尾追加「疑似 AI 生成」chip |
| 阅读页文章列表 | `/` 左栏（`ArticleCardView.vue`） | 封面缩略图右上角追加「AI」紧凑角标（完整文案走 `title` / `aria-label`） |
| 文章详情 | `/` 右栏预览面板（`ArticleContentPreviewPanel.vue`） | 标签区追加「疑似 AI 生成」静态 chip |
| 日报线索文章 | `/tags/:boardId/[date]` → 线索文章展开（`DailyReportTopicSection.vue`） | 线索 chip 追加「疑似 AI 生成」标记，疑似文章排至展开列表末尾 |
| 订阅源设置 | 设置 → 订阅源管理 → 订阅源详情（`FeedDetailEditor.vue`） | 处理管线 toggle 组新增「AI 生成检测」开关 |
| 文章详情 ⋯ 菜单 | `/` 右栏 ⋯（`ArticleStatusMenu.vue`） | 「手动操作」组新增「标记为疑似 AI 生成」/「取消疑似 AI 标记」两项 |

完整文案统一为「疑似 AI 生成」（详情胶囊 / 归档 chip / 日报 chip / 菜单动作）。列表行角标受 165px 窄 meta 行限制，采用两字符短标「AI」，完整文案经 `title` 与 `aria-label` 提供（2026-11-25 修订，见文末「修订记录」）。

## 受影响状态

标记随既有列表/详情/日报响应一并返回，**不产生额外请求**，因此四态影响如下：

- **loading**：无新增状态。既有列表/详情/日报的骨架态不变，标记不单独占位。
- **empty**：无新增状态。空列表、空日报、空归档区按既有空态渲染。
- **success**：
  - 判定状态为 `suspect` → 显示「疑似 AI 生成」标记（列表封面角标 / 详情 chip / 归档 chip / 日报 chip）。
  - 判定状态为 `unknown`（未判定 / 未开启该 feed 的检测）→ **不显示任何标记，也不显示占位或「未检测」文案**，界面与变更前完全一致。
  - 判定状态为 `not_ai`（人工确认非 AI）→ 不显示标记。
  - 日报线索展开列表中，`suspect` 文章稳定排至末尾（集合与条数不变，仅顺序）。
- **error**：
  - 判定作业失败（`ai_detection` capability 未绑定模型、provider 全部降级失败）→ 文章判定状态保持 `unknown`，界面不显示标记；失败原因进 `ai_call_logs` 审计并可经设置页 AI 健康面板观测，**不在文章界面上以错误横幅打扰用户**。
  - 人工标记/取消标记请求失败 → 沿用既有 `ArticleStatusMenu` 的 `asm-menu-error` 内联错误位，不新增错误呈现形态。

## 复用组件与布局模式

**不新增基础组件，不引入自由宽度。** 四处展示位均落在既有宿主的既有交互槽位内：

| 展示位 | 复用形态 | 来源 |
| --- | --- | --- |
| 列表行角标 | 封面缩略图右上角的覆盖式小角标（不占行内任何 flex 宽度；中性淡底 + 描边，与详情胶囊同源 token） | `front/app/components/article/ArticleCard.css` |
| 详情 chip | 静态胶囊（`.article-tag` 同款形态；**不复用** `ArticleTagList`，其绑定 topic-tag 且带 watch 按钮） | `front/app/features/articles/components/ArticleTagList.vue` |
| 归档行 chip | 复用归档行既有标签带 chip 形态（规则色边框体系不变，疑似标记用中性色，不占用规则色语义） | `front/app/features/tags/components/console/boardArchiveView.css` |
| 日报 chip | 复用线索文章 chip 形态 | `front/app/features/tags/components/daily-report/DailyReportTopicSection.vue` |
| feed 开关 | `AppToggle` | `front/app/components/ui/AppToggle.vue` |
| 菜单动作 | 复用既有 `.asm-menu-item` 菜单项形态 | `front/app/features/articles/components/ArticleStatusMenu.vue` |

**布局模式沿用宿主，不做变更**：

| 视图 | layout mode | 说明 |
| --- | --- | --- |
| 阅读页 | `split`（左列表 + 右详情） | 不动栏宽、不动行高；列表仍为固定行高虚拟列表，徽标在行内原地渲染，不引入折叠或行高变化 |
| 版块态势页 / 归档视图 | `workspace` | 不动栏宽与溢出策略 |
| 日报 | `workspace` | 不动栏宽与溢出策略 |
| 订阅源设置 | `contained` | 不动区块结构 |

不新增 `AppDialog`，因此不涉及 `sm/md/lg/xl` 尺寸档选择。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 文章详情 ⋯ 菜单中的「标记为疑似 AI 生成」/「取消疑似 AI 标记」是就地改单个字段，不弹二次确认 | inline-edit | `dialog`（不得为标记动作新增确认弹窗） | pinned |
| D2 | 订阅源详情「AI 生成检测」开关就地切换并即时保存，不弹确认 | inline-edit | `dialog` | pinned |
| D3 | 疑似 AI 标记只做展示与排序降权，任何界面都不得提供「删除该文章」或「移出版块」的联动动作 | inline-edit | `dialog`（不得出现删除类确认弹窗） | pinned |

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 列表行「疑似 AI 生成」徽标 | api GET /api/articles | 列表窄投影新增非正文类判定字段，不违反正文大字段禁令 |
| 详情标签区「疑似 AI 生成」chip | api GET /api/articles/:article_id | 详情 DTO 新增同一判定字段 |
| 归档视图文章行 chip | api GET /api/semantic-boards/:id/articles | 版块文章列表响应项新增同一判定字段 |
| 日报线索文章 chip 标记 | api GET /api/daily-reports/:id | 线索线程关联文章带出判定字段 |
| 日报线索疑似文章排后 | 派生 <疑似文章稳定排至展开列表末尾> | 由服务端排序后的 `related_article_ids` 顺序承载，不新增字段 |
| 订阅源「AI 生成检测」开关状态 | api PUT /api/feeds/:id | feed 实体新增 `ai_detection_enabled`，走既有 feed 更新接口与前端字段映射 |
| 人工标记 / 取消标记结果 | api PUT /api/articles/:article_id | 写入判定状态与来源（llm / manual） |
| 判定阈值 | 表 ai_settings | 键 `ai_detection_threshold`，数值型在线可覆盖 |

## 明确不做

- 阅读页文章列表中不出现「隐藏疑似 AI 文章」开关，也不出现按疑似状态过滤列表的入口。
- 归档视图不出现「疑似 AI」筛选维度（归档视图既有三个筛选：全部 / 未纳入日报 / 弱证据，保持不变）。
- 日报线索文章不出现「移除该线索文章」操作；日报线索的「N 篇」计数不因疑似标记而变化。
- 文章详情不出现「判定依据全文」「查看模型判定理由」展开面板。
- 版块归档视图的标签带不因疑似标记新增规则色边框；疑似标记不参与「入板 chip」步号体系与强弱表达。

## Prototype

原型形态为**可点 HTML**（本变更主要交互模式是 `inline-edit`，非 `canvas`，故不产状态图 + `interaction-table.md`）：

- 入口：`ui-prototype/index.html`（直接打开即可；用 http 打开时数据源为同目录 `fixture.json`，file:// 直开时回落到页面内同内容镜像）
- 数据：`ui-prototype/fixture.json` —— 文章 / 归档行 / 日报线索 / 订阅源均为本地后端真实快照（2026-11-20），唯每篇文章的判定字段是本次变更待上线的演示态
- 说明：`ui-prototype/README.md`（本目录唯一说明文档，≤300 行）

| 场景 | 产品界面位置 | 原型内可验证的行为 |
| --- | --- | --- |
| ① 阅读页列表行 | `/` 左栏（`ArticleCardView.vue`） | 角标压在封面右上角（短标「AI」）；未判定不显示也不占位；标题两行 clamp、meta 行时间/来源名/状态图标宽度均不受角标影响 |
| ② 文章详情 + ⋯ 菜单 | `/` 右栏（`ArticleContentPreviewPanel.vue` + `ArticleStatusMenu.vue`） | 点「标记为疑似 AI 生成」→ 标签区出胶囊、菜单项变「取消疑似 AI 标记」；「模拟请求失败」→ 菜单内联错误位（`asm-menu-error`） |
| ③ 版块归档行 | `/tags/:boardId` 归档 tab（`BoardArchiveView.vue`） | 中性色紧凑标记追加在标签带末尾（入板 chip 之后），不进规则色、不进「入板」步号 |
| ④ 日报线索文章 | `/tags/:boardId/[date]`（`DailyReportTopicSection.vue`） | 疑似文章稳定排到展开列表末尾；条数与集合不变，只换顺序 |
| ⑤ 订阅源处理管线开关 | 设置 → 订阅源详情（`FeedDetailEditor.vue`） | 「AI 生成检测」就地切换、即时保存、默认关闭，与「AI 打标签」互不联动 |

**标记视觉规范（三种形态，全部中性色，实现按此取值）**：

| 形态 | 落点 | 几何 | 建议类名 |
| --- | --- | --- | --- |
| 角标 | 阅读页列表行（封面右上角） | 短标「AI」：10px / 700 / 字距 .5px / padding 0 4px / radius 4px / 1px 描边 + `--color-bg-elevated` 淡底 + `0 1px 2px rgba(0,0,0,.16)` 细投影（覆盖在缩略图上必须有实底与分层，否则图色穿透/兑色不可读） | `.suspect-badge` |
| 胶囊 | 文章详情标签区 | 同 `.article-tag`：min-height 1.9rem / radius 999px / padding .25rem .72rem / 0.76rem 700 / 1px 描边 + 弱底 | `.suspect-chip` |
| 紧凑标记 | 版块归档行、日报线索 | 同 `.ar-tag`：0.7rem / 1.5 / padding .04rem .4rem / radius 4px / 1px 描边 / 无底色 | `.suspect-tag` |

取值：描边 `var(--color-border-medium)`（角标 / 紧凑标记）/ `var(--color-border-subtle)`（详情胶囊），字色 `var(--color-text-secondary)`，角标底色 `var(--color-bg-elevated)`（同详情胶囊，双主题自适应），**不用** `--color-text-muted`（既有 `.archived-badge` 的 muted 在 10.5px 下仅 3.3:1，达不到 4.5:1），**不用**警示色与规则色（结论是「疑似」不是「出错」；`--color-match-*` 专供版块匹配规则），**不加图标**（`.archived-badge` / `.article-tag` 两族本身无图标）。

**实现落点要点**：

- 列表角标插在 `.row-cover` 内、与封面图/`FeedIcon` 同级，绝对定位到右上角（`.row-cover` 加 `position: relative`）；**不进** `.row-title` 的 clamp 盒、**不进** `.row-meta` 的 flex 行——前者会被截断，后者实测 165px 内容宽装不下时间 49px + 状态图标 19px + 77px 全称角标，角标被 `overflow:hidden` 裁 4px 且把来源名压到 0 宽（2026-11-25 修订）。行高固定 100px 不变。
- 详情胶囊插在 `.tag-row` 内、`<ArticleTagList>` 之后、手动打标按钮之前；`.article-tag` 是 `ArticleTagList.vue` 的 scoped 样式、不外溢，而 `ArticleContentPreviewPanel.vue` 当前无 `<style>` 块，胶囊样式须在面板内自行声明。
- 归档紧凑标记追加在 `.ar-row-tags` 内、`.ar-flow-chip` 之后。
- 日报标记**嵌在 `.drm-article` 的标题 `<span>` 内部**：该元素是 `grid-template-columns: 3px auto 1fr`，加第 4 个子元素会产生隐式列并挤窄标题列。
- 菜单两项加在 `.asm-menu-pop` 的「手动操作」组内、既有三项之后、两处 `.asm-menu-error` 之前；不新增分组标题（组名沿用「手动操作」）。
- 订阅源新行放在「AI 打标签」之后，图标 `mdi:robot-outline`（已在本地 iconify 子集内，无需重新生成子集）。

## 验收映射

实现阶段按下列既有测试文件补断言（均为仓库中已存在的真实测试文件）：

| 验收点 | 测试文件 |
| --- | --- |
| 列表行封面角标渲染、「未判定不显示」、不吃 meta 行宽度 | `front/app/features/articles/components/ArticleCardView.test.ts` |
| 详情标签区 chip | `front/app/features/articles/components/ArticleContentPreviewPanel.test.ts` |
| ⋯ 菜单「标记/取消标记」动作与失败内联错误 | `front/app/features/articles/components/ArticleStatusMenu.test.ts` |
| 归档视图文章行 chip | `front/app/features/tags/components/console/BoardArchiveView.test.ts` |
| 日报线索 chip 标记与疑似文章排后 | `front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts` |
| 订阅源「AI 生成检测」toggle 即时保存 | `front/app/features/settings/components/FeedDetailEditor.test.ts` |

四态中 error 态依赖后端判定作业失败场景，前端组件测试覆盖「失败后标记不显示且无错误横幅」；判定作业本身的失败路径由后端包级测试覆盖（不在本节 UI 验收范围）。

## 修订记录

| 日期 | 修订 | 依据 |
| --- | --- | --- |
| 2026-11-21 | 初版：列表角标挂标题尾（同 `.archived-badge`） | review R-5 前的原始形态 |
| 2026-11-22 | 修订 A：角标移出 `.row-title` 的 clamp 盒 → `.row-heading` flex 行（R-5） | 长标题两行 clamp 会整块裁掉角标 |
| 2026-11-22 | 修订 B：角标从 `.row-heading` 移入 `.row-meta` 行尾（R-13） | 角标吃掉标题列 52% 可用宽 |
| 2026-11-25 | 修订 C：角标改为**封面右上角短标「AI」+ 淡底**，meta 行不再承载角标 | 浏览器实测（430 / 1600 双视口）：`.row-meta` 内容宽恒为 165px（面板 `20rem` 固定），时间 49.4 + 状态图标 19 + 全称角标 77 + gap 24 = 169.4px → 角标右侧被 `overflow:hidden` 裁 4.4px，同时来源名被压到 0 宽；且 10.5px 无底描边在 meta 行尾与灰色状态图标同排，用户反馈「不明显」。改为覆盖式短标后 meta 行宽度零占用，角标完整可见并获实底对比 |
