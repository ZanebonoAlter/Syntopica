package watched

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
)

// WatchedTagInfo extends TopicTag with abstract-tag metadata for the API response.
type WatchedTagInfo struct {
	ID         uint       `json:"id"`
	Slug       string     `json:"slug"`
	Label      string     `json:"label"`
	Category   string     `json:"category"`
	Icon       string     `json:"icon"`
	WatchedAt  *time.Time `json:"watched_at"`
	IsAbstract bool       `json:"is_abstract"`
	ChildSlugs []string   `json:"child_slugs"`
}

// WatchTag marks a tag as watched. Validates that the tag exists and is active.
func WatchTag(db *gorm.DB, tagID uint) (*models.TopicTag, error) {
	var tag models.TopicTag
	if err := db.First(&tag, tagID).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	tag.IsWatched = true
	tag.WatchedAt = &now
	if err := db.Save(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// UnwatchTag removes the watched status from a tag.
func UnwatchTag(db *gorm.DB, tagID uint) (*models.TopicTag, error) {
	var tag models.TopicTag
	if err := db.First(&tag, tagID).Error; err != nil {
		return nil, err
	}

	tag.IsWatched = false
	tag.WatchedAt = nil
	if err := db.Save(&tag).Error; err != nil {
		return nil, err
	}
	return &tag, nil
}

// ListWatchedTags returns all watched tags with abstract-tag metadata.
func ListWatchedTags(db *gorm.DB) ([]WatchedTagInfo, error) {
	var tags []models.TopicTag
	if err := db.Where("is_watched = ? AND status = ?", true, "active").
		Order("watched_at DESC").
		Find(&tags).Error; err != nil {
		return nil, err
	}

	// Collect all watched tag IDs to check for abstract relationships
	watchedIDs := make([]uint, len(tags))
	for i, t := range tags {
		watchedIDs[i] = t.ID
	}

	// Find which watched tags are abstract (have children in topic_tag_relations)
	abstractMap := make(map[uint]bool)
	childSlugMap := make(map[uint][]string)
	if len(watchedIDs) > 0 {
		var relations []models.TopicTagRelation
		if err := db.Where("parent_id IN ?", watchedIDs).Find(&relations).Error; err != nil {
			return nil, fmt.Errorf("query tag relations: %w", err)
		}

		// Collect unique child IDs
		childIDSet := make(map[uint]bool)
		parentToChildren := make(map[uint][]uint)
		for _, rel := range relations {
			abstractMap[rel.ParentID] = true
			childIDSet[rel.ChildID] = true
			parentToChildren[rel.ParentID] = append(parentToChildren[rel.ParentID], rel.ChildID)
		}

		// Batch load child tag slugs
		childIDs := make([]uint, 0, len(childIDSet))
		for id := range childIDSet {
			childIDs = append(childIDs, id)
		}
		if len(childIDs) > 0 {
			var childTags []models.TopicTag
			if err := db.Where("id IN ?", childIDs).Select("id, slug").Find(&childTags).Error; err != nil {
				return nil, fmt.Errorf("query child tag slugs: %w", err)
			}
			slugMap := make(map[uint]string)
			for _, ct := range childTags {
				slugMap[ct.ID] = ct.Slug
			}
			for parentID, children := range parentToChildren {
				slugs := make([]string, 0, len(children))
				for _, cid := range children {
					if s, ok := slugMap[cid]; ok {
						slugs = append(slugs, s)
					}
				}
				childSlugMap[parentID] = slugs
			}
		}
	}

	result := make([]WatchedTagInfo, len(tags))
	for i, t := range tags {
		result[i] = WatchedTagInfo{
			ID:         t.ID,
			Slug:       t.Slug,
			Label:      t.Label,
			Category:   t.Category,
			Icon:       t.Icon,
			WatchedAt:  t.WatchedAt,
			IsAbstract: abstractMap[t.ID],
			ChildSlugs: childSlugMap[t.ID],
		}
		// Ensure non-nil child_slugs for consistent JSON
		if result[i].ChildSlugs == nil {
			result[i].ChildSlugs = []string{}
		}
	}

	return result, nil
}

// GetWatchedTagIDsExpanded returns all watched tag IDs plus recursively expanded child tag IDs.
func GetWatchedTagIDsExpanded(db *gorm.DB) ([]uint, []uint, error) {
	var watchedTags []models.TopicTag
	if err := db.Where("is_watched = ? AND status = ?", true, "active").
		Select("id").
		Find(&watchedTags).Error; err != nil {
		return nil, nil, err
	}

	if len(watchedTags) == 0 {
		return nil, nil, nil
	}

	watchedIDs := make([]uint, len(watchedTags))
	for i, t := range watchedTags {
		watchedIDs[i] = t.ID
	}

	allChildIDs := make([]uint, 0)
	visited := make(map[uint]bool)
	queue := make([]uint, len(watchedIDs))
	copy(queue, watchedIDs)

	for len(queue) > 0 {
		var relations []models.TopicTagRelation
		if err := db.Where("parent_id IN ?", queue).Find(&relations).Error; err != nil {
			return nil, nil, fmt.Errorf("query tag relations for expansion: %w", err)
		}

		queue = queue[:0]
		for _, rel := range relations {
			if !visited[rel.ChildID] {
				visited[rel.ChildID] = true
				allChildIDs = append(allChildIDs, rel.ChildID)
				queue = append(queue, rel.ChildID)
			}
		}
	}

	return watchedIDs, allChildIDs, nil
}
