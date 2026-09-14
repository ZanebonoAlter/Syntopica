package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 发现运行维护（improve-discovery-recommendations 4.6，design D2/D9）──
//
// DiscoveryRun 在运行期间是 running：ask/refresh 编排在失败时自行落 failed，
// 但进程崩溃/重启会留下永远 running 的僵尸 run（前端「查询中」永不结束，且
// 同 request_key 的重试被 ensureRun 复用旧行而不再执行）。维护任务按固定阈值
// 把超时仍 running 的行标 failed，使 run 状态可终结、可重试。

const (
	// DiscoveryRunStatusRunning 是运行中状态。
	DiscoveryRunStatusRunning = "running"
	// DiscoveryRunStatusFailed 是失败终态。
	DiscoveryRunStatusFailed = "failed"
	// DiscoveryRunErrorStaleRunning 是僵尸运行的错误码（前端按可识别 code 展示）。
	DiscoveryRunErrorStaleRunning = "stale_running"
	// DiscoveryRunStaleThresholdDefault 是判定僵尸运行的运行时长阈值（1 小时）。
	// 正常 ask/refresh 的 LLM 调用远低于该值；超时配置异常时也不至于永久占用。
	DiscoveryRunStaleThresholdDefault = time.Hour
)

// MarkStaleRunningDiscoveryRuns 把 started_at 早于 now-threshold 且仍 running 的
// DiscoveryRun 置为 failed（error_code=stale_running，finished_at=now），返回受影响行数。
// threshold <= 0 取默认 1 小时。幂等：已 failed 的行不再匹配。
func MarkStaleRunningDiscoveryRuns(ctx context.Context, db *gorm.DB, now time.Time, threshold time.Duration) (int64, error) {
	if threshold <= 0 {
		threshold = DiscoveryRunStaleThresholdDefault
	}
	cutoff := now.Add(-threshold)
	res := db.WithContext(ctx).
		Model(&models.DiscoveryRun{}).
		Where("status = ? AND started_at < ?", DiscoveryRunStatusRunning, cutoff).
		Updates(map[string]any{
			"status":      DiscoveryRunStatusFailed,
			"error_code":  DiscoveryRunErrorStaleRunning,
			"finished_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
