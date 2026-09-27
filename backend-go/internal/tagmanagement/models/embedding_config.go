package models

import "time"

// EmbeddingConfig stores key-value configuration for the embedding system
type EmbeddingConfig struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Key         string    `gorm:"size:100;unique;not null;index" json:"key"`
	Value       string    `gorm:"type:text;not null" json:"value"`
	Description string    `gorm:"size:200" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName specifies the table name for EmbeddingConfig
func (EmbeddingConfig) TableName() string {
	return "embedding_config"
}
