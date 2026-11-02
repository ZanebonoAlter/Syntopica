/**
 * 体检分诊 normalizer（board-health-triage-advice seam C1 / task 3.1）。
 *
 * 后端 `GET /semantic-boards/:id/health-checkup` 的 snake_case wire 协议**不改**；
 * 本模块只在 API 边界把同轮快照的 triage / magnet / neighbors / desc_flags
 * 归一为 camelCase + 字符串 ID 视图（code-style §9：转换只在 API/store 层）。
 *
 * 口径纪律（红线 22/红线 3）：
 *  - 前端**不复制**任何病型/统计公式——磁铁/最近邻/词根一律搬运服务端信号；
 *  - 分诊只是建议：非法分类 / 非法 aux 引用 / 重复记录 / 无证据一律**不作为
 *    可执行建议**透出，不猜 rewrite/remove、不伪造证据；
 *  - keep/insufficient 是只读结论，永不携带可执行建议；
 *  - 构成清单混有组合标签（composite）：`label_type` 逐条透传，**非 auxiliary 的
 *    rewrite 项不可执行**（描述修复链只覆盖辅助标签 → 没有修复入口），证据照常
 *    展示、不隐藏、不当它健康；remove 项不受类型影响；类型未知（旧快照缺字段）
 *    同样按不可执行处理，不臆造类型；
 *  - 历史快照缺 triage 字段 = 旧形态（available=false），不拿现有统计反推诊断。
 */
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'

/** 分诊四类（后端闭集；非法值不落此类型）。 */
export type BoardHealthTriageCategory = 'remove' | 'rewrite' | 'keep' | 'insufficient'

const TRIAGE_CATEGORIES: readonly string[] = ['remove', 'rewrite', 'keep', 'insufficient']

/** 分诊项只读视图（字符串 auxId；可执行性由后端证据决定，不在前端重判）。 */
export interface BoardHealthTriageItemView {
  auxId: string
  category: BoardHealthTriageCategory
  evidence: string
  /** 建议文案：仅 rewrite 语义持有；其它类别恒为空串。 */
  suggestion: string
  /** 可执行动作：remove/rewrite 且证据齐备（rewrite 还需建议文案）。 */
  actionable: boolean
  /**
   * 构成标签类型（auxiliary | composite；空串 = 未知/旧快照缺字段）。
   *
   * 描述修复链全程只认 auxiliary：组合标签的「重写描述」在服务端没有对应
   * 修复入口（context 404），因此这类 rewrite 项一律**不可执行**（证据仍
   * 展示，不隐藏、不当它健康）。摘除不受影响（走既有 DELETE 链）。
   */
  labelType: string
}

/** 本轮分诊整体视图（available=false = 历史快照/无分诊，按旧形态展示）。 */
export interface BoardHealthTriageView {
  /** 快照是否携带分诊字段（false = 旧形态，不展示问题清单）。 */
  available: boolean
  /** 合法可展示项（非法项已剔除，不透出为可执行建议）。 */
  items: BoardHealthTriageItemView[]
  /** 本轮被剔除的非法项数量（供上层提示"分诊数据异常"，不逐条展示）。 */
  dropped: number
}

/** 磁铁信号只读视图（服务端口径，前端不重算 gap）。 */
export interface BoardHealthMagnetView {
  allMean: number | null
  failedMean: number | null
  allSamples: number
  failedSamples: number
  suspicious: boolean
  insufficient: boolean
}

/** 最近邻证据只读视图（字符串 auxId）。 */
export interface BoardHealthNeighborView {
  auxId: string
  label: string
  cosine: number
}

/** 标签级分诊信号只读视图（磁铁/最近邻/描述预筛 + 描述快照）。 */
export interface BoardHealthLabelSignalView {
  auxId: string
  label: string
  /** 体检时刻的描述快照（病C 证据源；可能为空串）。 */
  description: string
  magnet: BoardHealthMagnetView | null
  /** null = 自身向量缺失（信号不足）；空数组 = 已算但空间内无其它标签。 */
  neighbors: BoardHealthNeighborView[] | null
  /** 命中的文章式词根（空 = 未命中）。 */
  descFlags: string[]
}

/**
 * 失败挂载主要流向只读视图。
 *
 * `topBoard` 为 null = 服务端未能给出**可靠**流向（失败项在其他版块上的挂载本身也失败）
 * → 显式未知，不臆造目标版块；前端**不**重算 top1、不判断"可靠命中"
 * （口径唯一实现于 boardhealth 包）。
 */
export interface BoardHealthMagnetFlowView {
  /** 计入的失败挂载数。 */
  failedTags: number
  /** 在其他有效版块上有挂载的失败 tag 数（可靠流向的样本）。 */
  flowTags: number
  /** 主要流向版块标签；null = 未知。 */
  topBoard: string | null
  /** 流向该版块的失败 tag 数。 */
  topTags: number
}

