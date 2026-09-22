/**
 * PinCard + CardGroup — topic cards pinned to the cork wall.
 *
 * Each card is a Group: paper box + red pin + title sprite + status bar +
 * meta text. Positions come from layoutCards(); micro-tilt and Z jitter are
 * baked in at build time.
 *
 * @see specs/detective-wall-scene/spec.md §PinCard / §CardGroup / §Layout Algorithm
 */
import {
  Group, Mesh, BoxGeometry, CylinderGeometry, SphereGeometry,
  MeshStandardMaterial, MeshBasicMaterial, Color, Scene, PlaneGeometry,
  Vector3, CanvasTexture, SRGBColorSpace,
} from 'three'
import { CSS2DObject } from 'three/examples/jsm/renderers/CSS2DRenderer.js'
import gsap from 'gsap'
import type { PinCard as IPinCard, SectionTimelineNode, SectionRelation, DateRange } from './types'
import { STYLE } from './types'
import { layoutCards } from './utils'
import { proxiedImageUrl } from '~/utils/imageProxy'

/** Status → 中文标签（tooltip 与详情面板共用）。 */
const STATUS_LABELS: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  split: '分化',
  merge: '合并',
  ending: '结束',
}

function seededNoise(seed: number): () => number {
  let t = seed + 0x6D2B79F5
  return () => {
    t += 0x6D2B79F5
    let r = Math.imul(t ^ (t >>> 15), 1 | t)
    r ^= r + Math.imul(r ^ (r >>> 7), 61 | r)
    return ((r ^ (r >>> 14)) >>> 0) / 4294967296
  }
}

function nodeImageUrl(data: SectionTimelineNode): string | undefined {
  return data.imageUrl || data.image_url
}

function roundedRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number): void {
  ctx.beginPath()
  ctx.moveTo(x + radius, y)
  ctx.lineTo(x + width - radius, y)
  ctx.quadraticCurveTo(x + width, y, x + width, y + radius)
  ctx.lineTo(x + width, y + height - radius)
  ctx.quadraticCurveTo(x + width, y + height, x + width - radius, y + height)
  ctx.lineTo(x + radius, y + height)
  ctx.quadraticCurveTo(x, y + height, x, y + height - radius)
  ctx.lineTo(x, y + radius)
  ctx.quadraticCurveTo(x, y, x + radius, y)
  ctx.closePath()
}

function wrapText(
  ctx: CanvasRenderingContext2D,
  text: string,
  maxWidth: number,
  maxLines: number,
): string[] {
  const chars = Array.from(text)
  const lines: string[] = []
  let current = ''

  for (const char of chars) {
    const next = current + char
    if (ctx.measureText(next).width <= maxWidth || current.length === 0) {
      current = next
      continue
    }
    lines.push(current)
    current = char
    if (lines.length === maxLines) break
  }
  if (lines.length < maxLines && current) lines.push(current)

  if (lines.length === maxLines && chars.join('').length > lines.join('').length) {
    let last = lines[maxLines - 1] ?? ''
    while (last.length > 0 && ctx.measureText(`${last}…`).width > maxWidth) {
      last = last.slice(0, -1)
    }
    lines[maxLines - 1] = `${last}…`
  }

  return lines
}

