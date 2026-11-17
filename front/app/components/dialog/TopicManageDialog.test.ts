import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import TopicManageDialog from './TopicManageDialog.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import type { BoardTopicListItem } from '~/api/dailyReports'

/**
 * 泳道管理弹窗（lane-manual-correction-loop 6.4）：
 * 标题由「话题管理」更名为「泳道管理」；行内补「编辑」入口 → emit `editLane`（进修正工作面）。
 */
const { listBoardTopicsMock, updateTopicMock } = vi.hoisted(() => ({
  listBoardTopicsMock: vi.fn(),
  updateTopicMock: vi.fn(),
}))

vi.mock('~/api/dailyReports', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/dailyReports')>()
  return {
    ...original,
    useDailyReportsApi: () => ({
      listBoardTopics: listBoardTopicsMock,
      updateTopic: updateTopicMock,
      deleteTopic: vi.fn(),
      mergeTopics: vi.fn(),
      backfillPersistentTopics: vi.fn(),
    }),
  }
})
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const TOPIC: BoardTopicListItem = {
  id: 42,
  semantic_board_id: 1974,
  label: 'Claude Code 工具链',
  description: '',
  status: 'active',
  first_seen_date: '2026-10-01',
  last_seen_date: '2026-10-23',
  hit_count: 4,
  consecutive_hits: 4,
  section_count: 4,
  color: '#888',
  can_activate: true,
}

let wrapper: VueWrapper | null = null

/** 弹窗按真实用法打开：先隐藏挂载、再置为可见（取数在 modelValue 的 watch 里触发）。 */
async function mountDialog() {
  wrapper = mount(TopicManageDialog, {
    props: { modelValue: false, boardId: 1974 },
    global: { components: { AppButton, AppDialog, AppInput } },
    attachTo: document.body,
  })
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

beforeEach(() => {
  listBoardTopicsMock.mockReset()
  listBoardTopicsMock.mockResolvedValue({ success: true, data: { topics: [TOPIC] } })
  updateTopicMock.mockReset()
  updateTopicMock.mockResolvedValue({ success: true, data: { id: 42 } })
})

/** 找弹窗内文案的按钮（泳道管理面板与重命名子弹窗同在 body 里，按文案定位）。 */
function buttonByText(text: string, scope: ParentNode = document.body): HTMLButtonElement {
  const btn = [...scope.querySelectorAll('button')].find(b => b.textContent?.trim() === text)
  if (!btn) throw new Error(`按钮未找到：${text}`)
  return btn
}

describe('TopicManageDialog — 泳道管理', () => {
  it('标题为「泳道管理」（不再是「话题管理」）', async () => {
    await mountDialog()
    const dialog = document.body.querySelector('.app-dialog') as HTMLElement
    expect(dialog.textContent).toContain('泳道管理')
    expect(dialog.textContent).not.toContain('话题管理')
  })

  it('列表行有「编辑」入口', async () => {
    await mountDialog()
    const editBtn = document.body.querySelector('[data-testid="topic-edit-lane"]') as HTMLButtonElement
    expect(editBtn).not.toBeNull()
    expect(editBtn.textContent?.trim()).toBe('编辑')
  })

  it('点「编辑」→ emit editLane(topicId)，且不触发任何写操作', async () => {
    const w = await mountDialog()
    ;(document.body.querySelector('[data-testid="topic-edit-lane"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(w.emitted('editLane')).toEqual([[42]])
    expect(w.emitted('changed')).toBeUndefined()
  })

  it('泳道名与状态如实展示', async () => {
    await mountDialog()
    const text = document.body.textContent ?? ''
    expect(text).toContain('Claude Code 工具链')
    expect(text).toContain('活跃')
    expect(text).toContain('4 条')
  })

  // 故事 C2（test-cases.md）：改名次要路径 = 泳道管理列表的 sm 档重命名弹窗（420px）保存生效。
  it('「重命名」次要路径：sm 档弹窗（420px）改名保存生效', async () => {
    const w = await mountDialog()
    buttonByText('重命名').click()
    await flushPromises()

    const dialogs = [...document.body.querySelectorAll('.app-dialog')] as HTMLElement[]
    const renameDialog = dialogs.find(d => d.textContent?.includes('重命名泳道'))
    expect(renameDialog).toBeDefined()
    expect(renameDialog!.className).toContain('is-sized')
    expect(renameDialog!.getAttribute('style')).toContain('420px')

    const input = renameDialog!.querySelector('input') as HTMLInputElement
    input.value = 'Claude Code 工具链（改名）'
    input.dispatchEvent(new Event('input'))
    await flushPromises()

    buttonByText('保存', renameDialog!).click()
    await flushPromises()

    expect(updateTopicMock).toHaveBeenCalledWith(42, { label: 'Claude Code 工具链（改名）' })
    expect(w.emitted('changed')).toBeTruthy()
  })
})
