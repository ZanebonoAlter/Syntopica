package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 日报关键事件推荐（add-topic-key-event-markers design D3）─────────────────
//
// 一次有界 best-effort：新日报 SaveReport 成功并完成既有终态后，派发独立、
// 脱离 HTTP request 的单次推荐。不加入 SignalDiscovery 的 202/409 分析任务、
// 不延长日报域闸、不发第二条终态通知；失败不改日报结果（spec 红线）。
//
// 每板块最多一个推荐任务（boardMutex 非阻塞让路）；重建产生新 generation
// token，旧任务保存时 token 不匹配整批丢弃。重启不自动恢复付费调用——
// running 存量读时如实降级为 failed（IsKeyEventRecommendationActive 的内存
// 集合只在进程内有效）。用户打开页面只读，不触发推荐。

const (
	// keyEventRecommendMaxCandidates caps the AI candidate list (design D3:
	// 候选最多 40 条，按日报顺序、稳定 ID).
	keyEventRecommendMaxCandidates = 40
	// keyEventRecommendContextRunes is the TOTAL input text budget (design
	// D3: 总文字预算 12000 runes，有界裁剪并留 input 日志说明).
	keyEventRecommendContextRunes = 12000
	// keyEventRecommendHistoryLimit: 最新已确认及已不采纳/取消节点各最多条数.
	keyEventRecommendHistoryLimit = 20
	// keyEventRecommendRecentDays: 泳道近期事实指纹窗口（14 天）.
	keyEventRecommendRecentDays = 14
	// keyEventRecommendTimeout bounds the single Chat call (design D3).
	keyEventRecommendTimeout = 90 * time.Second
)

// keyEventRecommendTimeoutVar is the injectable timeout (tests pin it small).
var keyEventRecommendTimeoutVar = keyEventRecommendTimeout

// keyEventActiveRuns tracks in-process running recommendation tasks. Keys are
// report IDs. Used by the read path to degrade persisted-but-orphaned
// "running" rows (restart killed the task) to "failed" honestly.
var keyEventActiveRuns sync.Map

// IsKeyEventRecommendationActive reports whether a recommendation task for
// the report is alive IN THIS PROCESS. A DB "running" row without a live
// in-memory entry means the process restarted mid-run — the read path
// degrades it to failed (重启不自动恢复付费调用).
func IsKeyEventRecommendationActive(reportID uint) bool {
	_, ok := keyEventActiveRuns.Load(reportID)
	return ok
}

// keyEventBoardMutexes serializes recommendation tasks per board (每板块最多
// 一个推荐任务). tryLock semantics: a second task for the same board skips.
var keyEventBoardMutexes sync.Map // boardID → *sync.Mutex (advisory only)

// keyEventRecommendChatFn is the swappable LLM hook (test injection; mirrors
// laneSnapshotChatFn). Default: single airouter Chat, operation
// daily_report.key_event_recommend, CapabilityDigestPolish, sub-session
// derived from the daily-report session.
var keyEventRecommendChatFn = func(ctx context.Context, sessionID, system, user string) (string, error) {
	temperature := 0.2
	maxTokens := 2048
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "daily_report.key_event_recommend",
		SessionID:  sessionID,
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// spawnKeyEventRecommendation launches the recommendation as a detached
// goroutine after a report's existing terminal work finished. Injectable so
// tests can run it synchronously.
var spawnKeyEventRecommendation = func(boardID, reportID uint, sessionID string) {
	go runKeyEventRecommendationSafe(boardID, reportID, sessionID)
}

func runKeyEventRecommendationSafe(boardID, reportID uint, sessionID string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("key-event-recommend: panic for board %d report %d: %v", boardID, reportID, r)
			_ = repository.Repo.FinishKeyEventRecommendationRun(reportID, "", repository.KeyEventRunStateFailed, 0, 0, fmt.Sprintf("panic: %v", r))
		}
	}()
	runKeyEventRecommendation(context.Background(), boardID, reportID, sessionID)
}

// keyEventEvidence is one candidate article id paired with its resolved
// title — the prompt must render ids (not bare titles) or the model cannot
// cite evidence_refs as required by the output contract (R-8: bare titles
// made every suggestion fail []uint unmarshal and get dropped wholesale).
type KeyEventEvidence struct {
	ID    uint
	Title string
}

