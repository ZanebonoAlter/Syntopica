package main

import (
	"fmt"
	"os"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicanalysis"
	"my-robot-backend/internal/platform/config"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type cleanupStats struct {
	phaseAZeroArticle struct {
		tagsFound       int
		tagsDeactivated int
	}
	phaseBLowQuality struct {
		tagsFound       int
		tagsDeactivated int
	}
	preStats  map[string]int
	postStats map[string]int
}

func countByStatus() map[string]int {
	var rows []struct {
		Status string `gorm:"column:status"`
		Cnt    int    `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTag{}).
		Select("status, count(*) as cnt").
		Group("status").
		Scan(&rows)

	m := make(map[string]int)
	for _, r := range rows {
		m[r.Status] = r.Cnt
	}
	return m
}

func countByCategory() map[string]int {
	var rows []struct {
		Category string `gorm:"column:category"`
		Cnt      int    `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTag{}).
		Select("category, count(*) as cnt").
		Where("status = ?", "active").
		Group("category").
		Scan(&rows)

	m := make(map[string]int)
	for _, r := range rows {
		m[r.Category] = r.Cnt
	}
	return m
}

func countZeroArticleTags() (int, error) {
	var count int64
	err := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND kind != ? AND source != ?", "active", "abstract", "abstract").
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)").
		Count(&count).Error
	return int(count), err
}

func countLowQualityKeywordTags() (int, error) {
	var allActiveIDs []uint
	err := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND category = ?", "active", models.TagCategoryKeyword).
		Pluck("topic_tags.id", &allActiveIDs).Error
	if err != nil {
		return 0, err
	}
	if len(allActiveIDs) == 0 {
		return 0, nil
	}

	var articleCounts []struct {
		TopicTagID uint `gorm:"column:topic_tag_id"`
		Cnt        int  `gorm:"column:cnt"`
	}
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("topic_tag_id, count(*) as cnt").
		Where("topic_tag_id IN ?", allActiveIDs).
		Group("topic_tag_id").
		Scan(&articleCounts)

	countMap := make(map[uint]int, len(articleCounts))
	for _, ac := range articleCounts {
		countMap[ac.TopicTagID] = ac.Cnt
	}

	var lowQualityIDs []uint
	err = database.DB.Model(&models.TopicTag{}).
		Where("id IN ? AND quality_score < ?", allActiveIDs, 0.15).
		Pluck("topic_tags.id", &lowQualityIDs).Error
	if err != nil {
		return 0, err
	}

	var count int
	for _, id := range lowQualityIDs {
		if countMap[id] <= 1 {
			count++
		}
	}
	return count, nil
}

func printStats(label string, statusMap, catMap map[string]int) {
	logging.Infof("  %s — status: %v", label, statusMap)
	logging.Infof("  %s — active by category: %v", label, catMap)
}

func main() {
	dryRun := os.Getenv("DRY_RUN") != "false"

	logging.Infoln("=== Tag Cleanup Tool ===")
	if dryRun {
		logging.Infoln("Mode: DRY RUN (set DRY_RUN=false to apply changes)")
	} else {
		logging.Infoln("Mode: LIVE — changes will be committed!")
	}

	if err := config.LoadConfig("./configs"); err != nil {
		logging.Infof("Warning: Failed to load config: %v", err)
	}

	if err := database.InitDB(config.AppConfig); err != nil {
		logging.Fatalf("Failed to initialize database: %v", err)
	}

	stats := &cleanupStats{}
	stats.preStats = countByStatus()
	preCatStats := countByCategory()
	printStats("Before", stats.preStats, preCatStats)

	fmt.Println()

	allCategories := []string{models.TagCategoryEvent, models.TagCategoryPerson, models.TagCategoryKeyword}

	if dryRun {
		logging.Infoln("--- Phase A: Zero-article tags (dry-run) ---")
		aFound, err := countZeroArticleTags()
		if err != nil {
			logging.Fatalf("Phase A count failed: %v", err)
		}
		stats.phaseAZeroArticle.tagsFound = aFound
		logging.Infof("[Phase A] Would deactivate %d zero-article tags", aFound)

		fmt.Println()
		logging.Infoln("--- Phase B: Low-quality keyword tags (dry-run) ---")
		bFound, err := countLowQualityKeywordTags()
		if err != nil {
			logging.Fatalf("Phase B count failed: %v", err)
		}
		stats.phaseBLowQuality.tagsFound = bFound
		logging.Infof("[Phase B] Would deactivate %d low-quality keyword tags", bFound)
	} else {
		logging.Infoln("--- Phase A: Zero-article tags (deactivate) ---")
		aDeact, err := topicanalysis.CleanupZeroArticleTags(allCategories)
		if err != nil {
			logging.Fatalf("Phase A failed: %v", err)
		}
		stats.phaseAZeroArticle.tagsFound = aDeact
		stats.phaseAZeroArticle.tagsDeactivated = aDeact

		fmt.Println()
		logging.Infoln("--- Phase B: Low-quality keyword tags (quality_score < 0.15, articles <= 1) ---")
		bDeact, err := topicanalysis.CleanupLowQualitySingleArticleTags(models.TagCategoryKeyword, 0.15)
		if err != nil {
			logging.Fatalf("Phase B failed: %v", err)
		}
		stats.phaseBLowQuality.tagsFound = bDeact
		stats.phaseBLowQuality.tagsDeactivated = bDeact
	}

	fmt.Println()
	stats.postStats = countByStatus()
	postCatStats := countByCategory()
	printStats("After", stats.postStats, postCatStats)

	fmt.Println()
	logging.Infoln("=== Cleanup Summary ===")
	logging.Infof("Phase A (zero-article): %d tags %s",
		stats.phaseAZeroArticle.tagsFound, dryRunVerb(dryRun))
	logging.Infof("Phase B (low-quality keyword): %d tags %s",
		stats.phaseBLowQuality.tagsFound, dryRunVerb(dryRun))

	if dryRun {
		logging.Infoln("\n*** DRY RUN — no changes were committed ***")
	}

	logging.Infoln("Done.")
}

func dryRunVerb(dryRun bool) string {
	if dryRun {
		return "would be deactivated"
	}
	return "deactivated"
}
