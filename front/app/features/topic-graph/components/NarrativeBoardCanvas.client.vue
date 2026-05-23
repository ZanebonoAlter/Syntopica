<script setup lang="ts">
import { ref, watch, onBeforeUnmount, shallowRef } from 'vue'
import type { BoardTimelineDay, BoardItem, BoardNarrativeItem } from '~/api/topicGraph'

interface Props {
  days: BoardTimelineDay[]
  selectedId: number | null
  expandedBoardIds?: Set<number>
}

const props = defineProps<Props>()

const emit = defineEmits<{
  select: [id: number]
  hover: [id: number | null]
  'board-toggle': [ids: Set<number>]
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const p5Instance = shallowRef<any>(null)
const expandedBoardIds = ref(new Set<number>(props.expandedBoardIds ?? []))

watch(() => props.expandedBoardIds, (ids) => {
  if (ids) expandedBoardIds.value = new Set(ids)
}, { deep: true })

onBeforeUnmount(() => {
  if (p5Instance.value) {
    p5Instance.value.remove()
    p5Instance.value = null
  }
})

type PaletteEntry = { dot: number[]; line: number[]; bg: number[]; badge: number[]; glow: number[] }

const PALETTE: Record<string, PaletteEntry> = {
  emerging:   { dot: [52, 211, 153],  line: [52, 211, 153],  bg: [20, 50, 40],  badge: [52, 211, 153, 40],  glow: [52, 211, 153, 30] },
  continuing: { dot: [96, 165, 250],  line: [96, 165, 250],  bg: [20, 35, 55],  badge: [96, 165, 250, 40],  glow: [96, 165, 250, 30] },
  splitting:  { dot: [251, 146, 60],  line: [251, 146, 60],  bg: [50, 35, 18],  badge: [251, 146, 60, 40], glow: [251, 146, 60, 30] },
  merging:    { dot: [192, 132, 252], line: [192, 132, 252], bg: [40, 28, 55],  badge: [192, 132, 252, 40], glow: [192, 132, 252, 30] },
  ending:     { dot: [107, 114, 128], line: [107, 114, 128], bg: [30, 30, 35],  badge: [107, 114, 128, 40], glow: [107, 114, 128, 30] },
}

const STATUS_LABELS: Record<string, string> = {
  emerging: '新兴', continuing: '持续', splitting: '分化', merging: '融合', ending: '终结',
}

function pal(status: string): PaletteEntry {
  return PALETTE[status] ?? PALETTE.ending!
}

function formatDate(dateStr: string) {
  const d = new Date(dateStr + 'T00:00:00')
  const m = d.getMonth() + 1
  const day = d.getDate()
  const weekdays = ['日', '一', '二', '三', '四', '五', '六']
  return `${m}/${day} ${weekdays[d.getDay()]}`
}

function computeAggregateStatus(narratives: BoardNarrativeItem[]): string {
  if (narratives.length === 0) return 'continuing'
  if (narratives.some(n => n.status === 'emerging')) return 'emerging'
  if (narratives.every(n => n.status === 'ending')) return 'ending'
  return 'continuing'
}

async function initP5() {
  if (!containerRef.value) return
  if (p5Instance.value) {
    p5Instance.value.remove()
    p5Instance.value = null
  }

  const p5Module = await import('p5')
  const P5 = p5Module.default

  const sketch = (p: any) => {
    const BOARD_W = 220
    const BOARD_HEADER_H = 44
    const BOARD_PAD = 8
    const COL_W = 260
    const HEADER_H = 44
    const BOARD_GAP = 12
    const PAD_LEFT = 20
    const PAD_TOP = 16
    const ACCENT_W = 5

    const SUB_W = 190
    const SUB_H = 48
    const SUB_GAP = 6

    interface BoardLayout {
      board: BoardItem
      col: number
      x: number
      y: number
      w: number
      h: number
      expanded: boolean
      subNodes: SubNodeLayout[]
    }

    interface SubNodeLayout {
      narrative: BoardNarrativeItem
      boardId: number
      x: number
      y: number
      w: number
      h: number
    }

    interface LayoutEdge {
      from: number
      to: number
      fx: number; fy: number
      tx: number; ty: number
      status: string
      childStatus: string
      isContinuation: boolean
      isBoardEdge: boolean
    }

    let boards = new Map<number, BoardLayout>()
    let narrativeNodes = new Map<number, SubNodeLayout>()
    let boardEdges: LayoutEdge[] = []
    let narrativeEdges: LayoutEdge[] = []
    let canvasW = 800
    let canvasH = 400
    let hoveredId: number | null = null
    let highlightedIds: Set<number> | null = null
    let highlightedEdgeSet = new Set<number>()
    let dashOffset = 0
    let canvasEl: HTMLCanvasElement

    function computeLayout() {
      const days = (props.days ?? []).filter((d: BoardTimelineDay) => (d?.boards ?? []).length > 0)
      if (!days.length) {
        boards = new Map()
        narrativeNodes = new Map()
        boardEdges = []
        narrativeEdges = []
        canvasW = 800
        canvasH = 400
        return
      }

      const bm = new Map<number, BoardLayout>()
      const nm = new Map<number, SubNodeLayout>()
      let maxCanvasH = 0

      days.forEach((day: BoardTimelineDay, col: number) => {
        const colX = PAD_LEFT + col * COL_W
        let curY = PAD_TOP + HEADER_H

        ;(day.boards ?? []).forEach((board: BoardItem) => {
          const isExpanded = expandedBoardIds.value.has(board.id)
          const bx = colX + (COL_W - BOARD_W) / 2
          const by = curY
          const subNodes: SubNodeLayout[] = []

          let bh = BOARD_HEADER_H
          if (isExpanded && (board.narratives ?? []).length > 0) {
            bh += BOARD_PAD
            ;(board.narratives ?? []).forEach((narr, idx) => {
              const sx = bx + (BOARD_W - SUB_W) / 2
              const sy = by + bh + idx * (SUB_H + SUB_GAP)
              const sn: SubNodeLayout = {
                narrative: narr,
                boardId: board.id,
                x: sx, y: sy,
                w: SUB_W, h: SUB_H,
              }
              subNodes.push(sn)
              nm.set(narr.id, sn)
            })
            bh += (board.narratives ?? []).length * (SUB_H + SUB_GAP) + BOARD_PAD
          }

          const tagCount = (board.event_tags ?? []).length + (board.abstract_tags ?? []).length
          if (isExpanded && tagCount > 0) {
            bh += 6 + 18 + 4
          }

          bm.set(board.id, {
            board,
            col,
            x: bx, y: by,
            w: BOARD_W, h: bh,
            expanded: isExpanded,
            subNodes,
          })

          curY += bh + BOARD_GAP
        })

        maxCanvasH = Math.max(maxCanvasH, curY)
      })

      const allNarratives: BoardNarrativeItem[] = []
      for (const day of days) {
        for (const board of (day.boards ?? [])) {
          allNarratives.push(...(board.narratives ?? []))
        }
      }

      const ne: LayoutEdge[] = []
      for (const n of allNarratives) {
        const child = nm.get(n.id)
        if (!child) continue
        for (const pid of n.parent_ids) {
          const parent = nm.get(pid)
          if (!parent) continue
          ne.push({
            from: pid, to: n.id,
            fx: parent.x + parent.w / 2, fy: parent.y,
            tx: child.x + child.w / 2, ty: child.y + child.h,
            status: parent.narrative.status,
            childStatus: child.narrative.status,
            isContinuation: child.narrative.status === 'continuing',
            isBoardEdge: false,
          })
        }
      }

      const be: LayoutEdge[] = []
      const boardList = Array.from(bm.values())
      for (const bl of boardList) {
        const childBoard = bl.board
        for (const prevId of (childBoard.prev_board_ids ?? [])) {
          const parentLayout = bm.get(prevId)
          if (!parentLayout) continue
          be.push({
            from: prevId, to: childBoard.id,
            fx: parentLayout.x + parentLayout.w / 2,
            fy: parentLayout.y + parentLayout.h,
            tx: bl.x + bl.w / 2,
            ty: bl.y,
            status: pal(parentLayout.board.aggregate_status ?? computeAggregateStatus(parentLayout.board.narratives ?? [])).line ? parentLayout.board.aggregate_status ?? computeAggregateStatus(parentLayout.board.narratives ?? []) : 'continuing',
            childStatus: childBoard.aggregate_status ?? computeAggregateStatus(childBoard.narratives ?? []),
            isContinuation: false,
            isBoardEdge: true,
          })
        }
      }

      boards = bm
      narrativeNodes = nm
      narrativeEdges = ne
      boardEdges = be
      canvasW = PAD_LEFT + days.length * COL_W + PAD_LEFT
      canvasH = Math.max(maxCanvasH + PAD_TOP, 400)
    }

    function hitTest(mx: number, my: number): { type: 'narrative'; id: number } | { type: 'board'; id: number } | { type: 'boardTags'; id: number } | null {
      for (const [, sn] of narrativeNodes) {
        if (mx >= sn.x && mx <= sn.x + sn.w &&
            my >= sn.y && my <= sn.y + sn.h) {
          return { type: 'narrative', id: sn.narrative.id }
        }
      }
      for (const [id, bl] of boards) {
        if (mx >= bl.x && mx <= bl.x + bl.w &&
            my >= bl.y && my <= bl.y + BOARD_HEADER_H) {
          return { type: 'board', id }
        }
        const tagCount = (bl.board.event_tags ?? []).length + (bl.board.abstract_tags ?? []).length
        if (bl.expanded && tagCount > 0) {
          const badgeW = 76
          const badgeH = 18
          const badgeX = bl.x + (bl.w - badgeW) / 2
          const badgeY = bl.y + bl.h - badgeH - 4
          if (mx >= badgeX && mx <= badgeX + badgeW &&
              my >= badgeY && my <= badgeY + badgeH) {
            return { type: 'boardTags', id }
          }
        }
      }
      return null
    }

    function computeHighlight(nodeId: number | null) {
      if (nodeId === null) {
        highlightedIds = null
        highlightedEdgeSet = new Set()
        return
      }
      const connected = new Set<number>([nodeId])
      const allN: BoardNarrativeItem[] = []
      for (const day of (props.days ?? [])) {
        for (const board of (day.boards ?? [])) {
          allN.push(...(board.narratives ?? []))
        }
      }
      const nMap = new Map(allN.map((n) => [n.id, n]))

      const upQ = [nodeId]
      const upV = new Set<number>()
      while (upQ.length) {
        const cur = upQ.shift()!
        if (upV.has(cur)) continue
        upV.add(cur); connected.add(cur)
        const n = nMap.get(cur)
        if (n) for (const pid of n.parent_ids) if (!upV.has(pid)) upQ.push(pid)
      }
      const downQ = [nodeId]
      const downV = new Set<number>()
      while (downQ.length) {
        const cur = downQ.shift()!
        if (downV.has(cur)) continue
        downV.add(cur); connected.add(cur)
        const n = nMap.get(cur)
        if (n) for (const cid of n.child_ids) if (!downV.has(cid)) downQ.push(cid)
      }

      highlightedIds = connected
      highlightedEdgeSet = new Set<number>()
      narrativeEdges.forEach((e, i) => {
        if (connected.has(e.from) && connected.has(e.to)) highlightedEdgeSet.add(i)
      })
    }

    function drawBezier(x1: number, y1: number, x2: number, y2: number) {
      const midX = (x1 + x2) / 2
      p.noFill()
      p.beginShape()
      for (let t = 0; t <= 1; t += 0.02) {
        p.vertex(p.bezierPoint(x1, midX, midX, x2, t), p.bezierPoint(y1, y1, y2, y2, t))
      }
      p.endShape()
    }

    function drawBezierPartial(x1: number, y1: number, x2: number, y2: number, fromT: number, toT: number) {
      const midX = (x1 + x2) / 2
      p.noFill()
      p.beginShape()
      for (let t = fromT; t <= toT; t += 0.02) {
        p.vertex(p.bezierPoint(x1, midX, midX, x2, t), p.bezierPoint(y1, y1, y2, y2, t))
      }
      p.endShape()
    }

    function drawDashedBezier(x1: number, y1: number, x2: number, y2: number, dashLen: number, gapLen: number, offset: number) {
      const midX = (x1 + x2) / 2
      const pts: Array<{ x: number; y: number }> = []
      for (let t = 0; t <= 1; t += 0.01) {
        pts.push({
          x: p.bezierPoint(x1, midX, midX, x2, t),
          y: p.bezierPoint(y1, y1, y2, y2, t),
        })
      }
      let dist = offset % (dashLen + gapLen)
      if (dist < 0) dist += dashLen + gapLen
      let drawing = true
      p.noFill()
      p.beginShape()
      for (let i = 1; i < pts.length; i++) {
        const dx = pts[i]!.x - pts[i - 1]!.x
        const dy = pts[i]!.y - pts[i - 1]!.y
        dist += Math.sqrt(dx * dx + dy * dy)
        if (drawing) p.vertex(pts[i]!.x, pts[i]!.y)
        const period = drawing ? dashLen : gapLen
        if (dist >= period) {
          dist = 0
          drawing = !drawing
          if (drawing) { p.endShape(); p.beginShape(); p.vertex(pts[i]!.x, pts[i]!.y) }
          else { p.endShape() }
        }
      }
      p.endShape()
    }

    function wrapText(text: string, maxW: number, maxLines: number): string[] {
      const lines: string[] = []
      let current = ''
      for (const ch of text) {
        const test = current + ch
        if (p.textWidth(test) > maxW && current.length > 0) {
          lines.push(current)
          current = ch
          if (lines.length >= maxLines) {
            lines[lines.length - 1] += '…'
            return lines
          }
        } else {
          current = test
        }
      }
      if (current) lines.push(current)
      return lines
    }

    p.setup = () => {
      computeLayout()
      canvasEl = p.createCanvas(canvasW, canvasH).parent(containerRef.value!).elt as HTMLCanvasElement
      canvasEl.style.cursor = 'default'
      p.textFont('system-ui, -apple-system, sans-serif')
      p.frameRate(30)
    }

    p.draw = () => {
      p.background(10, 15, 22)
      dashOffset -= 0.4

      p.noStroke()
      p.fill(186, 206, 226, 10)
      for (let gx = 0; gx < canvasW; gx += 20) {
        for (let gy = 0; gy < canvasH; gy += 20) {
          p.circle(gx, gy, 1.5)
        }
      }

      const visDays = (props.days ?? []).filter((d: BoardTimelineDay) => (d?.boards ?? []).length > 0)
      for (let di = 0; di < visDays.length; di++) {
        const day = visDays[di]!
        const colX = PAD_LEFT + di * COL_W

        if (di > 0) {
          p.stroke(255, 255, 255, 8)
          p.strokeWeight(0.5)
          p.line(colX, PAD_TOP, colX, canvasH - PAD_TOP)
          p.noStroke()
        }

        p.fill(186, 206, 226, 80)
        p.textSize(12)
        p.textAlign(p.CENTER, p.CENTER)
        p.text(formatDate(day.date), colX + COL_W / 2, PAD_TOP + HEADER_H / 2)

        p.stroke(255, 255, 255, 12)
        p.strokeWeight(0.5)
        p.line(colX + 10, PAD_TOP + HEADER_H, colX + COL_W - 10, PAD_TOP + HEADER_H)
        p.noStroke()
      }

      /* board-level edges */
      for (const e of boardEdges) {
        const pc = pal(e.status)
        p.strokeWeight(3)
        p.stroke(pc.line[0]!, pc.line[1]!, pc.line[2]!, 25)
        drawBezier(e.fx, e.fy, e.tx, e.ty)
      }

      /* narrative-level edges */
      for (let i = 0; i < narrativeEdges.length; i++) {
        const e = narrativeEdges[i]!
        const isHl = highlightedEdgeSet.has(i)
        const hasHighlight = highlightedIds !== null
        const pc = pal(e.status)
        const cc = pal(e.childStatus)

        if (hasHighlight && !isHl) {
          p.stroke(255, 255, 255, 12)
          p.strokeWeight(1)
          drawBezier(e.fx, e.fy, e.tx, e.ty)
          continue
        }

        if (e.isContinuation) {
          p.strokeWeight(1.5)
          p.stroke(pc.line[0]!, pc.line[1]!, pc.line[2]!, isHl ? 120 : 50)
          drawDashedBezier(e.fx, e.fy, e.tx, e.ty, 8, 6, dashOffset)
          if (isHl) {
            p.strokeWeight(4)
            p.stroke(pc.line[0]!, pc.line[1]!, pc.line[2]!, 20)
            drawBezier(e.fx, e.fy, e.tx, e.ty)
          }
        } else {
          const alpha = isHl ? 160 : 60
          p.strokeWeight(isHl ? 2.5 : 1.8)
          p.stroke(pc.line[0]!, pc.line[1]!, pc.line[2]!, alpha)
          drawBezierPartial(e.fx, e.fy, e.tx, e.ty, 0, 0.5)
          p.stroke(cc.line[0]!, cc.line[1]!, cc.line[2]!, alpha)
          drawBezierPartial(e.fx, e.fy, e.tx, e.ty, 0.5, 1)
          if (isHl) {
            p.strokeWeight(6)
            p.stroke(pc.line[0]!, pc.line[1]!, pc.line[2]!, 15)
            drawBezier(e.fx, e.fy, e.tx, e.ty)
          }
        }
      }

      /* board nodes */
      for (const [boardId, bl] of boards) {
        const status = bl.board.aggregate_status ?? computeAggregateStatus(bl.board.narratives ?? [])
        const st = pal(status)
        const isSelected = props.selectedId !== null && (bl.board.narratives ?? []).some(n => n.id === props.selectedId)
        const isHovered = hoveredId !== null && (bl.board.narratives ?? []).some(n => n.id === hoveredId)

        const isHotspotBoard = bl.board.is_system === true
        const isConceptBoard = !isHotspotBoard && bl.board.board_concept_id !== undefined && bl.board.board_concept_id !== null && bl.board.board_concept_id > 0
        const typeAccent = isHotspotBoard ? [251, 191, 36] : isConceptBoard ? [96, 165, 250] : [107, 114, 128]
        const typeBg = isHotspotBoard ? [32, 26, 16] : isConceptBoard ? [18, 26, 38] : [16, 22, 32]

        p.push()

        if (isSelected || isHovered) {
          p.noStroke()
          p.fill(st.glow[0]!, st.glow[1]!, st.glow[2]!, 18)
          p.rect(bl.x - 4, bl.y - 4, bl.w + 8, bl.h + 8, 14)
        }

        p.fill(typeBg[0]!, typeBg[1]!, typeBg[2]!, 240)
        p.stroke(255, 255, 255, isSelected || isHovered ? 25 : 10)
        p.strokeWeight(isSelected || isHovered ? 1.2 : 0.8)
        p.rect(bl.x, bl.y, bl.w, bl.h, 10)

        p.noStroke()
        p.fill(typeAccent[0]!, typeAccent[1]!, typeAccent[2]!, 200)
        p.rect(bl.x, bl.y + 6, ACCENT_W, BOARD_HEADER_H - 12, 2, 0, 0, 2)

        const typeLabel = isHotspotBoard ? '热点' : isConceptBoard ? '概念' : ''
        p.textSize(9)
        if (typeLabel) {
          const tlw = p.textWidth(typeLabel) + 10
          const tlx = bl.x + bl.w - tlw - 8
          p.fill(typeAccent[0]!, typeAccent[1]!, typeAccent[2]!, 35)
          p.rect(tlx, bl.y + 6, tlw, 16, 5)
          p.fill(typeAccent[0]!, typeAccent[1]!, typeAccent[2]!, 220)
          p.textAlign(p.CENTER, p.CENTER)
          p.text(typeLabel, tlx + tlw / 2, bl.y + 14)
        }

        p.fill(241, 247, 252, 230)
        p.textSize(12)
        p.textAlign(p.LEFT, p.CENTER)
        const nameMaxW = typeLabel ? bl.w - 68 : bl.w - 20
        const nameLines = wrapText(bl.board.name, nameMaxW, 2)
        for (let li = 0; li < nameLines.length; li++) {
          p.text(nameLines[li]!, bl.x + ACCENT_W + 8, bl.y + BOARD_HEADER_H / 2 - (nameLines.length - 1) * 7 + li * 14)
        }

        p.fill(186, 206, 226, 55)
        p.textSize(9)
        p.textAlign(p.RIGHT, p.CENTER)
        p.text(`${bl.board.narrative_count}条叙事`, bl.x + bl.w - 8, bl.y + BOARD_HEADER_H - 10)

        if (bl.expanded) {
          p.fill(186, 206, 226, 30)
          p.textSize(10)
          p.textAlign(p.CENTER, p.CENTER)
          p.text('▾', bl.x + bl.w / 2, bl.y + BOARD_HEADER_H - 10)

          for (const sn of bl.subNodes) {
            const nst = pal(sn.narrative.status)
            const isNarrativeSelected = props.selectedId === sn.narrative.id
            const isNarrativeHovered = hoveredId === sn.narrative.id
            const isDimmed = highlightedIds !== null && !highlightedIds.has(sn.narrative.id)
            const alpha = isDimmed ? 40 : 255
            const isAbstract = sn.narrative.source === 'abstract'

            p.fill(20, 28, 40, alpha * 0.92)
            if (isAbstract) {
              p.stroke(nst.dot[0]!, nst.dot[1]!, nst.dot[2]!, isDimmed ? 20 : (isNarrativeHovered || isNarrativeSelected ? 80 : 40))
              p.strokeWeight(isNarrativeHovered || isNarrativeSelected ? 1.2 : 0.8)
              p.drawingContext.setLineDash([4, 3])
            } else {
              p.stroke(255, 255, 255, isDimmed ? 10 : (isNarrativeHovered || isNarrativeSelected ? 30 : 12))
              p.strokeWeight(isNarrativeHovered || isNarrativeSelected ? 1.2 : 0.6)
              p.drawingContext.setLineDash([])
            }
            p.rect(sn.x, sn.y, sn.w, sn.h, 7)
            p.drawingContext.setLineDash([])

            p.noStroke()
            p.fill(nst.dot[0]!, nst.dot[1]!, nst.dot[2]!, isDimmed ? 30 : 180)
            p.rect(sn.x, sn.y + 5, 3, sn.h - 10, 1.5, 0, 0, 1.5)

            if (isAbstract) {
              p.fill(nst.badge[0]!, nst.badge[1]!, nst.badge[2]!, isDimmed ? 15 : 35)
              p.textSize(8)
              const absLabel = '抽象'
              const abw = p.textWidth(absLabel) + 8
              p.rect(sn.x + 7, sn.y + 4, abw, 12, 5)
              p.fill(nst.dot[0]!, nst.dot[1]!, nst.dot[2]!, isDimmed ? 50 : 200)
              p.textAlign(p.CENTER, p.CENTER)
              p.text(absLabel, sn.x + 7 + abw / 2, sn.y + 10)
            }

            p.fill(241, 247, 252, isDimmed ? 40 : 210)
            p.textSize(10)
            p.textAlign(p.LEFT, p.CENTER)
            const titleMaxW = sn.w - 14
            const tLines = wrapText(sn.narrative.title, titleMaxW, 1)
            p.text(tLines[0] ?? '', sn.x + 7, sn.y + sn.h / 2 + (isAbstract ? 4 : 0))
          }

          const tagCount = (bl.board.event_tags ?? []).length + (bl.board.abstract_tags ?? []).length
          if (tagCount > 0) {
            p.fill(186, 206, 226, 16)
            p.stroke(186, 206, 226, 25)
            p.strokeWeight(0.5)
            const badgeW = 76
            const badgeH = 18
            const badgeX = bl.x + (bl.w - badgeW) / 2
            const badgeY = bl.y + bl.h - badgeH - 4
            p.rect(badgeX, badgeY, badgeW, badgeH, 6)
            p.noStroke()
            p.fill(186, 206, 226, 55)
            p.textSize(9)
            p.textAlign(p.CENTER, p.CENTER)
            p.text(`${tagCount} 个标签 ▸`, badgeX + badgeW / 2, badgeY + badgeH / 2)
          }
        } else {
          p.fill(186, 206, 226, 30)
          p.textSize(10)
          p.textAlign(p.CENTER, p.CENTER)
          p.text('▸', bl.x + bl.w / 2, bl.y + BOARD_HEADER_H - 10)
        }

        p.pop()
      }

      if (p.width !== canvasW || p.height !== canvasH) {
        p.resizeCanvas(canvasW, canvasH)
      }
    }

    p.mouseMoved = () => {
      if (!canvasEl) return
      const hit = hitTest(p.mouseX, p.mouseY)
      let newHoveredId: number | null = null

      if (hit) {
        if (hit.type === 'narrative') {
          newHoveredId = hit.id
        }
        canvasEl.style.cursor = 'pointer'
      } else {
        canvasEl.style.cursor = 'default'
      }

      if (newHoveredId !== hoveredId) {
        hoveredId = newHoveredId
        computeHighlight(newHoveredId)
        canvasEl.style.cursor = hit ? 'pointer' : 'default'
        emit('hover', newHoveredId)
      }
    }

    p.mousePressed = () => {
      const hit = hitTest(p.mouseX, p.mouseY)
      if (!hit) return

      if (hit.type === 'narrative') {
        emit('select', hit.id)
      } else if (hit.type === 'board') {
        const set = new Set(expandedBoardIds.value)
        if (set.has(hit.id)) {
          set.delete(hit.id)
        } else {
          set.add(hit.id)
        }
        expandedBoardIds.value = set
        emit('board-toggle', set)
        computeLayout()
        if (p5Instance.value) {
          p5Instance.value.resizeCanvas(canvasW, canvasH)
        }
        computeHighlight(hoveredId)
      }
    }

    watch(() => props.days, () => {
      computeLayout()
      if (p5Instance.value) {
        p5Instance.value.resizeCanvas(canvasW, canvasH)
      }
      computeHighlight(hoveredId)
    }, { deep: true })

    watch(() => props.selectedId, () => {
    })

    watch(expandedBoardIds, () => {
      computeLayout()
      if (p5Instance.value) {
        p5Instance.value.resizeCanvas(canvasW, canvasH)
      }
    }, { deep: true })
  }

  p5Instance.value = new P5(sketch)
}

watch(containerRef, (el) => {
  if (el) initP5()
}, { immediate: true })
</script>

<template>
  <div ref="containerRef" class="narrative-board-canvas-wrap" />
</template>

<style scoped>
.narrative-board-canvas-wrap {
  position: relative;
  width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  border-radius: 16px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(10, 15, 22, 0.8);
}

.narrative-board-canvas-wrap :deep(canvas) {
  display: block;
}
</style>
