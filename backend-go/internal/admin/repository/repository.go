package repository

import (
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/scheduler"
)

// ============================================================================
// Package-level wiring
// ============================================================================

var Repo *AdminRepository

func InitRepository(db *gorm.DB) {
	Repo = NewAdminRepository(db)
}

// ============================================================================
// AdminRepository — centralized data access for the admin feature module
// ============================================================================

type AdminRepository struct {
	db *gorm.DB
}

func NewAdminRepository(db *gorm.DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// DB returns the underlying gorm.DB for complex ad-hoc queries.
func (r *AdminRepository) DB() *gorm.DB {
	return r.db
}

// ============================================================================
// AI Provider operations
// ============================================================================

func (r *AdminRepository) ListProviders() ([]models.AIProvider, error) {
	var providers []models.AIProvider
	err := r.db.Order("name ASC").Find(&providers).Error
	return providers, err
}

func (r *AdminRepository) GetProviderByID(id uint) (*models.AIProvider, error) {
	var provider models.AIProvider
	if err := r.db.First(&provider, id).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

func (r *AdminRepository) UpsertProvider(p *models.AIProvider) error {
	return r.db.Save(p).Error
}

func (r *AdminRepository) DeleteProvider(id uint) error {
	return r.db.Delete(&models.AIProvider{}, id).Error
}

// ============================================================================
// AI Route operations
// ============================================================================

func (r *AdminRepository) ListRoutes() ([]models.AIRoute, error) {
	var routes []models.AIRoute
	err := r.db.Preload("RouteProviders.Provider").
		Preload("RouteProviders", func(db *gorm.DB) *gorm.DB {
			return db.Order("priority ASC")
		}).
		Order("capability ASC, name ASC").
		Find(&routes).Error
	return routes, err
}

func (r *AdminRepository) GetRouteByID(id uint) (*models.AIRoute, error) {
	var route models.AIRoute
	if err := r.db.Preload("RouteProviders.Provider").First(&route, id).Error; err != nil {
		return nil, err
	}
	return &route, nil
}

func (r *AdminRepository) UpsertRoute(rt *models.AIRoute) error {
	return r.db.Save(rt).Error
}

func (r *AdminRepository) DeleteRoute(id uint) error {
	return r.db.Delete(&models.AIRoute{}, id).Error
}

// ============================================================================
// AI Settings operations
// ============================================================================

func (r *AdminRepository) GetAISettings(key string) (*models.AISettings, error) {
	var setting models.AISettings
	if err := r.db.Where("key = ?", key).First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

func (r *AdminRepository) GetAllAISettings() (map[string]string, error) {
	var settings []models.AISettings
	if err := r.db.Order("key ASC").Find(&settings).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

func (r *AdminRepository) SaveAISettings(settings []models.AISettings) error {
	for _, s := range settings {
		var existing models.AISettings
		err := r.db.Where("key = ?", s.Key).First(&existing).Error
		switch err {
		case nil:
			existing.Value = s.Value
			if s.Description != "" {
				existing.Description = s.Description
			}
			if saveErr := r.db.Save(&existing).Error; saveErr != nil {
				return saveErr
			}
		case gorm.ErrRecordNotFound:
			if createErr := r.db.Create(&models.AISettings{
				Key:         s.Key,
				Value:       s.Value,
				Description: s.Description,
			}).Error; createErr != nil {
				return createErr
			}
		default:
			return err
		}
	}
	return nil
}

// ============================================================================
// Reading Behavior operations
// ============================================================================

func (r *AdminRepository) TrackReadingBehavior(b *models.ReadingBehavior) error {
	return r.db.Create(b).Error
}

func (r *AdminRepository) TrackReadingBehaviors(behaviors []models.ReadingBehavior) error {
	return r.db.Create(&behaviors).Error
}

func (r *AdminRepository) GetReadingStats(days int) (*ReadingStatsResult, error) {
	var stats ReadingStatsResult

	r.db.Model(&models.ReadingBehavior{}).
		Distinct("article_id").
		Count(&stats.TotalArticles)

	if err := r.db.Model(&models.ReadingBehavior{}).
		Select("COALESCE(SUM(reading_time), 0)").
		Scan(&stats.TotalReadingTime).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&models.ReadingBehavior{}).
		Where("reading_time > 0").
		Select("AVG(reading_time)").
		Scan(&stats.AvgReadingTime).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&models.ReadingBehavior{}).
		Where("scroll_depth > 0").
		Select("AVG(scroll_depth)").
		Scan(&stats.AvgScrollDepth).Error; err != nil {
		return nil, err
	}

	var feedCounts []struct {
		FeedID uint
		Count  int64
	}
	if err := r.db.Model(&models.ReadingBehavior{}).
		Select("feed_id, COUNT(*) as count").
		Group("feed_id").
		Order("count DESC").
		Limit(1).
		Scan(&feedCounts).Error; err != nil {
		return nil, err
	}
	if len(feedCounts) > 0 {
		stats.MostActiveFeedID = &feedCounts[0].FeedID
	}

	var categoryCounts []struct {
		CategoryID *uint
		Count      int64
	}
	if err := r.db.Model(&models.ReadingBehavior{}).
		Select("category_id, COUNT(*) as count").
		Where("category_id IS NOT NULL").
		Group("category_id").
		Order("count DESC").
		Limit(1).
		Scan(&categoryCounts).Error; err != nil {
		return nil, err
	}
	if len(categoryCounts) > 0 {
		stats.MostActiveCategory = categoryCounts[0].CategoryID
	}

	return &stats, nil
}

type ReadingStatsResult struct {
	TotalArticles      int64   `json:"total_articles"`
	TotalReadingTime   int     `json:"total_reading_time"`
	AvgReadingTime     float64 `json:"avg_reading_time"`
	AvgScrollDepth     float64 `json:"avg_scroll_depth"`
	MostActiveFeedID   *uint   `json:"most_active_feed_id"`
	MostActiveCategory *uint   `json:"most_active_category"`
}

func (r *AdminRepository) GetFeedByID(id uint) (*models.Feed, error) {
	var feed models.Feed
	if err := r.db.First(&feed, id).Error; err != nil {
		return nil, err
	}
	return &feed, nil
}

// ============================================================================
// Scheduler Task operations
// ============================================================================

func (r *AdminRepository) ListSchedulerTasks() ([]scheduler.SchedulerTask, error) {
	var tasks []scheduler.SchedulerTask
	err := r.db.Order("name ASC").Find(&tasks).Error
	return tasks, err
}

func (r *AdminRepository) GetSchedulerTaskByName(name string) (*scheduler.SchedulerTask, error) {
	var task scheduler.SchedulerTask
	if err := r.db.Where("name = ?", name).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *AdminRepository) SaveSchedulerTask(task *scheduler.SchedulerTask) error {
	return r.db.Save(task).Error
}

func (r *AdminRepository) UpdateSchedulerTask(name string, updates map[string]interface{}) error {
	var task scheduler.SchedulerTask
	if err := r.db.Where("name = ?", name).First(&task).Error; err != nil {
		return err
	}
	return r.db.Model(&task).Updates(updates).Error
}
