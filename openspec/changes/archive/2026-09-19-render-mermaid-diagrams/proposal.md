<!-- complexity: simple -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading -->

## Why

AI 总结与抓取正文里的 mermaid 图在阅读页只渲染出源代码（存量：AI 总结 29 篇带 ` ```mermaid ` 围栏 + firecrawl 抓取丢语言标记的裸围栏形态，实锤文章 #131733）——`marked` 把 ` ```mermaid ` 围栏当普通代码块输出 `<pre><code class="language-mermaid">`，全项目无任何 mermaid 集成。用户读不到图，信息损失。

## What Changes

- 新增共享 mermaid 渲染能力：markdown 块级渲染产物中出现 `language-mermaid` 代码块时，客户端动态 `import('mermaid')`（按需 chunk，无 mermaid 块的会话零下载），原地替换为 SVG 图。
- 渲染失败（LLM 产出语法不合法常见）保留原源码块并附轻量错误提示，不白屏、不吞内容。
- 图容器主题随 `data-theme`（editorial/dark）双主题适配，主题切换时已渲染图重绘。
- 覆盖全部 LLM 产出 markdown 宿主：阅读页 AI 整理稿 + firecrawl 正文（`useArticleContentView`）与 `utils/markdown.ts` 的 `renderMarkdown` 块级调用方（QAPanel / CausalAnalysisReport / BoardEnrichmentPanel）。
- 纯前端改动：无后端/DB/数据迁移，29 篇存量文章部署后自动变图。

## Capabilities

### New Capabilities

- `markdown-mermaid-render`: markdown 宿主内 mermaid 围栏块的客户端渲染行为契约——围栏→SVG 的转换时机、动态加载边界、失败降级（保留源码+提示）、主题双适配与重绘、共享渲染器全部调用方的覆盖要求。

### Modified Capabilities

- `reading-article-pane`: 阅读列内 mermaid 图的呈现要求——图容器样式遵守去卡片化（留白/细分隔线，不做边框+投影卡片）、列宽内自适应缩放（横向可滚动兜底）、与 AI 整理稿/正文的排版衔接；不改既有版式需求。

## Impact

- **前端**：新增 composable（mermaid 动态加载 + DOM 后处理）；`useArticleContentView.ts` / `ArticleContentPreviewPanel.vue` 挂接；`utils/markdown.ts` 消费方四面板复用同一挂接；`ArticleContent.css` 新增 `.mermaid-block` 容器规则（纯增量，不动既有元素排版——遵守 `.markdown-body` 共享宿主红线）。
- **依赖**：`front/package.json` 新增 `mermaid`（v11，动态 import，仅客户端；构建产物 +一个按需 chunk）。
- **后端**：零改动。
- **数据**：零迁移；存量 29 篇自动受益。
- **部署影响**：用户打开文章即见图（此前为源码块）；无需任何手动操作。