function str(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function num(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function optNum(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

/**
 * 归一化本轮分诊清单。
 *
 * 逐项校验：aux 引用必须是本轮构成标签（metrics.labels）、分类必须在闭集内、
 * 证据非空、同标签不得重复。
 *
 * **同标签分组规则（一致才去重）**：按 aux 分组后，
 *  - 组内条目在 category/evidence/suggestion 上**完全一致** → 视为重复记录，
 *    保留一条（其余计入 dropped）；
 *  - 组内条目存在任何**矛盾**（分类不同、或证据/建议文案不同）→ 整组不可执行：
 *    全部丢弃并按组内条目数计入 dropped（不保留任何可执行项、不猜哪个对）。
 * 丢弃的条目一律不透出为可执行建议；其余合法条目照常保留。
 */
export function normalizeBoardHealthTriage(snapshot: BoardHealthSnapshotDTO | null): BoardHealthTriageView {
  if (!snapshot) return { available: false, items: [], dropped: 0 }
  const raw = snapshot.metrics?.triage
  if (!Array.isArray(raw)) return { available: false, items: [], dropped: 0 }

  // 分诊只能指向本轮构成标签：metrics.labels 是构成标签全集（服务端同轮落库）。
  // 同时记下每个标签的 label_type——修复链只认 auxiliary（4.1 M1）。
  const known = new Set<number>()
  const labelTypeByID = new Map<number, string>()
  for (const label of snapshot.metrics?.labels ?? []) {
    if (typeof label?.aux_id !== 'number') continue
    known.add(label.aux_id)
    labelTypeByID.set(label.aux_id, str(label?.label_type).trim())
  }

  interface Group {
    auxId: number
    category: string
    evidence: string
    suggestion: string
    size: number
    consistent: boolean
  }
  const groups = new Map<number, Group>()
  let dropped = 0

  for (const entry of raw) {
    const auxId = entry?.aux_id
    if (typeof auxId !== 'number' || !Number.isFinite(auxId) || !known.has(auxId)) {
      // 非法 aux 引用（不在本轮构成标签里）：不落任何组，直接计丢弃。
      dropped++
      continue
    }
    const category = str(entry?.category)
    if (!TRIAGE_CATEGORIES.includes(category)) {
      dropped++
      continue
    }
    const evidence = str(entry?.evidence).trim()
    if (!evidence) {
      dropped++
      continue
    }
    const suggestion = str(entry?.suggestion).trim()

    const existing = groups.get(auxId)
    if (!existing) {
      groups.set(auxId, { auxId, category, evidence, suggestion, size: 1, consistent: true })
      continue
    }
    existing.size += 1
    if (existing.category !== category || existing.evidence !== evidence || existing.suggestion !== suggestion) {
      existing.consistent = false
    }
  }

  const items: BoardHealthTriageItemView[] = []
  for (const group of groups.values()) {
    if (!group.consistent) {
      // 矛盾组：整组不可执行（丢弃，不猜哪个对）。
      dropped += group.size
      continue
    }
    // 一致重复：留一条，其余计丢弃。
    dropped += group.size - 1
    const isRewrite = group.category === 'rewrite'
    const labelType = labelTypeByID.get(group.auxId) ?? ''
    // 组合标签（composite）/类型未知：描述修复链没有对应入口 → 不可执行
    // （只读展示证据，不隐藏、不当它健康）；摘除不受影响（既有 DELETE 链）。
    const repairSupported = labelType === 'auxiliary'
    items.push({
      auxId: String(group.auxId),
      category: group.category as BoardHealthTriageCategory,
      evidence: group.evidence,
      // suggestion 只对 rewrite 有语义；其它类别的多余文案在此清空。
      suggestion: isRewrite ? group.suggestion : '',
      actionable: isRewrite ? (group.suggestion !== '' && repairSupported) : group.category === 'remove',
      labelType,
    })
  }

  return { available: true, items, dropped }
}

/**
 * 归一化失败挂载主要流向（服务端已算好，前端只搬运）。
 *
 * - 快照缺该字段 / 本轮无失败挂载 → null（**未知**，不补 0、不臆造目标）；
 * - `top_board` 为 null → 保留 null（服务端未能给出可靠流向）。
 */
export function normalizeBoardHealthMagnetFlow(snapshot: BoardHealthSnapshotDTO | null): BoardHealthMagnetFlowView | null {
  const flow = snapshot?.metrics?.board?.magnet_flow
  if (!flow) return null
  return {
    failedTags: num(flow.failed_tags),
    flowTags: num(flow.flow_tags),
    topBoard: typeof flow.top_board === 'string' && flow.top_board ? flow.top_board : null,
    topTags: num(flow.top_tags),
  }
}

/**
 * 归一化标签级分诊信号（磁铁 / 最近邻 / 描述事件性预筛 + 描述快照）。
 *
 * 只搬运服务端信号：magnet 的 gap/阈值判定、neighbors 的 top-k、desc_flags 的
 * 词根命中都在 boardhealth 包内，前端不重算（红线 22）。
 */
export function normalizeBoardHealthLabelSignals(snapshot: BoardHealthSnapshotDTO | null): BoardHealthLabelSignalView[] {
  const labels = snapshot?.metrics?.labels
  if (!Array.isArray(labels)) return []

  return labels.map((label) => {
    const magnet = label?.magnet ?? null
    const neighbors = Array.isArray(label?.neighbors) ? label.neighbors : null
    return {
      auxId: String(label?.aux_id ?? ''),
      label: str(label?.label),
      description: str(label?.description),
      magnet: magnet
        ? {
            allMean: optNum(magnet.all_mean),
            failedMean: optNum(magnet.failed_mean),
            allSamples: num(magnet.all_samples),
            failedSamples: num(magnet.failed_samples),
            suspicious: magnet.suspicious === true,
            insufficient: magnet.insufficient === true,
          }
        : null,
      neighbors: neighbors
        ? neighbors.map((neighbor) => ({
            auxId: String(neighbor?.aux_id ?? ''),
            label: str(neighbor?.label),
            cosine: num(neighbor?.cosine),
          }))
        : null,
      descFlags: Array.isArray(label?.desc_flags) ? label.desc_flags.filter((flag): flag is string => typeof flag === 'string') : [],
    }
  })
}
