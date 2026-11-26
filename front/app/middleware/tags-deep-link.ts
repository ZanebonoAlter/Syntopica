/**
 * 旧版块深链迁移（rework-workshop-into-radar-and-desk 1.1）。
 *
 * `/tags?board=<id>` → `/tags/<id>`（保留其余 query）。规则：
 * - `board` 缺失/非法（0/负数/非整数/空串）→ 不重定向，旧页照常渲染。
 * - 带 `report` 深链（页边注「跳原日报」）→ 放行到 `/tags`，由页面反查日期后
 *   replace 到 `/tags/:boardId/:date?annotation=`（阅读台路由）。
 *
 * 纯解析在 `parseBoardDeepLink`（composables/useBoardConsole.ts），本文件只做导航。
 */
import { hasReportDeepLink, parseBoardDeepLink } from '~/features/tags/composables/useBoardConsole'

export default defineNuxtRouteMiddleware((to) => {
  if (to.path !== '/tags') return
  const query = to.query as Record<string, unknown>
  if (hasReportDeepLink(query)) return
  const deepLink = parseBoardDeepLink(query)
  if (!deepLink) return
  const rest: typeof to.query = { ...to.query }
  delete rest.board
  return navigateTo({ path: `/tags/${deepLink.boardId}`, query: rest }, { replace: true })
})
