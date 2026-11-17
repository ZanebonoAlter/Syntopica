package repository

import (
	"context"
	"encoding/json"
	"errors"
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
	// CentroidAnchorWeight (persistent_topic_centroid_anchor_weight) is the
	// weight of the topic's 首义向量 in the centroid blend:
	// centroid = α·首义 + (1-α)·滚动窗口均值. α=0 keeps the legacy pure
	// rolling mean, α=1 freezes the topic on its creation-time definition.
	CentroidAnchorWeight float64
	VacuumWindow         int // persistent_topic_vacuum_window: attraction stats span (days)
	L2CandidateK         int // persistent_topic_l2_candidate_k: top-K candidates for L2 LLM
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
	// Intra-lane theme bucketing (add-intra-lane-theme-bucketing): splits one
	// over-wide lane cluster's daily threads into 2-6 theme directions. The
	// thresholds double as a misfire guard — narrative boards (≤5 tags per
	// section) never reach them, so only domain-type lanes get split.
	IntraBucketEnabled    bool // daily_report_intra_bucket_enabled: global kill switch (default on)
	IntraBucketMinThreads int  // daily_report_intra_bucket_min_threads: OR-threshold on the day's thread count
	IntraBucketMinTags    int  // daily_report_intra_bucket_min_tags: OR-threshold on the cluster's tag count
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
		LaneL1Threshold: 0.18,
		LaneL2Threshold: 0.30,
		VacuumRatio:     0.20,
		CentroidWindow:  30,
		// anchor-topic-centroid-with-embedding: 首义与滚动均值各占一半，
		// 话题质心因此只做有界漂移（α=0.5 → 移动幅度 ≤ 纯滚动位移的一半）。
		CentroidAnchorWeight: 0.5,
		VacuumWindow:         7,
		L2CandidateK:         5,
		SectionMergeEnabled:  false,
		// candidate-topic-l2-gate: gate on by default — candidate topics are
		// system guesses and must pass L2 adjudication for every attach.
		CandidateL1GateEnabled: true,
		// Intra-lane theme bucketing defaults (measured: 51/621 sections in the
		// last 14 days reach 6 threads or 10 tags, ~8.2% — domain-type lanes only).
		IntraBucketEnabled:    true,
		IntraBucketMinThreads: 6,
		IntraBucketMinTags:    10,
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
		"persistent_topic_centroid_anchor_weight",
		"persistent_topic_vacuum_window",
		"persistent_topic_l2_candidate_k",
		"daily_report_section_merge_enabled",
		"persistent_topic_candidate_l1_gate_enabled",
		"daily_report_intra_bucket_enabled",
		"daily_report_intra_bucket_min_threads",
		"daily_report_intra_bucket_min_tags",
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
		case "persistent_topic_centroid_anchor_weight":
			if v, err := strconv.ParseFloat(r.Value, 64); err == nil && v >= 0 && v <= 1 {
				cfg.CentroidAnchorWeight = v
			} else {
				logging.Warnf("persistent-topic: invalid centroid anchor weight %q; using default %v", r.Value, cfg.CentroidAnchorWeight)
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
		case "daily_report_intra_bucket_enabled":
			if v, err := strconv.ParseBool(r.Value); err == nil {
				cfg.IntraBucketEnabled = v
			} else {
				logging.Warnf("daily-report: invalid intra bucket enabled %q; using default %t", r.Value, cfg.IntraBucketEnabled)
			}
		case "daily_report_intra_bucket_min_threads":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.IntraBucketMinThreads = v
			} else {
				logging.Warnf("daily-report: invalid intra bucket min threads %q; using default %d", r.Value, cfg.IntraBucketMinThreads)
			}
		case "daily_report_intra_bucket_min_tags":
			if v, err := strconv.Atoi(r.Value); err == nil && v > 0 {
				cfg.IntraBucketMinTags = v
			} else {
				logging.Warnf("daily-report: invalid intra bucket min tags %q; using default %d", r.Value, cfg.IntraBucketMinTags)
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

// ListTagIdentityEmbeddings loads the identity-track pgvector embedding for
// each requested tag from topic_tag_embeddings (embedding_type='identity').
// The daily-report dedup tiebreak (fix-digest-tag-leak) uses these to keep
// the tag semantically closer to the reporting board when two tags tie on
// article_count. Tags without an identity row are absent from the returned
// map; the caller falls back to the ID tiebreak for those duels.
func (r *TopicGraphRepository) ListTagIdentityEmbeddings(tagIDs []uint) (map[uint]string, error) {
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
		WHERE embedding_type = 'identity' AND topic_tag_id IN ?
	`, tagIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list tag identity embeddings: %w", err)
	}
	for _, rw := range rows {
		if rw.Embedding != "" {
			out[rw.TopicTagID] = rw.Embedding
		}
	}
	return out, nil
}

// GetBoardEmbedding returns the pgvector embedding string of an active board
// label (semantic_labels WHERE label_type='board'). Empty string when the
// board is missing, inactive, or carries no embedding — callers degrade to
// the ID tiebreak.
func (r *TopicGraphRepository) GetBoardEmbedding(boardID uint) (string, error) {
	type row struct {
		Embedding *string
	}
	var rw row
	err := r.db.Raw(`
		SELECT embedding
		FROM semantic_labels
		WHERE id = ? AND label_type = 'board' AND status = 'active' AND embedding IS NOT NULL
	`, boardID).Scan(&rw).Error
	if err != nil {
		return "", fmt.Errorf("get board embedding: %w", err)
	}
	if rw.Embedding == nil {
		return "", nil
	}
	return *rw.Embedding, nil
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

// blendCentroidWithAnchor mixes the rolling window mean with the topic's
// 首义向量 (creation-time definition embedding):
//
//	centroid = α·anchor + (1-α)·mean
//
// The blend is element-wise. No L2 normalisation is applied: matching is
// cosine-based (direction only), so the magnitude of the mixed vector is
// irrelevant and normalising would only add a failure mode.
//
// It returns false when the blend is impossible — empty mean, empty anchor
// (NULL / unparseable 首义) or a dimension mismatch (e.g. the 首义 was written
// by an older embedding model) — so the caller can fall back to the pure
// rolling mean instead of persisting a broken centroid.
//
// Extracted as a pure helper so the blend rule is unit-testable without a
// database (acceptance rail for spec scenario 「质心按近期 section 计算」).
func blendCentroidWithAnchor(mean, anchor []float64, alpha float64) ([]float64, bool) {
	if len(mean) == 0 || len(anchor) == 0 {
		return nil, false
	}
	if len(mean) != len(anchor) {
		return nil, false
	}
	blended := make([]float64, len(mean))
	for i := range mean {
		blended[i] = alpha*anchor[i] + (1-alpha)*mean[i]
	}
	return blended, true
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
// sections' embeddings, blended with the topic's 首义向量:
//
//	centroid = α·首义 embedding + (1-α)·最近 centroid_window 条命中 section embedding 等权均值
//
// α = persistent_topic_centroid_anchor_weight (default 0.5). It selects the
// most recent cfg.CentroidWindow sections (by report period_date) whose
// persistent_topic_id = topicID, parses each embedding, and takes the
// equal-weight element-wise mean; the 首义 (the topic's creation-time
// embedding) then pulls the result back so the anchor drifts only in a
// bounded way (a wide topic can no longer turn into a "news magnet").
//
// When fewer than 2 sections have a parseable embedding — the spec's
// "section不足退化首义向量" case — the centroid falls back to the topic's
// existing Embedding (the first-section 首义向量). When the 首义 is
// unavailable (NULL / unparseable) or its dimension differs from the rolling
// mean's, the centroid falls back to the pure rolling mean and a warning is
// logged — an unusable anchor never blocks a refresh.
//
// The returned string is a pgvector literal usable in
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
	// 首义锚定混合：滚动均值 + α·首义。首义不可用（空 / 解析失败 / 维度不符）
	// → 回退纯滚动均值并告警，不阻塞质心刷新。
	anchor, err := r.topicEmbeddingVector(topicID)
	if err != nil {
		return "", err
	}
	blended, ok := blendCentroidWithAnchor(mean, anchor, cfg.CentroidAnchorWeight)
	if !ok {
		logging.Warnf("persistent-topic: topic %d centroid anchor unusable (anchor_dim=%d, mean_dim=%d); using pure rolling mean",
			topicID, len(anchor), len(mean))
		return FloatsToPgVector(mean), nil
	}
	return FloatsToPgVector(blended), nil
}

// topicEmbeddingVector loads the topic's 首义向量 (the embedding column) and
// parses it into a float slice. An empty or unparseable column yields an empty
// slice with a nil error: the anchor is then unusable and the caller degrades
// to the pure rolling mean (an unusable anchor must never fail a refresh).
func (r *TopicGraphRepository) topicEmbeddingVector(topicID uint) ([]float64, error) {
	var topic BoardPersistentTopic
	if err := r.db.Select("embedding").First(&topic, topicID).Error; err != nil {
		return nil, fmt.Errorf("load topic %d embedding: %w", topicID, err)
	}
	if strings.TrimSpace(topic.Embedding) == "" {
		return nil, nil
	}
	v, err := repoParsePgVector(topic.Embedding)
	if err != nil {
		return nil, nil
	}
	return v, nil
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

// ListTopicsForCentroidRecompute returns the topics taking part in a one-off
// centroid recompute: every non-archived topic (candidate + active),
// optionally scoped to one board. Archived topics are excluded — they are not
// anchorable, so their centroid is never consumed by matching.
func (r *TopicGraphRepository) ListTopicsForCentroidRecompute(ctx context.Context, boardID *uint) ([]BoardPersistentTopic, error) {
	q := r.db.WithContext(ctx).Where("status != ?", TopicStatusArchived)
	if boardID != nil {
		q = q.Where("semantic_board_id = ?", *boardID)
	}
	var topics []BoardPersistentTopic
	if err := q.Order("id ASC").Find(&topics).Error; err != nil {
		return nil, fmt.Errorf("list topics for centroid recompute: %w", err)
	}
	return topics, nil
}

// RecomputeTopicCentroids recomputes the centroid of every non-archived topic
// (optionally scoped to one board) with the current formula and config. It is
// the storage side of POST /api/daily-reports/recompute-topic-centroids — the
// one-off correction after the centroid formula changed.
//
// Per-topic failures are logged and skipped: one broken topic must not abort
// the rest of the run. Returns the number of topics updated and the number
// that failed.
//
// Watch-materialized topics (sentence_topic) keep their seeded centroid: their
// sections carry no embedding, so the <2-section degradation path writes the
// same query-vector anchor back.
func (r *TopicGraphRepository) RecomputeTopicCentroids(ctx context.Context, boardID *uint) (updated, failed int, err error) {
	topics, err := r.ListTopicsForCentroidRecompute(ctx, boardID)
	if err != nil {
		return 0, 0, err
	}
	for _, topic := range topics {
		if cerr := r.UpdateCentroidOnSectionChange(nil, topic.ID); cerr != nil {
			failed++
			logging.Errorf("persistent-topic: recompute centroid for topic %d failed: %v", topic.ID, cerr)
			continue
		}
		updated++
	}
	return updated, failed, nil
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

// ── 人工重设首义（lane-manual-correction D3） ──────────────────────────────

// TopicAnchorSourceSelectedSections is the only accepted input mode for a
// manual 首义 rewrite: the user picks a range of lane nodes and the server
// aggregates their embeddings (D3). Free text is NOT an input mode — a
// natural-language description is rejected, never vectorized.
const TopicAnchorSourceSelectedSections = "selected_sections"

// AnchorEffectiveFromNextReport is the effective-time marker returned to the
// client. The rewrite lands in the fact layer immediately, but the centroid
// (and therefore the matching behaviour) only moves on the next daily report
// (D1: there is no "recompute now" endpoint).
const AnchorEffectiveFromNextReport = "next_report"

// AnchorOperatorUser is the operator recorded for manual corrections. The
// deployment is single-user without auth (AGENTS.md), so every manual
// correction is attributed to the local user.
const AnchorOperatorUser = "user"

var (
	// ErrTopicNotFound means the addressed topic does not exist.
	ErrTopicNotFound = errors.New("topic not found")
	// ErrTopicArchived means the addressed topic is archived — archived lanes
	// are not anchorable.
	ErrTopicArchived = errors.New("topic is archived")
)

// AnchorSelectionError explains why a chosen node range was rejected. It
// carries the offending section id so the UI can point at the node the user
// has to fix — silently skipping a node would make "N nodes selected" a lie.
type AnchorSelectionError struct {
	SectionID uint
	// Reason is a stable machine-readable code: section_not_found /
	// other_board / not_in_topic / no_embedding / dim_mismatch.
	Reason  string
	Message string
}

func (e *AnchorSelectionError) Error() string { return e.Message }

// AnchorFromSectionsResult is the outcome of a manual 首义 rewrite.
type AnchorFromSectionsResult struct {
	TopicID uint `json:"topic_id"`
	// Source is the input mode ("selected_sections").
	Source string `json:"source"`
	// SectionIDs are the chosen nodes (deduplicated).
	SectionIDs []uint `json:"selected_sections"`
	// OverrideCount is how many times the topic's 首义 has been rewritten by
	// hand (the number of audit rows).
	OverrideCount int `json:"override_count"`
	// EffectiveFrom marks when the change takes effect on the matching side.
	EffectiveFrom string `json:"effective_from"`
}

// SetTopicAnchorFromSections rewrites a topic's 首义 vector from a chosen
// range of lane nodes (lane-manual-correction D3 / task 2.1).
//
// One transaction, three writes (design 图 4):
//  1. board_persistent_topics.embedding ← mean pooling of the chosen
//     sections' embeddings (the same 口径 as CreateManualTopic);
//  2. topic_anchor_overrides ← one appended audit row (previous vector, new
//     vector, input mode, chosen section ids, operator, timestamp);
//  3. board_persistent_topics.centroid_pending_recompute ← true.
//
// The centroid column is NOT touched: recomputing it belongs to the next
// daily-report refresh round (D1 — there is no "recompute now" path).
//
// Validation is all-or-nothing: every chosen section must belong to this
// topic, sit on the same board and carry a parseable embedding of a
// consistent dimension. One bad node rejects the whole write — silently
// skipping it would make the reported node count a lie.
func (r *TopicGraphRepository) SetTopicAnchorFromSections(topicID uint, sectionIDs []uint, operator string) (*AnchorFromSectionsResult, error) {
	unique := dedupeSectionIDs(sectionIDs)
	if len(unique) == 0 {
		return nil, fmt.Errorf("SetTopicAnchorFromSections: no sections provided")
	}

	var topic BoardPersistentTopic
	if err := r.db.Select("id", "semantic_board_id", "embedding", "status").First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopicNotFound
		}
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}
	if topic.Status == TopicStatusArchived {
		return nil, ErrTopicArchived
	}

	vectors, err := r.loadAnchorSectionVectors(topicID, topic.SemanticBoardID, unique)
	if err != nil {
		return nil, err
	}

	// Mean pooling — the same 口径 as CreateManualTopic (aggregateEmbeddings).
	mean, skipped := aggregateEmbeddings(vectors)
	if len(mean) == 0 || skipped > 0 {
		// aggregateEmbeddings skips unusable vectors; the validation above
		// already rejected every one of them, so a skip here means the input
		// changed under us — refuse rather than write a partial aggregate.
		return nil, fmt.Errorf("SetTopicAnchorFromSections: selected sections did not aggregate cleanly")
	}
	newEmbedding := FloatsToPgVector(mean)
	oldEmbedding := strings.TrimSpace(topic.Embedding)

	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).Updates(map[string]interface{}{
			"embedding":                  newEmbedding,
			"centroid_pending_recompute": true,
			"updated_at":                 time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("write topic %d anchor: %w", topicID, err)
		}
		override := TopicAnchorOverride{
			TopicID:      topicID,
			NewEmbedding: newEmbedding,
			Source:       TopicAnchorSourceSelectedSections,
			InputSummary: anchorInputSummary(unique),
			Operator:     operator,
		}
		if oldEmbedding != "" {
			override.OldEmbedding = &oldEmbedding
		}
		if err := tx.Create(&override).Error; err != nil {
			return fmt.Errorf("append anchor override for topic %d: %w", topicID, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var overrideCount int64
	if err := r.db.Model(&TopicAnchorOverride{}).Where("topic_id = ?", topicID).Count(&overrideCount).Error; err != nil {
		return nil, fmt.Errorf("count anchor overrides for topic %d: %w", topicID, err)
	}

	return &AnchorFromSectionsResult{
		TopicID:       topicID,
		Source:        TopicAnchorSourceSelectedSections,
		SectionIDs:    unique,
		OverrideCount: int(overrideCount),
		EffectiveFrom: AnchorEffectiveFromNextReport,
	}, nil
}

// loadAnchorSectionVectors loads the chosen sections and turns them into
// vectors, rejecting the whole set as soon as one node breaks a rule
// (cross-topic / cross-board / missing vector / mismatched dimension).
func (r *TopicGraphRepository) loadAnchorSectionVectors(topicID, boardID uint, sectionIDs []uint) ([][]float64, error) {
	type secRow struct {
		ID                uint
		Embedding         string
		PersistentTopicID *uint
		BoardID           uint
	}
	var rows []secRow
	if err := r.db.Raw(`
		SELECT s.id AS id,
		       s.embedding AS embedding,
		       s.persistent_topic_id AS persistent_topic_id,
		       rpt.semantic_board_id AS board_id
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		WHERE s.id IN ?
	`, sectionIDs).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load anchor sections for topic %d: %w", topicID, err)
	}
	byID := make(map[uint]secRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	vectors := make([][]float64, 0, len(sectionIDs))
	dim := 0
	for _, id := range sectionIDs {
		row, ok := byID[id]
		if !ok {
			return nil, &AnchorSelectionError{
				SectionID: id,
				Reason:    "section_not_found",
				Message:   fmt.Sprintf("节点 %d 不存在", id),
			}
		}
		if row.BoardID != boardID {
			return nil, &AnchorSelectionError{
				SectionID: id,
				Reason:    "other_board",
				Message:   fmt.Sprintf("节点 %d 不属于当前版块", id),
			}
		}
		if row.PersistentTopicID == nil || *row.PersistentTopicID != topicID {
			return nil, &AnchorSelectionError{
				SectionID: id,
				Reason:    "not_in_topic",
				Message:   fmt.Sprintf("节点 %d 不属于该话题", id),
			}
		}
		vec, perr := repoParsePgVector(row.Embedding)
		if perr != nil || len(vec) == 0 {
			return nil, &AnchorSelectionError{
				SectionID: id,
				Reason:    "no_embedding",
				Message:   fmt.Sprintf("节点 %d 没有可用向量", id),
			}
		}
		if dim == 0 {
			dim = len(vec)
		} else if len(vec) != dim {
			return nil, &AnchorSelectionError{
				SectionID: id,
				Reason:    "dim_mismatch",
				Message:   fmt.Sprintf("节点 %d 的向量维度与其它节点不一致", id),
			}
		}
		vectors = append(vectors, vec)
	}
	return vectors, nil
}

// dedupeSectionIDs removes duplicates while keeping the caller's order.
func dedupeSectionIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// anchorInputSummary renders the chosen section ids as a JSON array — the
// audit trail's「输入摘要」(what the user actually picked).
func anchorInputSummary(ids []uint) string {
	if len(ids) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return ""
	}
	return string(raw)
}

// ── 质心刷新范围：touched ∪ 待重算（lane-manual-correction D2） ───────────

// ListPendingCentroidTopicIDs returns the board's topics whose centroid is
// flagged for a forced recompute. These topics join the refresh round even
// when the day's report never touched them — that is the only path on which
// a manual correction on a lane without new hits can ever take effect.
func (r *TopicGraphRepository) ListPendingCentroidTopicIDs(boardID uint) ([]uint, error) {
	var ids []uint
	if err := r.db.Model(&BoardPersistentTopic{}).
		Where("semantic_board_id = ? AND centroid_pending_recompute = ?", boardID, true).
		Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list pending centroid topics for board %d: %w", boardID, err)
	}
	return ids, nil
}

// centroidRefreshTargets merges the topics the day's report touched with the
// board's topics flagged for a forced recompute (D2): the refresh scope is
// 「当日 touched 话题 ∪ centroid_pending_recompute=true 的话题」.
func (r *TopicGraphRepository) centroidRefreshTargets(boardID uint, touched []uint) ([]uint, error) {
	pending, err := r.ListPendingCentroidTopicIDs(boardID)
	if err != nil {
		return nil, err
	}
	seen := make(map[uint]struct{}, len(touched)+len(pending))
	ids := make([]uint, 0, len(touched)+len(pending))
	for _, id := range touched {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, id := range pending {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// RefreshBoardCentroids recomputes the centroids of the topics the day's
// report touched plus every topic flagged centroid_pending_recompute on the
// board (D2), then clears the flag of the topics it refreshed successfully.
//
// Per-topic isolation holds: a topic whose refresh fails keeps its flag (the
// next round retries it) and does not abort the remaining topics.
//
// The flag is cleared **conditionally** (review R-11): the trail version read
// before the recompute is passed to the clear, so a manual correction that
// lands *during* the recompute (and therefore re-arms the flag) is not wiped
// by this round's cleanup.
func (r *TopicGraphRepository) RefreshBoardCentroids(boardID uint, touched []uint) (updated, failed int) {
	ids, err := r.centroidRefreshTargets(boardID, touched)
	if err != nil {
		logging.Errorf("persistent-topic: collect centroid refresh targets for board %d: %v", boardID, err)
		return 0, 0
	}
	return refreshCentroidsIsolated(ids,
		func(topicID uint) (int64, error) { return r.correctionTrailVersion(nil, topicID) },
		func(topicID uint) error { return r.UpdateCentroidOnSectionChange(nil, topicID) },
		func(topicID uint, version int64) error { return r.clearCentroidPendingFlag(nil, topicID, version) },
	)
}

// refreshCentroidsIsolated runs the per-topic centroid refresh with per-topic
// isolation: a failing topic keeps its pending flag and does not abort the
// rest (D2 / spec「重算失败保留标记、不中断其余话题」). Kept as a pure
// function over the callbacks so the failure path is testable without a
// broken database.
//
// The three callbacks are, per topic:
//
//	snapshot    — read the correction-trail version BEFORE the recompute;
//	refresh     — recompute the centroid;
//	clearPending— clear the pending flag, but only if the trail version is
//	              unchanged since the snapshot (review R-11: a correction that
//	              lands during the recompute re-arms the flag and must not be
//	              wiped by this round's cleanup).
//
// A failing snapshot means the trail state is unknown → the flag is kept
// (safe direction: the next round recomputes again, which is idempotent).
func refreshCentroidsIsolated(ids []uint, snapshot func(uint) (int64, error), refresh func(uint) error, clearPending func(uint, int64) error) (updated, failed int) {
	for _, id := range ids {
		version, err := snapshot(id)
		if err != nil {
			logging.Errorf("persistent-topic: read correction trail version for topic %d: %v", id, err)
		}
		if err := refresh(id); err != nil {
			failed++
			logging.Errorf("persistent-topic: centroid refresh failed for topic %d: %v", id, err)
			continue
		}
		updated++
		if err != nil {
			// Unknown trail state → keep the flag (safe direction).
			continue
		}
		if err := clearPending(id, version); err != nil {
			// The centroid itself was written; only the flag write failed.
			// Leaving the flag set is the safe direction — the next round
			// recomputes again (idempotent).
			logging.Errorf("persistent-topic: clear centroid pending flag for topic %d: %v", id, err)
		}
	}
	return updated, failed
}

// correctionTrailVersion counts the append-only audit rows that mention the
// topic: 首义改写留痕 + 泳道归属变更留痕.
//
// Every manual correction appends exactly one row **in the same transaction**
// that sets centroid_pending_recompute, so a change of this number between
// the pre-recompute read and the clear means "a new correction landed" → the
// pending flag must survive (review R-11).
func (r *TopicGraphRepository) correctionTrailVersion(tx *gorm.DB, topicID uint) (int64, error) {
	exec := r.db
	if tx != nil {
		exec = tx
	}
	var version int64
	if err := exec.Raw(`
		SELECT
			(SELECT count(*) FROM topic_anchor_overrides WHERE topic_id = ?) +
			(SELECT count(*) FROM section_assignment_overrides WHERE topic_id_from = ? OR topic_id_to = ?) AS version
	`, topicID, topicID, topicID).Scan(&version).Error; err != nil {
		return 0, fmt.Errorf("read correction trail version for topic %d: %w", topicID, err)
	}
	return version, nil
}

// clearCentroidPendingFlag clears the forced-recompute flag after a
// successful centroid refresh (D2: 重算成功后清标记；失败保留待下次重试).
//
// The clear is **conditional** (review R-11): the flag is only cleared when
// the correction trail still holds the same number of rows it held when the
// recompute started. A manual correction that landed during the recompute
// appended a row (and re-armed the flag) → the count differs → the flag is
// kept, so that correction's centroid refresh is not silently skipped.
func (r *TopicGraphRepository) clearCentroidPendingFlag(tx *gorm.DB, topicID uint, version int64) error {
	exec := r.db
	if tx != nil {
		exec = tx
	}
	if err := exec.Exec(`
		UPDATE board_persistent_topics
		SET centroid_pending_recompute = false
		WHERE id = ?
		  AND centroid_pending_recompute = true
		  AND (
			(SELECT count(*) FROM topic_anchor_overrides WHERE topic_id = ?) +
			(SELECT count(*) FROM section_assignment_overrides WHERE topic_id_from = ? OR topic_id_to = ?)
		  ) = ?
	`, topicID, topicID, topicID, topicID, version).Error; err != nil {
		return fmt.Errorf("clear centroid pending flag for topic %d: %w", topicID, err)
	}
	return nil
}

// ── 人工归属变更（lane-manual-correction D4/D5/D6） ────────────────────────
// 归属变更是双向的：移除（解除归属）与加回，共用一个端点、同一套快照列口径。
// 事实层只有人工可写的几列；质心/命中计数/状态一律不动（D1/D2）。

// Manual assignment actions recorded in the section's audit columns.
const (
	// ManualAssignmentActionDetach — the node was removed from its lane.
	ManualAssignmentActionDetach = "detach"
	// ManualAssignmentActionAttach — the node was attached to a lane.
	ManualAssignmentActionAttach = "attach"
)

var (
	// ErrSectionNotFound means the addressed section does not exist.
	ErrSectionNotFound = errors.New("section not found")
	// ErrSectionAlreadyUnassigned means the section has no lane to leave —
	// there is nothing to detach.
	ErrSectionAlreadyUnassigned = errors.New("section has no topic assignment")
	// ErrSectionAlreadyAssigned means the section already belongs to another
	// lane that is NOT eligible for a 改挂. 改挂 from a **candidate**（动态）
	// lane is allowed (its lane has not been promoted yet, so the material is
	// still fluid); 改挂 from an **active** lane is deliberately not
	// supported: the user expresses 「它不该在这儿」, not 「它该在那儿」 (D4).
	ErrSectionAlreadyAssigned = errors.New("section already has a topic assignment")
	// ErrReassignFromActiveLane means the section belongs to an active lane —
	// moving it out of a promoted lane is not supported.
	ErrReassignFromActiveLane = errors.New("section belongs to an active lane; 改挂 is not supported")
	// ErrTopicOnOtherBoard means the attach target lives on another board.
	ErrTopicOnOtherBoard = errors.New("topic belongs to another board")
)

// AssignmentChangeResult is the outcome of a manual lane-membership change.
type AssignmentChangeResult struct {
	SectionID uint `json:"section_id"`
	// TopicID is the section's lane after the change; nil = 游离节点（不归属
	// 任何泳道）.
	TopicID *uint `json:"topic_id"`
	// PreviousTopicID is the lane the section belonged to before the change.
	PreviousTopicID *uint `json:"previous_topic_id"`
	// Action is "detach" or "attach".
	Action string `json:"action"`
	// Changed reports whether any row was written. The idempotent attach
	// (the node already belongs to the target lane) writes nothing and
	// appends no audit row.
	Changed bool `json:"changed"`
	// Distance is the recomputed distance to the target anchor (attach only;
	// nil when the node has no usable vector).
	Distance *float64 `json:"topic_match_distance,omitempty"`
	// BoardID is the board whose relation graph was rebuilt.
	BoardID uint `json:"board_id"`
}

// BatchDetachResult is the outcome of a per-day batch detach.
type BatchDetachResult struct {
	TopicID uint `json:"topic_id"`
	// PeriodDate is the released day ("YYYY-MM-DD").
	PeriodDate string `json:"period_date"`
	// Detached is how many nodes were released.
	Detached int `json:"detached"`
	// SectionIDs are the released nodes.
	SectionIDs []uint `json:"section_ids"`
	// BoardID is the board whose relation graph was rebuilt.
	BoardID uint `json:"board_id"`
}

// SetSectionTopicAssignment changes one node's lane membership
// (lane-manual-correction D4/D5, tasks 3.1/3.3/3.5/3.6/3.7).
//
// targetTopicID == nil → 移除（解除归属）: the four snapshot columns
// (persistent_topic_id / topic_match_distance / topic_match_confidence /
// topic_status_at_report) are set to NULL, lane_tier keeps its value and the
// audit columns record action=detach with the topic it came from.
//
// targetTopicID != nil → 加回: the node must currently be unassigned (改挂 is
// rejected), the target must live on the same board and must not be archived.
// persistent_topic_id becomes the target, topic_match_confidence becomes
// "manual", topic_match_distance is recomputed against the target's anchor
// (centroid first, embedding fallback) and topic_status_at_report is NULL.
//
// Both directions run in ONE transaction that ends with RebuildBoardRelations
// (identity edges are derived from persistent_topic_id, so a membership change
// necessarily changes edges) and flags the affected topic with
// centroid_pending_recompute — the centroid itself is NOT recomputed here
// (D1/D2: 质心只由下一次日报生成统一重算).
//
// The topic's hit_count / consecutive_hits / status / first_seen_date /
// last_seen_date are never touched: hit counters belong to the algorithm
// (flow/topic-graph 红线 1) and archiving is a user-explicit operation only.
func (r *TopicGraphRepository) SetSectionTopicAssignment(sectionID uint, targetTopicID *uint) (*AssignmentChangeResult, error) {
	row, err := r.loadAssignmentRow(sectionID)
	if err != nil {
		return nil, err
	}

	if targetTopicID == nil {
		return r.detachSection(row)
	}
	return r.attachSection(row, *targetTopicID)
}

// assignmentRow is the slice of the section row a membership change needs:
// the vector (for the recomputed distance), the current lane and the board
// the node lives on.
type assignmentRow struct {
	ID                uint
	Embedding         string
	PersistentTopicID *uint
	BoardID           uint
}

// loadAssignmentRow loads the section together with its board. The board is
// derived from the report the section belongs to — the only place a section
// learns which board it is on.
func (r *TopicGraphRepository) loadAssignmentRow(sectionID uint) (*assignmentRow, error) {
	var row assignmentRow
	result := r.db.Raw(`
		SELECT s.id AS id,
		       s.embedding AS embedding,
		       s.persistent_topic_id AS persistent_topic_id,
		       rpt.semantic_board_id AS board_id
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		WHERE s.id = ?
	`, sectionID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("load section %d: %w", sectionID, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, ErrSectionNotFound
	}
	return &row, nil
}

// detachSection releases a node from its lane (移除). Only the four snapshot
// columns and the audit columns are written; lane_tier and the content stay.
//
// The append-only trail (section_assignment_overrides) gets one row in the
// same transaction — the section's own trace columns only keep the latest
// change (review R-2: 「移除 → 加回」 used to erase the detach trail).
func (r *TopicGraphRepository) detachSection(row *assignmentRow) (*AssignmentChangeResult, error) {
	if row.PersistentTopicID == nil {
		return nil, ErrSectionAlreadyUnassigned
	}
	previous := *row.PersistentTopicID
	now := time.Now()

	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&DailyReportSection{}).Where("id = ?", row.ID).Updates(map[string]interface{}{
			"persistent_topic_id":             nil,
			"topic_match_distance":            nil,
			"topic_match_confidence":          nil,
			"topic_status_at_report":          nil,
			"manual_assignment_at":            now,
			"manual_assignment_from_topic_id": previous,
			"manual_assignment_action":        ManualAssignmentActionDetach,
		}).Error; err != nil {
			return fmt.Errorf("detach section %d: %w", row.ID, err)
		}
		if err := r.appendAssignmentOverride(tx, row.ID, &previous, nil, ManualAssignmentActionDetach); err != nil {
			return err
		}
		// The lane that lost the node must be recomputed on the next daily
		// report — its rolling window no longer contains the node.
		if err := r.markTopicCentroidPending(tx, previous); err != nil {
			return err
		}
		if err := r.rebuildRelations(tx, row.BoardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &AssignmentChangeResult{
		SectionID:       row.ID,
		TopicID:         nil,
		PreviousTopicID: &previous,
		Action:          ManualAssignmentActionDetach,
		Changed:         true,
		BoardID:         row.BoardID,
	}, nil
}

// loadTopicForReassign loads the lane a node currently belongs to, so the
// 改挂 guard can look at its status (candidate = movable, active = not).
func (r *TopicGraphRepository) loadTopicForReassign(topicID uint) (*BoardPersistentTopic, error) {
	var topic BoardPersistentTopic
	if err := r.db.First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopicNotFound
		}
		return nil, fmt.Errorf("load topic %d for reassign: %w", topicID, err)
	}
	return &topic, nil
}

// reassignSection moves a node from a candidate（动态）lane into the target
// lane (改挂). The source lane has not been promoted yet, so the material is
// still fluid; moving a node out of an active lane is rejected upstream.
//
// Both lanes' rolling windows change → both get the pending-recompute mark,
// and the assignment override is appended with the source lane recorded.
func (r *TopicGraphRepository) reassignSection(
	row *assignmentRow,
	target BoardPersistentTopic,
	source *BoardPersistentTopic,
) (*AssignmentChangeResult, error) {
	// 实算距离：section embedding → 目标锚点（centroid 优先、embedding 兜底）。
	// 节点没有可用向量时距离无从算起 → 写 NULL（不伪造数字）。
	var distance *float64
	if vec, perr := repoParsePgVector(row.Embedding); perr == nil && len(vec) > 0 {
		if anchor := topicAnchorVec(target); len(anchor) > 0 {
			d := cosineDistance(vec, anchor)
			distance = &d
		}
	}

	now := time.Now()
	updates := map[string]interface{}{
		"persistent_topic_id":             target.ID,
		"topic_match_confidence":          TopicConfManual,
		"topic_status_at_report":          nil,
		"manual_assignment_at":            now,
		"manual_assignment_from_topic_id": &source.ID,
		"manual_assignment_action":        ManualAssignmentActionAttach,
	}
	if distance != nil {
		updates["topic_match_distance"] = *distance
	} else {
		updates["topic_match_distance"] = nil
	}

	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&DailyReportSection{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("reassign section %d: %w", row.ID, err)
		}
		if err := r.appendAssignmentOverride(tx, row.ID, &source.ID, &target.ID, ManualAssignmentActionAttach); err != nil {
			return err
		}
		// Both lanes' rolling windows changed → both get the pending mark.
		if err := r.markTopicCentroidPending(tx, target.ID); err != nil {
			return err
		}
		if err := r.markTopicCentroidPending(tx, source.ID); err != nil {
			return err
		}
		if err := r.rebuildRelations(tx, row.BoardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &AssignmentChangeResult{
		SectionID:       row.ID,
		TopicID:         &target.ID,
		PreviousTopicID: &source.ID,
		Action:          ManualAssignmentActionAttach,
		Changed:         true,
		BoardID:         row.BoardID,
	}, nil
}

// attachSection puts a currently unassigned node back into a lane (加回).
// 改挂 from an active lane is rejected: the node must be removed first, then
// re-attached. 改挂 from a candidate（动态）lane IS allowed — see
// reassignSection.
func (r *TopicGraphRepository) attachSection(row *assignmentRow, targetTopicID uint) (*AssignmentChangeResult, error) {
	var topic BoardPersistentTopic
	if err := r.db.First(&topic, targetTopicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopicNotFound
		}
		return nil, fmt.Errorf("load topic %d: %w", targetTopicID, err)
	}
	if topic.SemanticBoardID != row.BoardID {
		return nil, ErrTopicOnOtherBoard
	}
	if topic.Status == TopicStatusArchived {
		return nil, ErrTopicArchived
	}
	if row.PersistentTopicID != nil {
		if *row.PersistentTopicID == topic.ID {
			// 幂等：节点已属目标泳道 → 不写任何列、不追加留痕。
			return &AssignmentChangeResult{
				SectionID:       row.ID,
				TopicID:         &topic.ID,
				PreviousTopicID: &topic.ID,
				Action:          ManualAssignmentActionAttach,
				Changed:         false,
				BoardID:         row.BoardID,
			}, nil
		}
		// 改挂：节点已属另一泳道。只有对方泳道是 **candidate（动态）** 时
		// 允许挪走（尚未转正，素材仍在流动）；对方是 active（已转正）则拒绝
		// （D4 的「SHALL NOT 改挂到 active 泳道」）。
		previous, err := r.loadTopicForReassign(*row.PersistentTopicID)
		if err != nil {
			return nil, err
		}
		if previous.Status != TopicStatusCandidate {
			return nil, ErrReassignFromActiveLane
		}
		return r.reassignSection(row, topic, previous)
	}

	// 实算距离：section embedding → 目标锚点（centroid 优先、embedding 兜底）。
	// 节点没有可用向量时距离无从算起 → 写 NULL（与移除方向同义：无可比距离）。
	var distance *float64
	if vec, perr := repoParsePgVector(row.Embedding); perr == nil && len(vec) > 0 {
		if anchor := topicAnchorVec(topic); len(anchor) > 0 {
			d := cosineDistance(vec, anchor)
			distance = &d
		}
	}

	now := time.Now()
	updates := map[string]interface{}{
		"persistent_topic_id":             topic.ID,
		"topic_match_confidence":          TopicConfManual,
		"topic_status_at_report":          nil,
		"manual_assignment_at":            now,
		"manual_assignment_from_topic_id": nil,
		"manual_assignment_action":        ManualAssignmentActionAttach,
	}
	if distance != nil {
		updates["topic_match_distance"] = *distance
	} else {
		updates["topic_match_distance"] = nil
	}

	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&DailyReportSection{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("attach section %d: %w", row.ID, err)
		}
		if err := r.appendAssignmentOverride(tx, row.ID, nil, &topic.ID, ManualAssignmentActionAttach); err != nil {
			return err
		}
		// The target lane's rolling window now contains the node again.
		if err := r.markTopicCentroidPending(tx, topic.ID); err != nil {
			return err
		}
		if err := r.rebuildRelations(tx, row.BoardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &AssignmentChangeResult{
		SectionID:       row.ID,
		TopicID:         &topic.ID,
		PreviousTopicID: nil,
		Action:          ManualAssignmentActionAttach,
		Changed:         true,
		Distance:        distance,
		BoardID:         row.BoardID,
	}, nil
}

// DetachTopicSectionsByDate releases every node of one lane on one day
// (lane-manual-correction「批量移除某一天」, task 3.2/3.3/3.5).
//
// Each released node follows the single-detach口径 (four snapshot columns
// NULL, lane_tier preserved, audit action=detach); the whole batch runs in one
// transaction that ends with a single RebuildBoardRelations.
func (r *TopicGraphRepository) DetachTopicSectionsByDate(topicID uint, periodDate time.Time) (*BatchDetachResult, error) {
	var topic BoardPersistentTopic
	if err := r.db.First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopicNotFound
		}
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}
	day := NormalizeReportDate(periodDate).Format("2006-01-02")

	var ids []uint
	if err := r.db.Raw(`
		SELECT s.id
		FROM daily_report_sections s
		JOIN board_daily_reports rpt ON rpt.id = s.report_id
		WHERE s.persistent_topic_id = ?
		  AND rpt.semantic_board_id = ?
		  AND rpt.period_date = ?
		ORDER BY s.id ASC
	`, topicID, topic.SemanticBoardID, day).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("list nodes of topic %d on %s: %w", topicID, day, err)
	}

	now := time.Now()
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			if err := tx.Model(&DailyReportSection{}).Where("id = ?", id).Updates(map[string]interface{}{
				"persistent_topic_id":             nil,
				"topic_match_distance":            nil,
				"topic_match_confidence":          nil,
				"topic_status_at_report":          nil,
				"manual_assignment_at":            now,
				"manual_assignment_from_topic_id": topicID,
				"manual_assignment_action":        ManualAssignmentActionDetach,
			}).Error; err != nil {
				return fmt.Errorf("detach section %d: %w", id, err)
			}
			if err := r.appendAssignmentOverride(tx, id, &topicID, nil, ManualAssignmentActionDetach); err != nil {
				return err
			}
		}
		if len(ids) == 0 {
			return nil
		}
		if err := r.markTopicCentroidPending(tx, topicID); err != nil {
			return err
		}
		if err := r.rebuildRelations(tx, topic.SemanticBoardID); err != nil {
			return fmt.Errorf("rebuild relations: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if ids == nil {
		ids = []uint{}
	}
	return &BatchDetachResult{
		TopicID:    topicID,
		PeriodDate: day,
		Detached:   len(ids),
		SectionIDs: ids,
		BoardID:    topic.SemanticBoardID,
	}, nil
}

// markTopicCentroidPending flags a topic for a forced centroid recompute in
// the next daily-report refresh round (D2). Only the flag is written — the
// centroid column belongs to the algorithm (D1).
func (r *TopicGraphRepository) markTopicCentroidPending(tx *gorm.DB, topicID uint) error {
	if err := tx.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).
		Update("centroid_pending_recompute", true).Error; err != nil {
		return fmt.Errorf("flag topic %d for centroid recompute: %w", topicID, err)
	}
	return nil
}
