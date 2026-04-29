//go:build integration

package ftp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"datasus/internal/queue"
	"datasus/internal/repository"
	"datasus/internal/testutil"
)

func newScanner(t *testing.T, dir string) (*Scanner, func()) {
	t.Helper()
	pool := testutil.TestDB(t)
	ctx := context.Background()

	fileRepo := repository.NewFileRepository(pool)
	stageRepo := repository.NewStageRepository(pool)
	q := queue.New(pool, "scan-pipeline-test", 30*time.Second, time.Hour, 30*time.Minute, slog.Default())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM job_queue WHERE file_id IN (SELECT id FROM files WHERE ftp_dir = $1)`, dir)
		_, _ = pool.Exec(ctx, `DELETE FROM file_stages WHERE file_id IN (SELECT id FROM files WHERE ftp_dir = $1)`, dir)
		_, _ = pool.Exec(ctx, `DELETE FROM files WHERE ftp_dir = $1`, dir)
	}
	cleanup()

	s := &Scanner{
		dirs:      []string{dir},
		fileRepo:  fileRepo,
		stageRepo: stageRepo,
		queue:     q,
		policy:    nil, // disable policy checks: every file is allowed
		rootPath:  "/data",
		log:       slog.Default(),
		batchSize: 200,
	}
	return s, cleanup
}

func makeEntries(dir string, count int, mod time.Time, size int64) []Entry {
	states := []string{"SP", "RJ", "MG", "BA", "PR"}
	out := make([]Entry, 0, count)
	for i := 0; i < count; i++ {
		st := states[i%len(states)]
		month := (i % 12) + 1
		name := fmt.Sprintf("PI%s24%02d.DBC", st, month)
		out = append(out, Entry{
			Name:       name,
			Size:       size,
			ModTime:    mod,
			RemotePath: dir + "/" + name,
		})
	}
	return out
}

func TestScanPipeline_FullScanThenRepeatThenChange(t *testing.T) {
	dir := "/ftp/pipeline_e2e"
	s, cleanup := newScanner(t, dir)
	t.Cleanup(cleanup)

	ctx := context.Background()
	mod := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	entries := makeEntries(dir, 25, mod, 100)

	// Pass 1: full scan, all new.
	r, err := s.processEntriesBatch(ctx, dir, entries)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if r.Found != 25 {
		t.Fatalf("found = %d, want 25", r.Found)
	}
	if r.New != 25 {
		t.Fatalf("new = %d, want 25 (entries collide on same name; check generator)", r.New)
	}
	if r.Skipped != 0 {
		t.Fatalf("first scan skipped = %d, want 0", r.Skipped)
	}
	if len(r.Errors) != 0 {
		t.Fatalf("errors: %v", r.Errors)
	}

	// Pass 2: same entries → all unchanged.
	r2, err := s.processEntriesBatch(ctx, dir, entries)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if r2.Found != 25 {
		t.Fatalf("found pass2 = %d, want 25", r2.Found)
	}
	if r2.New != 0 {
		t.Fatalf("pass2 new = %d, want 0", r2.New)
	}
	if r2.Changed != 0 {
		t.Fatalf("pass2 changed = %d, want 0", r2.Changed)
	}
	if r2.Skipped != 25 {
		t.Fatalf("pass2 skipped = %d, want 25 (all unchanged)", r2.Skipped)
	}

	// Pass 3: 10% changed via different mod time.
	changed := make([]Entry, len(entries))
	copy(changed, entries)
	for i := 0; i < len(changed)/10+1; i++ {
		changed[i].ModTime = mod.Add(time.Hour)
		changed[i].Size = 200
	}
	r3, err := s.processEntriesBatch(ctx, dir, changed)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if r3.Changed == 0 {
		t.Fatalf("expected at least one changed file, got 0")
	}
	if r3.New != 0 {
		t.Fatalf("pass3 should not produce new files, got %d", r3.New)
	}
}

func TestScanPipeline_InvalidFilenameSkipped(t *testing.T) {
	dir := "/ftp/pipeline_invalid"
	s, cleanup := newScanner(t, dir)
	t.Cleanup(cleanup)

	ctx := context.Background()
	mod := time.Now()

	entries := []Entry{
		{Name: "BADNAME.txt", Size: 1, ModTime: mod, RemotePath: dir + "/BADNAME.txt"},
		{Name: "WRONGFMT.dbc", Size: 1, ModTime: mod, RemotePath: dir + "/WRONGFMT.dbc"},
		{Name: "ZZSP2401.dbc", Size: 100, ModTime: mod, RemotePath: dir + "/ZZSP2401.dbc"},
	}
	r, err := s.processEntriesBatch(ctx, dir, entries)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if r.Found != 1 {
		t.Fatalf("found = %d, want 1 (only ZZSP2401.dbc is valid)", r.Found)
	}
	if r.Skipped < 1 {
		t.Fatalf("skipped = %d, want at least 1 (WRONGFMT.dbc)", r.Skipped)
	}
	if !strings.EqualFold("ZZSP2401.DBC", strings.ToUpper("ZZSP2401.dbc")) {
		t.Fatal("guard against ToUpper bug")
	}
}
