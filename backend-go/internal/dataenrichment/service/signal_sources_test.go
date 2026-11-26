package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/models"
)

// 问题驱动研究（add-question-driven-news-research）TestQuestion 系列白盒
// fixture：协议隔离、来源冻结、先读门、证据资格、预算、引用与答题范围。
// 全部纯函数/stub，不起真模型、不连外网、不写真库；危险目标断言「请求为0」
// 以 fake fetcher 计数保证。故事锚：S1/S3/S4/S6 + 白盒附加表。

// ── fixture helpers ──────────────────────────────────────────────────────────

// qFixtureMaterial 返回带冻结来源的 question-v1 材料（两份本地来源 n1/n2）。
func qFixtureMaterial(contract string) *SignalMaterial {
	m := &SignalMaterial{
		Granularity:  repository.SignalGranularityMonth,
		Period:       "2026-09",
		AnalysisMode: repository.SignalAnalysisModeRetrospective,
		Cutoff:       time.Date(2026, 10, 1, 0, 0, 0, 0, models.ShanghaiTZ),
		Lanes: []SignalLaneMaterial{{
			LaneID: 7,
			Label:  "原油",
			Status: "active",
			Articles: []SignalArticleSlice{
				{SectionID: 101, ClusterLabel: "政策转向", ArticleCount: 5},
				{SectionID: 102, ClusterLabel: "解释分歧", ArticleCount: 3},
			},
		}},
		Gaps: []SignalMaterialGap{},
	}
	m.ResearchContract = contract
	return m
}

func qFixtureSources() []SignalFrozenSource {
	captured := time.Date(2026, 10, 2, 8, 0, 0, 0, models.ShanghaiTZ)
	pub := time.Date(2026, 9, 20, 0, 0, 0, 0, models.ShanghaiTZ)
	return []SignalFrozenSource{
		{
			SourceID: "n1", ArticleID: 501, SectionIDs: []uint{101}, LaneID: 7,
			Title: "政策转向报道A", URL: "https://news.example.com/a",
			PublishedAt: &pub, CapturedAt: captured,
			ContentHash: "hash-n1", Text: "报道A正文：政策出现转向迹象，口径尚未统一。",
			OriginalRunes: len([]rune("报道A正文：政策出现转向迹象，口径尚未统一。")),
		},
		{
			SourceID: "n2", ArticleID: 502, SectionIDs: []uint{102}, LaneID: 7,
			Title: "解释分歧报道B", URL: "https://news.example.com/b",
			PublishedAt: &pub, CapturedAt: captured,
			ContentHash: "hash-n2", Text: "报道B正文：多方对同一变化给出不同解释。",
			OriginalRunes: len([]rune("报道B正文：多方对同一变化给出不同解释。")),
		},
	}
}

func qFixtureDetection(refs ...string) SignalDetection {
	return SignalDetection{
		Signal:           "政策口径出现转向",
		WhyItMatters:     "影响后续判断的基线",
		ResearchQuestion: "这次转向的实质内容是什么",
		EvidenceRefs:     refs,
		Score:            8,
		Rationale:        "理解价值高、未知具体",
		Unknown:          "转向的具体边界尚不清楚",
		Verification:     "先读两篇原始报道核对口径",
	}
}

// ── TestQuestionProtocol：研究协议隔离（白盒表第1行）────────────────────────