function drawPhoto(ctx: CanvasRenderingContext2D, image: HTMLImageElement | null): void {
  const frame = { x: 458, y: 130, w: 240, h: 170 }
  ctx.save()
  ctx.translate(frame.x + frame.w / 2, frame.y + frame.h / 2)
  ctx.rotate(-0.045)
  ctx.fillStyle = 'rgba(55, 37, 22, 0.22)'
  ctx.fillRect(-frame.w / 2 + 12, -frame.h / 2 + 14, frame.w, frame.h)
  ctx.fillStyle = '#F8EFE0'
  ctx.fillRect(-frame.w / 2, -frame.h / 2, frame.w, frame.h)
  ctx.fillStyle = '#15110D'
  ctx.fillRect(-frame.w / 2 + 12, -frame.h / 2 + 12, frame.w - 24, frame.h - 34)

  if (image) {
    const boxW = frame.w - 24
    const boxH = frame.h - 34
    const scale = Math.max(boxW / image.naturalWidth, boxH / image.naturalHeight)
    const w = image.naturalWidth * scale
    const h = image.naturalHeight * scale
    ctx.save()
    ctx.beginPath()
    ctx.rect(-boxW / 2, -boxH / 2 - 5, boxW, boxH)
    ctx.clip()
    ctx.drawImage(image, -w / 2, -h / 2 - 5, w, h)
    ctx.restore()
  } else {
    const boxX = -frame.w / 2 + 12
    const boxY = -frame.h / 2 + 12
    const boxW = frame.w - 24
    const boxH = frame.h - 34
    const grad = ctx.createLinearGradient(boxX, boxY, boxX + boxW, boxY + boxH)
    grad.addColorStop(0, '#25313A')
    grad.addColorStop(0.42, '#705038')
    grad.addColorStop(1, '#1B1512')
    ctx.fillStyle = grad
    ctx.fillRect(boxX, boxY, boxW, boxH)

    ctx.strokeStyle = 'rgba(255, 238, 190, 0.18)'
    ctx.lineWidth = 2
    ctx.setLineDash([8, 7])
    ctx.beginPath()
    ctx.moveTo(boxX + 22, boxY + boxH - 26)
    ctx.bezierCurveTo(boxX + 70, boxY + 78, boxX + 112, boxY + 92, boxX + 168, boxY + 26)
    ctx.stroke()
    ctx.setLineDash([])

    const dots = [
      [boxX + 24, boxY + boxH - 26, 7],
      [boxX + 88, boxY + 76, 5],
      [boxX + 168, boxY + 26, 9],
    ] as const
    for (const [x, y, r] of dots) {
      ctx.fillStyle = 'rgba(120, 25, 25, 0.46)'
      ctx.beginPath()
      ctx.arc(x + 2, y + 2, r + 4, 0, Math.PI * 2)
      ctx.fill()
      ctx.fillStyle = '#DC2626'
      ctx.beginPath()
      ctx.arc(x, y, r, 0, Math.PI * 2)
      ctx.fill()
    }

    ctx.fillStyle = 'rgba(255, 245, 210, 0.2)'
    ctx.beginPath()
    ctx.arc(boxX + boxW - 38, boxY + 34, 28, 0, Math.PI * 2)
    ctx.fill()
    ctx.strokeStyle = 'rgba(255, 251, 235, 0.16)'
    ctx.lineWidth = 1
    for (let y = boxY + 18; y < boxY + boxH; y += 20) {
      ctx.beginPath()
      ctx.moveTo(boxX + 12, y)
      ctx.lineTo(boxX + boxW - 12, y)
      ctx.stroke()
    }

    ctx.fillStyle = 'rgba(255, 251, 235, 0.68)'
    ctx.font = '900 18px "Courier New", monospace'
    ctx.fillText('NO IMAGE', boxX + 16, boxY + 28)
  }

  ctx.fillStyle = '#2B2116'
  ctx.font = '700 16px "Courier New", monospace'
  ctx.fillText('EVIDENCE', -frame.w / 2 + 16, frame.h / 2 - 10)
  ctx.restore()
}

