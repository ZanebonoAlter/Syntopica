package repository

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// PersistentTopicConfig holds the runtime-tunable parameters that govern
// assignment, lifecycle and backfill. Loaded from ai_settings with the
// defaults baked into the migration seed.
type PersistentTopicConfig struct {
	MatchThreshold       float64 // persistent_topic_match_threshold
	UpgradeThreshold     int     // persistent_topic_upgrade_threshold
	CandidateDecayWindow int     // persistent_topic_candidate_decay_window (days)
	CandidatePromptLimit int     // persistent_topic_candidate_prompt_limit
	ClusterThreshold     float64 // persistent_topic_cluster_threshold
	// Lane-driven clustering params (daily-report-lane-driven-clustering).
	LaneL1Threshold float64 // persistent_topic_lane_l1_threshold: direct-attach ceiling
	LaneL2Threshold float64 // persistent_topic_lane_l2_threshold: weak-zone upper bound
	VacuumRatio     float64 // persistent_topic_vacuum_ratio: is_vacuum trigger
	CentroidWindow  int     // persistent_topic_centroid_window: # recent sections averaged
	VacuumWindow    int     // persistent_topic_vacuum_window: attraction stats span (days)
	L2CandidateK    int     // persistent_topic_l2_candidate_k: top-K candidates for L2 LLM
	// SectionMergeEnabled (daily_report_section_merge_enabled) gates the
	// same-day two-stage section merge (fix-section-merge-blackhole). The
	// content-based section embedding changed the distance geometry so the
	// 0.20/0.25 thresholds no longer discriminate narratives; default off
	// keeps sections at the lane pipeline's original granularity.
	SectionMergeEnabled bool // daily_report_section_merge_enabled: same-day section merge kill switch
	// CandidateL1GateEnabled (persistent_topic_candidate_l1_gate_enabled,
	// candidate-topic-l2-gate) gates the observation-period L1 direct-attach:
	// when on (default), candidate topics do NOT direct-attach near-distance
	// tags — they fall through to the L2 LLM band; active topics keep direct
	// attach. When off, both active and candidate direct-attach (legacy
	// behavior). Runtime kill switch for online rollback without a release.
	CandidateL1GateEnabled bool // persistent_topic_candidate_l1_gate_enabled: candidate L1 direct-attach gate
}

// DefaultPersistentTopicConfig returns the seed defaults; used when ai_settings
// rows are absent (e.g. before migration runs, or in fresh test DBs).
//
// ClusterThreshold default is 0.28 (complete-link). Real-data diagnosis on the
// sampled production boards (108 sections / 3 boards) calibrated this:
//   - 0.30 centroid/greedy → collapsed each board to 1 mega-topic (chaining)
//   - 0.25 complete-link   → too fragmented (b1980: 47 sections → 21 topics)
//   - 0.28 complete-link   → in target range (b1974=11, b1980=15, b2197=15)
//
// See the change verification-report for the full threshold scan.
func DefaultPersistentTopicConfig() PersistentTopicConfig {
	return PersistentTopicConfig{
		MatchThreshold:       0.30,
		UpgradeThreshold:     3,
		CandidateDecayWindow: 7,
		CandidatePromptLimit: 20,
		ClusterThreshold:     0.28,
		// Lane defaults calibrated on the real-data diagnosis
		// (docs/experience/cluster-bias-investigation.md): L1<0.18 lifts
		// strong-attach from 14% to 62%, L2[0.18,0.30] leaves ~51% to the
		// LLM, L3>0.30 is the ~1.3% new-narrative tail.
		LaneL1Threshold:     0.18,
		LaneL2Threshold:     0.30,
		VacuumRatio:         0.20,
		CentroidWindow:      30,
		VacuumWindow:        7,
		L2CandidateK:        5,
		SectionMergeEnabled: false,
		// candidate-topic-l2-gate: gate on by default — candidate topics are
		// system guesses and must pass L2 adjudication for every attach.
		CandidateL1GateEnabled: true,
	}
}

type AnchorableTopicStats struct {
	ActiveCount      int
	CandidateCount   int
	FilteredByWindow int
	TruncatedByLimit int
}

