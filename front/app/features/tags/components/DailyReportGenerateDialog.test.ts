/**
 * DailyReportGenerateDialog（R28）：真实进度 done 上升沿 emit completed，
 * 供壳层刷新独立日报元信息与 A；不触发真实生成。
 */
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { todayKey } from '~/utils/todayKey'

const state = vi.hoisted(() => ({ done: null as unknown as { value: boolean } }))

vi.mock('~/composables/useDailyReportProgress', async () => {
  const { ref } = await import('vue')
  const done = ref(false)
  state.done = done
  return {
    useDailyReportProgress: () => ({
      progress: ref(new Map()),
      done,
      jobId: ref(null),
      totalSaved: ref(0),
      totalBoards: ref(0),
      reset: vi.fn(),
    }),
  }
})
vi.mock('~/api/dailyReports', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/dailyReports')>()
  return { ...original, useDailyReportsApi: () => ({ generateDailyReport: vi.fn() }) }
})
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

import DailyReportGenerateDialog from './DailyReportGenerateDialog.vue'

// 默认日期必须按**本地**日判定：把时区钉在 UTC+8（本地日与 UTC 日存在差异的
// 区间才能区分两种实现），并在用例结束后恢复原设置（不污染同 worker 的其他文件）。
const originalTz = process.env.TZ
beforeAll(() => { process.env.TZ = 'Asia/Shanghai' })
afterAll(() => {
  if (originalTz === undefined) delete process.env.TZ
  else process.env.TZ = originalTz
})

describe('DailyReportGenerateDialog — 完成事件', () => {
  it('done false→true 时 emit completed（供 R28 刷新元信息/A）', async () => {
    const wrapper = mount(DailyReportGenerateDialog, {
      props: { visible: true, boards: [] },
      global: {
        stubs: {
          AppDialog: { name: 'AppDialog', props: ['modelValue'], template: '<div><slot /></div>' },
          AppInput: true,
          AppButton: true,
        },
      },
    })
    await flushPromises()
    state.done.value = true
    await nextTick()
    expect(wrapper.emitted('completed')).toHaveLength(1)

    state.done.value = false
    await nextTick()
    state.done.value = true
    await nextTick()
    expect(wrapper.emitted('completed')).toHaveLength(2)
  })
})

describe('DailyReportGenerateDialog — 默认日期', () => {
  it('默认日期是本地今天（todayKey；不走 toISOString 的 UTC 截断）', async () => {
    vi.useFakeTimers()
    try {
      // 绝对时刻：UTC 还是 10-24，UTC+8 的本地已是 10-25
      vi.setSystemTime(new Date('2026-10-24T20:00:00Z'))
      const wrapper = mount(DailyReportGenerateDialog, {
        props: { visible: true, boards: [] },
        global: {
          stubs: {
            AppDialog: { name: 'AppDialog', props: ['modelValue'], template: '<div><slot /></div>' },
            AppInput: { name: 'AppInput', props: ['modelValue'], template: '<input type="date" :value="modelValue">' },
            AppButton: true,
          },
        },
      })
      await nextTick()

      const value = (wrapper.find('input[type="date"]').element as HTMLInputElement).value
      expect(value).toBe(todayKey())
      expect(value).toBe('2026-10-25')
      // UTC 截断会落到昨天（本地 0–8 点正是这个坑）
      expect(value).not.toBe('2026-10-24')
    } finally {
      vi.useRealTimers()
    }
  })
})
