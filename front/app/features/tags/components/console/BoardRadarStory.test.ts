import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import BoardRadarStory from './BoardRadarStory.vue'
import type { RadarBoard } from '~/api/semanticBoards'

/**
 * 雷达行尾「⋯」入口（lane-manual-correction-loop 6.5）：
 *  点行尾「⋯」**不触发行跳转**（不 emit openBoard）；菜单两项动作以 emit 上抛。
 *
 * 「展示合理性」机械锚：菜单 Teleport 到 body（不落在行 DOM 内），层级取自 `Z_INDEX` 档位表。
 */
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const ROW: RadarBoard = {
  boardId: '1974',
  label: '中东地缘政治与美伊关系',
  tagCount: 144,
  reportDate: '2026-10-24',
  reportTitle: '伊朗警告打击海湾海底电缆',
  reportArticleCount: 14,
  reportEventCount: 13,
  todayClues: 8,
  last3Clues: 13,
  prev3Clues: 10,
  attention: 3.5,
  heatRuleVersion: 'heat-v2-lane-clues-1d',
  clueDays: Array.from({ length: 14 }, (_, i) => (i === 13 ? 8 : 1)),
  dataDelayed: false,
  dataUnavailable: false,
  lanes: [
    { topicId: '1', label: '美伊博弈', threadCount14d: 40, clueCounts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 3 : 0)) },
  ],
}

let wrapper: VueWrapper | null = null

function mountStory() {
  wrapper = mount(BoardRadarStory, {
    props: { row: ROW, rank: 1, globalDate: '2026-10-24' },
    attachTo: document.body,
  })
  return wrapper
}

