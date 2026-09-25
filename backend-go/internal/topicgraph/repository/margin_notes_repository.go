package repository

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
)

// 日报页边注三表（daily-report-margin-notes design D1）。schema 由版本化迁移
// 20260922_0003 创建（模型不注册 AutoMigrate，SQL 是唯一权威）。

// ReportAnnotation — one anchored margin note on a daily report narrative text.
// 锚定 = quoted_text + report/section/thread 归属 + 字符偏移线索；日报 period
// 生成后不可变（backfill 整份覆盖重建除外）保证可重放定位，重建后偏移失配由
// 前端降级为 quoted_text 模糊匹配、再失配显示「原文已变更」。
type ReportAnnotation struct {
	ID        uint `gorm:"primarykey" json:"id"`
	ReportID  uint `gorm:"not null;index:idx_report_annotations_report" json:"report_id"`
	SectionID uint `gorm:"not null" json:"section_id"`
	// ThreadID is NULL for masthead lead-summary annotations (no thread).
	ThreadID          *uint     `json:"thread_id,omitempty"`
	QuotedText        string    `gorm:"type:text;not null" json:"quoted_text"`
	AnchorOffsetStart int       `gorm:"not null;default:0" json:"anchor_offset_start"`
	AnchorOffsetEnd   int       `gorm:"not null;default:0" json:"anchor_offset_end"`
	CreatedAt         time.Time `json:"created_at"`

	// QAs are the question-answer turns on this annotation (1:N), loaded via
	// Preload on read paths, never persisted directly from the parent.
	QAs []AnnotationQA `gorm:"foreignKey:AnnotationID" json:"qas"`
}

func (ReportAnnotation) TableName() string { return "report_annotations" }

