import { ref } from "vue";
import { useBoardEnrichmentApi, type BoardRelationDetail, type BoardRelationRow } from "~/api/boardEnrichment";
import { useNotify } from "~/composables/useNotify";

/**
 * 跨版块关系发现 composable（add-evidence-backed-cross-board-relations 6.1）。
 *
 * 职责：从简报观察/研究问题触发发现（202/409 → job 轮询）、关系列表加载
 * （status 过滤）、详情、confirm / dismiss(reason) / re-resolve 状态动作。
 *
 * 纪律（对齐 useBoardEnrichment 的 board view epoch 守卫）：
 *  - captureBoardView {boardId, epoch}：await 返回后失配（卸载/切板块/epoch++）
 *    → 静默丢弃，不写 state、不 toast；
 *  - 同一 source 重复提交在触发前拦截（triggeringSource 非空时拒绝）；
 *  - 409 恢复：后端 data 携当前任务身份 → 按其 job_id 接管轮询；
 *  - 轮询串行排程（上一发返回才排下一发），切板块/卸载即停；
 *  - 失败/空态是正常态：relations 空数组 + 直白文案，不冒充错误。
 */

const POLL_INTERVAL_MS = 3000;

export function useBoardRelations() {
	const api = useBoardEnrichmentApi();
	const { success: notifySuccess, error: notifyError } = useNotify();

	// ── 视图身份守卫（board view epoch）──────────────────────────────────────
	let viewEpoch = 0;
	let disposed = false;

	function captureBoardView(boardId: number) {
		return { boardId, epoch: viewEpoch };
	}
	function boardViewStillCurrent(view: { boardId: number; epoch: number }): boolean {
		if (disposed) return false;
		if (view.epoch !== viewEpoch) return false;
		return true;
	}
	/** 切板块/重置：epoch++ 使所有旧捕获失效。 */
	function resetRelationView() {
		viewEpoch++;
		stopRelationPoll();
		relations.value = [];
		relationsLoading.value = false;
		relationsError.value = null;
		relationDetail.value = null;
		triggeringSource.value = null;
	}
	function disposeRelationView() {
		disposed = true;
		resetRelationView();
	}

	// ── 列表 / 详情 ──────────────────────────────────────────────────────────
	const relations = ref<BoardRelationRow[]>([]);
	const relationsLoading = ref(false);
	const relationsError = ref<string | null>(null);
	const relationDetail = ref<BoardRelationDetail | null>(null);
	const relationDetailLoading = ref(false);

	async function loadRelations(boardId: number, status?: string): Promise<boolean> {
		const view = captureBoardView(boardId);
		relationsLoading.value = true;
		relationsError.value = null;
		try {
			const res = await api.listBoardRelations(boardId, status);
			if (!boardViewStillCurrent(view)) return false;
			if (res.success && res.data) {
				relations.value = res.data;
			} else {
				if (res.status !== undefined && res.status >= 400) {
					relationsError.value = res.error ?? "关系列表加载失败";
				}
				relations.value = [];
			}
			return res.success;
		} catch {
			if (boardViewStillCurrent(view)) {
				relationsError.value = "关系列表加载失败";
				relations.value = [];
			}
			return false;
		} finally {
			if (boardViewStillCurrent(view)) relationsLoading.value = false;
		}
	}

	async function loadRelationDetail(boardId: number, relationId: number): Promise<boolean> {
		const view = captureBoardView(boardId);
		relationDetailLoading.value = true;
		try {
			const res = await api.getBoardRelation(boardId, relationId);
			if (!boardViewStillCurrent(view)) return false;
			if (res.success && res.data) {
				relationDetail.value = res.data;
			} else {
				relationDetail.value = null;
			}
			return res.success;
		} catch {
			if (boardViewStillCurrent(view)) relationDetail.value = null;
			return false;
		} finally {
			if (boardViewStillCurrent(view)) relationDetailLoading.value = false;
		}
	}

	// ── 发现触发 + job 轮询（202/409 → analysis-status）────────────────────
	const triggeringSource = ref<{ sourceKind: string; sourceKey: string } | null>(null);
	let relationPollTimer: ReturnType<typeof setTimeout> | null = null;
	let relationPollGen = 0;

	/** 停轮询（timer + gen++）但保留 triggeringSource：接管轮询/重进恢复时
	 * 占位仍在（UI 显示哪个 source 在跑 + 重复提交拦截）。 */
	function stopRelationPoll() {
		relationPollGen++;
		if (relationPollTimer) {
			clearTimeout(relationPollTimer);
			relationPollTimer = null;
		}
	}

	/** 终态清理：停轮询 + 清 source 占位（完成/失败/失效）。 */
	function finishRelationPoll() {
		stopRelationPoll();
		triggeringSource.value = null;
	}

	async function triggerDiscovery(
		boardId: number,
		payload: { briefing_result_id: number; source_kind: string; source_key: string },
	): Promise<boolean> {
		// 重复提交拦截：同面板一次只跑一个 source（后端按 source 互斥 409，
		// 这里前置拒绝省一次请求 + 防抖动）。
		if (triggeringSource.value) return false;
		const view = captureBoardView(boardId);
		triggeringSource.value = { sourceKind: payload.source_kind, sourceKey: payload.source_key };
		try {
			const res = await api.triggerRelationDiscovery(boardId, payload);
			if (!boardViewStillCurrent(view)) return false;
			if (res.success) {
				const jobId = res.data?.job_id ?? "";
				notifySuccess("关系发现已在后台运行，可离开页面");
				if (jobId) startRelationPoll(boardId, jobId);
				return true;
			}
			// 409：同 source 已在跑（或撞上其它任务）→ 按后端给的任务身份接管轮询。
			if (res.status === 409) {
				const jobId = (res.data as unknown as { job_id?: string } | undefined)?.job_id ?? "";
				notifySuccess("该来源的发现任务已在运行");
				if (jobId) startRelationPoll(boardId, jobId);
				return true;
			}
			notifyError(res.error ?? "关系发现触发失败");
			triggeringSource.value = null;
			return false;
		} catch {
			if (boardViewStillCurrent(view)) {
				notifyError("关系发现触发失败");
				triggeringSource.value = null;
			}
			return false;
		}
	}

	function startRelationPoll(boardId: number, jobId: string) {
		stopRelationPoll();
		const gen = relationPollGen;
		const view = captureBoardView(boardId);
		scheduleRelationPoll(view, jobId, gen, 0);
	}

	function scheduleRelationPoll(
		view: { boardId: number; epoch: number },
		jobId: string,
		gen: number,
		delayMs: number,
	) {
		relationPollTimer = setTimeout(() => {
			relationPollTimer = null;
			void pollRelationJobOnce(view, jobId, gen);
		}, delayMs);
	}

	async function pollRelationJobOnce(
		view: { boardId: number; epoch: number },
		jobId: string,
		gen: number,
	) {
		const res = await api.getAnalysisStatusByJobId(jobId);
		// 身份校验：await 期间切板块/停轮询 → 迟到响应丢弃。
		if (!boardViewStillCurrent(view) || gen !== relationPollGen) return;
		if (!res.success || !res.data) {
			if (res.status === 404) {
				finishRelationPoll();
				notifyError("发现任务已失效（服务可能重启过），请重新触发");
				return;
			}
			scheduleRelationPoll(view, jobId, gen, POLL_INTERVAL_MS);
			return;
		}
		const st = res.data;
		if (st.running) {
			scheduleRelationPoll(view, jobId, gen, POLL_INTERVAL_MS);
			return;
		}
		finishRelationPoll();
		if (st.error) {
			notifyError(`关系发现失败：${st.error}`);
			return;
		}
		notifySuccess("关系发现已完成");
		// 完成后重拉列表（守卫失配则不写旧板块）。
		await loadRelations(view.boardId);
	}

	// ── 生命周期动作（confirm / dismiss / re-resolve）───────────────────────
	const confirmingRelationId = ref<number | null>(null);
	const dismissingRelationId = ref<number | null>(null);
	const reResolvingRelationId = ref<number | null>(null);

	async function confirmRelation(boardId: number, relationId: number): Promise<boolean> {
		if (confirmingRelationId.value !== null) return false;
		const view = captureBoardView(boardId);
		confirmingRelationId.value = relationId;
		try {
			const res = await api.confirmBoardRelation(boardId, relationId);
			if (!boardViewStillCurrent(view)) return false;
			if (!res.success) {
				notifyError(res.error ?? "确认失败");
				return false;
			}
			notifySuccess("关系已确认（将注入下一份简报背景）");
			await loadRelations(view.boardId);
			if (relationDetail.value?.id === relationId) await loadRelationDetail(boardId, relationId);
			return true;
		} catch {
			if (boardViewStillCurrent(view)) notifyError("确认失败");
			return false;
		} finally {
			if (boardViewStillCurrent(view)) confirmingRelationId.value = null;
		}
	}

	async function dismissRelation(boardId: number, relationId: number, reason: string): Promise<boolean> {
		if (dismissingRelationId.value !== null) return false;
		const view = captureBoardView(boardId);
		dismissingRelationId.value = relationId;
		try {
			const res = await api.dismissBoardRelation(boardId, relationId, reason);
			if (!boardViewStillCurrent(view)) return false;
			if (!res.success) {
				notifyError(res.error ?? "驳回失败");
				return false;
			}
			notifySuccess("关系已驳回（同款建议将进入冷却期）");
			await loadRelations(view.boardId);
			if (relationDetail.value?.id === relationId) await loadRelationDetail(boardId, relationId);
			return true;
		} catch {
			if (boardViewStillCurrent(view)) notifyError("驳回失败");
			return false;
		} finally {
			if (boardViewStillCurrent(view)) dismissingRelationId.value = null;
		}
	}

	async function reResolveRelation(boardId: number, relationId: number): Promise<boolean> {
		if (reResolvingRelationId.value !== null) return false;
		const view = captureBoardView(boardId);
		reResolvingRelationId.value = relationId;
		try {
			const res = await api.reResolveBoardRelation(boardId, relationId);
			if (!boardViewStillCurrent(view)) return false;
			if (!res.success) {
				notifyError(res.error ?? "重解析失败");
				return false;
			}
			notifySuccess("重解析已完成");
			await loadRelations(view.boardId);
			if (relationDetail.value?.id === relationId) await loadRelationDetail(boardId, relationId);
			return true;
		} catch {
			if (boardViewStillCurrent(view)) notifyError("重解析失败");
			return false;
		} finally {
			if (boardViewStillCurrent(view)) reResolvingRelationId.value = null;
		}
	}

	return {
		// view guard
		resetRelationView,
		disposeRelationView,
		// list / detail
		relations,
		relationsLoading,
		relationsError,
		loadRelations,
		relationDetail,
		relationDetailLoading,
		loadRelationDetail,
		// trigger + poll
		triggeringSource,
		triggerDiscovery,
		// actions
		confirmingRelationId,
		dismissingRelationId,
		reResolvingRelationId,
		confirmRelation,
		dismissRelation,
		reResolveRelation,
	};
}
