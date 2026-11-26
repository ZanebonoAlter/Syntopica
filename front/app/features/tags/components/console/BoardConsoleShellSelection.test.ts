/**
 * BoardConsoleShell — 唯一选中日与档/自由观察范围（8C G4b 从 1094 行巨型壳测试按组拆出；用例逐字保留，
 * fullName/断言不变）。通用 mock/挂载/hooks 见 `boardConsoleShellTestHarness.ts`。
 */
import { describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  routeState,
  router,
  applyQuery,
  getLaneDynamics,
  getDailyReportDetail,
  openArticlePreviewMock,
  poolResponse,
  response,
  lane,
  TODAY,
} from './boardConsoleShellTestHarness'
import ObservationRangeControls from './ObservationRangeControls.vue'
import ObservationMatrix from './ObservationMatrix.vue'
import LaneChronicle from './LaneChronicle.vue'

installShellTestHooks()

describe('BoardConsoleShell — 唯一选中日与就地下读', () => {
  it('点有分区格：共享 selectedDate、编年就地显示全量真实 thread，不打开抽屉', async () => {
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle"]').exists()).toBe(true)
    await wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(routeState.query.lane).toBe('10')
    expect(wrapper.find('[data-testid="lane-chronicle"]').text()).toContain(TODAY)
    expect(wrapper.find('[data-testid="lane-chronicle"]').text()).toContain('完整线索标题')
    expect(getDailyReportDetail).toHaveBeenCalledWith(60)
  })

  it('编年文章主点击接既有站内原文 reader（不加导语中间层）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent(LaneChronicle).vm.$emit('openArticle', '42')
    expect(openArticlePreviewMock).toHaveBeenCalledWith(42)
  })

  it('矩阵/演进/编年共用选中日：演进节点 click 改日，编年跟随', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle"]').text()).toContain(TODAY)
  })
})

