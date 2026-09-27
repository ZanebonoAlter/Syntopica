package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ── Signal discovery batches & candidates (board-signal-reports) ─────────────
//
// Atomicity contract (design §3): the batch row and its candidates share one
// transaction — a half batch (batch without candidates on failure, or
// candidates without a batch) is impossible. Only successful batches are
// written, including zero-candidate batches (candidate_count=0 is a legal,
// quiet outcome). Within one batch, candidates with an identical signal title
// + normalized evidence set are mechanically deduplicated; cross-batch
// similarity is never merged.

// CreateSignalDiscoveryBatch atomically saves one discovery batch with its
// validated candidates. Owner/period columns of every candidate are stamped
// from the batch inside the transaction; the composite FK added by migration
// 20260922_0001 is the DB-level backstop for writes that bypass this code.
// It returns the persisted candidates (IDs assigned) and how many were
// dropped as within-batch duplicates.
func (r *Repository) CreateSignalDiscoveryBatch(ctx context.Context, discovery *BoardSignalDiscovery, candidates []*BoardSignalCandidate) ([]*BoardSignalCandidate, int, error) {
	if err := validateSignalDiscovery(discovery); err != nil {
		return nil, 0, err
	}
	seen := make(map[string]bool, len(candidates))
	unique := make([]*BoardSignalCandidate, 0, len(candidates))
	deduped := 0
	for _, c := range candidates {
		if err := validateSignalCandidateShape(c); err != nil {
			return nil, 0, err
		}
		key := ComputeSignalCandidateDedupeKey(c.Signal, SignalEvidenceRefs(c.EvidenceRefs))
		if seen[key] {
			deduped++
			continue
		}
		seen[key] = true
		unique = append(unique, c)
	}
	discovery.CandidateCount = len(unique)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(discovery).Error; err != nil {
			return err
		}
		for _, c := range unique {
			// Stamp owner/period from the batch — the candidate can never
			// disagree with its batch even on a programming mistake.
			c.DiscoveryID = discovery.ID
			c.SemanticBoardID = discovery.SemanticBoardID
			c.Granularity = discovery.Granularity
			c.Period = discovery.Period
			if err := tx.Create(c).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return unique, deduped, nil
}

func validateSignalDiscovery(d *BoardSignalDiscovery) error {
	if d == nil {
		return fmt.Errorf("signal discovery batch is nil")
	}
	if d.SemanticBoardID == 0 {
		return fmt.Errorf("signal discovery batch requires a board")
	}
	if !ValidSignalGranularity(d.Granularity) {
		return fmt.Errorf("signal discovery granularity must be month|year, got %q", d.Granularity)
	}
	if !ValidSignalPeriod(d.Granularity, d.Period) {
		return fmt.Errorf("signal discovery period %q does not match granularity %q", d.Period, d.Granularity)
	}
	if d.Cutoff.IsZero() {
		return fmt.Errorf("signal discovery batch requires a cutoff")
	}
	if d.AnalysisMode != SignalAnalysisModeCurrent && d.AnalysisMode != SignalAnalysisModeRetrospective {
		return fmt.Errorf("signal discovery analysis_mode must be current|retrospective, got %q", d.AnalysisMode)
	}
	return nil
}

// validateSignalCandidateShape enforces the detect contract (design §3):
// non-empty signal/why_it_matters/research_question/rationale, integer score
// 1..10, non-empty evidence_refs array. Whitelist membership (dropping
// dangling refs, dropping signals left without evidence) belongs to detect.
func validateSignalCandidateShape(c *BoardSignalCandidate) error {
	if c == nil {
		return fmt.Errorf("signal candidate is nil")
	}
	if strings.TrimSpace(c.Signal) == "" || strings.TrimSpace(c.WhyItMatters) == "" ||
		strings.TrimSpace(c.ResearchQuestion) == "" || strings.TrimSpace(c.Rationale) == "" {
		return fmt.Errorf("signal candidate requires non-empty signal/why_it_matters/research_question/rationale")
	}
	if c.Score < 1 || c.Score > 10 {
		return fmt.Errorf("signal candidate score must be 1..10, got %d", c.Score)
	}
	var refs []json.RawMessage
	if len(c.EvidenceRefs) == 0 || json.Unmarshal(c.EvidenceRefs, &refs) != nil || len(refs) == 0 {
		return fmt.Errorf("signal candidate requires a non-empty evidence_refs array")
	}
	return nil
}

// SignalCandidateListItem is one list row: the candidate plus the batch's
// creation time (design §3: 发现时间来自批次).
type SignalCandidateListItem struct {
	ID                 uint            `json:"id"`
	DiscoveryID        uint            `json:"discovery_id"`
	SemanticBoardID    uint            `json:"semantic_board_id"`
	Granularity        string          `json:"granularity"`
	Period             string          `json:"period"`
	Signal             string          `json:"signal"`
	WhyItMatters       string          `json:"why_it_matters"`
	ResearchQuestion   string          `json:"research_question"`
	EvidenceRefs       json.RawMessage `json:"evidence_refs"`
	Score              int             `json:"score"`
	Rationale          string          `json:"rationale"`
	CreatedAt          time.Time       `json:"created_at"`
	DiscoveryCreatedAt time.Time       `json:"discovery_created_at"`
}

// GetSignalCandidateByID fetches one candidate. Board ownership is enforced
// by the caller comparing SemanticBoardID (cross-board access must 404, not
// 403 — existence is not disclosed across boards).
func (r *Repository) GetSignalCandidateByID(ctx context.Context, id uint) (*BoardSignalCandidate, error) {
	var candidate BoardSignalCandidate
	err := r.db.WithContext(ctx).First(&candidate, id).Error
	if err != nil {
		return nil, fmt.Errorf("get signal candidate: %w", err)
	}
	return &candidate, nil
}

// GetSignalDiscoveryByID fetches one discovery batch (board-signal-reports
// 2b additive read): research deserializes the batch's InputSnapshot — the
// frozen material — and never re-assembles fresh material (PC-5).
func (r *Repository) GetSignalDiscoveryByID(ctx context.Context, id uint) (*BoardSignalDiscovery, error) {
	var discovery BoardSignalDiscovery
	err := r.db.WithContext(ctx).First(&discovery, id).Error
	if err != nil {
		return nil, fmt.Errorf("get signal discovery: %w", err)
	}
	return &discovery, nil
}

// ListSignalCandidatesByPeriod returns a board's candidates for one period,
// newest batch first (then candidate id), keyed by a candidate-id cursor
// (exclusive beforeID, 0 = head). Empty newer batches never remove older
// rows — the list is append-only by construction.
func (r *Repository) ListSignalCandidatesByPeriod(ctx context.Context, boardID uint, granularity, period string, beforeID uint, limit int) ([]SignalCandidateListItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := r.db.WithContext(ctx).
		Table("board_signal_candidate AS c").
		Select("c.id, c.discovery_id, c.semantic_board_id, c.granularity, c.period, c.signal, c.why_it_matters, c.research_question, c.evidence_refs, c.score, c.rationale, c.created_at, d.created_at AS discovery_created_at").
		Joins("JOIN board_signal_discovery d ON d.id = c.discovery_id").
		Where("c.semantic_board_id = ? AND c.granularity = ? AND c.period = ?", boardID, granularity, period)
	if beforeID > 0 {
		q = q.Where("c.id < ?", beforeID)
	}
	var rows []SignalCandidateListItem
	err := q.Order("c.discovery_id DESC, c.id DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list signal candidates: %w", err)
	}
	return rows, nil
}

