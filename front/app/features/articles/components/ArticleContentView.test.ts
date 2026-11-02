import { describe, expect, it } from 'vitest'
// Vite 的 ?raw 导入：测试环境不经过 Nuxt 管线，?raw 拿源码文本最直接
import source from './ArticleContentView.vue?raw'

/**
 * ArticleContentView — 全屏层 z-index 源码锚（2026-10-28 用户反馈修复）。
 *
 * 缺陷：`fullscreen-article` 层硬编码 `z-[100]`，而它在弹窗内（AppDialog 遮罩 9100）
 * 之下 → 全屏被遮罩盖住，点「全屏」看起来毫无反应。修法：层级由宿主经
 * `fullscreenZIndex` prop 传入（弹窗宿主传 9200）。
 *
 * 真组件挂载成本高（pinia stores + Nuxt runtime + 多个 API composable），
 * 故用源码锚守住「层级取自 prop、不得回迁硬编码」这条不变量
 * （同 change 源码守卫先例：防规则回迁）。
 */
const sourceText: string = source

describe('ArticleContentView — 全屏层 z-index 锚', () => {
  it('全屏层层级取自 fullscreenZIndex prop', () => {
    expect(sourceText).toMatch(/fullscreenZIndex\?: number/)
    expect(sourceText).toMatch(/class="fullscreen-article[^"]*"[\s\S]{0,200}zIndex: fullscreenZIndex/)
  })

  it('不得回迁硬编码 z-[100]（会再次被弹窗遮罩盖住）', () => {
    expect(sourceText).not.toContain('z-[100]')
  })

  it('prop 缺省值 100（阅读页原地全屏行为不变）', () => {
    expect(sourceText).toMatch(/fullscreenZIndex: 100/)
  })
})
