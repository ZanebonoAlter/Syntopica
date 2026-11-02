/**
 * useLaneDayReading 测试共用 harness（8C.4S 结构规范包从 useLaneDayReading.test.ts 抽出）。
 *
 * 只放**测试数据与装配**：固定板块/今天/锚点、日报索引/详情/文章 fixture、deferred、
 * 注入 fetchers 的 store harness 与 okList 包装。断言与故事留在各 `*.test.ts` 子套件：
 *  - `useLaneDayReading.test.ts`：lane 读取主路（RD-1/2/3、RD-4、RD-5、join 分层、R24 离开上下文）
 *  - `useLaneDayReading.reportIndex.test.ts`：停更 lookback、报日索引共享与强刷（RDC-1/RDC-6/R1/R3）
 *  - `useLaneDayReading.reportDay.test.ts`：报告优先读取（ensureReportDay、RDC-2/3/4b、RDC-5、R24 报告优先级）
 *  - `useLaneDayReading.cache.test.ts`：有界缓存 14/20、FIFO、失败不占位、切板块/卸载释放（CA-3..CA-8）
 *  - `useLaneDayReading.relatedArticles.test.ts`：相关文章跨板块晚到不发布（R2）
 *
 * 本文件不得以 `*.test.ts` 命名（会被 Vitest 当测试收集，且无用例可跑）；只用 `vi.fn()`
 * 做 fetcher 桩，不 mock 模块、不跨测试文件共享状态（每个子套件自建 harness 实例）。
 */
import { vi } from 'vitest'
import { ref, type Ref } from 'vue'
import type { WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'
import type {
  LaneDayArticleMeta,
  LaneDayReportDetail,
  LaneDayReportIndexItem,
} from '~/api/normalizers/laneDayReading'
import type { Article } from '~/types'
import { useLaneDayReading, type LaneDayReadingFetchers } from './useLaneDayReading'

export const BOARD = 1974
export const TODAY = '2026-10-28'
export const ANCHOR = '2026-10-24'

export function makeDay(date: string, sections: { sectionId: string, foldedCount?: number, eventTitles?: string[] }[] = [{ sectionId: '501' }]): WindowLaneDay {
  return {
    date,
    sections: sections.map(section => ({
      sectionId: section.sectionId,
      label: `桶${section.sectionId}`,
      threadCount: null,
      fallbackClueCount: null,
      foldedCount: section.foldedCount ?? 0,
      eventTitles: section.eventTitles ?? ['聚合标题'],
    })),
  }
}

export function indexItems(entries: [string, string][]): LaneDayReportIndexItem[] {
  return entries.map(([periodDate, reportId]) => ({ reportId, periodDate, title: `日报${reportId}`, createdAt: '' }))
}

export function reportDetail(reportId: string, periodDate: string, sections: {
  sectionId: string
  threads?: { threadId: string, title: string, summary?: string, relatedArticleIds?: string[] }[]
}[]): LaneDayReportDetail {
  return {
    reportId,
    boardId: String(BOARD),
    periodDate,
    sections: sections.map(section => ({
      sectionId: section.sectionId,
      clusterLabel: `桶${section.sectionId}`,
      bucketTitle: null,
      threads: (section.threads ?? []).map(thread => ({
        threadId: thread.threadId,
        sectionId: section.sectionId,
        title: thread.title,
        summary: thread.summary ?? '',
        confidence: 0.8,
        relatedArticleIds: thread.relatedArticleIds ?? [],
        fitDistance: null,
        createdAt: '',
      })),
    })),
  }
}

export function articleMeta(articleId: string, title = `文章${articleId}`): LaneDayArticleMeta {
  const article = {
    id: articleId, feedId: '3', title, description: '', content: '', excerpt: '', link: '',
    pubDate: '2026-10-22T09:00:00+08:00', category: '', read: false, favorite: false,
  } as unknown as Article
  return { articleId, title, pubDate: article.pubDate, read: false, article }
}

export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

export interface Harness {
  list: ReturnType<typeof vi.fn>
  detail: ReturnType<typeof vi.fn>
  article: ReturnType<typeof vi.fn>
  store: ReturnType<typeof useLaneDayReading>
  boardId: Ref<number>
  setAnchor: (anchor: string | null) => void
}

export function harness(options: { anchor?: string | null, boardId?: number } = {}): Harness {
  const list = vi.fn()
  const detail = vi.fn()
  const article = vi.fn()
  const boardId = ref(options.boardId ?? BOARD)
  let anchor: string | null = options.anchor === undefined ? ANCHOR : options.anchor
  const fetchers: LaneDayReadingFetchers = { list, detail, article }
  const store = useLaneDayReading({
    boardId,
    today: () => TODAY,
    anchorDate: () => anchor,
    fetchers,
  })
  return { list, detail, article, store, boardId, setAnchor: next => { anchor = next } }
}

export function okList(items: LaneDayReportIndexItem[]) {
  return { success: true, data: items }
}