func TestQuestionProtocol(t *testing.T) {
	// 旧协议（空 contract）：不建 question runtime，工具面仍只七源。
	legacy := qFixtureMaterial("")
	if rt, err := newSignalQuestionRuntime(legacy, &SignalResearchBrief{}, nil, nil); err != nil || rt != nil {
		t.Fatalf("legacy material must yield nil runtime (got %v, err %v); question tools stay closed", rt, err)
	}
	// question-v1：runtime 建立，授权 read_source/web_search/fetch_page。
	v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
	v1.Sources = qFixtureSources()
	rt, err := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1", "n2"}}, nil, nil)
	if err != nil {
		t.Fatalf("question-v1 runtime: %v", err)
	}
	if rt == nil {
		t.Fatalf("question-v1 runtime must not be nil")
	}
	for _, tool := range []string{"read_source", "web_search", "fetch_page"} {
		if !rt.authorizesTool(tool) {
			t.Fatalf("question-v1 must authorize %s", tool)
		}
	}
	if rt.authorizesTool("some_random_tool") {
		t.Fatalf("unknown tool must never be authorized")
	}
	// 未知协议：拒绝研究，不默认放开工具。
	bad := qFixtureMaterial("question-v2")
	if _, err := newSignalQuestionRuntime(bad, &SignalResearchBrief{}, nil, nil); err == nil {
		t.Fatalf("unknown research contract must be rejected, not silently downgraded")
	}
	// 协议只来自冻结快照；question-v1 的 detect 输出 unknown 必须非空。
	if _, err := ValidateSignalBriefText("unknown", "  "); err == nil {
		t.Fatalf("blank unknown must be rejected in question-v1 detect output")
	}
}

// ── TestQuestionSourceSnapshot：本地候选关联与冻结边界（白盒表第2行）────────

func TestQuestionSourceSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 2, 8, 0, 0, 0, models.ShanghaiTZ)
	pub := time.Date(2026, 9, 20, 0, 0, 0, 0, models.ShanghaiTZ)

	mkArticles := func(n int) []SignalSourceArticle {
		out := make([]SignalSourceArticle, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, SignalSourceArticle{
				ArticleID: uint(500 + i), SectionID: uint(101 + i), LaneID: 7,
				Title: fmt.Sprintf("文章%d", i), URL: fmt.Sprintf("https://news.example.com/%d", i),
				PubDate: &pub, Content: strings.Repeat("字", 100),
			})
		}
		return out
	}

	t.Run("有界8篇与第9篇记缺口", func(t *testing.T) {
		m := qFixtureMaterial("")
		for sid := uint(103); sid <= 109; sid++ {
			m.Lanes[0].Articles = append(m.Lanes[0].Articles, SignalArticleSlice{SectionID: sid, ClusterLabel: "扩展"})
		}
		dets := []SignalDetection{qFixtureDetection("101", "102", "103", "104", "105", "106", "107", "108", "109")}
		freeze := FreezeSignalQuestionSources(m, dets, mkArticles(9), now)
		if len(freeze.Sources) != SignalSourceMaxPerCandidate {
			t.Fatalf("expected at most %d frozen sources, got %d", SignalSourceMaxPerCandidate, len(freeze.Sources))
		}
		if len(freeze.Briefs) != 1 || len(freeze.Briefs[0].SourceIDs) != SignalSourceMaxPerCandidate {
			t.Fatalf("brief must cap source_ids at %d, got %v", SignalSourceMaxPerCandidate, freeze.Briefs[0].SourceIDs)
		}
		if len(freeze.Gaps) == 0 {
			t.Fatalf("9th article must be recorded as gap, not silently dropped")
		}
	})

	t.Run("同article跨section去重", func(t *testing.T) {
		m := qFixtureMaterial("")
		dets := []SignalDetection{qFixtureDetection("101", "102")}
		articles := []SignalSourceArticle{
			{ArticleID: 501, SectionID: 101, LaneID: 7, Title: "同文", PubDate: &pub, Content: "正文"},
			{ArticleID: 501, SectionID: 102, LaneID: 7, Title: "同文", PubDate: &pub, Content: "正文"},
		}
		freeze := FreezeSignalQuestionSources(m, dets, articles, now)
		if len(freeze.Sources) != 1 {
			t.Fatalf("same article across two sections must dedupe to 1 source, got %d", len(freeze.Sources))
		}
		if len(freeze.Sources[0].SectionIDs) != 2 {
			t.Fatalf("deduped source must keep both section ids, got %v", freeze.Sources[0].SectionIDs)
		}
		if strings.Count(freeze.Sources[0].Text, "正文") != 1 {
			t.Fatalf("deduped article must not duplicate body text")
		}
	})

	t.Run("8000rune截取与标记", func(t *testing.T) {
		m := qFixtureMaterial("")
		dets := []SignalDetection{qFixtureDetection("101")}
		long := strings.Repeat("甲", SignalSourceMaxArticleRunes+1)
		articles := []SignalSourceArticle{{ArticleID: 501, SectionID: 101, LaneID: 7, Title: "长文", PubDate: &pub, Content: long}}
		freeze := FreezeSignalQuestionSources(m, dets, articles, now)
		src := freeze.Sources[0]
		if len([]rune(src.Text)) != SignalSourceMaxArticleRunes {
			t.Fatalf("expected exactly %d runes, got %d", SignalSourceMaxArticleRunes, len([]rune(src.Text)))
		}
		if !src.Truncated || src.OriginalRunes != SignalSourceMaxArticleRunes+1 {
			t.Fatalf("truncation must be flagged with original rune count, got truncated=%v original=%d", src.Truncated, src.OriginalRunes)
		}
	})

	t.Run("周期后pub_date拒绝", func(t *testing.T) {
		m := qFixtureMaterial("")
		dets := []SignalDetection{qFixtureDetection("101")}
		late := m.Cutoff.Add(time.Hour)
		articles := []SignalSourceArticle{{ArticleID: 501, SectionID: 101, LaneID: 7, Title: "迟到", PubDate: &late, Content: "正文"}}
		freeze := FreezeSignalQuestionSources(m, dets, articles, now)
		if len(freeze.Sources) != 0 {
			t.Fatalf("article published after cutoff must not freeze, got %d", len(freeze.Sources))
		}
		if len(freeze.Gaps) == 0 {
			t.Fatalf("late article must be recorded as gap")
		}
	})

	t.Run("未知日期不当作合窗原文", func(t *testing.T) {
		m := qFixtureMaterial("")
		dets := []SignalDetection{qFixtureDetection("101")}
		articles := []SignalSourceArticle{{ArticleID: 501, SectionID: 101, LaneID: 7, Title: "无日期", PubDate: nil, Content: "正文"}}
		freeze := FreezeSignalQuestionSources(m, dets, articles, now)
		if len(freeze.Sources) == 0 && len(freeze.Gaps) == 0 {
			t.Fatalf("unknown-date article must either freeze with note or record a gap")
		}
		for _, s := range freeze.Sources {
			if s.PublishedAt == nil && s.TemporalNote == "" {
				t.Fatalf("unknown-date frozen source must carry a temporal_note, got %+v", s)
			}
		}
	})
}

// ── TestQuestionSnapshotAtomicity：冻结快照不随后续改动（白盒表第3行）──────

