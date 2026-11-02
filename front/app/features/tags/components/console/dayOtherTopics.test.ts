/**
 * dayOtherTopics 纯模型测试（8C G1；test-cases-matrix-day-others OTH 组 + 状态机）。
 *
 *  - 成员身份：日报详情 sections 排除「正式/休眠泳道当日可读 sectionId」∪「persistentTopicId ∈ A 名单」；
 *    候选池/watch/辅助标签不得凑行（本模型根本不接这些输入）。
 *  - 行序：报告 cluster_index 升序（不按线索数/名称重排）；同名不同 id 各自保留。
 *  - 计数：该 section 详情 threads.length（0 合法）；非 ready 状态不冒 0。
 *  - 状态：只读 topic_status_at_report 历史快照（active/candidate/archived/null/未知大小写/无 topicId）。
 *  - 状态机：no-date / index-loading / index-error / loading / no-report / error / ready。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDayReportSection } from '~/api/normalizers/laneDayReading'
import type { WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'
import {
  buildOtherTopics,
  OTHER_TOPICS_EMPTY_TEXT,
  OTHER_TOPICS_UNAVAILABLE_NOTE,
  OTHER_TOPIC_STATUS_TEXT,
} from './dayOtherTopics'

const DATE = '2026-10-24'

function laneRow(topicId: string, overrides: Partial<ObservationLaneRow> = {}): ObservationLaneRow {
  return {
    topicId,
    label: `泳道${topicId}`,
    watchLinked: false,
    sectionTotal: 3,
    clueTotal: 5,
    dormant: false,
    snapshot: null,
    ...overrides,
  }
}

/** 详情 section 夹具：默认当次未归属、状态未知。 */
function section(
  sectionId: string,
  overrides: Partial<LaneDayReportSection> & { threadIds?: string[] } = {},
): LaneDayReportSection {
  const { threadIds, ...rest } = overrides
  return {
    sectionId,
    clusterIndex: Number(sectionId),
    clusterLabel: `分区${sectionId}`,
    bucketTitle: null,
    topicStatusAtReport: null,
    persistentTopicId: null,
    threads: (threadIds ?? []).map(threadId => ({
      threadId,
      sectionId,
      title: `线索${threadId}`,
      summary: '',
      confidence: 0,
      relatedArticleIds: [],
      fitDistance: null,
      createdAt: '',
    })),
    ...rest,
  }
}

function reportDay(
  sections: LaneDayReportSection[],
  overrides: Partial<LaneReportDay> = {},
): LaneReportDay {
  return {
    date: DATE,
    status: 'ready',
    error: null,
    errorSource: null,
    reportId: '1211',
    sections,
    ...overrides,
  }
}

/** 单日解析夹具：`${topicId}|${date}` → 分区 id 列表 / gap / unavailable。 */
function resolverFor(table: Record<string, string[] | 'gap' | 'unavailable'>): LaneDayResolver {
  return (topicId, date) => {
    const entry = table[`${topicId}|${date}`]
    if (entry === undefined || entry === 'gap') return { kind: 'gap' }
    if (entry === 'unavailable') return { kind: 'unavailable' }
    const day: WindowLaneDay = {
      date,
      sections: entry.map(sectionId => ({
        sectionId,
        label: `桶${sectionId}`,
        threadCount: null,
        fallbackClueCount: null,
        foldedCount: 0,
        eventTitles: [],
      })),
    }
    return { kind: 'content', source: 'tier', day }
  }
}

function build(
  sections: LaneDayReportSection[],
  options: {
    lanes?: ObservationLaneRow[]
    table?: Record<string, string[] | 'gap' | 'unavailable'>
    day?: Partial<LaneReportDay> | null
    selectedDate?: string | null
    indexLoading?: boolean
    indexError?: string | null
    windowReportDayCount?: number | null
  } = {},
) {
  return buildOtherTopics({
    selectedDate: options.selectedDate === undefined ? DATE : options.selectedDate,
    reportDay: options.day === null ? null : reportDay(sections, options.day ?? {}),
    allLanes: options.lanes ?? [],
    resolveLaneDay: resolverFor(options.table ?? {}),
    indexLoading: options.indexLoading ?? false,
    indexError: options.indexError ?? null,
    windowReportDayCount: options.windowReportDayCount ?? null,
  })
}

