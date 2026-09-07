package scheduler

import (
	"context"
	"fmt"
	"time"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	boardsvc "syntopica-backend/internal/tagmanagement/service/board"
)

const defaultBoardUpgradeSuggestTime = "06:30"

// NextBoardUpgradeSuggestTime computes the next wall-clock trigger for the
// board-upgrade-suggest job (default 06:30 local). It reads HH:MM from
// ai_settings key semantic_board_upgrade_suggest_time (design D4: fixed time,
// loosely coupled — not guaranteed to follow the daily report) and falls back to
// 06:30 on any read/parse error. Mirrors NextDailyReportTime's shape.
func NextBoardUpgradeSuggestTime(now time.Time) time.Time {
	h, m := 6, 30
	var setting models.AISettings
	if err := repository.Repo.DB().Where("key = ?", "semantic_board_upgrade_suggest_time").First(&setting).Error; err == nil {
		if ph, pm, parseErr := parseBoardUpgradeHHMM(setting.Value); parseErr == nil {
			h, m = ph, pm
		} else {
			logging.Warnf("board_upgrade_suggest: invalid time %q, using default %s", setting.Value, defaultBoardUpgradeSuggestTime)
		}
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if now.Before(today) {
		return today
	}
	return today.Add(24 * time.Hour)
}

func parseBoardUpgradeHHMM(s string) (int, int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("out of range")
	}
	return h, m, nil
}

// BoardUpgradeSuggestJob runs one discover_new generation pass and then GCs
// stale watch suggestions (spec: scheduler 定期生成建议 + 观察池建议自动回收).
// Only discover_new is run (design D4). A generation failure is logged only and
// surfaces as a non-nil JobResult carrying the error string but a nil error —
// the scheduler loop stays healthy and sibling jobs keep running. Returns
// inserted/skipped/cooldown/watch_gc counts for status display.
// BoardUpgradeSuggestJob runs the two create-direction generation passes
// ({create,aux} then {create,composite}) and nothing else (spec: 定时生成仅创建
// 方向——版块扩充纯手动，必须选版块触发). A per-pass failure is logged only
// and does not abort the sibling pass; the job returns a nil error so the
// scheduler loop stays healthy (flow 红线 10). Returns inserted/skipped/cooldown
// counts for status display.
func BoardUpgradeSuggestJob() JobFunc {
	return func(ctx context.Context) (*JobResult, error) {
		startTime := time.Now()
		db := repository.Repo.DB()
		svc := boardsvc.NewSemanticBoardUpgradeService(db, boardsvc.NewDefaultSemanticBoardUpgradeLLM(), nil)

		passes := []boardsvc.UpgradeGenerateRequest{
			{Direction: boardsvc.UpgradeDirectionCreate, Source: boardsvc.UpgradeSourceAux},
			{Direction: boardsvc.UpgradeDirectionCreate, Source: boardsvc.UpgradeSourceComposite},
		}
		var inserted, skipped, cooldownBlocked int
		for _, pass := range passes {
			pInserted, pSkipped, pCooldown, err := svc.GenerateAndPersist(ctx, pass)
			if err != nil {
				// Failure is logged only (flow 红线 10)；继续兄弟段，不标记 task failed。
				logging.Errorf("board_upgrade_suggest: %s pass failed: %v", pass.ModeKey(), err)
				continue
			}
			inserted += pInserted
			skipped += pSkipped
			cooldownBlocked += pCooldown
		}

		return &JobResult{
			Data: map[string]interface{}{
				"inserted":         inserted,
				"skipped":          skipped,
				"cooldown_blocked": cooldownBlocked,
				"started_at":       startTime.Format(time.RFC3339),
				"finished_at":      time.Now().Format(time.RFC3339),
			},
			Summary: fmt.Sprintf("board upgrade: inserted=%d skipped=%d cooldown=%d", inserted, skipped, cooldownBlocked),
		}, nil
	}
}