// keyEventCandidate is one AI candidate thread with its resolved facts.
type keyEventCandidate struct {
	ThreadID   uint
	SectionID  uint
	TopicID    uint
	TopicLabel string
	Title      string
	Summary    string
	ArticleIDs []uint
	Evidence   []KeyEventEvidence // bounded, id-paired evidence titles
}

// keyEventSuggestion is the raw parsed model output item (pre-validation).
type keyEventSuggestion struct {
	SourceThreadID uint   `json:"source_thread_id"`
	Reason         string `json:"reason"`
	NewProgress    bool   `json:"new_progress"`
	EvidenceRefs   []uint `json:"evidence_refs"`
}

// SetKeyEventRecommendationChatForTest swaps the chat hook and returns a
// restore function (handler-level composition tests use it; mirrors the
// package-internal injection pattern).
func SetKeyEventRecommendationChatForTest(fn func(ctx context.Context, sessionID, system, user string) (string, error)) func() {
	orig := keyEventRecommendChatFn
	keyEventRecommendChatFn = fn
	return func() { keyEventRecommendChatFn = orig }
}

// SetKeyEventRecommendationSpawnForTest swaps the spawn hook and returns a
// restore function. Handler-level composition tests inject a SYNCHRONOUS
// spawn so the terminal-state assertions observe the final state without
// racing the detached goroutine.
func SetKeyEventRecommendationSpawnForTest(spawn func(boardID, reportID uint, sessionID string)) func() {
	orig := spawnKeyEventRecommendation
	spawnKeyEventRecommendation = spawn
	return func() { spawnKeyEventRecommendation = orig }
}

// SpawnKeyEventRecommendationForReport wires the recommendation into the
// daily-report pipeline. Called by GenerateAndSaveReport AFTER the report (and
// its existing terminal work) succeeded — the spawn is fire-and-forget and by
// construction cannot change the report result or emit a second terminal
// notification.
func SpawnKeyEventRecommendationForReport(boardID, reportID uint, sessionID string) {
	spawnKeyEventRecommendation(boardID, reportID, sessionID)
}

// runKeyEventRecommendation is the full bounded task (design D3). Every error
// path lands in the run row as failed and returns — nothing here may touch
// the report or notify.
func runKeyEventRecommendation(ctx context.Context, boardID, reportID uint, sessionID string) {
	// Per-board mutual exclusion: at most ONE running recommendation task per
	// board. Rebuild rotations: SaveReport 的重建路径已轮换 run 行 token
	// （invalidateKeyEventRunToken），旧任务的 Save/Finish 都按旧 token 匹配
	// 落空——建议批次被丢、终态不误写新行；此处 TryLock 失败只说明同板块
	// 已有任务在跑（跳过本次 spawn 是正确去重，下个日报日自然补上）。
	mAny, _ := keyEventBoardMutexes.LoadOrStore(boardID, &sync.Mutex{})
	mu := mAny.(*sync.Mutex)
	if !mu.TryLock() {
		logging.Infof("key-event-recommend: board %d already has a task running; skip report %d", boardID, reportID)
		return
	}
	defer mu.Unlock()

	// Generation token: a rebuild of the same (board, date) overwrites the
	// run row with a NEW token; our save then detects staleness and drops.
	token := uuid.NewString()
	if err := repository.Repo.UpsertKeyEventRecommendationRun(reportID, boardID, token); err != nil {
		logging.Warnf("key-event-recommend: start run for report %d failed: %v", reportID, err)
		return
	}
	keyEventActiveRuns.Store(reportID, struct{}{})
	defer keyEventActiveRuns.Delete(reportID)

	candidates, err := loadKeyEventCandidates(boardID, reportID)
	if err != nil {
		logging.Warnf("key-event-recommend: load candidates for report %d failed: %v", reportID, err)
		_ = repository.Repo.FinishKeyEventRecommendationRun(reportID, token, repository.KeyEventRunStateFailed, 0, 0, err.Error())
		return
	}
	if len(candidates) == 0 {
		// 无有效候选不调 AI（design D3）— zero-suggestion is a legal done.
		_ = repository.Repo.FinishKeyEventRecommendationRun(reportID, token, repository.KeyEventRunStateDone, 0, 0, "")
		return
	}

	userPrompt, truncated := buildKeyEventRecommendPrompt(boardID, reportID, candidates)
	if truncated > 0 {
		logging.Infof("key-event-recommend: report %d context clamped by %d runes (budget %d)",
			reportID, truncated, keyEventRecommendContextRunes)
	}

	ctx, cancel := context.WithTimeout(ctx, keyEventRecommendTimeoutVar)
	defer cancel()
	raw, err := keyEventRecommendChatFn(ctx, sessionID+"_keyevent", keyEventRecommendSystemPrompt(), userPrompt)
	if err != nil {
		logging.Warnf("key-event-recommend: chat for report %d failed: %v", reportID, err)
		_ = repository.Repo.FinishKeyEventRecommendationRun(reportID, token, repository.KeyEventRunStateFailed, len(candidates), 0, err.Error())
		return
	}

	suggestions, dropped := parseKeyEventSuggestions(raw, candidates)
	if dropped > 0 {
		logging.Infof("key-event-recommend: report %d dropped %d invalid suggestions", reportID, dropped)
	}
	saved := 0
	if len(suggestions) > 0 {
		saved, err = repository.Repo.SaveSuggestedMarkers(boardID, reportID, token, suggestions)
		if err != nil {
			logging.Warnf("key-event-recommend: save suggestions for report %d failed: %v", reportID, err)
			_ = repository.Repo.FinishKeyEventRecommendationRun(reportID, token, repository.KeyEventRunStateFailed, len(candidates), 0, err.Error())
			return
		}
	}
	_ = repository.Repo.FinishKeyEventRecommendationRun(reportID, token, repository.KeyEventRunStateDone, len(candidates), saved, "")
}

