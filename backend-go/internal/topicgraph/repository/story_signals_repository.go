package repository

import (
	"fmt"
	"sort"
	"strconv"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/textutil"
)

// RecomputeSignalsForStory rebuilds evidence from the current article union.
// Delete and insert share a transaction, so repeated attribution converges.
func (r *TopicGraphRepository) RecomputeSignalsForStory(storyID uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return recomputeSignalsForStoryWith(tx, storyID)
	})
}

func recomputeSignalsForStoryWith(tx *gorm.DB, storyID uint) error {
	var threads []DailyReportThread
	if err := tx.Select("related_article_ids").Where("story_id = ?", storyID).Find(&threads).Error; err != nil {
		return fmt.Errorf("load story %d signal articles: %w", storyID, err)
	}
	unique := make(map[uint]bool)
	for _, thread := range threads {
		for _, id := range jsonUintIDs(thread.RelatedArticleIDs) {
			unique[id] = true
		}
	}
	ids := make([]uint, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var articles []models.Article
	if len(ids) > 0 {
		// archived means feed-window rotation, not withdrawal/invisibility: a
		// source's reporting remains evidence after rotation. See sourcestats
		// package constraint 2: window queries MUST NOT filter on archived.
		// Future withdrawal/abuse exclusion needs a dedicated column, not archived.
		if err := tx.Select("id", "feed_id", "pub_date", "created_at", "link").
			Where("id IN ?", ids).Find(&articles).Error; err != nil {
			return fmt.Errorf("load story %d evidence articles: %w", storyID, err)
		}
	}
	if err := tx.Where("story_id = ?", storyID).Delete(&StorySignal{}).Error; err != nil {
		return fmt.Errorf("delete story %d signals: %w", storyID, err)
	}
	signals := make([]StorySignal, 0, len(articles))
	for _, article := range articles {
		observed, source := article.CreatedAt, "fallback"
		if article.PubDate != nil {
			observed, source = *article.PubDate, "pub_date"
		}
		signals = append(signals, StorySignal{
			StoryID: storyID, ArticleID: article.ID, FeedID: article.FeedID,
			ParticipantKey: "feed:" + strconv.FormatUint(uint64(article.FeedID), 10),
			// 内容身份（内容身份优先、feed 兜底）：同一篇内容被多个 feed 抓
			// 取时两副本的 content_key 相同（规范化链接），后续去重按它而不
			// 是 feed（design D2/D4）。空串 = 内容身份不可判定（link 为空），
			// 消费者退化为 participant_key。
			//
			// 红线：archived 不得过滤——archived 只表示 feed 活跃窗口轮转，不是
			// 撤回/不可见；证据与参与者照常计入（见上方加载处的说明）。
			ContentKey: textutil.NormalizeArticleLink(article.Link),
			Kind:       StorySignalKindEditorial, ObservedAt: observed, ObservedAtSource: source,
		})
	}
	if len(signals) > 0 {
		if err := tx.CreateInBatches(&signals, 200).Error; err != nil {
			return fmt.Errorf("insert story %d signals: %w", storyID, err)
		}
	}
	return recomputeStoryParticipantsWith(tx, storyID)
}

// StoryParticipantCount returns the participant count of a story computed from
// current evidence — the read-only twin of recomputeStoryParticipantsWith
// (same SQL, same formula). Callers use it to spot stories whose stored
// participant_count no longer matches their evidence (e.g. rows backfilled
// without a count recompute).
func (r *TopicGraphRepository) StoryParticipantCount(storyID uint) (int64, error) {
	return storyParticipantCountWith(r.db, storyID)
}

func recomputeStoryParticipantsWith(tx *gorm.DB, storyID uint) error {
	count, err := storyParticipantCountWith(tx, storyID)
	if err != nil {
		return err
	}
	if err := tx.Model(&BoardStory{}).Where("id = ?", storyID).Update("participant_count", count).Error; err != nil {
		return fmt.Errorf("write story %d participants: %w", storyID, err)
	}
	return nil
}

// storyParticipantCountWith is the single implementation of the participant
// count (fix-duplicate-article-heat design D3).
func storyParticipantCountWith(tx *gorm.DB, storyID uint) (int64, error) {
	var count int64
	// As in sourcestats package constraint 2, never filter archived here:
	// feed-window rotation does not erase a source's reporting. Withdrawal or
	// abuse exclusion must use a dedicated column, never the archived flag.
	//
	// 参与者去重键（fix-duplicate-article-heat design D3）：内容身份优先、
	// feed 兜底——COALESCE(NULLIF(content_key,''), participant_key)。同一篇
	// 内容被多个 feed 各抓一份时只算 1 个参与者；content_key 为空/NULL（link
	// 为空或存量行未回填）时退化为 feed:<id>，不同 feed 不合并。该表达式与
	// Go 侧 heatParticipantKey 逐字等价。
	//
	// 注意：不能用 gorm 的 Distinct(expr)+Count——v1.25.12 的 Count 会把
	// Selects[0] 重写成 count(*)，DISTINCT 表达式被静默丢弃（口径失效）。
	// 因此显式写出 COUNT(DISTINCT ...)。
	if err := tx.Table("story_signals AS s").
		Joins("JOIN board_stories AS b ON b.id = s.story_id").
		Joins("JOIN articles AS a ON a.id = s.article_id").
		Where("(b.id = ? OR b.merged_into = ?)", storyID, storyID).
		Select("COUNT(DISTINCT COALESCE(NULLIF(s.content_key, ''), s.participant_key))").
		Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count story %d participants: %w", storyID, err)
	}
	return count, nil
}
