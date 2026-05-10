package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Artifact is the idempotency row for one consolidated Parquet output.
type Artifact struct {
	OutputPath       string
	Catalog          string
	State            string
	Year             int
	Month            int
	LogicalBase      string
	InputFingerprint string
	ParquetSha256    sql.NullString
	LastStatus       string
	LastError        sql.NullString
	UpdatedAt        time.Time
}

// Store wraps SQL Server access.
type Store struct {
	db *sql.DB
}

func Open(connectionString string) (*Store, error) {
	db, err := sql.Open("sqlserver", connectionString)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sql ping: %w", err)
	}
	return &Store{db: db}, nil
}

// SetMaxOpenConns adjusts the pool size (e.g. for parallel conversion workers).
func (s *Store) SetMaxOpenConns(n int) {
	if n < 1 {
		n = 1
	}
	s.db.SetMaxOpenConns(n)
}

func (s *Store) Close() error {
	return s.db.Close()
}

func normalizeOutputKey(outputPath string) string {
	return filepath.Clean(strings.TrimSpace(outputPath))
}

// GetArtifactByOutputPath loads an artifact row by normalized output path.
func (s *Store) GetArtifactByOutputPath(ctx context.Context, outputPath string) (*Artifact, error) {
	key := normalizeOutputKey(outputPath)
	const q = `
SELECT DC_OUTPUT_PATH, CD_CATALOG, CD_STATE, NR_FILE_YEAR, NR_FILE_MONTH, DC_LOGICAL_BASE,
       RTRIM(BN_INPUT_FINGERPRINT_SHA256), RTRIM(BN_PARQUET_SHA256_HEX), DC_LAST_STATUS, DC_LAST_ERROR, DT_UPDATED_UTC
  FROM dbo.LOG_DATASUS_DBC_PARQUET_ARTIFACT
 WHERE DC_OUTPUT_PATH = @p1`
	var a Artifact
	var pqHash sql.NullString
	err := s.db.QueryRowContext(ctx, q, key).Scan(
		&a.OutputPath,
		&a.Catalog,
		&a.State,
		&a.Year,
		&a.Month,
		&a.LogicalBase,
		&a.InputFingerprint,
		&pqHash,
		&a.LastStatus,
		&a.LastError,
		&a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.ParquetSha256 = pqHash
	return &a, nil
}

// UpsertArtifact inserts or updates the idempotency row.
func (s *Store) UpsertArtifact(ctx context.Context,
	outputPath string,
	catalog, state string, year, month int, logicalBase string,
	inputFingerprint, parquetSHA256Hex, status string,
	errMsg *string,
) error {
	key := normalizeOutputKey(outputPath)
	var errStr interface{}
	if errMsg != nil && *errMsg != "" {
		errStr = *errMsg
	} else {
		errStr = nil
	}
	var pqHex interface{}
	if strings.TrimSpace(parquetSHA256Hex) != "" {
		pqHex = parquetSHA256Hex
	} else {
		pqHex = nil
	}
	const q = `
MERGE dbo.LOG_DATASUS_DBC_PARQUET_ARTIFACT AS t
USING (SELECT @p1 AS DC_OUTPUT_PATH) AS s
ON t.DC_OUTPUT_PATH = s.DC_OUTPUT_PATH
WHEN MATCHED THEN UPDATE SET
  CD_CATALOG = @p2, CD_STATE = @p3, NR_FILE_YEAR = @p4, NR_FILE_MONTH = @p5, DC_LOGICAL_BASE = @p6,
  BN_INPUT_FINGERPRINT_SHA256 = @p7, BN_PARQUET_SHA256_HEX = @p8, DC_LAST_STATUS = @p9, DC_LAST_ERROR = @p10,
  DT_UPDATED_UTC = SYSUTCDATETIME()
WHEN NOT MATCHED THEN INSERT (
  DC_OUTPUT_PATH, CD_CATALOG, CD_STATE, NR_FILE_YEAR, NR_FILE_MONTH, DC_LOGICAL_BASE,
  BN_INPUT_FINGERPRINT_SHA256, BN_PARQUET_SHA256_HEX, DC_LAST_STATUS, DC_LAST_ERROR, DT_UPDATED_UTC
) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, SYSUTCDATETIME());`
	_, err := s.db.ExecContext(ctx, q,
		key,
		catalog, state, year, month, logicalBase,
		inputFingerprint, pqHex, status, errStr,
	)
	return err
}

// InsertRun appends an audit row for one conversion attempt.
func (s *Store) InsertRun(ctx context.Context,
	started, finished time.Time,
	outputPath string,
	success bool,
	errMsg *string,
	dataRows *int64,
	sourcePathsJSON string,
	inputFingerprint, outputParquetSHA256 *string,
) error {
	var em interface{}
	if errMsg != nil && *errMsg != "" {
		em = *errMsg
	} else {
		em = nil
	}
	var rows interface{}
	if dataRows != nil {
		rows = *dataRows
	} else {
		rows = nil
	}
	var fp interface{}
	if inputFingerprint != nil && *inputFingerprint != "" {
		fp = *inputFingerprint
	} else {
		fp = nil
	}
	var outHash interface{}
	if outputParquetSHA256 != nil && *outputParquetSHA256 != "" {
		outHash = *outputParquetSHA256
	} else {
		outHash = nil
	}
	var fin interface{}
	if finished.IsZero() {
		fin = nil
	} else {
		fin = finished
	}
	const q = `
INSERT INTO dbo.LOG_DATASUS_DBC_PARQUET_RUN (
  DT_ATTEMPT_STARTED_UTC, DT_ATTEMPT_FINISHED_UTC, DC_OUTPUT_PATH, IC_SUCCESS, DC_ERROR_MESSAGE,
  QT_DATA_ROWS, DC_SOURCE_PATHS_JSON, BN_INPUT_FINGERPRINT_SHA256, BN_OUTPUT_PARQUET_SHA256_HEX
) VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9)`
	_, err := s.db.ExecContext(ctx, q,
		started, fin, normalizeOutputKey(outputPath), success, em,
		rows, sourcePathsJSON, fp, outHash,
	)
	return err
}