describe('OTH — 成员身份排除', () => {
  it('OTH-1 排除正式泳道当日可读 sectionId', () => {
    const view = build(
      [section('501', { threadIds: ['t1'] }), section('777', { threadIds: ['t2'] })],
      { lanes: [laneRow('10')], table: { [`10|${DATE}`]: ['501'] } },
    )
    expect(view.state).toBe('ready')
    expect(view.rows.map(row => row.sectionId)).toEqual(['777'])
  })

  it('OTH-1b 休眠泳道同样按 sectionId 排除（A 名单两组都算）', () => {
    const view = build(
      [section('501', { threadIds: ['t1'] }), section('777', { threadIds: ['t2'] })],
      { lanes: [laneRow('10', { dormant: true })], table: { [`10|${DATE}`]: ['501'] } },
    )
    expect(view.rows.map(row => row.sectionId)).toEqual(['777'])
  })

  it('OTH-2 persistentTopicId ∈ A 名单：泳道当日 gap 也排除（防重复归类）', () => {
    const view = build(
      [section('501', { persistentTopicId: '10', threadIds: ['t1'] }), section('777', { threadIds: ['t2'] })],
      { lanes: [laneRow('10')], table: { [`10|${DATE}`]: 'gap' } },
    )
    expect(view.rows.map(row => row.sectionId)).toEqual(['777'])
  })

  it('OTH-3 候选池/watch/辅助标签不得凑行：candidates 无对应分区 → 0 行合法', () => {
    // 模型输入里没有 candidates/watch —— 只要日报没有分区就是 0 行
    const view = build([], { lanes: [laneRow('10')] })
    expect(view.state).toBe('ready')
    expect(view.rows).toEqual([])
    expect(view.count).toBe(0)
    expect(view.statusText).toBe(OTHER_TOPICS_EMPTY_TEXT)
  })

  it('OTH-10 泳道当日未取到：有 A 身份的分区仍排除、无法证明归属的照列 + 未取到备注', () => {
    const view = build(
      [section('501', { persistentTopicId: '10', threadIds: ['t1'] }), section('777', { threadIds: ['t2'] })],
      { lanes: [laneRow('10')], table: { [`10|${DATE}`]: 'unavailable' } },
    )
    // 不把该泳道分区补进组（persistentTopicId 身份仍生效）；也不把组判失败
    expect(view.state).toBe('ready')
    expect(view.rows.map(row => row.sectionId)).toEqual(['777'])
    expect(view.note).toBe(OTHER_TOPICS_UNAVAILABLE_NOTE)
  })
})

describe('OTH — 行序与计数', () => {
  it('OTH-4 行序 = cluster_index 升序（乱序输入不改写、不按计数/名称重排）', () => {
    const view = build(
      [
        section('900', { clusterIndex: 3, clusterLabel: '甲', threadIds: ['a', 'b', 'c', 'd'] }),
        section('901', { clusterIndex: 0, clusterLabel: '乙', threadIds: [] }),
        section('902', { clusterIndex: 2, clusterLabel: '丙', threadIds: ['e'] }),
      ],
    )
    expect(view.rows.map(row => row.sectionId)).toEqual(['901', '902', '900'])
  })

  it('OTH-4b 乱序输入不被原地改写（纯函数）', () => {
    const sections = [section('1', { clusterIndex: 5 }), section('2', { clusterIndex: 1 })]
    build(sections)
    expect(sections.map(item => item.sectionId)).toEqual(['1', '2'])
  })

  it('OTH-5 行计数 = 详情 threads.length（0 合法；不取 lane 聚合 thread_count）', () => {
    const view = build(
      [section('501', { threadIds: ['t1', 't2', 't3'] }), section('777')],
      { lanes: [laneRow('10')], table: { [`10|${DATE}`]: ['501'] } },
    )
    const rows = new Map(view.rows.map(row => [row.sectionId, row]))
    // 501 已被泳道覆盖 → 只在 777 上验证 0 合法；另建一例验证 3 条
    expect(rows.get('777')?.threadCount).toBe(0)
    const full = build([section('501', { threadIds: ['t1', 't2', 't3'] })])
    expect(full.rows[0]!.threadCount).toBe(3)
    expect(full.count).toBe(1)
  })

  it('OTH-8 全部分区被覆盖：ready + 0 行 + 「都已可在泳道读到」', () => {
    const view = build([section('501'), section('502')], {
      lanes: [laneRow('10')],
      table: { [`10|${DATE}`]: ['501', '502'] },
    })
    expect(view.state).toBe('ready')
    expect(view.rows).toEqual([])
    expect(view.count).toBe(0)
    expect(view.statusText).toBe(OTHER_TOPICS_EMPTY_TEXT)
    expect(view.retryable).toBe(false)
  })

  it('OTH-11 同日同名不同 id：两行各自保留，不合并', () => {
    const view = build([
      section('501', { clusterLabel: '海峡局势', threadIds: ['t1'] }),
      section('502', { clusterLabel: '海峡局势', threadIds: ['t2', 't3'] }),
    ])
    expect(view.rows.map(row => [row.sectionId, row.label, row.threadCount])).toEqual([
      ['501', '海峡局势', 1],
      ['502', '海峡局势', 2],
    ])
  })
})

