/**
 * 新建泳道编排态 · 纯展示计算（rework-workshop-into-radar-and-desk / workshop-lane-compose）。
 *
 * 本文件是原型 `ui-prototype/revision-2026-10-30-lane-compose-selection/compose-logic.js`
 * 的**逐字移植**（唯一改动：字段名从原型的 snake_case 换成产品 API 层的 camelCase，
 * 数字 id 换成字符串 id）。原型是视觉与行为的唯一真源，移植后不再各自演化。
 *
 * 移植清单：`centroidOf` / `cosineFor` / `centroidRecommendations` /
 * `laneSelectionState` / `candidateSelection` / `estimateTextWidth` /
 * `truncateText` / `boxesOverlap` / `placeLabels` / `pointAppearance`。
 *
 * **不含** `candidateChains`——候选链口径（连通片段 + 跨度 ≥3 天）由服务端唯一实现，
 * 前端只渲染（契约 §2.1）。
 *
 * **末尾的「画布缩放」一节是 2026-11-01 后新增的**（滚轮缩放），**不在原型移植清单里**：
 * 它只做呈现换算（档位 / 步进 / 光标锚点 / 点半径下限），不碰任何口径。
 *
 * 纪律（契约 §2.2 / §2.3）：
 * - 质心 = 全部**有效**已选成员向量的均值（不是首个成员、不是点击焦点、不是旧 Embedding）；
 * - 有效 = 长度与首个有效向量相同且全为有限数；
 * - 推荐 = 窗口内未选且 distance ≤ matchThreshold 的 section，按 distance → id 稳定排序；
 * - `unassigned`（非 active，含其他 candidate 与归档来源）优先，`active` 次级只读；
 * - 推荐**不是**置信度、不生成关系边；推荐点半径恒小于已选点半径。
 */

/** 编排态最小 section 形态：只依赖 id / embedding / 归属 status。 */
export interface LaneSection {
  id: string
  embedding: number[]
  persistentTopic?: { id?: string, label?: string, status?: string } | null
}

/** 单条推荐（质心距离 + 命中的 section）。 */
export interface LaneRecommendation<T extends LaneSection = LaneSection> {
  section: T
  /** 1 − cos(质心, section.embedding)，越小越近。 */
  distance: number
}

/** 质心推荐结果：优先组（非 active）+ 次级只读组（active）。 */
export interface LaneRecommendations<T extends LaneSection = LaneSection> {
  /** 有效已选成员向量的均值；无有效成员时为 null（不伪造距离）。 */
  center: number[] | null
  /** 非 active 来源（其他 candidate / 未归属 / 归档来源）——优先组，可选入。 */
  unassigned: LaneRecommendation<T>[]
  /** active 泳道来源——次级只读参照，不可选入。 */
  active: LaneRecommendation<T>[]
}

/** 组选择三态派生（unchecked / indeterminate / checked 同源）。 */
export interface LaneSelectionState {
  count: number
  total: number
  checked: boolean
  mixed: boolean
}

/** 标题避让用的矩形框。 */
export interface LaneBox {
  x: number
  y: number
  w: number
  h: number
}

/** 参与避让的标题候选。 */
export interface LaneLabelCandidate {
  id: string
  x: number
  y: number
  /** 关联点的半径（避让时留出间隙）。 */
  r: number
  w: number
  h: number
  /** 优先级大者先放。 */
  priority: number
  text: string
}

/** 避让后的标题（skipped=true 表示四个位置都撞上，宁缺勿叠）。 */
export interface LanePlacedLabel extends LaneLabelCandidate {
  box?: LaneBox
  skipped?: boolean
}

/** 标题避让参数。 */
export interface LanePlaceLabelsOptions {
  /** 标签之间的最小间隙（px）。 */
  gap?: number
  /** 依次尝试的位置顺序。 */
  orders?: ('right' | 'left' | 'above' | 'below')[]
  minX?: number
  maxX?: number
  minY?: number
  maxY?: number
}

/** 原关系的一条边（编排画布用；只认服务端透出的原关系）。 */
export interface LaneRelation {
  fromId: string
  toId: string
}