function trigger(): HTMLButtonElement {
  return document.body.querySelector('[data-testid="radar-menu-trigger-1974"]') as HTMLButtonElement
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('BoardRadarStory — 行尾「⋯」入口', () => {
  it('点行尾「⋯」不 emit openBoard（不误跳版块态势页）', async () => {
    const w = mountStory()
    trigger().click()
    await flushPromises()

    expect(w.emitted('openBoard')).toBeUndefined()
    expect(document.body.querySelector('[data-testid="row-action-menu"]')).not.toBeNull()
  })

  it('菜单含「编辑版块」「归档版块」两项', async () => {
    mountStory()
    trigger().click()
    await flushPromises()

    const items = [...document.body.querySelectorAll('.row-menu-item')].map(el => el.textContent?.trim())
    expect(items).toEqual(['编辑版块', '归档版块'])
  })

  it('选「编辑版块」→ emit editBoard(boardId)，不触发 openBoard', async () => {
    const w = mountStory()
    trigger().click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="radar-menu-trigger-1974-edit"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(w.emitted('editBoard')).toEqual([['1974']])
    expect(w.emitted('openBoard')).toBeUndefined()
  })

  it('选「归档版块」→ emit archiveBoard(boardId)，不触发 openBoard', async () => {
    const w = mountStory()
    trigger().click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="radar-menu-trigger-1974-archive"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(w.emitted('archiveBoard')).toEqual([['1974']])
    expect(w.emitted('openBoard')).toBeUndefined()
  })

  it('菜单 Teleport 到 body，层级取自 Z_INDEX 档位表', async () => {
    mountStory()
    trigger().click()
    await flushPromises()

    const menu = document.body.querySelector('[data-testid="row-action-menu"]') as HTMLElement
    expect(menu.parentElement).toBe(document.body)
    expect(menu.style.zIndex).toBe('9999')
  })

  it('点「矩阵」仍进版块态势页（原有入口不回归）', async () => {
    const w = mountStory()
    const enter = document.body.querySelector('.rd-enter') as HTMLButtonElement
    enter.click()
    await flushPromises()

    expect(w.emitted('openBoard')).toEqual([['1974']])
  })
})

describe('BoardRadarStory — 滞后版块沉底（fix-radar-overview-false-zero 追加）', () => {
  it('滞后行（锚点日 ≠ 全局锚点日）：分档徽标退为「数据至 MM/DD」，不染热点/关注色', () => {
    const w = mount(BoardRadarStory, {
      props: { row: { ...ROW, anchorDate: '2026-11-07', attention: 20 }, rank: 3, globalDate: '2026-10-24' },
      attachTo: document.body,
    })
    wrapper = w
    expect(w.find('.rd-story').classes()).not.toContain('is-hot')
    expect(w.find('.rd-story').classes()).not.toContain('is-warm')
    expect(w.find('.rd-tier').text()).toContain('数据至 11/07')
    expect(w.find('.rd-tier').text()).not.toContain('关注强度')
    // 关注度数值照实（沉底/弱化不是折扣）
    expect(w.find('.rd-attention').text()).toBe('20.0')
  })

  it('非滞后行（锚点日 = 全局锚点日）：照旧染关注色、挂分档徽标', () => {
    const w = mount(BoardRadarStory, {
      props: { row: { ...ROW, anchorDate: '2026-10-24' }, rank: 1, globalDate: '2026-10-24' },
      attachTo: document.body,
    })
    wrapper = w
    expect(w.find('.rd-story').classes()).toContain('is-warm')
    expect(w.find('.rd-tier').text()).toContain('关注强度 · 关注')
  })
})

describe('BoardRadarStory — 「没取到 ≠ 0」（fix-radar-overview-false-zero）', () => {
  /** 未取到的版块行：计数全是未知（null），不是 0。 */
  const MISSING: RadarBoard = {
    ...ROW,
    reportDate: '',
    reportTitle: '',
    reportArticleCount: null,
    reportEventCount: null,
    todayClues: null,
    last3Clues: null,
    prev3Clues: null,
    attention: null,
    dataUnavailable: true,
    lanes: [],
  }

  function mountMissing() {
    wrapper = mount(BoardRadarStory, {
      props: { row: MISSING, rank: 1, globalDate: '2026-10-24' },
      attachTo: document.body,
    })
    return wrapper
  }

  it('未取到的行：写「数据未取到」，不拿 0 冒充（无「0.0」/「0 篇文章」/「无日报日期」）', () => {
    const w = mountMissing()
    const text = w.text()
    expect(text).toContain('数据未取到')
    expect(text).not.toContain('0.0')
    expect(text).not.toContain('0 篇文章')
    expect(text).not.toContain('0 个事件')
    expect(text).not.toContain('无日报日期')
    expect(text).not.toContain('关注强度')
    expect(text).not.toContain('数据延迟')
    // 关注强度未知 → 「—」，不挂分档徽标
    expect(w.find('.rd-attention').text()).toBe('—')
    expect(w.find('.rd-tier').text()).toBe('数据未取到')
  })

  it('未取到的行：不染热点/关注色，3 日变化为「—」，不显示伪空态', () => {
    const w = mountMissing()
    expect(w.find('.rd-story').classes()).not.toContain('is-hot')
    expect(w.find('.rd-story').classes()).not.toContain('is-warm')
    expect(w.find('.rd-trend-value').text()).toBe('—')
    expect(w.find('.rd-trend-base').text()).toContain('近 3 日 — / 前 3 日 —')
    expect(w.find('.rd-mini').text()).toContain('数据未取到')
    expect(w.find('.rd-mini').text()).not.toContain('无活跃泳道')
  })

  it('日报信息没取到（brief 失败、泳道数据正常）：不说「无日报日期 / 今天暂无日报」（review R-5）', () => {
    wrapper = mount(BoardRadarStory, {
      props: {
        row: {
          ...ROW,
          // 报告计数未知、日期空串，但行本身没被标成未取到（泳道数据是好的）。
          reportDate: '',
          reportTitle: '',
          reportArticleCount: null,
          reportEventCount: null,
        },
        rank: 1,
        globalDate: '2026-10-24',
      },
      attachTo: document.body,
    })
    const text = wrapper.text()
    expect(text).toContain('日报信息未取到')
    expect(text).not.toContain('无日报日期')
    expect(text).not.toContain('今天暂无日报')
    expect(text).toContain('— 篇文章 · — 个事件')
    // 泳道数据正常 → 行本身不当成未取到
    expect(wrapper.find('.rd-tier').text()).not.toBe('数据未取到')
  })
})
