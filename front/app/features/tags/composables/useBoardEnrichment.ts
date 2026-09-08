import { ref, computed, onUnmounted } from "vue";
import { useNotify } from "~/composables/useNotify";
import {
	useBoardEnrichmentApi,
	type ContextRow,
	type ContextGranularity,
	type ResultSummaryRow,
	type ResultDetailRow,
	type ReviewRow,
	type DataSourceRow,
	type StockDebateResult,
	type CreateReviewBody,
	type UpsertDataSourceBody,
	type TopicEnrichmentQA,
	type AskQAResponse,
	type BoardAnalysisResultRow,
	type TriggerInvestigationBody,
} from "~/api/boardEnrichment";
import {
	useDailyReportsApi,
	type BoardTopicListItem,
} from "~/api/dailyReports";

/**
 * 数据增强面板的 composable（仿 useBoardCRUD 模式）。
 *
 * 维度拆分：
 *  - topic 维度（表 1/2/3）：contexts / results / reviews，挂在 selectedTopicId 上；
 *  - board 维度（数据源绑定）：dataSources，挂在 boardId 上。
 *
 * 面板组件直接 `const en = useBoardEnrichment()`，board/topic 切换时显式调
 * loadTopics / selectTopic / loadDataSources（不自动 watch，保持与 useBoardCRUD 一致
 * 的显式调用风格，便于测试与可控加载）。
 */