/**
 * 与已选成员**有原关系**（1 跳）的其它节点 id 集合。
 *
 * - 只认原关系（`identity` 与 `similarity` 都算「原关系」），**不新增、不推断**语义边；
 * - **1 跳**：与已选直接相连的节点，不做传递闭包（否则整张图会被点亮成一片）；
 * - 已选成员本身不在结果里（用户要的是「**其他**相关节点」）；
 * - 传入的 relations SHALL 已过滤到「两端都在画布可见集合」的边（调用方负责）；
 * - `recommendedIds`（可选，unify-lane-workbench D6）：**已算出的推荐 ID**（质心口径，
 *   由调用方按各自可选资格过滤后传入）——相关集合 = 一跳原关系 ∪ 推荐，
 *   去重、排除种子；这是两画布「相关节点」的唯一并集实现入口。焦点高亮传
 *   种子 `{focusId}`、推荐仍用成员质心那批；已选标题入口传已选集做种子。
 */
export function relatedNodeIds(relations: LaneRelation[], selectedIds: Set<string>, recommendedIds?: Iterable<string>): Set<string> {
  const out = new Set<string>()
  if (selectedIds.size) {
    for (const rel of relations) {
      const from = String(rel?.fromId ?? '')
      const to = String(rel?.toId ?? '')
      if (!from || !to) continue
      if (selectedIds.has(from) && !selectedIds.has(to)) out.add(to)
      else if (selectedIds.has(to) && !selectedIds.has(from)) out.add(from)
    }
  }
  if (recommendedIds) {
    for (const id of recommendedIds) {
      const key = String(id ?? '')
      if (!key) continue
      if (!selectedIds.has(key)) out.add(key)
    }
  }
  return out
}

/**
 * 相关节点标题的落位：**只放节点下方**，撞上已放标题时**向下错行**（最多 `lines` 行），
 * 仍撞上才跳过（宁缺勿叠）；越出画布边界直接跳过。
 *
 * 与 `placeLabels` 的差别：`placeLabels` 是原型口径（四方向、不重叠即放弃），
 * 本函数服务「相关节点标题」——相关节点常落在相邻日期列，逐点标题本就会互相压，
 * 向下错一行既保住信息又不压字。
 */
export function placeRelatedTitles(
  cands: LaneLabelCandidate[],
  opts: LanePlaceLabelsOptions & { lines?: number, lineHeight?: number } = {},
): LanePlacedLabel[] {
  const lines = opts.lines ?? 2
  const lineHeight = opts.lineHeight ?? 16
  const gap = opts.gap ?? 0
  const placed: LaneBox[] = []
  const out: LanePlacedLabel[] = []
  const minX = opts.minX ?? -Infinity
  const maxX = opts.maxX ?? Infinity
  const minY = opts.minY ?? -Infinity
  const maxY = opts.maxY ?? Infinity
  for (const c of [...cands].sort((a, b) => b.priority - a.priority || String(a.id).localeCompare(String(b.id)))) {
    let done = false
    for (let line = 0; line <= lines; line++) {
      const box: LaneBox = {
        x: c.x - c.w / 2,
        y: c.y + c.r + 5 + line * lineHeight,
        w: c.w,
        h: c.h,
      }
      if (box.x < minX || box.x + box.w > maxX || box.y < minY || box.y + box.h > maxY) continue
      const hit = placed.some(p => boxesOverlap(
        { x: box.x - gap, y: box.y - gap, w: box.w + 2 * gap, h: box.h + 2 * gap },
        p,
      ))
      if (!hit) {
        placed.push({ ...box })
        out.push({ ...c, box })
        done = true
        break
      }
    }
    if (!done) out.push({ ...c, skipped: true })
  }
  return out
}

/**
 * 拖拽平移：把「起点 → 当前点」的位移换算成新的滚动位置（拖内容，滚动条反向走）。
 *
 * - `start` / `current` 是指针的**屏幕坐标**（px）；
 * - `scroll` 是拖拽开始那一刻的滚动位置；
 * - 返回值直接写回 `scrollLeft` / `scrollTop`（超出范围由浏览器夹紧）。
 */
export function panScroll(
  start: { x: number, y: number },
  current: { x: number, y: number },
  scroll: { left: number, top: number },
): { left: number, top: number } {
  return {
    left: scroll.left - (current.x - start.x),
    top: scroll.top - (current.y - start.y),
  }
}

/**
 * 拖拽判定：位移超过阈值才算「拖」（否则算「点」）。
 * 阈值用来区分「点节点」与「拖画布」——点节点要照常触发选中和节点卡。
 */