// AnnotationQA — one question-answer turn. cited_article_ids /
// extracted_terms follow the thread 引用数组契约: JSON arrays, 空写 [] 不写
// null（InsertAnnotationQA 强制归一/拒绝），保序去重。operation/provider 是
// ai_call_logs（7 天清理）之外的审计冗余，便于离线核对。
type AnnotationQA struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	AnnotationID uint   `gorm:"not null;index:idx_annotation_qas_annotation" json:"annotation_id"`
	Question     string `gorm:"type:text;not null" json:"question"`
	Answer       string `gorm:"type:text;not null" json:"answer"`
	// CitedArticleIDs is the whitelist-filtered article id array (保序去重).
	CitedArticleIDs JSON `gorm:"type:jsonb;not null" json:"cited_article_ids"`
	// CitedWebSources is the whitelist-filtered web source array (D7：
	// [{title,url}] 保序去重，url 必须在本次搜索结果集内；空写 [] 不写 null).
	CitedWebSources JSON      `gorm:"type:jsonb;not null" json:"cited_web_sources"`
	ExtractedTerms  JSON      `gorm:"type:jsonb;not null" json:"extracted_terms"`
	Operation       string    `gorm:"size:80" json:"operation,omitempty"`
	Provider        string    `gorm:"size:100" json:"provider,omitempty"`
	Model           string    `gorm:"size:100" json:"model,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func (AnnotationQA) TableName() string { return "annotation_qas" }

// TermNote — derived term library. term_norm 是归一化唯一键（trim + 全角→半角
// + 英文大小写折叠），同词再次出现命中既有条目 hit_count 累计，不重复建条。
// Embedding 是 P2 相似词归并预留（vector(2560)，P1 永不写入）；指针类型保证
// 零值落 NULL 而非空串（vector 列拒绝 ""，见 testing.md 陷阱表）。
type TermNote struct {
	ID               uint   `gorm:"primarykey" json:"id"`
	TermNorm         string `gorm:"type:text;not null;uniqueIndex:uq_term_notes_term_norm" json:"term_norm"`
	TermDisplay      string `gorm:"type:text;not null" json:"term_display"`
	HitCount         int    `gorm:"not null;default:1" json:"hit_count"`
	FirstSeenBoardID *uint  `json:"first_seen_board_id,omitempty"`
	// FirstSeenDate is the report period date of the first QA that derived the term.
	FirstSeenDate *time.Time `gorm:"type:date" json:"first_seen_date,omitempty"`
	LastSeenAt    time.Time  `gorm:"not null" json:"last_seen_at"`
	CreatedAt     time.Time  `json:"created_at"`
	Embedding     *string    `gorm:"type:vector(2560)" json:"-"`
}

func (TermNote) TableName() string { return "term_notes" }

// NormalizeTerm folds a raw term string onto its canonical form:
// trim → fullwidth→halfwidth → English lowercase folding. P1 精确匹配去重，
// 不做向量相似归并。
func NormalizeTerm(s string) string {
	// Fullwidth ASCII variants (U+FF01–U+FF5E) fold onto ASCII; fullwidth
	// space (U+3000) folds onto a plain space.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			b.WriteRune(r - 0xFEE0)
		case r == 0x3000:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(strings.ToLower(b.String()))
}

// NormalizeTerms normalizes a raw term list: per-term NormalizeTerm, 空串/纯
// 空白过滤（WB-6），保序去重。
func NormalizeTerms(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		norm := NormalizeTerm(t)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out
}

// canonicalizeIDJSONArray validates/normalizes a jsonb column that MUST hold an
// array of ids (thread 引用数组契约): nil → `[]`（空写 [] 不写 null）；`null`
// 标量或非数组 → 拒绝（WB-7）；合法数组 → 保序去重后重排。
func canonicalizeIDJSONArray(raw JSON) (JSON, error) {
	if raw == nil {
		return JSON("[]"), nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return JSON("[]"), nil
	}
	if trimmed == "null" {
		return nil, fmt.Errorf("cited_article_ids: null scalar violates the array contract (empty must be [])")
	}
	var ids []uint
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, fmt.Errorf("cited_article_ids: not a json array of ids: %w", err)
	}
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("cited_article_ids: encode: %w", err)
	}
	return JSON(encoded), nil
}

// canonicalizeStringJSONArray mirrors canonicalizeIDJSONArray for
// extracted_terms: nil/空 → `[]`，`null`/非数组拒绝，保序去重。
func canonicalizeStringJSONArray(raw JSON) (JSON, error) {
	if raw == nil {
		return JSON("[]"), nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return JSON("[]"), nil
	}
	if trimmed == "null" {
		return nil, fmt.Errorf("extracted_terms: null scalar violates the array contract (empty must be [])")
	}
	var terms []string
	if err := json.Unmarshal(raw, &terms); err != nil {
		return nil, fmt.Errorf("extracted_terms: not a json string array: %w", err)
	}
	encoded, err := json.Marshal(NormalizeTerms(terms))
	if err != nil {
		return nil, fmt.Errorf("extracted_terms: encode: %w", err)
	}
	return JSON(encoded), nil
}

// MarginNoteWebSource is one whitelisted web citation persisted on a QA turn
// (shape of annotation_qas.cited_web_sources jsonb entries, design D7).
type MarginNoteWebSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// canonicalizeWebSourceJSONArray enforces the cited_web_sources contract
// (design D7): nil/空 → `[]`，`null`/非数组/条目缺 url 拒绝；保序、按 url 去重，
// title 缺失保留空串（前端回退域名展示）。服务层已做白名单/截断，这里是
// jsonb 形状的最后一道防线（对齐 WB-7）。
func canonicalizeWebSourceJSONArray(raw JSON) (JSON, error) {
	if raw == nil {
		return JSON("[]"), nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return JSON("[]"), nil
	}
	if trimmed == "null" {
		return nil, fmt.Errorf("cited_web_sources: null scalar violates the array contract (empty must be [])")
	}
	var sources []MarginNoteWebSource
	if err := json.Unmarshal(raw, &sources); err != nil {
		return nil, fmt.Errorf("cited_web_sources: not a json array of {title,url}: %w", err)
	}
	seen := make(map[string]struct{}, len(sources))
	out := make([]MarginNoteWebSource, 0, len(sources))
	for _, s := range sources {
		u := strings.TrimSpace(s.URL)
		if u == "" {
			return nil, fmt.Errorf("cited_web_sources: entry with empty url")
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, MarginNoteWebSource{Title: strings.TrimSpace(s.Title), URL: u})
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("cited_web_sources: encode: %w", err)
	}
	return JSON(encoded), nil
}

// CreateAnnotation persists a new margin note anchor. The target ownership
// (section ∈ report, thread ∈ section) is validated by the caller via
// ValidateAnnotationTarget before this insert.
func (r *TopicGraphRepository) CreateAnnotation(a *ReportAnnotation) error {
	if err := r.db.Create(a).Error; err != nil {
		return fmt.Errorf("annotation: create: %w", err)
	}
	return nil
}

// GetAnnotation loads one annotation with its QA turns (ordered oldest→newest).
// Returns a gorm.ErrRecordNotFound-wrapped error when absent (handler → 404).
func (r *TopicGraphRepository) GetAnnotation(id uint) (*ReportAnnotation, error) {
	var a ReportAnnotation
	err := r.db.Preload("QAs", func(db *gorm.DB) *gorm.DB {
		return db.Order("annotation_qas.id ASC")
	}).First(&a, id).Error
	if err != nil {
		return nil, fmt.Errorf("annotation: get %d: %w", id, err)
	}
	return &a, nil
}

// ListAnnotationsByReport loads all annotations of one report with QA turns,
// creation order (≈ 正文出现顺序)。
func (r *TopicGraphRepository) ListAnnotationsByReport(reportID uint) ([]ReportAnnotation, error) {
	var out []ReportAnnotation
	err := r.db.Preload("QAs", func(db *gorm.DB) *gorm.DB {
		return db.Order("annotation_qas.id ASC")
	}).Where("report_id = ?", reportID).Order("id ASC").Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("annotation: list by report %d: %w", reportID, err)
	}
	return out, nil
}

// DeleteAnnotation removes an annotation AND all its QA turns in one
// transaction (删除批注连带问答)。The FK fk_annotation_qas_annotation would
// cascade anyway; the explicit delete keeps the contract visible and testable.
func (r *TopicGraphRepository) DeleteAnnotation(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("annotation_id = ?", id).Delete(&AnnotationQA{}).Error; err != nil {
			return fmt.Errorf("annotation: delete qas of %d: %w", id, err)
		}
		res := tx.Delete(&ReportAnnotation{}, id)
		if res.Error != nil {
			return fmt.Errorf("annotation: delete %d: %w", id, res.Error)
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// ValidateAnnotationTarget checks anchor ownership: section must belong to the
// report and, when present, thread must belong to that (report, section).
// 例外：头条 lead 批注（threadID 为 nil）可以没有 section 归属——masthead 头条
// 可能是 highlight 派生（不属于任何 section），此时前端落 section_id=0 哨兵值；
// 无 section 归属不影响锚定（定位靠 data-mn-anchor="lead" + quoted_text）。
func (r *TopicGraphRepository) ValidateAnnotationTarget(reportID, sectionID uint, threadID *uint) error {
	if threadID == nil && sectionID == 0 {
		return nil
	}
	var count int64
	if err := r.db.Model(&DailyReportSection{}).
		Where("id = ? AND report_id = ?", sectionID, reportID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("annotation: validate section: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("section %d does not belong to report %d", sectionID, reportID)
	}
	if threadID != nil {
		if err := r.db.Model(&DailyReportThread{}).
			Where("id = ? AND report_id = ? AND section_id = ?", *threadID, reportID, sectionID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("annotation: validate thread: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("thread %d does not belong to (report %d, section %d)", *threadID, reportID, sectionID)
		}
	}
	return nil
}

// InsertAnnotationQA persists one QA turn, enforcing the jsonb array contract
// on both array columns before the write.
func (r *TopicGraphRepository) InsertAnnotationQA(qa *AnnotationQA) error {
	cited, err := canonicalizeIDJSONArray(qa.CitedArticleIDs)
	if err != nil {
		return err
	}
	qa.CitedArticleIDs = cited
	webSources, err := canonicalizeWebSourceJSONArray(qa.CitedWebSources)
	if err != nil {
		return err
	}
	qa.CitedWebSources = webSources
	terms, err := canonicalizeStringJSONArray(qa.ExtractedTerms)
	if err != nil {
		return err
	}
	qa.ExtractedTerms = terms
	if err := r.db.Create(qa).Error; err != nil {
		return fmt.Errorf("annotation qa: insert: %w", err)
	}
	return nil
}

// UpsertTermNotes derives terms into term_notes: per-term 归一化（NormalizeTerm），
// 空串过滤、保序去重；同词命中既有条目 hit_count+1 不新建（WB-5），新词记录
// 首次遇到的版块与日期，display 保留首次出现的原始形态（trim 后）。Returns
// the normalized terms that were newly created this call（前端「入库」角标）。
// seenAt 是报告 period date 派生的时间锚。
func (r *TopicGraphRepository) UpsertTermNotes(boardID *uint, seenAt time.Time, terms []string) (newTerms []string, err error) {
	norms := NormalizeTerms(terms)
	if len(norms) == 0 {
		return nil, nil
	}
	// display 保留首次出现的原始形态（norm → first raw trimmed variant）。
	display := make(map[string]string, len(norms))
	seen := make(map[string]struct{}, len(terms))
	for _, raw := range terms {
		norm := NormalizeTerm(raw)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		display[norm] = strings.TrimSpace(raw)
	}

	// Which norms already exist? Single-user system — pre-select instead of
	// RETURNING tricks keeps the upsert dialect-portable.
	var existing []string
	if err := r.db.Model(&TermNote{}).Where("term_norm IN ?", norms).
		Pluck("term_norm", &existing).Error; err != nil {
		return nil, fmt.Errorf("term_notes: probe existing: %w", err)
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existingSet[e] = struct{}{}
	}

	newTerms = make([]string, 0, len(norms))
	for _, norm := range norms {
		seenDate := time.Date(seenAt.Year(), seenAt.Month(), seenAt.Day(), 0, 0, 0, 0, seenAt.Location())
		row := TermNote{
			TermNorm:      norm,
			TermDisplay:   display[norm],
			HitCount:      1,
			LastSeenAt:    seenAt,
			FirstSeenDate: &seenDate,
		}
		if boardID != nil {
			bid := *boardID
			row.FirstSeenBoardID = &bid
		}
		q := r.db.Clauses(upsertTermClause()).
			Create(&row)
		if q.Error != nil {
			return nil, fmt.Errorf("term_notes: upsert %q: %w", norm, q.Error)
		}
		if _, ok := existingSet[norm]; !ok {
			newTerms = append(newTerms, norm)
		}
	}
	return newTerms, nil
}

// AnnotationListFilter drives the cross-report management-page list.
type AnnotationListFilter struct {
	BoardID  *uint  // optional board filter
	Query    string // matches quoted_text / question / term_norm (ILIKE)
	Page     int    // 1-based; <=0 → 1
	PageSize int    // <=0 → default 20; hard cap 100
}

// AnnotationListItem is one management-page row: annotation + QA turns +
// resolved board/period display fields + aggregated term chips (deduped across
// the annotation's QAs, normalized form). BoardID/BoardLabel 是管理页展示与
// 「跳原日报」深链所需（前端 board 查询参数同 id，缺了会落「未知版块」且跳转丢失）。
type AnnotationListItem struct {
	ReportAnnotation
	BoardID    uint      `json:"board_id"`
	BoardLabel string    `json:"board_label"`
	PeriodDate time.Time `json:"period_date"`
	Terms      []string  `json:"terms"`
}

// annotationListRow is the raw scan target for the management list (no
// association fields — GORM Scan cannot map relations and would warn).
type annotationListRow struct {
	ID                uint      `gorm:"column:id"`
	ReportID          uint      `gorm:"column:report_id"`
	SectionID         uint      `gorm:"column:section_id"`
	ThreadID          *uint     `gorm:"column:thread_id"`
	QuotedText        string    `gorm:"column:quoted_text"`
	AnchorOffsetStart int       `gorm:"column:anchor_offset_start"`
	AnchorOffsetEnd   int       `gorm:"column:anchor_offset_end"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	SemanticBoardID   uint      `gorm:"column:semantic_board_id"`
	BoardLabel        string    `gorm:"column:board_label"`
	PeriodDate        time.Time `gorm:"column:period_date"`
}

