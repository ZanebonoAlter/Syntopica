package models

import (
	"time"
)

var (
	shanghaiTZ = time.FixedZone("CST", 8*3600)
)

func FormatDatetimeCST(t time.Time) string {
	return t.In(shanghaiTZ).Format("2006-01-02T15:04:05Z07:00")
}

func FormatDatetimeCSTPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := FormatDatetimeCST(*t)
	return &formatted
}
