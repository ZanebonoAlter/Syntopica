package repository

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm"
)

// repoParsePgVector parses a pgvector string "[0.1,0.2]" into a float slice.
// Mirrors the service-layer parsePgVector; kept local to avoid an import cycle
// (service imports repository, not the reverse).
func repoParsePgVector(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil, fmt.Errorf("empty vector")
	}
	parts := strings.Split(s, ",")
	result := make([]float64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, fmt.Errorf("parse vector element %d: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}

// cosineDistance = 1 - cosineSimilarity. Mismatched or zero-length vectors
// return +Inf so the caller treats them as non-matches.
func cosineDistance(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return math.MaxFloat64
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return math.MaxFloat64
	}
	return 1 - dot/(math.Sqrt(na)*math.Sqrt(nb))
}

// topicAnchor pairs a topic id with its parsed matching anchor vector: the
// Centroid (首义锚定+滚动均值混合，见 ComputeTopicCentroid) when present,
// degrading to the Embedding first-section 首义向量 otherwise. This is the
// lane-driven matching anchor (supersedes the old first-section-only
// embedding). Topics with neither a parseable centroid nor embedding are
// dropped.
type topicAnchor struct {
	ID     uint
	Anchor []float64
	Status string
}

// parseTopicAnchors parses each topic's matching anchor (Centroid, else
// Embedding). Unparseable topics are dropped — they cannot participate in
// distance computation.
func parseTopicAnchors(topics []BoardPersistentTopic) []topicAnchor {
	out := make([]topicAnchor, 0, len(topics))
	for _, t := range topics {
		vec := topicAnchorVec(t)
		if len(vec) == 0 {
			continue
		}
		out = append(out, topicAnchor{ID: t.ID, Anchor: vec, Status: t.Status})
	}
	return out
}

// topicAnchorVec returns the topic's centroid vector when present, else its
// first-section embedding. Empty when neither parses.
func topicAnchorVec(t BoardPersistentTopic) []float64 {
	if v, err := repoParsePgVector(t.Centroid); err == nil && len(v) > 0 {
		return v
	}
	if v, err := repoParsePgVector(t.Embedding); err == nil && len(v) > 0 {
		return v
	}
	return nil
}

// findNearestAnchor returns the anchor with the smallest cosine distance to
// vec and that distance. ok=false when no comparable anchor.
func findNearestAnchor(vec []float64, anchors []topicAnchor) (id uint, dist float64, ok bool) {
	best := math.MaxFloat64
	var bestID uint
	found := false
	for _, a := range anchors {
		if len(a.Anchor) == 0 {
			continue
		}
		d := cosineDistance(vec, a.Anchor)
		if d < best {
			best = d
			bestID = a.ID
			found = true
		}
	}
	return bestID, best, found
}

// topicAssignmentDecision is the per-section outcome of the lane-driven
// assignment. topicID is set for anchor_hit; newCandidate is set for auto_new.
// laneTier records the bucketing source carried onto the section row.
type topicAssignmentDecision struct {
	sectionIdx          int
	topicID             uint
	distance            float64
	confidence          string
	laneTier            string
	newCandidate        *candidateTopicSpec
	topicStatusAtReport *string
}

// candidateTopicSpec describes a candidate topic to insert during assignment.
type candidateTopicSpec struct {
	label     string
	embedding string
	firstSeen time.Time
}

// planTopicAssignments computes the per-section assignment plan with NO DB
// access — pure and unit-testable. Attribution was already decided upstream by
// the lane bucketing (section.LaneTier + section.MatchedTopicID); this step
// only maps the lane outcome onto a confidence + distance:
//
//	l1_direct / l2_llm with a MatchedTopicID → anchor_hit (distance = section
//	  embedding to that topic's centroid anchor)
//	l3_new or no MatchedTopicID → auto_new (distance = nearest anchor, for
//	  diagnostics)
//	empty section embedding → unmatched (distance 0)
//
// cfg is retained in the signature for caller symmetry; the lane-driven path
// no longer applies an embedding threshold (the AND-gate is gone).
func planTopicAssignments(sections []DailyReportSection, existingTopics []BoardPersistentTopic, cfg PersistentTopicConfig, today time.Time) []topicAssignmentDecision {
	anchors := parseTopicAnchors(existingTopics)
	anchorByID := make(map[uint]topicAnchor, len(anchors))
	for _, a := range anchors {
		anchorByID[a.ID] = a
	}
	decisions := make([]topicAssignmentDecision, 0, len(sections))
	var candidates []candidateTopicSpec
	for i := range sections {
		sec := &sections[i]
		vec, err := repoParsePgVector(sec.Embedding)
		if err != nil || len(vec) == 0 {
			decisions = append(decisions, topicAssignmentDecision{
				sectionIdx: i, confidence: TopicConfUnmatched, laneTier: sec.LaneTier,
			})
			continue
		}
		if (sec.LaneTier == "l1_direct" || sec.LaneTier == "l2_llm") && sec.MatchedTopicID != nil {
			if a, ok := anchorByID[*sec.MatchedTopicID]; ok {
				status := a.Status
				decisions = append(decisions, topicAssignmentDecision{
					sectionIdx: i, topicID: a.ID, distance: cosineDistance(vec, a.Anchor),
					confidence: TopicConfAnchorHit, laneTier: sec.LaneTier,
					topicStatusAtReport: &status,
				})
				continue
			}
		}
		// l3_new, or lane said anchor but the topic is absent from the anchorable
		// set → open a new candidate. lane_tier becomes l3_new so the persisted
		// row keeps confidence + lane consistent.
		_, nearestDist, _ := findNearestAnchor(vec, anchors)
		candidates = append(candidates, candidateTopicSpec{
			label:     sec.ClusterLabel,
			embedding: sec.Embedding,
			firstSeen: today,
		})
		decisions = append(decisions, topicAssignmentDecision{
			sectionIdx: i, distance: nearestDist,
			confidence: TopicConfAutoNew, laneTier: "l3_new",
			newCandidate:        &candidates[len(candidates)-1],
			topicStatusAtReport: topicStatusPtr(TopicStatusCandidate),
		})
	}
	return decisions
}

func topicStatusPtr(value string) *string {
	return &value
}

// topicLifecycleChange is a pure lifecycle transition computed without DB
// access. Updated fields only; the caller persists them.
type topicLifecycleChange struct {
	topicID         uint
	status          string
	consecutiveHits int
	hitCount        int
	lastSeen        time.Time
}

// planLifecycle computes hit-count updates for a board's topics given today's
// hit set. Pure and unit-testable.
//
// hit:   consecutive_hits+1, hit_count+1, last_seen=today, status unchanged.
// miss:  consecutive_hits reset to 0, status unchanged (manual-archive only).
//
// No automatic archiving — candidate→archived and active→archived transitions
// are exclusively triggered by user operations (updateTopicStatus, DeleteTopic).
//
// New candidates created by the assignment step already carry consecutive_hits=1
// in the DB; they are NOT passed through here (they are in hitTopicIDs only if
// the caller includes them, which it should not — see assignAndUpdateTopics).
func planLifecycle(topics []BoardPersistentTopic, today time.Time, hitTopicIDs map[uint]bool) []topicLifecycleChange {
	todayDate := NormalizeReportDate(today)
	var changes []topicLifecycleChange
	for _, t := range topics {
		if hitTopicIDs[t.ID] {
			newHits := t.ConsecutiveHits + 1
			ch := topicLifecycleChange{
				topicID: t.ID,
				status:  t.Status, consecutiveHits: newHits,
				hitCount: t.HitCount + 1, lastSeen: todayDate,
			}
			changes = append(changes, ch)
			continue
		}
		// Miss: consecutive resets to 0; status and last_seen stay unchanged.
		if t.Status != TopicStatusArchived && t.ConsecutiveHits != 0 {
			changes = append(changes, topicLifecycleChange{
				topicID: t.ID, status: t.Status,
				consecutiveHits: 0, hitCount: t.HitCount,
				lastSeen: NormalizeReportDate(t.LastSeenDate),
			})
		}
	}
	return changes
}

// assignAndUpdateTopics runs the full assignment + lifecycle pipeline for one
// report save, inside the caller's transaction:
//  1. load existing anchorable topics + config
//  2. plan per-section assignment (pure, lane-driven)
//  3. create candidate topics
//  4. write assignment columns (incl. lane_tier) onto sections
//  5. advance the topic lifecycle (candidate→active→archived)
//
// It returns the set of topic ids touched by this report (anchor_hit topics +
// newly created candidates) so the caller can refresh their centroids after
// the transaction commits. Identity edges written later by
// RebuildBoardRelations depend on the persistent_topic_id set here, so this
// must run before relation rebuild and inside the same transaction.
func assignAndUpdateTopics(tx *gorm.DB, boardID uint, periodDate time.Time, sections []DailyReportSection) ([]uint, error) {
	cfg := LoadPersistentTopicConfig(tx)
	// periodDate must equal the orchestrator's startOfDay (=report.PeriodDate).
	// Both sides call ListAnchorableTopicsByBoard with the same (boardID, date, cfg)
	// so the injection set and the acceptance set are guaranteed identical.
	existingTopics, anchorStats, err := Repo.ListAnchorableTopicsByBoard(boardID, periodDate, cfg)
	if err != nil {
		return nil, fmt.Errorf("load existing topics: %w", err)
	}
	today := NormalizeReportDate(periodDate)

	// Watch-materialized sections (watch-materialized-topic design §D5):
	//   - watch_keyword sections are EXCLUDED from the planner entirely —
	//     with no embedding they would fall into the unmatched/auto_new
	//     branches and the auto_new branch would adopt them into a fresh
	//     candidate topic (spec violation: keyword track owns no topic).
	//     Their columns are already correct as inserted.
	//   - watch_sentence sections carry their own assignment columns
	//     (PersistentTopicID/manual/active) as inserted; they skip the planner
	//     too, but their topic joins the lifecycle hit set so consecutive_hits
	//     / hit_count / last_seen advance like any owned section.
	plannerSections := sections[:0:0]
	watchHitTopicIDs := make(map[uint]bool)
	for i := range sections {
		switch sections[i].LaneTier {
		case "watch_keyword":
			// excluded — nothing to do
		case "watch_sentence":
			if sections[i].PersistentTopicID != nil {
				watchHitTopicIDs[*sections[i].PersistentTopicID] = true
			}
		default:
			plannerSections = append(plannerSections, sections[i])
		}
	}
	decisions := planTopicAssignments(plannerSections, existingTopics, cfg, today)

	// Create candidates and remember their ids.
	candSpecs := make([]*candidateTopicSpec, 0, len(decisions))
	for i := range decisions {
		if decisions[i].newCandidate != nil {
			candSpecs = append(candSpecs, decisions[i].newCandidate)
		}
	}
	candIDs := make(map[*candidateTopicSpec]uint, len(candSpecs))
	for _, spec := range candSpecs {
		topic := BoardPersistentTopic{
			SemanticBoardID: boardID,
			Label:           spec.label,
			Embedding:       spec.embedding,
			Status:          TopicStatusCandidate,
			FirstSeenDate:   spec.firstSeen,
			LastSeenDate:    spec.firstSeen,
			HitCount:        1,
			ConsecutiveHits: 1,
		}
		if err := Repo.CreateTopic(tx, &topic); err != nil {
			return nil, fmt.Errorf("create candidate topic: %w", err)
		}
		candIDs[spec] = topic.ID
	}

	// Write assignment columns and collect the set of hit topic ids + every
	// touched topic (anchor targets + new candidates) for centroid refresh.
	// Newly created candidates are already at consecutive_hits=1, so they are
	// excluded from the lifecycle hit set to avoid double-counting.
	hitTopicIDs := make(map[uint]bool, len(decisions))
	touched := make(map[uint]bool, len(decisions))
	for _, d := range decisions {
		var topicID *uint
		switch d.confidence {
		case TopicConfAnchorHit:
			id := d.topicID
			topicID = &id
			hitTopicIDs[id] = true
			touched[id] = true
		case TopicConfAutoNew:
			if d.newCandidate != nil {
				id := candIDs[d.newCandidate]
				topicID = &id
				touched[id] = true
			}
		}
		if d.sectionIdx >= len(plannerSections) {
			continue
		}
		secID := plannerSections[d.sectionIdx].ID
		if err := Repo.UpdateSectionTopicAssignment(tx, secID, topicID, d.distance, d.confidence, d.laneTier, d.topicStatusAtReport); err != nil {
			return nil, fmt.Errorf("update section %d assignment: %w", secID, err)
		}
	}

	// Advance the lifecycle for pre-existing topics only. Watch-topic hits
	// (watch_sentence sections) merge into the same hit set — the lifecycle
	// rules are identical for every owned section. Watch topics are NOT added
	// to the centroid-refresh `touched` set: their sections carry no
	// embeddings, and the anchor semantics is the user's query intent — the
	// centroid stays at the seeded query vector by design.
	allTopics, err := Repo.ListAllTopicsByBoard(boardID)
	if err != nil {
		return nil, fmt.Errorf("load topics for lifecycle: %w", err)
	}
	for id := range watchHitTopicIDs {
		hitTopicIDs[id] = true
	}
	changes := planLifecycle(allTopics, today, hitTopicIDs)
	if len(changes) > 0 {
		toSave := make([]BoardPersistentTopic, 0, len(changes))
		for _, ch := range changes {
			for _, t := range allTopics {
				if t.ID == ch.topicID {
					t.Status = ch.status
					t.ConsecutiveHits = ch.consecutiveHits
					t.HitCount = ch.hitCount
					t.LastSeenDate = ch.lastSeen
					toSave = append(toSave, t)
					break
				}
			}
		}
		if err := Repo.SaveTopics(tx, toSave); err != nil {
			return nil, fmt.Errorf("save topic lifecycle changes: %w", err)
		}
	}

	matched, autoNew, unmatched := 0, 0, 0
	for _, d := range decisions {
		switch d.confidence {
		case TopicConfAnchorHit:
			matched++
		case TopicConfAutoNew:
			autoNew++
		case TopicConfUnmatched:
			unmatched++
		}
	}
	logging.Infof("persistent-topic: board %d anchors active=%d candidates=%d filtered_window=%d truncated_limit=%d; assigned %d sections (anchor_hit=%d, auto_new=%d, unmatched=%d)",
		boardID, anchorStats.ActiveCount, anchorStats.CandidateCount, anchorStats.FilteredByWindow, anchorStats.TruncatedByLimit,
		len(decisions), matched, autoNew, unmatched)

	touchedIDs := make([]uint, 0, len(touched))
	for id := range touched {
		touchedIDs = append(touchedIDs, id)
	}
	return touchedIDs, nil
}