// GetLatestSignalReportIDForCandidate returns the id of the candidate's most
// recent successful report, or nil when it has none (design §3: derived
// status 待研究/研究中/已有报告; the running half comes from live jobs, never
// a stored flag).
func (r *Repository) GetLatestSignalReportIDForCandidate(ctx context.Context, candidateID uint) (*uint, error) {
	var id *uint
	err := r.db.WithContext(ctx).
		Model(&TopicEnrichmentResult{}).
		Select("id").
		Where("source_signal_id = ? AND result_kind = ?", candidateID, ResultKindSignalReport).
		Order("id DESC").Limit(1).
		Scan(&id).Error
	if err != nil {
		return nil, fmt.Errorf("get latest signal report for candidate: %w", err)
	}
	return id, nil
}

// ListSignalReportResults lists successful signal reports for a board+period,
// newest first, cursor = result id (exclusive beforeID, 0 = head). Only
// kind=signal_report rows appear; other kinds keep their legacy queries.
func (r *Repository) ListSignalReportResults(ctx context.Context, boardID uint, granularity, period string, beforeID uint, limit int) ([]TopicEnrichmentResult, error) {
	if !ValidSignalGranularity(granularity) || !ValidSignalPeriod(granularity, period) {
		return nil, fmt.Errorf("invalid signal period %q/%q", granularity, period)
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND result_kind = ? AND granularity = ? AND period = ?", boardID, ResultKindSignalReport, granularity, period)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var list []TopicEnrichmentResult
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list signal reports: %w", err)
	}
	return list, nil
}

