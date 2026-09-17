
## ArticleContent.css 共享样式作用域

`front/app/components/article/ArticleContent.css` 是共享样式文件，除阅读页三组件（ArticleContentView/ArticleContentToolbar/ArticleContentPreviewPanel）外，tags 域 QAPanel.vue / CausalAnalysisReport.vue / BoardEnrichmentPanel.vue 也 import 它，借用全局 `.markdown-body` 类渲染 markdown 产物，并在各自 scoped 样式覆盖字号（如 QAPanel 364 行把 1.0625rem 覆盖回 13.5px）。

重排设计的作用域结论：
- 阅读列版式类（.preview-mode/.article-title-full/.article-body/.summary-surface/.article-meta 等）可自由重写——仅阅读页消费；
- `.markdown-body` 基础元素排版（p/h/li 等）保持兼容不动——tags 面板依赖；
- 去卡片化 blockquote、宋体 h2/h3、表格/图片 breakout 负边距等新编辑版式规则 MUST 限定作用域在阅读上下文（`.preview-mode` 下或 `.markdown-article/.markdown-summary`），否则负边距 breakout 会击穿 tags 面板紧凑布局、blockquote 大色块 removal 也会改变其他面板观感。

<!-- pinned 2026-09-17T10:30:19Z -->
