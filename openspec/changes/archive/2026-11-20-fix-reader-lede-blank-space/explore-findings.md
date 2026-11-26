
## 智源正文空白根因：description 存整篇 HTML + v-html 直出

【现象】用户报「智源文章大片空白 + 疑似 SEO 关键词堆砌 aaaa」。

【aaaa 是误判】全库 description/content/firecrawl_content/ai_content_summary 剥标签、剥图片链接后搜 'aaaa' **0 命中**；浏览器实测页面可见文本也不含 aaaa。真相是智源配图 CDN 文件名哈希（simg.baai.ac.cn/hub-detail/3a8fed6844ebaaaae1a18986c50781fa1790748001235.webp），十六进制里连着四个 a。**源站没有关键词堆砌，不要按 SEO 方向排查。**

【空白真因】智源 RSS 条目 description = 整篇公众号 HTML（根 div 带 style="visibility: visible; opacity: 0;"）。链路：rss_parser.go:126 原样透传 → feed_service.go:buildArticleFromEntry 原样入库 → 前端 ArticleContentPreviewPanel.vue:119 `v-html="article.description"` 直出。根节点 opacity:0 → 整块不可见但照样占位：实测 .lede 高 8046px、.markdown-article 从 8787px 才开始；两处 innerText 均为 4819 字（同一篇文章渲染两遍）。visible 页面 body_has_aaaa=false。

【规模】智源 2148 篇，1958 篇 description 是 >2KB HTML，均值 66KB，最大 864KB；description 与 content 2148/2148 逐字相同（136MB 纯冗余）。全库 13602 行 description 含 HTML、合计 276MB；articles 表总 1020MB。同型受害源：掘金人工智能（4023/4101 篇，均值 16.6KB，最大 1.68MB）、A股资讯-金融界（3501 篇）、华尔街见闻-最热文章（995 篇）。

【既有护栏与它的盲区】shouldShowArticleDescription（front/app/utils/articleContentGuards.ts）用**纯文本归一化**比较 description 与正文，归一化后两者不等 → 判定「不重复」→ 保留导语；护栏只管「渲不渲染」，不管「渲什么」，不拦 opacity:0 的不可见占位。列表 excerpt 已有成熟清洗（reader/handler/article_excerpt.go:stripExcerptMarkup + decodeHTMLEntities，≤200），但只服务列表投影，详情 description 完全没走这套口径。

【不受影响的消费方】dataenrichment/service/signal_sources.go 明确只取 firecrawl_content/content、**不取 description**；tagmanagement 里的 Description 是标签/版块描述，与 articles.description 无关；models/article.go:58 是唯一 JSON 输出点。

【结论】治本 = 入库即纯文本化（≤300 字符）+ 渲染层护栏（不 v-html 直出第三方 HTML）；content 是正文源（fallback_crawler、signal_sources 都依赖），不能改。

**引用**：backend-go/internal/reader/service/rss_parser.go:126、backend-go/internal/reader/service/feed_service.go:buildArticleFromEntry、backend-go/internal/reader/handler/article_excerpt.go:stripExcerptMarkup、front/app/features/articles/components/ArticleContentPreviewPanel.vue:119、front/app/utils/articleContentGuards.ts:shouldShowArticleDescription、backend-go/internal/dataenrichment/service/signal_sources.go:138

<!-- pinned 2026-11-20T06:03:09Z -->
