/**
 * 内部滚动条样式单一事实源的机械回归（change unify-inner-scrollbar-style）。
 *
 * 背景（三条事实）：
 * 1. `scrollbar-color` 是**继承**属性、`scrollbar-width` **不继承**（CSS Scrollbars L1）→ 类必须
 *    加在**每个**内部滚动容器上，不能指望父级带一带。
 * 2. Chromium ≥121 / Firefox 一旦在元素上声明这两个标准属性就接管滚动条绘制，并忽略该元素上的
 *    `::-webkit-scrollbar` 伪元素 → 标准属性与 webkit 伪元素两条通道必须**同写**、共用同一 token，
 *    否则 dark 主题会掉回系统浅色原生条。标准通道没有 hover 语法，`:hover` 只兜底老 Chromium。
 * 3. 组件里裸写 `scrollbar-width` / 硬编码滚动条色值会顶掉 `.scroll-thin` 的主题色 → 一律禁止。
 *
 * 纯源码文本断言（读 .vue / .css 文件），不挂载组件、不依赖渲染环境。
 */
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

/** 相对本测试文件（app/assets/css/）读取源码文本。 */
function readSource(relativePath: string): string {
  return readFileSync(new URL(relativePath, import.meta.url), 'utf8')
}

/** 取出 SFC 的模板段（根 <template> 到最后一个 </template>）。 */
function templateOf(source: string): string {
  const start = source.indexOf('<template>')
  const end = source.lastIndexOf('</template>')
  return start >= 0 && end > start ? source.slice(start, end) : ''
}

/**
 * 模板里所有**静态** `class="..."` 的 token 列表（逐元素一组）。
 * 前置否定断言排掉 `:class` / `v-bind:class` 动态绑定，只认写死在标签上的类。
 */
function staticClassTokens(template: string): string[][] {
  const groups: string[][] = []
  for (const match of template.matchAll(/(?<![:\w.-])class="([^"]*)"/g)) {
    groups.push((match[1] ?? '').split(/\s+/).filter(Boolean))
  }
  return groups
}

