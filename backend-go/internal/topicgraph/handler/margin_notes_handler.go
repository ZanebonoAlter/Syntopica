package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// 日报页边注（daily-report-margin-notes design D3）。四端点全部挂
// topicgraph handler 群；`/daily-reports/:id/annotations` 的参数名必须沿用
// `:id`（gin 路由树与现有 /daily-reports/:id 同段通配符同名约束）。

// 参数上限（runes）：quoted_text 2000（brief 建议）、question 1000（IN-4）。
const (
	maxQuotedTextRunes = 2000
	maxQuestionRunes   = 1000
)

// RegisterMarginNoteRoutes registers margin note routes under the api group.
func RegisterMarginNoteRoutes(api *gin.RouterGroup) {
	// 日报阅读层：按报告批注列表 + 落锚
	api.GET("/daily-reports/:id/annotations", listReportAnnotations)
	api.POST("/daily-reports/:id/annotations", createReportAnnotation)
	// 跨报告管理页列表 + 单条操作（删除连带问答；提问/追问）
	api.GET("/annotations", listAnnotationsManagement)
	api.DELETE("/annotations/:id", deleteAnnotation)
	api.POST("/annotations/:id/questions", askAnnotationQuestion)
}

type createAnnotationRequest struct {
	SectionID         uint   `json:"section_id"`
	ThreadID          *uint  `json:"thread_id"`
	QuotedText        string `json:"quoted_text"`
	AnchorOffsetStart int    `json:"anchor_offset_start"`
	AnchorOffsetEnd   int    `json:"anchor_offset_end"`
}

// listReportAnnotations handles GET /api/daily-reports/:id/annotations —
// 页边注栏一次拉齐批注（含问答轮与术语 chips）。
func listReportAnnotations(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report id"})
		return
	}
	annotations, err := repository.Repo.ListAnnotationsByReport(uint(reportID))
	if err != nil {
		logging.Errorf("list report annotations: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list annotations"})
		return
	}
	items := make([]gin.H, 0, len(annotations))
	for i := range annotations {
		a := &annotations[i]
		items = append(items, gin.H{
			"id":                  a.ID,
			"report_id":           a.ReportID,
			"section_id":          a.SectionID,
			"thread_id":           a.ThreadID,
			"quoted_text":         a.QuotedText,
			"anchor_offset_start": a.AnchorOffsetStart,
			"anchor_offset_end":   a.AnchorOffsetEnd,
			"created_at":          a.CreatedAt,
			"qas":                 a.QAs,
			"terms":               termsFromQAs(a.QAs),
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"annotations": items}})
}

// createReportAnnotation handles POST /api/daily-reports/:id/annotations —
// 落锚（校验：quoted 非空 + 长度上限、section/thread 归属；report 缺失 404）。
func createReportAnnotation(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report id"})
		return
	}
	var req createAnnotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if err := validateQuotedText(req.QuotedText); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// PR-2：report 不存在 → 404；section/thread 归属不符 → 400。
	if _, err := repository.Repo.GetReportByID(uint(reportID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "report not found"})
		return
	}
	if err := repository.Repo.ValidateAnnotationTarget(uint(reportID), req.SectionID, req.ThreadID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	annotation := &repository.ReportAnnotation{
		ReportID:          uint(reportID),
		SectionID:         req.SectionID,
		ThreadID:          req.ThreadID,
		QuotedText:        req.QuotedText,
		AnchorOffsetStart: req.AnchorOffsetStart,
		AnchorOffsetEnd:   req.AnchorOffsetEnd,
	}
	if err := repository.Repo.CreateAnnotation(annotation); err != nil {
		logging.Errorf("create annotation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create annotation"})
		return
	}
	// 响应数组契约与 GET 同口径：新建行的 qa 关联未装载，零值切片保证 JSON 是 []
	// 而不是 null（前端卡片读 note.qas.length，null 会直接抛错）。
	if annotation.QAs == nil {
		annotation.QAs = []repository.AnnotationQA{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"annotation": annotation}})
}

// deleteAnnotation handles DELETE /api/annotations/:id — 删除批注连带全部
// 问答（确认在前端；服务端直接删）。
func deleteAnnotation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid annotation id"})
		return
	}
	if err := repository.Repo.DeleteAnnotation(uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "annotation not found"})
			return
		}
		logging.Errorf("delete annotation: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to delete annotation"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": id}})
}

type askQuestionRequest struct {
	Question string `json:"question"`
}

