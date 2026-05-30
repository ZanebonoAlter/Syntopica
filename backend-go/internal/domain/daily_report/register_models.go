package daily_report

import "syntopica-backend/internal/platform/database"

func init() {
	database.RegisterModels(
		&BoardDailyReport{},
		&DailyReportSection{},
		&DailyReportThread{},
	)
}