func TestQuestionSnapshotAtomicity(t *testing.T) {
	m := qFixtureMaterial("")
	m.Sources = qFixtureSources()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var revived SignalMaterial
	if err := json.Unmarshal(raw, &revived); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if revived.Sources[0].Text != "报道A正文：政策出现转向迹象，口径尚未统一。" {
		t.Fatalf("frozen snapshot text must survive round-trip, got %q", revived.Sources[0].Text)
	}
	// brief JSON 形状：unknown/verification/source_ids 三键。
	brief := &SignalResearchBrief{Unknown: "u", Verification: "v", SourceIDs: []string{"n1"}}
	bj, err := brief.ToJSON()
	if err != nil {
		t.Fatalf("brief json: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(bj, &probe); err != nil {
		t.Fatalf("brief json decode: %v", err)
	}
	for _, k := range []string{"unknown", "verification", "source_ids"} {
		if _, ok := probe[k]; !ok {
			t.Fatalf("brief json must carry %q", k)
		}
	}
	// brief 校验：600rune 边界（599/600 合法，601 拒绝）。
	ok600 := strings.Repeat("正", 600)
	if _, err := ValidateSignalBriefText("unknown", ok600); err != nil {
		t.Fatalf("600 runes must pass: %v", err)
	}
	if _, err := ValidateSignalBriefText("unknown", strings.Repeat("正", 601)); err == nil {
		t.Fatalf("601 runes must be rejected")
	}
}

// ── TestQuestionReadFirst：先读门（白盒表第4行）──────────────────────────────

func TestQuestionReadFirst(t *testing.T) {
	v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
	v1.Sources = qFixtureSources()
	brief := &SignalResearchBrief{Unknown: "u", Verification: "v", SourceIDs: []string{"n1", "n2"}}
	rt, err := newSignalQuestionRuntime(v1, brief, nil, nil)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}

	// 存在可读来源且未读：外搜/七源/calculate 全部被拦。
	if err := rt.gateTool("web_search"); err == nil {
		t.Fatalf("web_search must be gated before first successful read_source")
	}
	if err := rt.gateTool("yahoo_chart"); err == nil {
		t.Fatalf("data sources must be gated before first successful read_source")
	}
	if err := rt.gateTool("calculate"); err == nil {
		t.Fatalf("calculate must be gated before first successful read_source")
	}
	if err := rt.gateTool("read_source"); err != nil {
		t.Fatalf("read_source itself must never be gated: %v", err)
	}

	// 初始 prompt 只含目录：目录行不含正文。
	catalog := rt.sourceCatalog()
	if strings.Contains(catalog, "报道A正文") {
		t.Fatalf("catalog must not leak source body text, got: %s", catalog)
	}
	for _, id := range []string{"n1", "n2"} {
		if !strings.Contains(catalog, id) {
			t.Fatalf("catalog must list %s (id/title/url/date)", id)
		}
	}

	// 白名单只读本候选来源；未知 id 拒绝。
	if _, err := rt.readSource("n9"); err == nil {
		t.Fatalf("read_source of unknown id must be rejected")
	}
	// 成功读取一份后放行。
	res, err := rt.readSource("n1")
	if err != nil {
		t.Fatalf("readSource(n1): %v", err)
	}
	if !strings.Contains(res, "报道A正文") {
		t.Fatalf("readSource must return the frozen text, got %q", res)
	}
	if err := rt.gateTool("web_search"); err != nil {
		t.Fatalf("web_search must be allowed after first read: %v", err)
	}
	if err := rt.gateTool("calculate"); err != nil {
		t.Fatalf("calculate must be allowed after first read: %v", err)
	}

	// 无存文：不外搜死锁（gate 放行，gap 由调用方记录）。
	empty := qFixtureMaterial(SignalResearchContractQuestionV1)
	empty.Sources = nil
	rt2, err := newSignalQuestionRuntime(empty, &SignalResearchBrief{Unknown: "u", Verification: "v"}, nil, nil)
	if err != nil {
		t.Fatalf("runtime without sources: %v", err)
	}
	if err := rt2.gateTool("web_search"); err != nil {
		t.Fatalf("no frozen text → web_search must pass with recorded gap, got %v", err)
	}
}

// ── TestQuestionEvidenceEligibility：证据资格（白盒表第5行）─────────────────