function renderDossierTexture(
  canvas: HTMLCanvasElement,
  data: SectionTimelineNode,
  image: HTMLImageElement | null,
): void {
  const ctx = canvas.getContext('2d')!
  const rnd = seededNoise(data.id)
  const { card } = STYLE

  const grad = ctx.createLinearGradient(0, 0, canvas.width, canvas.height)
  grad.addColorStop(0, card.paper)
  grad.addColorStop(0.46, card.paperWarm)
  grad.addColorStop(1, card.paperAged)
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, canvas.width, canvas.height)

  for (let i = 0; i < 2400; i++) {
    const alpha = 0.02 + rnd() * 0.065
    const tone = rnd() > 0.54 ? 255 : 66
    ctx.fillStyle = `rgba(${tone}, ${tone * 0.84}, ${tone * 0.48}, ${alpha})`
    ctx.fillRect(rnd() * canvas.width, rnd() * canvas.height, 1 + rnd() * 2, 1)
  }

  ctx.strokeStyle = 'rgba(76, 43, 14, 0.24)'
  ctx.lineWidth = 5
  roundedRect(ctx, 18, 18, canvas.width - 36, canvas.height - 36, 18)
  ctx.stroke()
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.26)'
  ctx.lineWidth = 2
  roundedRect(ctx, 31, 31, canvas.width - 62, canvas.height - 62, 14)
  ctx.stroke()

  ctx.fillStyle = 'rgba(95, 47, 20, 0.16)'
  ctx.fillRect(0, 385, canvas.width, 92)
  ctx.strokeStyle = 'rgba(74, 38, 18, 0.28)'
  ctx.beginPath()
  ctx.moveTo(0, 382)
  ctx.lineTo(canvas.width, 356)
  ctx.stroke()

  ctx.fillStyle = 'rgba(124, 45, 18, 0.14)'
  ctx.font = '900 74px "Courier New", monospace'
  ctx.rotate(-0.1)
  ctx.fillText('CLASSIFIED', 306, 456)
  ctx.setTransform(1, 0, 0, 1, 0, 0)

  drawPhoto(ctx, image)

  ctx.fillStyle = card.mutedInk
  ctx.font = '700 21px "Courier New", monospace'
  ctx.fillText(`CASE #${data.id}`, 64, 86)

  ctx.fillStyle = 'rgba(255, 251, 235, 0.46)'
  roundedRect(ctx, 48, 112, 382, 154, 14)
  ctx.fill()
  ctx.strokeStyle = 'rgba(76, 43, 14, 0.16)'
  ctx.lineWidth = 2
  roundedRect(ctx, 48, 112, 382, 154, 14)
  ctx.stroke()

  ctx.fillStyle = '#21140B'
  ctx.font = '900 42px "Microsoft YaHei", "Noto Sans SC", sans-serif'
  const titleLines = wrapText(ctx, data.cluster_label, 350, 3)
  titleLines.forEach((line, index) => {
    ctx.fillText(line, 62, 150 + index * 46)
  })

  const statusColor = STYLE.statusColors[data.status] ?? '#9ca3af'
  const statusLabel = STATUS_LABELS[data.status] ?? data.status
  ctx.fillStyle = statusColor
  roundedRect(ctx, 62, 330, 148, 46, 10)
  ctx.fill()
  ctx.fillStyle = '#FFF7ED'
  ctx.font = '800 24px "Microsoft YaHei", "Noto Sans SC", sans-serif'
  ctx.fillText(statusLabel, 88, 361)

  ctx.fillStyle = card.mutedInk
  ctx.font = '700 23px "Microsoft YaHei", "Noto Sans SC", sans-serif'
  ctx.fillText(`${data.article_count}篇文章`, 232, 360)
  ctx.fillText(`${data.thread_count}条线索`, 372, 360)

  ctx.strokeStyle = 'rgba(26, 26, 26, 0.18)'
  ctx.setLineDash([12, 8])
  ctx.beginPath()
  ctx.moveTo(62, 302)
  ctx.lineTo(410, 302)
  ctx.stroke()
  ctx.setLineDash([])
}

function makeDossierTexture(data: SectionTimelineNode): CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = 768
  canvas.height = 512
  const texture = new CanvasTexture(canvas)
  texture.colorSpace = SRGBColorSpace
  renderDossierTexture(canvas, data, null)
  texture.needsUpdate = true

  const imageUrl = nodeImageUrl(data)
  if (imageUrl) {
    const image = new Image()
    image.crossOrigin = 'anonymous'
    image.onload = () => {
      renderDossierTexture(canvas, data, image)
      texture.needsUpdate = true
    }
    // 经代理加载：图床有 Referer 防盗链，直连（尤其显式不发 Referer）会 403 图裂
    //（spec：侦探墙贴图不再 403；本处曾置空发送策略，是 403 主因）。
    image.src = proxiedImageUrl(imageUrl)
  }

  return texture
}

function disposeMaterial(mat: unknown): void {
  const material = mat as MeshStandardMaterial & { map?: { dispose: () => void } | null }
  material.map?.dispose()
  material.dispose()
}

