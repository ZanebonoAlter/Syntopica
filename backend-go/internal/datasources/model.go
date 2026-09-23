package datasources

import (
	"time"

	"syntopica-backend/internal/platform/database"
)

// DataSource is the catalog table row (spec「数据源目录」). Static columns are
// a snapshot of the catalog constants, re-upserted on every startup; runtime
// columns (LastProbeAt/LastError) are never touched by the seed.
type DataSource struct {
	Code          string `gorm:"primaryKey;size:32" json:"code"`
	Name          string `gorm:"size:128" json:"name"`
	Provider      string `gorm:"size:128" json:"provider"`
	HomepageURL   string `gorm:"size:256" json:"homepage_url"`
	Coverage      string `gorm:"size:256" json:"coverage"`
	TopicsJSON    string `gorm:"column:topics;size:256" json:"-"` // JSON array of topic tags
	Frequency     string `gorm:"size:16" json:"frequency"`
	TypicalLag    string `gorm:"size:128" json:"typical_lag"`
	UnitPolicy    string `gorm:"size:256" json:"unit_policy"`
	RequiresKey   bool   `json:"requires_key"`
	ConfigKeyName string `gorm:"size:64" json:"config_key_name,omitempty"`

	// Runtime state. Status is recomputed at seed time from configuration
	// (a key-requiring source with no key stays disabled with a reason);
	// probe results never mutate it (spec「probe 端点」).
	Status       string     `gorm:"size:16;index" json:"status"` // enabled / disabled
	StatusReason string     `gorm:"size:256" json:"status_reason,omitempty"`
	LastProbeAt  *time.Time `json:"last_probe_at,omitempty"`
	LastError    string     `gorm:"size:512" json:"last_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DataSource) TableName() string { return "data_sources" }

func init() {
	database.RegisterModels(&DataSource{})
}

// Status values.
const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
)
