package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
)

// ── improve-discovery-recommendations 4.3：双路召回测试（S4 故事）──
//
// 锚 spec feed-discovery「Independent Board and Behavior Recall」三 Scenario：
// 近期阅读不能挤掉版块方向 / 单路缺失与重复候选 / 模型不兼容。
// 白盒分支（test-cases.md S4）：资格过滤在截 top-8 之前；配额基础8/行为8/seed 预算；
// 批次 ≤20；全局桶独立批次不冒充版块。

// recallRoute 是一条带候选向量的 fixture 路由。
type recallRoute struct {
	routeID     uint
	candidateID uint
}

// setupRecallRoute 建单条 ok 路由 + 候选向量（model 固定 test），返回引用。
func setupRecallRoute(t *testing.T, db *gorm.DB, name string, vec []float64) recallRoute {
	t.Helper()
	r := models.RSSHubRoute{
		Namespace: "rn", Path: "/r" + name, Name: name,
		Example: "/rn/r" + name, UsableDirectly: true, Status: "ok", Parameters: "{}",
	}
	require.NoError(t, db.Create(&r).Error)
	candID := setupCandidateVector(t, db, r.ID, r.Namespace, r.Path, "test", vec)
	return recallRoute{routeID: r.ID, candidateID: candID}
}

// captureAllChatFn 返回全选 chatFn 并把每次 prompt 存入 prompts（供批次断言）。
func captureAllChatFn(prompts *[]string) func(airouter.ChatRequest) (string, error) {
	return func(req airouter.ChatRequest) (string, error) {
		*prompts = append(*prompts, req.Messages[0].Content)
		return selectAllChatFn()(req)
	}
}

// promptIDSet 解析 prompt 中「- id=N |」行 → id 集合。
func promptIDSet(prompt string) map[uint]bool {
	out := map[uint]bool{}
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- id=") {
			continue
		}
		rest := line[len("- id="):]
		if i := strings.IndexByte(rest, ' '); i > 0 {
			rest = rest[:i]
		}
		if n, err := strconv.ParseUint(rest, 10, 64); err == nil {
			out[uint(n)] = true
		}
	}
	return out
}

// countPromptIDs 统计 prompt 候选行数。
func countPromptIDs(prompt string) int {
	return len(promptIDSet(prompt))
}

// gradedVec 构造与 [1,0,0] 夹角随 eps 增大的单位向量（距离单调、无并列）。
func gradedVec(eps float64) []float64 {
	return normalizeVector([]float64{1, eps, 0})
}

// TestRefresh_BoardBasePathGuaranteedNotSqueezed（S4 主链路步 1：近期阅读不能挤掉
// 版块方向）：日本新闻版块 label 向量 [1,0,0]，行为画像偏财经 [0,1,0]；8 条版块向
// 候选 + 10 条财经候选。版块批合并两路（基础 8 + 行为 8），行为路高分候选不挤占
// 基础路保底名额——8 条版块基础候选全部进入精排，同时行为路 top-8 并存。
func TestRefresh_BoardBasePathGuaranteedNotSqueezed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID := setupSeedLabel(t, db, "日本新闻", "s4-jp-news", "board", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: &boardID, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{0, 1, 0}), Dimension: 3, Model: "test",
		TagWeights: models.MetadataMap{"财经": 0.9},
	}).Error)
	boardRoutes := make([]uint, 0, 8)
	for i := 0; i < 8; i++ {
		r := setupRecallRoute(t, db, fmt.Sprintf("jp%d", i), []float64{1, 0, 0})
		boardRoutes = append(boardRoutes, r.candidateID)
	}
	for i := 0; i < 10; i++ {
		setupRecallRoute(t, db, fmt.Sprintf("fin%d", i), []float64{0, 1, 0})
	}

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	require.Len(t, prompts, 1, "单版块单批（基础路+行为路合并），无全局/seed 批")
	ids := promptIDSet(prompts[0])
	require.Len(t, ids, 16, "基础 8 + 行为 8 去重后 16 候选（两路完全不相交）")
	for _, id := range boardRoutes {
		require.Contains(t, ids, id, "版块基础路 8 条全部保留进精排（保底不被行为路挤占）")
	}

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 16, "精排全选后基础路与行为路候选并存发布")
}

