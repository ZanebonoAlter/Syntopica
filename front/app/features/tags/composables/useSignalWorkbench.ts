import { ref, computed, onUnmounted } from "vue";
import { useNotify } from "~/composables/useNotify";
import {
  useBoardSignalsApi,
  type SignalCandidateRow,
  type SignalGranularity,
  type SignalReportDetail,
  type SignalReportRow,
  type SignalResearchProgress,
} from "~/api/boardSignals";
import { validateSignalPeriodFormat } from "../components/signalReport";

/**
 * 板块信号工作台 composable（board-signal-reports 5.2/5.4）。
 *
 * 轮询纪律仿 useBoardEnrichment 的 board 档：串行 setTimeout（上一发返回
 * 才排下一发，3s）、generation+boardId 双重身份守卫（迟到响应静默丢弃）。
 *
 * 核心边界（FE-1）：「发现信号」只发 discovery 一个请求，job 终态后只重拉
 * 候选/报告列表——绝不串发任何 research 请求；研究必须用户逐条点击。
 *
 * job 生命周期（phase-2a/2b ②）：
 *  - discovery：202 → 轮询 → finished && outcome=no_signal 安静空态（不清
 *    列表不弹消息）/ discovered 重拉候选 / failed 错误态可重试；
 *  - research：202 → 轮询（恢复原 job 用 409 data 的 running 身份）→
 *    succeeded 用 result_id 打开报告 / failed 候选回待研究可重试；
 *  - 任意 job 404（后端重启丢内存任务）：停轮询 + 提示状态不可恢复可重试
 *    + 重拉候选/报告列表（候选不卡「研究中」——派生状态来自 live job）。
 */