func selectAnchorableTopics(topics []BoardPersistentTopic, reportDate time.Time, cfg PersistentTopicConfig) ([]BoardPersistentTopic, AnchorableTopicStats) {
	stats := AnchorableTopicStats{}
	active := make([]BoardPersistentTopic, 0, len(topics))
	candidates := make([]BoardPersistentTopic, 0, len(topics))
	reportDay := NormalizeReportDate(reportDate)
	defaults := DefaultPersistentTopicConfig()
	candidateDecayWindow := cfg.CandidateDecayWindow
	if candidateDecayWindow <= 0 {
		candidateDecayWindow = defaults.CandidateDecayWindow
	}
	candidatePromptLimit := cfg.CandidatePromptLimit
	if candidatePromptLimit <= 0 {
		candidatePromptLimit = defaults.CandidatePromptLimit
	}

	for _, topic := range topics {
		switch topic.Status {
		case TopicStatusActive:
			stats.ActiveCount++
			active = append(active, topic)
		case TopicStatusCandidate:
			stats.CandidateCount++
			lastSeen := NormalizeReportDate(topic.LastSeenDate)
			// gapDays relies on NormalizeReportDate midnight normalisation:
			// the difference of two noon-UTC timestamps is an integer multiple of 24h.
			gapDays := int(reportDay.Sub(lastSeen).Hours() / 24)
			if gapDays > candidateDecayWindow {
				stats.FilteredByWindow++
				continue
			}
			candidates = append(candidates, topic)
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if !left.LastSeenDate.Equal(right.LastSeenDate) {
			return left.LastSeenDate.After(right.LastSeenDate)
		}
		if left.HitCount != right.HitCount {
			return left.HitCount > right.HitCount
		}
		return left.ID < right.ID
	})
	if len(candidates) > candidatePromptLimit {
		stats.TruncatedByLimit = len(candidates) - candidatePromptLimit
		candidates = candidates[:candidatePromptLimit]
	}

	return append(active, candidates...), stats
}

// LoadPersistentTopicConfig reads the persistent_topic_* keys from
// ai_settings, falling back to defaults on missing/invalid rows. Reading is
// best-effort: a malformed value never fails the report pipeline.
func LoadPersistentTopicConfig(db *gorm.DB) PersistentTopicConfig {
	cfg := DefaultPersistentTopicConfig()
	keys := []string{
		"persistent_topic_match_threshold",
		"persistent_topic_upgrade_threshold",
		"persistent_topic_candidate_decay_window",
		"persistent_topic_candidate_prompt_limit",
		"persistent_topic_cluster_threshold",
		"persistent_topic_lane_l1_threshold",
		"persistent_topic_lane_l2_threshold",
		"persistent_topic_vacuum_ratio",
		"persistent_topic_centroid_window",
		"persistent_topic_vacuum_window",
		"persistent_topic_l2_candidate_k",
		"daily_report_section_merge_enabled",
		"persistent_topic_candidate_l1_gate_enabled",
	}
	var rows []models.AISettings
	if err := db.Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return cfg
	}
	for _, r := range rows {
		switch r.Key {
		case "persistent_topic_match_threshold":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil {
				cfg.MatchThreshold = v
			}
		case "persistent_topic_upgrade_threshold":
			if v, err := strconv.Atoi(r.Value); err == nil {
				cfg.UpgradeThreshold = v
			}
		case "persistent_topic_candidate_decay_window":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.CandidateDecayWindow = v
			} else {
				logging.Warnf("persistent-topic: invalid candidate decay window %q; using default %d", r.Value, cfg.CandidateDecayWindow)
			}
		case "persistent_topic_candidate_prompt_limit":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.CandidatePromptLimit = v
			} else {
				logging.Warnf("persistent-topic: invalid candidate prompt limit %q; using default %d", r.Value, cfg.CandidatePromptLimit)
			}
		case "persistent_topic_cluster_threshold":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil {
				cfg.ClusterThreshold = v
			}
		case "persistent_topic_lane_l1_threshold":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil {
				cfg.LaneL1Threshold = v
			}
		case "persistent_topic_lane_l2_threshold":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil {
				cfg.LaneL2Threshold = v
			}
		case "persistent_topic_vacuum_ratio":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil {
				cfg.VacuumRatio = v
			}
		case "persistent_topic_centroid_window":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.CentroidWindow = v
			} else {
				logging.Warnf("persistent-topic: invalid centroid window %q; using default %d", r.Value, cfg.CentroidWindow)
			}
		case "persistent_topic_vacuum_window":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.VacuumWindow = v
			} else {
				logging.Warnf("persistent-topic: invalid vacuum window %q; using default %d", r.Value, cfg.VacuumWindow)
			}
		case "persistent_topic_l2_candidate_k":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.L2CandidateK = v
			} else {
				logging.Warnf("persistent-topic: invalid l2 candidate k %q; using default %d", r.Value, cfg.L2CandidateK)
			}
		case "daily_report_section_merge_enabled":
			if v, err := strconv.ParseBool(r.Value); err == nil {
				cfg.SectionMergeEnabled = v
			} else {
				logging.Warnf("daily-report: invalid section merge enabled %q; using default %t", r.Value, cfg.SectionMergeEnabled)
			}
		case "persistent_topic_candidate_l1_gate_enabled":
			if v, err := strconv.ParseBool(r.Value); err == nil {
				cfg.CandidateL1GateEnabled = v
			} else {
				logging.Warnf("persistent-topic: invalid candidate l1 gate enabled %q; using default %t", r.Value, cfg.CandidateL1GateEnabled)
			}
		}
	}
	return cfg
}