func TestQuestionEvidenceEligibility(t *testing.T) {
	v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
	v1.Sources = qFixtureSources()
	rt, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1"}}, nil, nil)

	ledger := newSignalResearchLedger(v1.Cutoff)
	rt.bindLedger(ledger)

	// 搜索 snippet 不可引用：登记 URL 不产生证据（可引用宇宙 = ledger
	// sources，行为断言经 sourceIndex——成文校验消费的就是它）。
	rt.registerSearchHits([]WebSearchResult{{Title: "t", URL: "https://x.example.com/1", Snippet: "只有摘要"}})
	if ids := ledger.sourceIndex(); len(ids) != 0 {
		t.Fatalf("search snippet must not produce citable evidence, got %v", ids)
	}
	// 登记的 URL 才可 fetch（未登记 URL 直接被拒，行为经 fetchPage 入口闸）。
	if _, err := rt.fetchPage(t.Context(), "https://unregistered.example.com/x"); err == nil || !strings.Contains(err.Error(), "未登记") {
		t.Fatalf("unregistered URL must be rejected by fetch_page, got %v", err)
	}

	// 未读取的 n1 不入证据宇宙；读取后进入。
	if _, ok := ledger.sourceIndex()["n1"]; ok {
		t.Fatalf("unread local source must not be citable")
	}
	if _, err := rt.readSource("n1"); err != nil {
		t.Fatalf("readSource: %v", err)
	}
	if _, ok := ledger.sourceIndex()["n1"]; !ok {
		t.Fatalf("successfully read local source must be citable")
	}
}

// ── TestQuestionSourceTime：外部来源时间边界（白盒表第7行，纯函数半边）──────

func TestQuestionSourceTime(t *testing.T) {
	cutoff := time.Date(2026, 10, 1, 0, 0, 0, 0, models.ShanghaiTZ)

	// cutoff−1ns 接受；恰等于接受；+1ns 拒绝。
	for _, tc := range []struct {
		off time.Duration
		ok  bool
	}{
		{-time.Nanosecond, true},
		{0, true},
		{time.Nanosecond, false},
	} {
		at := cutoff.Add(tc.off)
		if got := !signalWebEvidenceTime(&at, nil).After(cutoff); got != tc.ok {
			t.Fatalf("offset %v: eligible=%v want %v", tc.off, got, tc.ok)
		}
	}

	// dateOnly：上海时区日末保守判断（日末 ≤ cutoff 才合格）。
	end, ok := signalDateOnlyEndOfDay("2026-09-30")
	if !ok {
		t.Fatalf("dateOnly parse failed")
	}
	if end.In(models.ShanghaiTZ).Format("2006-01-02 15:04:05") != "2026-09-30 23:59:59" {
		t.Fatalf("dateOnly end-of-day must be the last instant of that day in ShanghaiTZ, got %v", end)
	}
	if end.After(cutoff) {
		t.Fatalf("2026-09-30 end must be ≤ 2026-10-01 cutoff")
	}
	octEnd, _ := signalDateOnlyEndOfDay("2026-10-01")
	if !octEnd.After(cutoff) {
		t.Fatalf("2026-10-01 end must be after 2026-10-01 00:00 cutoff (conservative)")
	}

	// dateModified 更晚 → 取更晚值判断（不能用更早值绕过）。
	published := time.Date(2026, 9, 10, 0, 0, 0, 0, models.ShanghaiTZ)
	modified := time.Date(2026, 10, 5, 0, 0, 0, 0, models.ShanghaiTZ)
	effective := signalWebEvidenceTime(&published, &modified)
	if !effective.Equal(modified) {
		t.Fatalf("conflict must resolve to the LATER parseable value, got %v", effective)
	}
	if !effective.After(cutoff) {
		t.Fatalf("later-than-cutoff effective date must be ineligible")
	}
}

// ── TestQuestionSourceBudget：3搜/5页预算（白盒表第8行）─────────────────────

type qFakeSearcher struct {
	calls int
	fail  bool
}

func (s *qFakeSearcher) Search(_ context.Context, _ string) ([]WebSearchResult, error) {
	s.calls++
	if s.fail {
		return nil, fmt.Errorf("backend down")
	}
	return []WebSearchResult{{Title: "t", URL: "https://x.example.com/hit", Snippet: "s"}}, nil
}

