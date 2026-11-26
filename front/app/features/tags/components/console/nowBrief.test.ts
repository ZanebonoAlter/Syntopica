/**
 * 右轨「当日日报」读模型单测（console-rails-live-signals tasks 1.5）。
 *
 * 锁住 design D5 的两层口径：
 *  - 点评层 = `report.highlights` 原序（**不重复**日报 title/summary）；article_count
 *    缺省 = 不显示「N 篇来源」（不补 0）；
 *  - 泳道层 = active 分区 ∩ lane-dynamics 正式泳道；句子取 thread 标题否则分区概述，
 *    两者皆无不占位；线索数取 lane-dynamics 当日 clue_count（取不到 = null）；
 *    按线索数降序、并列保持日报原序、null 排末位；candidate 分区排除。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDayReportHighlight, LaneDayReportSection } from '~/api/normalizers/laneDayReading'
import type { WindowLane } from '~/api/normalizers/laneDynamicsWindow'
import { buildNowBriefModel, laneTopicColor, mergeLaneDynamicsLanes, type NowBriefReport } from './nowBrief'

const DATE = '2026-11-18'

function thread(title: string) {
  return {
    threadId: `t-${title}`,
    sectionId: 's1',
    title,
    summary: '',
    confidence: 0.9,
    relatedArticleIds: [],
    fitDistance: null,
    scope: null,
    createdAt: '',
  }
}

function section(overrides: Partial<LaneDayReportSection> = {}): LaneDayReportSection {
  return {
    sectionId: 's1',
    clusterIndex: null,
    clusterLabel: '分区概述',
    bucketTitle: null,
    topicStatusAtReport: 'active',
    persistentTopicId: '1204',
    threads: [thread('胡塞武装指责沙特空袭')],
    ...overrides,
  }
}

function highlight(overrides: Partial<LaneDayReportHighlight> = {}): LaneDayReportHighlight {
  return {
    title: '伊朗袭击巴林美军基地',
    reason: '多家媒体报道了袭击细节。',
    clusterId: null,
    articleCount: null,
    clusterMode: null,
    dataAsOf: null,
    ...overrides,
  }
}

function lane(topicId: string, label: string, clueCount: number | null = 0, date = DATE): WindowLane {
  return {
    topicId,
    label,
    watchLinked: false,
    sectionCountWindow: 1,
    threadCountWindow: null,
    laneHeat: 0,
    snapshot: null,
    timeline: [{
      date,
      sections: clueCount === null ? [] : [{
        sectionId: 'x1',
        label: '桶',
        threadCount: clueCount,
        compositeCount: 0,
        clueCount,
        foldedCount: 0,
        eventTitles: [],
      }],
      clueCount,
    }],
  }
}

function report(overrides: Partial<NowBriefReport> = {}): NowBriefReport {
  return {
    date: DATE,
    status: 'ready',
    error: null,
    sections: [],
    highlights: [],
    ...overrides,
  }
}

describe('nowBrief — 点评层（highlights 原样，不重复日报标题）', () => {
  it('点评 = highlights 原序（2–3 条）；模型里没有日报标题/摘要字段（不重复展示）', () => {
    const model = buildNowBriefModel(report({
      highlights: [
        highlight({ title: '第一条', reason: '理由一' }),
        highlight({ title: '第二条', reason: '理由二', articleCount: 3, clusterId: 42 }),
      ],
    }), DATE, [])

    expect(model.status).toBe('ready')
    expect(model.hasReport).toBe(true)
    expect(model.reviews).toHaveLength(2)
    expect(model.reviews[0]!.title).toBe('第一条')
    expect(model.reviews[1]!.articleCount).toBe(3)
    expect(model.reviews[1]!.clusterId).toBe(42)
    expect(Object.keys(model)).not.toContain('title')
    expect(Object.keys(model)).not.toContain('summary')
  })

  it('article_count 缺省 = null（展示层不显示「N 篇来源」，不补 0）', () => {
    const model = buildNowBriefModel(report({ highlights: [highlight({ articleCount: null })] }), DATE, [])
    expect(model.reviews[0]!.articleCount).toBeNull()
  })

  it('真实 0 篇是合法值（照实透传，不当缺省）', () => {
    const model = buildNowBriefModel(report({ highlights: [highlight({ articleCount: 0 })] }), DATE, [])
    expect(model.reviews[0]!.articleCount).toBe(0)
  })

  it('标题与正文皆空的条目不占位', () => {
    const model = buildNowBriefModel(report({ highlights: [highlight({ title: '', reason: '' })] }), DATE, [])
    expect(model.reviews).toHaveLength(0)
  })
})

describe('nowBrief — 泳道层（active 分区 ∩ 正式泳道）', () => {
  it('句子取该分区首条 thread 的 title；无 thread 时退回分区概述', () => {
    const withThread = buildNowBriefModel(
      report({ sections: [section({ threads: [thread('首条线索'), thread('第二条')] })] }),
      DATE,
      [lane('1204', '沙特与也门胡塞武装冲突')],
    )
    expect(withThread.lanes[0]!.sentence).toBe('首条线索')

    const withLabel = buildNowBriefModel(
      report({ sections: [section({ threads: [] })] }),
      DATE,
      [lane('1204', '沙特与也门胡塞武装冲突')],
    )
    expect(withLabel.lanes[0]!.sentence).toBe('分区概述')
  })

  it('thread 标题与分区概述皆无 → 该行不出', () => {
    const model = buildNowBriefModel(
      report({ sections: [section({ threads: [], clusterLabel: '' })] }),
      DATE,
      [lane('1204', '某泳道')],
    )
    expect(model.lanes).toHaveLength(0)
  })

  it('candidate 分区（其他动态）不进右轨', () => {
    const model = buildNowBriefModel(
      report({ sections: [section({ topicStatusAtReport: 'candidate' })] }),
      DATE,
      [lane('1204', '某泳道')],
    )
    expect(model.lanes).toHaveLength(0)
  })

  it('无 persistent_topic 的分区不进右栏', () => {
    const model = buildNowBriefModel(
      report({ sections: [section({ persistentTopicId: null })] }),
      DATE,
      [lane('1204', '某泳道')],
    )
    expect(model.lanes).toHaveLength(0)
  })

  it('lane-dynamics 里按 topic_id 找不到的分区不进右栏（保证每个「N 条」在矩阵里有对应格）', () => {
    const model = buildNowBriefModel(
      report({ sections: [section({ persistentTopicId: '9999' })] }),
      DATE,
      [lane('1204', '某泳道')],
    )
    expect(model.lanes).toHaveLength(0)
  })

  it('线索数取 lane-dynamics 当日 clue_count（与矩阵日格同源同值）', () => {
    const model = buildNowBriefModel(
      report({ sections: [section()] }),
      DATE,
      [lane('1204', '某泳道', 4)],
    )
    expect(model.lanes[0]!.clueCount).toBe(4)
  })

  it('该泳道当日取不到（timeline 里没有这一天）→ null（展示「—」，不补 0）', () => {
    const model = buildNowBriefModel(
      report({ sections: [section()] }),
      DATE,
      [lane('1204', '某泳道', 4, '2026-11-01')],
    )
    expect(model.lanes[0]!.clueCount).toBeNull()
  })
})

describe('nowBrief — 泳道层排序', () => {
  it('按线索数降序（1/4/2/0 → 4/2/1/0）', () => {
    const sections = [
      section({ sectionId: 'a', persistentTopicId: '10' }),
      section({ sectionId: 'b', persistentTopicId: '11' }),
      section({ sectionId: 'c', persistentTopicId: '12' }),
      section({ sectionId: 'd', persistentTopicId: '13' }),
    ]
    const model = buildNowBriefModel(report({ sections }), DATE, [
      lane('10', '一', 1),
      lane('11', '二', 4),
      lane('12', '三', 2),
      lane('13', '四', 0),
    ])
    expect(model.lanes.map(item => item.label)).toEqual(['二', '三', '一', '四'])
  })

  it('并列时保持日报原序；线索数未知的排末位', () => {
    const sections = [
      section({ sectionId: 'a', persistentTopicId: '10' }),
      section({ sectionId: 'b', persistentTopicId: '11' }),
      section({ sectionId: 'c', persistentTopicId: '12' }),
    ]
    const model = buildNowBriefModel(report({ sections }), DATE, [
      lane('10', '甲', 2),
      lane('11', '乙', 2),
      lane('12', '丙', null),
    ])
    expect(model.lanes.map(item => item.label)).toEqual(['甲', '乙', '丙'])
  })
})

describe('nowBrief — 状态分层', () => {
  it('无日报 = 诚实空态（不回落到别日）', () => {
    const model = buildNowBriefModel(report({ status: 'no-report' }), DATE, [])
    expect(model.status).toBe('no-report')
    expect(model.hasReport).toBe(false)
    expect(model.reviews).toHaveLength(0)
  })

  it('详情失败 = 局部降级（错误信息透传，其余层不受影响）', () => {
    const model = buildNowBriefModel(report({ status: 'error', error: '日报详情加载失败' }), DATE, [])
    expect(model.status).toBe('error')
    expect(model.error).toBe('日报详情加载失败')
  })

  it('详情未到 = loading', () => {
    expect(buildNowBriefModel(null, DATE, []).status).toBe('loading')
  })
})

describe('nowBrief — A 档 + 365 池合并', () => {
  it('池只补 A 同 ID 的更早日期；A 没有的 topic 不并入', () => {
    const tier = [{ ...lane('10', '甲', 3), timeline: [{ date: DATE, sections: [], clueCount: 3 }] }]
    const pool = [{
      ...lane('10', '甲', 1, '2026-09-01'),
      timeline: [{ date: '2026-09-01', sections: [], clueCount: 1 }],
    }, {
      ...lane('11', '乙', 1, '2026-09-01'),
      timeline: [{ date: '2026-09-01', sections: [], clueCount: 1 }],
    }]
    const merged = mergeLaneDynamicsLanes(tier, pool)
    expect(merged).toHaveLength(1)
    expect(merged[0]!.timeline.map(day => day.date)).toEqual([DATE, '2026-09-01'])
  })

  // review R-4：日期闸门必须在纯函数里可测——详情日期 ≠ 矩阵选中日时按「未到」处理，
  // MUST NOT 把上一日日报挂到今日标题下（spec「跟随矩阵选中日」「选中日无日报」的守门代码）。
  it('详情日期与矩阵选中日不一致 → 按未到处理，不显示邻日内容', () => {
    const model = buildNowBriefModel(
      report({ date: '2026-11-18' }),
      '2026-11-19',
      [],
    )
    expect(model.date).toBe('2026-11-19')
    expect(model.status).toBe('loading')
    expect(model.hasReport).toBe(false)
    expect(model.reviews).toHaveLength(0)
    expect(model.lanes).toHaveLength(0)
  })

  // review R-1：泳道色点优先用后端下发的稳定色（同一话题全站同色）；只有旧数据缺该字段才回退哈希。
  it('泳道色点优先用后端 persistent_topic.color，缺省才回退本地哈希', () => {
    const withColor = buildNowBriefModel(
      report({
        sections: [
          section({ persistentTopicId: '1204', persistentTopicColor: '#123456' }),
        ],
      }),
      DATE,
      [lane('1204', '美伊博弈升级')],
    )
    expect(withColor.lanes[0]!.color).toBe('#123456')

    const withoutColor = buildNowBriefModel(
      report({ sections: [section({ persistentTopicId: '1204' })] }),
      DATE,
      [lane('1204', '美伊博弈升级')],
    )
    expect(withoutColor.lanes[0]!.color).toBe(laneTopicColor('1204'))
  })
})