export function useSignalWorkbench() {
  const api = useBoardSignalsApi();
  const { success: notifySuccess, error: notifyError, warn: notifyWarn } = useNotify();

  const POLL_INTERVAL_MS = 3000;
  const MAX_DECISIONS = 40;

  // ── 周期选择（列表间共享，返回列表时保持）─────────────────────────────
  const granularity = ref<SignalGranularity>("month");
  const period = ref<string>("");

  // ── 列表状态 ───────────────────────────────────────────────────────────
  const candidates = ref<SignalCandidateRow[]>([]);
  const candidatesLoading = ref(false);
  const candidatesError = ref<string | null>(null);
  const reports = ref<SignalReportRow[]>([]);
  const reportsLoading = ref(false);
  const reportsError = ref<string | null>(null);

  // ── discovery 任务态（同板块同时最多一个 discovery job）────────────────
  const discoveryRunning = ref(false);
  const discoveryPhase = ref<string | null>(null);
  const discoveryError = ref<string | null>(null);

  // ── research 任务态（只追踪当前选中的候选；其他候选绝不跟跑）──────────
  const researchCandidateId = ref<number | null>(null);
  const researchPhase = ref<string | null>(null);
  const researchError = ref<string | null>(null);
  const researchJobId = ref<string | null>(null);
  // 实时研究进展（tasks 4.8，实现 ui-design「研究进度|显示决策n/40、取数x、
  // 计算y」契约）：进行中每轮轮询顺带拉取；失败/404 终态保留供全程回看；
  // 成功清空（报告 appendix 接管每步链路展示）。
  const researchProgress = ref<SignalResearchProgress | null>(null);

  // ── 报告阅读视图 ───────────────────────────────────────────────────────
  const activeReportId = ref<number | null>(null);
  const activeReport = ref<SignalReportDetail | null>(null);
  const activeReportLoading = ref(false);
  const activeReportError = ref<string | null>(null);

  // ── 视图守卫（切板块隔离：epoch+boardId，迟到响应全数丢弃）────────────
  let viewBoardId: number | null = null;
  let viewEpoch = 0;
  let discoveryPollGen = 0;
  let researchPollGen = 0;
  let discoveryPollTimer: ReturnType<typeof setTimeout> | null = null;
  let researchPollTimer: ReturnType<typeof setTimeout> | null = null;

  function stillCurrent(epoch: number, boardId: number): boolean {
    return epoch === viewEpoch && boardId === viewBoardId;
  }

  /** 首次直接使用（未经 setBoard）时绑定板块；此后只有 setBoard 可重绑。 */
  function bindBoard(boardId: number) {
    if (viewBoardId === null) viewBoardId = boardId;
  }

  /** 切板块/重挂载入口：停全部轮询、epoch++ 使在途请求失效、重置任务态。 */
  function setBoard(boardId: number) {
    viewEpoch++;
    viewBoardId = boardId;
    stopDiscoveryPoll();
    stopResearchPoll();
    candidates.value = [];
    candidatesError.value = null;
    reports.value = [];
    reportsError.value = null;
    discoveryError.value = null;
    discoveryPhase.value = null;
    researchError.value = null;
    researchPhase.value = null;
    researchCandidateId.value = null;
    researchJobId.value = null;
    researchProgress.value = null;
    closeReport();
  }

  function stopDiscoveryPoll() {
    discoveryPollGen++;
    if (discoveryPollTimer) {
      clearTimeout(discoveryPollTimer);
      discoveryPollTimer = null;
    }
    discoveryRunning.value = false;
    discoveryPhase.value = null;
  }

  function stopResearchPoll() {
    researchPollGen++;
    if (researchPollTimer) {
      clearTimeout(researchPollTimer);
      researchPollTimer = null;
    }
    researchCandidateId.value = null;
    researchPhase.value = null;
    researchJobId.value = null;
  }

  // ── 列表加载 ───────────────────────────────────────────────────────────

  async function loadCandidates(boardId: number): Promise<void> {
    bindBoard(boardId);
    if (!period.value) return;
    const epoch = viewEpoch;
    candidatesLoading.value = true;
    candidatesError.value = null;
    try {
      const res = await api.listSignalCandidates(boardId, {
        granularity: granularity.value,
        period: period.value,
      });
      if (!stillCurrent(epoch, boardId)) return; // 迟到响应丢弃
      if (res.success && Array.isArray(res.data)) {
        candidates.value = res.data;
      } else {
        // list error：错误态不伪装无数据
        candidatesError.value = res.error || "候选列表加载失败";
      }
    } finally {
      if (stillCurrent(epoch, boardId)) candidatesLoading.value = false;
    }
  }

  async function loadReports(boardId: number): Promise<void> {
    bindBoard(boardId);
    if (!period.value) return;
    const epoch = viewEpoch;
    reportsLoading.value = true;
    reportsError.value = null;
    try {
      const res = await api.listSignalReports(boardId, {
        granularity: granularity.value,
        period: period.value,
      });
      if (!stillCurrent(epoch, boardId)) return;
      if (res.success && Array.isArray(res.data)) {
        reports.value = res.data;
      } else {
        reportsError.value = res.error || "报告列表加载失败";
      }
    } finally {
      if (stillCurrent(epoch, boardId)) reportsLoading.value = false;
    }
  }

  /** 首次进入/切周期：拉候选 + 报告，并同步 live 任务状态（刷新后自动恢复
   * 研究轮询——后端 analysis-status 是内存真相：running 才接管，进程重启即
   * idle，天然排除进展表被杀残留行；误接管兜底走 job 404「已失效可重试」）。 */
  async function loadPeriod(boardId: number): Promise<void> {
    await Promise.all([loadCandidates(boardId), loadReports(boardId)]);
    await syncSignalStatus(boardId);
  }

  /** 重进恢复（tasks 4.8 补全）：board analysis-status 报 running 的 signal
   * 任务直接接回轮询，刷新页面不再静默丢进度。已有在跟轮询时不重复接管。 */
  async function syncSignalStatus(boardId: number): Promise<void> {
    const epoch = viewEpoch;
    try {
      const res = await api.getBoardAnalysisStatus(boardId);
      if (!stillCurrent(epoch, boardId)) return;
      if (!res.success || !res.data?.running || !res.data.job_id) return;
      const kind = res.data.job_kind;
      if (kind === "board_signal_report" && researchCandidateId.value === null) {
        notifyWarn("研究进行中，已恢复进度显示");
        startResearchPoll(boardId, res.data.job_id, res.data.candidate_id ?? 0, epoch);
      } else if (kind === "board_signal_discovery" && discoveryRunning.value === false) {
        notifyWarn("信号发现进行中，已恢复进度显示");
        startDiscoveryPoll(boardId, res.data.job_id, epoch);
      }
    } catch {
      // 状态查询失败不打断列表加载
    }
  }

  /** 查看候选最近一次研究的全程进展（历史入口，tasks 4.8）：进行中不让查，
   * 防止历史数据覆盖实时轮询数据。 */
  async function openProgressHistory(boardId: number, candidateId: number): Promise<void> {
    if (researchCandidateId.value !== null) {
      notifyWarn("研究进行中，先看实时进度");
      return;
    }
    await refreshResearchProgress(boardId, candidateId, viewEpoch);
  }

  /** 周期切换（工具栏）：换周期清空任务态并重新拉取。 */
  async function changePeriod(
    boardId: number,
    gran: SignalGranularity,
    nextPeriod: string,
  ): Promise<boolean> {
    if (!validateSignalPeriodFormat(gran, nextPeriod)) {
      notifyError(gran === "year" ? "年度周期格式为 YYYY，如 2026" : "月度周期格式为 YYYY-MM，如 2026-09");
      return false;
    }
    granularity.value = gran;
    period.value = nextPeriod;
    discoveryError.value = null;
    researchError.value = null;
    await loadPeriod(boardId);
    return true;
  }

  // ── 发现信号（FE-1：只发 discovery，结束即停）──────────────────────────

  async function discover(boardId: number): Promise<boolean> {
    if (discoveryRunning.value || researchCandidateId.value !== null) {
      notifyWarn("已有任务在运行中，请等待完成");
      return false;
    }
    if (!validateSignalPeriodFormat(granularity.value, period.value)) {
      notifyError(granularity.value === "year" ? "年度周期格式为 YYYY，如 2026" : "月度周期格式为 YYYY-MM，如 2026-09");
      return false;
    }
    bindBoard(boardId);
    const epoch = viewEpoch;
    discoveryError.value = null;
    const res = await api.triggerSignalDiscovery(boardId, {
      granularity: granularity.value,
      period: period.value,
    });
    if (!stillCurrent(epoch, boardId)) return false; // 迟到：后端 job 已启动，回视图由 sync 兜底
    if (res.success) {
      const jobId = res.data?.job_id ?? "";
      if (jobId) {
        startDiscoveryPoll(boardId, jobId, epoch);
        return true;
      }
      discoveryError.value = "发现任务启动异常（无 job_id）";
      return false;
    }
    if (res.status === 409 && res.data && typeof res.data === "object" && "job_id" in res.data) {
      // 同板块任一任务在跑（共享互斥）：按 job_kind 接管恢复——signal 种类恢复
      // 轮询；旧 brief/investigation 不接管（bootstrap 的 syncBoardAnalysisStatus
      // 已恢复其轮询），只提示不谎报「已恢复」。
      const conflict = res.data as { job_id?: string; job_kind?: string; candidate_id?: number };
      if (conflict.job_id && conflict.job_kind === "board_signal_discovery") {
        notifyWarn(kindRunningLabel(conflict.job_kind));
        startDiscoveryPoll(boardId, conflict.job_id, epoch);
        return true;
      }
      if (conflict.job_id && conflict.job_kind === "board_signal_report") {
        notifyWarn(kindRunningLabel(conflict.job_kind));
        startResearchPoll(boardId, conflict.job_id, conflict.candidate_id ?? 0, epoch);
        return true;
      }
      notifyWarn("该板块的旧版任务（简报/调查）正在运行，完成后才能发现信号");
      return false;
    }
    // 400/其它同步预检失败：显错、可重试
    discoveryError.value = res.error || "发现信号失败";
    return false;
  }

  function startDiscoveryPoll(boardId: number, jobId: string, epoch: number) {
    stopDiscoveryPoll();
    discoveryRunning.value = true;
    discoveryPhase.value = "prepare";
    const gen = discoveryPollGen;
    scheduleDiscoveryPoll(boardId, jobId, epoch, gen, 0);
  }

  function scheduleDiscoveryPoll(boardId: number, jobId: string, epoch: number, gen: number, delayMs: number) {
    discoveryPollTimer = setTimeout(() => {
      discoveryPollTimer = null;
      void pollDiscoveryOnce(boardId, jobId, epoch, gen);
    }, delayMs);
  }

  async function pollDiscoveryOnce(boardId: number, jobId: string, epoch: number, gen: number) {
    const res = await api.getSignalJobStatus(jobId);
    if (gen !== discoveryPollGen || !stillCurrent(epoch, boardId)) return; // 迟到响应丢弃
    if (!res.success || !res.data) {
      if (res.status === 404) {
        // 后端重启丢内存任务：停轮询 + 如实提示 + 重拉列表（候选不卡 researching）
        stopDiscoveryPoll();
        discoveryError.value = "发现任务状态已失效（后端可能重启过），可重新点击「发现信号」重试";
        notifyError("发现任务已失效，可重试");
        await loadPeriod(boardId);
        return;
      }
      scheduleDiscoveryPoll(boardId, jobId, epoch, gen, POLL_INTERVAL_MS); // 瞬时网络错误继续轮询
      return;
    }
    const st = res.data;
    if (st.running) {
      if (st.phase) discoveryPhase.value = st.phase;
      scheduleDiscoveryPoll(boardId, jobId, epoch, gen, POLL_INTERVAL_MS);
      return;
    }
    stopDiscoveryPoll();
    await handleDiscoveryTerminal(boardId, st.outcome, st.error);
  }

  async function handleDiscoveryTerminal(
    boardId: number,
    outcome: string | undefined,
    error: string | undefined,
  ) {
    if (outcome === "failed") {
      discoveryError.value = error || "信号发现失败";
      return;
    }
    // discovered / no_signal：重拉列表。no_signal 安静空态——不清旧列表、
    // 不弹消息（空数组时组件自己显示安静空态）。
    await loadPeriod(boardId);
  }

  // ── 深入研究（只针对所点候选；重复点击 409 恢复原 job；完成 200 零费用）─

  async function research(
    boardId: number,
    candidateId: number,
    options: { regenerate?: boolean } = {},
  ): Promise<boolean> {
    if (discoveryRunning.value) {
      notifyWarn("信号发现进行中，请等待完成");
      return false;
    }
    if (researchCandidateId.value !== null) {
      notifyWarn("已有研究任务在运行，已恢复其进度显示");
      return false;
    }
    bindBoard(boardId);
    const epoch = viewEpoch;
    researchError.value = null;
    const res = await api.triggerSignalResearch(boardId, candidateId, {
      regenerate: options.regenerate ?? false,
    });
    if (!stillCurrent(epoch, boardId)) return false;
    if (res.success) {
      const data = res.data as { status?: string; job_id?: string; result_id?: number } | undefined;
      if (data?.status === "already_reported" && data.result_id) {
        // 幂等复用：零新费用，直接打开已有报告
        await openReport(boardId, data.result_id);
        return true;
      }
      const jobId = data?.job_id ?? "";
      if (jobId) {
        startResearchPoll(boardId, jobId, candidateId, epoch);
        return true;
      }
      researchError.value = "研究任务启动异常（无 job_id）";
      return false;
    }
    if (res.status === 409 && res.data && typeof res.data === "object" && "job_id" in res.data) {
      // running 一律 409（含自身候选重复点击）：signal 种类按冲突体身份恢复原
      // job 展示（研究恢复后 candidate_id 来自冲突体——运行中的身份以服务端为
      // 准）；旧 brief/investigation 在跑时不接管，只提示。
      const conflict = res.data as { job_id?: string; job_kind?: string; candidate_id?: number };
      if (conflict.job_id && conflict.job_kind === "board_signal_report") {
        notifyWarn(kindRunningLabel(conflict.job_kind));
        startResearchPoll(boardId, conflict.job_id, conflict.candidate_id || candidateId, epoch);
        return true;
      }
      if (conflict.job_id && conflict.job_kind === "board_signal_discovery") {
        notifyWarn(kindRunningLabel(conflict.job_kind));
        startDiscoveryPoll(boardId, conflict.job_id, epoch);
        return true;
      }
      notifyWarn("该板块的旧版任务（简报/调查）正在运行，完成后才能深入研究");
      return false;
    }
    researchError.value = res.error || "深入研究失败";
    return false;
  }

  function startResearchPoll(boardId: number, jobId: string, candidateId: number, epoch: number) {
    stopResearchPoll();
    researchJobId.value = jobId;
    researchCandidateId.value = candidateId;
    researchPhase.value = "research";
    researchProgress.value = null;
    const gen = researchPollGen;
    scheduleResearchPoll(boardId, jobId, candidateId, epoch, gen, 0);
  }

  function scheduleResearchPoll(boardId: number, jobId: string, candidateId: number, epoch: number, gen: number, delayMs: number) {
    researchPollTimer = setTimeout(() => {
      researchPollTimer = null;
      void pollResearchOnce(boardId, jobId, candidateId, epoch, gen);
    }, delayMs);
  }

  async function pollResearchOnce(boardId: number, jobId: string, candidateId: number, epoch: number, gen: number) {
    const res = await api.getSignalJobStatus(jobId);
    if (gen !== researchPollGen || !stillCurrent(epoch, boardId)) return;
    if (!res.success || !res.data) {
      if (res.status === 404) {
        stopResearchPoll();
        researchError.value = "研究任务状态已失效（后端可能重启过），候选已恢复待研究，可重新点击「深入分析」重试";
        notifyError("研究任务已失效，可重试");
        // 进展表在 DB：job 丢了已落库轮次仍在，拉一次供错误区回看（断了不能白跑）
        await refreshResearchProgress(boardId, candidateId, epoch);
        await loadPeriod(boardId);
        return;
      }
      scheduleResearchPoll(boardId, jobId, candidateId, epoch, gen, POLL_INTERVAL_MS);
      return;
    }
    const st = res.data;
    if (st.running) {
      if (st.phase) researchPhase.value = st.phase;
      // 串行纪律：job status 成功且仍在跑才拉进展，拉完再排下一发；
      // 进展拉取失败静默（展示增强不抢轮询主链路）
      await refreshResearchProgress(boardId, candidateId, epoch);
      scheduleResearchPoll(boardId, jobId, candidateId, epoch, gen, POLL_INTERVAL_MS);
      return;
    }
    stopResearchPoll();
    await handleResearchTerminal(boardId, candidateId, st.outcome, st.error, st.result_id, epoch);
  }

  /** 拉取候选最近研究进展全量（含 ledger）；失败静默，保持旧值下轮再拉。 */
  async function refreshResearchProgress(boardId: number, candidateId: number, epoch: number): Promise<void> {
    try {
      const res = await api.getSignalResearchProgress(boardId, candidateId);
      if (!stillCurrent(epoch, boardId)) return; // 迟到响应丢弃
      if (res.success) researchProgress.value = res.data ?? null;
    } catch {
      // 瞬时网络错误：下一轮轮询再拉
    }
  }

  async function handleResearchTerminal(
    boardId: number,
    candidateId: number,
    outcome: string | undefined,
    error: string | undefined,
    resultId: number | undefined,
    epoch: number,
  ) {
    if (outcome === "succeeded" && resultId) {
      researchProgress.value = null; // 每步链路由报告 appendix 接管展示
      notifySuccess("研究完成，可阅读报告");
      await loadPeriod(boardId);
      await openReport(boardId, resultId);
      return;
    }
    if (outcome === "failed") {
      // 先显错（旧时序：错误立即可见）；再重拉列表——abandoned 进展行已由后端
      // 持久化，重拉后的候选行携带最新 last_research_progress，追加进展摘要
      //（tasks 4.7「断了不能白跑」要在错误态可见）。
      researchError.value = stageErrorLabel(error);
      await refreshResearchProgress(boardId, candidateId, epoch);
      await loadPeriod(boardId);
      researchError.value = researchFailureMessage(candidateId, candidates.value, error);
      return;
    }
    // 未知终态兜底：重拉列表如实呈现
    researchProgress.value = null; // 不留给下一个候选
    await loadPeriod(boardId);
  }

  // ── 报告阅读 ───────────────────────────────────────────────────────────

  async function openReport(boardId: number, resultId: number): Promise<void> {
    bindBoard(boardId);
    const epoch = viewEpoch;
    activeReportId.value = resultId;
    activeReportLoading.value = true;
    activeReportError.value = null;
    try {
      const res = await api.getSignalReport(boardId, resultId);
      if (!stillCurrent(epoch, boardId)) return;
      if (res.success && res.data) {
        activeReport.value = res.data;
      } else {
        // detail error：错误态不伪装无数据
        activeReport.value = null;
        activeReportError.value = res.error || "报告加载失败";
      }
    } finally {
      if (stillCurrent(epoch, boardId)) activeReportLoading.value = false;
    }
  }

  function closeReport() {
    activeReportId.value = null;
    activeReport.value = null;
    activeReportLoading.value = false;
    activeReportError.value = null;
  }

  /** 是否该显示「报告生成中」类的 research 进度（discovery 中绝不显示）。 */
  const researchProgressLabel = computed(() => {
    if (researchCandidateId.value === null) return "";
    const phase = researchPhase.value === "compose" ? "成文" : "研究";
    const p = researchProgress.value;
    if (p && p.rounds_done > 0) {
      return `正在${phase} · 第 ${p.rounds_done}/40 轮 · 取数 ${p.source_calls} · 计算 ${p.calculation_calls}`;
    }
    return `正在${phase} · 决策上限 ${MAX_DECISIONS} 轮 · 可离开页面`;
  });

  onUnmounted(() => {
    viewEpoch++;
    stopDiscoveryPoll();
    stopResearchPoll();
  });

  return {
    // 周期
    granularity, period, changePeriod,
    // 列表
    candidates, candidatesLoading, candidatesError, loadCandidates,
    reports, reportsLoading, reportsError, loadReports, loadPeriod,
    // 发现
    discoveryRunning, discoveryPhase, discoveryError, discover,
    // 研究
    researchCandidateId, researchPhase, researchError, researchJobId,
    research, researchProgressLabel, researchProgress, openProgressHistory,
    // 报告阅读
    activeReportId, activeReport, activeReportLoading, activeReportError,
    openReport, closeReport,
    // 视图守卫
    setBoard,
  };
}