func TestQuestionSourceBudget(t *testing.T) {
	v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
	v1.Sources = qFixtureSources()
	searcher := &qFakeSearcher{}
	rt, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1"}}, searcher, nil)
	if _, err := rt.readSource("n1"); err != nil {
		t.Fatalf("read first: %v", err)
	}

	// 3 次搜索内执行；第 4 次拒绝。
	for i := 0; i < SignalQuestionMaxWebSearches; i++ {
		if _, err := rt.webSearch(context.Background(), "主题"+fmt.Sprint(i)); err != nil {
			t.Fatalf("search #%d must execute: %v", i+1, err)
		}
	}
	if _, err := rt.webSearch(context.Background(), "第四次"); err == nil {
		t.Fatalf("4th search must be rejected (budget)")
	}
	if searcher.calls != SignalQuestionMaxWebSearches {
		t.Fatalf("only %d searches must reach the backend, got %d", SignalQuestionMaxWebSearches, searcher.calls)
	}

	// 5 次网页读取内执行；第 6 次拒绝（fake fetcher 计数）。
	fetcher := &qFakePageFetcher{body: `<html><head><meta property="article:published_time" content="2026-09-01T00:00:00Z"></head><body><p>正文内容</p></body></html>`}
	rt2, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1"}}, searcher, fetcher)
	if _, err := rt2.readSource("n1"); err != nil {
		t.Fatalf("read first: %v", err)
	}
	for i := 0; i < SignalQuestionMaxWebPages; i++ {
		url := fmt.Sprintf("https://x.example.com/page%d", i)
		rt2.registerSearchHits([]WebSearchResult{{Title: "t", URL: url, Snippet: "s"}})
		if _, err := rt2.fetchPage(context.Background(), url); err != nil {
			t.Fatalf("page #%d must execute: %v", i+1, err)
		}
	}
	rt2.registerSearchHits([]WebSearchResult{{Title: "t", URL: "https://x.example.com/page6", Snippet: "s"}})
	if _, err := rt2.fetchPage(context.Background(), "https://x.example.com/page6"); err == nil {
		t.Fatalf("6th page fetch must be rejected (budget)")
	}
	if fetcher.calls != SignalQuestionMaxWebPages {
		t.Fatalf("only %d fetches must reach the network, got %d", SignalQuestionMaxWebPages, fetcher.calls)
	}

	// 失败也计入预算（不冒充成功）。
	failSearcher := &qFakeSearcher{fail: true}
	rt3, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1"}}, failSearcher, nil)
	if _, err := rt3.readSource("n1"); err != nil {
		t.Fatalf("read first: %v", err)
	}
	for i := 0; i < SignalQuestionMaxWebSearches; i++ {
		if _, err := rt3.webSearch(context.Background(), "主题"); err == nil {
			t.Fatalf("failing search #%d must surface error, not fake success", i+1)
		}
	}
	if _, err := rt3.webSearch(context.Background(), "再来"); err == nil {
		t.Fatalf("failed searches must still consume budget")
	}
}

// ── TestQuestionSourceText：截取与空正文（白盒表第9行）──────────────────────

func TestQuestionSourceText(t *testing.T) {
	// 网页正文 4000rune 上限 + 原字符数标记。
	long := strings.Repeat("页", SignalWebPageMaxRunes+10)
	text, truncated, original := boundSignalWebText(long)
	if len([]rune(text)) != SignalWebPageMaxRunes {
		t.Fatalf("web text must cap at %d runes, got %d", SignalWebPageMaxRunes, len([]rune(text)))
	}
	if !truncated || original != SignalWebPageMaxRunes+10 {
		t.Fatalf("web truncation flag/orig runes wrong: %v %d", truncated, original)
	}
	// 空正文不可用。
	if signalUsableWebText("   ") {
		t.Fatalf("whitespace-only page text must be unusable")
	}
	// HTML 不执行：正文以纯文本形式保存（检测不到脚本标签）。
	fetcher := &qFakePageFetcher{body: "<html><head><meta property=\"article:published_time\" content=\"2026-09-01T00:00:00Z\"></head><body><p>第一段</p><script>alert('x')</script></body></html>"}
	res, err := fetcher.FetchPage(context.Background(), "https://x.example.com/html")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if strings.Contains(res.Text, "<script>") || strings.Contains(res.Text, "alert(") {
		t.Fatalf("web text must not retain executable HTML, got %q", res.Text)
	}
	if !strings.Contains(res.Text, "第一段") {
		t.Fatalf("web text must keep readable body, got %q", res.Text)
	}
}

