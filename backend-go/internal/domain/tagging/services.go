package tagging

import (
	"fmt"
	"time"
)

var TopicGraphCST = time.FixedZone("CST", 8*3600)

func ParseAnchorDate(value string) (time.Time, error) {
	if value == "" {
		return time.Now().In(TopicGraphCST), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, TopicGraphCST)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func ResolveWindow(kind string, anchor time.Time) (time.Time, time.Time, string, error) {
	current := anchor.In(TopicGraphCST)
	dayStart := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, TopicGraphCST)

	switch kind {
	case "daily":
		return dayStart, dayStart.AddDate(0, 0, 1), fmt.Sprintf("%s 当日", dayStart.Format("2006-01-02")), nil
	case "weekly":
		daysSinceMonday := (int(current.Weekday()) + 6) % 7
		weekStart := dayStart.AddDate(0, 0, -daysSinceMonday)
		weekEnd := weekStart.AddDate(0, 0, 7)
		return weekStart, weekEnd, fmt.Sprintf("%s - %s", weekStart.Format("01-02"), weekEnd.AddDate(0, 0, -1).Format("01-02")), nil
	case "all":
		return time.Date(2000, 1, 1, 0, 0, 0, 0, TopicGraphCST), time.Date(2100, 1, 1, 0, 0, 0, 0, TopicGraphCST), "全部", nil
	default:
		return time.Time{}, time.Time{}, "", fmt.Errorf("unsupported topic graph type: %s", kind)
	}
}