export class PinCardImpl implements IPinCard {
  readonly data: SectionTimelineNode
  readonly group: Group
  readonly position: Vector3
  private readonly paperMaterial: MeshStandardMaterial
  private readonly faceMaterial: MeshBasicMaterial
  private readonly pinMaterial: MeshStandardMaterial
  /** CSS2D tooltip that follows the card in screen space (spec §Card Tooltip). */
  readonly tooltip: CSS2DObject
  private baseZ = 0
  private state: 'normal' | 'elevated' | 'highlighted' | 'dimmed' = 'normal'

  constructor(data: SectionTimelineNode) {
    this.data = data
    this.group = new Group()
    this.position = new Vector3()
    this.group.userData.cardId = data.id

    const { card, pin } = STYLE

    const frontZ = card.depth / 2 + 0.012

    // Slightly offset sheets behind the folder make it read as a physical dossier.
    const stackMat = new MeshBasicMaterial({ color: new Color(card.paperAged), transparent: true, opacity: 0.42 })
    const backSheet = new Mesh(new PlaneGeometry(card.width * 0.96, card.height * 0.88), stackMat)
    backSheet.position.set(0.09, -0.1, -card.depth / 2 - 0.012)
    backSheet.rotation.z = 0.035
    const midSheet = new Mesh(new PlaneGeometry(card.width * 0.98, card.height * 0.92), stackMat.clone())
    midSheet.position.set(-0.06, -0.04, -card.depth / 2 - 0.006)
    midSheet.rotation.z = -0.026
    this.group.add(backSheet, midSheet)

    // Soft drop shadow, offset behind the card to make it feel pinned to a wall.
    const shadowMat = new MeshBasicMaterial({
      color: new Color(card.shadow),
      transparent: true,
      opacity: 0.32,
      depthWrite: false,
    })
    const shadow = new Mesh(new PlaneGeometry(card.width * 1.08, card.height * 1.08), shadowMat)
    shadow.position.set(0.08, -0.08, -card.depth / 2 - 0.018)
    this.group.add(shadow)

    // Paper card. A small canvas texture gives every card a non-flat evidence-file feel.
    // Cards tint by persistent topic so a narrative's sections share a hue across
    // days; cards without a topic keep the default warm paper. Candidate (under
    // observation) topics use the aged paper tone so they read as provisional.
    const topic = data.persistent_topic
    const isCandidate = topic?.status === 'candidate'
    const paperColor = topic?.color ?? card.paperWarm
    this.paperMaterial = new MeshStandardMaterial({
      color: new Color(isCandidate ? card.paperAged : paperColor),
      roughness: card.roughness,
      metalness: card.metalness,
      emissive: new Color(isCandidate ? card.paperAged : paperColor),
      // Candidate cards dim slightly to signal "under observation".
      emissiveIntensity: isCandidate ? 0.10 : 0.18,
    })
    const paper = new Mesh(
      new BoxGeometry(card.width, card.height, card.depth),
      this.paperMaterial,
    )
    this.group.add(paper)

    this.faceMaterial = new MeshBasicMaterial({
      map: makeDossierTexture(data),
    })
    const face = new Mesh(new PlaneGeometry(card.width * 0.985, card.height * 0.965), this.faceMaterial)
    face.position.set(0, 0, frontZ + 0.003)
    this.group.add(face)

    // Folder tab, pocket lip, aged edge strips and tape complete the dossier shape.
    const tabMat = new MeshBasicMaterial({ color: new Color('#EBCB8B'), transparent: true, opacity: 0.92 })
    const tab = new Mesh(new PlaneGeometry(0.82, 0.2), tabMat)
    tab.position.set(-0.46, card.height / 2 + 0.08, -0.012)
    tab.rotation.z = -0.02
    const pocketMat = new MeshBasicMaterial({ color: new Color('#B7783A'), transparent: true, opacity: 0.18 })
    const pocket = new Mesh(new PlaneGeometry(card.width * 0.98, 0.28), pocketMat)
    pocket.position.set(0, -card.height / 2 + 0.14, frontZ + 0.006)
    pocket.rotation.z = -0.018
    const edgeMat = new MeshBasicMaterial({ color: new Color(card.paperAged), transparent: true, opacity: 0.72 })
    const topEdge = new Mesh(new PlaneGeometry(card.width * 0.96, 0.035), edgeMat)
    topEdge.position.set(0, card.height / 2 - 0.035, frontZ)
    const bottomEdge = new Mesh(new PlaneGeometry(card.width * 0.92, 0.026), edgeMat.clone())
    bottomEdge.position.set(0, -card.height / 2 + 0.04, frontZ)
    const tapeMat = new MeshBasicMaterial({ color: new Color(card.tape), transparent: true, opacity: 0.64 })
    const tape = new Mesh(new PlaneGeometry(card.width * 0.5, 0.18), tapeMat)
    tape.position.set(-0.42, card.height / 2 - 0.11, frontZ + 0.004)
    tape.rotation.z = -0.05
    const cornerMat = new MeshBasicMaterial({ color: new Color('#F7E7C4'), transparent: true, opacity: 0.42 })
    const corner = new Mesh(new PlaneGeometry(0.24, 0.24), cornerMat)
    corner.position.set(card.width / 2 - 0.15, card.height / 2 - 0.15, frontZ + 0.01)
    corner.rotation.z = 0.78
    this.group.add(tab, pocket, topEdge, bottomEdge, tape, corner)

    // Red pin (cylinder shaft + sphere head), top-center of the card.
    this.pinMaterial = new MeshStandardMaterial({
      color: new Color(pin.color),
      roughness: pin.roughness,
      metalness: pin.metalness,
      emissive: new Color('#5f0707'),
      emissiveIntensity: 0.12,
    })
    const shaft = new Mesh(
      new CylinderGeometry(pin.radius * 0.34, pin.radius * 0.34, 0.18, 10),
      this.pinMaterial,
    )
    shaft.position.set(0, card.height / 2 + 0.07, frontZ)
    shaft.rotation.x = Math.PI / 2
    const head = new Mesh(
      new SphereGeometry(pin.radius * 1.18, 18, 18),
      this.pinMaterial,
    )
    head.position.set(0, card.height / 2 + 0.15, frontZ + 0.08)
    this.group.add(shaft, head)

    const statusLabel = STATUS_LABELS[data.status] ?? data.status

    // CSS2D tooltip (spec §Card Tooltip): shown on hover, hidden otherwise.
    // Positioned above the card; CSS2DRenderer projects it into screen space.
    // pointer-events are enabled so clicking the tooltip text also selects the
    // card (the label sits above the 3D mesh, which users naturally click).
    const tooltipEl = document.createElement('div')
    tooltipEl.className = 'tdw-card-tooltip'
    tooltipEl.dataset.cardId = String(data.id)
    tooltipEl.style.display = 'none'
    tooltipEl.style.pointerEvents = 'auto'
    tooltipEl.style.cursor = 'pointer'
    const labelLine = document.createElement('div')
    labelLine.className = 'tdw-card-tooltip-label'
    labelLine.textContent = data.cluster_label
    const statusLine = document.createElement('div')
    statusLine.className = 'tdw-card-tooltip-status'
    statusLine.textContent = `${statusLabel} · ${data.article_count}篇`
    tooltipEl.append(labelLine, statusLine)
    this.tooltip = new CSS2DObject(tooltipEl)
    this.tooltip.position.set(0, card.height / 2 + 0.5, 0)
    this.group.add(this.tooltip)
  }

