<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI Design Contract（minor：复用/状态契约）

## 入口与入口变更

入口不变：应用首页 → 左侧「全部文章」/ 分类 → 文章列表 → 点选文章 → 阅读页右侧文章主体（`ArticleContentPreviewPanel.vue`，阅读列 `.reading-col`）。本 change **不新增页面、面板、弹窗或导航**，不改变任何 layout mode。

唯一入口级变化是**阅读页同一位置的内容呈现形态**：

- 导语段（`.lede`，标题与标签行之下）：由「`v-html` 直出第三方 HTML」改为「去标签 → 纯文本 → ≤300 字符 → `v-text` 渲染」。视觉位置、字号、配色、浅色无边框形态全部沿用既有 `.lede` 契约（`docs/reference/standard/frontend/layout.md`），只换内容来源与渲染方式。
- 内容源切换（工具栏 ⋯ 菜单 →「原文」源）：`articles.content` 由「原样 `v-html` 直出」改为「保守清洗后渲染」（去 `<script>`/`<style>`、事件属性、隐藏节点与隐藏样式声明，保留其余结构与排版）。切换入口、菜单位置、默认源（firecrawl 优先）均不变。

数据链变化：详情接口 `GET /api/articles/:id` 的 `description` 字段语义由「源站原文（可能是整篇 HTML）」变为「≤300 字符纯文本摘要」；字段名、路径、其余响应字段集不变（`content` 不变，仍是全文 HTML）。

## 受影响状态

| 状态 | 受影响 | 说明 |
| --- | --- | --- |
| loading | 否 | 导语与正文的加载骨架不动 |
| empty | **是** | 旧形态：description 为整篇 HTML 时，导语段渲染出 8046 px 的不可见空白（用户感知为"页面崩了/空白"）。新形态：description 为纯文本摘要 → 导语段正常显示 ≤300 字摘要；若摘要与正文重复，`shouldShowArticleDescription` 既有 guard 判定为不重复性命中 → 不渲染导语段、不留占位块（沿用既有 empty 契约） |
| error | 否 | 详情接口失败、Firecrawl 失败等错误态与既有处理一致 |
| success | **是** | 导语段显示纯文本摘要（不再是一整篇被隐藏的文章）；「原文」源显示清洗后的文章 HTML（隐藏节点不再占位、正文可见） |

`excerpt`（列表窄投影的 ≤200 字符导语）字段契约与判定不变；首帧「详情未就绪 → 回退 excerpt」的既有回退链不变。

## 复用组件与布局模式

- **零新组件、零新布局**：复用 `ArticleContentPreviewPanel.vue`（组合）、`useArticleContentView.ts`（取值与派生）、`articleContentGuards.ts`（导语去重 guard）、`imageProxy.ts`（外链图片代理改写）。
- 布局模式不变：阅读页维持 reader 阅读列版式（`.reading-col` max-width 840px / 面板窄于 840 取可用宽），本 change 不触碰 `ArticleContent.css` 的 `.markdown-body` 共享宿主排版与 `.preview-mode` 覆写作用域红线。
- 新增纯函数模块（无组件）：`front/app/utils/htmlSanitize.ts`（第三方 HTML 保守清洗 + 纯文本抽取），由导语段与原文源共用，避免两套清洗口径。
- 后端新增纯函数包（无组件）：`backend-go/internal/platform/htmltext/`（清洗口径唯一实现），与既有 `reader/handler/article_excerpt.go` 的 excerpt 清洗合并。

## 验收映射

- **前端工具层**：`front/app/utils/htmlSanitize.test.ts`（新增）——隐藏节点（`display:none`/`visibility:hidden`/`opacity:0`/`font-size:0`）被剔除且其文本不出现；`<script>`/`<style>` 内容不出现；事件属性（`on*`）被移除；实体还原；空白折叠；截断 ≤300 且带省略号；幂等（二次清洗结果不变）。
- **前端组件层**：`front/app/features/articles/components/ArticleContentView.test.ts`（既有）——导语段渲染为文本节点而非 HTML 节点；`description` 含 `<img onerror=...>` 时不产生真实 DOM 元素与事件处理器。
- **前端合成层**：`front/app/features/articles/composables/useArticleContentView` 相关既有测试保持绿（内容源切换取值链不变）。
- **后端包层**：`backend-go/internal/platform/htmltext/htmltext_test.go`（新增）——智源真实样本（`<div id="js_content" style="opacity: 0">…`）清洗后不含标签、不含 `opacity`、长度 ≤300、幂等；纯文本输入原样通过；清洗口径与 excerpt 清洗器一致（同一输入两处输出一致，除长度上限）。
- **真实页面核对**（静态 :5100，agent-browser，建账 `ui-verification/report.md`）：打开一篇智源文章（feed 1），导语段显示纯文本摘要而非空白，正文段首屏可见（不再被 8046 px 空白顶下去）；切到「原文」源后正文可见、无隐藏占位。
