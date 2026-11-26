/**
 * 名称「看全」浮层：矩阵标签列与左栏版块名共用（2026-11-26 用户反馈泳道名展示不全）。
 *
 * 为什么不用原生 `title`：悬停约 1s 才出、键盘聚焦不出、长名浏览器照样截断；而且矩阵滚动
 * 容器 `overflow-x: auto` 会裁掉绝对定位浮层 → 卡片 Teleport 到 body，`strategy: 'fixed'`
 * 跟随触发元素。
 *
 * 契约：
 *  - **截断闸门**：只对真的被省略号切掉的文本弹卡（`scrollWidth > clientWidth + 1`），
 *    名字放得下时零噪声；
 *  - **键鼠同权**：`mouseenter` 与 `focusin` 都走 `show`（聚焦看全，弱视放大字号场景有效）；
 *  - 卡片是**视觉冗余**（调用方挂 `aria-hidden`）——名称按钮的 `aria-label` 已含全名，
 *    不重复播报；层级由调用方取 `Z_INDEX` 档位表绑定，本文件不写层级数字。
 */
import { onBeforeUnmount, onMounted, ref, type ComponentPublicInstance, type CSSProperties, type Ref } from 'vue'
import { autoUpdate, flip, offset, shift, useFloating } from '@floating-ui/vue'
import type { Placement } from '@floating-ui/dom'

export interface NameHoverCardOptions {
  placement?: Placement
  /** 悬停到出现的最短停留（ms）：避免扫过整列时连续闪卡。 */
  delay?: number
}

export interface NameHoverCardHandle {
  visible: Ref<boolean>
  /** 模板 `:ref` 绑定用（卡片元素；模板里直接绑 ref 对象会被自动解包，故给函数式 setter）。 */
  setFloating: (el: Element | ComponentPublicInstance | null) => void
  /** floating-ui 位移样式（含 fixed 定位坐标）。 */
  floatingStyles: Ref<CSSProperties>
  /**
   * 打开卡片：`reference` = 触发元素（用于定位与层级），`clip` = 承载名称文本的元素
   * （用于判定是否真被截断；缺省取 `reference` 自身）。
   */
  show: (reference: HTMLElement | null, clip?: HTMLElement | null) => void
  hide: () => void
}

export function useNameHoverCard(options: NameHoverCardOptions = {}): NameHoverCardHandle {
  const referenceRef = ref<HTMLElement>()
  const floatingRef = ref<HTMLElement>()
  const visible = ref(false)
  let timer: ReturnType<typeof setTimeout> | null = null

  const { floatingStyles } = useFloating(referenceRef, floatingRef, {
    placement: options.placement ?? 'bottom-start',
    strategy: 'fixed',
    middleware: [offset(6), flip({ padding: 8 }), shift({ padding: 8 })],
    whileElementsMounted: autoUpdate,
  })

  function cancel(): void {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }

  function hide(): void {
    cancel()
    visible.value = false
  }

  function show(reference: HTMLElement | null, clip?: HTMLElement | null): void {
    if (!reference) return
    const text = clip ?? reference
    // 放得下就不打扰：只有被省略号切掉的名字才出卡。
    if (text.scrollWidth <= text.clientWidth + 1) {
      hide()
      return
    }
    referenceRef.value = reference
    cancel()
    timer = setTimeout(() => {
      timer = null
      visible.value = true
    }, options.delay ?? 200)
  }

  /** 容器/整页滚动时立即收起：卡片 fixed 定位，滚出锚点后会悬在半空。 */
  function onScroll(): void {
    if (visible.value) hide()
  }

  onMounted(() => window.addEventListener('scroll', onScroll, true))
  onBeforeUnmount(() => {
    cancel()
    window.removeEventListener('scroll', onScroll, true)
  })

  function setFloating(el: Element | ComponentPublicInstance | null): void {
    floatingRef.value = el instanceof HTMLElement ? el : undefined
  }

  return { visible, setFloating, floatingStyles, show, hide }
}
