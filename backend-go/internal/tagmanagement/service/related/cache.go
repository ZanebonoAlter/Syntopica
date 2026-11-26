package related

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// SourceTag is the reduced projection both refresh and query need: the tag's
// identity, the two descriptive fields and its CURRENT auxiliary labels.
type SourceTag struct {
	ID          uint
	Label       string
	Description string
	CreatedAt   time.Time
	AuxLabels   []string
}

// boardExists reports whether id is a semantic board (label_type='board').
// A disabled board is still a board — visibility filtering is deliberately not
// part of this check (design D5 says nothing about board status).
func boardExists(ctx context.Context, db *gorm.DB, boardID uint) (bool, error) {
	var count int64
	err := db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Where("id = ? AND label_type = ?", boardID, "board").
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check board %d: %w", boardID, err)
	}
	return count > 0, nil
}

// loadBoardEventTags returns the board's effective event tags in id order.
// "Effective" reproduces the archive visibility rule: the tag is active and its
// mount is not direction_mismatch (mismatched mounts are recorded but hidden by
// default, so they are never related-event candidates either).
//
// The window is applied to topic_tags.created_at — the label's ingestion time,
// which is NOT the event's occurrence date and is never presented as one.
func loadBoardEventTags(ctx context.Context, db *gorm.DB, boardID uint, from, to *time.Time, tagIDs []uint) ([]SourceTag, error) {
	query := db.WithContext(ctx).
		Table("topic_tags AS tt").
		Select("tt.id, tt.label, tt.description, tt.created_at").
		Joins("JOIN topic_tag_board_labels AS tbl ON tbl.topic_tag_id = tt.id AND tbl.semantic_board_id = ?", boardID).
		Where("tt.category = ?", models.TagCategoryEvent).
		Where("tt.status = ?", "active").
		Where("NOT COALESCE(tbl.direction_mismatch, false)")
	if from != nil {
		query = query.Where("tt.created_at >= ?", *from)
	}
	if to != nil {
		query = query.Where("tt.created_at <= ?", *to)
	}
	if len(tagIDs) > 0 {
		query = query.Where("tt.id IN ?", tagIDs)
	}

	var rows []struct {
		ID          uint      `gorm:"column:id"`
		Label       string    `gorm:"column:label"`
		Description string    `gorm:"column:description"`
		CreatedAt   time.Time `gorm:"column:created_at"`
	}
	if err := query.Order("tt.id ASC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load board event tags: %w", err)
	}
	tags := make([]SourceTag, 0, len(rows))
	for _, r := range rows {
		tags = append(tags, SourceTag{ID: r.ID, Label: r.Label, Description: r.Description, CreatedAt: r.CreatedAt})
	}
	if err := attachAuxLabels(ctx, db, tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// attachAuxLabels fills AuxLabels with the labels of the tag's CURRENT
// auxiliary links (label_type='auxiliary', status='active'). A disabled
// auxiliary label keeps its row but is semantically out of circulation, so it
// must not shape the representation.
func attachAuxLabels(ctx context.Context, db *gorm.DB, tags []SourceTag) error {
	if len(tags) == 0 {
		return nil
	}
	ids := make([]uint, len(tags))
	for i := range tags {
		ids[i] = tags[i].ID
	}
	type row struct {
		TopicTagID uint   `gorm:"column:topic_tag_id"`
		Label      string `gorm:"column:label"`
	}
	var rows []row
	err := db.WithContext(ctx).
		Table("topic_tag_semantic_labels AS ttsl").
		Select("ttsl.topic_tag_id, sl.label").
		Joins("JOIN semantic_labels AS sl ON sl.id = ttsl.semantic_label_id").
		Where("ttsl.topic_tag_id IN ?", ids).
		Where("sl.label_type = ? AND sl.status = ?", "auxiliary", "active").
		Order("ttsl.topic_tag_id ASC, sl.id ASC").
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("load auxiliary labels: %w", err)
	}
	byTag := make(map[uint][]string, len(tags))
	for _, r := range rows {
		byTag[r.TopicTagID] = append(byTag[r.TopicTagID], r.Label)
	}
	for i := range tags {
		tags[i].AuxLabels = byTag[tags[i].ID]
	}
	return nil
}

// cachedVector is the minimal read projection of a cache row. The embedding
// itself stays in SQL unless the caller needs it.
type cachedVector struct {
	TagID     uint   `gorm:"column:topic_tag_id"`
	TextHash  string `gorm:"column:text_hash"`
	Dimension int    `gorm:"column:dimension"`
}

// loadCachedVectors returns the cache rows of the given tags inside ONE
// representation version and ONE space. Rows from other versions/spaces are
// invisible by construction (they are keyed separately).
func loadCachedVectors(ctx context.Context, db *gorm.DB, tagIDs []uint, version, spaceKey string) (map[uint]cachedVector, error) {
	out := make(map[uint]cachedVector, len(tagIDs))
	if len(tagIDs) == 0 {
		return out, nil
	}
	var rows []cachedVector
	err := db.WithContext(ctx).
		Model(&tagmodels.RelatedEventEmbedding{}).
		Select("topic_tag_id, text_hash, dimension").
		Where("topic_tag_id IN ?", tagIDs).
		Where("representation_version = ? AND space_key = ?", version, spaceKey).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load cached vectors: %w", err)
	}
	for _, row := range rows {
		out[row.TagID] = row
	}
	return out, nil
}

// upsertCachedVector writes one freshly computed vector. Only successful,
// validated computations reach this function — a failure never overwrites a
// previously valid row.
func upsertCachedVector(ctx context.Context, db *gorm.DB, row *tagmodels.RelatedEventEmbedding) error {
	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "topic_tag_id"},
			{Name: "representation_version"},
			{Name: "space_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"text_hash", "dimension", "embedding", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return fmt.Errorf("upsert related event vector (tag %d): %w", row.TopicTagID, err)
	}
	return nil
}

// loadSourceArticles returns the newest readable article per tag: pub_date
// descending, id descending (design D7). Archived articles stay readable (the
// same policy as thread references), so `archived` is deliberately NOT
// filtered. A tag with no article simply has no entry — the UI reports "暂无
// 可读来源" instead of inventing an id.
func loadSourceArticles(ctx context.Context, db *gorm.DB, tagIDs []uint) (map[uint]SourceArticle, error) {
	out := make(map[uint]SourceArticle, len(tagIDs))
	if len(tagIDs) == 0 {
		return out, nil
	}
	type row struct {
		TopicTagID uint   `gorm:"column:topic_tag_id"`
		ArticleID  uint   `gorm:"column:article_id"`
		Title      string `gorm:"column:title"`
	}
	var rows []row
	err := db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (att.topic_tag_id)
		       att.topic_tag_id AS topic_tag_id,
		       a.id AS article_id,
		       a.title AS title
		FROM article_topic_tags att
		JOIN articles a ON a.id = att.article_id
		WHERE att.topic_tag_id IN ?
		ORDER BY att.topic_tag_id ASC, a.pub_date DESC NULLS LAST, a.id DESC`, tagIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load source articles: %w", err)
	}
	for _, r := range rows {
		out[r.TopicTagID] = SourceArticle{ID: r.ArticleID, Title: r.Title}
	}
	return out, nil
}

// SourceArticle is the readable first source of a related tag.
type SourceArticle struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
}
