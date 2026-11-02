/**
 * ObservationMatrix 测试共用 fixture 与挂载装配（8C.4S 结构规范包从 ObservationMatrix.test.ts 抽出）。
 *
 * 只放**测试数据与 mount 装配**：14 日轴/档起始、档内三态 resolver、主 fixture（10 多桶/11 单日/
 * 12 休眠）、行拆分、矩阵 mount 默认 props、`buildOtherTopics` 组视图 fixture。断言与故事留在：
 *  - `ObservationMatrix.test.ts`：矩阵派生（MX 组）、名单稳定与三态、交互与状态
 *  - `ObservationMatrix.dayHeaders.test.ts`：报日列头三态（RDY/GRP-1）
 *  - `ObservationMatrix.groups.test.ts`：组顺序与折叠独立性、其他行只标所选日（GRP-1/3/4、OTH-12）
 *
 * 本文件不得以 `*.test.ts` 命名（会被 Vitest 当测试收集）；`vi.mock` 留在各测试文件内
 * （模块 mock 必须在该测试文件的 import 图里注册），本文件不 mock 模块、不跨文件共享状态。
 */
import { mount } from '@vue/test-utils'
import ObservationMatrix from './ObservationMatrix.vue'
import { buildObservationRows, splitObservationRows } from './observationLanes'
import type { OtherTopicsView } from './dayOtherTopics'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import { normalizeLaneDynamicsWindow, type LaneDynamicsWindowData, type WindowLaneDay, type WindowSection } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'

export const TODAY = '2026-10-23'
export const AXIS14 = buildDayAxis(TODAY, 14)
export const TIER_START = '2026-10-10'

function sectionRaw(id: number, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return { section_id: id, label: `桶${id}`, events: [`事件${id}`], ...overrides }
}

function normalize(lanes: Record<string, unknown>[]): LaneDynamicsWindowData {
  return normalizeLaneDynamicsWindow({ window_days: 14, has_reports: true, lanes, candidates: [] } as unknown as LaneDynamicsResponse)
}

/** 主 fixture：10=双日多桶（峰值 7）、11=单日、12=休眠（1 桶）。 */
export function fixtureSource(): LaneDynamicsWindowData {
  return normalize([
    {
      topic_id: 10, label: '主线 A', watch_linked: true, section_count_14d: 2, snapshot: null,
      timeline: [
        { date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 3 }), sectionRaw(2, { thread_count: 7 }), sectionRaw(3, { thread_count: 1 })] },
        { date: '2026-10-21', sections: [sectionRaw(4, { thread_count: 0 })] },
      ],
    },
    { topic_id: 11, label: '主线 B', watch_linked: false, section_count_14d: 3, snapshot: null, timeline: [
      { date: '2026-10-22', sections: [sectionRaw(5, { thread_count: 4 })] },
      { date: '2026-10-21', sections: [sectionRaw(7, { thread_count: 1 }), sectionRaw(8, { thread_count: 2 })] },
    ] },
    { topic_id: 12, label: '休眠线 C', watch_linked: false, section_count_14d: 1, snapshot: null, timeline: [
      { date: '2026-10-20', sections: [sectionRaw(6, { thread_count: 1 })] },
    ] },
  ])
}

export function fixtureRows(source = fixtureSource()) {
  return splitObservationRows(buildObservationRows(source, AXIS14))
}

export function windowDay(date: string, sections: Partial<WindowSection>[]): WindowLaneDay {
  return {
    date,
    sections: sections.map((section, index) => ({
      sectionId: section.sectionId ?? String(index + 1),
      label: section.label ?? `桶${index + 1}`,
      threadCount: section.threadCount ?? null,
      fallbackClueCount: section.fallbackClueCount ?? null,
      foldedCount: section.foldedCount ?? 0,
      eventTitles: section.eventTitles ?? [],
    })),
  }
}

/** 档内三态 resolver（对齐 G1 resolveLaneDay 语义；10 的 10-21 为已知 0 日）。 */
export function tierResolver(source: LaneDynamicsWindowData = fixtureSource()) {
  const index = new Map(source.lanes.map(lane => [lane.topicId, new Map(lane.timeline.map(day => [day.date, day]))]))
  return (topicId: string, date: string) => {
    const day = index.get(topicId)?.get(date)
    return day ? { kind: 'content' as const, source: 'tier' as const, day } : { kind: 'gap' as const }
  }
}

export function mountMatrix(overrides: Record<string, unknown> = {}) {
  const { active, dormant } = fixtureRows()
  return mount(ObservationMatrix, {
    props: {
      axis: AXIS14,
      activeRows: active,
      dormantRows: dormant,
      selectedLaneId: '10',
      selectedDate: null,
      today: TODAY,
      sourceDays: 14,
      anchorDate: TODAY,
      resolveDay: tierResolver(),
      expandedDormant: false,
      loading: false,
      error: null,
      staleNotice: false,
      emptyState: null,
      ...overrides,
    },
  })
}

/** G1 `buildOtherTopics` 派生的组视图 fixture（矩阵只透传，不重算）。 */
export function otherView(overrides: Partial<OtherTopicsView> = {}): OtherTopicsView {
  return {
    state: 'ready',
    rows: [
      {
        sectionId: '701',
        label: '乌干达因过境旅客确诊加德满都口岸加强防疫',
        clusterIndex: 0,
        threadCount: 1,
        persistentTopicId: null,
        statusKind: 'unattributed',
        statusText: '当次未归属',
      },
      {
        sectionId: '702',
        label: '第二条其他分区（零线索）',
        clusterIndex: 1,
        threadCount: 0,
        persistentTopicId: '88',
        statusKind: 'active',
        statusText: '当时已转正',
      },
    ],
    count: 2,
    statusText: null,
    note: null,
    retryable: false,
    ...overrides,
  }
}
