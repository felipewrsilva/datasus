package repository

import (
	"encoding/json"
	"testing"
	"time"

	"datasus/internal/domain"
)

func TestLogEntryJSONUsesRFC3339Timestamp(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, time.April, 28, 13, 14, 15, 0, time.UTC)
	entry := LogEntry{
		ID:        1,
		FileID:    "f1",
		Stage:     domain.StageDownload,
		EventType: "completed",
		Message:   "ok",
		CreatedAt: &created,
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal log entry: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	got, _ := payload["created_at"].(string)
	want := "2026-04-28T13:14:15Z"
	if got != want {
		t.Fatalf("created_at = %q, want %q", got, want)
	}
}
