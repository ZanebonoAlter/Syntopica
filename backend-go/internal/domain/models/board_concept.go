package models

import "time"

type BoardConcept struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Name            string    `gorm:"size:300;not null" json:"name"`
	Description     string    `gorm:"type:text" json:"description"`
	Embedding       *string   `gorm:"type:vector;column:embedding" json:"-"`
	Category        string    `gorm:"size:20;not null;default:keyword;index" json:"category"`
	ScopeType       string    `gorm:"size:20;not null;default:global" json:"scope_type"`
	ScopeCategoryID *uint     `json:"scope_category_id"`
	IsSystem        bool      `gorm:"not null;default:false" json:"is_system"`
	Status          string    `gorm:"size:20;not null;default:pending;index" json:"status"`
	DisplayOrder    int       `gorm:"not null;default:0" json:"display_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (BoardConcept) TableName() string {
	return "board_concepts"
}