// ListAnnotationsForManagement returns the cross-report annotation list,
// date-descending, with QA turns and term chips resolved.
func (r *TopicGraphRepository) ListAnnotationsForManagement(filter AnnotationListFilter) ([]AnnotationListItem, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	base := r.db.Table("report_annotations AS ra").
		Joins("JOIN board_daily_reports br ON br.id = ra.report_id").
		Joins("LEFT JOIN semantic_labels sb ON sb.id = br.semantic_board_id")
	if filter.BoardID != nil {
		base = base.Where("br.semantic_board_id = ?", *filter.BoardID)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + marginNoteLikePattern(q) + "%"
		// LOWER(...) LIKE ... 配 ESCAPE 子句——PostgreSQL 与 SQLite 行为一致
		// （ILIKE 在 SQLite 不存在）。命中范围：划词 / 提问 / 术语（spec「列表与筛选」）。
		base = base.Where(
			"LOWER(ra.quoted_text) LIKE LOWER(?) ESCAPE '\\' OR EXISTS (SELECT 1 FROM annotation_qas aq WHERE aq.annotation_id = ra.id AND LOWER(aq.question) LIKE LOWER(?) ESCAPE '\\') OR EXISTS (SELECT 1 FROM annotation_qas aq2 WHERE aq2.annotation_id = ra.id AND LOWER(CAST(aq2.extracted_terms AS TEXT)) LIKE LOWER(?) ESCAPE '\\')",
			like, like, like,
		)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("annotation: management count: %w", err)
	}

	var rawRows []annotationListRow
	err := base.
		Select("ra.id AS id, ra.report_id AS report_id, ra.section_id AS section_id, ra.thread_id AS thread_id, ra.quoted_text AS quoted_text, ra.anchor_offset_start AS anchor_offset_start, ra.anchor_offset_end AS anchor_offset_end, ra.created_at AS created_at, br.semantic_board_id AS semantic_board_id, CASE WHEN sb.label IS NULL THEN '' ELSE sb.label END AS board_label, br.period_date AS period_date").
		Order("ra.created_at DESC, ra.id DESC").
		Limit(filter.PageSize).Offset((filter.Page - 1) * filter.PageSize).
		Scan(&rawRows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("annotation: management list: %w", err)
	}
	rows := make([]AnnotationListItem, 0, len(rawRows))
	for _, raw := range rawRows {
		rows = append(rows, AnnotationListItem{
			ReportAnnotation: ReportAnnotation{
				ID:                raw.ID,
				ReportID:          raw.ReportID,
				SectionID:         raw.SectionID,
				ThreadID:          raw.ThreadID,
				QuotedText:        raw.QuotedText,
				AnchorOffsetStart: raw.AnchorOffsetStart,
				AnchorOffsetEnd:   raw.AnchorOffsetEnd,
				CreatedAt:         raw.CreatedAt,
			},
			BoardID:    raw.SemanticBoardID,
			BoardLabel: raw.BoardLabel,
			PeriodDate: raw.PeriodDate,
		})
	}

	if err := r.attachQAsAndTerms(rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// attachQAsAndTerms batch-loads QA turns for the listed annotations and folds
// each annotation's terms (extracted_terms across QAs) into Terms chips.
func (r *TopicGraphRepository) attachQAsAndTerms(rows []AnnotationListItem) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var qas []AnnotationQA
	if err := r.db.Where("annotation_id IN ?", ids).Order("id ASC").Find(&qas).Error; err != nil {
		return fmt.Errorf("annotation: management preload qas: %w", err)
	}
	byAnnotation := make(map[uint][]AnnotationQA, len(rows))
	for _, qa := range qas {
		byAnnotation[qa.AnnotationID] = append(byAnnotation[qa.AnnotationID], qa)
	}
	for i := range rows {
		rows[i].QAs = byAnnotation[rows[i].ID]
		if rows[i].QAs == nil {
			rows[i].QAs = []AnnotationQA{}
		}
		terms := make([]string, 0, 4)
		seen := make(map[string]struct{}, 4)
		for _, qa := range rows[i].QAs {
			var qaTerms []string
			if len(qa.ExtractedTerms) > 0 {
				if err := json.Unmarshal(qa.ExtractedTerms, &qaTerms); err != nil {
					continue // 冗余展示字段，解析失败跳过不阻断列表
				}
			}
			for _, t := range NormalizeTerms(qaTerms) {
				if _, ok := seen[t]; ok {
					continue
				}
				seen[t] = struct{}{}
				terms = append(terms, t)
			}
		}
		rows[i].Terms = terms
	}
	return nil
}

// GetThreadContext loads the (title, summary, related article ids) of a thread
// for QA prompt assembly. A thread's related_article_ids order is the prompt's
// article priority order.
func (r *TopicGraphRepository) GetThreadContext(threadID uint) (title, summary string, relatedIDs []uint, err error) {
	var thread DailyReportThread
	if err := r.db.First(&thread, threadID).Error; err != nil {
		return "", "", nil, fmt.Errorf("thread: get %d: %w", threadID, err)
	}
	if len(thread.RelatedArticleIDs) > 0 {
		if err := json.Unmarshal(thread.RelatedArticleIDs, &relatedIDs); err != nil {
			return "", "", nil, fmt.Errorf("thread %d: parse related_article_ids: %w", threadID, err)
		}
	}
	return thread.Title, thread.Summary, relatedIDs, nil
}

// ArticleExcerpt is one related-article context block for the QA prompt.
type ArticleExcerpt struct {
	ID      uint   `json:"id"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
}

// GetArticleExcerpts loads articles by id and builds prompt excerpts (标题 +
// 正文摘录 ≤maxRunes)。引用文章反查豁免 archived 过滤（daily-report 红线 10，
// 与 thread 引用同口径——缺行/软删均可读，保证历史日报问答永久可用）。Output
// preserves the caller's id order; missing ids are silently dropped.
func (r *TopicGraphRepository) GetArticleExcerpts(ids []uint, maxRunes int) ([]ArticleExcerpt, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var articles []models.Article
	if err := r.db.Where("id IN ?", ids).Find(&articles).Error; err != nil {
		return nil, fmt.Errorf("articles: excerpt lookup: %w", err)
	}
	byID := make(map[uint]models.Article, len(articles))
	for _, a := range articles {
		byID[a.ID] = a
	}
	out := make([]ArticleExcerpt, 0, len(ids))
	for _, id := range ids {
		a, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, ArticleExcerpt{ID: a.ID, Title: a.Title, Excerpt: truncateRunesMax(articleBody(a), maxRunes)})
	}
	return out, nil
}

// articleBody picks the best available excerpt source: crawled/full content →
// AI summary → feed description.
func articleBody(a models.Article) string {
	switch {
	case strings.TrimSpace(a.Content) != "":
		return a.Content
	case strings.TrimSpace(a.AIContentSummary) != "":
		return a.AIContentSummary
	default:
		return a.Description
	}
}

// truncateRunesMax keeps at most max runes of s.
func truncateRunesMax(s string, max int) string {
	runes := []rune(s)
	if max <= 0 || len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// marginNoteLikePattern escapes LIKE wildcards for the management-page query,
// paired with the `ESCAPE '\'` clause (portable across PostgreSQL/SQLite).
func marginNoteLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// upsertTermClause builds the dialect-portable ON CONFLICT upsert: same term
// hits the existing row and accumulates hit_count (WB-5)，不重复建条。
func upsertTermClause() clause.Expression {
	return clause.OnConflict{
		Columns: []clause.Column{{Name: "term_norm"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"hit_count":    gorm.Expr("term_notes.hit_count + ?", 1),
			"last_seen_at": gorm.Expr("excluded.last_seen_at"),
		}),
	}
}
