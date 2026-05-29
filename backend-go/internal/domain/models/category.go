package models

import (
	"crypto/md5" //nolint:gosec // only used for slug generation
	"encoding/hex"
	"time"
)

type Category struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100;unique;not null" json:"name"`
	Slug        string    `gorm:"size:50;unique" json:"slug"`
	Icon        string    `gorm:"size:50;default:folder" json:"icon"`
	Color       string    `gorm:"size:20;default:#6366f1" json:"color"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Feeds       []Feed    `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"feeds,omitempty"`
}

func (c *Category) ToDict() map[string]interface{} {
	feedCount := len(c.Feeds)
	return map[string]interface{}{
		"id":          c.ID,
		"name":        c.Name,
		"slug":        c.Slug,
		"icon":        c.Icon,
		"color":       c.Color,
		"description": c.Description,
		"created_at":  FormatDatetimeCST(c.CreatedAt),
		"feed_count":  feedCount,
	}
}

func GenerateSlug(name string) string {
	hash := md5.Sum([]byte(name)) //nolint:gosec // only used for slug generation
	return hex.EncodeToString(hash[:])[:8]
}
