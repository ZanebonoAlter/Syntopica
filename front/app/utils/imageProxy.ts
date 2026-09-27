/**
 * 外链图片统一改写（openspec change add-image-proxy）。
 *
 * 库里存原始外链，渲染时才改写为同源代理——回滚零数据迁移（design D3）。
 * 新增图片加载点一律经 proxiedImageUrl / proxyImagesInHtml，不要直连外链：
 * 图床普遍有 Referer 防盗链，直连会 403 图裂（spec：前端外链图片统一改写）。
 */

const PROXY_PATH = "/api/image-proxy";

/**
 * 把 http(s) 外链图片改写为同源代理地址；以下情况原样返回（不二次嵌套、
 * 不误伤自身资源）：空值、已代理地址、相对路径与 `//` 协议相对、`data:`/
 * `blob:`、其它 scheme、指向当前页面源的同源绝对地址。
 */
export function proxiedImageUrl(url: string | null | undefined): string {
  if (!url) return "";
  if (url.includes(PROXY_PATH)) return url;
  if (/^(data|blob):/i.test(url)) return url;
  if (!/^https?:\/\//i.test(url)) return url;
  // 同源绝对地址直连即可，喂给代理会被自身的防循环校验拒绝。
  if (typeof window !== "undefined" && url.startsWith(window.location.origin)) return url;
  return `${PROXY_PATH}?url=${encodeURIComponent(url)}`;
}

/** `<img src="...">` / `<img src='...'>`，大小写不敏感；src 两侧引号必须成对。 */
const IMG_SRC_RE = /(<img[^>]*\ssrc=["'])([^"']+)(["'])/gi;

/** 把整段 HTML 里的外链 `<img src>` 批量改写为代理地址（正文/整理稿渲染产物共用）。 */
export function proxyImagesInHtml(html: string | null | undefined): string {
  if (!html) return "";
  return html.replace(IMG_SRC_RE, (_match, pre: string, src: string, post: string) =>
    pre + proxiedImageUrl(src) + post,
  );
}
