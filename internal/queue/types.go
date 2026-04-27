package queue

import (
	"time"

	"datasus/internal/domain"
)

type QueueStatus string

const (
	QueueStatusPending QueueStatus = "pending"
	QueueStatusRunning QueueStatus = "running"
	QueueStatusDone    QueueStatus = "done"
	QueueStatusFailed  QueueStatus = "failed"
)

// Job represents a claimed work item from the job_queue table.
type Job struct {
	ID          string
	FileID      string
	Stage       domain.StageName
	Status      QueueStatus
	AvailableAt time.Time
	LockedAt    *time.Time
	LockedBy    *string
	Attempts    int
	PayloadJSON []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
