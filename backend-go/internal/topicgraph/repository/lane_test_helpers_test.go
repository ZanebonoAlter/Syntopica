package repository

// Shared test helpers for lane-dynamics / lane-snapshot integration tests.
// Extracted from the retired topic-landscape test file (overview-lane-dynamics
// tasks 4.x: the view is gone, the seeding helpers live on).

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// seedLandscapeTopic creates a persistent topic with the given lifecycle fields
// and a 1-dim embedding (test-DB vector column accepts it).
func seedLandscapeTopic(t *testing.T, db *gorm.DB, boardID uint, label, status string, hitCount, cons int, lastSeen time.Time) BoardPersistentTopic {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          status,
		Source:          TopicSourceAuto,
		FirstSeenDate:   NormalizeReportDate(lastSeen),
		LastSeenDate:    NormalizeReportDate(lastSeen),
		HitCount:        hitCount,
		ConsecutiveHits: cons,
	}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("seed persistent topic: %v", err)
	}
	return topic
}

// assignSection links a section to a topic (mirrors the assignment write that
// production performs inside SaveReport).
func assignSection(t *testing.T, db *gorm.DB, sectionID, topicID uint) {
	t.Helper()
	if err := db.Model(&DailyReportSection{}).
		Where("id = ?", sectionID).
		Update("persistent_topic_id", topicID).Error; err != nil {
		t.Fatalf("assign section %d to topic %d: %v", sectionID, topicID, err)
	}
}
