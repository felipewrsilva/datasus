package domain

import (
	"fmt"
	"time"
)

type StageName string

const (
	StageDownload         StageName = "download"
	StageCSVConversion    StageName = "csv_conversion"
	StageParquetConversion StageName = "parquet_conversion"
)

type StageStatus string

const (
	StageStatusPending StageStatus = "pending"
	StageStatusRunning StageStatus = "running"
	StageStatusDone    StageStatus = "done"
	StageStatusFailed  StageStatus = "failed"
	StageStatusPurged  StageStatus = "purged"
)

const MaxStageAttempts = 5

// stagePrerequisites defines which stage must be Done before another may run.
var stagePrerequisites = map[StageName]StageName{
	StageCSVConversion:     StageDownload,
	StageParquetConversion: StageDownload,
}

// Stage tracks the execution state of one pipeline stage for a file.
type Stage struct {
	ID           string     `json:"id"`
	FileID       string     `json:"file_id"`
	Stage        StageName  `json:"stage"`
	Status       StageStatus `json:"status"`
	Attempts     int        `json:"attempts"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	ErrorMessage *string    `json:"error_message"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CanRun checks whether this stage is eligible to run given the status of its prerequisite.
// prerequisiteStatus is the current status of the required preceding stage (empty string if none).
func (s *Stage) CanRun(prerequisiteStatus StageStatus) error {
	if _, hasPrereq := stagePrerequisites[s.Stage]; hasPrereq {
		if prerequisiteStatus != StageStatusDone {
			return fmt.Errorf("%w: %s requires %s to be done (current: %s)",
				ErrPrerequisiteNotMet, s.Stage, stagePrerequisites[s.Stage], prerequisiteStatus)
		}
	}
	if s.Status == StageStatusRunning {
		return ErrStageRunning
	}
	if s.Status == StageStatusPurged {
		return ErrAlreadyPurged
	}
	return nil
}

// PrerequisiteFor returns the stage that must be done before this stage, if any.
func PrerequisiteFor(stage StageName) (StageName, bool) {
	prereq, ok := stagePrerequisites[stage]
	return prereq, ok
}

// IncrementAttempts increments the attempt counter and returns whether max attempts is reached.
func (s *Stage) IncrementAttempts() bool {
	s.Attempts++
	return s.Attempts >= MaxStageAttempts
}
