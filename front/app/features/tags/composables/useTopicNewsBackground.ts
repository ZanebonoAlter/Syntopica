import { computed, onMounted, onUnmounted, ref } from "vue";
import {
	useBoardEnrichmentApi,
	type ContextRow,
	type ContextGranularity,
} from "~/api/boardEnrichment";
import { useConfirm } from "~/composables/useConfirm";

/**
 * 话题新闻背景独立上下文（redesign-enrichment-research-workbench 4.3 / D7）。
 *
 * 数据键 = topicId + granularity + period（周/月/年三档，旧 all API 不扩展），
 * 不依赖旧 useBoardEnrichment 的聚焦分析选择状态。四态：loading / empty / error /
 * success；编辑保存失败保留草稿；切换 topic/period 用 epoch 守卫防晚响应串台；
 * 补生成/重新汇总必须先弹确认（useConfirm → AppDialog size="sm"）。
 *
 * 与研究完全隔离：只调 contexts GET/PUT/regenerate，不发任何研究/发现请求，
 * 不读研究开关。readonly（研究工作台只读预览）由组件侧决定渲染哪些操作入口。
 */

/** 首版 UI 提供的三档周期（all 由旧界面承担，此处不扩展）。 */
export type BackgroundGranularity = Extract<ContextGranularity, "week" | "month" | "year">;

export const BACKGROUND_GRANULARITIES: readonly BackgroundGranularity[] = [
	"week",
	"month",
	"year",
] as const;

export const BACKGROUND_GRANULARITY_LABELS: Record<BackgroundGranularity, string> = {
	week: "周",
	month: "月",
	year: "年",
};

