package preferences

import (
	"math"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/logging"
)

type PreferenceService struct {
	db *gorm.DB
}

func NewPreferenceService(db *gorm.DB) *PreferenceService {
	return &PreferenceService{db: db}
}

func (s *PreferenceService) UpdateAllPreferences() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		txService := NewPreferenceService(tx)

		repairedBehaviors, err := txService.repairOrphanReadingBehaviors()
		if err != nil {
			return err
		}

		deletedBehaviors, err := txService.deleteUnrecoverableReadingBehaviors()
		if err != nil {
			return err
		}

		deletedPrefs, err := txService.deleteOrphanPreferences()
		if err != nil {
			return err
		}

		if err := tx.Exec("DELETE FROM user_preferences").Error; err != nil {
			return err
		}

		feedCount, err := txService.updateFeedPreferences()
		if err != nil {
			return err
		}

		categoryCount, err := txService.updateCategoryPreferences()
		if err != nil {
			return err
		}

		logging.Infof(
			"Preference update completed: repaired_behaviors=%d deleted_behaviors=%d deleted_orphan_preferences=%d rebuilt_feed_preferences=%d rebuilt_category_preferences=%d",
			repairedBehaviors,
			deletedBehaviors,
			deletedPrefs,
			feedCount,
			categoryCount,
		)

		return nil
	})
}

func (s *PreferenceService) repairOrphanReadingBehaviors() (int64, error) {
	result := s.db.Exec(`
		UPDATE reading_behaviors
		SET category_id = (
			SELECT feeds.category_id
			FROM feeds
			WHERE feeds.id = reading_behaviors.feed_id
		)
		WHERE category_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM categories
			WHERE categories.id = reading_behaviors.category_id
		  )
		  AND EXISTS (
			SELECT 1
			FROM feeds
			JOIN categories ON categories.id = feeds.category_id
			WHERE feeds.id = reading_behaviors.feed_id
		  )
	`)

	return result.RowsAffected, result.Error
}

func (s *PreferenceService) deleteUnrecoverableReadingBehaviors() (int64, error) {
	var totalDeleted int64

	categoryResult := s.db.Exec(`
		DELETE FROM reading_behaviors
		WHERE category_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM categories
			WHERE categories.id = reading_behaviors.category_id
		  )
	`)
	if categoryResult.Error != nil {
		return 0, categoryResult.Error
	}
	totalDeleted += categoryResult.RowsAffected

	feedResult := s.db.Exec(`
		DELETE FROM reading_behaviors
		WHERE NOT EXISTS (
			SELECT 1 FROM feeds
			WHERE feeds.id = reading_behaviors.feed_id
		)
	`)
	if feedResult.Error != nil {
		return totalDeleted, feedResult.Error
	}
	totalDeleted += feedResult.RowsAffected

	return totalDeleted, nil
}

func (s *PreferenceService) deleteOrphanPreferences() (int64, error) {
	var totalDeleted int64

	categoryResult := s.db.Exec(`
		DELETE FROM user_preferences
		WHERE category_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM categories
			WHERE categories.id = user_preferences.category_id
		  )
	`)
	if categoryResult.Error != nil {
		return 0, categoryResult.Error
	}
	totalDeleted += categoryResult.RowsAffected

	feedResult := s.db.Exec(`
		DELETE FROM user_preferences
		WHERE feed_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM feeds
			WHERE feeds.id = user_preferences.feed_id
		  )
	`)
	if feedResult.Error != nil {
		return 0, feedResult.Error
	}
	totalDeleted += feedResult.RowsAffected

	return totalDeleted, nil
}

func (s *PreferenceService) updateFeedPreferences() (int, error) {
	type FeedStats struct {
		FeedID           uint
		TotalEvents      int64
		TotalReadingTime int
		AvgScrollDepth   float64
		LastInteraction  string
	}

	var feedStats []FeedStats
	if err := s.db.Model(&models.ReadingBehavior{}).
		Select(`
			feed_id,
			COUNT(*) as total_events,
			COALESCE(SUM(reading_time), 0) as total_reading_time,
			COALESCE(AVG(scroll_depth), 0) as avg_scroll_depth,
			MAX(created_at) as last_interaction
		`).
		Where("feed_id IS NOT NULL AND feed_id > 0").
		Where("EXISTS (SELECT 1 FROM feeds WHERE feeds.id = reading_behaviors.feed_id)").
		Group("feed_id").
		Scan(&feedStats).Error; err != nil {
		return 0, err
	}

	for _, stats := range feedStats {
		lastInteraction, err := time.Parse(time.RFC3339, stats.LastInteraction)
		if err != nil {
			lastInteraction = time.Now()
		}

		preferenceScore := s.calculatePreferenceScore(
			stats.TotalEvents,
			stats.TotalReadingTime,
			stats.AvgScrollDepth,
			lastInteraction,
		)

		avgReadingTime := 0
		if stats.TotalEvents > 0 {
			avgReadingTime = stats.TotalReadingTime / int(stats.TotalEvents)
		}

		pref := models.UserPreference{
			FeedID:            &stats.FeedID,
			PreferenceScore:   preferenceScore,
			AvgReadingTime:    avgReadingTime,
			InteractionCount:  int(stats.TotalEvents),
			ScrollDepthAvg:    stats.AvgScrollDepth,
			LastInteractionAt: &lastInteraction,
		}
		if err := s.db.Create(&pref).Error; err != nil {
			return 0, err
		}
	}

	return len(feedStats), nil
}

