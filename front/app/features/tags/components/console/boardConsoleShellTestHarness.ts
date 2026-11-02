/**
 * BoardConsoleShell 测试装配（8C G4b 从巨型壳测试按 §8 抽出）：模块 mock、路由/media 桩、
 * 挂载与全局 hooks。测试断言与故事留在各分组子套件：
 *  - `BoardConsoleShell.test.ts`：骨架与最新日报独立锚
 *  - `BoardConsoleShellSelection.test.ts`：唯一选中日/就地下读、档 vs 自由观察范围
 *  - `BoardConsoleShellNavigation.test.ts`：入口/管理/真实路由（含返回上下文与滚动）
 *  - `BoardConsoleShellStates.test.ts`：版块切换/归档/失败态
 *  - `BoardConsoleShell.integration.test.ts`：G4b 真实组件链（报日列头/其他组/阅读槽/重试）
 *
 * `vi.mock` 按 Vitest 约定提升到本模块顶部，故测试文件只要先 import 本 harness 再 import
 * 组件，mock 一定先注册；fixture 由本文件转出，子套件无需二次引用。
 */
import { afterEach, beforeEach, vi } from 'vitest'
import { reactive } from 'vue'
import { mount } from '@vue/test-utils'
const routeState = reactive<{ query: Record<string, unknown>, params: Record<string, string>, path: string }>({
  query: {},
  params: { boardId: '1974' },
  path: '/tags/1974',
})
function applyQuery(to: { query: Record<string, unknown> }): Promise<void> {
  routeState.query = { ...to.query }
  return Promise.resolve()
}

const router = {
  push: vi.fn(() => Promise.resolve()),
  replace: vi.fn(applyQuery),
}

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => router,
}))

const getBoards = vi.fn()
const getBoardHealthCheckup = vi.fn()
const addComposition = vi.fn()
const getComposition = vi.fn()
const removeFromComposition = vi.fn()
const getBoardSourceBreakdown = vi.fn()
const listWatches = vi.fn()
const getLaneDynamics = vi.fn()
const getBoardArticles = vi.fn()
const getBoardSectionTimeline = vi.fn()
const getBoardDailyReports = vi.fn()
const getDailyReportDetail = vi.fn()
const getOverview = vi.fn()
const openArticlePreviewMock = vi.hoisted(() => vi.fn())
const mediaControl = vi.hoisted(() => ({ set: (_value: boolean) => {} }))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return {
    ...original,
    useSemanticBoardsApi: () => ({
      getBoards,
      getBoardHealthCheckup,
      addComposition,
      getComposition,
      removeFromComposition,
      getBoardSourceBreakdown,
      getBoardArticles,
    }),
    useBoardRadarApi: () => ({ getOverview }),
  }
})
vi.mock('~/api/topicWatches', () => ({
  useTopicWatchesApi: () => ({ listWatches }),
}))
vi.mock('~/api/laneDynamics', () => ({
  useLaneDynamicsApi: () => ({ getLaneDynamics }),
}))
vi.mock('~/api/dailyReports', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/dailyReports')>()
  return {
    ...original,
    useDailyReportsApi: () => ({ getBoardDailyReports, getDailyReportDetail, getBoardSectionTimeline }),
  }
})
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }),
}))
vi.mock('~/composables/useMediaQuery', async () => {
  const { ref } = await import('vue')
  const narrow = ref(false)
  mediaControl.set = (value: boolean) => { narrow.value = value }
  return { useIsNarrowViewport: () => narrow }
})
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))
vi.mock('~/components/ui/ThemeToggle.vue', () => ({
  default: { name: 'ThemeToggle', template: '<button data-testid="theme-toggle-stub" />' },
}))
vi.mock('../ArticlePreviewModal.vue', () => ({
  default: {
    name: 'ArticlePreviewModal',
    props: ['visible'],
    emits: ['close'],
    template: '<div data-testid="article-preview-stub" :data-visible="String(visible)" />',
  },
}))
vi.mock('~/components/dialog/TopicManageDialog.vue', () => ({
  default: { name: 'TopicManageDialog', props: ['modelValue', 'boardId'], template: '<div data-testid="topic-manage-stub" :data-open="String(modelValue)" :data-board="boardId" />' },
}))
vi.mock('../topic-watch/WatchManagePanel.vue', () => ({
  default: {
    name: 'WatchManagePanel',
    props: ['modelValue', 'boardId'],
    template: '<div data-testid="watch-panel-stub" :data-open="String(modelValue)" :data-board="boardId" />',
  },
}))
vi.mock('./BoardWideWorkface.vue', () => ({
  default: {
    name: 'BoardWideWorkface',
    props: ['open', 'tab', 'boardId', 'removedAuxIds'],
    emits: ['update:open', 'update:tab', 'locate-lane', 'open-watch-manage', 'open-topics', 'remove-composition'],
    template: '<div data-testid="workface-stub" :data-open="String(open)" :data-tab="tab" :data-board="boardId" :data-removed-count="(removedAuxIds || []).length">'
      + '<button data-testid="workface-remove" @click="$emit(\'remove-composition\', \'77\')" /></div>',
  },
}))
vi.mock('./BoardComposeOverlay.vue', () => ({
  default: {
    name: 'BoardComposeOverlay',
    props: ['boardId', 'boardLabel'],
    emits: ['close', 'saved'],
    template: '<div data-testid="compose-stub" />',
  },
}))
vi.mock('~/features/tags/composables/useTagsPage', async () => {
  const { ref } = await import('vue')
  return {
    useTagsPage: () => ({
      showArticlePreview: ref(false),
      selectedPreviewArticle: ref(null),
      previewArticles: ref([]),
      loadingPreviewArticle: ref(false),
      openArticlePreview: openArticlePreviewMock,
      closeArticlePreview: vi.fn(),
      handleArticleFavorite: vi.fn(),
      handleArticleUpdate: vi.fn(),
    }),
  }
})

