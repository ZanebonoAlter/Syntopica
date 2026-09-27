import { beforeEach, describe, expect, it } from 'vitest'
import { useConfirm } from './useConfirm'

/**
 * useConfirm 全局确认弹窗管道（fix-provider-delete-route-deadlock 2b）：
 * promise 结算 / 幂等 settle / 顶替语义 / 默认文案。
 * state 经 vitest.setup.ts 的 useState mock（Vue ref）驱动。
 */

describe('useConfirm', () => {
  beforeEach(() => {
    // 重置全局单例 state，避免用例间串扰
    const { state } = useConfirm()
    state.value = null
  })

  it('confirm 后 state 承载选项；settle(true) 结算为 true 且弹窗关闭', async () => {
    const { confirm, state, settle } = useConfirm()

    const pending = confirm({ title: '删除确认', message: '确定删除吗？', danger: true })

    expect(state.value?.open).toBe(true)
    expect(state.value?.title).toBe('删除确认')
    expect(state.value?.message).toBe('确定删除吗？')
    expect(state.value?.danger).toBe(true)

    settle(state.value!.id, true)
    await expect(pending).resolves.toBe(true)
    expect(state.value?.open).toBe(false)
  })

  it('settle(false) 结算为 false（取消）', async () => {
    const { confirm, state, settle } = useConfirm()

    const pending = confirm({ title: 't', message: 'm' })
    settle(state.value!.id, false)

    await expect(pending).resolves.toBe(false)
  })

  it('默认文案：确认/取消；danger 默认 false', () => {
    const { confirm, state } = useConfirm()

    confirm({ title: 't', message: 'm' })

    expect(state.value?.confirmText).toBe('确认')
    expect(state.value?.cancelText).toBe('取消')
    expect(state.value?.danger).toBe(false)
  })

  it('重复 settle 幂等：首次结算后再次 settle 不改变结果', async () => {
    const { confirm, state, settle } = useConfirm()

    const pending = confirm({ title: 't', message: 'm' })
    settle(state.value!.id, true)
    settle(state.value!.id, false)

    await expect(pending).resolves.toBe(true)
  })

  it('未决弹窗时再次 confirm：前一个以 false（取消）结算，新弹窗顶替', async () => {
    const { confirm, state, settle } = useConfirm()

    const first = confirm({ title: 'first', message: 'm1' })
    const second = confirm({ title: 'second', message: 'm2' })

    await expect(first).resolves.toBe(false)
    expect(state.value?.title).toBe('second')

    settle(state.value!.id, true)
    await expect(second).resolves.toBe(true)
  })
})
