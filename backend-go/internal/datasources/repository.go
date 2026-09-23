package datasources

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// KeyResolver returns the configured secret for a config key name (e.g.
// "COMTRADE_API_KEY" → the configured value, "" when unset). Injected so the
// repository stays decoupled from the config package and stays testable.
type KeyResolver func(configKeyName string) string

// UpsertCatalog re-projects the catalog constants into data_sources on every
// startup (design decision 5). Static columns update; runtime columns
// (last_probe_at / last_error) are preserved. Status is recomputed from the
// key resolver — a key-requiring source with an unconfigured key stays
// disabled with a reason naming the config key (spec「未配置 key 的源在目录中
// 显式禁用」). Idempotent by code (test-cases M19).
func UpsertCatalog(db *gorm.DB, resolve KeyResolver) error {
	for _, d := range Catalog() {
		status, reason := StatusFor(d, resolve)
		topics, err := json.Marshal(d.Topics)
		if err != nil {
			return fmt.Errorf("marshal topics for %s: %w", d.Code, err)
		}
		row := DataSource{
			Code: d.Code, Name: d.Name, Provider: d.Provider,
			HomepageURL: d.HomepageURL, Coverage: d.Coverage,
			TopicsJSON: string(topics), Frequency: d.Frequency,
			TypicalLag: d.TypicalLag, UnitPolicy: d.UnitPolicy,
			RequiresKey: d.RequiresKey, ConfigKeyName: d.ConfigKeyName,
			Status: status, StatusReason: reason,
		}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "provider", "homepage_url", "coverage", "topics", "frequency", "typical_lag", "unit_policy", "requires_key", "config_key_name", "status", "status_reason", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("upsert catalog row %s: %w", d.Code, err)
		}
	}
	return nil
}

// StatusFor computes the catalog status + reason for a source definition
// under the given key resolver — the single enabled/disabled judgment shared
// by UpsertCatalog (startup seed) and UpdateStatus (UI save path), so the two
// call sites can never drift apart.
func StatusFor(d DataSourceDefinition, resolve KeyResolver) (string, string) {
	if d.RequiresKey && strings.TrimSpace(resolve(d.ConfigKeyName)) == "" {
		return StatusDisabled, fmt.Sprintf("未配置 %s：请在环境变量或 config.yaml 配置后重启（见 docs/reference/configuration.md 研究数据源节）", d.ConfigKeyName)
	}
	return StatusEnabled, ""
}

// UpdateStatus recomputes one source's status under resolve and persists just
// the status columns (the row itself comes from the startup seed). The UI
// save path calls this right after the key is written, so a freshly saved
// key flips the catalog status without a restart.
func UpdateStatus(db *gorm.DB, code string, resolve KeyResolver) error {
	d, ok := Definition(code)
	if !ok {
		return fmt.Errorf("unknown data source %q", code)
	}
	status, reason := StatusFor(d, resolve)
	if err := db.Model(&DataSource{}).Where("code = ?", code).Updates(map[string]any{
		"status": status, "status_reason": reason,
	}).Error; err != nil {
		return fmt.Errorf("update status for %s: %w", code, err)
	}
	return nil
}

// ListCatalog returns all catalog rows ordered by code.
func ListCatalog(db *gorm.DB) ([]DataSource, error) {
	var rows []DataSource
	if err := db.Order("code").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list catalog: %w", err)
	}
	return rows, nil
}

// CatalogDTO is the API shape of a catalog row: TopicsJSON is decoded into a
// real array (spec: 目录 metadata 为结构化字段).
type CatalogDTO struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	HomepageURL   string   `json:"homepage_url"`
	Coverage      string   `json:"coverage"`
	Topics        []string `json:"topics"`
	Frequency     string   `json:"frequency"`
	TypicalLag    string   `json:"typical_lag"`
	UnitPolicy    string   `json:"unit_policy"`
	RequiresKey   bool     `json:"requires_key"`
	ConfigKeyName string   `json:"config_key_name,omitempty"`
	Status        string   `json:"status"`
	StatusReason  string   `json:"status_reason,omitempty"`
	LastProbeAt   *string  `json:"last_probe_at,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
}

// ToDTO converts a row; a malformed TopicsJSON degrades to an empty array
// rather than failing the whole listing.
func (r DataSource) ToDTO() CatalogDTO {
	dto := CatalogDTO{
		Code: r.Code, Name: r.Name, Provider: r.Provider,
		HomepageURL: r.HomepageURL, Coverage: r.Coverage,
		Topics: []string{}, Frequency: r.Frequency,
		TypicalLag: r.TypicalLag, UnitPolicy: r.UnitPolicy,
		RequiresKey: r.RequiresKey, ConfigKeyName: r.ConfigKeyName,
		Status: r.Status, StatusReason: r.StatusReason,
		LastError: r.LastError,
	}
	if r.LastProbeAt != nil {
		s := r.LastProbeAt.UTC().Format("2006-01-02T15:04:05Z")
		dto.LastProbeAt = &s
	}
	_ = json.Unmarshal([]byte(r.TopicsJSON), &dto.Topics)
	if dto.Topics == nil {
		dto.Topics = []string{}
	}
	return dto
}
