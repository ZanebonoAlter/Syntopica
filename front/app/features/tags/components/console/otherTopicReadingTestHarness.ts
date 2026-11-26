/**
 * OtherTopicReading 测试共用 fixture 与挂载装配（8C.4S 结构规范包从 OtherTopicReading.test.ts 抽出）。
 *
 * 只放**测试数据与 mount 装配**：固定 today/anchor/轴/选中日、thread/section/日报日 fixture、
 * 观察记录 fixture、deferred、注入真实 store 的 `mountReading`。断言与故事留在：
 *  - `OtherTopicReading.test.ts`：阅读正文（ORD）
 *  - `OtherTopicReading.observation.test.ts`：同归属跨日观察带（OBV）
 *  - `OtherTopicReading.signals.test.ts`：受控报告重试意图（M1）+ 阅读终态信号（M2）
 *
 * 本文件不得以 `*.test.ts` 命名（会被 Vitest 当测试收集）；`vi.hoisted`/`vi.mock` 与
 * beforeEach/afterEach 留在各测试文件内（模块 mock 必须在该测试文件的 import 图里注册），
 * 本文件不 mock 模块、不跨文件共享状态（每次 mountReading 自建 store/观察实例）。
 */
import { vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import OtherTopicReading from './OtherTopicReading.vue'
import type { LaneDayReportSection, LaneDayThread } from '~/api/normalizers/laneDayReading'
import type { LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import { useLaneDayReading } from '~/features/tags/composables/useLaneDayReading'
import { useOtherTopicObservation } from '~/features/tags/composables/useOtherTopicObservation'
import type { ObservationResponse, ObservationSectionRecord, ObservationTimelineData } from '~/api/otherTopicObservation'

export const TODAY = '2026-10-23'
export const ANCHOR = '2026-10-22'
export const AXIS = ['2026-10-20', '2026-10-21', '2026-10-22', '2026-10-23']
export const DATE = '2026-10-22'

export function thread(
  threadId: string,
  title: string,
  options: { summary?: string, related?: string[], scope?: string } = {},
): LaneDayThread {
  return {
    threadId,
    sectionId: '501',
    title,
    summary: options.summary ?? `摘要${threadId}`,
    confidence: 0.8,
    relatedArticleIds: options.related ?? [],
    fitDistance: null,
    scope: options.scope ?? null,
    createdAt: '',
  }
}

export function section(
  sectionId: string,
  label: string,
  threads: LaneDayThread[],
  persistentTopicId: string | null = '10',
): LaneDayReportSection {
  return {
    sectionId,
    clusterIndex: 0,
    clusterLabel: label,
    bucketTitle: null,
    topicStatusAtReport: 'active',
    persistentTopicId,
    threads,
  }
}

export function readyDay(date: string, sections: LaneDayReportSection[], reportId = '900'): LaneReportDay {
  return { date, status: 'ready', error: null, errorSource: null, reportId, sections }
}

export function errorDay(date: string, error = '详情读取失败'): LaneReportDay {
  return { date, status: 'error', error, errorSource: 'detail', reportId: null, sections: [] }
}

export function obsRecord(
  sectionId: string,
  date: string,
  threadCount: number | null,
  options: { topicId?: string | null, reportId?: string } = {},
): ObservationSectionRecord {
  return {
    sectionId,
    reportId: options.reportId ?? '900',
    date,
    clusterLabel: `历史分区${sectionId}`,
    threadCount,
    persistentTopicId: options.topicId === undefined ? '10' : options.topicId,
  }
}

export function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => { resolve = res })
  return { promise, resolve }
}

export function createObservation(timeline: ReturnType<typeof vi.fn>) {
  return useOtherTopicObservation({
    boardId: ref(1974),
    axis: () => AXIS,
    fetchers: { timeline: timeline as (boardId: number) => Promise<ObservationResponse<ObservationTimelineData>> },
  })
}

export function mountReading(options: {
  reportDay?: LaneReportDay
  selectedDate?: string | null
  selectedSectionId?: string | null
  topicId?: string | null
  timeline?: ReturnType<typeof vi.fn>
  /** 已预置数据的观察实例（M2「旧 dataset 强刷在途/挂载前已 ready」需要）。 */
  observation?: ReturnType<typeof createObservation>
  /** 挂到 document.body（M2 终态回调需要能看见当前 DOM）。 */
  attach?: boolean
  onSettled?: (payload: {
    laneKey: string
    date: string
    status: string
    sectionId: string
    topicId: string | null
  }) => void
} = {}) {
  const reading = useLaneDayReading({
    boardId: ref(1974),
    today: () => TODAY,
    anchorDate: () => ANCHOR,
  })
  const timeline = options.timeline ?? vi.fn(async () => ({
    success: true as const,
    data: { records: [] as ObservationSectionRecord[], coverage: 'all' as const },
  }))
  const observation = options.observation ?? createObservation(timeline)
  const wrapper = mount(OtherTopicReading, {
    ...(options.attach ? { attachTo: document.body } : {}),
    props: {
      selectedDate: options.selectedDate === undefined ? DATE : options.selectedDate,
      selectedSectionId: options.selectedSectionId === undefined ? '501' : options.selectedSectionId,
      reportDay: options.reportDay ?? readyDay(DATE, [section('501', '海峡局势', [thread('t1', '线索一'), thread('t2', '线索二')])]),
      reading,
      observation,
      topicId: options.topicId === undefined ? '10' : options.topicId,
    },
    ...(options.onSettled ? { attrs: { onReadingSettled: options.onSettled } } : {}),
  })
  return { wrapper, reading, observation, timeline }
}
