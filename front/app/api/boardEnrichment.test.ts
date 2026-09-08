import { beforeEach, describe, expect, it, vi } from "vitest";

const { getMock, postMock, putMock, deleteMock } = vi.hoisted(() => ({
	getMock: vi.fn(),
	postMock: vi.fn(),
	putMock: vi.fn(),
	deleteMock: vi.fn(),
}));

vi.mock("./client", () => ({
	apiClient: {
		get: getMock,
		post: postMock,
		put: putMock,
		delete: deleteMock,
	},
}));

import { useBoardEnrichmentApi } from "./boardEnrichment";

describe("useBoardEnrichmentApi — endpoint contracts (data-enrichment P6)", () => {
	beforeEach(() => {
		getMock.mockReset();
		postMock.mockReset();
		putMock.mockReset();
		deleteMock.mockReset();
		getMock.mockResolvedValue({ success: true, data: {} });
		postMock.mockResolvedValue({ success: true, data: {} });
		putMock.mockResolvedValue({ success: true, data: {} });
		deleteMock.mockResolvedValue({ success: true, data: {} });
	});

	// ── Table 1: topic_lifeline_context (topic dimension) ───────────────────
	describe("table 1 — contexts", () => {
		it("listContexts → GET .../contexts（无过滤）", async () => {
			const api = useBoardEnrichmentApi();
			await api.listContexts(7);
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/contexts",
			);
		});

		it("listContexts → GET .../contexts?granularity=week（按粒度过滤，周期筛选器翻历史）", async () => {
			const api = useBoardEnrichmentApi();
			await api.listContexts(7, "week");
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/contexts?granularity=week",
			);
		});

		it("getContext → GET .../contexts/:granularity/:period", async () => {
			const api = useBoardEnrichmentApi();
			await api.getContext(7, "week", "2026-W27");
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/contexts/week/2026-W27",
			);
		});

		it("updateContext → PUT .../contexts/:granularity/:period with {content}", async () => {
			const api = useBoardEnrichmentApi();
			await api.updateContext(7, "month", "2026-06", { content: "hello" });
			expect(putMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/contexts/month/2026-06",
				{ content: "hello" },
			);
		});

		it("regenerateContext → POST .../contexts/:granularity/regenerate（当前周期）", async () => {
			const api = useBoardEnrichmentApi();
			await api.regenerateContext(7, "year");
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/contexts/year/regenerate",
			);
		});

		it("regenerateContext → POST .../contexts/:granularity/regenerate?period=…（指定周期）", async () => {
			const api = useBoardEnrichmentApi();
			await api.regenerateContext(7, "year", "2025");
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/contexts/year/regenerate?period=2025",
			);
		});
	});

	// ── Table 2: topic_enrichment_result (topic dimension) ──────────────────
	describe("table 2 — results", () => {
		it("listResults → GET .../results", async () => {
			const api = useBoardEnrichmentApi();
			await api.listResults(7);
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/results",
			);
		});

		it("getResult → GET .../results/:id (topicId kept in path per backend route)", async () => {
			const api = useBoardEnrichmentApi();
			await api.getResult(7, 42);
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/results/42",
			);
		});

		it("triggerEnrichment → POST .../results/trigger（无 body）", async () => {
			const api = useBoardEnrichmentApi();
			await api.triggerEnrichment(7);
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/results/trigger",
				undefined,
			);
		});

		it("triggerEnrichment(prefillLens) → POST with {prefill_lens}（D8 下钻）", async () => {
			const api = useBoardEnrichmentApi();
			await api.triggerEnrichment(7, "供需错配");
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/results/trigger",
				{ prefill_lens: "供需错配" },
			);
		});
	});

	// ── Table 3: topic_enrichment_review (topic dimension) ──────────────────
	describe("table 3 — reviews", () => {
		it("listReviews → GET .../reviews", async () => {
			const api = useBoardEnrichmentApi();
			await api.listReviews(7);
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/reviews",
			);
		});

		it("createReview → POST .../reviews with manual body", async () => {
			const api = useBoardEnrichmentApi();
			await api.createReview(7, {
				curr_result_id: 10,
				deviation_summary: "手动批注",
			});
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/reviews",
				{
					curr_result_id: 10,
					deviation_summary: "手动批注",
				},
			);
		});

		it("createReview forwards optional prev_result_id", async () => {
			const api = useBoardEnrichmentApi();
			await api.createReview(7, {
				curr_result_id: 10,
				deviation_summary: "x",
				prev_result_id: 9,
			});
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/reviews",
				{
					curr_result_id: 10,
					deviation_summary: "x",
					prev_result_id: 9,
				},
			);
		});

		it("updateReviewDeviation → PUT .../reviews/:id with {deviation_summary}", async () => {
			const api = useBoardEnrichmentApi();
			await api.updateReviewDeviation(7, 3, { deviation_summary: "调整" });
			expect(putMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/reviews/3",
				{ deviation_summary: "调整" },
			);
		});

		it("applyReview → POST .../reviews/:id/apply", async () => {
			const api = useBoardEnrichmentApi();
			await api.applyReview(7, 3);
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/reviews/3/apply",
			);
		});
	});

	// ── Board data sources (board dimension) ────────────────────────────────
	describe("board — data sources", () => {
		it("listDataSources → GET /semantic-boards/:id/data-sources", async () => {
			const api = useBoardEnrichmentApi();
			await api.listDataSources(1974);
			expect(getMock).toHaveBeenCalledWith(
				"/semantic-boards/1974/data-sources",
			);
		});

		it("upsertDataSource → PUT /semantic-boards/:id/data-sources with body", async () => {
			const api = useBoardEnrichmentApi();
			await api.upsertDataSource(1974, {
				source_type: "etf_quote",
				config: { keywords: ["半导体"] },
				enabled: true,
			});
			expect(putMock).toHaveBeenCalledWith(
				"/semantic-boards/1974/data-sources",
				{
					source_type: "etf_quote",
					config: { keywords: ["半导体"] },
					enabled: true,
				},
			);
		});

		it("deleteDataSource → DELETE /semantic-boards/:id/data-sources/:sourceType", async () => {
			const api = useBoardEnrichmentApi();
			await api.deleteDataSource(1974, "etf_quote");
			expect(deleteMock).toHaveBeenCalledWith(
				"/semantic-boards/1974/data-sources/etf_quote",
			);
		});
	});

	// ── Board-level analysis (board-level-deep-analysis D8 + 5.x) ─────────
	describe("board — analysis", () => {
		it("triggerBoardAnalysis → POST /semantic-boards/:id/enrichment/analysis/trigger", async () => {
			const api = useBoardEnrichmentApi();
			await api.triggerBoardAnalysis(31);
			expect(postMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/trigger",
			);
		});

		it("getAnalysisStatusByJobId → GET /enrichment/analysis-status?job_id=…（按 job_id 精确轮询）", async () => {
			const api = useBoardEnrichmentApi();
			await api.getAnalysisStatusByJobId("abc123 def");
			expect(getMock).toHaveBeenCalledWith(
				"/enrichment/analysis-status?job_id=abc123%20def",
			);
		});

		it("triggerBoardInvestigation(generated) → POST investigations/trigger with {briefing_result_id, question_id}", async () => {
			const api = useBoardEnrichmentApi();
			await api.triggerBoardInvestigation(31, {
				briefing_result_id: 88,
				question_id: "q1",
			});
			expect(postMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/investigations/trigger",
				{ briefing_result_id: 88, question_id: "q1" },
			);
		});

		it("triggerBoardInvestigation(custom) → POST investigations/trigger with {briefing_result_id, question}", async () => {
			const api = useBoardEnrichmentApi();
			await api.triggerBoardInvestigation(31, {
				briefing_result_id: 88,
				question: "两条泳道是否同一驱动？",
			});
			expect(postMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/investigations/trigger",
				{ briefing_result_id: 88, question: "两条泳道是否同一驱动？" },
			);
		});

		it("listBoardAnalysisResults → GET .../analysis/results（无 kind = 全部）", async () => {
			const api = useBoardEnrichmentApi();
			await api.listBoardAnalysisResults(31);
			expect(getMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/results",
			);
		});

		it("listBoardAnalysisResults(kind) → GET .../analysis/results?kind=board_brief", async () => {
			const api = useBoardEnrichmentApi();
			await api.listBoardAnalysisResults(31, "board_brief");
			expect(getMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/results?kind=board_brief",
			);
		});

		it("getBoardAnalysisResult → GET .../analysis/results/:rid", async () => {
			const api = useBoardEnrichmentApi();
			await api.getBoardAnalysisResult(31, 88);
			expect(getMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/results/88",
			);
		});
	});

	// ── QA: report follow-up (causal-analysis-agent P3) ─────────────────────
	describe("qa — report follow-up", () => {
		it("askQA → POST .../results/:id/qa with {question}", async () => {
			const api = useBoardEnrichmentApi();
			await api.askQA(7, 42, "油价还会涨吗");
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/results/42/qa",
				{ question: "油价还会涨吗" },
			);
		});

		it("listQA → GET .../results/:id/qa", async () => {
			const api = useBoardEnrichmentApi();
			await api.listQA(7, 42);
			expect(getMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/results/42/qa",
			);
		});

		it("sedimentQA → POST .../qa/:id/sediment", async () => {
			const api = useBoardEnrichmentApi();
			await api.sedimentQA(7, 99);
			expect(postMock).toHaveBeenCalledWith(
				"/persistent-topics/7/enrichment/qa/99/sediment",
			);
		});
	});

	// ── Board QA: 版块报告追问（board-level-deep-analysis 6.2，design D5）──
	describe("board qa — 版块报告追问（三 kind 均可，result 不可变 + QA append-only）", () => {
		it("askBoardQA → POST /semantic-boards/:id/enrichment/analysis/results/:rid/qa with {question}", async () => {
			const api = useBoardEnrichmentApi();
			await api.askBoardQA(31, 88, "旧结论还成立吗");
			expect(postMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/results/88/qa",
				{ question: "旧结论还成立吗" },
			);
		});

		it("listBoardQA → GET 同路径（多轮历史）", async () => {
			const api = useBoardEnrichmentApi();
			await api.listBoardQA(31, 88);
			expect(getMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/results/88/qa",
			);
		});

		it("sedimentBoardQA → POST .../results/:rid/qa/:qid/sediment（按 result+qa 双定位）", async () => {
			const api = useBoardEnrichmentApi();
			await api.sedimentBoardQA(31, 88, 99);
			expect(postMock).toHaveBeenCalledWith(
				"/semantic-boards/31/enrichment/analysis/results/88/qa/99/sediment",
			);
		});
	});
});
