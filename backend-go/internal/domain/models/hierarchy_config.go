package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type HierarchyTemplatesJSON map[string]json.RawMessage

func (h HierarchyTemplatesJSON) Value() (driver.Value, error) {
	if h == nil {
		return "{}", nil
	}
	b, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (h *HierarchyTemplatesJSON) Scan(value any) error {
	if value == nil {
		*h = HierarchyTemplatesJSON{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("scan hierarchy templates: unsupported value type %T", value)
	}
	if len(data) == 0 {
		*h = HierarchyTemplatesJSON{}
		return nil
	}
	return json.Unmarshal(data, h)
}

type HierarchyConfig struct {
	ID        uint                   `gorm:"primaryKey" json:"id"`
	Templates HierarchyTemplatesJSON `gorm:"type:jsonb;serializer:json;default:'{}'" json:"templates"`
	Version   int                    `gorm:"not null;default:1" json:"version"`
	UpdatedAt time.Time              `json:"updated_at"`
}

func (HierarchyConfig) TableName() string {
	return "hierarchy_config"
}

type HierarchyConfigVersion struct {
	ID        uint                   `gorm:"primaryKey" json:"id"`
	ConfigID  uint                   `gorm:"not null;index" json:"config_id"`
	Version   int                    `gorm:"not null" json:"version"`
	Templates HierarchyTemplatesJSON `gorm:"type:jsonb;serializer:json;default:'{}'" json:"templates"`
	ChangeLog string                 `gorm:"type:text" json:"change_log"`
	CreatedAt time.Time              `json:"created_at"`
}

func (HierarchyConfigVersion) TableName() string {
	return "hierarchy_config_versions"
}