  /** Apply a layout result (position + rotation). */
  place(position: Vector3, rotationZ: number): void {
    this.baseZ = position.z
    this.position.copy(position)
    this.group.position.copy(position)
    this.group.rotation.z = rotationZ
  }

  elevate(): void {
    if (this.state === 'elevated') return
    this.state = 'elevated'
    gsap.to(this.group.position, { z: this.baseZ + 0.2, duration: 0.2, ease: 'power2.out' })
    this.tooltip.element.style.display = 'block'
  }

  settle(): void {
    if (this.state !== 'elevated') return
    this.state = 'normal'
    gsap.to(this.group.position, { z: this.baseZ, duration: 0.2, ease: 'power2.out' })
    this.tooltip.element.style.display = 'none'
  }

  highlight(): void {
    this.state = 'highlighted'
    // Keep the warm glow on the physical folder body; the readable face texture
    // is rendered by an unlit plane above it.
    gsap.to(this.paperMaterial, { emissiveIntensity: 0.42, opacity: 1, transparent: false, duration: 0.3 })
    gsap.to(this.faceMaterial, { opacity: 1, transparent: false, duration: 0.3 })
    // Hide tooltip in lifeline mode (detail panel shows the info instead).
    this.tooltip.element.style.display = 'none'
  }

  dim(): void {
    this.state = 'dimmed'
    gsap.to(this.paperMaterial, { opacity: 0.35, transparent: true, duration: 0.3 })
    gsap.to(this.faceMaterial, { opacity: 0.48, transparent: true, duration: 0.3 })
    this.tooltip.element.style.display = 'none'
  }

