type BrowserLocationLike = {
  origin: string
}

function getBrowserLocation(): BrowserLocationLike | null {
  if (import.meta.server) return null
  return (globalThis as { window?: { location: BrowserLocationLike } }).window?.location ?? null
}

function getConfigApiBase(): string {
  const config = useRuntimeConfig()
  return config.public.apiBase as string
}

/**
 * API base 解析规则（契约见 openspec spec `dev-api-networking`）：
 * - 绝对 http(s) base（默认 `http://localhost:5100/api`，或显式 `NUXT_PUBLIC_API_BASE`）
 *   原样生效——浏览器/WSL 工具直连后端（后端 CORS 白名单放行前端 origin）；
 * - 相对 base（如 `/api`）同源解析——供未来同源反代部署使用。
 */
export function getApiBaseUrl(): string {
  return getConfigApiBase()
}

/** API origin：绝对 base 取其 origin；相对 base 取页面 origin（WS/资源 URL 拼接用）。 */
export function getApiOrigin(): string {
  const base = getConfigApiBase()
  if (base.startsWith('http')) return new URL(base).origin
  return getBrowserLocation()?.origin ?? ''
}
