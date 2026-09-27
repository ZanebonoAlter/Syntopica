package scheduler

import (
	"time"

	"syntopica-backend/internal/platform/database"
)

// Register SchedulerTask with AutoMigrate via the injection registry (same
// pattern as topicgraph/repository). The static list in database/migrator.go
// cannot reference this package: migrator→scheduler plus
// scheduler→analysispause→aihealth→airouter→database would close an import
// cycle.
func init() {
	database.RegisterModels(&SchedulerTask{})
}

// SchedulerTask is the DB row backing a scheduler's persistence hooks
// (TaskPersistence). Migrated from internal/models in decouple-backend-domains:
// the model is exclusively owned by the scheduler framework, and app/platform
// consumers import it from here (platform→platform / app→platform are allowed
// by the package boundary rules).
type SchedulerTask struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	Name                  string     `gorm:"size:50;unique;index" json:"name"`
	Description           string     `gorm:"size:200" json:"description"`
	CheckInterval         int        `json:"check_interval"` // seconds
	LastExecutionTime     *time.Time `json:"last_execution_time"`
	NextExecutionTime     *time.Time `json:"next_execution_time"`
	Status                string     `gorm:"size:20;index" json:"status"`
	LastError             string     `gorm:"type:text" json:"last_error"`
	LastErrorTime         *time.Time `json:"last_error_time"`
	TotalExecutions       int        `json:"total_executions"`
	SuccessfulExecutions  int        `json:"successful_executions"`
	FailedExecutions      int        `json:"failed_executions"`
	ConsecutiveFailures   int        `json:"consecutive_failures"`
	LastExecutionDuration *float64   `json:"last_execution_duration"` // seconds
	LastExecutionResult   string     `gorm:"type:text" json:"last_execution_result"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (s *SchedulerTask) ToDict() map[string]interface{} {
	successRate := 0.0
	if s.TotalExecutions > 0 {
		successRate = float64(s.SuccessfulExecutions) / float64(s.TotalExecutions) * 100
	}

	return map[string]interface{}{
		"id":                      s.ID,
		"name":                    s.Name,
		"description":             s.Description,
		"check_interval":          s.CheckInterval,
		"last_execution_time":     formatDatetimeCSTPtr(s.LastExecutionTime),
		"next_execution_time":     formatDatetimeCSTPtr(s.NextExecutionTime),
		"status":                  s.Status,
		"last_error":              s.LastError,
		"last_error_time":         formatDatetimeCSTPtr(s.LastErrorTime),
		"total_executions":        s.TotalExecutions,
		"successful_executions":   s.SuccessfulExecutions,
		"failed_executions":       s.FailedExecutions,
		"consecutive_failures":    s.ConsecutiveFailures,
		"last_execution_duration": s.LastExecutionDuration,
		"last_execution_result":   s.LastExecutionResult,
		"created_at":              formatDatetimeCST(s.CreatedAt),
		"updated_at":              formatDatetimeCST(s.UpdatedAt),
		"success_rate":            successRate,
	}
}

// CST formatting helpers mirror internal/models/utils.go (same 8h-fixed CST
// zone). Duplicated here because models is a business-facing shared package
// and the scheduler framework must not depend on it.

var shanghaiTZ = time.FixedZone("CST", 8*3600)

func formatDatetimeCST(t time.Time) string {
	return t.In(shanghaiTZ).Format("2006-01-02T15:04:05Z07:00")
}

func formatDatetimeCSTPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := formatDatetimeCST(*t)
	return &formatted
}
