package main

import (
	"flag"
	"fmt"
	"sort"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

const maxEdges = 4

func main() {
	dryRun := flag.Bool("dry-run", true, "只显示将要删除的关系，不实际执行")
	flag.Parse()

	if err := config.LoadConfig("./configs"); err != nil {
		logging.Warnf("Failed to load config: %v", err)
	}
	cfg := config.AppConfig

	if err := database.InitDB(cfg); err != nil {
		logging.Fatalf("Failed to initialize database: %v", err)
	}
	db := database.DB

	var allRelations []models.TopicTagRelation
	db.Where("relation_type = ?", "abstract").Find(&allRelations)

	if len(allRelations) == 0 {
		fmt.Println("没有抽象关系")
		return
	}

	childToParents := make(map[uint][]uint)
	hasParent := make(map[uint]bool)

	for _, r := range allRelations {
		childToParents[r.ChildID] = append(childToParents[r.ChildID], r.ParentID)
		hasParent[r.ChildID] = true
	}

	overLimitTags := make(map[uint]int)

	var dfsUp func(tagID uint, depth int, visited map[uint]bool) int
	dfsUp = func(tagID uint, depth int, visited map[uint]bool) int {
		if depth > maxEdges {
			if _, exists := overLimitTags[tagID]; !exists {
				overLimitTags[tagID] = depth
			}
		}

		parents := childToParents[tagID]
		maxDepth := depth
		for _, parentID := range parents {
			if visited[parentID] {
				continue
			}
			visited[parentID] = true
			d := dfsUp(parentID, depth+1, visited)
			if d > maxDepth {
				maxDepth = d
			}
			delete(visited, parentID)
		}
		return maxDepth
	}

	for tagID := range hasParent {
		visited := map[uint]bool{tagID: true}
		dfsUp(tagID, 0, visited)
	}

	var overLimitIDs []uint
	for id := range overLimitTags {
		overLimitIDs = append(overLimitIDs, id)
	}
	sort.Slice(overLimitIDs, func(i, j int) bool { return overLimitIDs[i] < overLimitIDs[j] })

	if len(overLimitIDs) == 0 {
		fmt.Println("没有超过深度限制的标签")
		return
	}

	fmt.Printf("发现 %d 个标签超过 %d 层边深度限制\n", len(overLimitIDs), maxEdges)

	var relationsToDelete []models.TopicTagRelation
	relSeen := make(map[string]bool)

	for _, tagID := range overLimitIDs {
		for _, parentID := range childToParents[tagID] {
			key := fmt.Sprintf("%d-%d", parentID, tagID)
			if relSeen[key] {
				continue
			}
			relSeen[key] = true
			var rel models.TopicTagRelation
			if err := db.Where("parent_id = ? AND child_id = ? AND relation_type = ?",
				parentID, tagID, "abstract").First(&rel).Error; err == nil {
				relationsToDelete = append(relationsToDelete, rel)
			}
		}
	}

	if *dryRun {
		fmt.Println("\n=== DRY RUN 模式 (使用 --dry-run=false 执行实际删除) ===")
		for _, r := range relationsToDelete {
			var parent, child models.TopicTag
			db.First(&parent, r.ParentID)
			db.First(&child, r.ChildID)
			fmt.Printf("  删除: %s (%d) <- %s (%d) [maxDepth=%d]\n",
				parent.Label, r.ParentID, child.Label, r.ChildID, overLimitTags[r.ChildID])
		}
		fmt.Printf("\n共 %d 条关系将被删除\n", len(relationsToDelete))
	} else {
		deleted := 0
		for _, r := range relationsToDelete {
			result := db.Where("parent_id = ? AND child_id = ? AND relation_type = ?",
				r.ParentID, r.ChildID, "abstract").Delete(&models.TopicTagRelation{})
			if result.Error != nil {
				fmt.Printf("  删除失败: parent=%d child=%d: %v\n", r.ParentID, r.ChildID, result.Error)
			} else {
				deleted++
			}
		}
		fmt.Printf("\n已删除 %d 条关系\n", deleted)
	}
}
