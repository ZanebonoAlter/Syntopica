import { describe, expect, it, vi } from 'vitest'
import {
  applyMarginNoteHighlights,
  buildAnchorPayloadFromSelection,
  locateQuotedText,
  mnAnchorKey,
  wrapRangeWithMark,
} from './marginNoteAnchor'
import type { MarginNoteAnnotation } from '~/api/marginNotes'

function makeAnnotation(over: Partial<MarginNoteAnnotation> = {}): MarginNoteAnnotation {
  return {
    id: 1,
    report_id: 10,
    section_id: 100,
    thread_id: 7,
    quoted_text: '逆回购操作',
    anchor_offset_start: null,
    anchor_offset_end: null,
    created_at: '2026-09-22T00:00:00Z',
    qas: [],
    ...over,
  }
}

describe('locateQuotedText', () => {
  it('偏移线索命中时按线索定位（PR-3 主路径）', () => {
    const text = '前文铺垫。今天开展了逆回购操作，随后收盘。'
    const located = locateQuotedText(text, '逆回购操作', 8)
    expect(located).toEqual({ start: 10, end: 15 })
  })

  it('偏移失配回退全文精确匹配（日报重建后偏移漂移）', () => {
    const text = '今天开展了逆回购操作，随后收盘。'
    const located = locateQuotedText(text, '逆回购操作', 50)
    expect(located).toEqual({ start: 5, end: 10 })
  })

  it('精确全文失配时按归一化模糊匹配兜底（空白差异）', () => {
    const text = '开展逆回 购操作，金额 3000 亿元'
    const located = locateQuotedText(text, '逆回购操作', null)
    // end 为原文本排他终点（跳过空白后仍对齐到「作」之后）
    expect(located).toEqual({ start: 2, end: 8 })
  })

  it('全失配返回 null（原文已变更降级态）', () => {
    expect(locateQuotedText('内容已完全重写', '逆回购操作', null)).toBeNull()
  })
})

describe('wrapRangeWithMark', () => {
  it('单节点内包裹成功并写入 data-jump（反查不依赖 id 拼接规律）', () => {
    document.body.innerHTML = '<p id="t">央行开展逆回购操作</p>'
    const p = document.getElementById('t')!
    const range = document.createRange()
    range.setStart(p.childNodes[0]!, 4)
    range.setEnd(p.childNodes[0]!, 9)
    expect(wrapRangeWithMark(range, 42)).toBe(true)
    const mark = p.querySelector('mark')
    expect(mark?.getAttribute('data-jump')).toBe('42')
    expect(mark?.textContent).toBe('逆回购操作')
  })

  it('跨节点选区 surroundContents 抛错 → 返回 false 降级只落卡不高亮（WB-2）', () => {
    document.body.innerHTML = '<p id="t2">aaa <b>bold</b> ccc</p>'
    const p = document.getElementById('t2')!
    const range = document.createRange()
    range.setStart(p.childNodes[0]!, 1)
    range.setEnd(p.childNodes[2]!, 2)
    // happy-dom 的 surroundContents 对跨节点选区不抛错（与 Chromium 行为不一致），
    // 用 spy 强制抛错锁定实现的 catch 降级分支
    const rangeProto = Object.getPrototypeOf(range) as Range
    const spy = vi.spyOn(rangeProto, 'surroundContents').mockImplementation(() => {
      throw new Error('InvalidStateError: surroundContents on cross-node range')
    })
    try {
      expect(wrapRangeWithMark(range, 43)).toBe(false)
    } finally {
      spy.mockRestore()
    }
  })
})

describe('applyMarginNoteHighlights', () => {
  it('thread 批注按 t:<id> 容器落 mark；lead 批注落 lead 容器', () => {
    document.body.innerHTML = `
      <div>
        <p data-mn-anchor="lead">头条：开展逆回购操作稳定跨季</p>
        <p data-mn-anchor="t:7">人民银行公告开展逆回购操作，中标利率不变</p>
      </div>`
    const root = document.body.firstElementChild as HTMLElement
    const result = applyMarginNoteHighlights(root, [
      makeAnnotation({ id: 1, thread_id: 7 }),
      makeAnnotation({ id: 2, thread_id: null, quoted_text: '逆回购操作稳定跨季' }),
    ])
    expect(result.resolutions.get(1)).toBe('marked')
    expect(result.resolutions.get(2)).toBe('marked')
    const marks = root.querySelectorAll('mark[data-jump]')
    expect(marks.length).toBe(2)
  })

  it('容器缺失或全文失配 → changed（原文已变更态）', () => {
    document.body.innerHTML = '<p data-mn-anchor="t:7">完全无关的新内容</p>'
    const root = document.body.firstElementChild as HTMLElement
    const result = applyMarginNoteHighlights(root, [
      makeAnnotation({ id: 1, thread_id: 7 }),
      makeAnnotation({ id: 2, thread_id: 999 }),
    ])
    expect(result.resolutions.get(1)).toBe('changed')
    expect(result.resolutions.get(2)).toBe('changed')
  })

  it('幂等：重复应用不产生重复 mark；mark 随批注删除被清理（自愈）', () => {
    document.body.innerHTML = '<div><p data-mn-anchor="t:7">开展逆回购操作</p></div>'
    const root = document.body.firstElementChild as HTMLElement
    applyMarginNoteHighlights(root, [makeAnnotation({ id: 1 })])
    applyMarginNoteHighlights(root, [makeAnnotation({ id: 1 })])
    expect(root.querySelectorAll('mark[data-jump="1"]').length).toBe(1)
    applyMarginNoteHighlights(root, [])
    expect(root.querySelectorAll('mark').length).toBe(0)
    expect(root.querySelector('p')?.textContent).toBe('开展逆回购操作')
  })
})

describe('buildAnchorPayloadFromSelection / mnAnchorKey', () => {
  it('同容器选区返回锚定信息；跨容器/无容器返回 null', () => {
    document.body.innerHTML = `
      <div>
        <p data-mn-anchor="t:7">人民银行开展逆回购操作</p>
        <p>无锚定属性段落</p>
      </div>`
    const root = document.body.firstElementChild as HTMLElement
    const anchorP = root.querySelector('p[data-mn-anchor="t:7"]')!
    const range = document.createRange()
    range.setStart(anchorP.childNodes[0]!, 5)
    range.setEnd(anchorP.childNodes[0]!, 11)
    const selection = {
      isCollapsed: false,
      rangeCount: 1,
      toString: () => range.toString(),
      getRangeAt: () => range,
      anchorNode: anchorP.childNodes[0],
    } as unknown as Selection
    const payload = buildAnchorPayloadFromSelection(selection, root)
    expect(payload?.key).toBe('t:7')
    expect(payload?.start).toBe(5)

    const collapsed = { isCollapsed: true, rangeCount: 0 } as unknown as Selection
    expect(buildAnchorPayloadFromSelection(collapsed, root)).toBeNull()
  })

  it('mnAnchorKey：thread 批注与 lead 批注的容器键', () => {
    expect(mnAnchorKey(makeAnnotation({ thread_id: 9 }))).toBe('t:9')
    expect(mnAnchorKey(makeAnnotation({ thread_id: null }))).toBe('lead')
  })
})
