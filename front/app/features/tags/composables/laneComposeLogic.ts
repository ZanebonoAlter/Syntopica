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
  /**
   * 标题框上缘的显式位置（px）：给了就按它落位，不再用 `y + r + 5`。
   *
   * 编排画布的常显节点标题要固定「点下 14px 基线」，不能跟着点半径浮动；
   * 缺省（不传）保持 `y + r + 5`，既有相关标题口径逐位不变。
   */
  boxTop?: number
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
  /** 已经占位、不再参与避让的矩形（如常显的节点标题；只读，不回写）。 */
  avoid?: readonly LaneBox[]
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

/** 「节点标题」落位参数（在通用避让参数上补错行与动态障碍）。 */
export interface LaneRelatedTitleOptions extends LanePlaceLabelsOptions {
  /** 向下错行的行数上限（0 = 只放正下方）。 */
  lines?: number
  /** 每次错行下移的像素。 */
  lineHeight?: number
  /**
   * 每个候选的**动态障碍**（如「画布上所有节点的圆」）：与 `avoid` 一起参与避让。
   *
   * **包含候选点自身的框会被跳过**——调用方因此不必为每个候选减掉它自己的圆
   * （节点标题本来就贴着该点下缘，不跳过会永远撞自己、全被挤到下一行）。
   */
  obstaclesOf?: (c: LaneLabelCandidate) => readonly LaneBox[]
}

/** 框是否包含候选点自身（= 这个框就是该点自己的圆，不该挡自己的标题）。 */
function boxContainsPoint(box: LaneBox, point: { x: number, y: number }): boolean {
  return point.x > box.x && point.x < box.x + box.w && point.y > box.y && point.y < box.y + box.h
}

/**
 * 节点标题的落位：**只放节点下方**，撞上已放标题时**向下错行**（最多 `lines` 行），
 * 仍撞上才跳过（宁缺勿叠）；越出画布边界直接跳过。
 *
 * 与 `placeLabels` 的差别：`placeLabels` 是原型口径（四方向、不重叠即放弃），
 * 本函数服务「节点标题」——节点常落在相邻日期列，逐点标题本就会互相压，
 * 向下错一行既保住信息又不压字。
 */