// persistentTopicPalette is the colour pool for per-topic card tinting in the
// detective wall. These are warm, low-saturation tones that read as "paper
// variants" against the cork board, distinct from the status colours (which
// stay green/blue/orange/purple/grey). Indexed by hash(topicID) % len.
var persistentTopicPalette = []string{
	"#F7E7C4", // paper warm (default)
	"#E9D5C5", // rose paper
	"#D6E4E0", // sage paper
	"#E3D9E8", // lilac paper
	"#E8D9C5", // amber paper
	"#CFE0E8", // sky paper
	"#EDE0C8", // sand paper
	"#D8E3D0", // moss paper
}

// PersistentTopicColor returns a stable colour for a topic id. The colour is
// computed from a fixed palette indexed by a hash of the id, so the same topic
// always gets the same colour across renders (no client-side flicker). Used to
// tint detective-wall cards so sections of one narrative share a hue.
func PersistentTopicColor(topicID uint) string {
	if len(persistentTopicPalette) == 0 {
		return "#F7E7C4"
	}
	// FNV-1a-ish mixing; stable and dependency-free.
	h := uint64(topicID)
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	return persistentTopicPalette[h%uint64(len(persistentTopicPalette))]
}

// ListAnchorableTopicsByBoard returns the exact active+candidate frame set
// shared by clustering and dual-confirmation assignment for one report date.
func (r *TopicGraphRepository) ListAnchorableTopicsByBoard(boardID uint, reportDate time.Time, cfg PersistentTopicConfig) ([]BoardPersistentTopic, AnchorableTopicStats, error) {
	var topics []BoardPersistentTopic
	err := r.db.Where("semantic_board_id = ? AND status IN ?", boardID,
		[]string{TopicStatusCandidate, TopicStatusActive}).
		Find(&topics).Error
	if err != nil {
		return nil, AnchorableTopicStats{}, fmt.Errorf("list anchorable topics: %w", err)
	}
	selected, stats := selectAnchorableTopics(topics, reportDate, cfg)
	return selected, stats, nil
}

// ListAllTopicsByBoard returns every non-archived topic (candidate+active) for
// a board. It does NOT apply window or limit filtering. Used by the lifecycle
// updater (planLifecycle) and diagnostics — never for ClusterTags injection or
// assignment (that is ListAnchorableTopicsByBoard's responsibility).
func (r *TopicGraphRepository) ListAllTopicsByBoard(boardID uint) ([]BoardPersistentTopic, error) {
	var topics []BoardPersistentTopic
	err := r.db.Where("semantic_board_id = ? AND status != ?", boardID, TopicStatusArchived).
		Find(&topics).Error
	if err != nil {
		return nil, fmt.Errorf("list all topics: %w", err)
	}
	return topics, nil
}

// ListTopicsByBoardAll returns every persistent topic on a board, including
// archived ones and topics that no section currently references (orphans).
// Unlike ListAllTopicsByBoard, this is for the management UI which must show
// everything so anomalous/orphan topics can be archived or hard-deleted.
func (r *TopicGraphRepository) ListTopicsByBoardAll(boardID uint) ([]BoardPersistentTopic, error) {
	var topics []BoardPersistentTopic
	err := r.db.Where("semantic_board_id = ?", boardID).
		Order("status = 'archived', hit_count DESC, id ASC").Find(&topics).Error
	if err != nil {
		return nil, fmt.Errorf("list all topics (incl. archived): %w", err)
	}
	return topics, nil
}

// DeleteTopic hard-deletes a persistent topic. Sections that referenced it are
// unlinked (persistent_topic_id set to NULL) rather than deleted — the section
// timeline still renders them as standalone nodes. Board relations are rebuilt
// so stale identity/similarity edges disappear. This is irreversible; archive
// (UpdateTopic status=archived) is the reversible soft path.
func (r *TopicGraphRepository) DeleteTopic(topicID uint) error {
	var topic BoardPersistentTopic
	if err := r.db.First(&topic, topicID).Error; err != nil {
		return fmt.Errorf("load topic %d: %w", topicID, err)
	}
	boardID := topic.SemanticBoardID
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Unlink sections; they keep their content but lose the topic assignment.
		if err := tx.Model(&DailyReportSection{}).
			Where("persistent_topic_id = ?", topicID).
			Updates(map[string]interface{}{
				"persistent_topic_id":    nil,
				"topic_match_distance":   nil,
				"topic_match_confidence": nil,
				"topic_status_at_report": nil,
			}).Error; err != nil {
			return fmt.Errorf("unlink sections: %w", err)
		}
		// Hard delete the topic row.
		if err := tx.Where("id = ?", topicID).Delete(&BoardPersistentTopic{}).Error; err != nil {
			return fmt.Errorf("delete topic %d: %w", topicID, err)
		}
		// Rebuild relations so edges referencing the deleted topic are dropped.
		if err := RebuildBoardRelations(tx, boardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	})
}