export function isDragBeyond(
  start: { x: number, y: number },
  current: { x: number, y: number },
  threshold = 4,
): boolean {
  return Math.abs(current.x - start.x) > threshold || Math.abs(current.y - start.y) > threshold
}

/** 点的视觉映射结果（半径 / 接近程度 / 填色强度）。 */
export interface LanePointAppearance {
  radius: number
  /** 接近程度 [0,1]；非推荐点与只读参照为 null。 */
  proximity: number | null
  /** 填色强度百分比（仅表达接近程度，不是置信度）。 */
  strength: number
  /** 成员实心（实心＋✓表达选中；尺寸不编码接近度）。 */
  solid: boolean
  /** 正交标志（unify-lane-workbench D7）：移出划除 / 首义虚线 / 焦点环 / 相关高亮 / 弱化。 */
  removed: boolean
  anchor: boolean
  focus: boolean
  related: boolean
  dimmed: boolean
}

/**
 * 点形正交标志（unify-lane-workbench D7）：身份（成员/候选/只读）由位置参数表达，
 * 移出/首义/焦点/相关/弱化是**正交标志**——互不顶掉，由调用方叠加渲染。
 */
export interface LanePointFlags {
  /** 已移出（本次）：划除线标志。 */
  removed?: boolean
  /** 首义节点：虚线标志。 */
  anchor?: boolean
  /** 焦点节点：焦点环标志。 */
  focus?: boolean
  /** 相关高亮（焦点的一跳/推荐并集命中）。 */
  related?: boolean
  /** 弱化（画布有焦点时的非相关节点）。 */
  dimmed?: boolean
  /** 成员固定半径覆盖（两画布布局密度不同；缺省用 cap 档位公式）。 */
  memberRadius?: number
}

/** id 升序比较：数字 id 按数值升序，非数字 id 按字典序（原型用 Number()，此处补兜底）。 */
function compareIds(a: string, b: string): number {
  const na = Number(a)
  const nb = Number(b)
  if (Number.isFinite(na) && Number.isFinite(nb)) return na - nb
  return String(a).localeCompare(String(b))
}

/**
 * 质心 = 全部有效已选成员的向量均值。
 *
 * 有效 = 长度与**首个有效向量**相同且全为有限数；无效成员不参与平均、不污染结果。
 * 无任何有效成员时返回 null（调用方据此不伪造距离，也不取消已选成员）。
 */
export function centroidOf(items: LaneSection[]): number[] | null {
  const first = items.find(s => Array.isArray(s.embedding) && s.embedding.length && s.embedding.every(Number.isFinite))
  if (!first) return null
  const valid = items.filter(s => Array.isArray(s.embedding) && s.embedding.length === first.embedding.length && s.embedding.every(Number.isFinite))
  const result = first.embedding.map(() => 0)
  for (const s of valid) {
    for (let i = 0; i < result.length; i++) result[i]! += s.embedding[i]! / valid.length
  }
  return result
}

/** 余弦**距离**（1 − cos）；长度不等 / 零范数 / 缺参一律 null（不伪造距离）。 */
export function cosineFor(a?: number[] | null, b?: number[] | null): number | null {
  if (!a?.length || a.length !== b?.length) return null
  let dot = 0
  let x = 0
  let y = 0
  for (let i = 0; i < a.length; i++) {
    dot += a[i]! * b[i]!
    x += a[i]! * a[i]!
    y += b[i]! * b[i]!
  }
  return x && y ? 1 - dot / Math.sqrt(x * y) : null
}

/**
 * 质心推荐。
 *
 * - 质心来自 `pool` 中**已选**成员（焦点不参与，焦点不是质心来源）；
 * - 候选 = 窗口内未选且有有效 embedding 的 section，distance = 1 − cos(质心, embedding)；
 * - 保留 distance ≤ threshold（等于阈值也保留），按 distance → section id 升序稳定排序；
 * - 分组：非 active → `unassigned`（优先），active → `active`（次级只读）。
 */
