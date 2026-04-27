package domain

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrInvalidTransition   = errors.New("invalid status transition")
	ErrPrerequisiteNotMet  = errors.New("stage prerequisite not met")
	ErrAlreadyPurged       = errors.New("already purged")
	ErrStageRunning        = errors.New("stage is currently running")
	ErrPolicyBlocked       = errors.New("blocked by processing policy")
)