// ── TestQuestionSourceReferences：成文引用验证（白盒表第10行）───────────────

func TestQuestionSourceReferences(t *testing.T) {
	v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
	v1.Sources = qFixtureSources()
	rt, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1"}}, nil, nil)
	ledger := newSignalResearchLedger(v1.Cutoff)
	rt.bindLedger(ledger)
	if _, err := rt.readSource("n1"); err != nil {
		t.Fatalf("read: %v", err)
	}

	view := &signalLedgerView{obsByID: ledger.obsIndex, calcByID: map[string]*SignalCalculationResult{}, newsRefs: map[string]bool{}, sourcesByID: ledger.sourceIndex()}
	// 行为断言走生产单防线：段落校验对每个 token 调 resolveSignalRef。
	resolveAll := func(text string) error {
		for _, ref := range collectSignalRefs(text) {
			if ref.kind != "source" {
				continue
			}
			if err := resolveSignalRef(ref, view); err != nil {
				return err
			}
		}
		return nil
	}
	// 已读 n1 可引用。
	if err := resolveAll("结论见 [[source:n1]]。"); err != nil {
		t.Fatalf("cited read source must resolve: %v", err)
	}
	// 未读 n2 拒绝。
	if err := resolveAll("[[source:n2]]"); err == nil {
		t.Fatalf("unread source must be rejected")
	}
	// 他 run/未知 id 拒绝。
	if err := resolveAll("[[source:n9]]"); err == nil {
		t.Fatalf("dangling source ref must be rejected")
	}
	// 搜索 snippet 不产生可引用 id：w 前缀只属于成功 fetch_page。
	rt.registerSearchHits([]WebSearchResult{{Title: "t", URL: "https://x.example.com/1", Snippet: "s"}})
	if err := resolveAll("[[source:w1]]"); err == nil {
		t.Fatalf("snippet-only source must not be citable")
	}
	// source 进 calculate 拒绝（calculate 输入只认 observation_id 形态）。
	res := runSignalCalculation(ledger.obsIndex, SignalCalculationRequest{Op: SignalCalcOpMean, Inputs: []string{"n1"}, Question: "q"}, "k1", time.Now())
	if res.Status == SignalCalcStatusOK {
		t.Fatalf("source id must never be a valid calculate input")
	}
}

// ── TestQuestionAnswerContract：答题范围（白盒表第11行）─────────────────────