/** 409 冲突体按 job_kind 区分提示（旧 brief/investigation 在跑也共用互斥）。 */
function kindRunningLabel(jobKind?: string): string {
  switch (jobKind) {
    case "board_brief":
      return "该板块正在生成简报，已恢复进度显示";
    case "board_investigation":
      return "该板块正在调查中，已恢复进度显示";
    case "board_signal_discovery":
      return "信号发现进行中，已恢复进度显示";
    case "board_signal_report":
      return "研究进行中，已恢复进度显示";
    default:
      return "该板块已有任务在运行，已恢复进度显示";
  }
}

/** research 失败按 error_stage 组织可读文案（原始错误随附）。 */
function stageErrorLabel(error?: string): string {
  return error || "研究失败，候选已恢复待研究，可重试";
}

/**
 * 研究失败文案：携带已保留进展摘要（tasks 4.7）——从候选行
 * last_research_progress 读（重拉后的最新值），断了不能白跑要在错误态可见。
 */
function researchFailureMessage(
  candidateId: number,
  rows: SignalCandidateRow[],
  error?: string,
): string {
  const base = stageErrorLabel(error);
  const p = rows.find((c) => c.id === candidateId)?.last_research_progress;
  if (!p || p.rounds_done <= 0) return base;
  return `${base}（已保留 ${p.rounds_done} 轮进展（${p.source_calls} 次取数））`;
}