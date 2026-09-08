/**
 * useBoardRelations — 跨版块关系发现 composable
 * （add-evidence-backed-cross-board-relations 6.1）。
 *
 * 覆盖：
 *  - 202 触发：triggeringSource 占位，按 job_id 精确轮询，完成重拉列表
 *  - 409 冲突：从 data 恢复当前任务轮询
 *  - 重复提交：triggeringSource 非空时直接拒绝（不发请求）
 *  - 切板块迟到响应：epoch++ 后旧响应不写 state、不 toast
 *  - 失败/404：停止轮询如实提示；列表失败不冒充空态
 *  - 空态：列表空数组 + 无 error
 *  - confirm/dismiss/re-resolve：成功刷新列表与详情，失败提示不抛
 */
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import type { BoardRelationRow } from "~/api/boardEnrichment";

const apiMocks = vi.hoisted(() => ({
	triggerRelationDiscovery: vi.fn(),
	listBoardRelations: vi.fn(),
	getBoardRelation: vi.fn(),
	confirmBoardRelation: vi.fn(),
	dismissBoardRelation: vi.fn(),
	reResolveBoardRelation: vi.fn(),
	getAnalysisStatusByJobId: vi.fn(),
}));

const notifyMocks = vi.hoisted(() => ({
	success: vi.fn(),
	error: vi.fn(),
	warn: vi.fn(),
}));

vi.mock("~/api/boardEnrichment", () => ({
	useBoardEnrichmentApi: () => apiMocks,
}));
vi.mock("~/composables/useNotify", () => ({
	useNotify: () => notifyMocks,
}));

const { useBoardRelations } = await import(
	"~/features/tags/composables/useBoardRelations"
);

type En = ReturnType<typeof useBoardRelations>;

let en: En | null = null;
let wrapper: ReturnType<typeof mount> | null = null;
const Harness = defineComponent({
	setup() {
		en = useBoardRelations();
		return () => h("div");
	},
});
function setup(): En {
	en = null;
	wrapper = mount(Harness);
	return en!;
}

function makeRelation(id: number, status = "proposed"): BoardRelationRow {
	return {
		id,
		source_board_id: 7701,
		target_board_id: 7702,
		target_concept: "日债",
		relation_type: "causal",
		claim: `关系 ${id}`,
		verification_verdict: "supported",
		quality_grade: "medium",
		status,
	};
}

function runningSt(jobId: string) {
	return {
		success: true,
		data: {
			job_id: jobId,
			job_kind: "relation_discovery",
			scope: "relation",
			target_id: 7701,
			running: true,
			finished: false,
		},
	};
}
function finishedSt(jobId: string, error?: string) {
	return {
		success: true,
		data: {
			job_id: jobId,
			job_kind: "relation_discovery",
			scope: "relation",
			target_id: 7701,
			running: false,
			finished: true,
			...(error ? { error } : {}),
		},
	};
}

beforeEach(() => {
	vi.useFakeTimers();
	vi.clearAllMocks();
	apiMocks.getAnalysisStatusByJobId.mockResolvedValue({ success: true, data: undefined });
	apiMocks.listBoardRelations.mockResolvedValue({ success: true, data: [] });
	apiMocks.getBoardRelation.mockResolvedValue({ success: true, data: null });
	apiMocks.confirmBoardRelation.mockResolvedValue({ success: true, data: makeRelation(1, "confirmed") });
	apiMocks.dismissBoardRelation.mockResolvedValue({ success: true, data: makeRelation(1, "dismissed") });
	apiMocks.reResolveBoardRelation.mockResolvedValue({ success: true, data: { new_status: "proposed" } });
});

afterEach(() => {
	wrapper?.unmount();
	wrapper = null;
	vi.useRealTimers();
});

describe("useBoardRelations — 列表 / 详情", () => {
	it("列表成功：写 relations，loading 复位", async () => {
		const e = setup();
		apiMocks.listBoardRelations.mockResolvedValueOnce({
			success: true,
			data: [makeRelation(1), makeRelation(2, "unresolved")],
		});
		await e.loadRelations(7701);
		expect(e.relations.value).toHaveLength(2);
		expect(e.relationsLoading.value).toBe(false);
		expect(e.relationsError.value).toBeNull();
		expect(apiMocks.listBoardRelations).toHaveBeenCalledWith(7701, undefined);
	});

	it("空列表是正常态：空数组、无错误", async () => {
		const e = setup();
		await e.loadRelations(7701);
		expect(e.relations.value).toEqual([]);
		expect(e.relationsError.value).toBeNull();
	});

	it("列表失败：error 文案 + 空（不冒充空态）", async () => {
		const e = setup();
		apiMocks.listBoardRelations.mockResolvedValueOnce({
			success: false,
			status: 500,
			error: "数据库罢工",
		});
		await e.loadRelations(7701);
		expect(e.relations.value).toEqual([]);
		expect(e.relationsError.value).toBe("数据库罢工");
	});

	it("详情加载：写 relationDetail；404 → null", async () => {
		const e = setup();
		apiMocks.getBoardRelation.mockResolvedValueOnce({
			success: true,
			data: { ...makeRelation(1), run: { id: 9, status: "succeeded", trigger_kind: "manual", source_kind: "observation", source_key: "o1" } },
		});
		await e.loadRelationDetail(7701, 1);
		expect(e.relationDetail.value?.run?.id).toBe(9);

		apiMocks.getBoardRelation.mockResolvedValueOnce({ success: false, status: 404 });
		await e.loadRelationDetail(7701, 99);
		expect(e.relationDetail.value).toBeNull();
	});
});