// FilterVisibleTopics returns topics that should appear in the management UI.
// Active and archived always visible; candidates only visible when their
// cumulative hit_count >= upgrade_threshold (observing candidates are hidden).
// Note: the gate is cumulative hits, NOT consecutive hits — a topic that
// appears on-and-off still qualifies once it has been hit enough times total.
func FilterVisibleTopics(topics []BoardPersistentTopic, upgradeThreshold int) []BoardPersistentTopic {
	result := make([]BoardPersistentTopic, 0, len(topics))
	for _, t := range topics {
		if t.Status != TopicStatusCandidate || t.HitCount >= upgradeThreshold {
			result = append(result, t)
		}
	}
	return result
}

// CreateTopic inserts a new persistent topic row and returns it with the ID set.
func (r *TopicGraphRepository) CreateTopic(tx *gorm.DB, topic *BoardPersistentTopic) error {
	if tx == nil {
		tx = r.db
	}
	if err := tx.Create(topic).Error; err != nil {
		return fmt.Errorf("create persistent topic: %w", err)
	}
	return nil
}

// SaveTopics persists updated topic rows (lifecycle transitions). Uses Updates
// on the concrete fields to avoid clobbering embedding (stored as pgvector
// string, not re-serializable via Save).
func (r *TopicGraphRepository) SaveTopics(tx *gorm.DB, topics []BoardPersistentTopic) error {
	if tx == nil {
		tx = r.db
	}
	for i := range topics {
		if err := tx.Model(&BoardPersistentTopic{}).Where("id = ?", topics[i].ID).
			Updates(map[string]interface{}{
				"status":           topics[i].Status,
				"hit_count":        topics[i].HitCount,
				"consecutive_hits": topics[i].ConsecutiveHits,
				"first_seen_date":  topics[i].FirstSeenDate,
				"last_seen_date":   topics[i].LastSeenDate,
				"label":            topics[i].Label,
				"description":      topics[i].Description,
				"updated_at":       time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("save topic %d: %w", topics[i].ID, err)
		}
	}
	return nil
}

// UpdateSectionTopicAssignment writes the assignment columns onto an
// already-inserted section row (sections have IDs after CreateInBatches).
// laneTier records which lane produced the assignment (l1_direct/l2_llm/
// l3_new); pass "" to leave the column NULL (historical/backfill rows).
func (r *TopicGraphRepository) UpdateSectionTopicAssignment(tx *gorm.DB, sectionID uint, topicID *uint, distance float64, confidence string, laneTier string, topicStatusAtReport *string) error {
	if tx == nil {
		tx = r.db
	}
	updates := map[string]interface{}{
		"persistent_topic_id":    topicID,
		"topic_match_distance":   distance,
		"topic_match_confidence": confidence,
		"topic_status_at_report": topicStatusAtReport,
	}
	// lane_tier: empty string writes NULL (historical rows), a real lane writes
	// the lane label. Writes happen for every new section so the column reflects
	// the bucketing source.
	if laneTier == "" {
		updates["lane_tier"] = nil
	} else {
		updates["lane_tier"] = laneTier
	}
	return tx.Model(&DailyReportSection{}).Where("id = ?", sectionID).
		Updates(updates).Error
}

// ListTagSemanticEmbeddings loads the semantic-track pgvector embedding for
// each requested tag from topic_tag_embeddings. Tags without a semantic row
// are absent from the returned map; the caller (lane bucketing) routes such
// tags to the L3 new-narrative bucket. Additive read-only helper for the
// lane-driven clustering pipeline.
func (r *TopicGraphRepository) ListTagSemanticEmbeddings(tagIDs []uint) (map[uint]string, error) {
	out := make(map[uint]string, len(tagIDs))
	if len(tagIDs) == 0 {
		return out, nil
	}
	type row struct {
		TopicTagID uint
		Embedding  string
	}
	var rows []row
	err := r.db.Raw(`
		SELECT topic_tag_id, embedding
		FROM topic_tag_embeddings
		WHERE embedding_type = 'semantic' AND topic_tag_id IN ?
	`, tagIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list tag semantic embeddings: %w", err)
	}
	for _, rw := range rows {
		if rw.Embedding != "" {
			out[rw.TopicTagID] = rw.Embedding
		}
	}
	return out, nil
}

// ListSectionsByBoardOrdered returns all sections for a board ordered by date
// then id, with their embedding and topic assignment. Used by backfill and
// relation-rebuild identity-edge logic.
func (r *TopicGraphRepository) ListSectionsByBoardOrdered(boardID uint) ([]DailyReportSection, error) {
	type row struct {
		ID                   uint
		ReportID             uint
		ClusterIndex         int
		ClusterLabel         string
		Embedding            string
		PersistentTopicID    *uint
		TopicMatchDistance   float64
		TopicMatchConfidence string
		Day                  time.Time
	}
	var rows []row
	err := r.db.Raw(`
		SELECT s.id, s.report_id, s.cluster_index, s.cluster_label, s.embedding,
		       s.persistent_topic_id, s.topic_match_distance, s.topic_match_confidence,
		       r.period_date AS day
		FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id = s.report_id
		WHERE r.semantic_board_id = ?
		ORDER BY r.period_date ASC, s.id ASC
	`, boardID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list sections by board: %w", err)
	}
	// Preserve date order in returned slices; the Day field is carried on
	// CreatedAt as a lightweight carrier (sections themselves don't store date).
	secs := make([]DailyReportSection, len(rows))
	for i, rw := range rows {
		secs[i] = DailyReportSection{
			ID:                   rw.ID,
			ReportID:             rw.ReportID,
			ClusterIndex:         rw.ClusterIndex,
			ClusterLabel:         rw.ClusterLabel,
			Embedding:            rw.Embedding,
			PersistentTopicID:    rw.PersistentTopicID,
			TopicMatchDistance:   rw.TopicMatchDistance,
			TopicMatchConfidence: rw.TopicMatchConfidence,
			CreatedAt:            rw.Day,
		}
	}
	return secs, nil
}

// UpdateTopic mutates the user-editable fields of a persistent topic (label
// and/or status). This backs PATCH /api/daily-reports/topics/:id — manual
// rename, archive and reactivate. Lifecycle counters (hit_count,
// consecutive_hits, seen dates) stay owned by the assignment algorithm and
// are never touched here. status is restricted to active/archived: demoting a
// topic back to candidate is an algorithm decision, not a user one.
func (r *TopicGraphRepository) UpdateTopic(topicID uint, label *string, status *string) (*BoardPersistentTopic, error) {
	var topic BoardPersistentTopic
	if err := r.db.First(&topic, topicID).Error; err != nil {
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}
	updates := map[string]interface{}{"updated_at": time.Now()}
	if label != nil {
		if trimmed := strings.TrimSpace(*label); trimmed != "" {
			updates["label"] = trimmed
		}
	}
	if status != nil && *status != "" {
		switch *status {
		case TopicStatusActive, TopicStatusArchived:
			if *status == TopicStatusActive && topic.Status == TopicStatusCandidate {
				threshold := LoadPersistentTopicConfig(r.db).UpgradeThreshold
				if topic.HitCount < threshold {
					return nil, fmt.Errorf("话题需累计命中至少 %d 次后才能人工确认（当前 %d 次）", threshold, topic.HitCount)
				}
			}
			updates["status"] = *status
		default:
			return nil, fmt.Errorf("invalid status %q (allowed: active, archived)", *status)
		}
	}
	if len(updates) == 1 { // only updated_at → nothing to do
		return &topic, nil
	}
	if err := r.db.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update topic %d: %w", topicID, err)
	}
	if err := r.db.First(&topic, topicID).Error; err != nil {
		return nil, fmt.Errorf("reload topic %d: %w", topicID, err)
	}
	return &topic, nil
}

