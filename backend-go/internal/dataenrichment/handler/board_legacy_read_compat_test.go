package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── legacy_board_analysis 只读兼容（tasks 6.2 / spec「旧论文式版块报告兼容」）──
//
// testcontainer PG + 真实 legacy fixture（result_kind=legacy_board_analysis，
// sectors 含 thesis/candidates/argument/depth/lane_refs，另带 tool_calls 与
// input_snapshot jsonb）：
//   - board 列表 / kind 过滤 / 详情 / QA 按 result id 读取全部可用
//   - 跨板块详情仍 404（所有权不因兼容放松）
//   - 读取前后 PG sectors::text / tool_calls::text / input_snapshot::text
//     逐字节不变，result 无任何写入（append-only 红线：读取路径永不改写）
//
// 注意这是 handler 包里唯一的 PG 集成测试：读路径语义（PG jsonb 原样透传）
// 无法在 SQLite 上等效验证，故按 test-design 层选择规则落 testcontainer PG。

// legacyFixtureSectors mirrors the pre-brief board sectors shape (D1 五字段,
// 论文式 argument + 强制 depth + lane_refs)——与旧写链落库时的 payload 逐字段
// 同构，含嵌套数组与中文字符串以覆盖 JSON 语义稳定性。
const legacyFixtureSectors = `{"scope":"board","form":"board",
	"thesis":"存储涨价不是需求反转，而是产能纪律的重新定价",
	"angle":"概念重命名",
	"candidates":[{"thesis":"候选甲：需求反转论","hook":"钩子甲","angle":"周期反转"},{"thesis":"候选乙：产能纪律论","hook":"钩子乙","angle":"供给约束"}],
	"chosen_index":1,"reason":"候选乙覆盖泳道更全",
	"argument":{"intro":"开篇：两条泳道价格同向上行但订单未放量。","layers":[
		{"layer":"表层现象","deep_logic":"价格上行由供给收缩驱动。","basis":"态势卡#1"},
		{"layer":"传导机制","deep_logic":"原厂减产传导至现货升水。","basis":"agent 检索"}],
		"boundary":"还不能确认传导是否已闭环。","conclusion":{"cert":"medium","judgment":"错位仍在扩大。"}},
	"depth":{"system_reframe":"放进全球产能周期系统讲。","mechanism_layers":[{"layer":"产能纪律","deep_logic":"寡头协同减产。","basis":"季度报告"}],
		"historical_analogy":{"case":"2019 下行周期","mechanism":"同类协同","diff":"本次更快"},"regime_shift":null,
		"boundary":"数据未覆盖衍生品。","evidence_chain":[{"source_type":"web","url":"https://example.com/r","quote":"原文摘录一句","institution":"BIS","date":"2026-08"}]},
	"lane_refs":[{"lane_id":901,"note":"主观察"}]}`

const legacyFixtureToolCalls = `[{"tool":"web_search","args":{"query":"产能 明细"},"result_preview":"命中3条"}]`
const legacyFixtureSnapshot = `{"gate":{"refreshed":2,"budget_exhausted":0}}`