// ListSignalReportVersionsByCandidate lists one candidate's report versions
// (re-research appends, never overwrites — design §7). The board predicate
// mirrors the composite FK: a candidate from another board can never surface.
func (r *Repository) ListSignalReportVersionsByCandidate(ctx context.Context, boardID, candidateID uint, beforeID uint, limit int) ([]TopicEnrichmentResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := r.db.WithContext(ctx).
		Where("semantic_board_id = ? AND result_kind = ? AND source_signal_id = ?", boardID, ResultKindSignalReport, candidateID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var list []TopicEnrichmentResult
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list signal report versions: %w", err)
	}
	return list, nil
}

// CountSignalDiscoveriesByPeriod reports how many batches exist for a
// board+period (list pagination meta; zero means quiet empty state).
func (r *Repository) CountSignalDiscoveriesByPeriod(ctx context.Context, boardID uint, granularity, period string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&BoardSignalDiscovery{}).
		Where("semantic_board_id = ? AND granularity = ? AND period = ?", boardID, granularity, period).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count signal discoveries: %w", err)
	}
	return count, nil
}

// ── 研究进展持久化（board-signal-reports tasks 4.7「断了不能白跑」）──────────────
//
// 同 job_id 一行滚动更新：研究 loop 每轮结束后 upsert 一次（≤40 次写，无压
// 力）。超时/失败终态写 abandoned + stop_reason + error 后行保留；成功落库
// 报告后标 superseded 保留供追溯。进展表只是编排层快照：不回写候选、不替代
// 不可变报告。注意：终态写入时 job ctx 已死，服务层必须传入脱离原 ctx 的
// 后台 context（见 service 层 signalProgressRecorder）。

// UpsertSignalResearchProgress inserts or rolls forward one job's progress
// row keyed by job_id (ON CONFLICT → update rounds/counts/ledger/status/
// updated_at；owner/周期列一并覆盖，保持与候选一致）。进度快照语义：最新一
// 次写代表当前累计状态。
func (r *Repository) UpsertSignalResearchProgress(ctx context.Context, p *BoardSignalResearchProgress) error {
	if p == nil {
		return fmt.Errorf("signal research progress is nil")
	}
	if p.JobID == "" {
		return fmt.Errorf("signal research progress requires a job id")
	}
	if p.CandidateID == 0 || p.SemanticBoardID == 0 {
		return fmt.Errorf("signal research progress requires candidate and board")
	}
	if p.Status == "" {
		p.Status = SignalResearchProgressRunning
	}
	if len(p.Ledger) == 0 {
		p.Ledger = json.RawMessage("{}")
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "job_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"candidate_id", "semantic_board_id", "granularity", "period",
				"rounds_done", "source_calls", "calculation_calls",
				"ledger", "status", "stop_reason", "error", "updated_at",
			}),
		}).
		Create(p).Error
	if err != nil {
		return fmt.Errorf("upsert signal research progress: %w", err)
	}
	return nil
}