// askAnnotationQuestion handles POST /api/annotations/:id/questions — 提问/
// 追问同端点，同步返回回答 + 引用 + 术语。
func askAnnotationQuestion(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid annotation id"})
		return
	}
	var req askQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	question := req.Question
	if utf8.RuneCountInString(question) == 0 || utf8.RuneCountInString(question) > maxQuestionRunes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "question must be 1-1000 runes"})
		return
	}

	// PR-2：annotation 不存在 → 404，且不发起 LLM 调用（service 内先查锚点）。
	result, err := service.AskMarginNote(c.Request.Context(), uint(id), question)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "annotation not found"})
			return
		}
		// LLM/上游失败 → 502 行内可重试（已提交问题与历史问答轮不受影响）。
		logging.Errorf("margin note qa (annotation %d): %v", id, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "failed to answer question"})
		return
	}
	// 响应形状与 GET /daily-reports/:id/annotations 的 qas 条目对齐：前端卡片直接
	// 消费 qa.id / question / answer / cited_article_ids / cited_web_sources /
	// extracted_terms；extracted_terms 本轮带 is_new（新词「入库」角标），历史轮次
	// （GET）为纯字符串数组，前端两者兼容。cited_web_sources 为白名单后的网络
	// 来源（D7：[{title,url}]，新窗口外链 chips）。
	qa := result.QA
	if qa == nil { // 防御：service 永远返回已落库轮次，缺失属实现错误
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": "failed to answer question"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"qa": gin.H{
			"id":                   qa.ID,
			"annotation_id":        qa.AnnotationID,
			"question":             qa.Question,
			"answer":               qa.Answer,
			"cited_article_ids":    result.CitedArticleIDs,
			"cited_web_sources":    webSourcesFromResult(result.CitedWebSources),
			"extracted_terms":      termsWithNewFlag(result.Terms, result.NewTerms),
			"pure_model_knowledge": result.PureModelKnowledge,
			"provider":             result.Provider,
			"created_at":           qa.CreatedAt,
		},
	}})
}

// webSourcesFromResult maps service web sources to response objects (always a
// non-nil array — jsonb 数组契约：空写 [] 不写 null).
func webSourcesFromResult(sources []repository.MarginNoteWebSource) []gin.H {
	out := make([]gin.H, 0, len(sources))
	for _, s := range sources {
		out = append(out, gin.H{"title": s.Title, "url": s.URL})
	}
	return out
}

// listAnnotationsManagement handles GET /api/annotations?board_id=&q=&page_size=&page=
// — 跨报告管理页列表（含问答轮与术语，按日期倒序）。
func listAnnotationsManagement(c *gin.Context) {
	filter := repository.AnnotationListFilter{Query: c.Query("q")}
	if v := c.Query("board_id"); v != "" {
		boardID, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board_id"})
			return
		}
		bid := uint(boardID)
		filter.BoardID = &bid
	}
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			filter.Page = p
		}
	}
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil {
			filter.PageSize = ps
		}
	}

	rows, total, err := repository.Repo.ListAnnotationsForManagement(filter)
	if err != nil {
		logging.Errorf("list annotations (management): %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list annotations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"annotations": rows,
		"total":       total,
		"page":        filter.Page,
		"page_size":   filter.PageSize,
	}})
}

// termsWithNewFlag folds the derived terms into the card contract: each entry is
// {term, is_new} so the「入库」badge only lights on terms created by this turn.
func termsWithNewFlag(terms, newTerms []string) []gin.H {
	if terms == nil {
		return []gin.H{}
	}
	fresh := make(map[string]struct{}, len(newTerms))
	for _, term := range newTerms {
		fresh[term] = struct{}{}
	}
	out := make([]gin.H, 0, len(terms))
	for _, term := range terms {
		_, isNew := fresh[term]
		out = append(out, gin.H{"term": term, "is_new": isNew})
	}
	return out
}

// termsFromQAs folds an annotation's per-turn extracted_terms into deduped
// chips（保持轮次顺序）。
func termsFromQAs(qas []repository.AnnotationQA) []string {
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	for _, qa := range qas {
		if len(qa.ExtractedTerms) == 0 {
			continue
		}
		var terms []string
		if err := json.Unmarshal(qa.ExtractedTerms, &terms); err != nil {
			continue // 冗余展示字段，解析失败跳过
		}
		for _, t := range repository.NormalizeTerms(terms) {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// validateQuotedText enforces 非空 + 上限（IN-1/IN-2：空串/纯空白 400，超长 400）。
func validateQuotedText(s string) error {
	n := utf8.RuneCountInString(s)
	if n == 0 || utf8.RuneCountInString(strings.TrimFunc(s, unicode.IsSpace)) == 0 {
		return errors.New("quoted_text is required")
	}
	if n > maxQuotedTextRunes {
		return errors.New("quoted_text exceeds 2000 runes")
	}
	return nil
}