describe("useBoardRelations — 触发与轮询（202/409）", () => {
	it("202：占位 triggeringSource → 轮询 running → finished 重拉列表并复位", async () => {
		const e = setup();
		apiMocks.triggerRelationDiscovery.mockResolvedValueOnce({
			success: true,
			status: 202,
			data: { status: "started", job_id: "job-1", job_kind: "relation_discovery", scope: "relation", target_id: 7701 },
		});
		apiMocks.getAnalysisStatusByJobId
			.mockResolvedValueOnce(runningSt("job-1"))
			.mockResolvedValueOnce(finishedSt("job-1"));
		apiMocks.listBoardRelations.mockResolvedValueOnce({
			success: true,
			data: [makeRelation(1)],
		});

		const ok = await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		expect(ok).toBe(true);
		expect(e.triggeringSource.value).toEqual({ sourceKind: "observation", sourceKey: "o1" });
		expect(notifyMocks.success).toHaveBeenCalledWith("关系发现已在后台运行，可离开页面");

		await vi.advanceTimersByTimeAsync(0); // first poll → running
		await vi.advanceTimersByTimeAsync(3000); // second poll → finished
		expect(e.triggeringSource.value).toBeNull();
		expect(e.relations.value).toHaveLength(1);
		expect(notifyMocks.success).toHaveBeenCalledWith("关系发现已完成");
	});

	it("409：从 data 恢复当前任务轮询，不重复触发", async () => {
		const e = setup();
		apiMocks.triggerRelationDiscovery.mockResolvedValueOnce({
			success: false,
			status: 409,
			data: { job_id: "job-running", job_kind: "relation_discovery", running: true },
		});
		apiMocks.getAnalysisStatusByJobId.mockResolvedValueOnce(finishedSt("job-running"));
		apiMocks.listBoardRelations.mockResolvedValueOnce({ success: true, data: [] });

		const ok = await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		expect(ok).toBe(true);
		expect(notifyMocks.success).toHaveBeenCalledWith("该来源的发现任务已在运行");
		await vi.advanceTimersByTimeAsync(0);
		expect(e.triggeringSource.value).toBeNull();
	});

	it("重复提交：triggeringSource 占位中直接拒绝（零请求）", async () => {
		const e = setup();
		apiMocks.triggerRelationDiscovery.mockResolvedValue({
			success: true,
			status: 202,
			data: { status: "started", job_id: "job-x", job_kind: "relation_discovery", scope: "relation", target_id: 7701 },
		});
		apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt("job-x"));
		await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		const calls = apiMocks.triggerRelationDiscovery.mock.calls.length;
		const again = await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		expect(again).toBe(false);
		expect(apiMocks.triggerRelationDiscovery.mock.calls.length).toBe(calls);
	});

	it("触发 400：如实提示失败并复位占位", async () => {
		const e = setup();
		apiMocks.triggerRelationDiscovery.mockResolvedValueOnce({
			success: false,
			status: 400,
			error: "source_key q404 not found",
		});
		const ok = await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "question", source_key: "q404" });
		expect(ok).toBe(false);
		expect(notifyMocks.error).toHaveBeenCalledWith("source_key q404 not found");
		expect(e.triggeringSource.value).toBeNull();
	});

	it("轮询 404（服务重启）：停止并如实提示", async () => {
		const e = setup();
		apiMocks.triggerRelationDiscovery.mockResolvedValueOnce({
			success: true,
			status: 202,
			data: { status: "started", job_id: "job-2", job_kind: "relation_discovery", scope: "relation", target_id: 7701 },
		});
		apiMocks.getAnalysisStatusByJobId.mockResolvedValueOnce({ success: false, status: 404 });
		await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		await vi.advanceTimersByTimeAsync(0);
		expect(e.triggeringSource.value).toBeNull();
		expect(notifyMocks.error).toHaveBeenCalledWith("发现任务已失效（服务可能重启过），请重新触发");
	});

	it("轮询完成但带 error：停止并提示失败", async () => {
		const e = setup();
		apiMocks.triggerRelationDiscovery.mockResolvedValueOnce({
			success: true,
			status: 202,
			data: { status: "started", job_id: "job-3", job_kind: "relation_discovery", scope: "relation", target_id: 7701 },
		});
		apiMocks.getAnalysisStatusByJobId.mockResolvedValueOnce(finishedSt("job-3", "博查不可用"));
		await e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		await vi.advanceTimersByTimeAsync(0);
		expect(notifyMocks.error).toHaveBeenCalledWith("关系发现失败：博查不可用");
	});
});

