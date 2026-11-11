package repository

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"gorm.io/gorm"
)

// ReportStoryCandidate is a current-report identity, not an alternative heat source.
type ReportStoryCandidate struct {
	StoryID          uint      `json:"story_id"`
	StoryTitle       string    `json:"story_title"`
	ParticipantCount int       `json:"participant_count"`
	StoryHeat        float64   `json:"story_heat"`
	TagIDs           []uint    `json:"tag_ids"`
	HeatAsOf         time.Time `json:"-"`
	HeatIncomplete   bool      `json:"-"`
}

// highlightStorySurvivors rejects broken, cross-board (not loaded), cyclic and
// multi-hop references. It never changes stored identity or manual assignment.
func highlightStorySurvivors(stories []BoardStory) map[uint]uint {
	edges := make([]storyIdentityEdge, 0, len(stories))
	for _, s := range stories {
		edges = append(edges, storyIdentityEdge{ID: s.ID, MergedInto: s.MergedInto, Status: s.Status})
	}
	return strictStorySurvivors(edges)
}

func (r *TopicGraphRepository) reportStoryIdentities(boardID uint) ([]BoardStory, map[uint]uint, error) {
	var stories []BoardStory
	err := r.db.Select("id", "title", "merged_into", "status").Where("board_id = ?", boardID).Find(&stories).Error
	return stories, highlightStorySurvivors(stories), err
}

// YesterdayHighlightStoryIDs uses the exact preceding Beijing calendar day,
// never prev_report_id or the nearest report. Old JSON naturally yields no ids.
func (r *TopicGraphRepository) YesterdayHighlightStoryIDs(boardID uint, date time.Time) (map[uint]bool, error) {
	day := date.In(storyHeatZone).AddDate(0, 0, -1).Format("2006-01-02")
	var report BoardDailyReport
	out := make(map[uint]bool)
	err := r.db.Select("highlights").Where("semantic_board_id = ? AND period_date = ? AND status IN ?", boardID, day, []string{"completed", "done"}).Take(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var highlights []Highlight
	if err = json.Unmarshal(report.Highlights, &highlights); err != nil {
		return out, nil
	}
	_, ids, err := r.reportStoryIdentities(boardID)
	if err != nil {
		return nil, err
	}
	for _, h := range highlights {
		if h.StoryID != nil {
			if id := ids[*h.StoryID]; id != 0 {
				out[id] = true
			}
		}
	}
	return out, nil
}

func (r *TopicGraphRepository) resolveReportHighlightIdentities(report *BoardDailyReport) error {
	_, ids, err := r.reportStoryIdentities(report.SemanticBoardID)
	if err != nil {
		return err
	}
	yesterday, err := r.YesterdayHighlightStoryIDs(report.SemanticBoardID, report.PeriodDate)
	if err != nil {
		return err
	}
	var highlights []Highlight
	if json.Unmarshal(report.Highlights, &highlights) == nil {
		for i := range highlights {
			h := &highlights[i]
			h.IsProgress = false
			if h.StoryID != nil {
				id := ids[*h.StoryID]
				h.StoryID = nil
				if id != 0 {
					h.StoryID = &id
					h.IsProgress = yesterday[id]
				}
			}
		}
		if highlights == nil {
			highlights = []Highlight{}
		}
		report.Highlights, _ = json.Marshal(highlights)
	}
	for i := range report.Sections {
		for j := range report.Sections[i].Threads {
			t := &report.Sections[i].Threads[j]
			if t.StoryID != nil {
				id := ids[*t.StoryID]
				t.StoryID = nil
				if id != 0 {
					t.StoryID = &id
				}
			}
		}
	}
	return nil
}

func rankReportStoryCandidates(candidates []ReportStoryCandidate, evidence boardHeatEvidence, at time.Time) []ReportStoryCandidate {
	heat, _ := evidence.compute(at)
	participants := make(map[storyParticipant]bool)
	counts := make(map[uint]int)
	for _, s := range evidence.signals {
		id := evidence.survivors[s.StoryID]
		if id == 0 || s.ObservedAt.After(at) || s.ObservedAt.Before(at.Add(-StoryHeatWindowHours*time.Hour)) {
			continue
		}
		key := storyParticipant{id, heatParticipantKey(s.ContentKey, s.ParticipantKey)}
		if !participants[key] {
			participants[key] = true
			counts[id]++
		}
	}
	for i := range candidates {
		candidates[i].StoryHeat = heat[candidates[i].StoryID]
		candidates[i].ParticipantCount = counts[candidates[i].StoryID]
		candidates[i].HeatAsOf = at
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.StoryHeat != b.StoryHeat {
			return a.StoryHeat > b.StoryHeat
		}
		if a.ParticipantCount != b.ParticipantCount {
			return a.ParticipantCount > b.ParticipantCount
		}
		return a.StoryID < b.StoryID
	})
	// No top-8 truncation here: the service confirms usable facts first and
	// only then caps the prompt set (surface-story-heat-in-report-top defect
	// fix — 先确认有可用事实再截 top 8，前 8 全无事实时后续有事实的候选补入).
	return candidates
}