import BoardConsoleShell from './BoardConsoleShell.vue'
import { BOARD_LABEL, overview, reportList, reportDetail, response } from './boardConsoleShellFixtures'

const mounted: Array<{ unmount: () => void }> = []

function mountShell(boardId = 1974) {
  const wrapper = mount(BoardConsoleShell, {
    props: { boardId },
    attachTo: document.body,
    global: {
      stubs: {
        NuxtLink: {
          name: 'NuxtLink',
          props: ['to'],
          template: '<a :href="to" @click.prevent="$emit(\'click\')"><slot /></a>',
        },
        DailyReportGenerateDialog: {
          name: 'DailyReportGenerateDialog',
          props: ['visible', 'boards'],
          emits: ['cancel', 'completed'],
          template: '<div data-testid="generate-stub" :data-visible="String(visible)" />',
        },
        AppDialog: { name: 'AppDialog', props: ['modelValue'], template: '<div><slot /></div>' },
      },
    },
  })
  mounted.push(wrapper)
  return wrapper
}

/** 每个子套件在模块顶层调用一次：注册通用 afterEach（卸载窗口/清 body/media 复位）与 beforeEach（重置 mock）。 */
export function installShellTestHooks(): void {
  afterEach(() => {
    for (const wrapper of mounted.splice(0)) wrapper.unmount()
    document.body.innerHTML = ''
    mediaControl.set(false)
  })

  beforeEach(() => {
    routeState.query = {}
    routeState.params = { boardId: '1974' }
    routeState.path = '/tags/1974'
    router.push.mockClear()
    router.replace.mockClear()
    router.replace.mockImplementation(applyQuery)
    window.sessionStorage.clear()
    openArticlePreviewMock.mockClear()
    getBoards.mockReset().mockResolvedValue({
      success: true,
      data: {
        items: [
          { id: 1974, label: BOARD_LABEL, tag_count: 128 },
          { id: 1975, label: 'AI 与机器学习', tag_count: 96 },
        ],
        total: 2,
      },
  })
  getLaneDynamics.mockReset().mockResolvedValue({ success: true, data: response() })
  getOverview.mockReset().mockResolvedValue(overview())
  getBoardDailyReports.mockReset().mockResolvedValue(reportList())
  getDailyReportDetail.mockReset().mockResolvedValue(reportDetail())
  getBoardHealthCheckup.mockReset().mockResolvedValue({
    success: true,
    data: { latest: null, trend: [] },
  })
  addComposition.mockReset()
  getComposition.mockReset().mockResolvedValue({
    success: true,
    data: { items: [], composites: [], total: 0 },
  })
  removeFromComposition.mockReset()
  getBoardSourceBreakdown.mockReset().mockResolvedValue({
    success: true,
    data: { total_articles: 0, source_count: 0, sources: [] },
  })
  listWatches.mockReset().mockResolvedValue({ success: true, data: [] })
  getBoardArticles.mockReset().mockResolvedValue({
    success: true,
    data: [],
    pagination: { pages: 1 },
  })
  getBoardSectionTimeline.mockReset().mockResolvedValue({
    success: true,
    data: { sections: [], relations: [] },
  })
  })
}


export {
  routeState,
  router,
  applyQuery,
  mediaControl,
  openArticlePreviewMock,
  mounted,
  mountShell,
  getBoards,
  getBoardHealthCheckup,
  addComposition,
  getComposition,
  removeFromComposition,
  getBoardSourceBreakdown,
  listWatches,
  getLaneDynamics,
  getBoardArticles,
  getBoardSectionTimeline,
  getBoardDailyReports,
  getDailyReportDetail,
  getOverview,
}

export * from './boardConsoleShellFixtures'