export function centroidRecommendations<T extends LaneSection>(
  pool: T[],
  chosen: Set<string>,
  threshold: number,
): LaneRecommendations<T> {
  const center = centroidOf(pool.filter(s => chosen.has(s.id)))
  if (!center) return { center: null, unassigned: [], active: [] }
  const rank = pool
    .filter(s => !chosen.has(s.id))
    .map(section => ({ section, distance: cosineFor(section.embedding, center) }))
    .filter((x): x is { section: T, distance: number } => x.distance !== null && Number.isFinite(x.distance) && x.distance <= threshold)
    .sort((a, b) => a.distance - b.distance || compareIds(a.section.id, b.section.id))
  return {
    center,
    unassigned: rank.filter(x => x.section.persistentTopic?.status !== 'active'),
    active: rank.filter(x => x.section.persistentTopic?.status === 'active'),
  }
}

/** 组选择三态：空组 = unchecked（不是 indeterminate）；部分选 = indeterminate。 */
export function laneSelectionState(chosen: Set<string>, ids: string[]): LaneSelectionState {
  const count = ids.filter(id => chosen.has(id)).length
  return {
    count,
    total: ids.length,
    checked: ids.length > 0 && count === ids.length,
    mixed: count > 0 && count < ids.length,
  }
}

/**
 * 整组勾选 / 取消：追加去重，**不截条数**、**不自动补相似**、**不吞组外人工补选**。
 * 返回新集合（不改入参）。
 */
export function candidateSelection(chosen: Set<string>, ids: string[], checked: boolean): Set<string> {
  const next = new Set(chosen)
  for (const id of ids) {
    if (checked) next.add(id)
    else next.delete(id)
  }
  return next
}

/** 文字宽度估算（CJK 按字号、其余按 0.55 字号），只用于避让与截断预算。 */
export function estimateTextWidth(text: string, size: number): number {
  let w = 0
  for (const ch of String(text ?? '')) w += ch.charCodeAt(0) > 255 ? size : size * 0.55
  return w
}

/** 截断到上限并补省略号（长度不超限时原样返回）。 */
export function truncateText(text: string, max: number): string {
  const s = String(text ?? '')
  return s.length <= max ? s : s.slice(0, Math.max(1, max - 1)) + '…'
}

/** 两个矩形是否相交（标题避让用）。 */
export function boxesOverlap(a: LaneBox, b: LaneBox): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
}

/**
 * 标题避让：按优先级依次尝试右/左/上/下四个位置，都撞上（或越界）就放弃这条标题
 * （宁缺勿叠）。`minX/maxX/minY/maxY` 是画布边界，越界的候选直接跳过。
 */
export function placeLabels(
  cands: LaneLabelCandidate[],
  opts: LanePlaceLabelsOptions = {},
): LanePlacedLabel[] {
  const gap = opts.gap ?? 3
  const orders = opts.orders ?? ['right', 'left', 'above', 'below']
  const placed: LaneBox[] = []
  const out: LanePlacedLabel[] = []
  const minX = opts.minX ?? -Infinity
  const maxX = opts.maxX ?? Infinity
  const minY = opts.minY ?? -Infinity
  const maxY = opts.maxY ?? Infinity
  for (const c of [...cands].sort((a, b) => b.priority - a.priority || String(a.id).localeCompare(String(b.id)))) {
    const box: LaneBox = { x: 0, y: 0, w: c.w, h: c.h }
    let done = false
    for (const pos of orders) {
      if (pos === 'right') { box.x = c.x + c.r + 5; box.y = c.y - c.h / 2 }
      else if (pos === 'left') { box.x = c.x - c.r - 5 - c.w; box.y = c.y - c.h / 2 }
      else if (pos === 'above') { box.x = c.x - c.w / 2; box.y = c.y - c.r - 5 - c.h }
      else { box.x = c.x - c.w / 2; box.y = c.y + c.r + 5 }
      if (box.x < minX || box.x + box.w > maxX || box.y < minY || box.y + box.h > maxY) continue
      if (!placed.some(p => boxesOverlap({ x: box.x - gap, y: box.y - gap, w: box.w + 2 * gap, h: box.h + 2 * gap }, p))) {
        placed.push({ ...box })
        out.push({ ...c, box })
        done = true
        break
      }
    }
    if (!done) out.push({ ...c, skipped: true })
  }
  return out
}