func TestQuestionAnswerContract(t *testing.T) {
	// 三枚举合法。
	for _, s := range []string{"explained", "partial", "unresolved"} {
		if err := validateSignalAnswerShape(&SignalReportAnswer{Status: s, Reason: "理由", Unanswered: []string{"未答部分"}}); err != nil {
			t.Fatalf("status %s must be valid: %v", s, err)
		}
	}
	// unknown 状态拒绝。
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "unknown", Reason: "r"}); err == nil {
		t.Fatalf("unknown answer status must be rejected")
	}
	// 空 reason 拒绝；超 400rune 拒绝。
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "explained", Reason: "  "}); err == nil {
		t.Fatalf("blank reason must be rejected")
	}
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "explained", Reason: strings.Repeat("长", 401)}); err == nil {
		t.Fatalf("reason over 400 runes must be rejected")
	}
	// partial/unresolved 必须有非空 unanswered；每项 ≤300rune、最多5项。
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "partial", Reason: "r", Unanswered: nil}); err == nil {
		t.Fatalf("partial without unanswered must be rejected")
	}
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "unresolved", Reason: "r", Unanswered: []string{"  "}}); err == nil {
		t.Fatalf("blank unanswered item must be rejected")
	}
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "unresolved", Reason: "r", Unanswered: []string{strings.Repeat("长", 301)}}); err == nil {
		t.Fatalf("unanswered item over 300 runes must be rejected")
	}
	if err := validateSignalAnswerShape(&SignalReportAnswer{Status: "unresolved", Reason: "r", Unanswered: []string{"a", "b", "c", "d", "e", "f"}}); err == nil {
		t.Fatalf("more than 5 unanswered items must be rejected")
	}
	// explained 无证据拒绝；unresolved 可成文（只有缺口也无妨）。
	if err := validateSignalAnswerEvidence(&SignalReportAnswer{Status: "explained", Reason: "r"}, false); err == nil {
		t.Fatalf("explained without any evidence must be rejected")
	}
	if err := validateSignalAnswerEvidence(&SignalReportAnswer{Status: "explained", Reason: "r"}, true); err != nil {
		t.Fatalf("explained with evidence must pass: %v", err)
	}
	if err := validateSignalAnswerEvidence(&SignalReportAnswer{Status: "unresolved", Reason: "r", Unanswered: []string{"x"}}, false); err != nil {
		t.Fatalf("unresolved must be writable without evidence: %v", err)
	}
	// answer 文案计入 3000 可见字符预算（上限不变）。
	if signalReportMaxVisibleRunes != 3000 {
		t.Fatalf("visible budget must stay 3000")
	}
}

// TestQuestionReadFirstGatesSevenSourcesAtPolicy：H1——先读门必须在 policy
// CheckCall 总闸把守七源调用（不只是 runtime 工具内部）：question-v1 未读时
// call_tool yahoo_chart 被 Blocked(question_read_first)，工具执行为 0；读取后
// 放行。旧协议（q=nil）七源不受门影响。
func TestQuestionReadFirstGatesSevenSourcesAtPolicy(t *testing.T) {
	v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
	v1.Sources = qFixtureSources()
	rt, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{Unknown: "u", Verification: "v", SourceIDs: []string{"n1"}}, nil, nil)
	ledger := newSignalResearchLedger(v1.Cutoff)
	rt.bindLedger(ledger)
	pol := newSignalResearchPolicy(ledger)
	pol.q = rt

	dec := func(tool string) map[string]any {
		return map[string]any{"tool": tool, "question": "先核对一个数据点"}
	}

	// 未读：七源 call_tool 被 question_read_first 拦截（此前从 else-if 分支放行）。
	v := pol.CheckCall(1, dec("yahoo_chart"))
	if !v.Blocked || v.BlockedReason != "question_read_first" {
		t.Fatalf("seven-source call must be gated by read-first before any read, got %+v", v)
	}
	// 非法工具仍是白名单拦截（不被先读门掩盖）。
	v = pol.CheckCall(2, dec("rm_rf"))
	if !v.Blocked || v.BlockedReason != "tool_not_in_research_whitelist" {
		t.Fatalf("unknown tool must stay whitelist-blocked, got %+v", v)
	}
	// 读取后放行。
	if _, err := rt.readSource("n1"); err != nil {
		t.Fatalf("read: %v", err)
	}
	if v = pol.CheckCall(3, dec("yahoo_chart")); v.Blocked {
		t.Fatalf("seven-source call must pass after first successful read, got %+v", v)
	}

	// 旧协议：policy.q=nil，七源调用不受先读门影响（语义零变化）。
	legacy := qFixtureMaterial("")
	ledger2 := newSignalResearchLedger(legacy.Cutoff)
	pol2 := newSignalResearchPolicy(ledger2)
	if v := pol2.CheckCall(1, dec("yahoo_chart")); v.Blocked && v.BlockedReason == "question_read_first" {
		t.Fatalf("legacy seven-source call must never hit read-first gate, got %+v", v)
	}
}
