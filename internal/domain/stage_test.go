package domain_test

import (
	"testing"

	"datasus/internal/domain"
)

func TestStage_Prerequisites(t *testing.T) {
	cases := []struct {
		stage        domain.StageName
		prereqStatus domain.StageStatus
		wantErr      bool
	}{
		// download has no prerequisite — any prereq status is fine
		{domain.StageDownload, "", false},
		{domain.StageDownload, domain.StageStatusFailed, false},

		// csv_conversion requires download done
		{domain.StageCSVConversion, domain.StageStatusDone, false},
		{domain.StageCSVConversion, domain.StageStatusPending, true},
		{domain.StageCSVConversion, domain.StageStatusRunning, true},
		{domain.StageCSVConversion, domain.StageStatusFailed, true},

		// parquet_conversion requires download done
		{domain.StageParquetConversion, domain.StageStatusDone, false},
		{domain.StageParquetConversion, domain.StageStatusPending, true},
		{domain.StageParquetConversion, domain.StageStatusFailed, true},
	}

	for _, tc := range cases {
		t.Run(string(tc.stage)+"/prereq="+string(tc.prereqStatus), func(t *testing.T) {
			s := &domain.Stage{
				Stage:  tc.stage,
				Status: domain.StageStatusPending,
			}
			err := s.CanRun(tc.prereqStatus)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestStage_CanRun_BlocksRunning(t *testing.T) {
	s := &domain.Stage{
		Stage:  domain.StageDownload,
		Status: domain.StageStatusRunning,
	}
	if err := s.CanRun(""); err == nil {
		t.Error("expected error for running stage, got nil")
	}
}

func TestStage_CanRun_BlocksPurged(t *testing.T) {
	s := &domain.Stage{
		Stage:  domain.StageDownload,
		Status: domain.StageStatusPurged,
	}
	if err := s.CanRun(""); err == nil {
		t.Error("expected error for purged stage, got nil")
	}
}

func TestStage_IncrementAttempts(t *testing.T) {
	s := &domain.Stage{Stage: domain.StageDownload, Attempts: 0}
	for i := 1; i < domain.MaxStageAttempts; i++ {
		maxed := s.IncrementAttempts()
		if maxed {
			t.Errorf("expected not maxed at attempt %d", i)
		}
		if s.Attempts != i {
			t.Errorf("attempts: want %d, got %d", i, s.Attempts)
		}
	}
	// Next increment should hit max
	maxed := s.IncrementAttempts()
	if !maxed {
		t.Error("expected maxed=true at max attempts")
	}
}

func TestPrerequisiteFor(t *testing.T) {
	prereq, ok := domain.PrerequisiteFor(domain.StageCSVConversion)
	if !ok || prereq != domain.StageDownload {
		t.Errorf("csv_conversion prereq: want download/true, got %s/%v", prereq, ok)
	}

	prereq, ok = domain.PrerequisiteFor(domain.StageParquetConversion)
	if !ok || prereq != domain.StageDownload {
		t.Errorf("parquet prereq: want download/true, got %s/%v", prereq, ok)
	}

	_, ok = domain.PrerequisiteFor(domain.StageDownload)
	if ok {
		t.Error("download should have no prerequisite")
	}
}
