package articlerefs

import (
	"fmt"

	"gorm.io/gorm"
)

// RewireArticleRefs points every daily-report thread reference from oldID at
// keeperID. It is the maintenance step for a delete path that has a surviving
// equivalent row — today the (feed_id, link) duplicate merge, where the loser
// copy's content is exactly the keeper's.
//
// Semantics (spec "文章行删除时必须同步维护 jsonb 引用数组"): order preserved,
// the keeper appears once (first occurrence wins), empty arrays stay [].
// Returns the number of updated rows; 0 means nothing referenced oldID and no
// UPDATE was issued (idempotent).
func RewireArticleRefs(db *gorm.DB, oldID, keeperID uint) (int64, error) {
	if db == nil {
		return 0, errNilDB
	}
	// Without a survivor there is nothing to point at; an identical pair is a
	// no-op by definition. Callers use PruneArticleRefs for the survivor-less
	// case.
	if oldID == 0 || keeperID == 0 || oldID == keeperID {
		return 0, nil
	}
	rows, err := loadRefRowsContaining(db, idStrings([]uint{oldID}))
	if err != nil {
		return 0, fmt.Errorf("rewire article %d to %d: %w", oldID, keeperID, err)
	}
	return rewriteRows(db, rows, func(ids []uint) ([]uint, bool) {
		return rewriteRefIDs(ids, func(id uint) (uint, bool) {
			if id == oldID {
				return keeperID, true
			}
			return id, true
		})
	})
}

// PruneArticleRefs drops ids from every daily-report thread reference array. It
// is the maintenance step for a delete path with no surviving equivalent row:
// once the article is gone there is no target to point at, and a dangling ID is
// worse than a shorter list (the reader degrades to "文章 #<id>" otherwise).
// Empty/nil ids is a no-op, as is an id set nobody references.
func PruneArticleRefs(db *gorm.DB, ids []uint) (int64, error) {
	if db == nil {
		return 0, errNilDB
	}
	texts := idStrings(ids)
	if len(texts) == 0 {
		return 0, nil
	}
	rows, err := loadRefRowsContaining(db, texts)
	if err != nil {
		return 0, fmt.Errorf("prune article refs: %w", err)
	}
	drop := make(map[uint]bool, len(texts))
	for _, id := range ids {
		if id != 0 {
			drop[id] = true
		}
	}
	return rewriteRows(db, rows, func(current []uint) ([]uint, bool) {
		return rewriteRefIDs(current, func(id uint) (uint, bool) {
			if drop[id] {
				return 0, false
			}
			return id, true
		})
	})
}
