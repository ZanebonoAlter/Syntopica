import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useBoardComposition } from './useBoardComposition'

/**
 * 治理抽屉构成数据源（3.2）：取数 / 重取 / 移除后刷新 / 失败只提示不刷新。
 * 以宿主组件承载（composable 内 onMounted 需要组件上下文）。
 */
const { apiMock } = vi.hoisted(() => ({
  apiMock: { getComposition: vi.fn(), removeFromComposition: vi.fn() },
}))
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMocks }))

const mounted: Array<{ unmount: () => void }> = []

function labelItem(id: number, text: string) {
  return {
    id,
    label: text,
    slug: `l-${id}`,
    aliases: [],
    ref_count: 1,
    description: '',
    display_order: 0,
    source: 'manual',
    status: 'active',
    protected: false,
  }
}

function mountHost(onRemoved = vi.fn(), boardId = 1974) {
  const Host = defineComponent({
    setup() {
      const state = useBoardComposition(ref(boardId), { onRemoved })
      return { state }
    },
    template: '<div><span data-testid="labels">{{ state.labels.value.length }}</span></div>',
  })
  const wrapper = mount(Host)
  mounted.push(wrapper)
  return { wrapper, onRemoved }
}

beforeEach(() => {
  apiMock.getComposition.mockReset()
  apiMock.getComposition.mockResolvedValue({
    success: true,
    data: {
      items: [labelItem(7, '美伊关系'), labelItem(8, '霍尔木兹海峡')],
      composites: [{ id: 31, label: '组合', slug: 'c-31', status: 'active', ref_count: 2, components: ['a', 'b'] }],
      total: 2,
    },
  })
  apiMock.removeFromComposition.mockReset()
  apiMock.removeFromComposition.mockResolvedValue({ success: true, data: { board_id: 1974, auxiliary_label_id: 7 } })
  notifyMocks.success.mockReset()
  notifyMocks.error.mockReset()
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
})

describe('useBoardComposition', () => {
  it('挂载即取构成（标签 + 组合）', async () => {
    const { wrapper } = mountHost()
    await flushPromises()
    expect(apiMock.getComposition).toHaveBeenCalledWith(1974)
    expect(wrapper.find('[data-testid="labels"]').text()).toBe('2')
  })

  it('移除成功后重取构成并触发 onRemoved', async () => {
    const onRemoved = vi.fn()
    const Host = defineComponent({
      setup() {
        const state = useBoardComposition(ref(1974), { onRemoved })
        return { remove: state.removeLabel }
      },
      template: '<button data-testid="remove" @click="remove(7)" />',
    })
    const wrapper = mount(Host)
    mounted.push(wrapper)
    await flushPromises()

    await wrapper.find('[data-testid="remove"]').trigger('click')
    await flushPromises()

    expect(apiMock.removeFromComposition).toHaveBeenCalledWith(1974, 7)
    expect(apiMock.getComposition).toHaveBeenCalledTimes(2)
    expect(notifyMocks.success).toHaveBeenCalled()
    expect(onRemoved).toHaveBeenCalledTimes(1)
  })

  it('移除失败：提示错误，不重取也不触发 onRemoved', async () => {
    apiMock.removeFromComposition.mockResolvedValueOnce({ success: false, error: '后端 500' })
    const onRemoved = vi.fn()
    const Host = defineComponent({
      setup() {
        const state = useBoardComposition(ref(1974), { onRemoved })
        return { remove: state.removeLabel }
      },
      template: '<button data-testid="remove" @click="remove(7)" />',
    })
    const wrapper = mount(Host)
    mounted.push(wrapper)
    await flushPromises()

    await wrapper.find('[data-testid="remove"]').trigger('click')
    await flushPromises()

    expect(notifyMocks.error).toHaveBeenCalledWith('后端 500')
    expect(apiMock.getComposition).toHaveBeenCalledTimes(1)
    expect(onRemoved).not.toHaveBeenCalled()
  })
})
