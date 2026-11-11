package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// Uses only a throwaway PostgreSQL and fake external calls; no service spawns.
func TestDailyReportStoryHighlightsRebuildAndFallback(t *testing.T) {
	db := testutil.SetupTestDB(t)
	oldRepo, oldAttribute, oldChat, oldTimeout := repository.Repo, attributeReportOnline, highlightsChatFn, reportAttributionTimeout
	repository.Repo = repository.NewTopicGraphRepository(db)
	defer func() {
		repository.Repo = oldRepo
		attributeReportOnline = oldAttribute
		highlightsChatFn = oldChat
		reportAttributionTimeout = oldTimeout
	}()
	board := seedBoard(t, db)
	day := repository.NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	story := repository.BoardStory{BoardID: board, Title: "story", FirstSeenDate: day, LastSeenDate: day}
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&story).Error)
	inputs := &reportHighlightInputs{tags: []repository.TagInput{{ID: 123, Label: "a documented fact", Description: "reported action"}}, clusters: []repository.ClusterGroup{{GroupName: "facts", TagIDs: []uint{123}}}}
	mode := "success"
	calls := 0
	attributeReportOnline = func(ctx context.Context, boardID, reportID uint, _ time.Time) error {
		switch mode {
		case "failure":
			return errors.New("attribution unavailable")
		case "timeout":
			<-ctx.Done()
			return ctx.Err()
		case "empty":
			return nil
		}
		return db.WithContext(ctx).Model(&repository.DailyReportThread{}).Where("report_id = ?", reportID).Update("story_id", story.ID).Error
	}
	highlightsChatFn = func(_ context.Context, _, _ string, schema *airouter.JSONSchema, _ string) (string, error) {
		calls++
		if _, storyMode := schema.Properties["highlights"].Items.Properties["story_id"]; storyMode {
			require.Equal(t, "success", mode)
			return fmt.Sprintf(`{"highlights":[{"story_id":%d,"title":"fact title","reason":"fact reason","tag_ids":[123]}]}`, story.ID), nil
		}
		require.NotEqual(t, "success", mode)
		return `{"highlights":[{"title":"tag title","reason":"tag reason","tag_ids":[123]}]}`, nil
	}
	var previous repository.JSON
	var reportID uint
	for _, scenario := range []string{"success", "success", "failure", "timeout", "empty"} {
		mode = scenario
		calls = 0
		reportAttributionTimeout = reportAttributionBudget
		if mode == "timeout" {
			reportAttributionTimeout = 5 * time.Millisecond
		}
		report := &repository.BoardDailyReport{SemanticBoardID: board, PeriodDate: day, Title: "temporary", Highlights: []byte("[]"), Status: "completed"}
		sections := []repository.DailyReportSection{{ClusterLabel: "facts"}}
		threads := [][]repository.DailyReportThread{{{Title: "fact thread", TagIDs: []byte("[123]")}}}
		require.NoError(t, repository.Repo.SaveReport(report, sections, threads))
		require.Equal(t, []byte("[]"), []byte(report.Highlights))
		if reportID != 0 {
			require.Equal(t, reportID, report.ID)
		}
		reportID = report.ID
		finalizeDailyReportHighlights(context.Background(), report, inputs)
		require.Equal(t, 1, calls, "one selected mode, no story-then-tag invocation")
		var highlights []repository.Highlight
		require.NoError(t, json.Unmarshal(report.Highlights, &highlights))
		require.Len(t, highlights, 1)
		if mode == "success" {
			require.Equal(t, &story.ID, highlights[0].StoryID)
			if previous != nil {
				require.JSONEq(t, string(previous), string(report.Highlights))
			}
			previous = report.Highlights
		} else {
			require.Nil(t, highlights[0].StoryID)
			require.Nil(t, highlights[0].ParticipantCount)
		}
		require.Equal(t, highlights[0].Title, report.Title)
		require.Equal(t, highlights[0].Reason, report.Summary)
	}
	var count int64
	require.NoError(t, db.Model(&repository.BoardDailyReport{}).Where("semantic_board_id = ?", board).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