  reset(): void {
    this.state = 'normal'
    gsap.to(this.group.position, { z: this.baseZ, duration: 0.2, ease: 'power2.out' })
    gsap.to(this.paperMaterial, { emissiveIntensity: 0.18, opacity: 1, transparent: false, duration: 0.3 })
    gsap.to(this.faceMaterial, { opacity: 1, transparent: false, duration: 0.3 })
    this.tooltip.element.style.display = 'none'
  }
}

export class CardGroup {
  readonly cards: PinCardImpl[] = []
  private readonly byId = new Map<number, PinCardImpl>()

  buildCards(
    sections: SectionTimelineNode[],
    _relations: SectionRelation[],
    _dateRange: DateRange,
    scene: Scene,
    viewMode: 'timeline' | 'lanes' = 'timeline',
    laneBatch = 0,
  ): void {
    const layout = layoutCards(sections, viewMode, laneBatch)
    for (const section of sections) {
      // 批次外的赛道没有布局坐标，不创建卡片。
      const pos = layout.get(section.id)
      if (!pos) continue
      const card = new PinCardImpl(section)
      card.place(pos.position, pos.rotationZ)
      this.cards.push(card)
      this.byId.set(section.id, card)
      scene.add(card.group)
    }
  }

  getCardById(id: number): PinCardImpl | undefined {
    return this.byId.get(id)
  }

  /** Stagger entrance: cards rise from below the wall. */
  staggerEntrance(intervalSec: number): gsap.core.Timeline {
    const tl = gsap.timeline()
    this.cards.forEach((card, i) => {
      const targetY = card.group.position.y
      card.group.position.y = targetY - 3
      card.group.visible = false
      tl.set(card.group, { visible: true }, i * intervalSec)
      tl.to(card.group.position, { y: targetY, duration: 0.4, ease: 'back.out(1.4)' }, i * intervalSec)
    })
    return tl
  }

  /** Stagger exit: cards fall away. */
  staggerExit(intervalSec: number): gsap.core.Timeline {
    const tl = gsap.timeline()
    this.cards.forEach((card, i) => {
      tl.to(card.group.position, { y: card.group.position.y - 3, duration: 0.3, ease: 'power2.in' }, i * intervalSec)
    })
    return tl
  }

  highlightSet(ids: Set<number>): void {
    for (const card of this.cards) {
      if (ids.has(card.data.id)) card.highlight()
      else card.dim()
    }
  }

  dimAll(): void {
    for (const card of this.cards) card.dim()
  }

  resetAll(): void {
    for (const card of this.cards) card.reset()
  }

  /** Per-frame update hook (reserved for future idle animation). */
  update(_dt: number): void {
    // no-op for now
  }

  clear(scene: Scene): void {
    for (const card of this.cards) {
      scene.remove(card.group)
      // Remove the CSS2D tooltip element from the DOM (it lives outside the canvas).
      card.tooltip.element.remove()
      card.group.traverse((obj) => {
        const mesh = obj as Mesh
        if (mesh.geometry) mesh.geometry.dispose()
        const mat = mesh.material
        if (Array.isArray(mat)) mat.forEach(disposeMaterial)
        else if (mat) disposeMaterial(mat)
      })
    }
    this.cards.length = 0
    this.byId.clear()
  }
}