// MergeTopics reassigns every section on the source topics to the target
// topic, archives the sources, and rebuilds board relations so identity edges
// follow the new ownership. Sources must share the target's board and must not
// equal the target. Sources are archived (not physically deleted) to preserve
// historical rows and keep the audit trail intact.
func (r *TopicGraphRepository) MergeTopics(targetTopicID uint, sourceTopicIDs []uint) (*BoardPersistentTopic, error) {
	if len(sourceTopicIDs) == 0 {
		return nil, fmt.Errorf("merge: no source topics")
	}
	// Deduplicate and drop the target id if the client redundantly included it.
	uniq := make(map[uint]bool)
	for _, sid := range sourceTopicIDs {
		if sid == targetTopicID {
			continue
		}
		uniq[sid] = true
	}
	if len(uniq) == 0 {
		return nil, fmt.Errorf("merge: source topics must differ from target")
	}
	srcIDs := make([]uint, 0, len(uniq))
	for id := range uniq {
		srcIDs = append(srcIDs, id)
	}

	var target BoardPersistentTopic
	if err := r.db.First(&target, targetTopicID).Error; err != nil {
		return nil, fmt.Errorf("load target topic %d: %w", targetTopicID, err)
	}
	var sources []BoardPersistentTopic
	if err := r.db.Where("id IN ?", srcIDs).Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("load source topics: %w", err)
	}
	if len(sources) != len(srcIDs) {
		return nil, fmt.Errorf("merge: some source topics not found")
	}
	for _, s := range sources {
		if s.SemanticBoardID != target.SemanticBoardID {
			return nil, fmt.Errorf("merge: topic %d belongs to a different board", s.ID)
		}
	}

	boardID := target.SemanticBoardID
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Reassign every section on the sources to the target.
		if err := tx.Model(&DailyReportSection{}).
			Where("persistent_topic_id IN ?", srcIDs).
			Updates(map[string]interface{}{
				"persistent_topic_id":    targetTopicID,
				"topic_match_confidence": TopicConfAnchorHit,
			}).Error; err != nil {
			return fmt.Errorf("reassign sections: %w", err)
		}
		// Archive the sources (soft delete: the rows remain for audit).
		if err := tx.Model(&BoardPersistentTopic{}).
			Where("id IN ?", srcIDs).
			Updates(map[string]interface{}{
				"status":     TopicStatusArchived,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("archive sources: %w", err)
		}
		// Rebuild relations so identity edges follow the new ownership.
		if err := RebuildBoardRelations(tx, boardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := r.db.First(&target, targetTopicID).Error; err != nil {
		return nil, fmt.Errorf("reload target topic: %w", err)
	}
	return &target, nil
}

// SplitTopic carves the given sections out of an existing topic into a freshly
// created topic. The new topic's embedding is the mean of the carved sections'
// embeddings (empty when none have embeddings, which leaves it unmatchable by
// findNearestTopic until the next backfill). At least one section must remain
// on the source topic — carving all sections is a rename, not a split.
func (r *TopicGraphRepository) SplitTopic(sourceTopicID uint, sectionIDs []uint, label string) (*BoardPersistentTopic, error) {
	if len(sectionIDs) == 0 {
		return nil, fmt.Errorf("split: no sections")
	}
	label = strings.TrimSpace(label)

	var source BoardPersistentTopic
	if err := r.db.First(&source, sourceTopicID).Error; err != nil {
		return nil, fmt.Errorf("load source topic %d: %w", sourceTopicID, err)
	}
	// Validate every requested section currently belongs to the source topic.
	var count int64
	if err := r.db.Model(&DailyReportSection{}).
		Where("id IN ? AND persistent_topic_id = ?", sectionIDs, sourceTopicID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count carved sections: %w", err)
	}
	if int(count) != len(sectionIDs) {
		return nil, fmt.Errorf("split: some sections do not belong to topic %d", sourceTopicID)
	}
	// Refuse to empty the source — the caller should rename instead.
	var total int64
	if err := r.db.Model(&DailyReportSection{}).
		Where("persistent_topic_id = ?", sourceTopicID).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count source sections: %w", err)
	}
	if int(total) <= len(sectionIDs) {
		return nil, fmt.Errorf("split: cannot carve all sections (rename the topic instead)")
	}

	// Load carved sections (embedding for the new topic centroid) and their
	// date span (for first/last_seen).
	var carved []DailyReportSection
	if err := r.db.Select("id, report_id, embedding").Where("id IN ?", sectionIDs).Find(&carved).Error; err != nil {
		return nil, fmt.Errorf("load carved sections: %w", err)
	}
	type dateRow struct {
		Day time.Time
	}
	var dateRows []dateRow
	if err := r.db.Raw(`
		SELECT r.period_date AS day
		FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id = s.report_id
		WHERE s.id IN ?
	`, sectionIDs).Scan(&dateRows).Error; err != nil {
		return nil, fmt.Errorf("load carved dates: %w", err)
	}
	var firstSeen, lastSeen time.Time
	if len(dateRows) > 0 {
		firstSeen, lastSeen = dateRows[0].Day, dateRows[0].Day
		for _, d := range dateRows {
			if d.Day.Before(firstSeen) {
				firstSeen = d.Day
			}
			if d.Day.After(lastSeen) {
				lastSeen = d.Day
			}
		}
	}

	// New topic embedding = mean of carved section embeddings.
	var embedding string
	var vecs [][]float64
	for _, s := range carved {
		if v, err := repoParsePgVector(s.Embedding); err == nil && len(v) > 0 {
			vecs = append(vecs, v)
		}
	}
	if len(vecs) > 0 {
		dim := len(vecs[0])
		mean := make([]float64, dim)
		for _, v := range vecs {
			for j := 0; j < dim; j++ {
				mean[j] += v[j]
			}
		}
		for j := 0; j < dim; j++ {
			mean[j] /= float64(len(vecs))
		}
		embedding = FloatsToPgVector(mean)
	}

	newLabel := label
	if newLabel == "" {
		newLabel = source.Label + "（拆分）"
	}

	boardID := source.SemanticBoardID
	newTopic := &BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           newLabel,
		Embedding:       embedding,
		Status:          TopicStatusActive,
		FirstSeenDate:   firstSeen,
		LastSeenDate:    lastSeen,
		HitCount:        len(sectionIDs),
		ConsecutiveHits: len(sectionIDs),
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := r.CreateTopic(tx, newTopic); err != nil {
			return fmt.Errorf("create split topic: %w", err)
		}
		if err := tx.Model(&DailyReportSection{}).
			Where("id IN ?", sectionIDs).
			Updates(map[string]interface{}{
				"persistent_topic_id":    newTopic.ID,
				"topic_match_confidence": TopicConfAnchorHit,
			}).Error; err != nil {
			return fmt.Errorf("reassign carved sections: %w", err)
		}
		if err := RebuildBoardRelations(tx, boardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return newTopic, nil
}

// ── Lane-driven clustering: centroid + vacuum (daily-report-lane-driven-clustering) ──
//
// The functions below are the Foundation layer (Wave 1): pure helpers plus the
// data-access methods Wave 2's algorithm will call. They touch ONLY additive
// state (board_persistent_topics.centroid/is_vacuum/vacuum_strong/vacuum_mid);
// no assignment/cluster/orchestrator logic is changed here.

// meanPgVectors returns the element-wise mean of the given vectors. Vectors of
// differing length are truncated to the shortest so a malformed row cannot
// poison the average. Empty input (or a zero-length first row) returns nil;
// the caller (ComputeTopicCentroid) treats nil as "degrade to first-section".
//
// Equal-weight averaging is used deliberately: the spec sets centroid_window
// (how many sections) but leaves the weighting open, and equal-weight is the
// simplest defensible choice (a weighted variant can land later without
// changing the signature). Extracted as a pure helper so the averaging logic
// is unit-testable without a database.
func meanPgVectors(vectors [][]float64) []float64 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	for _, v := range vectors[1:] {
		if len(v) < dim {
			dim = len(v)
		}
	}
	if dim == 0 {
		return nil
	}
	mean := make([]float64, dim)
	for _, v := range vectors {
		for j := 0; j < dim; j++ {
			mean[j] += v[j]
		}
	}
	for j := range mean {
		mean[j] /= float64(len(vectors))
	}
	return mean
}