// ReportStoryCandidates filters only display eligibility; flash/hidden evidence
// still contributes to the independent heat query.
func (r *TopicGraphRepository) ReportStoryCandidates(boardID, reportID uint, date time.Time) ([]ReportStoryCandidate, error) {
	stories, ids, err := r.reportStoryIdentities(boardID)
	if err != nil {
		return nil, err
	}
	titles := make(map[uint]string)
	for _, s := range stories {
		titles[s.ID] = s.Title
	}
	var threads []DailyReportThread
	if err = r.db.Select("story_id", "tag_ids").Where("report_id = ? AND scope = ? AND display_tier = ? AND story_id IS NOT NULL", reportID, ThreadScopeSingle, ThreadDisplayTierBody).Find(&threads).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]ReportStoryCandidate)
	for _, thread := range threads {
		id := ids[*thread.StoryID]
		if id == 0 {
			continue
		}
		c := byID[id]
		c.StoryID = id
		c.StoryTitle = titles[id]
		for _, tag := range jsonUintIDs(thread.TagIDs) {
			c.TagIDs = appendUniqueUint(c.TagIDs, tag)
		}
		byID[id] = c
	}
	candidates := make([]ReportStoryCandidate, 0, len(byID))
	for _, c := range byID {
		candidates = append(candidates, c)
	}
	at := laneHeatAnchorEnd(date)
	evidence, err := r.loadBoardHeatEvidence(boardID, at.Add(-StoryHeatWindowHours*time.Hour), at)
	if err != nil {
		return nil, err
	}
	// Use the strict one-hop identity map for candidate evidence as well.
	evidence.survivors = ids
	candidates = rankReportStoryCandidates(candidates, evidence, at)
	coverage, err := r.BoardHeatCoverage(boardID, time.Now())
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		candidates[i].HeatIncomplete = coverage.Incomplete
	}
	return candidates, nil
}

// FinalizeReportHighlights is one conditional SQL write (an implicit short
// transaction). No external calls occur inside it. The revision is read back
// by SaveReport, including PostgreSQL timestamp precision.
func (r *TopicGraphRepository) FinalizeReportHighlights(report *BoardDailyReport, highlights []Highlight) (bool, error) {
	if highlights == nil {
		highlights = []Highlight{}
	}
	payload, err := json.Marshal(highlights)
	if err != nil {
		return false, err
	}
	title := "日报 " + report.PeriodDate.Format("2006-01-02")
	summary := ""
	if len(highlights) > 0 {
		title = highlights[0].Title
		summary = highlights[0].Reason
	}
	result := r.db.Model(&BoardDailyReport{}).Where("id = ? AND updated_at = ?", report.ID, report.UpdatedAt).Updates(map[string]any{"highlights": JSON(payload), "title": title, "summary": summary})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	report.Highlights = payload
	report.Title = title
	report.Summary = summary
	return true, nil
}

func (c ReportStoryCandidate) Highlight(title, reason string, tags []uint) Highlight {
	id, p, h, incomplete, at := c.StoryID, c.ParticipantCount, math.Round(c.StoryHeat*10)/10, c.HeatIncomplete, c.HeatAsOf
	return Highlight{StoryID: &id, Title: title, Reason: reason, TagIDs: tags, ParticipantCount: &p, StoryHeat: &h, HeatRuleVersion: HeatRuleVersion, HeatAsOf: &at, HeatIncomplete: &incomplete}
}
