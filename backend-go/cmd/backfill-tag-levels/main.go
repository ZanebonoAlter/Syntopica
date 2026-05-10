package main

import (
	"flag"
	"fmt"
	"os"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicanalysis"
	"my-robot-backend/internal/platform/config"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Preview changes without modifying the database")
	categoryFilter := flag.String("category", "", "Only process tags of a specific category (event, person, keyword)")
	flag.Parse()

	if err := config.LoadConfig("./configs"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := database.InitDB(config.AppConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	mgr := topicanalysis.GetHierarchyManager()
	mgr.LoadSystemDefaults()

	logging.Infof("Starting tag level backfill...")
	if *dryRun {
		logging.Infof("DRY RUN mode - no changes will be made")
	}

	backfilled := backfillLevelsByDepth(*categoryFilter, *dryRun)
	logging.Infof("Backfilled %d tags with levels", backfilled)

	invalidCount, invalidRelations := findInvalidRelations(*categoryFilter, *dryRun)
	logging.Infof("Found %d invalid relations", invalidCount)

	if invalidCount > 0 && !*dryRun {
		repaired := repairInvalidRelations(invalidRelations, false)
		logging.Infof("Repaired %d invalid relations", repaired)
	} else if invalidCount > 0 {
		logging.Infof("Would repair %d invalid relations (dry-run)", invalidCount)
	}

	logging.Infof("Tag level backfill complete.")
}

type invalidRelationInfo struct {
	RelationID uint
	ParentID   uint
	ChildID    uint
	Issue      string
}

func backfillLevelsByDepth(categoryFilter string, dryRun bool) int {
	var tags []models.TopicTag
	query := database.DB.Where("status = 'active'")
	if categoryFilter != "" {
		query = query.Where("category = ?", categoryFilter)
	}
	if err := query.Find(&tags).Error; err != nil {
		logging.Errorf("Failed to load tags: %v", err)
		return 0
	}

	count := 0
	for _, tag := range tags {
		depth := topicanalysis.GetTagLevelByID(tag.ID, tag.Category)

		if !dryRun {
			if tag.Metadata == nil {
				tag.Metadata = models.MetadataMap{}
			}
			tag.Metadata["hierarchy_level"] = depth
			if err := database.DB.Save(&tag).Error; err != nil {
				logging.Warnf("Failed to save level for tag %d: %v", tag.ID, err)
				continue
			}
		}
		count++
	}

	return count
}

func findInvalidRelations(categoryFilter string, dryRun bool) (int, []invalidRelationInfo) {
	var relations []models.TopicTagRelation
	query := database.DB.Where("relation_type = 'abstract'").Preload("Parent").Preload("Child")
	if err := query.Find(&relations).Error; err != nil {
		logging.Errorf("Failed to load relations: %v", err)
		return 0, nil
	}

	var invalid []invalidRelationInfo
	for _, r := range relations {
		if r.Parent == nil || r.Child == nil {
			continue
		}
		if categoryFilter != "" && r.Parent.Category != categoryFilter && r.Child.Category != categoryFilter {
			continue
		}

		if r.Parent.Category != r.Child.Category {
			invalid = append(invalid, invalidRelationInfo{
				RelationID: r.ID, ParentID: r.ParentID, ChildID: r.ChildID,
				Issue: "cross_category",
			})
			logging.Infof("Cross-category relation: parent=%d(%s,%s) child=%d(%s,%s)",
				r.ParentID, r.Parent.Label, r.Parent.Category, r.ChildID, r.Child.Label, r.Child.Category)
		}

		tmpl := topicanalysis.GetHierarchyManager().GetTemplate(r.Parent.Category, "")
		if tmpl != nil {
			childDepth := topicanalysis.GetTagLevelByID(r.ChildID, r.Child.Category)
			if childDepth > tmpl.MaxLevel {
				invalid = append(invalid, invalidRelationInfo{
					RelationID: r.ID, ParentID: r.ParentID, ChildID: r.ChildID,
					Issue: "depth_exceeded",
				})
				logging.Infof("Depth exceeded: parent=%d child=%d depth=%d max=%d",
					r.ParentID, r.ChildID, childDepth, tmpl.MaxLevel)
			}
		}
	}

	return len(invalid), invalid
}

func repairInvalidRelations(invalid []invalidRelationInfo, dryRun bool) int {
	if dryRun {
		return 0
	}

	repaired := 0
	for _, info := range invalid {
		result := database.DB.Where("parent_id = ? AND child_id = ? AND relation_type = 'abstract'",
			info.ParentID, info.ChildID).Delete(&models.TopicTagRelation{})
		if result.Error != nil {
			logging.Warnf("Failed to delete invalid relation %d: %v", info.RelationID, result.Error)
			continue
		}
		if result.RowsAffected > 0 {
			logging.Infof("Removed invalid relation: %d -> %d (issue: %s)", info.ParentID, info.ChildID, info.Issue)
			repaired++
		}
	}

	return repaired
}