// computeVacuumFlag is the pure vacuum test extracted from
// RecomputeVacuumStats: a topic is flagged is_vacuum when
// strong/(strong+mid) < vacuumRatio. strong+mid == 0 returns false (no data
// → not a vacuum) to avoid a divide-by-zero and to keep brand-new topics out
// of the vacuum lane until they have attraction stats.
func computeVacuumFlag(strong, mid int, ratio float64) bool {
	total := strong + mid
	if total <= 0 {
		return false
	}
	return float64(strong)/float64(total) < ratio
}

// ComputeTopicCentroid recomputes the centroid for one topic from its recent
// sections' embeddings. It selects the most recent cfg.CentroidWindow sections
// (by report period_date) whose persistent_topic_id = topicID, parses each
// embedding, and takes the equal-weight element-wise mean. When fewer than 2
// sections have a parseable embedding — the spec's "section不足退化首义向量" case
// — the centroid falls back to the topic's existing Embedding (the first-
// section 首义向量). The returned string is a pgvector literal usable in
// UPDATE ... SET centroid = ?.
func (r *TopicGraphRepository) ComputeTopicCentroid(topicID uint) (string, error) {
	cfg := LoadPersistentTopicConfig(r.db)
	window := cfg.CentroidWindow
	if window <= 0 {
		window = DefaultPersistentTopicConfig().CentroidWindow
	}

	type secRow struct {
		Embedding string
	}
	var rows []secRow
	err := r.db.Raw(`
		SELECT s.embedding AS embedding
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		WHERE s.persistent_topic_id = ?
		  AND s.embedding IS NOT NULL
		ORDER BY rpt.period_date DESC, s.id DESC
		LIMIT ?
	`, topicID, window).Scan(&rows).Error
	if err != nil {
		return "", fmt.Errorf("load recent sections for centroid: %w", err)
	}

	var vecs [][]float64
	for _, rw := range rows {
		v, perr := repoParsePgVector(rw.Embedding)
		if perr != nil || len(v) == 0 {
			continue
		}
		vecs = append(vecs, v)
	}
	// Degradation: <2 parseable sections → first-section vector.
	if len(vecs) < 2 {
		return r.topicEmbeddingFallback(topicID)
	}
	mean := meanPgVectors(vecs)
	if mean == nil {
		return r.topicEmbeddingFallback(topicID)
	}
	return FloatsToPgVector(mean), nil
}