describe('BoardConsoleShell — 档 vs 自由观察范围', () => {
  it('仅独立档按钮切 sourceDays；自由范围恰 7 日不换档、不重发其他档', async () => {
    const wrapper = mountShell()
    await flushPromises()
    const callsAfterInitial = getLaneDynamics.mock.calls.length
    const controls = wrapper.findComponent(ObservationRangeControls)

    controls.vm.$emit('commit', { start: '2026-10-17', end: '2026-10-23' })
    await flushPromises()
    expect(getLaneDynamics.mock.calls.length).toBe(callsAfterInitial)

    const tierButtons = wrapper.findAll('.orc-tier')
    const thirty = tierButtons.find(button => button.text().includes('30 天'))
    expect(thirty).toBeTruthy()
    await thirty!.trigger('click')
    await flushPromises()
    expect(getLaneDynamics).toHaveBeenLastCalledWith(1974, 30)
  })

  it('R29：B 池失败独立提示/重试；只重发 365，A/选中/范围不变，成功恢复历史', async () => {
    let poolOk = false
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve(
      days === 365
        ? (poolOk ? { success: true, data: poolResponse() } : { success: false, error: 'pool 500' })
        : { success: true, data: response() },
    ))
    const wrapper = mountShell()
    await flushPromises()
    const aCallsBefore = getLaneDynamics.mock.calls.filter(([, days]) => days === 14).length

    wrapper.findComponent(ObservationRangeControls).vm.$emit('commit', { start: '2026-06-15', end: '2026-10-23' })
    await flushPromises()
    await flushPromises()
    const notice = wrapper.find('[data-testid="matrix-pool-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('pool 500')
    const aCallsAfterFailure = getLaneDynamics.mock.calls.filter(([, days]) => days === 14).length
    const poolCallsAfterFailure = getLaneDynamics.mock.calls.filter(([, days]) => days === 365).length
    expect(wrapper.find('[data-testid="matrix-lane-10"]').classes()).toContain('is-selected')
    expect(wrapper.find('[data-testid="matrix-tier-note"]').text()).toBe('历史仅补数据，泳道名单不变')
    expect(routeState.query.start).toBe('2026-06-15')
    expect(routeState.query.end).toBe('2026-10-23')

    poolOk = true
    await wrapper.find('[data-testid="matrix-pool-retry"]').trigger('click')
    await flushPromises()
    await flushPromises()
    expect(getLaneDynamics.mock.calls.filter(([, days]) => days === 365).length).toBe(poolCallsAfterFailure + 1)
    expect(getLaneDynamics.mock.calls.filter(([, days]) => days === 14).length).toBe(aCallsAfterFailure)
    expect(getLaneDynamics.mock.calls.filter(([, days]) => days === 14).length).toBe(aCallsBefore)
    expect(wrapper.find('[data-testid="matrix-pool-notice"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-cell-10-2026-06-15"]').exists()).toBe(true)
  })

  it('池首取失败：首屏即提示 + 可重试（不等用户拖出档窗；A 侧照常可读）', async () => {
    let poolOk = false
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve(
      days === 365
        ? (poolOk ? { success: true, data: poolResponse() } : { success: false, error: 'pool 500' })
        : { success: true, data: response() },
    ))
    const wrapper = mountShell()
    await flushPromises()
    // 用户未做任何拖动：池失败提示已出现（此前被 needsPool 门控，首屏静默）
    const notice = wrapper.find('[data-testid="matrix-pool-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('pool 500')
    // A 侧照常可读（名单/矩阵/选中泳道不受池失败影响）
    expect(wrapper.find('[data-testid="matrix-lane-10"]').exists()).toBe(true)
    // 重试成功 → 提示消失，历史格可读
    poolOk = true
    await wrapper.find('[data-testid="matrix-pool-retry"]').trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="matrix-pool-notice"]').exists()).toBe(false)
  })

  it('池失败后切「近 1 年」档：池提示消失（365 档即池，无独立池可失败）', async () => {
    let poolFail = true
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve(
      days === 365
        ? (poolFail ? { success: false, error: 'pool 500' } : { success: true, data: poolResponse() })
        : { success: true, data: response() },
    ))
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="matrix-pool-notice"]').exists()).toBe(true)

    poolFail = false
    const yearButton = wrapper.findAll('.orc-tier').find(button => button.text().includes('近 1 年'))
    expect(yearButton).toBeTruthy()
    await yearButton!.trigger('click')
    await flushPromises()
    await flushPromises()
    // 365 档下没有独立的池可失败 → 提示条不得残留误述
    expect(wrapper.find('[data-testid="matrix-pool-notice"]').exists()).toBe(false)
  })

  it('R35：手动 deferred 导航未提交时，缓存续体已跑也不吃回旧 2 日范围', async () => {
    let hold365 = false
    let release365: (() => void) | null = null
    router.replace.mockImplementation((to: { query: Record<string, unknown> }) => {
      const apply = () => { routeState.query = { ...to.query } }
      if (hold365 && String(to.query?.days) === '365') {
        return new Promise<void>((resolve) => {
          release365 = () => { apply(); resolve() }
        })
      }
      apply()
      return Promise.resolve()
    })
    const wrapper = mountShell()
    await flushPromises()

    const tierButton = (label: string) => wrapper.findAll('.orc-tier').find(button => button.text().includes(label))!
    // 预热 365 缓存
    await tierButton('近 1 年').trigger('click')
    await flushPromises()
    await flushPromises()
    // 30 档自定义 2 日
    await tierButton('30 天').trigger('click')
    await flushPromises()
    wrapper.findComponent(ObservationRangeControls).vm.$emit('commit', { start: '2026-10-15', end: '2026-10-16' })
    await flushPromises()
    expect(routeState.query.start).toBe('2026-10-15')

    // 目标档已缓存；导航 deferred 未提交，query 仍旧，缓存续体先完成
    hold365 = true
    await tierButton('近 1 年').trigger('click')
    await flushPromises()
    await flushPromises()

    const range = wrapper.findComponent(ObservationRangeControls).props('range') as { start: string, end: string }
    const matrixAxis = wrapper.findComponent(ObservationMatrix).props('axis') as string[]
    expect(range.end).toBe(TODAY)
    expect(range.start).not.toBe('2026-10-15')
    expect(matrixAxis).toHaveLength(365)
    expect(routeState.query.days).toBe('30')
    expect(routeState.query.start).toBe('2026-10-15')
    expect(routeState.query.end).toBe('2026-10-16')
    expect(release365).not.toBeNull()

    // 真正提交导航后，默认范围/轴/URL 一致且旧 start/end 被替换
    release365!()
    await flushPromises()
    await flushPromises()
    expect(routeState.query.days).toBe('365')
    expect('start' in routeState.query).toBe(false)
    expect('end' in routeState.query).toBe(false)
    expect((wrapper.findComponent(ObservationRangeControls).props('range') as { start: string, end: string }).start).toBe(range.start)
    expect((wrapper.findComponent(ObservationMatrix).props('axis') as string[])).toHaveLength(365)
    router.replace.mockImplementation(applyQuery)
  })

  it('A 名单行随响应更新：切 30 档后新名单立即进入矩阵（computed 不冻结）', async () => {
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve({
      success: true,
      data: days === 30
        ? {
            window_days: 30,
            has_reports: true,
            lanes: [lane(77, '三十日新名单', { [TODAY]: 2 })],
            candidates: [],
          }
        : response(),
    }))
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.text()).toContain('主线 A')

    const thirty = wrapper.findAll('.orc-tier').find(button => button.text().includes('30 天'))
    await thirty!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('三十日新名单')
    // 断言范围限定在矩阵区（切 30 档后名单换成「三十日新名单」）：
    // 右轨「当日其他话题」子块（add-other-topics-to-context-rail）也会渲染分区名「主线 A」，
    // 那是右轨内容、与矩阵名单无关，故不拿整页文本断言。
    expect(wrapper.find('[data-testid="board-matrix"]').text()).not.toContain('主线 A')
  })
})

