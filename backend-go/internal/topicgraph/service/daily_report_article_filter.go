package service

import (
	"bytes"
	"encoding/json"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/articlerefs"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// decodeThreadArticleRefs reads a thread's jsonb reference array.
//
// Three shapes reach this point. A JSON array decodes to its ids. A value that
// is not an array at all — the JSON null scalar historical writers stored, or
// SQL NULL — reads as "no references" and is reported through nonArray so the
// caller rewrites it as []: jsonb_array_elements_text cannot read such a value,
// and leaving it in place keeps breaking every later query (SQLSTATE 22023).
// Malformed input (truncated or non-numeric-shaped payload) reports undecodable
// and the caller keeps the stored value untouched — dropping it would silently
// truncate a 线索's sources, which is worse than one unreadable row.
func decodeThreadArticleRefs(raw repository.JSON) (ids []uint, nonArray, undecodable bool) {
	trimmed := bytes.TrimSpace([]byte(raw))
	if len(trimmed) == 0 {
		return nil, true, false
	}
	if trimmed[0] != '[' {
		var probe interface{}
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return nil, false, true
		}
		return nil, true, false
	}
	if err := json.Unmarshal(trimmed, &ids); err != nil {
		return nil, false, true
	}
	return ids, false, false
}

// applyExistingArticleRefs drops article references without a live articles row
// from one thread batch, preserving the order of the survivors. It is the pure
// core of filterVanishedArticleRefs, kept separate so the write-path contract is
// testable without a database. A thread that loses every reference is written as
// [] (never a JSON null scalar); a thread whose stored value cannot be decoded is
// left alone with a warning.
func applyExistingArticleRefs(threads []repository.DailyReportThread, existing map[uint]bool) int {
	dropped := 0
	for i := range threads {
		ids, nonArray, undecodable := decodeThreadArticleRefs(threads[i].RelatedArticleIDs)
		if undecodable {
			logging.Warnf("daily-report: thread %q has an unreadable related_article_ids value; keeping it as stored", threads[i].Title)
			continue
		}
		kept := make([]uint, 0, len(ids))
		for _, id := range ids {
			if existing[id] {
				kept = append(kept, id)
			}
		}
		if !nonArray && len(kept) == len(ids) {
			continue
		}
		dropped += len(ids) - len(kept)
		threads[i].RelatedArticleIDs = marshalJSONArray(kept)
	}
	return dropped
}

// collectThreadArticleRefs gathers every decodable candidate id across all
// thread batches, so one existence probe can cover the whole report.
func collectThreadArticleRefs(threadBatches [][]repository.DailyReportThread) []uint {
	var candidates []uint
	for i := range threadBatches {
		for j := range threadBatches[i] {
			ids, _, undecodable := decodeThreadArticleRefs(threadBatches[i][j].RelatedArticleIDs)
			if undecodable {
				continue
			}
			candidates = append(candidates, ids...)
		}
	}
	return candidates
}

// filterVanishedArticleRefs drops references to articles that disappeared
// between candidate collection and the report write. A delete path — the
// (feed_id, link) duplicate merge or a feed deletion — can commit in that
// window, and the report would then store a reference to a row that no longer
// exists, which is exactly how the daily report turned into "文章 #<id>"
// (heal-dangling-article-refs D3).
//
// It runs on the FINAL thread batches, immediately before the caller returns
// them for SaveReport: watch materialization (Step 7.5) appends its own threads
// after an LLM adjudication round trip — a window of seconds to tens of seconds
// — so an earlier pass over the clustered threads alone would leave exactly the
// race this guard exists to close. One report therefore probes existence once,
// over both the clustered and the materialized references.
//
// The probe is best-effort: when it fails every candidate is kept, because a
// dangling reference is repairable while a silently truncated 线索 loses its
// sources for good.
func filterVanishedArticleRefs(db *gorm.DB, threadBatches [][]repository.DailyReportThread) int {
	if len(threadBatches) == 0 {
		return 0
	}
	candidates := collectThreadArticleRefs(threadBatches)
	var existing map[uint]bool
	if len(candidates) > 0 {
		found, err := articlerefs.ExistingArticleIDs(db, candidates)
		if err != nil {
			logging.Warnf("daily-report: article existence check failed, keeping references as collected: %v", err)
			return 0
		}
		existing = found
	}

	dropped := 0
	for i := range threadBatches {
		dropped += applyExistingArticleRefs(threadBatches[i], existing)
	}
	if dropped > 0 {
		logging.Infof("daily-report: dropped %d article reference(s) to articles deleted during generation", dropped)
	}
	return dropped
}