func (s *PreferenceService) updateCategoryPreferences() (int, error) {
	type CategoryStats struct {
		CategoryID       *uint
		TotalEvents      int64
		TotalReadingTime int
		AvgScrollDepth   float64
		LastInteraction  string
	}

	var categoryStats []CategoryStats
	if err := s.db.Model(&models.ReadingBehavior{}).
		Select(`
			category_id,
			COUNT(*) as total_events,
			COALESCE(SUM(reading_time), 0) as total_reading_time,
			COALESCE(AVG(scroll_depth), 0) as avg_scroll_depth,
			MAX(created_at) as last_interaction
		`).
		Where("category_id IS NOT NULL").
		Group("category_id").
		Scan(&categoryStats).Error; err != nil {
		return 0, err
	}

	for _, stats := range categoryStats {
		if stats.CategoryID == nil {
			continue
		}

		lastInteraction, err := time.Parse(time.RFC3339, stats.LastInteraction)
		if err != nil {
			lastInteraction = time.Now()
		}

		preferenceScore := s.calculatePreferenceScore(
			stats.TotalEvents,
			stats.TotalReadingTime,
			stats.AvgScrollDepth,
			lastInteraction,
		)

		avgReadingTime := 0
		if stats.TotalEvents > 0 {
			avgReadingTime = stats.TotalReadingTime / int(stats.TotalEvents)
		}

		pref := models.UserPreference{
			CategoryID:        stats.CategoryID,
			PreferenceScore:   preferenceScore,
			AvgReadingTime:    avgReadingTime,
			InteractionCount:  int(stats.TotalEvents),
			ScrollDepthAvg:    stats.AvgScrollDepth,
			LastInteractionAt: &lastInteraction,
		}
		if err := s.db.Create(&pref).Error; err != nil {
			return 0, err
		}
	}

	return len(categoryStats), nil
}

func (s *PreferenceService) calculatePreferenceScore(
	totalEvents int64,
	totalReadingTime int,
	avgScrollDepth float64,
	lastInteraction time.Time,
) float64 {
	if totalEvents == 0 {
		return 0
	}

	scrollScore := avgScrollDepth / 100.0 * 0.4

	avgTime := float64(totalReadingTime) / float64(totalEvents)
	readingTimeScore := math.Min(avgTime/180.0, 1.0) * 0.3

	eventsScore := math.Min(float64(totalEvents)/50.0, 1.0) * 0.3

	baseScore := scrollScore + readingTimeScore + eventsScore

	daysSinceInteraction := time.Since(lastInteraction).Hours() / 24
	decayFactor := math.Exp(-daysSinceInteraction / 30.0)

	finalScore := baseScore * decayFactor

	return math.Max(0, math.Min(finalScore, 1.0))
}

func (s *PreferenceService) GetUserFeedPreferences() ([]models.UserPreference, error) {
	var preferences []models.UserPreference
	err := s.db.Joins("JOIN feeds ON feeds.id = user_preferences.feed_id").
		Where("user_preferences.feed_id IS NOT NULL").
		Preload("Feed").
		Order("preference_score DESC").
		Find(&preferences).Error
	return preferences, err
}

func (s *PreferenceService) GetUserCategoryPreferences() ([]models.UserPreference, error) {
	var preferences []models.UserPreference
	err := s.db.Joins("JOIN categories ON categories.id = user_preferences.category_id").
		Where("user_preferences.category_id IS NOT NULL").
		Preload("Category").
		Order("preference_score DESC").
		Find(&preferences).Error
	return preferences, err
}

func (s *PreferenceService) GetTopPreferredFeeds(limit int) ([]uint, error) {
	type FeedIDScore struct {
		FeedID uint
		Score  float64
	}

	var results []FeedIDScore
	err := s.db.Model(&models.UserPreference{}).
		Select("feed_id, MAX(preference_score) as score").
		Joins("JOIN feeds ON feeds.id = user_preferences.feed_id").
		Where("user_preferences.feed_id IS NOT NULL").
		Group("feed_id").
		Order("score DESC").
		Limit(limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	feedIDs := make([]uint, len(results))
	for i, r := range results {
		feedIDs[i] = r.FeedID
	}

	return feedIDs, nil
}

func (s *PreferenceService) GetTopPreferredCategories(limit int) ([]uint, error) {
	type CategoryIDScore struct {
		CategoryID *uint
		Score      float64
	}

	var results []CategoryIDScore
	err := s.db.Model(&models.UserPreference{}).
		Select("category_id, MAX(preference_score) as score").
		Joins("JOIN categories ON categories.id = user_preferences.category_id").
		Where("user_preferences.category_id IS NOT NULL").
		Group("category_id").
		Order("score DESC").
		Limit(limit).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	categoryIDs := make([]uint, 0, len(results))
	for _, r := range results {
		if r.CategoryID != nil {
			categoryIDs = append(categoryIDs, *r.CategoryID)
		}
	}

	return categoryIDs, nil
}