export function placeRelatedTitles(
  cands: LaneLabelCandidate[],
  opts: LaneRelatedTitleOptions = {},
): LanePlacedLabel[] {
  const lines = opts.lines ?? 2
  const lineHeight = opts.lineHeight ?? 16
  const gap = opts.gap ?? 0
  const placed: LaneBox[] = [...(opts.avoid ?? [])]
  const out: LanePlacedLabel[] = []
  const minX = opts.minX ?? -Infinity
  const maxX = opts.maxX ?? Infinity
  const minY = opts.minY ?? -Infinity
  const maxY = opts.maxY ?? Infinity
  for (const c of [...cands].sort((a, b) => b.priority - a.priority || String(a.id).localeCompare(String(b.id)))) {
    const obstacles = opts.obstaclesOf?.(c) ?? []
    let done = false
    for (let line = 0; line <= lines; line++) {
      const box: LaneBox = {
        x: c.x - c.w / 2,
        y: (c.boxTop ?? c.y + c.r + 5) + line * lineHeight,
        w: c.w,
        h: c.h,
      }
      if (box.x < minX || box.x + box.w > maxX || box.y < minY || box.y + box.h > maxY) continue
      const probe: LaneBox = { x: box.x - gap, y: box.y - gap, w: box.w + 2 * gap, h: box.h + 2 * gap }
      const hit = placed.some(p => boxesOverlap(probe, p))
        // 动态障碍：跳过「就是该点自己的那个圆」，其余一律避让
        || obstacles.some(o => !boxContainsPoint(o, c) && boxesOverlap(probe, o))
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
 * 客户端坐标 → 画布**内容坐标**（解除横向/纵向滚动与缩放）。
 *
 * - `clientX/clientY`：指针的视口坐标；
 * - `rect`：滚动容器的 `getBoundingClientRect()`（只用 left / top）；
 * - `scroll`：滚动容器的 `scrollLeft` / `scrollTop`；
 * - `zoom`：画布缩放比例（SVG 的 viewBox 缩放；非正数按 1 兜底）。
 *
 * 返回值与 SVG 的 viewBox 同一坐标系（1:1 内容像素）——落点判定、行位计算都用它。
 * 两画布共用这一份换算，避免各自漂移。
 */
export function canvasContentPoint(
  clientX: number,
  clientY: number,
  rect: { left: number, top: number },
  scroll: { left: number, top: number },
  zoom: number,
): { x: number, y: number } {
  const z = Number.isFinite(zoom) && zoom > 0 ? zoom : 1
  return {
    x: (clientX - rect.left + scroll.left) / z,
    y: (clientY - rect.top + scroll.top) / z,
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
  /**
   * 散料（未选、未被推荐）固定半径覆盖；缺省用 `min(8, baseRadius × 0.8)`。
   *
   * 只给「按自己的布局密度固定点尺寸」的画布用（编排画布提档：散料 11px 直径），
   * 不影响推荐点的接近度映射。
   */
  scatterRadius?: number
  /** 只读参照固定半径覆盖（缺省 `baseRadius × 0.65`，下限 2.5）。 */
  readonlyRadius?: number
  /**
   * 推荐点半径区间覆盖（**显式传入才生效**）：按接近度线性插值 `min → max`。
   *
   * 编排画布用它把「接近度 → 大小」拉到肉眼可见（5.9–7.4 = 直径 11.8–14.8）；
   * 缺省仍走 cap 档位公式（修正画布逐位不变）。区间不突破成员半径——
   * 「推荐 ≤ 已选」不变量由 `min(memberBase, …)` 兜住。
   */
  recommendRange?: { min: number, max: number }
  /** 接近度 → 填色强度（`--proximity-fill` 百分比）区间覆盖；缺省 12–67。 */
  strengthRange?: { min: number, max: number }
  /**
   * 接近度**显式覆盖**（编排画布在渲染批次内做相对归一，见 `relativeProximities`）。
   *
   * 显式传入且为有限数时直接替换「`1 − 距离 / 阈值`」的绝对换算；缺省（不传）走旧公式，
   * 修正画布 `LaneCorrectionCanvas` 逐位不变。
   */
  proximityOverride?: number
}

/** 区间参数兜底：两端非有限数视为未传（不把 NaN 半径带进渲染）。 */
function finiteRange(range?: { min: number, max: number }): { min: number, max: number } | null {
  if (!range || !Number.isFinite(range.min) || !Number.isFinite(range.max)) return null
  return range
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

/**
 * 按**可用宽度**截断并补省略号（CJK 按 1 个字宽、ASCII 按 0.55 个字宽估算）。
 *
 * 为什么不用 `truncateText(text, 字数)`：中英混排同一字数上限差别巨大——
 * `Gemini 4.5` 只占 6 个字宽，却和 10 个汉字占一样的字数配额，结果是 "Gemini 4 …"
 * 明明还有半列空白却截断了。宽度口径让两种文字都把列宽用满。
 */
export function truncateByWidth(text: string, maxWidth: number, size: number): string {
  const s = String(text ?? '')
  if (maxWidth <= 0 || !Number.isFinite(maxWidth)) return ''
  if (estimateTextWidth(s, size) <= maxWidth) return s
  const ellipsis = '…'
  const budget = maxWidth - estimateTextWidth(ellipsis, size)
  if (budget <= 0) return ellipsis
  let out = ''
  for (const ch of s) {
    if (estimateTextWidth(out + ch, size) > budget) break
    out += ch
  }
  return out ? out + ellipsis : ellipsis
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
 * - 选中只由「实心 + ✓」表达，不用大小或深浅暗示；
 * - `flags.memberRadius / scatterRadius / readonlyRadius` 是**按画布布局密度固定尺寸**的
 *   覆盖开关，缺省值保持原公式（修正画布行为不变）；
 * - `flags.recommendRange / strengthRange` 是**显式传入才生效**的区间覆盖：把「接近度 →
 *   半径 / 深浅」拉成肉眼可见的跨度（旧公式在编排画布的推荐半径只差 0.43px）；
 * - `flags.proximityOverride` 是**接近度本身**的覆盖（相对归一口径）：给了就不再看
 *   `distance / threshold`——绝对换算在距离与阈值同量级时只落在 0.04–0.14，肉眼等于常量。
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
  const override = !readonly && Number.isFinite(flags.proximityOverride)
    ? Math.max(0, Math.min(1, flags.proximityOverride!))
    : null
  const proximity = override !== null
    ? override
    : !readonly && Number.isFinite(distance) && Number.isFinite(threshold) && threshold > 0 && distance! <= threshold
      ? Math.max(0, Math.min(1, 1 - distance! / threshold))
      : null
  const ordinaryFallback = readonly ? Math.max(2.5, baseRadius * 0.65) : Math.max(3, Math.min(8, baseRadius * 0.8))
  const ordinaryOverride = readonly ? flags.readonlyRadius : flags.scatterRadius
  const ordinary = Number.isFinite(ordinaryOverride) && ordinaryOverride! > 0
    ? Math.max(readonly ? 2.5 : 3, ordinaryOverride!)
    : ordinaryFallback
  // 成员基准半径**不读接近度**（尺寸不编码成员远近）；memberRadius 允许修正画布
  // 按自己的布局密度给固定尺寸（缺省用 cap 档位公式，编排画布视觉不变）。
  const memberBase = Number.isFinite(flags.memberRadius) && flags.memberRadius! > 0
    ? Math.max(3, flags.memberRadius!)
    : defaultMemberRadius(columnWidth)
  // 推荐点半径恒小于已选点半径：大小只表达「接近程度」，选中只由实心＋✓表达。
  // memberRadius 覆盖时同步压 recommendCap（否则固定小尺寸成员会被大候选压过）。
  const recommendCap = Math.min(cap * 0.72, memberBase - 0.5)
  const recommendRange = finiteRange(flags.recommendRange)
  const recommend = proximity === null
    ? ordinary
    : recommendRange
      ? Math.max(3, Math.min(memberBase, recommendRange.min + (recommendRange.max - recommendRange.min) * proximity))
      : Math.max(3, Math.min(recommendCap, baseRadius * (0.9 + 1.0 * proximity)))
  const strengthRange = finiteRange(flags.strengthRange)
  const radius = selected ? memberBase : recommend
  return {
    radius,
    proximity,
    strength: proximity === null
      ? 0
      : strengthRange
        ? strengthRange.min + (strengthRange.max - strengthRange.min) * proximity
        : 12 + 55 * proximity,
    solid: selected,
    removed: Boolean(flags.removed),
    anchor: Boolean(flags.anchor),
    focus: Boolean(flags.focus),
    related: Boolean(flags.related),
    dimmed: Boolean(flags.dimmed),
  }
}

/**
 * 接近度**相对归一**（渲染批次内 min-max）：最近的 = 1、最远的 = 0。
 *
 * 为什么不用「`1 − 距离 / 阈值`」：实测一批数据的质心距离是 0.59–0.67、阈值也同量级，
 * 绝对除法把整批压到 **0.04–0.14**（半径 5.9→6.0、填充 18%→21%），肉眼等于常量。
 * 阈值只负责「谁进可补充的事件列表」与散料可见性门控，**不参与**这张视觉映射。
 *
 * 口径：
 * - 输入 = **本次渲染批次里全部**非已选、非只读且有可算距离的点（含候选链成员与散料，
 *   超阈值的也在——否则阈值又漏回视觉映射里）；
 * - `proximity = (max − d) / (max − min)`
 * - 边界：只有 1 个点或 `max === min`（全部等距）→ **0.5**（不把所有人推到 0 或 1）；
 * - 距离 null / 非有限 → 不进批次、也不出现在结果里（维持 ordinary 外观，不伪造）；
 * - 没有任何有效距离 → 空表（无质心时全画布退回流料视觉效果）。
 */
export function relativeProximities(distances: ReadonlyMap<string, number>): Map<string, number> {
  const out = new Map<string, number>()
  if (!distances.size) return out
  let min = Number.POSITIVE_INFINITY
  let max = Number.NEGATIVE_INFINITY
  let count = 0
  for (const d of distances.values()) {
    if (!Number.isFinite(d)) continue
    count += 1
    if (d < min) min = d
    if (d > max) max = d
  }
  if (!count) return out
  const span = max - min
  for (const [id, d] of distances) {
    if (!Number.isFinite(d)) continue
    out.set(id, span > 0 ? (max - d) / span : 0.5)
  }
  return out
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

// ── 行式泳道画布（rework-lane-compose-swimlane-canvas D1/D2/D4/D5/D6） ────────
/**
 * 画布纵向语法从「同日内 id 序号（无含义）」改为「**所属候选链**（身份）」：
 *
 * - 第 0 行 = **本次泳道**（用户正在构造的那条，恒定第 0 行，不绑任何候选链）；
 * - 第 1..N 行 = 服务端候选链（**按服务端顺序**，前端不重排、不重算资格）；
 * - 最后一行 = **散料地板**（不属于任何链的节点）。
 *
 * 行归属只从服务端 `chains` 派生（D1/D9）：`rowIndexOf` 不参与任何前端重算，
 * 「本次泳道」也不写进行归属表里的链身份——它只是渲染时的行 0 覆盖。
 */

/** 本次泳道行（恒定第 0 行）。 */
export const LANE_ROW_SELF = 0
/** 行标签区高度（名称 + 计数/跨度）。 */
export const LANE_ROW_LABEL_H = 32
/**
 * 同日堆叠的槽距（24px；2026-11-21 提档时试过 18px，几何上不成立后回到 24）。
 *
 * 下限由「已选点直径 15px + 常显标题（11px，基线在点下 14px）」共同决定：
 * 一槽的标题下缘 ≈ y+15，下一槽的圆顶边 = y+slot−7.5 → slot ≥ 22.5 才不重叠，
 * 取 24（= 原值）。**不要再调小**，否则标题会被下一个点压住。
 */
export const LANE_SLOT_H = 24
/** 行间留白（含 1px 分隔线）。 */
export const LANE_ROW_GAP = 26
/** 单日封顶槽数：超过则不无限拉高，折算成 `+N`（可点开该日全部）。 */
export const MAX_SLOT_PER_DAY = 6
/** 常显截断标题的字数上限（列宽足够时也不超过这个数）。 */
export const MAX_TITLE_CHARS = 10
/**
 * 标题档开启的列宽下限（= 时间列宽下限，124px）。
 *
 * 124px 的由来：11px 字号下 10 个字 + 省略号 ≈ 11 × 11 = 121px，留 3px 余量。
 * 2026-11-23 用户定档：原来 5 字（52px 列）在 30 天窗口只剩 3 字 + 省略号，看不出是哪件事。
 */
export const TITLE_MIN_COLUMN = 124
/**
 * 时间列宽下限（px）：低于它 11px 字号放不下 10 字。
 *
 * 天数多到均分不够时**宁可把内容撑宽让人横向滚动，也不缩字**（与修正画布同一原则）。
 */
export const LANE_MIN_COLUMN = TITLE_MIN_COLUMN
/** 11px 字号下每个字的均分宽度（`titleBudget` 的字宽除数）。 */
export const TITLE_CHAR_WIDTH = 11.5

/** 候选链行的行序表（只读派生，不重算候选资格）。 */
export interface LaneChainRow {
  /** 服务端候选链 key。 */
  key: string
  label: string
  /** 行号（1 起；0 留给本次泳道）。 */
  row: number
  /** 该链在窗口内的成员 id（保持服务端顺序）。 */
  ids: string[]
}

/** 行归属索引：`rowOf` 是唯一被渲染消费的映射。 */
export interface LaneRowIndex {
  /** sectionId → 行号（0 = 本次泳道，1..N = 候选链序，floorRow = 地板）。 */
  rowOf: Map<string, number>
  chainRows: LaneChainRow[]
  /** 地板行号（= 候选链数 + 1）。 */
  floorRow: number
}

/**
 * 把服务端 `chains` 展开成「成员 → 行号」映射（D1）。
 *
 * - 同一链的多个成员同属一行；重复出现在多条链的成员取**第一条**链（服务端顺序）；
 * - 不在任何链里的窗口内节点 → 地板行；
 * - `laneIds`（本次泳道成员）**优先级最高**：命中即归第 0 行（D9，渲染覆盖，
 *   不改链的身份，也不产生第二份真值）；
 * - 链成员若不在窗口内（`sectionIds` 未包含）→ 忽略，不产脏行。
 */
export function buildRowIndex(
  chains: readonly { key: string, topicLabel?: string, sectionIds: readonly string[] }[],
  sectionIds: Iterable<string>,
  laneIds: ReadonlySet<string> = new Set<string>(),
): LaneRowIndex {
  const rowOf = new Map<string, number>()
  const chainRows: LaneChainRow[] = []
  const windowIds = new Set<string>()
  for (const id of sectionIds) windowIds.add(String(id))
  let row = 0
  for (const chain of chains) {
    row += 1
    const ids: string[] = []
    for (const raw of chain.sectionIds ?? []) {
      const id = String(raw ?? '')
      if (!id || !windowIds.has(id)) continue
      if (rowOf.has(id)) continue
      rowOf.set(id, row)
      ids.push(id)
    }
    chainRows.push({ key: chain.key, label: chain.topicLabel ?? chain.key, row, ids })
  }
  const floorRow = row + 1
  for (const id of windowIds) {
    if (!rowOf.has(id)) rowOf.set(id, floorRow)
  }
  for (const id of laneIds) {
    if (rowOf.has(id)) rowOf.set(id, LANE_ROW_SELF)
  }
  return { rowOf, chainRows, floorRow }
}

/** 行内成员（画布布局只需要 id 与日期列）。 */
export interface LaneRowMember {
  id: string
  /** YYYY-MM-DD。 */
  date: string
}

/** 单行几何（D2）：行标签 + 槽位 + 行间留白。 */
export interface LaneRowLayout {
  row: number
  top: number
  height: number
  /** 该行占用的槽数（≥ `minSlots`）。 */
  slots: number
  /** 已落槽成员 → 槽位序号（0 起）；被单日封顶收起的成员不在表里。 */
  slotOf: Map<string, number>
  /** 每日被封顶收起的条数（只含 >0 的日期）。 */
  overflowByDate: Map<string, number>
  /** 被单日封顶收起的成员 id（展开该日后才会落槽）。 */
  hiddenIds: string[]
}

export interface LaneRowLayoutOptions {
  /** 第一行的 top（px）。 */
  top?: number
  labelHeight?: number
  slotHeight?: number
  rowGap?: number
  maxSlotPerDay?: number
  /** 行序（含需要占位的空行）；缺省取 `membersByRow` 的行号升序。 */
  rowOrder?: readonly number[]
  /** 每行**至少**占用的槽数（本次泳道行预留落点）。 */
  minSlots?: ReadonlyMap<number, number> | number
  /** 已展开的 `${row}:${date}`：展开后该日不再封顶。 */
  expanded?: ReadonlySet<string>
}

/** 行号 → 该行成员（缺行 = 空行）。 */
export type LaneRowMembers = ReadonlyMap<number, readonly LaneRowMember[]>

/** 成员在行内的 y（槽中心）。 */
export function rowMemberY(layout: Pick<LaneRowLayout, 'top' | 'slots'>, slot: number, opts: Pick<LaneRowLayoutOptions, 'labelHeight' | 'slotHeight'> = {}): number {
  const labelHeight = opts.labelHeight ?? LANE_ROW_LABEL_H
  const slotHeight = opts.slotHeight ?? LANE_SLOT_H
  return layout.top + labelHeight + slot * slotHeight + slotHeight / 2
}

/**
 * 行几何推导（D2）：行高 = 行标签 32 + 当日最大槽数 × 18 + 行间 26。
 *
 * - 同日按 `slot` 竖直下叠，**不折叠成计数点**（同一天多条要各自可读）；
 * - 单日超过 `maxSlotPerDay`（默认 6）封顶，超出部分进 `overflowByDate`/`hiddenIds`，
 *   由画布渲染可点的 `+N`（`expanded` 命中该日则不再封顶）；
 * - 行集合 = `rowOrder`（显式行序）∪ `membersByRow` 的行号；链被取空的行不传进来即可
 *   自然消失（空行不渲染）；
 * - 时间窗口外的成员直接跳过（不画到别的日期列上）。
 */
export function layoutRows(
  rowIndex: ReadonlyMap<string, number>,
  dates: readonly string[],
  membersByRow: LaneRowMembers,
  opts: LaneRowLayoutOptions = {},
): LaneRowLayout[] {
  const labelHeight = opts.labelHeight ?? LANE_ROW_LABEL_H
  const slotHeight = opts.slotHeight ?? LANE_SLOT_H
  const rowGap = opts.rowGap ?? LANE_ROW_GAP
  const cap = Math.max(1, Math.floor(opts.maxSlotPerDay ?? MAX_SLOT_PER_DAY))
  const expanded = opts.expanded ?? new Set<string>()
  const minSlotsOf = (row: number): number => {
    if (typeof opts.minSlots === 'number') return Math.max(0, Math.floor(opts.minSlots))
    return Math.max(0, Math.floor(opts.minSlots?.get(row) ?? 0))
  }
  const dayOrder = dates.map(date => String(date ?? '').slice(0, 10)).filter(Boolean)
  const daySet = new Set(dayOrder)
  // 行集合：显式行序优先（画布据此让取空的链行消失）；未给时取「成员行 ∪ 行归属表」。
  const rows = new Set<number>(opts.rowOrder ?? [])
  if (!opts.rowOrder) {
    for (const row of rowIndex.values()) rows.add(row)
  }
  for (const row of membersByRow.keys()) rows.add(row)
  const ordered = [...rows].filter(row => Number.isFinite(row)).sort((a, b) => a - b)
  const out: LaneRowLayout[] = []
  let cursor = opts.top ?? 0
  for (const row of ordered) {
    const byDate = new Map<string, string[]>()
    for (const member of membersByRow.get(row) ?? []) {
      const id = String(member?.id ?? '')
      const date = String(member?.date ?? '').slice(0, 10)
      if (!id || !daySet.has(date)) continue
      const list = byDate.get(date) ?? []
      list.push(id)
      byDate.set(date, list)
    }
    const slotOf = new Map<string, number>()
    const overflowByDate = new Map<string, number>()
    const hiddenIds: string[] = []
    let slots = 0
    for (const date of dayOrder) {
      const ids = byDate.get(date)
      if (!ids?.length) continue
      ids.sort(compareIds)
      const limit = expanded.has(`${row}:${date}`) ? ids.length : cap
      ids.forEach((id, index) => {
        if (index < limit) slotOf.set(id, index)
        else {
          hiddenIds.push(id)
          overflowByDate.set(date, (overflowByDate.get(date) ?? 0) + 1)
        }
      })
      slots = Math.max(slots, Math.min(ids.length, limit))
    }
    slots = Math.max(slots, minSlotsOf(row))
    const height = labelHeight + slots * slotHeight + rowGap
    out.push({ row, top: cursor, height, slots, slotOf, overflowByDate, hiddenIds })
    cursor += height
  }
  return out
}

/**
 * 节点常显截断标题的预算（D6）：列宽 < 124px → 整档关闭（null）；
 * 否则 `floor(columnWidth / 11.5)` 字（11px 字号档），且不超过 10 字。
 *
 * 不做「按列宽线性缩短到 2 字」的碎片降级——2 字标题的信息量接近 0。
 * 编排画布的时间列宽有 124px 下限，所以常显标题永不因窗口收窄关档。
 */
export function titleBudget(columnWidth: number): number | null {
  if (!Number.isFinite(columnWidth) || columnWidth < TITLE_MIN_COLUMN) return null
  return Math.max(1, Math.min(MAX_TITLE_CHARS, Math.floor(columnWidth / TITLE_CHAR_WIDTH)))
}

/** 散料可见性门控的三档（D4）。 */
export type LaneThresholdTier = 'tight' | 'standard' | 'all'

/** 三档 → 实际余弦距离阈值：收紧 = matchThreshold×0.5、标准 = matchThreshold、全部 = 1。 */
export function thresholdOf(tier: LaneThresholdTier, matchThreshold: number): number {
  const base = Number.isFinite(matchThreshold) && matchThreshold > 0 ? matchThreshold : 0.3
  if (tier === 'tight') return base * 0.5
  if (tier === 'all') return 1
  return base
}

/** 散料可见集与折叠计数（「另有 N 条未展开」的来源）。 */
export interface LaneScatterVisibility {
  /** 当前档位实际使用的距离阈值。 */
  threshold: number
  /** 画布上可见的全部 id：已选 ∪ 只读 ∪ 候选链成员 ∪ 达标的散料。 */
  visibleIds: Set<string>
  /** 当前档位可见的散料 id。 */
  visibleScatterIds: Set<string>
  /** 被阈值收起（未达标）的散料 id —— 折叠计数只数这批。 */
  hiddenScatterIds: string[]
}

/**
 * 散料可见集（D4）——阈值**只门控散料**，候选链成员 / 已选 / 只读参照走白名单旁路，
 * 永不因阈值被隐藏。
 *
 * - 散料 = 不属于任何候选链、不在只读参照里、未被选中的窗口内节点；
 * - 达标 = `1 − cos(center, embedding) ≤ thresholdOf(tier)`；无质心（已选为空）时
 *   除「全部」档外一律不可判定 → 全部收起（不伪造达标）；
 * - `all` 档不看质心（全部散料可见），距离缺失时按弱化空心渲染。
 */
export function visibleScatter<T extends LaneSection>(
  sections: readonly T[],
  selectedIds: ReadonlySet<string>,
  candidateIds: ReadonlySet<string>,
  readonlyIds: ReadonlySet<string>,
  center: number[] | null,
  tier: LaneThresholdTier,
  matchThreshold: number,
): LaneScatterVisibility {
  const threshold = thresholdOf(tier, matchThreshold)
  const visibleIds = new Set<string>()
  const visibleScatterIds = new Set<string>()
  const hiddenScatterIds: string[] = []
  for (const section of sections) {
    const id = String(section?.id ?? '')
    if (!id) continue
    if (selectedIds.has(id) || readonlyIds.has(id) || candidateIds.has(id)) {
      visibleIds.add(id)
      continue
    }
    const distance = center && tier !== 'all' ? cosineFor(section.embedding, center) : null
    const pass = tier === 'all' || (distance !== null && Number.isFinite(distance) && distance <= threshold)
    if (pass) {
      visibleIds.add(id)
      visibleScatterIds.add(id)
    } else {
      hiddenScatterIds.push(id)
    }
  }
  hiddenScatterIds.sort(compareIds)
  return { threshold, visibleIds, visibleScatterIds, hiddenScatterIds }
}

/**
 * 检索范围 = **未归类**（D5）：不属于任何候选链的 section。
 *
 * 用「不在任何链成员集合里」派生，**不是** `persistentTopicId == null`——否则
 * 挂着 candidate 话题但未成链的节点会被漏掉。
 *
 * 只读参照（`status === 'active'`）不计入：它属于正式泳道，不是「未归类」，
 * 也不可被选入本次草稿（画布默认不画它，命中它会造成「右栏有、画布没有」）。
 */
export function unclassifiedIds(
  sections: readonly { id: string, persistentTopic?: { status?: string } | null }[],
  chains: readonly { sectionIds: readonly string[] }[],
): Set<string> {
  const chained = new Set<string>()
  for (const chain of chains) {
    for (const raw of chain.sectionIds ?? []) {
      const id = String(raw ?? '')
      if (id) chained.add(id)
    }
  }
  const out = new Set<string>()
  for (const section of sections) {
    const id = String(section?.id ?? '')
    if (!id || chained.has(id)) continue
    if (section.persistentTopic?.status === 'active') continue
    out.add(id)
  }
  return out
}