/**
 * 成员节点基准半径（由列宽推导）。单一公式源：
 * `pointAppearance` 与画布行距计算都走这里，避免两处漂移。
 *
 * - `cap = max(3, min(16, columnWidth × 0.46))`——列宽越窄上限越小，但不小于 3px；
 * - 基准半径落在 `[min(cap,10), cap]` 的 0.85 位置（大列宽下稳定在 13.6px）。
 */
export function defaultMemberRadius(columnWidth: number): number {
  const cap = Math.max(3, Math.min(16, columnWidth * 0.46))
  return Math.max(Math.min(cap, 10), cap * 0.85)
}

/**
 * 点的视觉映射（**只表达接近程度，不是置信度**）。
 *
 * - 普通点半径下限 3px；只读参照下限 2.5px；
 * - 推荐点半径恒小于已选点半径（`recommendCap = cap * 0.72`，已选 ≥ `cap * 0.85`）；
 * - 选中只由「实心 + ✓」表达，不用大小或深浅暗示。
 */
export function pointAppearance(
  distance: number | null | undefined,
  threshold: number,
  selected: boolean,
  readonly: boolean,
  baseRadius: number,
  columnWidth: number,
  flags: LanePointFlags = {},
): LanePointAppearance {
  const cap = Math.max(3, Math.min(16, columnWidth * 0.46))
  const proximity = !readonly && Number.isFinite(distance) && Number.isFinite(threshold) && threshold > 0 && distance! <= threshold
    ? Math.max(0, Math.min(1, 1 - distance! / threshold))
    : null
  const ordinary = readonly ? Math.max(2.5, baseRadius * 0.65) : Math.max(3, Math.min(8, baseRadius * 0.8))
  // 成员基准半径**不读接近度**（尺寸不编码成员远近）；memberRadius 允许修正画布
  // 按自己的布局密度给固定尺寸（缺省用 cap 档位公式，编排画布视觉不变）。
  const memberBase = Number.isFinite(flags.memberRadius) && flags.memberRadius! > 0
    ? Math.max(3, flags.memberRadius!)
    : defaultMemberRadius(columnWidth)
  // 推荐点半径恒小于已选点半径：大小只表达「接近程度」，选中只由实心＋✓表达。
  // memberRadius 覆盖时同步压 recommendCap（否则固定小尺寸成员会被大候选压过）。
  const recommendCap = Math.min(cap * 0.72, memberBase - 0.5)
  const recommend = proximity === null
    ? ordinary
    : Math.max(3, Math.min(recommendCap, baseRadius * (0.9 + 1.0 * proximity)))
  const radius = selected ? memberBase : recommend
  return {
    radius,
    proximity,
    strength: proximity === null ? 0 : 12 + 55 * proximity,
    solid: selected,
    removed: Boolean(flags.removed),
    anchor: Boolean(flags.anchor),
    focus: Boolean(flags.focus),
    related: Boolean(flags.related),
    dimmed: Boolean(flags.dimmed),
  }
}

// ── 画布缩放（2026-11-01 用户补充） ───────────────────────────────────────
/**
 * 缩放档位：0.5× – 3×，步进 1.1×，默认 1（100%）。
 *
 * 缩放**只影响呈现**：不碰选择、候选资格、质心计算、相关节点口径、候选链口径。
 */
export const LANE_ZOOM_MIN = 0.5
export const LANE_ZOOM_MAX = 3
export const LANE_ZOOM_STEP = 1.1
export const LANE_ZOOM_DEFAULT = 1

/** 把缩放值夹到 [min, max]；非法值（非有限）回落到 100%。 */
export function clampZoom(zoom: number, min = LANE_ZOOM_MIN, max = LANE_ZOOM_MAX): number {
  if (!Number.isFinite(zoom)) return LANE_ZOOM_DEFAULT
  return Math.min(max, Math.max(min, zoom))
}

/**
 * 滚轮步进：deltaY < 0（向上滚）放大、> 0（向下滚）缩小。
 *
 * 步长按 `|deltaY| / 100` 折算：滚轮一格（100）正好一个 1.1 步进；
 * 触控板的小增量折成更细的步子（不会一滑就飞出去）。
 */
export function zoomStep(current: number, deltaY: number, step = LANE_ZOOM_STEP): number {
  if (!Number.isFinite(deltaY) || deltaY === 0) return clampZoom(current)
  return clampZoom(current * Math.pow(step, -deltaY / 100))
}

