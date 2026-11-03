package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ── 冻结来源（add-question-driven-news-research D2：发现后只冻结候选所引
// 的本地来源，不逐篇 AI 筛选）─────────────────────────────────────────────────
//
// 流程：周期装配 → detect（聚合材料）→ 校验候选 → 按候选有效 section refs
// 批量查询关联 article → 冻结有界本地正文 → 原子保存 discovery + candidates。
// 正文只在候选形成后从本地按关联取样冻结，不送入发现模型；发现后原文被修
// 改/删除不影响已冻结快照（快照即版本）。

// 冻结预算（D2：可读预算，不宣称最重要的 8 篇；未纳入数和原因记 gap）。
// 总量天然有界 = 8 篇 × 8000 rune，不再另设总量门（其永不先于篇数门触发）。
const (
	SignalSourceMaxPerCandidate = 8    // 每候选最多冻结的不同 article 数
	SignalSourceMaxArticleRunes = 8000 // 每篇冻结正文上限（rune）
)

// 来源类型（appendix sources[].kind；本地存文与外部网页分列）。
const (
	signalSourceKindLocal = "local_article"
	signalSourceKindWeb   = "web_page"
)

// SignalFrozenSource 是 input_snapshot.sources[] 的一行：系统生成 source_id
// （n1、n2…按 article 顺序稳定分配）、article/section 关联、来源时间与截取
// 说明。captured_at 是实际复制时间（不是历史可得性证明）。
type SignalFrozenSource struct {
	SourceID      string     `json:"source_id"`
	ArticleID     uint       `json:"article_id"`
	SectionIDs    []uint     `json:"section_ids"`
	LaneID        uint       `json:"lane_id"`
	Title         string     `json:"title"`
	URL           string     `json:"url,omitempty"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	CapturedAt    time.Time  `json:"captured_at"`
	ContentHash   string     `json:"content_hash"`
	Text          string     `json:"text"`
	Truncated     bool       `json:"truncated"`
	OriginalRunes int        `json:"original_runes"`
	TemporalNote  string     `json:"temporal_note,omitempty"`
}

// SignalSourceArticle 是 repository 批量取源的一行（已验证 section 在本批、
// 本 board 且 article 确经该 section 的 thread 关联；Content 为
// firecrawl_content > content 选出的非空内容）。
type SignalSourceArticle struct {
	ArticleID uint
	SectionID uint
	// SectionIDs 由聚合层填充（跨 section 的同 article 合并归属）；冻结来源
	// 保存聚合后的全部 section。
	SectionIDs []uint
	LaneID     uint
	Title      string
	URL        string
	PubDate    *time.Time
	Content    string
}

// LoadSignalSourceArticles 批量取源（repository 侧实现的真实查询）：
// 只接受材料白名单内的 section，经 daily_report_threads.related_article_ids
// 验证 article 关联，返回去重后的 article 行。跨 section 的同一 article 保
// 留两个 section_id 映射（由调用方聚合）。
// 返回值 badThreads 是 related_article_ids 损坏（非法 jsonb 形态）的 thread
// 行数——按缺失处理不中断整批，由调用方记 gap 可见（L5）。
func LoadSignalSourceArticles(ctx context.Context, db *gorm.DB, boardID uint, sectionIDs []uint, sectionLane map[uint]uint) ([]SignalSourceArticle, int, error) {
	if len(sectionIDs) == 0 {
		return nil, 0, nil
	}
	// 关联验证：section 属于本 board 的日报 → 该 section 的 thread →
	// thread.related_article_ids 展开成 (section, article) 对。
	var threadRows []struct {
		SectionID uint   `gorm:"column:section_id"`
		ArtIDs    []byte `gorm:"column:related_article_ids"`
	}
	if err := db.WithContext(ctx).
		Table("daily_report_threads t").
		Select("t.section_id, t.related_article_ids").
		Joins("JOIN daily_report_sections s ON s.id = t.section_id").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id").
		Where("t.section_id IN ? AND r.semantic_board_id = ?", sectionIDs, boardID).
		Find(&threadRows).Error; err != nil {
		return nil, 0, fmt.Errorf("load signal source threads: %w", err)
	}
	type artKey struct{ article, section uint }
	seen := make(map[artKey]bool)
	var articleIDs []uint
	articleSections := make(map[uint][]uint) // article → sections（保序去重）
	badLists := 0
	for _, row := range threadRows {
		ids, err := decodeUintList(row.ArtIDs)
		if err != nil {
			badLists++ // 损坏的关联列表按缺失处理，不中断整批；计数记 gap（L5）
			continue
		}
		for _, aid := range ids {
			k := artKey{aid, row.SectionID}
			if seen[k] {
				continue
			}
			seen[k] = true
			if _, known := articleSections[aid]; !known {
				articleIDs = append(articleIDs, aid)
			}
			articleSections[aid] = append(articleSections[aid], row.SectionID)
		}
	}
	if len(articleIDs) == 0 {
		return nil, badLists, nil
	}
	var articles []struct {
		ID              uint       `gorm:"column:id"`
		Title           string     `gorm:"column:title"`
		Link            string     `gorm:"column:link"`
		PubDate         *time.Time `gorm:"column:pub_date"`
		Content         string     `gorm:"column:content"`
		FirecrawlStatus string     `gorm:"column:firecrawl_status"`
		Firecrawl       string     `gorm:"column:firecrawl_content"`
	}
	if err := db.WithContext(ctx).
		Table("articles").
		Select("id, title, link, pub_date, content, firecrawl_status, firecrawl_content").
		Where("id IN ?", articleIDs).
		Find(&articles).Error; err != nil {
		return nil, badLists, fmt.Errorf("load signal source articles: %w", err)
	}
	out := make([]SignalSourceArticle, 0, len(articles))
	for _, a := range articles {
		// firecrawl_content > content：选非空内容；description 只作摘要不可
		// 冒充正文（不在此取）。
		content := pickSignalArticleContent(a.Firecrawl, a.Content)
		sections := articleSections[a.ID]
		laneID := uint(0)
		for _, sid := range sections {
			if l, ok := sectionLane[sid]; ok {
				laneID = l
				break
			}
		}
		out = append(out, SignalSourceArticle{
			ArticleID: a.ID,
			SectionID: sections[0],
			LaneID:    laneID,
			Title:     a.Title,
			URL:       a.Link,
			PubDate:   a.PubDate,
			Content:   content,
		})
	}
	return out, badLists, nil
}

// decodeUintList 解析 related_article_ids jsonb（uint 数组）。
func decodeUintList(raw []byte) ([]uint, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty")
	}
	var ids []uint
	if err := jsonUnmarshalStrict(raw, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// pickSignalArticleContent 选非空正文（firecrawl 优先）。
func pickSignalArticleContent(firecrawl, content string) string {
	if strings.TrimSpace(firecrawl) != "" {
		return firecrawl
	}
	return content
}

// signalMaterialSectionIndex 汇集材料的有效切片（section→lane 映射与列表），
// 供批量取源的白名单验证（只接受本批、本 board 的 section）。
func signalMaterialSectionIndex(material *SignalMaterial) ([]uint, map[uint]uint) {
	sectionLane := make(map[uint]uint)
	ids := make([]uint, 0)
	for _, lane := range material.Lanes {
		for _, a := range lane.Articles {
			if _, ok := sectionLane[a.SectionID]; !ok {
				ids = append(ids, a.SectionID)
			}
			sectionLane[a.SectionID] = lane.LaneID
		}
	}
	return ids, sectionLane
}

// SignalSourceFreeze 是一次发现的冻结产物：跨候选共享的去重来源集合（按
// article_id+content_hash 去重）+ 每候选 brief（含 source_ids）+ 未纳入
// 缺口说明。服务层把它并入 input_snapshot 与候选行后原子保存。
type SignalSourceFreeze struct {
	Sources []SignalFrozenSource
	Briefs  []*SignalResearchBrief // 与 detections 等长对齐
	Gaps    []string
}

// FreezeSignalQuestionSources 纯函数核心：按候选 evidence_refs 顺序（再按
// thread/article 稳定序）从已验证的 article 行冻结有界正文。规则（D2）：
//   - 每候选最多 8 篇不同 article；跨切片同 article 去重（不重复拼正文）；
//   - 每篇正文截 8000rune（保留 truncated/original_runes），单候选总量
//     64000rune；未纳入记 gap（不因此丢候选）；
//   - 本地 pub_date 晚于材料 cutoff 拒绝；未知日期冻结但带 temporal_note
//     （不当作合窗原文，研究端如实提示）；
//   - 材料不含正文相关字段（本函数只吃已取回的 article 行，不触网）。
func FreezeSignalQuestionSources(material *SignalMaterial, detections []SignalDetection, articles []SignalSourceArticle, now time.Time) *SignalSourceFreeze {
	freeze := &SignalSourceFreeze{Briefs: make([]*SignalResearchBrief, len(detections))}

	// 白名单：本批材料的有效切片（section → lane）。
	sectionLane := make(map[uint]uint)
	whitelisted := make(map[uint]bool)
	for _, lane := range material.Lanes {
		for _, a := range lane.Articles {
			sectionLane[a.SectionID] = lane.LaneID
			whitelisted[a.SectionID] = true
		}
	}
	// article 索引：article_id → 聚合行（跨 section 的同 article 合并
	// SectionIDs，不重复拼正文）；section → 归属 article 集合（稳定序）。
	byArticle := make(map[uint]*SignalSourceArticle, len(articles))
	var orderedIDs []uint
	for i := range articles {
		art := articles[i]
		if existing, ok := byArticle[art.ArticleID]; ok {
			if len(art.SectionIDs) > 0 {
				existing.SectionIDs = mergeUintUnique(existing.SectionIDs, art.SectionIDs)
			} else {
				existing.SectionIDs = mergeUintUnique(existing.SectionIDs, []uint{art.SectionID})
			}
			continue
		}
		merged := art
		merged.SectionIDs = []uint{art.SectionID}
		byArticle[art.ArticleID] = &merged
		orderedIDs = append(orderedIDs, art.ArticleID)
	}
	sectionArticles := make(map[uint][]uint)
	for _, aid := range orderedIDs {
		for _, sid := range byArticle[aid].SectionIDs {
			sectionArticles[sid] = append(sectionArticles[sid], aid)
		}
	}

	// 跨候选共享来源（discovery 级去重：article_id+content_hash）。
	shared := make(map[uint]*SignalFrozenSource)
	assignSourceID := func(n int) string { return fmt.Sprintf("n%d", n) }
	nextSource := 0

	for di, det := range detections {
		brief := &SignalResearchBrief{
			Unknown:      det.Unknown,
			Verification: det.Verification,
			SourceIDs:    []string{},
		}
		freeze.Briefs[di] = brief
		count := 0
		seenArticle := make(map[uint]bool)
		for _, ref := range det.EvidenceRefs {
			// L4：严格解析（"101abc" 不再被宽恕为 101），与材料侧 ParseUint 口径一致。
			v, err := strconv.ParseUint(strings.TrimSpace(ref), 10, 64)
			if err != nil {
				continue
			}
			sectionID := uint(v)
			if !whitelisted[sectionID] {
				continue // 依据白名单已由 detect 过滤，此处防御性跳过
			}
			for _, aid := range sectionArticles[sectionID] {
				if seenArticle[aid] || count >= SignalSourceMaxPerCandidate {
					continue
				}
				art := byArticle[aid]
				if art == nil {
					continue
				}
				if art.PubDate != nil && art.PubDate.After(material.Cutoff) {
					freeze.Gaps = append(freeze.Gaps, fmt.Sprintf("候选「%s」：文章 %d《%s》发布时间 %s 晚于材料截止 %s，不冻结（事后材料）", det.Signal, aid, art.Title, art.PubDate.Format(time.RFC3339), material.Cutoff.Format(time.RFC3339)))
					seenArticle[aid] = true
					continue
				}
				// 空正文不冻结、不开先读门、不可作证据（H3，spec「来源截取与空正文」：
				// 本地侧与网页侧同语义；记 gap 不中断整批）。
				if strings.TrimSpace(art.Content) == "" {
					freeze.Gaps = append(freeze.Gaps, fmt.Sprintf("候选「%s」：文章 %d《%s》正文为空，不冻结（原文不可用）", det.Signal, aid, art.Title))
					seenArticle[aid] = true
					continue
				}
				// 冻结正文（有界截取）。
				runes := []rune(art.Content)
				original := len(runes)
				var text string
				truncated := false
				budget := SignalSourceMaxArticleRunes
				if original > budget {
					text = string(runes[:budget])
					truncated = true
				} else {
					text = art.Content
				}
				src, exists := shared[aid]
				if !exists {
					nextSource++
					src = &SignalFrozenSource{
						SourceID:      assignSourceID(nextSource),
						ArticleID:     aid,
						SectionIDs:    append([]uint(nil), art.SectionIDs...),
						LaneID:        sectionLane[sectionID],
						Title:         art.Title,
						URL:           art.URL,
						PublishedAt:   art.PubDate,
						CapturedAt:    now,
						ContentHash:   hashSignalText(text),
						Text:          text,
						Truncated:     truncated,
						OriginalRunes: original,
					}
					if art.PubDate == nil {
						src.TemporalNote = "原文无可靠发布日期：不当作合窗原文使用，引用时如实说明"
					}
					if material.AnalysisMode == "retrospective" && src.TemporalNote == "" {
						src.TemporalNote = "历史周期：事后取回的当时材料"
					}
					shared[aid] = src
					freeze.Sources = append(freeze.Sources, *src)
				}
				brief.SourceIDs = append(brief.SourceIDs, src.SourceID)
				count++
				seenArticle[aid] = true
			}
			if count >= SignalSourceMaxPerCandidate {
				break
			}
		}
		if remaining := countRemaining(sectionArticles, det, whitelisted, seenArticle); remaining > 0 {
			freeze.Gaps = append(freeze.Gaps, fmt.Sprintf("候选「%s」：有 %d 篇关联文章超出 8 篇/总量预算未纳入冻结（预算截断不代表证据不存在）", det.Signal, remaining))
		}
	}
	return freeze
}

// mergeUintUnique 有序合并 uint 切片（去重）。
func mergeUintUnique(a, b []uint) []uint {
	seen := make(map[uint]bool, len(a)+len(b))
	out := append([]uint(nil), a...)
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// countRemaining 统计候选依据内未纳入冻结的关联文章数（记 gap 用）。
func countRemaining(sectionArticles map[uint][]uint, det SignalDetection, whitelisted map[uint]bool, seenArticle map[uint]bool) int {
	remaining := 0
	for _, ref := range det.EvidenceRefs {
		v, err := strconv.ParseUint(strings.TrimSpace(ref), 10, 64)
		if err != nil || !whitelisted[uint(v)] {
			continue
		}
		for _, aid := range sectionArticles[uint(v)] {
			if !seenArticle[aid] {
				remaining++
			}
		}
	}
	return remaining
}
