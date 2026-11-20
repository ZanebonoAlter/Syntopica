<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: content-enrichment, reading -->

## Why

智源社区等源（RSSHub `baai` 路由）的 RSS item 把**整篇文章的 HTML** 放进 `description`，入库后原样存进 `articles.description`（该源 2148 篇里 1958 篇是 >2KB 的 HTML，均值 66 KB、最大 864 KB；全库 13602 行 description 含 HTML，字段合计 276 MB）。阅读页导语段用 `v-html` 直出这段第三方 HTML，其中微信公众号文章的根节点带 `style="visibility: visible; opacity: 0;"` —— 内容整体不可见却照样占位，实测单篇导语段高 **8046 px**、正文从第 **8787 px** 才开始，用户看到的就是标题下方一大片空白；同一篇文章的可见文本还在导语段与正文段各渲染了一遍（实测两处 innerText 均为 4819 字、完全相同）。

**事实澄清（非 SEO 关键词堆砌）**：全库 `description`/`content`/`firecrawl_content`/`ai_content_summary` 剥掉标签与图片链接后搜索 `aaaa` **0 命中**，页面可见文本也不含 `aaaa`。用户看到的"人类不可读的 aaaa"是智源配图的 CDN 文件名哈希（`.../hub-detail/3a8fed6844ebaaaae1a18986c50781fa1790748001235.webp`），十六进制里恰好连着四个 a，与 SEO 无关。本 change 不针对该假设，只修真实存在的空白与冗余。

另有两处伴生问题：第三方 HTML 经 `v-html` 直出存在注入面（`<img onerror=...>` 一类会真实执行）；`description` 与 `content` 在智源源上 2148/2148 完全相同，276 MB 里约 136 MB 是纯冗余。

## What Changes

- **description 入库即纯文本化**：RSS 入库点（`reader/service/feed_service.go:buildArticleFromEntry`）把 `description` 收敛为纯文本摘要——剥 `<script>`/`<style>`、剔除隐藏节点（`display:none` / `visibility:hidden` / `opacity:0` / `font-size:0`）、去标签、还原 HTML 实体、折叠空白、截断至 ≤300 字符（尾部省略号）。`content` 字段**不动**（正文源继续保留全文 HTML）。
- **清洗口径唯一实现**：把列表 `excerpt` 已有的清洗逻辑（`reader/handler/article_excerpt.go:stripExcerptMarkup` + `decodeHTMLEntities`）上提到共享包（`internal/platform/htmltext`），handler 与 service 共用同一套清洗口径，只允许长度上限不同（excerpt ≤200 / description ≤300）。
- **导语段改纯文本渲染**：阅读页导语不再 `v-html` 直出 `description`，改为「去标签 → 纯文本 → 截断 → `v-text` 渲染」。即使存量数据尚未回填，第三方 HTML 也不会再进 DOM，空白随之消失（`opacity:0` 等行内样式随标签一并剥除）。
- **内容源切换的「原文」源加护栏**：切到原文源时对 `content` 做保守清洗——移除 `<script>`/`<style>`、事件属性（`on*`）、可执行嵌入元素（`iframe`/`object`/`embed`/`form`/`applet`/`base`）与 `srcdoc`、过滤 `javascript:`/`vbscript:`/`data:` 危险 URL 方案；隐藏处理分两类：**视觉隐藏声明**（`visibility:hidden`/`opacity:0`/`font-size:0`）剥离声明使内容重新可见，**语义不展示**（`display:none`/`hidden` 属性）移除元素连同子树；其余结构与排版保留，避免同一篇空白在原文源复现。
- **刷新判重改用清洗后的值**：入库存的是清洗后摘要，刷新判定 MUST 比较同口径的值（清洗后摘要 + 未改写的正文源字段），否则「摘要即整篇 HTML」的源每轮刷新都被判为「内容已变」→ 重置状态、删标签、重跑 Firecrawl + LLM + 打标链。
- **整理稿兜底改用正文源字段的文本投影**：Firecrawl 失败时降级用的不再是（可能为空的）摘要，而是 `articles.content` 的纯文本投影（保留隐藏子树文本——被视觉藏起来的正文对总结这个用途是有效内容）。
- **存量回填**：新增一次性命令 `cmd/clean-article-descriptions`，把历史 `description` 按同一口径清洗（默认 dry-run，`--apply` 才写库，幂等，可重复执行）。只清洗 `description`，不动 `content`。
- **不改**：AI 整理稿与正文渲染管线、列表 `excerpt` 的既有字段与判定契约、Firecrawl 抓取链与降级链、`articles.content` 语义、API 路径与响应字段集。

### Out of Scope

- 不删 `articles.content` 里的全文 HTML（正文源保留，回滚零数据迁移）；`description` 与 `content` 的冗余只通过清洗 `description` 消掉一半。
- 不引入 DOMPurify 等前端依赖：清洗用浏览器原生 `DOMParser` 实现（前端 utils 层），后端用标准库解析。
- 不改 RSSHub 侧 `baai` 路由的产出（源站行为不可控）。

## Capabilities

**New Capabilities**：

- `article-description-sanitization`：文章摘要字段的纯文本化入库契约、第三方 HTML 的安全渲染契约、存量回填契约。

**Modified Capabilities**：

- `reading-article-pane`：改写「简介导语段按实质内容呈现」——补「导语 MUST 以去标签纯文本渲染（禁止 `v-html` 直出第三方 HTML）、长度 ≤300 字符」；并在「手动操作与内容源切换收进更多菜单」补「原文源渲染前 MUST 经保守清洗」。
- `article-list-projection`：改写「excerpt 承载列表层导语」——把 excerpt 的清洗口径写全（与文章摘要同源同实现：还原实体 → 按 HTML 解析 → 剔隐藏子树 → 去标签 → 折叠空白；截断不带省略号；无字母数字即空串），并补「整体隐藏的源其导语为空串」。

## Impact

- **后端**：`backend-go/internal/platform/htmltext/`（新包）、`backend-go/internal/reader/service/feed_service.go`、`backend-go/internal/reader/handler/article_excerpt.go`、`backend-go/cmd/clean-article-descriptions/`（新命令）。
- **前端**：`front/app/utils/`（新增 HTML 清洗/纯文本工具）、`front/app/features/articles/components/ArticleContentPreviewPanel.vue`、`front/app/features/articles/composables/useArticleContentView.ts`。
- **数据**：`articles.description` 语义由「源站原文（可能是整篇 HTML）」变为「≤300 字符纯文本摘要」。字段名、API 路径与响应字段集均不变；`articles.content` 不变。
- **存量规模**：全库 13602 行 description 含 HTML；智源源 2148 行。清洗后该字段体积从 276 MB 降至 MB 级。
- **依赖**：无新增外部依赖；不引入前端包。