// GetLatestSignalResearchProgress returns the candidate's most recent progress
// row (by updated_at, then id), or nil when it has none — 候选从未研究过不是
// 错误，调用方据此序列化 null 摘要。
func (r *Repository) GetLatestSignalResearchProgress(ctx context.Context, candidateID uint) (*BoardSignalResearchProgress, error) {
	var row BoardSignalResearchProgress
	err := r.db.WithContext(ctx).
		Where("candidate_id = ?", candidateID).
		Order("updated_at DESC, id DESC").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest signal research progress: %w", err)
	}
	return &row, nil
}

// ListLatestSignalResearchProgress batch-returns each candidate's most recent
// progress row keyed by candidate id (missing candidates absent from the map).
// 候选列表的 last_research_progress 摘要经一次 IN 查询取齐——不顺手加重现有
// N+1。排序在 SQL、去重取首行在 Go，方言中立（SQLite handler 测试同路径）。
func (r *Repository) ListLatestSignalResearchProgress(ctx context.Context, candidateIDs []uint) (map[uint]BoardSignalResearchProgress, error) {
	out := make(map[uint]BoardSignalResearchProgress, len(candidateIDs))
	if len(candidateIDs) == 0 {
		return out, nil
	}
	var rows []BoardSignalResearchProgress
	err := r.db.WithContext(ctx).
		Where("candidate_id IN ?", candidateIDs).
		Order("candidate_id ASC, updated_at DESC, id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list signal research progress: %w", err)
	}
	for _, row := range rows {
		if _, seen := out[row.CandidateID]; !seen {
			out[row.CandidateID] = row
		}
	}
	return out, nil
}

// MarkSignalResearchProgressSuperseded archives the job's progress row after
// its report was saved (status=superseded，行保留供追溯；不删不改报告账本).
// 未知 job_id 静默成功——进展写入是尽力而为，不阻塞成功主流程。
func (r *Repository) MarkSignalResearchProgressSuperseded(ctx context.Context, jobID string) error {
	err := r.db.WithContext(ctx).
		Model(&BoardSignalResearchProgress{}).
		Where("job_id = ?", jobID).
		Updates(map[string]any{
			"status":      SignalResearchProgressSuperseded,
			"stop_reason": "",
			"error":       "",
			"updated_at":  time.Now(),
		}).Error
	if err != nil {
		return fmt.Errorf("mark signal research progress superseded: %w", err)
	}
	return nil
}

// SignalResearchStopReasonOrphanedByRestart marks rows converged by the
// startup sweep: the owning process died before any terminal write (the
// graceful-shutdown WithoutCancel write was lost with it), so the row was
// still running with no in-memory job behind it (2026-09-23 候选6：34 轮进展行
// 至今 running，UI 永久悬挂).
const SignalResearchStopReasonOrphanedByRestart = "orphaned_by_restart"

// SweepOrphanedSignalResearchProgress converges every status=running row to
// abandoned + stop_reason=orphaned_by_restart（board-signal-reports tasks
// 4.10；design §10.4）。MUST only run at startup when no research job can be
// live in memory — every running row is then definitionally an orphan；活进
// 程内路径仍由关机 defer 的 WithoutCancel 终态写负责。轮次/计数/账本/error
// 列原样保留（候选可查到该次研究已终止与当时轮次）；status CHECK 不变，
// stop_reason 自由文本。幂等：二次 sweep 无 running 行可动，返回 0。
func (r *Repository) SweepOrphanedSignalResearchProgress(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&BoardSignalResearchProgress{}).
		Where("status = ?", SignalResearchProgressRunning).
		Updates(map[string]any{
			"status":      SignalResearchProgressAbandoned,
			"stop_reason": SignalResearchStopReasonOrphanedByRestart,
			"updated_at":  time.Now(),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("sweep orphaned signal research progress: %w", result.Error)
	}
	return result.RowsAffected, nil
}