// RunKeyEventRecommendationSync is the synchronous test entry (same task;
// used by composition tests via the spawn hook).
func RunKeyEventRecommendationSync(ctx context.Context, boardID, reportID uint, sessionID string) {
	runKeyEventRecommendation(ctx, boardID, reportID, sessionID)
}

// loadKeyEventCandidates loads the report's topic-attributed threads, filters
// out sources whose fingerprint already has ANY marker row (机械过滤完全重复
// 来源 — 已建议/已确认/已不采纳都不再进入候选) and caps at 40.
func loadKeyEventCandidates(boardID, reportID uint) ([]keyEventCandidate, error) {
	var report repository.BoardDailyReport
	if err := repository.Repo.DB().First(&report, reportID).Error; err != nil {
		return nil, fmt.Errorf("load report: %w", err)
	}
	periodDate := repository.NormalizeReportDate(report.PeriodDate)

	var rows []struct {
		ID        uint
		SectionID uint
		Title     string
		Summary   string
		TagIDs    repository.JSON
		ArtIDs    repository.JSON
		TopicID   uint
		TopicLbl  string
	}
	if err := repository.Repo.DB().Table("daily_report_threads AS t").
		Select("t.id, t.section_id, t.title, t.summary, t.tag_ids, t.related_article_ids AS art_ids, s.persistent_topic_id AS topic_id, p.label AS topic_lbl").
		Joins("JOIN daily_report_sections s ON s.id = t.section_id").
		Joins("JOIN board_persistent_topics p ON p.id = s.persistent_topic_id").
		Where("t.report_id = ?", reportID).
		Order("t.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load threads: %w", err)
	}

	// Existing marker fingerprints for this board+date — exact repeats drop.
	existing, err := repository.Repo.ListReportKeyEventMarkers(reportID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(existing))
	for _, m := range existing {
		seen[m.SourceKey] = true
	}

	// Evidence titles (bounded lookup, one query).
	articleIDs := make(map[uint]bool)
	for _, r := range rows {
		for _, id := range jsonUintIDsExported(r.ArtIDs) {
			articleIDs[id] = true
		}
	}
	titles := make(map[uint]string)
	if len(articleIDs) > 0 {
		ids := make([]uint, 0, len(articleIDs))
		for id := range articleIDs {
			ids = append(ids, id)
		}
		var arts []struct {
			ID    uint
			Title string
		}
		if err := repository.Repo.DB().Table("articles").
			Select("id, title").Where("id IN ?", ids).Scan(&arts).Error; err == nil {
			for _, a := range arts {
				titles[a.ID] = a.Title
			}
		}
	}

	candidates := make([]keyEventCandidate, 0, len(rows))
	for _, r := range rows {
		arts := jsonUintIDsExported(r.ArtIDs)
		key := repository.ComputeKeyEventSourceKey(boardID, r.TopicID, periodDate, r.Title, jsonUintIDsExported(r.TagIDs), arts)
		if seen[key] {
			continue // 完全重复来源：已有建议/人工记录，不再进候选
		}
		evidence := make([]KeyEventEvidence, 0, len(arts))
		for _, id := range arts {
			// id always renders even without a resolvable title — the model
			// must be able to cite it; title is decoration.
			evidence = append(evidence, KeyEventEvidence{ID: id, Title: titles[id]})
		}
		candidates = append(candidates, keyEventCandidate{
			ThreadID: r.ID, SectionID: r.SectionID, TopicID: r.TopicID, TopicLabel: r.TopicLbl,
			Title: r.Title, Summary: r.Summary, ArticleIDs: arts, Evidence: evidence,
		})
		if len(candidates) >= keyEventRecommendMaxCandidates {
			logging.Infof("key-event-recommend: report %d candidates clamped at %d (report order, stable IDs)",
				reportID, keyEventRecommendMaxCandidates)
			break
		}
	}
	return candidates, nil
}