// topicEmbeddingFallback loads the topic's existing Embedding column (the
// first-section 首义向量) for use as the centroid when there is not enough
// section data to average. Shared by ComputeTopicCentroid's degradation paths.
func (r *TopicGraphRepository) topicEmbeddingFallback(topicID uint) (string, error) {
	var topic BoardPersistentTopic
	if err := r.db.Select("embedding").First(&topic, topicID).Error; err != nil {
		return "", fmt.Errorf("load topic %d embedding fallback: %w", topicID, err)
	}
	return topic.Embedding, nil
}

// UpdateCentroidOnSectionChange recomputes and persists the centroid for one
// topic. Intended to run after a section is added or its topic assignment
// changes, so the centroid reflects the latest window. tx is the caller's
// transaction (nil falls back to r.db) so the centroid write can commit
// atomically with the triggering section write.
func (r *TopicGraphRepository) UpdateCentroidOnSectionChange(tx *gorm.DB, topicID uint) error {
	centroid, err := r.ComputeTopicCentroid(topicID)
	if err != nil {
		return fmt.Errorf("compute centroid for topic %d: %w", topicID, err)
	}
	exec := r.db
	if tx != nil {
		exec = tx
	}
	if err := exec.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).
		Update("centroid", centroid).Error; err != nil {
		return fmt.Errorf("update topic %d centroid: %w", topicID, err)
	}
	return nil
}

