/**
 * BoardWideWorkface（矩阵阶段 G5）：单级 1040px 工作面——
 * 右轨同一份上下文全量、体检依据全量、构成/组合/候选/来源/设置复用既有真实面板；
 * 管理动作只 emit 给壳层，不在本面再叠一层抽屉。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardWideWorkface from './BoardWideWorkface.vue'
import { normalizeRailHealth } from '~/api/normalizers/consoleRail'
import type { RailContext } from './railContext'

vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const RAIL: RailContext = {
  boardId: 1974,
  laneIds: ['1204'],
  topicCandidateCount: 3,
  health: {
    snapshot: normalizeRailHealth(null),
    loading: false,
    error: null,
    stale: false,
    score: null,
    tier: null,
    checkedAtText: '',
    trendPolyline: '',
    trendSummary: '',
  },
  watches: { items: [], loading: false, error: null },
  now: { date: null, status: 'loading', error: null, hasReport: false, reviews: [], lanes: [] },
}

function mountWorkface(overrides: Record<string, unknown> = {}) {
  return mount(BoardWideWorkface, {
    props: {
      open: true,
      boardId: 1974,
      boardLabel: '中东',
      board: null,
      tab: 'context',
      rail: RAIL,
      labels: [],
      composites: [],
      compositionLoading: false,
      compositionError: null,
      snapshot: null,
      capturingId: null,
      ...overrides,
    },
    global: {
      stubs: {
        AppDialog: {
          name: 'AppDialog',
          props: ['modelValue', 'size'],
          template: '<div v-if="modelValue" data-testid="workface-dialog"><slot name="header" /><slot /></div>',
        },
        BoardContextRail: {
          name: 'BoardContextRail',
          props: ['variant'],
          emits: ['open-candidates', 'open-health', 'open-watch-manage', 'locate-lane', 'open-topics'],
          template: '<div data-testid="context-rail-stub" :data-variant="variant"><button data-testid="ctx-candidates" @click="$emit(\'open-candidates\')" /><button data-testid="ctx-topics" @click="$emit(\'open-topics\')" /></div>',
        },
        BoardHealthWorkspace: {
          name: 'BoardHealthWorkspace',
          props: ['boardId', 'boardLabel', 'removedAuxIds'],
          emits: ['remove-composition'],
          template: '<div data-testid="health-workspace-stub" :data-board="boardId" :data-label="boardLabel" :data-removed="(removedAuxIds || []).join(\',\')">'
            + '<button data-testid="ws-remove" @click="$emit(\'remove-composition\', \'77\')" /></div>',
        },
        BoardHealthDetail: {
          name: 'BoardHealthDetail',
          props: ['mode'],
          template: '<div data-testid="health-detail-stub" :data-mode="mode" />',
        },
        BoardSourcePanel: {
          name: 'BoardSourcePanel',
          props: ['boardId'],
          template: '<div data-testid="source-panel-stub" />',
        },
        BoardSettingsForm: {
          name: 'BoardSettingsForm',
          props: ['board'],
          emits: ['saved', 'deleted'],
          template: '<button data-testid="settings-deleted" @click="$emit(\'deleted\')">删除</button>',
        },
      },
    },
  })
}

describe('BoardWideWorkface', () => {
  it('context 页：右轨同源四块全量（variant=full），候选入口切到候选页', async () => {
    const wrapper = mountWorkface()
    expect(wrapper.find('[data-testid="context-rail-stub"]').attributes('data-variant')).toBe('full')
    await wrapper.find('[data-testid="ctx-candidates"]').trigger('click')
    expect(wrapper.emitted('update:tab')).toEqual([['candidates']])
    await wrapper.find('[data-testid="ctx-topics"]').trigger('click')
    expect(wrapper.emitted('openTopics')).toHaveLength(1)
  })

  it('health / quality / sources 页复用真实面板而不做静态摘要', () => {
    // 体检页签 = 问题处理工作面（seam C2/3.4；完整依据收进工作面内的折叠区）
    const health = mountWorkface({ tab: 'health' })
    expect(health.find('[data-testid="health-workspace-stub"]').exists()).toBe(true)

    const quality = mountWorkface({ tab: 'quality' })
    expect(quality.find('[data-testid="health-detail-stub"]').attributes('data-mode')).toBe('quality')

    const sources = mountWorkface({ tab: 'sources' })
    expect(sources.find('[data-testid="source-panel-stub"]').exists()).toBe(true)
  })

  it('health 页：工作面拿到版块/标签与已摘除反馈；摘除请求外抛给宿主', async () => {
    const wrapper = mountWorkface({ tab: 'health', boardId: 1974, boardLabel: '中东', removedAuxIds: ['88'] })
    const workspace = wrapper.find('[data-testid="health-workspace-stub"]')
    expect(workspace.attributes('data-board')).toBe('1974')
    expect(workspace.attributes('data-label')).toBe('中东')
    expect(workspace.attributes('data-removed')).toBe('88')

    await wrapper.find('[data-testid="ws-remove"]').trigger('click')
    expect(wrapper.emitted('remove-composition')).toEqual([['77']])
  })

  // 4.1 C3-L1：「已摘除」徽标 = 会话摘除记录 ∩ **不在当前构成**。
  it('health 页：重新挂回后「已摘除」徽标自动撤销（会话记录不撤销，但徽标按当前构成取差集）', () => {
    const removed = mountWorkface({ tab: 'health', removedAuxIds: ['88'] })
    expect(removed.find('[data-testid="health-workspace-stub"]').attributes('data-removed')).toBe('88')

    const remountedAux = mountWorkface({ tab: 'health', removedAuxIds: ['88'], labels: [{ id: 88, label: '上海' }] })
    expect(remountedAux.find('[data-testid="health-workspace-stub"]').attributes('data-removed')).toBe('')

    const remountedComposite = mountWorkface({ tab: 'health', removedAuxIds: ['88'], composites: [{ id: 88, label: '上海 × 展会' }] })
    expect(remountedComposite.find('[data-testid="health-workspace-stub"]').attributes('data-removed')).toBe('')
  })

  it('settings 删除流程仍由既有表单执行并外抛给壳层（不在此新写删除算法）', async () => {
    const wrapper = mountWorkface({ tab: 'settings' })
    await wrapper.find('[data-testid="settings-deleted"]').trigger('click')
    expect(wrapper.emitted('deleted')).toHaveLength(1)
  })
})
