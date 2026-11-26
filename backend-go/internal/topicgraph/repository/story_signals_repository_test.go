package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorySignalsArchivedArticlesRemainEvidence(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "archived-evidence", "", "", at)
	article := seedSignalArticle(t, db, 30, at, nil, true)
	report := seedStoryReport(t, db, 1, at)
	section := seedStorySection(t, db, report, nil)
	thread := seedStoryThread(t, db, report, section, nil, []uint{article}, false)
	require.NoError(t, repo.AttachThreadToBoardStory(report, thread, story, false))
	var signals []StorySignal
	require.NoError(t, db.Where("story_id = ?", story).Find(&signals).Error)
	assert.Len(t, signals, 1, "feed-window rotation does not retract reporting evidence")
	assertStoryParticipants(t, db, story, 1)
	// Isolate participant counting from signal construction: preexisting evidence
	// must also count when its article has rotated out of the active feed window.
	require.NoError(t, db.Save(&StorySignal{StoryID: story, ArticleID: article, FeedID: 30, ParticipantKey: "feed:30", Kind: StorySignalKindEditorial, ObservedAt: at, ObservedAtSource: "fallback"}).Error)
	require.NoError(t, repo.RecomputeBoardStoryCounts(story))
	assertStoryParticipants(t, db, story, 1)
}
