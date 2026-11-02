/**
 * DailyReportGenerateDialog（R28）：真实进度 done 上升沿 emit completed，
 * 供壳层刷新独立日报元信息与 A；不触发真实生成。
 */
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

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
