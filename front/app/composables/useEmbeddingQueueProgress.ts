/**
 * embedding 队列进度 composable（雷达管线状态块的第二环）
 *
 * 数据源：GET /api/poll 对账分发（usePollBundle，client-poll-budget）——
 * 与标签队列**同一个请求**，不新建轮询、不新建端点。
 *
 * 单位是**标签**（embedding_queues 行按 tag_id 排），与标签队列的**文章**
 * 单位不同：日报的 lane 分桶依赖 tag 向量到质心的距离，embedding 未清空会把
 * 文章落进 unmatched 桶（night-window-alignment 约束 23：日报自动生成要求
 * tag_jobs 与 embedding_queues **双队列**清空）。
 *
 * 可见性判据（与标签队列同口径，唯一判据）：pending + processing > 0
 * ——failed 是终态、不计入活跃量，只做红色提示、不阻断生成。
 */

export interface EmbeddingQueueProgressState {
  pending: number
  processing: number
  completedToday: number
  failed: number
}

export function useEmbeddingQueueProgress() {
  const status = useState<EmbeddingQueueProgressState>('embedding-queue-progress:status', () => ({
    pending: 0,
    processing: 0,
    completedToday: 0,
    failed: 0,
  }))
  /**
   * 对账至少成功过一次**且响应里带 embedding_queue 段**。
   * 旧后端没有该段时保持 false——此时不能断言「embedding 队列已清空」
   * （R-17：没有数据就不产出业务结论），生成门槛按「未知即挡住」处理。
   */
  const ready = useState<boolean>('embedding-queue-progress:ready', () => false)

  /** 活跃待向量化量 = pending + processing（清空判据）。 */
  const activeCount = computed(() => status.value.pending + status.value.processing)
  /** 本轮总量 = 活跃量 + 今日已完成（进度分母）。 */
  const roundTotal = computed(() => activeCount.value + status.value.completedToday)
  const progressPercent = computed(() => {
    if (roundTotal.value === 0) return 0
    return Math.round((status.value.completedToday / roundTotal.value) * 100)
  })
  /** 队列清空（仅在 ready 时有意义）。 */
  const drained = computed(() => activeCount.value === 0)

  return {
    status,
    ready,
    activeCount,
    roundTotal,
    progressPercent,
    drained,
  }
}