/**
 * 光标锚点的滚动位置换算：缩放后让**光标下的内容尽量不动**。
 *
 * - `anchorOffset`：光标在容器可视区里的偏移（px，可为负＝光标在容器外）；
 * - `scroll`：缩放前的滚动位置（px）；`from` / `to`：缩放前后的比例。
 *
 * 公式：内容坐标 `L = (anchorOffset + scroll) / from`；新滚动 `= L * to − anchorOffset`。
 * 结果不小于 0（内容缩小时浏览器本来也会把滚动位置夹回 0）。
 */
export function zoomAnchorScroll(anchorOffset: number, scroll: number, from: number, to: number): number {
  if (!Number.isFinite(from) || from <= 0) return Math.max(0, scroll)
  const content = (anchorOffset + scroll) / from
  return Math.max(0, content * to - anchorOffset)
}

/**
 * 点的**像素下限**补偿：缩小时保证**渲染半径**仍 ≥ 下限
 * （普通点 3px、只读参照 2.5px——不许把点缩到看不见）。
 *
 * 返回**逻辑坐标**下要写进 SVG 的半径（渲染半径 = 该值 × zoom）。
 * 100% 时原样返回（`radius ≥ 下限` 恒成立），不改变既有视觉。
 */
export function scaledPointRadius(radius: number, zoom: number, minRendered = 3): number {
  if (!Number.isFinite(radius) || radius <= 0) return radius
  const z = Number.isFinite(zoom) && zoom > 0 ? zoom : 1
  return Math.max(radius, minRendered / z)
}

// ── unify-lane-workbench：接近度方向统一 + 工作面曲线纯函数 ─────────────────
/**
 * 接近度展示换算（D7）：`接近度 = 1 − 距离`（= cos），数值越大越近。
 *
 * - 内部排序仍用距离（越小越近），**展示**统一换算成本函数；
 * - null / 非有限 → null（不伪造数字）；0 距离 → 1；距离 1 → 接近度 0（保留 0，
 *   不吞成 null）；
 * - 只做数值换算，不区分来源——快照（归属距离）与估算（质心距离）的分名分形
 *   由调用方标注，两者不得变成同一来源。
 */
export function proximityOf(distance: number | null | undefined): number | null {
  if (distance === null || distance === undefined || !Number.isFinite(distance)) return null
  return 1 - distance
}

/** 工作面曲线日点（与矩阵 ObservationRangeControls 消费的 series 同形）。 */
export interface WorkbenchTrendPoint {
  date: string
  /** null = 该日未知（未取到 / 应计项缺计数）；0 = 已知零。 */
  value: number | null
}

/** 编辑泳道曲线取数用的 lane-dynamics 泳道最小形态。 */
export interface WorkbenchDynamicsLane {
  topicId: string
  timeline: readonly { date: string, clueCount?: number | null }[]
}

/**
 * 编辑泳道曲线：从 lane-dynamics 归一读模型里选**目标泳道**的逐日线索数
 * （ui-design「编辑泳道」曲线口径；其它泳道增减不影响该曲线）。
 *
 * - 返回 `null` = 目标泳道未出现在响应里（未知，不造全 0 曲线）；
 * - 返回的 series 只含 timeline 里出现的日期，`clueCount` 缺失/非法 → 该日 null；
 *   未出现的日期由调用方在 bounds 轴上映射为 null（断档 = 未知，不补 0）。
 */
export function laneSeriesFromDynamics(
  lanes: readonly WorkbenchDynamicsLane[],
  topicId: string,
): WorkbenchTrendPoint[] | null {
  const lane = lanes.find(entry => entry.topicId === String(topicId))
  if (!lane) return null
  return lane.timeline
    .filter(day => typeof day?.date === 'string' && day.date)
    .map(day => ({
      date: String(day.date).slice(0, 10),
      value: typeof day.clueCount === 'number' && Number.isFinite(day.clueCount) ? day.clueCount : null,
    }))
}

/** 新增泳道曲线聚合的 section 最小形态（compose-candidates 归一后）。 */
export interface PoolCurveSection {
  id: string
  /** YYYY-MM-DD（前 10 位）。 */
  periodDate: string
  /** 该 section 的非综合稿线索数；null/缺省 = 未知（旧后端）。 */
  clueCount?: number | null
  persistentTopicId?: string
  persistentTopic?: { status?: string } | null
}