// TestRefresh_BoardBatchQualificationFilterBeforeTop8（S4 白盒：资格过滤在截 top-8
// 之前）：5 条距离最近的候选分别因 broken / 关推荐开关 / 已订阅 / 冷却中 / 长期排除
// 被过滤，8 条较远的干净候选照常入批（若过滤发生在截断后，批内只剩 3 条干净候选）。
func TestRefresh_BoardBatchQualificationFilterBeforeTop8(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupSeedLabel(t, db, "日本新闻", "s4-jp-news", "board", []float64{1, 0, 0})
	// 5 条被过滤的最近候选（eps 越小距离越近）。
	broken := setupRecallRoute(t, db, "f0", gradedVec(0.0002))
	require.NoError(t, db.Model(&models.RSSHubRoute{}).Where("id = ?", broken.routeID).
		Update("status", "broken").Error)

	disabled := setupRecallRoute(t, db, "f1", gradedVec(0.0004))
	off := false
	require.NoError(t, db.Model(&models.FeedCandidate{}).Where("id = ?", disabled.candidateID).
		Update("recommendation_enabled", &off).Error)

	subscribed := setupRecallRoute(t, db, "f2", gradedVec(0.0006))
	var subRoute models.RSSHubRoute
	require.NoError(t, db.First(&subRoute, subscribed.routeID).Error)
	require.NoError(t, db.Create(&models.Feed{Title: "已订", URL: DefaultRSSHubBaseURL + subRoute.Example}).Error)

	snoozed := setupRecallRoute(t, db, "f3", gradedVec(0.0008))
	require.NoError(t, db.Create(&models.CandidatePreference{
		CandidateID: snoozed.candidateID, SnoozedUntil: ptrTime(time.Now().Add(24 * time.Hour)),
	}).Error)

	excluded := setupRecallRoute(t, db, "f4", gradedVec(0.0010))
	require.NoError(t, db.Create(&models.CandidatePreference{
		CandidateID: excluded.candidateID, ExcludedAt: ptrTime(time.Now()),
	}).Error)

	// 8 条较远的干净候选（相似度排在被过滤者之后）。
	clean := make([]uint, 0, 8)
	for i := 0; i < 8; i++ {
		r := setupRecallRoute(t, db, fmt.Sprintf("c%d", i), gradedVec(0.02*float64(i+1)))
		clean = append(clean, r.candidateID)
	}

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	require.Len(t, prompts, 1)
	ids := promptIDSet(prompts[0])
	require.Len(t, ids, 8, "被过滤后不足 8 不凑满：批内恰为 8 条干净候选（13 条候选 - 5 条过滤 = 8）")
	for _, f := range []recallRoute{broken, disabled, subscribed, snoozed, excluded} {
		require.NotContains(t, ids, f.candidateID, "相似度第一的候选被资格过滤拦截，不进 top-8")
	}
	for _, id := range clean {
		require.Contains(t, ids, id)
	}
}

// TestRefresh_DuplicateCandidateMergedThreeKeySources（S4 主链路步 2：单路缺失与
// 重复候选）：版块基础路、行为路、seed 路同向量命中同 3 条候选——同候选合并为单卡，
// recall_sources 三键记录实际命中路（board=[版块名] / behavior=[版块名] / seed=[查询文本]）。
func TestRefresh_DuplicateCandidateMergedThreeKeySources(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID := setupSeedLabel(t, db, "日本新闻", "s4-jp-news", "board", []float64{1, 0, 0})
	board := boardID
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: &board, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)
	createInterestEntry(t, db, "日本新闻", &board, []float64{1, 0, 0}, 0)
	routeIDs := make([]uint, 0, 3)
	for i := 0; i < 3; i++ {
		r := setupRecallRoute(t, db, fmt.Sprintf("dup%d", i), []float64{1, 0, 0})
		routeIDs = append(routeIDs, r.routeID)
	}

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	require.Len(t, prompts, 2, "版块合并批（基础+行为）+ seed 批，共 2 批")
	require.Contains(t, prompts[0], "召回来源=版块基础+行为画像", "同候选双路命中去重为单行、徽标双来源")

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 3, "三路命中同一候选 → 单卡（同 hash 去重），不因多路重复出卡")

	var items []models.DiscoveryRunItem
	require.NoError(t, db.Find(&items).Error)
	require.Len(t, items, 3)
	for _, it := range items {
		boardList := metadataStrings(it.RecallSources["board"])
		behaviorList := metadataStrings(it.RecallSources["behavior"])
		seedList := metadataStrings(it.RecallSources["seed"])
		require.Equal(t, []string{"日本新闻"}, boardList, "board 键记录版块名")
		require.Equal(t, []string{"日本新闻"}, behaviorList, "behavior 键记录版块名")
		require.Equal(t, []string{"日本新闻"}, seedList, "seed 键记录查询文本")
	}
}

