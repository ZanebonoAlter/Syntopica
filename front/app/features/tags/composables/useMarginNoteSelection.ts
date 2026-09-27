import { onUnmounted, ref, watch, type Ref } from 'vue'
import { buildAnchorPayloadFromSelection, MN_MARK_CLASS, wrapRangeWithMark } from '../components/daily-report/marginNoteAnchor'

/**
 * 划词批注选区交互（daily-report-margin-notes design D2 / specs 红线冲突消解）：
 *
 * 1. selection guard：划选产生有效选区（trim 后 ≥2 字符）后，同一点击序列的 click 在
 *    document capture 阶段被吞（stopPropagation + preventDefault），thread/topic toggle
 *    不触发；选区清除后恢复原点击语义（RG-1/RG-2/RG-3）。
 * 2. mark 点击分层：正文批注 mark 的 click 在 reportRoot capture 委托层拦截
 *    （stopPropagation 跳对应卡，不触发 thread 展开，RG-4）。
 * 3. 气泡：mouseup 有效选区 → 「问一问」气泡浮于选区上方；<2 字符 / 空选区 /
 *    跨容器选区不弹（WB-1）。气泡 mousedown preventDefault 保选区不被清除。
 *
 * 行为基准：已批准原型 reader.html（同构三段式：guard / mark 委托 / mouseup 气泡）。
 */

export interface MarginNoteAnchorDraft {
  key: string
  quotedText: string
  start: number
  end: number
}

export interface MarginNoteBubbleState {
  visible: boolean
  x: number
  y: number
}

export function useMarginNoteSelection(options: {
  /** 阅读层开启态：开启时安装 document 监听，关闭时解除（互不污染全局）。 */
  active: Ref<boolean>
  /** 正文根容器（含 data-mn-anchor 宿主），气泡坐标与 mark 委托的参照系。 */
  reportRoot: Ref<HTMLElement | null>
  /** mark 点击 → 跳对应卡（窄屏由宿主转开抽屉定位）。 */
  onMarkClick: (annotationId: number) => void
}) {
  const { active, reportRoot, onMarkClick } = options

  const bubble = ref<MarginNoteBubbleState>({ visible: false, x: 0, y: 0 })
  /** 待落锚草稿：气泡确认时消费；选区变化即失效。 */
  let draft: MarginNoteAnchorDraft | null = null
  let draftRange: Range | null = null

  const MIN_SELECTION_RUNES = 2

  function selectionTrimmedLength(): number {
    const selection = window.getSelection()
    if (!selection || selection.isCollapsed) return 0
    return selection.toString().trim().length
  }

  function hideBubble() {
    bubble.value = { visible: false, x: bubble.value.x, y: bubble.value.y }
    draft = null
    draftRange = null
  }

  function handleDocumentMouseup(event: MouseEvent) {
    const root = reportRoot.value
    if (!root) return
    // 气泡自身/其内部的 mouseup 不参与选区判定（确认走 click）
    if (event.target instanceof Node && bubbleHostContains(event.target)) return

    const selection = window.getSelection()
    if (!selection || selection.isCollapsed || selectionTrimmedLength() < MIN_SELECTION_RUNES) {
      hideBubble()
      return
    }
    const payload = buildAnchorPayloadFromSelection(selection, root)
    if (!payload) {
      hideBubble()
      return
    }
    draft = { key: payload.key, quotedText: payload.quotedText, start: payload.start, end: payload.end }
    draftRange = payload.range

    const rect = payload.range.getBoundingClientRect()
    const hostRect = root.getBoundingClientRect()
    const x = rect.left - hostRect.left + rect.width / 2
    const y = rect.top - hostRect.top
    bubble.value = {
      visible: true,
      x: Math.min(Math.max(x, 56), Math.max(hostRect.width - 56, 56)),
      y,
    }
  }

  /**
   * guard：有效选区存续期间的 click 一律吞掉（capture，先于一切 toggle 处理器）。
   * 气泡自身豁免（批准原型同款）：气泡出现的前提就是选区存续，且 mousedown.prevent
   * 保选区，若不豁免则「问一问」的 click 永远到不了气泡处理器——落锚链路整体失效。
   */
  function handleDocumentClickCapture(event: MouseEvent) {
    if (event.target instanceof Node && bubbleHostContains(event.target)) return
    if (selectionTrimmedLength() >= MIN_SELECTION_RUNES) {
      event.stopPropagation()
      event.preventDefault()
    }
  }

  /** mark 委托：点高亮跳卡并阻断 toggle（capture 于 reportRoot）。 */
  function handleReportClickCapture(event: MouseEvent) {
    const target = event.target
    if (!(target instanceof Element)) return
    const mark = target.closest(`mark.${MN_MARK_CLASS}`)
    if (!mark) return
    const jump = mark.getAttribute('data-jump')
    if (!jump) return
    event.stopPropagation()
    event.preventDefault()
    onMarkClick(Number(jump))
  }

  function bubbleHostContains(node: Node): boolean {
    // 气泡组件由宿主渲染（portal 到 reportRoot 内），此处以 class 探测避免循环依赖
    let current: Node | null = node
    while (current) {
      if (current instanceof Element && current.classList.contains('mn-ask-bubble')) return true
      current = current.parentNode
    }
    return false
  }

  /** 气泡确认：消费草稿（返回 live range 供宿主包 mark 与落锚），清选区并隐藏气泡。 */
  function confirmBubble(): { draft: MarginNoteAnchorDraft; range: Range | null } | null {
    if (!draft) return null
    const payload = { draft, range: draftRange }
    hideBubble()
    window.getSelection()?.removeAllRanges()
    return payload
  }

  function attach() {
    document.addEventListener('mouseup', handleDocumentMouseup)
    document.addEventListener('click', handleDocumentClickCapture, true)
    reportRoot.value?.addEventListener('click', handleReportClickCapture, true)
  }

  function detach() {
    document.removeEventListener('mouseup', handleDocumentMouseup)
    document.removeEventListener('click', handleDocumentClickCapture, true)
    reportRoot.value?.removeEventListener('click', handleReportClickCapture, true)
  }

  watch(active, (isActive) => {
    if (isActive) attach()
    else {
      detach()
      hideBubble()
    }
  }, { flush: 'sync' })

  // reportRoot 挂载晚于 active 翻转（reader 渲染时序）：每次根节点变化都重挂 mark 委托
  watch(reportRoot, () => {
    if (!active.value) return
    reportRoot.value?.removeEventListener('click', handleReportClickCapture, true)
    reportRoot.value?.addEventListener('click', handleReportClickCapture, true)
  }, { flush: 'sync' })

  onUnmounted(detach)

  return {
    bubble,
    confirmBubble,
    hideBubble,
    /** 供宿主在「确认落锚」时把草稿 range 包成 mark（跨节点抛错降级只落卡）。 */
    buildMark: wrapRangeWithMark,
  }
}