describe("useBoardRelations — 视图守卫（切板块迟到响应）", () => {
	it("切板块后（resetRelationView）迟到列表响应不写 state、不 toast", async () => {
		const e = setup();
		let resolveList: (v: unknown) => void = () => {};
		apiMocks.listBoardRelations.mockReturnValueOnce(
			new Promise((r) => {
				resolveList = r;
			}),
		);
		const p = e.loadRelations(7701);
		e.resetRelationView(); // 切板块：epoch++
		resolveList({ success: true, data: [makeRelation(1)] });
		await p;
		expect(e.relations.value).toEqual([]);
		expect(e.relationsLoading.value).toBe(false);
	});

	it("触发在途切板块：迟到 202 不启动轮询、不 toast", async () => {
		const e = setup();
		let resolveTrigger: (v: unknown) => void = () => {};
		apiMocks.triggerRelationDiscovery.mockReturnValueOnce(
			new Promise((r) => {
				resolveTrigger = r;
			}),
		);
		const p = e.triggerDiscovery(7701, { briefing_result_id: 42, source_kind: "observation", source_key: "o1" });
		e.resetRelationView();
		resolveTrigger({ success: true, status: 202, data: { status: "started", job_id: "job-late", job_kind: "relation_discovery", scope: "relation", target_id: 7701 } });
		await p;
		expect(notifyMocks.success).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(5000);
		expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled();
	});

	it("unmount（disposeRelationView）后迟到响应静默丢弃", async () => {
		const e = setup();
		let resolveList: (v: unknown) => void = () => {};
		apiMocks.listBoardRelations.mockReturnValueOnce(
			new Promise((r) => {
				resolveList = r;
			}),
		);
		const p = e.loadRelations(7701);
		e.disposeRelationView();
		resolveList({ success: true, data: [makeRelation(1)] });
		await p;
		expect(e.relations.value).toEqual([]);
	});
});

describe("useBoardRelations — confirm / dismiss / re-resolve", () => {
	it("confirm 成功：刷新列表 + 当前详情", async () => {
		const e = setup();
		e.relationDetail.value = { ...makeRelation(1), run: null };
		apiMocks.listBoardRelations.mockResolvedValueOnce({ success: true, data: [makeRelation(1, "confirmed")] });
		const ok = await e.confirmRelation(7701, 1);
		expect(ok).toBe(true);
		expect(e.relations.value[0]?.status).toBe("confirmed");
		expect(notifyMocks.success).toHaveBeenCalledWith("关系已确认（将注入下一份简报背景）");
		expect(e.confirmingRelationId.value).toBeNull();
	});

	it("confirm 409（非 proposed）：提示失败不刷新", async () => {
		const e = setup();
		apiMocks.confirmBoardRelation.mockResolvedValueOnce({ success: false, status: 409, error: "relation is not confirmable" });
		const ok = await e.confirmRelation(7701, 2);
		expect(ok).toBe(false);
		expect(notifyMocks.error).toHaveBeenCalled();
		expect(apiMocks.listBoardRelations).not.toHaveBeenCalled();
	});

	it("dismiss 成功（带 reason）：刷新；空 reason 由组件层拦截（composable 透传）", async () => {
		const e = setup();
		apiMocks.listBoardRelations.mockResolvedValueOnce({ success: true, data: [] });
		const ok = await e.dismissRelation(7701, 1, "噪音");
		expect(ok).toBe(true);
		expect(apiMocks.dismissBoardRelation).toHaveBeenCalledWith(7701, 1, "噪音");
		expect(notifyMocks.success).toHaveBeenCalledWith("关系已驳回（同款建议将进入冷却期）");
	});

	it("re-resolve 成功：刷新列表与详情", async () => {
		const e = setup();
		e.relationDetail.value = { ...makeRelation(1, "unresolved"), run: null };
		apiMocks.listBoardRelations.mockResolvedValueOnce({ success: true, data: [makeRelation(1, "proposed")] });
		const ok = await e.reResolveRelation(7701, 1);
		expect(ok).toBe(true);
		expect(e.relations.value[0]?.status).toBe("proposed");
	});

	it("confirm 在途重复调用被拒绝（防双击）", async () => {
		const e = setup();
		let resolveConfirm: (v: unknown) => void = () => {};
		apiMocks.confirmBoardRelation.mockReturnValueOnce(
			new Promise((r) => {
				resolveConfirm = r;
			}),
		);
		const p = e.confirmRelation(7701, 1);
		const again = await e.confirmRelation(7701, 1);
		expect(again).toBe(false);
		resolveConfirm({ success: true, data: makeRelation(1, "confirmed") });
		await p;
	});
});