// TestRefresh_BoardWithoutBehaviorVectorBaseOnly（S4：单路缺失不伪造）：版块只有
// label 向量、无 behavior 画像 → 基础路照常成批；另一版块只有 behavior 画像、
// label 无向量 → 行为路独立成批（两批并存，缺失路不伪造候选）。
func TestRefresh_BoardWithoutBehaviorVectorBaseOnly(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardA := setupSeedLabel(t, db, "日本新闻", "s4-jp-news", "board", []float64{1, 0, 0})
	_ = boardA
	// 版块 B：label 无向量，但有 behavior 画像。
	labelB := models.SemanticLabel{Label: "开发工具", Slug: "s4-dev-tools", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&labelB).Error)
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: &labelB.ID, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)
	for i := 0; i < 3; i++ {
		setupRecallRoute(t, db, fmt.Sprintf("mix%d", i), []float64{1, 0, 0})
	}

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	require.Len(t, prompts, 2, "版块 A 基础批 + 版块 B 行为独立批")
	var sawBase, sawBehavior bool
	for _, p := range prompts {
		if strings.Contains(p, "【版块】日本新闻") {
			sawBase = true
			require.Contains(t, p, "召回来源=版块基础")
		}
		if strings.Contains(p, "【版块】开发工具") {
			sawBehavior = true
			require.Contains(t, p, "召回来源=行为画像", "无 label 向量的版块行为路独立成批")
		}
	}
	require.True(t, sawBase, "无 behavior 向量的版块基础路照常")
	require.True(t, sawBehavior)
}

// TestRefresh_GlobalBehaviorSeparateBatchNotBoard（S4：全局桶单独成批不冒充版块）：
// 仅全局桶 behavior 画像（board_id=NULL）→ 独立批，boardLabel=全局、不出现【版块】
// 上下文；发布卡 board_id 保持 NULL。
func TestRefresh_GlobalBehaviorSeparateBatchNotBoard(t *testing.T) {
	db := testutil.SetupTestDB(t)
	for i := 0; i < 3; i++ {
		setupRecallRoute(t, db, fmt.Sprintf("g%d", i), []float64{1, 0, 0})
	}
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	require.Len(t, prompts, 1)
	require.NotContains(t, prompts[0], "【版块】", "全局行为批不冒充版块上下文")
	require.Contains(t, prompts[0], "全局")

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.NotEmpty(t, pending)
	for _, p := range pending {
		require.Nil(t, p.BoardID, "全局桶批次发布 board_id=NULL")
	}
	var items []models.DiscoveryRunItem
	require.NoError(t, db.Find(&items).Error)
	for _, it := range items {
		behaviorList := metadataStrings(it.RecallSources["behavior"])
		require.Equal(t, []string{"全局"}, behaviorList)
	}
}

// TestRefresh_ModelMismatchSkipsIncompatibleCandidate（S4 主链路步 3 新语义，Medium 4）：
// 候选向量中一条与画像模型不同 → 该候选被跳过（记 skipped 日志），兼容候选照常参与精排
// 与发布；不再用全表不一致把整轮卡死（遗留旧模型行不清除也不阻断）。不同模型向量绝不混算。
func TestRefresh_ModelMismatchSkipsIncompatibleCandidate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	a := setupRecallRoute(t, db, "a", []float64{1, 0, 0})
	b := setupRecallRoute(t, db, "b", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)
	// 注入不一致：候选 b 的向量模型与画像不同（模型切换未完成覆盖的过渡态）。
	require.NoError(t, db.Model(&models.CandidateEmbedding{}).
		Where("candidate_id = ?", b.candidateID).Update("model", "other-model").Error)

	client := &mockDiscoveryClient{chatFn: selectAllChatFn()}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	summary, err := svc.Refresh(context.Background())
	require.NoError(t, err, "兼容向量缺失的候选跳过，不阻断整轮（D4 防混算 ≠ 被遗留旧行卡死）")

	var run models.DiscoveryRun
	require.NoError(t, db.First(&run, summary.RunID).Error)
	require.Equal(t, "succeeded", run.Status)
	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 1, "仅兼容候选进入精排与发布")
	require.NotNil(t, pending[0].CandidateID)
	require.Equal(t, a.candidateID, *pending[0].CandidateID, "不兼容的 b 不发布")
	for _, p := range pending {
		require.NotEqual(t, b.candidateID, *p.CandidateID)
	}
}

