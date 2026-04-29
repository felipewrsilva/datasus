//go:build integration

package repository

import (
	"context"
	"testing"

	"datasus/internal/testutil"
)

func TestStageDoneCounts_nonNegative(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	c, err := NewStageRepository(pool).StageDoneCounts(ctx)
	if err != nil {
		t.Fatalf("StageDoneCounts: %v", err)
	}
	if c.Download < 0 || c.CSVConversion < 0 || c.ParquetConversion < 0 {
		t.Fatalf("unexpected negative counts: %+v", c)
	}
}