// jsonUintIDsExported parses a JSON number array; nil on error.
func jsonUintIDsExported(raw repository.JSON) []uint {
	if len(raw) == 0 {
		return nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

// keyEventRecommendSystemPrompt is the instruction block: sparse selection,
// lane-relative significance, follow-up events must state the delta. The
// model never self-reports owner/state/importance.
func keyEventRecommendSystemPrompt() string {
	return "你是新闻关键事件筛选助手。给定某版块各话题泳道当日候选线索（标题/摘要/依据文章）与近期背景，" +
		"挑选少数对未来回看这条泳道真正关键的事件或重要进展。要求：" +
		"1）稀疏选择：宁缺毋滥，零条是合法结果，不为凑数而推荐；" +
		"2）理由必须说明该事件对所属泳道的意义；若是已有事件的后续，必须说明新增了什么实质变化（如正式落地、重大结果、政策生效），" +
		"纯转载、换标题、普通评论不算新进展；" +
		"3）只基于给定材料，不得编造事件、数字或因果；每条建议的 evidence_refs 必须填该线索依据列出的 article id 纯数字（如 14157），不要填标题文字；" +
		"4）用户已选择/已处理的节点清单仅供参考对照，不是事实来源，不得复述其内容为新建议。" +
		"输出 JSON 数组，每项形如 {\"source_thread_id\":<候选线索id>,\"reason\":\"<中文理由>\"," +
		"\"new_progress\":<true|false>,\"evidence_refs\":[<article id>...]}；无合适条目输出 []。" +
		"直接输出 JSON，不要任何前后缀或代码块围栏。"
}

// renderKeyEventCandidateLines renders block ①（纯函数，无 DB 依赖，可单测）。
func renderKeyEventCandidateLines(candidates []keyEventCandidate) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- 当日候选线索（source_thread_id｜话题｜标题｜摘要｜依据文章） ---\n")
	for _, c := range candidates {
		fmt.Fprintf(&sb, "%d｜%s｜%s｜%s", c.ThreadID, c.TopicLabel, c.Title, c.Summary)
		if len(c.Evidence) > 0 {
			parts := make([]string, 0, len(c.Evidence))
			for _, ev := range c.Evidence {
				if ev.Title != "" {
					parts = append(parts, fmt.Sprintf("%d=%s", ev.ID, ev.Title))
				} else {
					parts = append(parts, fmt.Sprintf("%d", ev.ID))
				}
			}
			fmt.Fprintf(&sb, "｜依据 article id=标题：%s", strings.Join(parts, "；"))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// clampKeyEventPrompt clamps the assembled prompt to the total rune budget
// and reports how many runes were cut（design D3：总文字预算 12000 runes，
// 超出截断并留 input 日志说明）。纯函数。
func clampKeyEventPrompt(text string) (string, int) {
	if total := len([]rune(text)); total > keyEventRecommendContextRunes {
		return string([]rune(text)[:keyEventRecommendContextRunes]), total - keyEventRecommendContextRunes
	}
	return text, 0
}

// buildKeyEventRecommendPrompt renders the three input blocks (design D3):
// ①当日候选线索事实与依据；②各泳道近14天事实指纹；③已确认/已不采纳各≤20条
// 人工处理对照。总预算 12000 runes，超出截断并在返回值中说明截断量。
func buildKeyEventRecommendPrompt(boardID, reportID uint, candidates []keyEventCandidate) (string, int) {
	var sb strings.Builder
	sb.WriteString(renderKeyEventCandidateLines(candidates))

	// ②近期事实指纹：每条候选所属泳道近 14 天（锚定报告日）。
	topicSeen := make(map[uint]bool)
	var topics []uint
	for _, c := range candidates {
		if !topicSeen[c.TopicID] {
			topicSeen[c.TopicID] = true
			topics = append(topics, c.TopicID)
		}
	}
	fmt.Fprintf(&sb, "\n--- 泳道近期14天事实指纹（日期｜节标题｜线索标题） ---\n")
	var report repository.BoardDailyReport
	if err := repository.Repo.DB().First(&report, reportID).Error; err == nil {
		to := repository.NormalizeReportDate(report.PeriodDate)
		from := to.AddDate(0, 0, -keyEventRecommendRecentDays)
		for _, topicID := range topics {
			rows, err := repository.Repo.ListLaneSnapshotMaterial(topicID, from, to, 3)
			if err != nil {
				continue
			}
			for _, row := range rows {
				line := fmt.Sprintf("%s｜%s", row.PeriodDate.Format("2006-01-02"), row.SectionLabel)
				if len(row.ThreadTitles) > 0 {
					line += fmt.Sprintf("｜%s", strings.Join(row.ThreadTitles, " / "))
				}
				sb.WriteString(line + "\n")
			}
		}
	}

	// ③人工处理对照：最新已确认与已不采纳/取消节点各 ≤20 条（仅标题/日期，
	// 不注入历史 AI 理由叙事）。
	fmt.Fprintf(&sb, "\n--- 用户已选择（confirmed，对照用，勿复述为新建议） ---\n")
	appendHistory := func(state string) {
		markers, err := repository.Repo.ListBoardKeyEventMarkersByState(boardID, state, keyEventRecommendHistoryLimit)
		if err != nil {
			return
		}
		for _, m := range markers {
			fmt.Fprintf(&sb, "%s｜%s\n", m.SourcePeriodDate.Format("2006-01-02"), m.TitleSnapshot)
		}
	}
	appendHistory(repository.KeyEventStateConfirmed)
	fmt.Fprintf(&sb, "\n--- 用户已不采纳/已取消（dismissed，对照用） ---\n")
	appendHistory(repository.KeyEventStateDismissed)

	return clampKeyEventPrompt(sb.String())
}

// parseKeyEventSuggestions validates the raw model output against the
// candidate set (design D3 输出校验): IDs must reference a candidate; reason
// must be non-blank after trim; evidence refs must intersect the thread's
// article IDs (invalid refs dropped individually, zero valid → drop). Returns
// the validated suggestions and how many raw items were dropped.
func parseKeyEventSuggestions(raw string, candidates []keyEventCandidate) ([]repository.SuggestedKeyEvent, int) {
	byID := make(map[uint]keyEventCandidate, len(candidates))
	for _, c := range candidates {
		byID[c.ThreadID] = c
	}
	cleaned := jsonutil.SanitizeLLMJSON(raw)
	var items []keyEventSuggestion
	if err := json.Unmarshal([]byte(cleaned), &items); err != nil {
		logging.Warnf("key-event-recommend: parse output failed: %v", err)
		return nil, 0
	}
	out := make([]repository.SuggestedKeyEvent, 0, len(items))
	dropped := 0
	seenThread := make(map[uint]bool)
	for _, it := range items {
		c, ok := byID[it.SourceThreadID]
		if !ok || seenThread[it.SourceThreadID] {
			dropped++ // 越界/重复 ID 丢弃
			continue
		}
		reason := strings.TrimSpace(it.Reason)
		if reason == "" {
			dropped++ // 空理由丢弃
			continue
		}
		valid := make([]uint, 0, len(it.EvidenceRefs))
		own := make(map[uint]bool, len(c.ArticleIDs))
		for _, id := range c.ArticleIDs {
			own[id] = true
		}
		for _, ref := range it.EvidenceRefs {
			if own[ref] {
				valid = append(valid, ref)
			}
		}
		if len(valid) == 0 {
			dropped++ // 无当期有效依据丢弃
			continue
		}
		sort.Slice(valid, func(i, j int) bool { return valid[i] < valid[j] })
		seenThread[it.SourceThreadID] = true
		out = append(out, repository.SuggestedKeyEvent{
			SourceThreadID: c.ThreadID,
			Reason:         reason,
			NewProgress:    it.NewProgress,
			EvidenceIDs:    valid,
		})
	}
	return out, dropped
}
