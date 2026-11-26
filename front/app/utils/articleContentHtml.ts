/**
 * 原文源内容的渲染前加工链（fix-reader-lede-blank-space）。
 *
 * 阅读页「原文」源喂进来的是订阅源的整篇 HTML，未经信任：直接进 `v-html` 会
 * 让隐藏节点照样占位（智源社区的公众号文章整篇被 `opacity: 0` 根节点包裹，
 * 实测单篇产生 8046 px 空白）、事件属性被真实执行。
 *
 * 顺序即契约：**清洗必须排在图片代理之前**——先洗掉脏属性与隐藏声明，再改写
 * 外链为同源代理，否则被改写的地址会把未清洗的标记一起带进 DOM。
 *
 * 放在 utils 层而不是 composable 里：
 * - 它是纯函数，不依赖任何 store；
 * - composable 顶层 import 了 pinia stores，测试环境拉不起（`defineStore` 是
 *   Nuxt auto-import），而这条链路需要行为断言而不是源码字符串锚。
 */
import { sanitizeHtml } from '~/utils/htmlSanitize'
import { proxyImagesInHtml } from '~/utils/imageProxy'

/** Firecrawl 抓取失败的占位图，渲染前剔除（与正文/整理稿共用同一标记）。 */
const BASE64_PLACEHOLDER_IMG_RE = /<img[^>]*src=["']Base64-Image-Removed["'][^>]*\/?>/gi

/**
 * 把原文源入库值加工成可渲染 HTML：保守清洗 → 剔除 Base64 占位图 → 外链图片
 * 改走同源代理。
 */
export function buildOriginalContentHtml(raw: string): string {
  if (!raw) return ''
  const sanitized = sanitizeHtml(raw)
  return proxyImagesInHtml(sanitized.replace(BASE64_PLACEHOLDER_IMG_RE, ''))
}