// seedLegacyResultPG inserts one legacy board row via raw SQL (bypasses any
// service-layer writer — the legacy write chain no longer exists) so the
// fixture is exactly what historical production rows look like.
func seedLegacyResultPG(t *testing.T, db *gorm.DB, boardID uint) (id uint) {
	t.Helper()
	sid := fmt.Sprintf("data_enrichment_board_%d_legacyfixture", boardID)
	if err := db.Raw(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, tool_calls, input_snapshot, session_id, created_at)
		VALUES (?, 'board', 'legacy_board_analysis', ?::jsonb, ?::jsonb, ?::jsonb, ?, now() - interval '3 days')
		RETURNING id`, boardID, legacyFixtureSectors, legacyFixtureToolCalls, legacyFixtureSnapshot, sid).Scan(&id).Error; err != nil {
		t.Fatalf("seed legacy result: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_qa WHERE topic_enrichment_result_id = ?`, id).Error
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE id = ?`, id).Error
	})
	return id
}

// legacyRowFingerprint snapshots the immutable bytes of one result row.
type legacyRowFingerprint struct {
	Sectors   string
	ToolCalls string
	Snapshot  string
	CreatedAt time.Time
}

func legacyFingerprint(t *testing.T, db *gorm.DB, id uint) legacyRowFingerprint {
	t.Helper()
	var fp legacyRowFingerprint
	if err := db.Raw(`SELECT sectors::text, COALESCE(tool_calls::text, ''), COALESCE(input_snapshot::text, ''),
		created_at FROM topic_enrichment_result WHERE id = ?`, id).
		Row().Scan(&fp.Sectors, &fp.ToolCalls, &fp.Snapshot, &fp.CreatedAt); err != nil {
		t.Fatalf("fingerprint legacy row: %v", err)
	}
	return fp
}

// jsonDeepEqual compares two JSON documents semantically (order-insensitive).
func jsonDeepEqual(t *testing.T, a, b string) {
	t.Helper()
	var ma, mb any
	if err := json.Unmarshal([]byte(a), &ma); err != nil {
		t.Fatalf("parse %q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &mb); err != nil {
		t.Fatalf("parse %q: %v", b, err)
	}
	ja, _ := json.Marshal(ma)
	jb, _ := json.Marshal(mb)
	if string(ja) != string(jb) {
		t.Fatalf("JSON semantics differ:\n%s\n%s", ja, jb)
	}
}

func newBoardLegacyReadRouter(t *testing.T, qaFactory func(db *gorm.DB) handler.QARunner) (*gin.Engine, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		t.Fatalf("create vector extension: %v", err)
	}
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repo := repository.NewRepository(db)
	var qaRunner handler.QARunner
	if qaFactory != nil {
		qaRunner = qaFactory(db)
	}
	gin.SetMode(gin.TestMode)
	h := handler.NewHandler(repo, nil, &mockOrchestrator{}, &enabledBoardConfigReader{}, nil, qaRunner, db)
	r := gin.New()
	h.RegisterRoutes(&r.RouterGroup)
	return r, db
}

func TestBoardLegacyReadCompatibility(t *testing.T) {
	r, db := newBoardLegacyReadRouter(t, nil)
	const boardID = uint(96201)
	legacyID := seedLegacyResultPG(t, db, boardID)
	// 同板块一份简报 + 他板块一份 legacy：列表混排隔离与跨板块 404 用。
	brief := briefResultRow(t, db, boardID, `[{"id":"q1","question":"后续节奏?","rationale":"r","related_lane_ids":[901]}]`)
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM topic_enrichment_result WHERE id = ?`, brief.ID).Error })
	foreignID := seedLegacyResultPG(t, db, 96299)

	before := legacyFingerprint(t, db, legacyID)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		return w
	}

	// 1. board 列表：legacy 与 brief 混排，sectors 原样透传（thesis 可见）。
	w := get(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results", boardID))
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var listEnv struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(listEnv.Data) != 2 {
		t.Fatalf("list must carry brief+legacy, got %d", len(listEnv.Data))
	}
	foundLegacy := false
	for _, row := range listEnv.Data {
		if row["result_kind"] == "legacy_board_analysis" {
			foundLegacy = true
			jb, err := json.Marshal(row["sectors"])
			if err != nil {
				t.Fatalf("marshal list sectors: %v", err)
			}
			jsonDeepEqual(t, string(jb), before.Sectors)
		}
	}
	if !foundLegacy {
		t.Fatalf("legacy row missing from list: %s", w.Body.String())
	}

	// 2. kind 过滤：legacy_board_analysis 只回 legacy 行。
	w = get(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results?kind=legacy_board_analysis", boardID))
	if w.Code != http.StatusOK {
		t.Fatalf("kind filter: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("parse kind list: %v", err)
	}
	if len(listEnv.Data) != 1 || listEnv.Data[0]["result_kind"] != "legacy_board_analysis" {
		t.Fatalf("kind=legacy_board_analysis must return exactly the legacy row, got %s", w.Body.String())
	}

	// 3. 详情：完整旧 JSON（thesis/argument/depth/lane_refs）语义不变。
	w = get(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d", boardID, legacyID))
	if w.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	var detailEnv struct {
		Data struct {
			ResultKind  string          `json:"result_kind"`
			ToolCalls   json.RawMessage `json:"tool_calls"`
			Snapshot    json.RawMessage `json:"input_snapshot"`
			Sectors     json.RawMessage `json:"sectors"`
			SessionID   string          `json:"session_id"`
			ParentNil   any             `json:"parent_result_id"`
			QuestionNil any             `json:"question_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detailEnv); err != nil {
		t.Fatalf("parse detail: %v", err)
	}
	if detailEnv.Data.ResultKind != "legacy_board_analysis" {
		t.Fatalf("detail kind: %s", detailEnv.Data.ResultKind)
	}
	jsonDeepEqual(t, string(detailEnv.Data.Sectors), before.Sectors)
	jsonDeepEqual(t, string(detailEnv.Data.ToolCalls), before.ToolCalls)
	jsonDeepEqual(t, string(detailEnv.Data.Snapshot), before.Snapshot)
	if detailEnv.Data.ParentNil != nil || detailEnv.Data.QuestionNil != nil {
		t.Fatal("legacy row must carry no parent/question_key")
	}
	var sectors map[string]any
	if err := json.Unmarshal(detailEnv.Data.Sectors, &sectors); err != nil {
		t.Fatalf("sectors not valid JSON: %v", err)
	}
	for _, key := range []string{"thesis", "candidates", "argument", "depth", "lane_refs"} {
		if _, ok := sectors[key]; !ok {
			t.Fatalf("legacy sectors missing %q", key)
		}
	}

	// 4. 跨板块仍 404（所有权不因兼容放松）。
	w = get(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d", boardID, foreignID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-board detail: want 404, got %d", w.Code)
	}

	// 5. QA 按 result id 继续工作（design D5：「只读」指 result 行不可变，
	//    QA 是独立 append-only 行）：旧 topic 路由对 board 档 result 已 404
	//    （listQA IDOR 修复），真实 QA 流在 TestBoardLegacyQAAppendOnly 验证。
	w = get(fmt.Sprintf("/persistent-topics/501/enrichment/results/%d/qa", legacyID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("topic-route qa list on board result: want 404 after IDOR fix, got %d", w.Code)
	}
	w = get(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, legacyID))
	if w.Code != http.StatusOK {
		t.Fatalf("board-route qa list: %d %s", w.Code, w.Body.String())
	}
	var qaEnv struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &qaEnv); err != nil {
		t.Fatalf("parse qa list: %v", err)
	}
	if len(qaEnv.Data) != 0 {
		t.Fatalf("legacy fixture must start with zero qa rows, got %d", len(qaEnv.Data))
	}

	// 6. 读取路径零写入：字节指纹与行数均不变（append-only 红线）。
	after := legacyFingerprint(t, db, legacyID)
	if after != before {
		t.Fatalf("legacy row mutated by read path:\nbefore=%+v\nafter=%+v", before, after)
	}
	var rowCount int
	if err := db.Raw(`SELECT COUNT(*) FROM topic_enrichment_result WHERE semantic_board_id = ?`, boardID).
		Scan(&rowCount).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 2 {
		t.Fatalf("read path must not create/remove rows, got %d", rowCount)
	}
}

// ── legacy 报告追问 QA append-only（tasks 6.2：QA 缺口修复，design D5）──────
//
// 真实 PG 上验证 D5 契约：「legacy 报告只读」指 result 行不可变，QA 是独立
// append-only 行，允许继续追问。persistingQARunner（board_qa_handler_test.go）
// 模拟 QAAgent.Ask 的可观察副作用（读 result + 落一行 source="qa" 的 QA，
// 不跑 LLM）——handler 路由/所有权/不可变指纹是本测试主角。
func TestBoardLegacyQAAppendOnly(t *testing.T) {
	r, db := newBoardLegacyReadRouter(t, func(db *gorm.DB) handler.QARunner {
		return &persistingQARunner{repo: repository.NewRepository(db)}
	})
	const boardID = uint(96202)
	legacyID := seedLegacyResultPG(t, db, boardID)
	foreignID := seedLegacyResultPG(t, db, 96298)
	// 同 board 另一份 result：sediment 跨 result 拒绝用。
	sibling := seedLegacyResultPG(t, db, boardID)

	before := legacyFingerprint(t, db, legacyID)

	post := func(path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// 1. ask：200 + 答案 + 真实落一行 QA。
	w := post(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, legacyID), `{"question":"旧结论后来怎样了"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy ask: %d %s", w.Code, w.Body.String())
	}
	var askEnv struct {
		Data struct {
			Answer string `json:"answer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &askEnv); err != nil {
		t.Fatalf("parse ask: %v", err)
	}
	if askEnv.Data.Answer == "" {
		t.Fatal("ask must return an answer")
	}
	var qaRows int
	if err := db.Raw(`SELECT COUNT(*) FROM topic_enrichment_qa WHERE topic_enrichment_result_id = ?`, legacyID).Scan(&qaRows).Error; err != nil {
		t.Fatalf("count qa: %v", err)
	}
	if qaRows != 1 {
		t.Fatalf("ask must append exactly one qa row, got %d", qaRows)
	}

	// 2. list：返回该行，sedimented=false。
	w2 := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, legacyID), nil)
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("legacy qa list: %d %s", w2.Code, w2.Body.String())
	}
	var listEnv struct {
		Data []struct {
			ID         uint   `json:"id"`
			Sedimented bool   `json:"sedimented"`
			Question   string `json:"question"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("parse qa list: %v", err)
	}
	if len(listEnv.Data) != 1 || listEnv.Data[0].Sedimented {
		t.Fatalf("qa list after ask: %+v", listEnv.Data)
	}
	qaID := listEnv.Data[0].ID

	// 3. sediment：只翻 QA flag；跨 board / 跨 result 一律 404。
	for _, bad := range []string{
		fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", 96298, legacyID, qaID),     // 跨 board
		fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardID, sibling, qaID),    // 跨 result（同 board）
		fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardID, legacyID, 999999), // 不存在 qa
	} {
		if w := post(bad, ""); w.Code != http.StatusNotFound {
			t.Fatalf("sediment ownership: want 404, got %d (%s) for %s", w.Code, w.Body.String(), bad)
		}
	}
	var flippedEarly int
	_ = db.Raw(`SELECT COUNT(*) FROM topic_enrichment_qa WHERE sedimented`).Scan(&flippedEarly).Error
	if flippedEarly != 0 {
		t.Fatalf("rejected sediment attempts must flip nothing, got %d", flippedEarly)
	}
	w3 := post(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardID, legacyID, qaID), "")
	if w3.Code != http.StatusOK {
		t.Fatalf("sediment: %d %s", w3.Code, w3.Body.String())
	}
	var sedEnv struct {
		Data struct {
			ID         uint `json:"id"`
			Sedimented bool `json:"sedimented"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &sedEnv); err != nil {
		t.Fatalf("parse sediment: %v", err)
	}
	if !sedEnv.Data.Sedimented || sedEnv.Data.ID != qaID {
		t.Fatalf("sediment result: %+v", sedEnv.Data)
	}

	// 4. result 不可变：整条 QA 流（ask/list/sediment）跑完，指纹逐字节不变。
	after := legacyFingerprint(t, db, legacyID)
	if after != before {
		t.Fatalf("legacy row mutated by qa flow:\nbefore=%+v\nafter=%+v", before, after)
	}
	// 跨 board ask 拒绝（PG 侧重复一道，与 SQLite 矩阵互补）：foreignID 属
	// 96298，从 96202 侧访问 → 404。
	if w := post(fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, foreignID), `{"question":"q"}`); w.Code != http.StatusNotFound {
		t.Fatalf("cross-board ask: want 404, got %d", w.Code)
	}
}