/** 取出 CSS 里某个选择器块的声明体；选择器不存在时返回空串。 */
function cssBlock(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`${escaped}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? ''
}

/** 已迁移容器：file 相对 app/；cls 是它自己的滚动容器类名。 */
const CONTAINERS = [
  { file: 'features/tags/components/console/BoardRadarView.vue', cls: 'rd-stage' },
  { file: 'features/tags/components/console/BoardRadarView.vue', cls: 'rd-rail-scroll' },
  { file: 'features/tags/components/console/BoardRadarReadingRail.vue', cls: 'rd-rail' },
  { file: 'features/settings/components/SettingsTabsNav.vue', cls: 'settings-tabs-nav__track' },
  { file: 'features/tags/components/console/BoardNarrowDayStrip.vue', cls: 'bns-strip-track' },
  { file: 'features/tags/components/daily-report/DailyReportSidebar.vue', cls: 'drm-sidebar' },
  { file: 'features/tags/components/daily-report/DailyReportDeskIndex.vue', cls: 'ddi' },
  { file: 'features/tags/components/DailyReportReader.vue', cls: 'drm-reader' },
  { file: 'features/tags/components/DailyReportReader.vue', cls: 'drm-notes-rail__scroller' },
  { file: 'features/shell/components/ArticleListPanelView.vue', cls: 'virtual-list' },
  { file: 'features/shell/components/AppSidebarView.vue', cls: 'sidebar-content' },
]

/** 迁移过的 .vue 文件去重列表（整文件不得再出现裸 scrollbar- 声明）。 */
const MIGRATED_FILES = [...new Set(CONTAINERS.map((entry) => entry.file))]

describe('内部滚动条样式 — 单一事实源 .scroll-thin（unify-inner-scrollbar-style）', () => {
  describe('main.css 的 .scroll-thin 工具类', () => {
    it('标准属性通道：声明 scrollbar-width: thin 与主题 token 化的 scrollbar-color', () => {
      const block = cssBlock(readSource('./main.css'), '.scroll-thin')

      expect(block, 'main.css 缺少 .scroll-thin 规则块').not.toBe('')
      expect(block, '.scroll-thin 未声明 scrollbar-width: thin').toContain('scrollbar-width: thin')
      expect(
        block,
        '.scroll-thin 的 scrollbar-color 未取主题 token --color-border-strong（dark 主题会掉回系统浅色原生条）',
      ).toContain('scrollbar-color: var(--color-border-strong) transparent')
    })

    it('webkit 伪元素兜底通道：滚动条尺寸、轨道、拇指与 hover 四段齐全', () => {
      const css = readSource('./main.css')

      const scrollbar = cssBlock(css, '.scroll-thin::-webkit-scrollbar')
      expect(scrollbar, '缺少 .scroll-thin::-webkit-scrollbar 兜底（老 Chromium 会退回原生条）').not.toBe('')
      expect(scrollbar, '.scroll-thin::-webkit-scrollbar 未限定 6px 细条宽度').toContain('width: 6px')
      expect(scrollbar, '.scroll-thin::-webkit-scrollbar 未限定 6px 细条高度').toContain('height: 6px')

      const track = cssBlock(css, '.scroll-thin::-webkit-scrollbar-track')
      expect(track, '缺少 .scroll-thin::-webkit-scrollbar-track 兜底').not.toBe('')
      expect(track, '.scroll-thin::-webkit-scrollbar-track 轨道应为透明').toContain('background: transparent')

      const thumb = cssBlock(css, '.scroll-thin::-webkit-scrollbar-thumb')
      expect(thumb, '缺少 .scroll-thin::-webkit-scrollbar-thumb 兜底').not.toBe('')
      expect(thumb, '.scroll-thin 拇指未取主题 token --color-border-strong').toContain(
        'background: var(--color-border-strong)',
      )
      expect(thumb, '.scroll-thin 拇指未设 3px 圆角').toContain('border-radius: 3px')

      const hover = cssBlock(css, '.scroll-thin::-webkit-scrollbar-thumb:hover')
      expect(hover, '缺少 .scroll-thin::-webkit-scrollbar-thumb:hover 兜底').not.toBe('')
      expect(hover, '.scroll-thin hover 态未取主题 token --color-text-muted').toContain(
        'background: var(--color-text-muted)',
      )
    })

    it('死代码 .content-scroll 已清除（零使用点，只留 .scroll-thin 一个内部滚动条事实源）', () => {
      const css = readSource('./main.css')

      expect(css, 'main.css 仍含 .content-scroll（零使用点的死代码，应删除）').not.toContain('content-scroll')
    })

    it('页面级 10px 杂志条未被波及（全局 ::-webkit-scrollbar 段保持原样）', () => {
      const css = readSource('./main.css')
      const beforeToolClass = css.slice(0, css.indexOf('.scroll-thin'))
      const globalBar = cssBlock(beforeToolClass, '::-webkit-scrollbar')

      expect(globalBar, 'main.css 缺少页面级 ::-webkit-scrollbar 段').toContain('width: 10px')
      expect(globalBar, 'main.css 缺少页面级 ::-webkit-scrollbar 段').toContain('height: 10px')
    })
  })

  describe('已迁移的滚动容器 — 类挂对了元素', () => {
    for (const { file, cls } of CONTAINERS) {
      it(`${file} 的 .${cls} 元素挂了 scroll-thin`, () => {
        const template = templateOf(readSource(`../../${file}`))
        const holders = staticClassTokens(template).filter((tokens) => tokens.includes(cls))

        expect(holders.length, `${file} 模板里找不到带 ${cls} 类的元素（元素被改名或删除？）`).toBeGreaterThan(0)
        expect(
          holders.some((tokens) => tokens.includes('scroll-thin')),
          `${file} 的 .${cls} 元素本身没有挂 scroll-thin（挂到相邻 wrapper 或子元素上都不算数）`,
        ).toBe(true)
      })
    }
  })

  describe('没有裸声明 / 硬编码色值残留', () => {
    for (const file of MIGRATED_FILES) {
      it(`${file} 整文件不含裸 scrollbar- 声明`, () => {
        const source = readSource(`../../${file}`)

        expect(
          source,
          `${file} 仍含裸 scrollbar- 声明（宽度 / 颜色统一由 .scroll-thin 提供，裸声明会顶掉主题色）`,
        ).not.toContain('scrollbar-')
      })
    }

    it('components/layout/AppSidebar.css 不含裸 scrollbar- 声明', () => {
      const source = readSource('../../components/layout/AppSidebar.css')

      expect(
        source,
        'AppSidebar.css 仍含裸 scrollbar- 声明（统一由 .scroll-thin 提供，硬编码色值不受主题控制）',
      ).not.toContain('scrollbar-')
    })
  })
})
