/**
 * 旧版块深链迁移中间件测试（rework-workshop-into-radar-and-desk task 1.1）。
 *
 * R-1/R-2/R-3：`/tags?board=` → `/tags/<id>`；带 report 的日报深链与非法 board 不重定向。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'

const navigateTo = vi.fn()

vi.stubGlobal('defineNuxtRouteMiddleware', (fn: unknown) => fn)
vi.stubGlobal('navigateTo', navigateTo)

const middleware = (await import('./tags-deep-link')).default as (
  to: { path: string, query: Record<string, unknown> },
) => unknown

function run(path: string, query: Record<string, unknown> = {}) {
  return middleware({ path, query })
}

beforeEach(() => {
  navigateTo.mockReset().mockReturnValue('navigated')
})

describe('tags-deep-link middleware', () => {
  it('版块深链：board 映射到路径，其余 query 保留（R-1）', () => {
    expect(run('/tags', { board: '1974', lane: '55' })).toBe('navigated')
    expect(navigateTo).toHaveBeenCalledWith(
      { path: '/tags/1974', query: { lane: '55' } },
      { replace: true },
    )
  })

  it('日报深链（report 非空）不重定向，仍由旧页承载（R-2，阶段①桥接）', () => {
    expect(run('/tags', { board: '1974', report: '1211', annotation: '8' })).toBeUndefined()
    expect(navigateTo).not.toHaveBeenCalled()
  })

  it('非法 board：缺省/空串/0/负数/非数字都不重定向（R-3）', () => {
    for (const query of [{}, { board: '' }, { board: '0' }, { board: '-3' }, { board: 'abc' }]) {
      expect(run('/tags', query)).toBeUndefined()
    }
    expect(navigateTo).not.toHaveBeenCalled()
  })

  it('非 /tags 路径不处理', () => {
    expect(run('/tags/1974', { board: '1975' })).toBeUndefined()
    expect(navigateTo).not.toHaveBeenCalled()
  })
})
