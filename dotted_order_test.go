package langsmith

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 45, 123456789, time.UTC)
	got := formatTime(ts)
	want := "2025-01-15T10:30:45.123456Z"
	if got != want {
		t.Errorf("formatTime = %q, want %q", got, want)
	}
}

func TestFormatDottedOrder(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 0, 0, 500000000, time.UTC)
	id := "abc-123"
	got := formatDottedOrder(ts, id)
	want := "20250601T120000500000Zabc-123"
	if got != want {
		t.Errorf("formatDottedOrder = %q, want %q", got, want)
	}
}

func TestFormatDottedOrderSubMicrosecond(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	id := "run-id"
	got := formatDottedOrder(ts, id)
	want := "20250101T000000000000Zrun-id"
	if got != want {
		t.Errorf("formatDottedOrder = %q, want %q", got, want)
	}
}