export function useBoardEnrichment() {
	const api = useBoardEnrichmentApi();
	const reportsApi = useDailyReportsApi();
	const { success: notifySuccess, error: notifyError, warn: notifyWarn } = useNotify();

	// ── topic selector ──────────────────────────────────────────────────────
	const topics = ref<BoardTopicListItem[]>([]);
	const topicsLoading = ref(false);
	const selectedTopicId = ref<number | null>(null);

	// ── table 1: contexts ───────────────────────────────────────────────────
	const contexts = ref<ContextRow[]>([]);
	const contextsLoading = ref(false);
	const regenerating = ref<ContextGranularity | null>(null);

	// ── table 2: results ────────────────────────────────────────────────────
	const results = ref<ResultSummaryRow[]>([]);
	const resultsLoading = ref(false);
	const triggering = ref(false);

	// ── table 3: reviews ────────────────────────────────────────────────────
	const reviews = ref<ReviewRow[]>([]);
	const reviewsLoading = ref(false);

	// ── board data sources ──────────────────────────────────────────────────
	const dataSources = ref<DataSourceRow[]>([]);
	const dataSourcesLoading = ref(false);

	// ── stock debates (FinGenius) ───────────────────────────────────────────
	const debates = ref<StockDebateResult[]>([]);
	const debatesLoading = ref(false);
	const debateTriggering = ref(false);
	const debateError = ref<string | null>(null);
	let debatePollTimer: ReturnType<typeof setInterval> | null = null;

	// ── qa (causal-analysis-agent 阶段3：报告追问) ──────────────────────────
	const qaList = ref<TopicEnrichmentQA[]>([]);
	const qaLoading = ref(false);
	const qaError = ref<string | null>(null);
	// 最近一次 ask 的即时响应（含 refs）。持久化 QA 行无 refs 列（后端只回显不落库），
	// 故仅最新这轮能渲染双类引用；QAPanel 按 answer 文本匹配挂到对应行。
	const latestAnswer = ref<AskQAResponse | null>(null);

	// ── board qa（board-level-deep-analysis 6.2：版块报告追问，独立 state）──
	// 与 topic QA 完全分开：挂在当前选中的版块报告（selectedBoardResult）上，
	// 不复用 topic 的 qa refs，防止聚焦区与版块主视图串台。三 kind（brief/
	// investigation/legacy）均可追问——后端 board 路由按 result id 工作。
	const boardQaList = ref<TopicEnrichmentQA[]>([]);
	const boardQaLoading = ref(false);
	const boardQaError = ref<string | null>(null);
	const boardLatestAnswer = ref<AskQAResponse | null>(null);
	/** 当前 boardQaList 所属 result id（sediment 路由需要；load 成功时写入）。 */
	const boardQaResultId = ref<number | null>(null);

	// ── workbench UI state（原型认知工作台）───────────────────────────────
	// ①周期筛选器：gran=按周/月/年（原型挊了 all），periodList 由 contexts 派生
	type PickerGran = "week" | "month" | "year";
	const selectedGran = ref<PickerGran>("week");
	const selectedPeriodIdx = ref(0); // 0=最新周期
	// loopnav 当前高亮 section（IntersectionObserver 写入）
	const activeSection = ref<string>("sec1");
	// ④辩论四态依赖的最新 result 详情（sectors 原始 + 辩论归属）
	const latestResultDetail = ref<ResultDetailRow | null>(null);
	const latestResultDetailLoading = ref(false);

	// ── board-level analysis（board-level-deep-analysis D2/D8 + 5.5 按 job_id 轮询）─
	const boardResults = ref<BoardAnalysisResultRow[]>([]);
	const boardResultsLoading = ref(false);
	const boardAnalysisTriggering = ref(false);
	/** 当前在跟的异步任务身份（brief 或 investigation；由 trigger 202/409 或重进恢复写入，
	 * 供面板区分「正在生成简报 / 正在调查」文案）。 */
	const activeBoardJob = ref<{ jobId: string; jobKind: string } | null>(null);
	// 当前展示的版块报告 id（默认最新简报；点历史可切换）
	const selectedBoardResultId = ref<number | null>(null);
	/** 最新简报行（列表中第一份 board_brief；旧行无 kind 时后端已兜底 legacy，不会误判）。 */
	const latestBoardBrief = computed<BoardAnalysisResultRow | null>(() =>
		boardResults.value.find((r) => r.result_kind === "board_brief") ?? null,
	);
	/** 当前选中的结果行：指定 id → 命中行；未指定/失效 → 最新简报兜底 → 首行（旧数据不崩）。 */
	const selectedBoardResult = computed<BoardAnalysisResultRow | null>(() => {
		const list = boardResults.value;
		if (!list.length) return null;
		const id = selectedBoardResultId.value;
		const found = id !== null ? list.find((r) => r.id === id) : undefined;
		return found ?? latestBoardBrief.value ?? list[0] ?? null;
	});

	const error = ref<string | null>(null);

	// ── board view context（5.4/5.5 review：统一视图守卫）─────────────────────
	// 面板每次 bootstrap 最前面 activateBoardContext(boardId)（内部停旧 board
	// 轮询 + viewEpoch++），onUnmounted 时 deactivateBoardContext（epoch++/
	// disposed/stop）。所有跨 await 的板块级副作用（trigger/sync/board 档 loader
	// 与 bootstrap 链的板块级 loader）先 capture {boardId, epoch}，await 返回后
	// boardViewStillCurrent 失配（已卸载/已切板块/epoch 变化）→ 静默丢弃：不
	// start poll、不 toast、不写共享 refs——旧板块慢响应不得串台新板块。
	// 未 activate 过的直调（单板块场景/旧测试）不设防，行为与旧版一致。
	// stopBoardPoll 为函数声明（定义在下方 board 轮询节，整体提升），调用时
	// setup 已完成初始化，此处引用安全。
	let activeBoardId: number | null = null;
	let viewEpoch = 0;
	let boardViewDisposed = false;

	/** 进入异步操作前捕获当前视图身份。 */
	function captureBoardView(boardId: number) {
		return { boardId, epoch: viewEpoch };
	}

	/** await 返回后校验视图身份：已销毁 / epoch 变化 / 激活板块不符 → false
	 * （迟到响应，调用方静默丢弃）。 */
	function boardViewStillCurrent(view: {
		boardId: number;
		epoch: number;
	}): boolean {
		if (boardViewDisposed) return false;
		if (view.epoch !== viewEpoch) return false;
		if (activeBoardId !== null && activeBoardId !== view.boardId) return false;
		return true;
	}

	/** 清空 topic 级展示 refs（表1/2/3、最新报告详情、QA、辩论及其 error/
	 * loading，按各自既有语义）。仅在「离开 topic 视图」的路径调用：切板块
	 * （activateBoardContext 检测到 boardId 真实变化）与 loadTopics 当前视图
	 * 失败——旧 board/旧 topic 的展示不得顶着新视图继续渲染（混合视图）。 */
	function clearTopicDisplayRefs() {
		contexts.value = [];
		contextsLoading.value = false;
		results.value = [];
		resultsLoading.value = false;
		reviews.value = [];
		reviewsLoading.value = false;
		latestResultDetail.value = null;
		latestResultDetailLoading.value = false;
		qaList.value = [];
		qaLoading.value = false;
		qaError.value = null;
		latestAnswer.value = null;
		boardQaList.value = [];
		boardQaLoading.value = false;
		boardQaError.value = null;
		boardLatestAnswer.value = null;
		boardQaResultId.value = null;
		debates.value = [];
		debatesLoading.value = false;
		debateError.value = null;
		debateTriggering.value = false;
	}

	/** 激活 board 视图上下文（面板每次 bootstrap 最前面调用）：切板块时三类
	 * 轮询（board/topic/debate）全停——各自 gen++ 使在途轮询响应失效、timer
	 * 清空，epoch++（旧捕获全部失效），旧板块任何 timer/迟到响应不得在新
	 * 板块写共享 refs/notify。仅当 boardId 真实变化（A→B）时同步把
	 * selectedTopicId 置空并清空旧 board 的 topic 级展示 refs——加载新 board
	 * 期间旧 topic 的表/详情/QA/辩论不得混合展示；同 board 手动刷新/
	 * 重进（boardId 不变）保留当前 topic 选择与展示，由后续 loader 刷新。 */
	function activateBoardContext(boardId: number) {
		// 真实切板块检测：activeBoardId 从有值变为不同值才算切换；首次
		// activate（null → id，挂载初始化）refs 本就为空，无需清场。
		const boardSwitched = activeBoardId !== null && activeBoardId !== boardId;
		stopBoardPoll();
		stopTopicPoll();
		stopDebatePolling();
		viewEpoch++;
		boardViewDisposed = false;
		activeBoardId = boardId;
		if (boardSwitched) {
			selectedTopicId.value = null;
			clearTopicDisplayRefs();
		}
	}

	/** 停用视图上下文（composable onUnmounted 调用）：停三类轮询、epoch++、
	 * 标记销毁——此后任何迟到 trigger/sync/loader/poll 响应一律丢弃，不重建
	 * timer、不 notify、不写共享 refs。 */
	function deactivateBoardContext() {
		stopBoardPoll();
		stopTopicPoll();
		stopDebatePolling();
		viewEpoch++;
		boardViewDisposed = true;
		activeBoardId = null;
	}

	/** 进入 topic 级异步操作前捕获身份（topicId + 当前 board 视图）。topic
	 * 维度共享 refs（contexts/results/reviews/detail/debates…）挂 selectedTopicId
	 * 上，写前须同时校验 board 视图与 topic 均未变（await 期间切板块/换 topic
	 * 的迟到响应静默丢弃）。 */
	function captureTopicView(topicId: number) {
		return { topicId, boardId: activeBoardId, epoch: viewEpoch };
	}

	/** await 返回后校验 topic 视图身份：board 视图失配（已销毁/切板块/epoch
	 * 变化/激活板块变化）或已换 topic → false。未 activate 过的直调（单板块
	 * 场景/旧测试）boardId 为 null 且不变化，行为与旧版一致（不设防）。 */
	function topicViewStillCurrent(view: {
		topicId: number;
		boardId: number | null;
		epoch: number;
	}): boolean {
		if (boardViewDisposed) return false;
		if (view.epoch !== viewEpoch) return false;
		if (activeBoardId !== view.boardId) return false;
		if (selectedTopicId.value !== view.topicId) return false;
		return true;
	}

	// ── topic selector actions ──────────────────────────────────────────────
	async function loadTopics(boardId: number) {
		const view = captureBoardView(boardId);
		topicsLoading.value = true;
		error.value = null;
		const res = await reportsApi.listBoardTopics(boardId);
		// 迟到守卫：切板块/卸载后旧响应不写 topics/selectedTopicId/loading
		if (!boardViewStillCurrent(view)) return;
		if (res.success && res.data) {
			topics.value = res.data.topics ?? [];
			const stillValid =
				selectedTopicId.value !== null &&
				topics.value.some((t) => t.id === selectedTopicId.value);
			if (!stillValid) {
				const firstActive = topics.value.find((t) => t.status === "active");
				selectedTopicId.value = topics.value.length
					? (firstActive?.id ?? topics.value[0]?.id ?? null)
					: null;
			}
		} else {
			// 失败语义（5.4/5.5 终审 Important，仅当前视图——迟到响应已被上方
			// epoch 守卫丢弃，不会清到新板块）：topics 置空 + 选择置空 + 清
			// topic 级展示 refs + 停 topic/debate 轮询——失败后不得继续展示/轮询
			// 旧 topic；bootstrap 随后读到 selectedTopicId=null 自然跳过
			// loadAllTopicTables/syncTopicAnalysisStatus（面板按无 topic 分支走）。
			stopTopicPoll();
			stopDebatePolling();
			selectedTopicId.value = null;
			clearTopicDisplayRefs();
			topics.value = [];
			error.value = res.error || "加载话题失败";
		}
		topicsLoading.value = false;
	}

	async function selectTopic(topicId: number) {
		// 切 topic：先停旧 topic 轮询与 debate 轮询（gen++ 使在途响应失效，迟到
		// 响应一律丢弃），再装载新 topic；board 轮询与 topic 无关，不动。
		stopTopicPoll();
		stopDebatePolling();
		selectedTopicId.value = topicId;
		await loadAllTopicTables(topicId);
	}

	// ── table loaders ───────────────────────────────────────────────────────
	async function loadContexts(topicId: number) {
		const view = captureTopicView(topicId);
		contextsLoading.value = true;
		const res = await api.listContexts(topicId);
		// 迟到守卫：await 期间切板块/换 topic → 不写 contexts/loading
		if (!topicViewStillCurrent(view)) return;
		if (res.success && res.data) {
			contexts.value = res.data;
		} else {
			contexts.value = [];
		}
		contextsLoading.value = false;
	}

	async function loadResults(topicId: number) {
		const view = captureTopicView(topicId);
		resultsLoading.value = true;
		const res = await api.listResults(topicId);
		// 迟到守卫：await 期间切板块/换 topic → 不写 results/loading（轮询终态
		// 重拉与手动刷新的迟到响应均不得污染新视图）。
		if (!topicViewStillCurrent(view)) return;
		if (res.success && res.data) {
			results.value = res.data;
		} else {
			results.value = [];
		}
		resultsLoading.value = false;
	}

	async function loadReviews(topicId: number) {
		const view = captureTopicView(topicId);
		reviewsLoading.value = true;
		const res = await api.listReviews(topicId);
		// 迟到守卫：await 期间切板块/换 topic → 不写 reviews/loading
		if (!topicViewStillCurrent(view)) return;
		if (res.success && res.data) {
			reviews.value = res.data;
		} else {
			reviews.value = [];
		}
		reviewsLoading.value = false;
	}

	async function loadAllTopicTables(topicId: number) {
		// 装载新 topic 前统一停旧 topic/debate 轮询（selectTopic / panel watch /
		// bootstrap 三入口兑底；幂等：已停时再 gen++ 无副作用）——board 轮询
		// 与 topic 无关，不动。
		stopTopicPoll();
		stopDebatePolling();
		const view = captureTopicView(topicId);
		await Promise.all([
			loadContexts(topicId),
			loadResults(topicId),
			loadReviews(topicId),
		]);
		// 迟到守卫：await 期间切板块/换 topic → 不动 periodIdx、不拉 detail/debates
		if (!topicViewStillCurrent(view)) return;
		selectedPeriodIdx.value = 0; // 切话题回到最新周期
		await loadLatestResultDetail(topicId);
		if (latestResultId.value !== null) {
			await loadDebates(latestResultId.value);
		}
	}

	// ── workbench helpers（原型认知工作台）──────────────────────────────
	/** 周期列表：当前粒度下的所有 period，按 period 降序（最新在前）。 */
	const periodList = computed(() =>
		contexts.value
			.filter((c) => c.granularity === selectedGran.value)
			.map((c) => c.period)
			.sort((a, b) => (a < b ? 1 : a > b ? -1 : 0)),
	);

	/** 当前选中的周期 context（periodList 为空时为 undefined）。 */
	const currentContext = computed<ContextRow | undefined>(() => {
		const p = periodList.value[selectedPeriodIdx.value];
		if (!p) return undefined;
		return contexts.value.find(
			(c) => c.granularity === selectedGran.value && c.period === p,
		);
	});

	function setGran(g: PickerGran) {
		if (selectedGran.value === g) return;
		selectedGran.value = g;
		selectedPeriodIdx.value = 0;
	}

	function shiftPeriod(delta: number) {
		const max = periodList.value.length - 1;
		if (max < 0) return;
		selectedPeriodIdx.value = Math.max(
			0,
			Math.min(max, selectedPeriodIdx.value + delta),
		);
	}

	/**
	 * 选中指定 period（7.3.1 手动补生成用）：设粒度 + 定位到 periodList 中的索引。
	 * regenerate 成功后 contexts 已含新行、periodList 重算，故能定位到刚生成的周期。
	 */
	function selectPeriod(granularity: PickerGran, period: string) {
		if (selectedGran.value !== granularity) selectedGran.value = granularity;
		const idx = periodList.value.indexOf(period);
		selectedPeriodIdx.value = idx >= 0 ? idx : 0;
	}

	function setActiveSection(id: string) {
		activeSection.value = id;
	}

	/** 最新 result id（results[0]），②③④ 都挂它。 */
	const latestResultId = computed<number | null>(
		() => results.value[0]?.id ?? null,
	);

	/** 拉最新 result 详情（含 sectors 原始结构）。 */
	async function loadLatestResultDetail(topicId: number) {
		const view = captureTopicView(topicId);
		const id = latestResultId.value;
		if (id === null) {
			// 同步早退也过守卫：迟到调用不得把新视图的 detail 清空
			if (topicViewStillCurrent(view)) latestResultDetail.value = null;
			return;
		}
		latestResultDetailLoading.value = true;
		const res = await api.getResult(topicId, id);
		// 迟到守卫：await 期间切板块/换 topic → 不写 detail/loading
		if (!topicViewStillCurrent(view)) return;
		if (res.success && res.data) latestResultDetail.value = res.data;
		else latestResultDetail.value = null;
		latestResultDetailLoading.value = false;
	}

	/** ④辩论四态：empty（无 result/无辩论）|loading（触发中或 running）|error|result。 */
	const debateStage = computed<"empty" | "loading" | "error" | "result">(() => {
		if (debateError.value) return "error";
		if (debateTriggering.value) return "loading";
		if (debates.value.some((d) => d.distill_status === "running"))
			return "loading";
		if (debates.value.length === 0) return "empty";
		return "result";
	});

	// ── table 1 actions ─────────────────────────────────────────────────────
	async function saveContext(
		topicId: number,
		granularity: ContextGranularity,
		period: string,
		content: string,
	): Promise<boolean> {
		const res = await api.updateContext(topicId, granularity, period, {
			content,
		});
		if (res.success && res.data) {
			const idx = contexts.value.findIndex(
				(c) => c.granularity === granularity && c.period === period,
			);
			if (idx >= 0) contexts.value[idx] = res.data;
			else contexts.value.push(res.data);
			notifySuccess("已保存上下文");
			return true;
		}
		notifyError(res.error || "保存失败");
		return false;
	}

	async function regenerateContext(
		topicId: number,
		granularity: ContextGranularity,
		period?: string,
	): Promise<boolean> {
		regenerating.value = granularity;
		try {
			const res = await api.regenerateContext(topicId, granularity, period);
			if (res.success && res.data) {
				const row = res.data;
				const idx = contexts.value.findIndex(
					(c) => c.granularity === granularity && c.period === row.period,
				);
				if (idx >= 0) contexts.value[idx] = row;
				else contexts.value.push(row);
				notifySuccess(
					`${granularity}${period ? " " + period : ""} 上下文已重生成`,
				);
				return true;
			}
			notifyError(res.error || "重生成失败");
			return false;
		} finally {
			regenerating.value = null;
		}
	}

	// ── table 2 actions ─────────────────────────────────────────────────────
	// 异步轮询（fix-board-analysis-material 8.x + board-level-deep-analysis 5.5）：
	// trigger 立即返回，分析后台跑；topic 档按 scope/id 定时轮询，board 档
	// 统一按 job_id 精确轮询（函数在下方 board-level analysis 节）。离开页面
	// 只停轮询，后台分析不受影响（回来时 sync 恢复显示）。
	// board 档为**串行轮询**：每次请求返回后才 setTimeout 排下一发（仍 3s），
	// 杜绝 interval 重叠；配合 generation + boardId + jobId 三重身份守卫，
	// 旧板块/旧任务的迟到响应一律丢弃（不 stop 新 timer、不 notify、不重拉）。
	let boardPollTimer: ReturnType<typeof setTimeout> | null = null;
	let topicPollTimer: ReturnType<typeof setInterval> | null = null;
	const POLL_INTERVAL_MS = 3000;

	/** 当前 board 轮询上下文（startBoardPoll 写入 / stopBoardPoll 置空）。
	 * 配合自增的 boardPollGen 做迟到响应身份校验。 */
	let boardPollCtx: { boardId: number; jobId: string; jobKind: string } | null =
		null;
	let boardPollGen = 0;

	/** await 返回后校验轮询身份是否仍当前（generation + boardId + jobId）。
	 * 迟到响应（切板块/停轮询/换任务后才返回）返回 false，调用方静默丢弃。 */
	function boardPollStillCurrent(
		boardId: number,
		jobId: string,
		gen: number,
	): boolean {
		return (
			gen === boardPollGen &&
			boardPollCtx !== null &&
			boardPollCtx.boardId === boardId &&
			boardPollCtx.jobId === jobId
		);
	}

	/** 进入面板时同步一次 board 状态：scope/id 入口只用于重进恢复当前/最近
	 * running 任务的身份，拿到 job_id 后立即转按 job_id 精确轮询（5.5；
	 * brief/investigation 的轮询函数见下方 board-level analysis 节）。
	 * scope 状态为 idle / 最近任务已 finished（不误当 running）/ 查询失败且
	 * 当前没有在跟的轮询时，解除 triggering——409 无 data 的旧后端 fallback
	 * 也走到这里，不清会把「生成简报」按钮永久卡死。 */
	async function syncBoardAnalysisStatus(boardId: number) {
		const view = captureBoardView(boardId);
		const res = await api.getAnalysisStatus("board", boardId);
		// 迟到守卫：await 期间卸载/切板块 → 不 start poll、不清/不写任务态
		if (!boardViewStillCurrent(view)) return;
		if (res.success && res.data?.running && res.data.job_id) {
			startBoardPoll(
				boardId,
				res.data.job_id,
				res.data.job_kind ?? "board_brief",
			);
			return;
		}
		// 未进入轮询：若也无在跟任务，清掉任务态（防按钮卡住）。
		// 有在跟任务（boardPollCtx 非空，如 trigger 与 sync 并发竞态）则不动。
		if (boardPollCtx === null) {
			boardAnalysisTriggering.value = false;
			activeBoardJob.value = null;
		}
	}

	/** 当前 topic 轮询上下文（startTopicPoll 写入 / stopTopicPoll 置空）。配合
	 * 自增的 topicPollGen 做迟到响应身份校验（伤照 board 档 boardPollCtx）。 */
	let topicPollCtx: { topicId: number; boardId: number | null; epoch: number } | null =
		null;
	let topicPollGen = 0;

	function stopTopicPoll() {
		topicPollGen++;
		topicPollCtx = null;
		if (topicPollTimer) {
			clearInterval(topicPollTimer);
			topicPollTimer = null;
		}
		triggering.value = false;
	}

	function startTopicPoll(topicId: number) {
		stopTopicPoll();
		triggering.value = true;
		// 启动时捕获 topicId + board 视图（boardId/epoch），后续每轮不读变化后
		// 的 selectedTopicId；await 返回后凭 gen+ctx+视图三重校验。旧 topic job
		// 在后端照跑，前端停轮询不影响，回原板块/话题重拉可见结果。
		const view = captureTopicView(topicId);
		topicPollCtx = view;
		const gen = topicPollGen;
		topicPollTimer = setInterval(() => {
			void pollTopicOnce(topicId, view, gen);
		}, POLL_INTERVAL_MS);
	}

	async function pollTopicOnce(
		topicId: number,
		view: { topicId: number; boardId: number | null; epoch: number },
		gen: number,
	) {
		const res = await api.getAnalysisStatus("topic", topicId);
		// 迟到守卫：await 期间切板块/换 topic/停轮询 → 丢弃（不 stop 新轮询、
		// 不 notify、不重拉/写任何共享 refs）。
		if (gen !== topicPollGen || topicPollCtx !== view) return;
		if (!topicViewStillCurrent(view)) return;
		if (!res.success || !res.data) {
			// 404：scope 入口正常不返回（仅 job_id 精查会），防御与 board 档一致。
			if (res.status === 404) {
				stopTopicPoll();
				notifyError("任务已失效（服务可能重启过），请重新触发");
			}
			return; // 其他瞬时错误：继续轮询
		}
		if (res.data.running) return;
		stopTopicPoll(); // 校验通过才停：不得误杀新轮询
		if (res.data.error) {
			notifyError(`增强失败：${res.data.error}`);
			return;
		}
		notifySuccess("增强完成");
		// 三个 loader 各自在内部写入前再校验当前 board epoch + selectedTopicId，
		// await 期间切板块/换 topic 的迟到响应逐个拒写。
		await loadResults(topicId);
		await loadReviews(topicId);
		await loadLatestResultDetail(topicId);
	}

	async function triggerEnrichment(
		topicId: number,
		prefillLens?: string,
	): Promise<boolean> {
		const view = captureTopicView(topicId);
		triggering.value = true;
		try {
			const res = await api.triggerEnrichment(topicId, prefillLens);
			// 迟到守卫：await 期间切板块/换 topic/卸载 → 不 toast、不启轮询、
			// 不动新视图的 triggering（后端 job 已启动，回原视图重拉可见）。
			if (!topicViewStillCurrent(view)) return false;
			if (res.success) {
				notifySuccess("分析已在后台开始，可离开页面");
				startTopicPoll(topicId);
				return true;
			}
			// 409 = 已在跑：后端冲突体携当前任务身份（runErr.Current，含 job_id）。
			// topic 档本切片不重构为 job_id 轮询：直接按 scope 入口恢复轮询显示，
			// 终态文案/重拉由 pollTopicOnce 统一处理（状态语义与 board 档一致）。
			if (res.status === 409 || res.error?.includes("already running")) {
				startTopicPoll(topicId);
				return true;
			}
			// 400 = 板块未开启增强
			notifyError(res.error || "触发失败：需先在板块编辑开启增强开关");
			triggering.value = false;
			return false;
		} catch {
			if (topicViewStillCurrent(view)) triggering.value = false;
			return false;
		}
	}

	/** 重进/切回/刷新时同步一次 topic 档任务状态（5.5 最终 review：topic 档
	 * 重进恢复）。scope 入口查当前/最近任务：running → 恢复 triggering +
	 * 启动 topic 轮询（同 board 档 syncBoardAnalysisStatus 的重进恢复语义）。
	 * idle / 最近任务已 finished（不误当 running）/ 查询失败（404/网络瞬断，
	 * scope 入口正常不返回 404，防御语义与 board 档一致：静默不惊扰）且当前
	 * 没有在跟的 topic 轮询时，解除 triggering——不清会把「重新分析」按钮
	 * 卡死；topicPollCtx 非空（trigger 与 sync 并发竞态/别的 topic 刚启动新
	 * poll）则不动，不误停新轮询。
	 * 迟到守卫：捕获 board epoch + topicId，await 后失配（卸载/切板块/换
	 * topic）→ 静默丢弃，不 start poll、不写新视图任务态——旧 board 的迟到
	 * status 不得写入新 board。 */
	async function syncTopicAnalysisStatus(topicId: number) {
		const view = captureTopicView(topicId);
		const res = await api.getAnalysisStatus("topic", topicId);
		// 迟到守卫：await 期间卸载/切板块/换 topic → 不 start poll、不清/不写任务态
		if (!topicViewStillCurrent(view)) return;
		if (res.success && res.data?.running) {
			startTopicPoll(topicId);
			return;
		}
		// 未进入轮询：若也无在跟任务（topicPollCtx 非空 = trigger 与 sync 并发
		// 竞态或刚恢复/启动的新轮询），不动 triggering。
		if (topicPollCtx === null) {
			triggering.value = false;
		}
	}

	// ── board-level analysis actions（board-level-deep-analysis D2/D8 + 5.5）──
	async function loadBoardAnalysisResults(boardId: number) {
		const view = captureBoardView(boardId);
		boardResultsLoading.value = true;
		try {
			const res = await api.listBoardAnalysisResults(boardId);
			// 迟到守卫：切板块/卸载后旧响应不写 boardResults/selected/loading
			// （poll finished 后的 reload 在途切板块也由此拒写）。
			if (!boardViewStillCurrent(view)) return;
			if (res.success && res.data) {
				boardResults.value = res.data;
				// 默认选中最新简报（legacy/investigation 行留在历史下拉里手动切换）
				selectedBoardResultId.value =
					res.data.find((r) => r.result_kind === "board_brief")?.id ?? null;
			} else {
				boardResults.value = [];
			}
		} finally {
			// loading 属当前视图：迟到响应不得提前清掉新板块的在途 loading
			if (boardViewStillCurrent(view)) boardResultsLoading.value = false;
		}
	}

	/** 停 board 档轮询并清任务态（timer + activeBoardJob/triggering + 上下文），
	 * 自增 generation 使在途请求返回后因身份失配被丢弃。切板块/卸载/任务终态
	 * 都走这里；导出供面板 bootstrap 在加载新板块前先隔离旧板块。 */
	function stopBoardPoll() {
		boardPollGen++;
		boardPollCtx = null;
		if (boardPollTimer) {
			clearTimeout(boardPollTimer);
			boardPollTimer = null;
		}
		boardAnalysisTriggering.value = false;
		activeBoardJob.value = null;
	}

	/** 按 job_id 精确轮询（trigger 202/409 或重进恢复后进入这里）。 */
	function startBoardPoll(boardId: number, jobId: string, jobKind: string) {
		stopBoardPoll();
		boardPollCtx = { boardId, jobId, jobKind };
		const gen = boardPollGen;
		activeBoardJob.value = { jobId, jobKind };
		boardAnalysisTriggering.value = true;
		scheduleBoardPoll(boardId, jobId, jobKind, gen, 0); // 立即首发
	}

	/** 串行排程：上一发请求返回并处理完才排下一发（仍 3s），无 interval 重叠。 */
	function scheduleBoardPoll(
		boardId: number,
		jobId: string,
		jobKind: string,
		gen: number,
		delayMs: number,
	) {
		boardPollTimer = setTimeout(() => {
			boardPollTimer = null;
			void pollBoardJobOnce(boardId, jobId, jobKind, gen);
		}, delayMs);
	}

	async function pollBoardJobOnce(
		boardId: number,
		jobId: string,
		jobKind: string,
		gen: number,
	) {
		const res = await api.getAnalysisStatusByJobId(jobId);
		// 身份校验：await 期间切板块/停轮询/换任务 → 迟到响应丢弃（不 stop、
		// 不 notify、不重拉/选中旧板块结果）。
		if (!boardPollStillCurrent(boardId, jobId, gen)) return;
		if (!res.success || !res.data) {
			// 404 unknown job：服务重启丢了内存任务表——停轮询如实提示，不再盲转。
			if (res.status === 404) {
				stopBoardPoll();
				notifyError("任务已失效（服务可能重启过），请重新触发");
				return;
			}
			scheduleBoardPoll(boardId, jobId, jobKind, gen, POLL_INTERVAL_MS); // 瞬时网络错误：继续轮询
			return;
		}
		const st = res.data;
		if (st.running) {
			scheduleBoardPoll(boardId, jobId, jobKind, gen, POLL_INTERVAL_MS);
			return;
		}
		stopBoardPoll();
		const kind = st.job_kind || jobKind;
		if (st.error) {
			notifyError(
				`${kind === "board_investigation" ? "调查" : "简报"}生成失败：${st.error}`,
			);
			return;
		}
		if (kind !== "board_brief" && kind !== "board_investigation") {
			// 未知 kind（防御）：不重拉不选中，如实提示
			notifySuccess("任务已完成");
			return;
		}
		// 5.7：brief 与 investigation 完成都重拉列表并选中 result_id——调查行被
		// 选中后主视图切到调查报告（BoardInvestigationReport）；但默认选中/
		// latestBoardBrief 计算只认 board_brief，调查绝不冒充简报（重拉后
		// loadBoardAnalysisResults 的默认选中会被下方 result_id 覆盖）。
		notifySuccess(
			kind === "board_investigation" ? "调查已完成" : "简报已生成",
		);
		// 停轮询已自增 gen；记录停后快照，重拉期间若又切板块/换任务（gen 再变）
		// 则不写旧板块的选中。
		const genAfterStop = boardPollGen;
		await loadBoardAnalysisResults(boardId);
		if (boardPollGen !== genAfterStop) return;
		if (st.result_id) selectedBoardResultId.value = st.result_id;
	}

	async function triggerBoardAnalysis(boardId: number): Promise<boolean> {
		const view = captureBoardView(boardId);
		boardAnalysisTriggering.value = true;
		try {
			const res = await api.triggerBoardAnalysis(boardId);
			// 迟到守卫：await 期间卸载/切板块 → 静默丢弃（不 toast、不 start poll、
			// 不动 triggering——新视图自己管理任务态；后端 job 已启动，回 A 由
			// bootstrap 的 sync 恢复）。
			if (!boardViewStillCurrent(view)) return false;
			if (res.success) {
				const jobId = res.data?.job_id ?? "";
				const jobKind = res.data?.job_kind ?? "board_brief";
				notifySuccess("简报已在后台生成，可离开页面");
				if (jobId) {
					startBoardPoll(boardId, jobId, jobKind);
				} else {
					// 老后端无 job_id：走 scope 入口恢复当前任务
					await syncBoardAnalysisStatus(boardId);
				}
				return true;
			}
			// 409 = 已在跑（brief 或 investigation）：按冲突体携带的当前任务身份恢复轮询，
			// 不重复启动、不误把另一 kind 当成本次触发。
			if (res.status === 409 && res.data?.job_id) {
				const kind = res.data.job_kind ?? "board_brief";
				notifyWarn(
					kind === "board_investigation"
					? "该板块正在调查中，已恢复进度显示"
					: "该板块正在生成简报，已恢复进度显示",
				);
				startBoardPoll(boardId, res.data.job_id, kind);
				return true;
			}
			// 兼容无冲突体的旧后端：scope 入口发现 running 后转精确轮询
			if (res.error?.includes("already running")) {
				await syncBoardAnalysisStatus(boardId);
				return true;
			}
			notifyError(res.error || "简报触发失败");
			boardAnalysisTriggering.value = false;
			return false;
		} catch {
			// 迟到守卫：卸载/切板块后异常返回也不动新视图的 triggering
			if (boardViewStillCurrent(view)) boardAnalysisTriggering.value = false;
			return false;
		}
	}

	/** 选历史报告（null = 回最新）。 */
	function selectBoardResult(id: number | null) {
		selectedBoardResultId.value = id;
	}

	/** 触发问题调查（board-level-deep-analysis 5.7 接线）。
	 * generated 问题传 question_id（文本以父简报为准），custom 传 question 文本。
	 * 与 triggerBoardAnalysis 同一套按 job_id 轮询 + 视图守卫：
	 *  - 202 → 保存任务身份并 startBoardPoll（detached 后台跑完）；
	 *  - 409 → 冲突体 data 携当前在跑任务身份，按其 job_id 恢复轮询（同板块
	 *    跨 kind 串行，不重复启动、不误把另一 kind 当本次触发）；
	 *  - 400/404 同步预检失败（板块未开启/父简报不存在或非 board_brief/
	 *    question_id 解析不到）→ 明确报错、不启动轮询、释放 triggering。 */
	async function triggerBoardInvestigation(
		boardId: number,
		payload: TriggerInvestigationBody,
	): Promise<boolean> {
		const view = captureBoardView(boardId);
		boardAnalysisTriggering.value = true;
		try {
			const res = await api.triggerBoardInvestigation(boardId, payload);
			// 迟到守卫：await 期间卸载/切板块 → 静默丢弃（不 toast、不 start poll、
			// 不动新视图 triggering——后端 job 已启动，回 A 由 bootstrap 的 sync 恢复）。
			if (!boardViewStillCurrent(view)) return false;
			if (res.success) {
				const jobId = res.data?.job_id ?? "";
				const jobKind = res.data?.job_kind ?? "board_investigation";
				notifySuccess("调查已在后台开始，可离开页面");
				if (jobId) {
					startBoardPoll(boardId, jobId, jobKind);
				} else {
					// 老后端无 job_id：走 scope 入口恢复当前任务
					await syncBoardAnalysisStatus(boardId);
				}
				return true;
			}
			// 409 = 已在跑（brief 或 investigation）：按冲突体携有的当前任务身份恢复轮询
			if (res.status === 409 && res.data?.job_id) {
				const kind = res.data.job_kind ?? "board_brief";
				notifyWarn(
					kind === "board_investigation"
					? "该板块正在调查中，已恢复进度显示"
					: "该板块正在生成简报，已恢复进度显示",
				);
				startBoardPoll(boardId, res.data.job_id, kind);
				return true;
			}
			// 兼容无冲突体的旧后端：scope 入口发现 running 后转精确轮询
			if (res.error?.includes("already running")) {
					await syncBoardAnalysisStatus(boardId);
				return true;
			}
			// 400/404 同步预检失败（或其它错误）：明确报错、不启动轮询
			notifyError(res.error || "调查触发失败");
			boardAnalysisTriggering.value = false;
			return false;
		} catch {
			// 迟到守卫：卸载/切板块后异常返回也不动新视图的 triggering
			if (boardViewStillCurrent(view)) boardAnalysisTriggering.value = false;
			return false;
		}
	}

	onUnmounted(() => {
		deactivateBoardContext();
		stopTopicPoll();
	});

	// ── table 3 actions ─────────────────────────────────────────────────────
	async function saveReviewDeviation(
		topicId: number,
		reviewId: number,
		deviation: string,
	): Promise<boolean> {
		const res = await api.updateReviewDeviation(topicId, reviewId, {
			deviation_summary: deviation,
		});
		if (res.success && res.data) {
			const idx = reviews.value.findIndex((r) => r.id === reviewId);
			if (idx >= 0) reviews.value[idx] = res.data;
			notifySuccess("已保存偏差说明");
			return true;
		}
		notifyError(res.error || "保存失败");
		return false;
	}

	async function applyReview(
		topicId: number,
		reviewId: number,
	): Promise<boolean> {
		const res = await api.applyReview(topicId, reviewId);
		if (res.success && res.data) {
			const idx = reviews.value.findIndex((r) => r.id === reviewId);
			if (idx >= 0) reviews.value[idx] = res.data;
			notifySuccess("已采纳");
			return true;
		}
		notifyError(res.error || "采纳失败");
		return false;
	}

	async function createReview(
		topicId: number,
		body: CreateReviewBody,
	): Promise<boolean> {
		const res = await api.createReview(topicId, body);
		if (res.success && res.data) {
			reviews.value.unshift(res.data);
			notifySuccess("已添加批注");
			return true;
		}
		notifyError(res.error || "添加批注失败");
		return false;
	}

	// ── board data source actions ───────────────────────────────────────────
	async function loadDataSources(boardId: number) {
		const view = captureBoardView(boardId);
		dataSourcesLoading.value = true;
		const res = await api.listDataSources(boardId);
		// 迟到守卫：切板块/卸载后旧响应不写 dataSources/loading
		if (!boardViewStillCurrent(view)) return;
		if (res.success && res.data) {
			dataSources.value = res.data;
		} else {
			dataSources.value = [];
		}
		dataSourcesLoading.value = false;
	}

	async function saveDataSource(
		boardId: number,
		body: UpsertDataSourceBody,
	): Promise<boolean> {
		const res = await api.upsertDataSource(boardId, body);
		if (res.success && res.data) {
			await loadDataSources(boardId);
			notifySuccess("已保存数据源");
			return true;
		}
		notifyError(res.error || "保存失败");
		return false;
	}

	async function removeDataSource(
		boardId: number,
		sourceType: string,
	): Promise<boolean> {
		const res = await api.deleteDataSource(boardId, sourceType);
		if (res.success) {
			dataSources.value = dataSources.value.filter(
				(d) => d.source_type !== sourceType,
			);
			notifySuccess("已删除数据源");
			return true;
		}
		notifyError(res.error || "删除失败");
		return false;
	}

	// ── stock debate actions ────────────────────────────────────────────────
	/** 当前 debate 轮询上下文（startDebatePolling 写入 / stopDebatePolling 置空）。
	 * 启动时捕获 topicId+resultId+board epoch，每轮不读变化后的 selectedTopicId；
	 * 配合自增的 debatePollGen 做迟到响应身份校验（伤照 board 档 boardPollCtx）。 */
	let debatePollCtx: {
		topicId: number;
		resultId: number;
		boardId: number | null;
		epoch: number;
	} | null = null;
	let debatePollGen = 0;
	const DEBATE_POLL_INTERVAL_MS = 5000;

	function stopDebatePolling() {
		debatePollGen++;
		debatePollCtx = null;
		if (debatePollTimer) {
			clearInterval(debatePollTimer);
			debatePollTimer = null;
		}
		// 旧视图的触发/加载态一并清：切 board/换 topic 后不得残留 loading 卡死
		debateTriggering.value = false;
		debatesLoading.value = false;
	}

	function startDebatePolling(topicId: number, resultId: number) {
		stopDebatePolling();
		const view = { topicId, resultId, boardId: activeBoardId, epoch: viewEpoch };
		debatePollCtx = view;
		const gen = debatePollGen;
		debatePollTimer = setInterval(() => {
			void pollDebateOnce(view, gen);
		}, DEBATE_POLL_INTERVAL_MS);
	}

	/** await 返回后校验 debate 轮询身份：gen+ctx 未换 且 topic 视图（board
	 * epoch + selectedTopicId）仍当前；迟到响应静默丢弃（不写 debates、不
	 * stop 新轮询）。 */
	function debatePollStillCurrent(
		view: {
			topicId: number;
			resultId: number;
			boardId: number | null;
			epoch: number;
		},
		gen: number,
	): boolean {
		if (gen !== debatePollGen) return false;
		if (debatePollCtx !== view) return false;
		return topicViewStillCurrent(view);
	}

	async function pollDebateOnce(
		view: {
			topicId: number;
			resultId: number;
			boardId: number | null;
			epoch: number;
		},
		gen: number,
	) {
		// 每轮读启动时捕获的 topicId/resultId，不读变化后的 selectedTopicId
		const res = await api.listDebates(view.topicId, view.resultId);
		if (!debatePollStillCurrent(view, gen)) return;
		if (res.success && res.data) {
			debates.value = res.data;
			const stillRunning = res.data.some(
				(d) => d.distill_status === "running",
			);
			if (!stillRunning) stopDebatePolling(); // 校验通过才停：不误杀新轮询
		}
	}

	async function loadDebates(resultId: number) {
		const topicId = selectedTopicId.value;
		if (topicId === null) {
			debates.value = [];
			return;
		}
		// 装载当前视图的辩论前先停旧轮询（旧 topic/result 的 timer 一律清）
		stopDebatePolling();
		const view = { topicId, resultId, boardId: activeBoardId, epoch: viewEpoch };
		const gen = debatePollGen; // 停后快照：期间又启新轮询则本响应不写/不再启
		debatesLoading.value = true;
		const res = await api.listDebates(topicId, resultId);
		// 迟到守卫：await 期间切板块/换 topic/又启新轮询 → 不写 debates/loading、
		// 不启轮询
		if (gen !== debatePollGen || !topicViewStillCurrent(view)) return;
		if (res.success && res.data) {
			debates.value = res.data;
			if (res.data.some((d) => d.distill_status === "running"))
				startDebatePolling(topicId, resultId);
		} else {
			debates.value = [];
		}
		debatesLoading.value = false;
	}

	async function triggerDebate(resultId: number): Promise<boolean> {
		const topicId = selectedTopicId.value;
		if (topicId === null) return false;
		const view = { topicId, resultId, boardId: activeBoardId, epoch: viewEpoch };
		debateTriggering.value = true;
		debateError.value = null;
		try {
			const res = await api.triggerDebate(topicId, resultId);
			// 迟到守卫：await 期间切板块/换 topic/停轮询 → 不写 debates/debateError、
			// 不 notify、不启轮询、不动新视图的触发态
			if (!topicViewStillCurrent(view)) return false;
			if (res.success && res.data) {
				debates.value = res.data.debates ?? [];
				notifySuccess("辩论已触发");
				startDebatePolling(topicId, resultId);
				return true;
			}
			debateError.value = res.error || "触发辩论失败";
			notifyError(debateError.value);
			return false;
		} finally {
			if (topicViewStillCurrent(view)) debateTriggering.value = false;
		}
	}

	// ── qa actions（causal-analysis-agent 阶段3：报告追问 + 沉淀）────────────
	/** topic QA 双重迟到守卫（6.x review hardening，与 board QA 同级）：topic
	 * 视图仍当前（board epoch + topicId）且挂载 result 未变（latestResultId）。
	 * 切板块 / 切 topic / 换最新报告后旧响应静默丢弃，不串台新视图的 QA。 */
	function topicQaStillCurrent(
		view: { topicId: number; boardId: number | null; epoch: number },
		resultId: number | null,
	): boolean {
		return topicViewStillCurrent(view) && latestResultId.value === resultId
	}

	/** 加载某 result 的多轮追问历史（oldest first）。 */
	async function loadQA(resultId: number): Promise<boolean> {
		const topicId = selectedTopicId.value;
		if (topicId === null) {
			qaList.value = [];
			return false;
		}
		const view = captureTopicView(topicId);
		qaLoading.value = true;
		qaError.value = null;
		const res = await api.listQA(topicId, resultId);
		// 迟到守卫：切板块 / 切 topic / 换最新报告 / 卸载 → 不写 qaList/qaError、
		// 不清新视图的在途 loading（新视图自管）
		if (!topicQaStillCurrent(view, resultId)) return false;
		if (res.success && res.data) {
			qaList.value = res.data;
		} else {
			qaList.value = [];
			qaError.value = res.error || "加载追问历史失败";
		}
		qaLoading.value = false;
		return res.success;
	}

	/**
	 * 问一轮（挂 latestResultId）。
	 * 后端 Ask 已 append 一行 source="qa" 的 QA，但响应只回 {answer,tool_calls,refs}
	 * （缺 id/source/sedimented/created_at），无法拼出完整行，故成功后重拉列表。
	 */
	async function askQuestion(question: string): Promise<boolean> {
		const topicId = selectedTopicId.value;
		if (topicId === null) return false;
		const resultId = latestResultId.value;
		if (resultId === null) {
			qaError.value = "暂无报告，无法追问";
			notifyError(qaError.value);
			return false;
		}
		const view = captureTopicView(topicId);
		qaLoading.value = true;
		qaError.value = null;
		try {
			const res = await api.askQA(topicId, resultId, question);
			// 迟到守卫：await 期间切板块 / 切 topic / 换最新报告 → 静默丢弃，
			// 不写 latestAnswer/qaError、不重拉不 toast
			if (!topicQaStillCurrent(view, resultId)) return false;
			if (res.success) {
				// 捕获即时响应（含 refs），供 QAPanel 给最新这轮渲染双类引用。
				latestAnswer.value = res.data ?? null;
				await loadQA(resultId);
				return true;
			}
			qaError.value = res.error || "追问失败";
			notifyError(qaError.value);
			return false;
		} finally {
			// loading 属当前视图：迟到响应不得清新视图的在途 loading
			if (topicQaStillCurrent(view, resultId)) qaLoading.value = false;
		}
	}

	/** 沉淀一轮（用后端回写的完整行替换 qaList 对应项，sedimented=true）。 */
	async function sedimentAnswer(qaId: number): Promise<boolean> {
		const topicId = selectedTopicId.value;
		if (topicId === null) return false;
		const resultId = latestResultId.value;
		const view = captureTopicView(topicId);
		const res = await api.sedimentQA(topicId, qaId);
		// 迟到守卫：切板块 / 切 topic / 换最新报告 → 静默丢弃，不替换行不 toast
		if (!topicQaStillCurrent(view, resultId)) return false;
		if (res.success && res.data) {
			const idx = qaList.value.findIndex((q) => q.id === qaId);
			if (idx >= 0) qaList.value[idx] = res.data;
			notifySuccess("已沉淀");
			return true;
		}
		notifyError(res.error || "沉淀失败");
		return false;
	}

	// ── board qa actions（board-level-deep-analysis 6.2：版块报告追问）──────
	// 当前有效展示的版块 result id（selectedBoardResult 计算值，含兜底链）。
	// QA 挂在这个 id 上：切历史报告 → QAPanel 的 resultId prop 变 → 重新 load。
	const currentBoardResultId = computed<number | null>(
		() => selectedBoardResult.value?.id ?? null,
	);

	/** board QA 双重迟到守卫：视图仍当前（board epoch）且选中 result 未变。
	 * 切板块 / 切历史报告后旧响应静默丢弃，不串台新报告的 QA。 */
	function boardQaStillCurrent(
		view: { boardId: number; epoch: number },
		resultId: number,
	): boolean {
		return boardViewStillCurrent(view) && currentBoardResultId.value === resultId;
	}

	/** 加载某份版块报告的多轮追问历史（oldest first，board 路由）。 */
	async function loadBoardQA(resultId: number): Promise<boolean> {
		const boardId = activeBoardId;
		if (boardId === null) {
			boardQaList.value = [];
			boardQaResultId.value = null;
			return false;
		}
		const view = captureBoardView(boardId);
		boardQaLoading.value = true;
		boardQaError.value = null;
		const res = await api.listBoardQA(boardId, resultId);
		// 迟到守卫：切板块 / 切报告 / 卸载 → 不写 boardQa*/loading（新视图自管）
		if (!boardQaStillCurrent(view, resultId)) return false;
		if (res.success && res.data) {
			boardQaList.value = res.data;
			boardQaResultId.value = resultId;
		} else {
			boardQaList.value = [];
			boardQaResultId.value = null;
			boardQaError.value = res.error || "加载追问历史失败";
		}
		boardQaLoading.value = false;
		return res.success;
	}

	/** 问一轮（挂当前选中的版块报告；三 kind 均可）。 */
	async function askBoardQuestion(question: string): Promise<boolean> {
		const boardId = activeBoardId;
		const resultId = currentBoardResultId.value;
		if (boardId === null || resultId === null) {
			boardQaError.value = "暂无版块报告，无法追问";
			notifyError(boardQaError.value);
			return false;
		}
		const view = captureBoardView(boardId);
		boardQaLoading.value = true;
		boardQaError.value = null;
		try {
			const res = await api.askBoardQA(boardId, resultId, question);
			// 迟到守卫：await 期间切板块 / 切报告 → 静默丢弃，不重拉不 toast
			if (!boardQaStillCurrent(view, resultId)) return false;
			if (res.success) {
				boardLatestAnswer.value = res.data ?? null;
				await loadBoardQA(resultId);
				return true;
			}
			boardQaError.value = res.error || "追问失败";
			notifyError(boardQaError.value);
			return false;
		} finally {
			// loading 属当前视图：迟到响应不得提前清新报告的在途 loading
			if (boardQaStillCurrent(view, resultId)) boardQaLoading.value = false;
		}
	}

	/** 沉淀一轮（board 路由按 result+qa 定位；用回写行替换 boardQaList 项）。 */
	async function sedimentBoardAnswer(qaId: number): Promise<boolean> {
		const boardId = activeBoardId;
		const resultId = boardQaResultId.value;
		if (boardId === null || resultId === null) return false;
		const view = captureBoardView(boardId);
		const res = await api.sedimentBoardQA(boardId, resultId, qaId);
		if (!boardViewStillCurrent(view)) return false;
		if (res.success && res.data) {
			// 列表仍属同一 result 才原地替换（期间切了报告则丢弃）
			if (boardQaResultId.value === resultId) {
				const idx = boardQaList.value.findIndex((q) => q.id === qaId);
				if (idx >= 0) boardQaList.value[idx] = res.data;
			}
			notifySuccess("已沉淀");
			return true;
		}
		notifyError(res.error || "沉淀失败");
		return false;
	}

	onUnmounted(stopDebatePolling);

	return {
		// topic selector
		topics,
		topicsLoading,
		selectedTopicId,
		loadTopics,
		selectTopic,
		// table 1
		contexts,
		contextsLoading,
		regenerating,
		loadContexts,
		saveContext,
		regenerateContext,
		// table 2
		results,
		resultsLoading,
		triggering,
		loadResults,
		triggerEnrichment,
		// table 3
		reviews,
		reviewsLoading,
		loadReviews,
		saveReviewDeviation,
		applyReview,
		createReview,
		// data sources
		dataSources,
		dataSourcesLoading,
		loadDataSources,
		saveDataSource,
		removeDataSource,
		// stock debates
		debates,
		debatesLoading,
		debateTriggering,
		debateError,
		debateStage,
		loadDebates,
		triggerDebate,
		// qa (causal-analysis-agent 阶段3)
		qaList,
		qaLoading,
		qaError,
		latestAnswer,
		loadQA,
		askQuestion,
		sedimentAnswer,
		// board qa (board-level-deep-analysis 6.2)
		boardQaList,
		boardQaLoading,
		boardQaError,
		boardLatestAnswer,
		boardQaResultId,
		currentBoardResultId,
		loadBoardQA,
		askBoardQuestion,
		sedimentBoardAnswer,
		// workbench UI（原型认知工作台）
		selectedGran,
		selectedPeriodIdx,
		periodList,
		currentContext,
		setGran,
		shiftPeriod,
		selectPeriod,
		activeSection,
		setActiveSection,
		latestResultId,
		latestResultDetail,
		latestResultDetailLoading,
		loadLatestResultDetail,
		// board-level analysis (board-level-deep-analysis)
		boardResults,
		boardResultsLoading,
		boardAnalysisTriggering,
		activeBoardJob,
		latestBoardBrief,
		selectedBoardResult,
		selectedBoardResultId,
		loadBoardAnalysisResults,
		triggerBoardAnalysis,
		triggerBoardInvestigation,
		syncBoardAnalysisStatus,
		syncTopicAnalysisStatus,
		stopBoardPoll,
		activateBoardContext,
		deactivateBoardContext,
		selectBoardResult,
		// misc
		error,
		loadAllTopicTables,
	};
}