export function useTopicNewsBackground() {
	const api = useBoardEnrichmentApi();
	const { confirm } = useConfirm();

	// ── 当前数据键（topicId + granularity + period）─────────────────────────
	const topicId = ref<number | null>(null);
	const semanticBoardId = ref<number | undefined>(undefined);
	const granularity = ref<BackgroundGranularity>("week");

	// ── 四态 ────────────────────────────────────────────────────────────────
	const rows = ref<ContextRow[]>([]);
	const loading = ref(false);
	const loadError = ref<string | null>(null);

	// 周期历史翻阅（降序，最新在前）。
	const periodList = computed(() =>
		rows.value
			.map((r) => r.period)
			.slice()
			.sort()
			.reverse(),
	);
	const selectedPeriod = ref<string | null>(null);

	const currentRow = computed(
		() =>
			rows.value.find(
				(r) => r.granularity === granularity.value && r.period === selectedPeriod.value,
			) ?? null,
	);

	/** 状态机派生：loading 优先，其次 error，再按选中周期有无行分 success/empty。 */
	const phase = computed<"loading" | "error" | "empty" | "success">(() => {
		if (loading.value) return "loading";
		if (loadError.value) return "error";
		return currentRow.value ? "success" : "empty";
	});

	// ── epoch 守卫：切 topic/granularity 后晚响应全数丢弃，不串台 ─────────────
	let epoch = 0;
	function isStale(reqEpoch: number, reqTopic: number, reqGran: BackgroundGranularity): boolean {
		return (
			reqEpoch !== epoch ||
			reqTopic !== topicId.value ||
			reqGran !== granularity.value
		);
	}

	// ── 草稿：按数据键保留，切周期/话题不丢，保存失败不丢 ────────────────────
	const drafts = new Map<string, string>();
	const draft = ref("");
	const saving = ref(false);
	const saveError = ref<string | null>(null);
	const editing = ref(false);

	function draftKey(topic: number, gran: string, period: string | null): string {
		return `${topic}:${gran}:${period ?? ""}`;
	}

	/**
	 * 把当前脏稿 stash 进 drafts Map（切键/宿主关闭前调用，R-1「保留」语义：
	 * 未保存草稿不静默丢失）。非脏时幂等无害。
	 */
	function stashDraft() {
		if (topicId.value === null) return;
		const key = draftKey(topicId.value, granularity.value, selectedPeriod.value);
		if (dirty.value) drafts.set(key, draft.value);
	}

	function syncDraft() {
		const key = draftKey(topicId.value ?? 0, granularity.value, selectedPeriod.value);
		draft.value = drafts.get(key) ?? currentRow.value?.content ?? "";
	}

	const dirty = computed(() => draft.value !== (currentRow.value?.content ?? ""));

	// ── 加载（listContexts 按 granularity 过滤，供周期历史翻阅）────────────────
	async function load() {
		if (topicId.value === null) return;
		const reqEpoch = ++epoch;
		const reqTopic = topicId.value;
		const reqGran = granularity.value;

		loading.value = true;
		loadError.value = null;
		try {
			const res = await api.listContexts(reqTopic, reqGran, semanticBoardId.value);
			if (isStale(reqEpoch, reqTopic, reqGran)) return; // 晚响应不串台
			if (res.success) {
				rows.value = res.data ?? [];
				// 保留仍存在的选中周期，否则回落到最新周期。
				const periods = periodList.value;
				if (!selectedPeriod.value || !periods.includes(selectedPeriod.value)) {
					selectedPeriod.value = periods[0] ?? null;
				}
				syncDraft();
			} else {
				rows.value = [];
				selectedPeriod.value = null;
				loadError.value = res.error || "新闻背景加载失败";
			}
		} finally {
			if (!isStale(reqEpoch, reqTopic, reqGran)) loading.value = false;
		}
	}

	/** 宿主入口：选中话题后打开（topic/板块变化都会重开一轮，晚响应被丢弃）。 */
	function open(id: number, boardId?: number) {
		stashDraft(); // R-1：切话题前保留旧键脏稿，不静默丢失
		topicId.value = id;
		semanticBoardId.value = boardId;
		rows.value = [];
		selectedPeriod.value = null;
		loadError.value = null;
		editing.value = false;
		draft.value = "";
		void load();
	}

	/** 切换周期档（week/month/year），各自独立读取。 */
	function selectGranularity(gran: BackgroundGranularity) {
		if (gran === granularity.value) return;
		stashDraft(); // R-1：切粒度前保留旧键脏稿
		granularity.value = gran;
		rows.value = [];
		selectedPeriod.value = null;
		loadError.value = null;
		editing.value = false;
		void load();
	}

	/** 周期历史翻阅：换周期只换草稿视图，不重新请求。 */
	function selectPeriod(period: string) {
		stashDraft(); // R-1：切周期前保留旧键脏稿（syncDraft 按新键取回）
		selectedPeriod.value = period;
		saveError.value = null;
		syncDraft();
	}

	/** GET 失败重试。 */
	function retry() {
		void load();
	}

	// ── 编辑保存（旧 PUT 语义：content + source=manual，只动目标话题周期）──────
	async function saveEdit(): Promise<boolean> {
		const topic = topicId.value;
		const period = selectedPeriod.value;
		if (topic === null || !period) return false;

		saving.value = true;
		saveError.value = null;
		try {
			const res = await api.updateContext(
				topic,
				granularity.value,
				period,
				{ content: draft.value },
				semanticBoardId.value,
			);
			if (!res.success || !res.data) {
				// 保存失败保留草稿（draft 不动），错误就地展示。
				saveError.value = res.error || "保存失败，草稿已保留";
				return false;
			}
			// 成功：行内替换（无则插入），清掉该键草稿。
			const idx = rows.value.findIndex(
				(r) => r.granularity === granularity.value && r.period === period,
			);
			if (idx >= 0) rows.value[idx] = res.data;
			else rows.value.push(res.data);
			drafts.delete(draftKey(topic, granularity.value, period));
			draft.value = res.data.content;
			editing.value = false;
			return true;
		} finally {
			saving.value = false;
		}
	}

	function startEdit() {
		syncDraft();
		editing.value = true;
	}

	function cancelEdit() {
		editing.value = false;
		saveError.value = null;
		// 未保存的输入留在 drafts（不静默丢），再次进入编辑可续写。
		const key = draftKey(topicId.value ?? 0, granularity.value, selectedPeriod.value);
		if (dirty.value) drafts.set(key, draft.value);
	}

	// ── 补生成/重新汇总：必须先弹确认（消耗模型 + 已有内容会被覆盖）─────────────
	const regenerating = ref(false);
	const regenerateError = ref<string | null>(null);

	/**
	 * 显式生成入口。目标周期已有内容 → 文案先告知覆盖；无内容 → 仅提示消耗模型。
	 * 用户取消/关弹窗 → 零生成调用。返回是否实际触发了生成。
	 */
	async function requestRegenerate(): Promise<boolean> {
		const topic = topicId.value;
		if (topic === null || regenerating.value) return false;

		const hasContent = !!currentRow.value?.content;
		const periodLabel = selectedPeriod.value ?? "当前周期";
		const label = `话题 #${topic}`;
		const ok = await confirm({
			title: hasContent ? "重新汇总新闻背景？" : "补生成新闻背景？",
			message: hasContent
				? `将为 ${label} 的 ${periodLabel} 周期重新生成新闻背景：会调用模型（消耗预算），已有的背景内容会被覆盖。`
				: `将为 ${label} 的 ${periodLabel} 周期生成新闻背景：会调用模型（消耗预算）。`,
			confirmText: hasContent ? "重新汇总" : "生成",
		});
		if (!ok) return false; // 只浏览/取消确认 → 零生成调用

		regenerating.value = true;
		regenerateError.value = null;
		try {
			const res = await api.regenerateContext(
				topic,
				granularity.value,
				selectedPeriod.value ?? undefined,
				semanticBoardId.value,
			);
			if (!res.success || !res.data) {
				// 失败保留旧内容与错误，不伪装空态。
				regenerateError.value = res.error || "生成失败，原内容已保留";
				return false;
			}
			// 行内替换（无则插入），选中刚生成的周期。
			const idx = rows.value.findIndex(
				(r) =>
					r.granularity === res.data!.granularity && r.period === res.data!.period,
			);
			if (idx >= 0) rows.value[idx] = res.data;
			else rows.value.push(res.data);
			selectedPeriod.value = res.data.period;
			syncDraft();
			return true;
		} finally {
			regenerating.value = false;
		}
	}

	// ── 脏草稿离开保护 ──────────────────────────────────────────────────────
	// 浏览器级 beforeunload 兜底（刷新/关标签）；宿主内关闭（弹窗/路由切换）的
	// 确认交互由宿主读 dirty 自行接入——TODO(宿主接入，后续任务)：宿主关闭容器前
	// 检查 dirty 并走 useConfirm，避免静默丢弃未保存草稿。
	function onBeforeUnload(e: BeforeUnloadEvent) {
		if (dirty.value) e.preventDefault();
	}
	onMounted(() => window.addEventListener("beforeunload", onBeforeUnload));
	onUnmounted(() => window.removeEventListener("beforeunload", onBeforeUnload));

	return {
		// 数据键
		topicId,
		granularity,
		selectedPeriod,
		// 四态
		phase,
		loading,
		loadError,
		rows,
		periodList,
		currentRow,
		// 编辑与草稿
		editing,
		draft,
		dirty,
		saving,
		saveError,
		// 生成
		regenerating,
		regenerateError,
		// 动作
		open,
		selectGranularity,
		selectPeriod,
		retry,
		startEdit,
		cancelEdit,
		saveEdit,
		requestRegenerate,
		stashDraft,
	};
}
