package langsmith

import (
	"fmt"
	"time"
)

// formatTime formats a time.Time as an ISO 8601 string for LangSmith.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// formatDottedOrder creates a LangSmith dotted order segment from a time and run ID.
// Format: YYYYMMDDTHHMMSSmmmmmmZ{uuid}
func formatDottedOrder(t time.Time, runID string) string {
	u := t.UTC()
	return fmt.Sprintf("%04d%02d%02dT%02d%02d%02d%06dZ%s",
		u.Year(), u.Month(), u.Day(),
		u.Hour(), u.Minute(), u.Second(),
		u.Nanosecond()/1000, // microseconds
		runID,
	)
}