// RecomputeVacuumStats recomputes the vacuum attraction statistics for every
// active/candidate topic on a board and persists them. For each topic it
// counts, over the last cfg.VacuumWindow days, the sections assigned to it:
//
//	strong = topic_match_distance <  LaneL1Threshold
//	mid    = topic_match_distance in [LaneL1Threshold, LaneL2Threshold]
//
// A topic is flagged is_vacuum when strong/(strong+mid) < VacuumRatio. This is
// the LLM-free vacuum proxy the main thread specified (an approximation of the
// spec's "attracted" count via recorded topic_match_distance). Topics with no
// recent attraction are reset to strong=0/mid=0/is_vacuum=false.
func (r *TopicGraphRepository) RecomputeVacuumStats(boardID uint) error {
	cfg := LoadPersistentTopicConfig(r.db)
	vacuumWindow := cfg.VacuumWindow
	if vacuumWindow <= 0 {
		vacuumWindow = DefaultPersistentTopicConfig().VacuumWindow
	}

	type stat struct {
		TopicID      uint
		VacuumStrong int
		VacuumMid    int
	}
	var stats []stat
	err := r.db.Raw(`
		SELECT s.persistent_topic_id AS topic_id,
		       COUNT(*) FILTER (WHERE s.topic_match_distance < ?) AS vacuum_strong,
		       COUNT(*) FILTER (WHERE s.topic_match_distance >= ? AND s.topic_match_distance <= ?) AS vacuum_mid
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		JOIN board_persistent_topics t ON t.id = s.persistent_topic_id
		WHERE t.semantic_board_id = ?
		  AND s.persistent_topic_id IS NOT NULL
		  AND rpt.period_date >= (CURRENT_DATE - ?::int)
		GROUP BY s.persistent_topic_id
	`, cfg.LaneL1Threshold, cfg.LaneL1Threshold, cfg.LaneL2Threshold, boardID, vacuumWindow).Scan(&stats).Error
	if err != nil {
		return fmt.Errorf("aggregate vacuum stats for board %d: %w", boardID, err)
	}

	statMap := make(map[uint]stat, len(stats))
	for _, st := range stats {
		statMap[st.TopicID] = st
	}

	topics, err := r.ListAllTopicsByBoard(boardID)
	if err != nil {
		return fmt.Errorf("list topics for vacuum update: %w", err)
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, t := range topics {
			st := statMap[t.ID] // zero-value when the topic had no recent section
			isVacuum := computeVacuumFlag(st.VacuumStrong, st.VacuumMid, cfg.VacuumRatio)
			if err := tx.Model(&BoardPersistentTopic{}).Where("id = ?", t.ID).
				Updates(map[string]interface{}{
					"vacuum_strong": st.VacuumStrong,
					"vacuum_mid":    st.VacuumMid,
					"is_vacuum":     isVacuum,
				}).Error; err != nil {
				return fmt.Errorf("update vacuum stats for topic %d: %w", t.ID, err)
			}
		}
		return nil
	})
}