/** 归属状态对可选池的计入判定。 */
type PoolCountKind = 'include' | 'exclude' | 'unknown'

function poolCountKind(section: PoolCurveSection): PoolCountKind {
  const status = section.persistentTopic?.status ?? ''
  if (!section.persistentTopicId) return 'include' // 无归属：可选池的一部分
  if (status === 'active') return 'exclude' // 不含当前 active 泳道
  if (status === 'candidate') return 'include' // 动态泳道：可选（改挂）
  if (status === 'archived' || status === 'disabled') return 'exclude' // 归档/禁用不计入
  return 'unknown' // 带归属但状态未知：不假定可选，也不假定排除
}

/**
 * 新增泳道曲线：**全部可选线索数**（不含 active）逐日聚合（D2 / lane-correction
 * delta「新增泳道曲线只计可选池」）。
 *
 * - 计入 = 无归属 + candidate 来源；排除 = active / archived / disabled；
 * - 带归属但缺状态 → 该日未知（不把已知子集冒充合计）；
 * - 任一应计 section 缺 `clueCount` → 该日未知；已知 0 保留；
 * - 该日无应计 section → 已知 0（可选池为空是合法结论）；
 * - 按 section id 去重（同 ID 重复行不双计）；草稿勾选不扣减（调用方不传已选集）。
 */
export function candidatePoolDayTotals(sections: readonly PoolCurveSection[]): Map<string, number | null> {
  const days = new Map<string, { total: number, hasUnknown: boolean, hasIncluded: boolean }>()
  const seen = new Set<string>()
  for (const section of sections) {
    if (!section || seen.has(section.id)) continue
    seen.add(section.id)
    const date = String(section.periodDate ?? '').slice(0, 10)
    if (!date) continue
    const kind = poolCountKind(section)
    if (kind === 'exclude') continue
    const entry = days.get(date) ?? { total: 0, hasUnknown: false, hasIncluded: false }
    if (kind === 'unknown') {
      entry.hasUnknown = true
      days.set(date, entry)
      continue
    }
    entry.hasIncluded = true
    if (typeof section.clueCount === 'number' && Number.isFinite(section.clueCount)) {
      entry.total += section.clueCount
    } else {
      entry.hasUnknown = true
    }
    days.set(date, entry)
  }
  const out = new Map<string, number | null>()
  for (const [date, entry] of days) {
    out.set(date, entry.hasUnknown ? null : entry.total)
  }
  return out
}

const WORKBENCH_DAY_MS = 86_400_000

function parseUtcDayStrict(value: string): number | null {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return null
  const ts = Date.parse(`${value}T00:00:00Z`)
  if (!Number.isFinite(ts)) return null
  return new Date(ts).toISOString().slice(0, 10) === value ? ts : null
}

/**
 * 工作面自选起止校验（D3/D4 + OQ-2 定案 365）：非法一律报错，不静默纠正、不截断。
 *
 * - 起止必须成对、YYYY-MM-DD 真实日历日、开始不晚于结束；
 * - 闭区间最多 `maxDays` 天（首末日差 ≤ maxDays − 1）；
 * - `latestCompleted` 给出时，开始不得早于「最新 completed 日 − (maxDays − 1)」
 *   （取数下界，与后端 `parseComposeWindowParams` 同口径）。
 */
export function validateWorkbenchRange(
  from: string,
  to: string,
  maxDays: number,
  latestCompleted?: string | null,
): string | null {
  const span = Math.max(1, Math.floor(maxDays) || 1)
  const fromTs = parseUtcDayStrict(from)
  const toTs = parseUtcDayStrict(to)
  if (fromTs === null || toTs === null) return '请选择有效的起止日期'
  if (fromTs > toTs) return '开始日期不能晚于结束日期'
  if (Math.round((toTs - fromTs) / WORKBENCH_DAY_MS) > span - 1) {
    return `日期范围不能超过 ${span} 天`
  }
  if (latestCompleted) {
    const latestTs = parseUtcDayStrict(latestCompleted)
    if (latestTs !== null) {
      const earliest = latestTs - (span - 1) * WORKBENCH_DAY_MS
      if (fromTs < earliest) {
        return `开始日期早于支持的取数窗（${new Date(earliest).toISOString().slice(0, 10)} 起）`
      }
    }
  }
  return null
}
