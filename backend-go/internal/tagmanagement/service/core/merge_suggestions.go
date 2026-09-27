package core

import (
	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"

	"gorm.io/gorm/clause"
)

func RecordMergeSuggestions(newTagID uint, newLabel string, category string, candidates []TagCandidate) {
	if len(candidates) == 0 {
		return
	}

	for _, c := range candidates {
		// Normalize direction: smaller ID always as new_tag_id
		var nID, eID uint
		var nLbl, eLbl string
		if newTagID < c.Tag.ID {
			nID, eID = newTagID, c.Tag.ID
			nLbl, eLbl = newLabel, c.Tag.Label
		} else {
			nID, eID = c.Tag.ID, newTagID
			nLbl, eLbl = c.Tag.Label, newLabel
		}

		suggestion := tagmodels.TagMergeSuggestion{
			NewTagID:      nID,
			ExistingTagID: eID,
			NewLabel:      nLbl,
			ExistingLabel: eLbl,
			Category:      category,
			Similarity:    c.Similarity,
			Status:        "pending",
			Source:        "incremental",
		}

		result := repository.Repo.DB().Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "new_tag_id"}, {Name: "existing_tag_id"}},
			DoNothing: true,
		}).Create(&suggestion)

		if result.Error != nil {
			logging.Warnf("RecordMergeSuggestions: failed to write suggestion new=%d existing=%d: %v", newTagID, c.Tag.ID, result.Error)
		} else if result.RowsAffected == 0 {
			logging.Infof("RecordMergeSuggestions: skipped duplicate new=%d existing=%d", newTagID, c.Tag.ID)
		}
	}
}