describe('OTH — 状态只取生成时快照', () => {
  it('OTH-6 active/candidate/archived → 当时已转正/当时待观察/当时已归档', () => {
    const view = build([
      section('1', { persistentTopicId: '10', topicStatusAtReport: 'active' }),
      section('2', { persistentTopicId: '11', topicStatusAtReport: 'candidate' }),
      section('3', { persistentTopicId: '12', topicStatusAtReport: 'archived' }),
    ])
    expect(view.rows.map(row => [row.statusKind, row.statusText])).toEqual([
      ['active', OTHER_TOPIC_STATUS_TEXT.active],
      ['candidate', OTHER_TOPIC_STATUS_TEXT.candidate],
      ['archived', OTHER_TOPIC_STATUS_TEXT.archived],
    ])
  })

  it('OTH-6b null + 有 topicId → 当时状态未记录；无 topicId → 当次未归属', () => {
    const view = build([
      section('1', { persistentTopicId: '10', topicStatusAtReport: null }),
      section('2', { persistentTopicId: null, topicStatusAtReport: 'candidate' }),
    ])
    expect(view.rows.map(row => [row.statusKind, row.statusText])).toEqual([
      ['unknown', OTHER_TOPIC_STATUS_TEXT.unknown],
      ['unattributed', OTHER_TOPIC_STATUS_TEXT.unattributed],
    ])
  })

  it('OTH-7 大小写/未知枚举不猜：ACTIVE / paused 都降级为当时状态未记录', () => {
    const view = build([
      section('1', { persistentTopicId: '10', topicStatusAtReport: 'ACTIVE' }),
      section('2', { persistentTopicId: '11', topicStatusAtReport: 'paused' }),
    ])
    expect(view.rows.map(row => row.statusText)).toEqual([
      OTHER_TOPIC_STATUS_TEXT.unknown,
      OTHER_TOPIC_STATUS_TEXT.unknown,
    ])
  })
})

describe('OTH — 旧数据兼容（缺历史字段）', () => {
  it('缺 clusterIndex/topicStatusAtReport/persistentTopicId（undefined）：按未知/未归属处理，行序回退输入序', () => {
    const legacy = [
      { sectionId: '1', clusterLabel: '甲', bucketTitle: null, threads: [] },
      { sectionId: '2', clusterLabel: '乙', bucketTitle: null, threads: [] },
    ] as LaneDayReportSection[]
    const view = build(legacy)
    expect(view.rows.map(row => row.sectionId)).toEqual(['1', '2'])
    expect(view.rows.map(row => row.clusterIndex)).toEqual([null, null])
    expect(view.rows.map(row => [row.statusKind, row.statusText])).toEqual([
      ['unattributed', OTHER_TOPIC_STATUS_TEXT.unattributed],
      ['unattributed', OTHER_TOPIC_STATUS_TEXT.unattributed],
    ])
  })
})

describe('OTH — 组状态机（不冒 0、不互相冒充）', () => {
  it('未选日：no-date（有报日可选 / 窗内无报日 两种文案）', () => {
    const waiting = build([], { selectedDate: null, windowReportDayCount: 8 })
    expect(waiting.state).toBe('no-date')
    expect(waiting.count).toBeNull()
    expect(waiting.statusText).not.toBeNull()

    const noReports = build([], { selectedDate: null, windowReportDayCount: 0 })
    expect(noReports.state).toBe('no-date')
    expect(noReports.statusText).not.toBe(waiting.statusText)
  })

  it('索引加载中 → index-loading；加载失败 → index-error 可重试（不冒无日报）', () => {
    const loadingView = build([], { selectedDate: null, indexLoading: true })
    expect(loadingView.state).toBe('index-loading')
    expect(loadingView.count).toBeNull()

    const failed = build([], { selectedDate: null, indexError: '列表挂了' })
    expect(failed.state).toBe('index-error')
    expect(failed.retryable).toBe(true)
    expect(failed.count).toBeNull()
    expect(failed.note).toContain('列表挂了')
  })

  it('已选日但报告读取未开始/在途 → loading（不给计数）', () => {
    const view = build([], { day: { status: 'loading', reportId: null } })
    expect(view.state).toBe('loading')
    expect(view.rows).toEqual([])
    expect(view.count).toBeNull()
  })

  it('OTH-9 无报日 → no-report（不是 0 条其他话题，不给 0）', () => {
    const view = build([], { day: { status: 'no-report', reportId: null } })
    expect(view.state).toBe('no-report')
    expect(view.count).toBeNull()
    expect(view.statusText).not.toBe(OTHER_TOPICS_EMPTY_TEXT)
  })

  it('OTH-9b 详情失败 → error 可重试；索引来源失败 → index-error', () => {
    const detailFailed = build([], { day: { status: 'error', error: '详情挂了', errorSource: 'detail' } })
    expect(detailFailed.state).toBe('error')
    expect(detailFailed.retryable).toBe(true)
    expect(detailFailed.count).toBeNull()
    expect(detailFailed.note).toContain('详情挂了')

    const indexFailed = build([], { day: { status: 'error', error: '列表挂了', errorSource: 'index' } })
    expect(indexFailed.state).toBe('index-error')
    expect(indexFailed.retryable).toBe(true)
  })

  it('RDC-4b display gate：reportDay.date 与所选日不一致时按未读到处理（不挂旧日 sections）', () => {
    const stale = reportDay([section('777', { threadIds: ['t1'] })], { date: '2026-10-20' })
    const view = buildOtherTopics({
      selectedDate: DATE,
      reportDay: stale,
      allLanes: [],
      resolveLaneDay: resolverFor({}),
      indexLoading: false,
      indexError: null,
      windowReportDayCount: 8,
    })
    expect(view.state).toBe('loading')
    expect(view.rows).toEqual([])
    expect(view.count).toBeNull()
  })
})