// TestRefresh_ZeroCompatibleVectorsFailsRound（Medium 4 边界）：存在候选向量行但零个候选
// 有兼容向量 → 真正的模型不兼容，整轮 configuration 失败、零精排、零发布。
func TestRefresh_ZeroCompatibleVectorsFailsRound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	a := setupRecallRoute(t, db, "a", []float64{1, 0, 0})
	b := setupRecallRoute(t, db, "b", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)
	for _, c := range []recallRoute{a, b} {
		require.NoError(t, db.Model(&models.CandidateEmbedding{}).
			Where("candidate_id = ?", c.candidateID).Update("model", "other-model").Error)
	}

	client := &mockDiscoveryClient{}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	summary, err := svc.Refresh(context.Background())
	require.Error(t, err)
	require.NotZero(t, summary.RunID)

	var run models.DiscoveryRun
	require.NoError(t, db.First(&run, summary.RunID).Error)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "configuration", run.ErrorCode, "零兼容向量 = 不兼容，整轮失败")
	require.Zero(t, client.chatCalls, "校验前置：零精排调用")
	var pending int64
	db.Model(&models.FeedRecommendation{}).Where("status = ?", "pending").Count(&pending)
	require.Zero(t, pending)
}

// TestAsk_Top20SingleBatch（批次上限 20 / ask 直查 top-20 单批）：25 条同向候选 →
// ask 单批恰 20 条（批次上限生效），全部发布。
func TestAsk_Top20SingleBatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	for i := 0; i < 25; i++ {
		setupRecallRoute(t, db, fmt.Sprintf("m%02d", i), gradedVec(0.001*float64(i)))
	}
	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "日本新闻", "")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status)
	require.Len(t, prompts, 1, "ask 单批")
	require.Equal(t, 20, countPromptIDs(prompts[0]), "top-20 截断（25 条候选取 20，批次上限 20）")
	require.Contains(t, prompts[0], "【用户查询】日本新闻", "查询原文进精排上下文")

	var pending int64
	db.Model(&models.FeedRecommendation{}).Where("status = ?", "pending").Count(&pending)
	require.EqualValues(t, 20, pending)
}

// ── 纯函数白盒：路径合并去重 / 批次上限 / 来源三键 ──

func TestMergePathCandidatesDedupAndCap(t *testing.T) {
	mk := func(ids ...uint) []candidateRow {
		out := make([]candidateRow, 0, len(ids))
		for _, id := range ids {
			out = append(out, candidateRow{CandidateID: id})
		}
		return out
	}
	// 去重合并：r2 双路命中保留双徽标。
	merged := mergePathCandidates(mk(1, 2), mk(2, 3))
	require.Len(t, merged, 3)
	require.Equal(t, []string{recallPathBoard}, merged[0].paths)
	require.Equal(t, []string{recallPathBoard, recallPathBehavior}, merged[1].paths)
	require.Equal(t, []string{recallPathBehavior}, merged[2].paths)

	// 批次上限 20：12 基础 + 12 行为不相交 → 截 20（基础 8 条在前不被挤出）。
	var baseIDs, behaviorIDs []uint
	for i := 0; i < 12; i++ {
		baseIDs = append(baseIDs, uint(100+i))
		behaviorIDs = append(behaviorIDs, uint(200+i))
	}
	capped := mergePathCandidates(mk(baseIDs...), mk(behaviorIDs...))
	require.Len(t, capped, recallBatchMaxCandidates)
	for i := 0; i < 8; i++ {
		require.Equal(t, uint(100+i), capped[i].CandidateID, "基础路保底在前 8")
	}
}

func TestCandidateRecallSourcesThreeKeys(t *testing.T) {
	boardID := uint(7)
	b := recallBatch{boardID: &boardID, boardLabel: "日本新闻", queryText: "日本新闻", path: recallPathBoard}
	c := recallCandidate{paths: []string{recallPathBoard, recallPathBehavior, recallPathSeed}}
	src := candidateRecallSources(b, c)
	require.Equal(t, []string{"日本新闻"}, src["board"])
	require.Equal(t, []string{"日本新闻"}, src["behavior"])
	require.Equal(t, []string{"日本新闻"}, src["seed"])

	ask := recallBatch{queryText: "查询原文超过一百个字符的边界测试", path: recallPathQuery}
	qc := recallCandidate{paths: []string{recallPathQuery}}
	asrc := candidateRecallSources(ask, qc)
	require.Equal(t, []string{truncateRunesSafe("查询原文超过一百个字符的边界测试", 100)}, asrc["query"])
}
