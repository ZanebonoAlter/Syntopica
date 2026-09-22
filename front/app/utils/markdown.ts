import { marked } from "marked";
import { proxyImagesInHtml } from "./imageProxy";

/**
 * 共用 markdown 渲染（marked@17，配置对齐 useArticleContentView）。
 *
 * 用于 LLM 产出的半可信文本（因果报告字段 / 追问回答 / 新闻背景叙事）。
 * 安全策略与 ArticleContentPreviewPanel 一致：marked + v-html，不额外消毒
 * （单用户本地应用，与项目现网用法同档，不引入新依赖）。
 *
 * 渲染产物配全局 `.markdown-body` class（components/article/ArticleContent.css）
 * 获得标题/列表/粗体/引用块样式 + 双主题适配。
 */
marked.setOptions({ gfm: true, breaks: true });

/** 块级渲染（段落/列表/标题），返回含 <p> 等块标签的 HTML。宿主用 <div> + v-html。 */
export function renderMarkdown(text: string | null | undefined): string {
	if (!text) return "";
	return proxyImagesInHtml(marked.parse(text) as string);
}

/** 行内渲染（粗体/链接/行内码），不含块级包裹，用于标题/短句/span 宿主。 */
export function renderMarkdownInline(text: string | null | undefined): string {
	if (!text) return "";
	return proxyImagesInHtml(marked.parseInline(text) as string);
}
