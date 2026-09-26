package repository

import (
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

var Repo *TagManagementRepository

func InitRepository(db *gorm.DB) {
	Repo = NewTagManagementRepository(db)
}

type TagManagementRepository struct {
	db *gorm.DB
}

func NewTagManagementRepository(db *gorm.DB) *TagManagementRepository {
	return &TagManagementRepository{db: db}
}

func (r *TagManagementRepository) DB() *gorm.DB {
	return r.db
}

// ---- TopicTag CRUD ----

func (r *TagManagementRepository) ListActiveTags(limit, offset int) ([]models.TopicTag, error) {
	var tags []models.TopicTag
	err := r.db.Where("status = ? OR status = '' OR status IS NULL", "active").
		Order("feed_count DESC, id DESC").Limit(limit).Offset(offset).Find(&tags).Error
	return tags, err
}

func (r *TagManagementRepository) SearchTags(query, category string, limit int) ([]models.TopicTag, error) {
	var tags []models.TopicTag
	q := r.db.Where("(status = 'active' OR status = '' OR status IS NULL)").
		Where("label ILIKE ?", "%"+query+"%").
		Order("feed_count DESC, id DESC").Limit(limit)
	if category != "" {
		q = q.Where("category = ?", category)
	}
	err := q.Find(&tags).Error
	return tags, err
}

func (r *TagManagementRepository) GetTagByID(id uint) (*models.TopicTag, error) {
	var tag models.TopicTag
	err := r.db.First(&tag, id).Error
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

func (r *TagManagementRepository) CreateTag(tag *models.TopicTag) error {
	return r.db.Create(tag).Error
}

func (r *TagManagementRepository) SaveTag(tag *models.TopicTag) error {
	return r.db.Save(tag).Error
}

func (r *TagManagementRepository) DeleteTag(id uint) error {
	return r.db.Delete(&models.TopicTag{}, id).Error
}

func (r *TagManagementRepository) GetTagArticleCount(tagID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", tagID).Count(&count).Error
	return count, err
}

// ---- TagMergeSuggestion ----

func (r *TagManagementRepository) ListPendingSuggestions(limit int) ([]tagmodels.TagMergeSuggestion, error) {
	var suggestions []tagmodels.TagMergeSuggestion
	err := r.db.Where("status = ?", "pending").Order("similarity DESC").Limit(limit).Find(&suggestions).Error
	return suggestions, err
}

func (r *TagManagementRepository) CreateSuggestion(s *tagmodels.TagMergeSuggestion) error {
	return r.db.Create(s).Error
}

func (r *TagManagementRepository) DismissSuggestion(newID, existingID uint) error {
	return r.db.Model(&tagmodels.TagMergeSuggestion{}).
		Where("new_tag_id = ? AND existing_tag_id = ? AND status = ?", newID, existingID, "pending").
		Update("status", "dismissed").Error
}

func (r *TagManagementRepository) MarkSuggestionsMerged(sourceIDs []uint, targetID uint) error {
	return r.db.Model(&tagmodels.TagMergeSuggestion{}).
		Where("status = ? AND (new_tag_id IN ? OR existing_tag_id IN ? OR new_tag_id = ? OR existing_tag_id = ?)",
			"pending", sourceIDs, sourceIDs, targetID, targetID).
		Update("status", "merged").Error
}

// ---- ArticleTopicTag ----

func (r *TagManagementRepository) CountArticlesByTag(tagID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", tagID).Count(&count).Error
	return count, err
}

func (r *TagManagementRepository) GetTagsForArticle(articleID uint) ([]models.TopicTag, error) {
	var tags []models.TopicTag
	err := r.db.Joins("JOIN article_topic_tags att ON att.topic_tag_id = topic_tags.id").
		Where("att.article_id = ?", articleID).Find(&tags).Error
	return tags, err
}

// ---- TagJobQueue delegate ----

func (r *TagManagementRepository) NewTagJobQueue() *TagJobQueue {
	return NewTagJobQueue(r.db)
}
