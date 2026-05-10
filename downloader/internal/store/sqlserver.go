package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RegistryRow is a row from LOG_DATASUS_DBC_FILE used for change detection.
type RegistryRow struct {
	ID                uuid.UUID
	QTRemoteSize      int64
	DTRemoteModified  sql.NullTime
	DCLocalPath       sql.NullString
	QTLocalSize       sql.NullInt64
	DTLastDownloadUTC sql.NullTime
}

// Store wraps SQL Server access for the downloader.
type Store struct {
	db *sql.DB
}

func Open(connectionString string) (*Store, error) {
	db, err := sql.Open("sqlserver", connectionString)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sql ping: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// GetRegistry loads a registry row by host and remote path, if present.
func (s *Store) GetRegistry(ctx context.Context, ftpHost, remotePath string) (*RegistryRow, error) {
	const q = `
SELECT CAST(ID_LOG_DATASUS_DBC_FILE AS CHAR(36)),
       QT_REMOTE_SIZE_BYTES,
       DT_REMOTE_MODIFIED_UTC,
       DC_LOCAL_PATH,
       QT_LOCAL_SIZE_BYTES,
       DT_LAST_DOWNLOAD_UTC
  FROM dbo.LOG_DATASUS_DBC_FILE
 WHERE DC_FTP_HOST = @p1 AND EX_FTP_REMOTE_PATH = @p2`
	var idStr string
	var row RegistryRow
	err := s.db.QueryRowContext(ctx, q, ftpHost, remotePath).Scan(
		&idStr,
		&row.QTRemoteSize,
		&row.DTRemoteModified,
		&row.DCLocalPath,
		&row.QTLocalSize,
		&row.DTLastDownloadUTC,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.ID, err = uuid.Parse(strings.TrimSpace(idStr))
	if err != nil {
		return nil, fmt.Errorf("parse id: %w", err)
	}
	return &row, nil
}

// InsertRegistryIfMissing inserts a stub row when none exists (for FK and first-time tracking).
func (s *Store) InsertRegistryIfMissing(ctx context.Context,
	ftpHost, remotePath, ftpDir, fileName string,
	catalog, state string, year int, month int, segment *string,
	remoteSize int64, remoteModUTC sql.NullTime,
) error {
	const q = `
IF NOT EXISTS (
  SELECT 1 FROM dbo.LOG_DATASUS_DBC_FILE
   WHERE DC_FTP_HOST = @p1 AND EX_FTP_REMOTE_PATH = @p2
)
INSERT INTO dbo.LOG_DATASUS_DBC_FILE (
  DC_FTP_HOST, EX_FTP_REMOTE_PATH, DC_FTP_DIRECTORY, DC_FILE_NAME,
  CD_CATALOG, CD_STATE, NR_FILE_YEAR, NR_FILE_MONTH, CD_SEGMENT,
  QT_REMOTE_SIZE_BYTES, DT_REMOTE_MODIFIED_UTC
) VALUES (
  @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11
)`
	var seg interface{}
	if segment != nil && *segment != "" {
		seg = *segment
	} else {
		seg = nil
	}
	_, err := s.db.ExecContext(ctx, q,
		ftpHost, remotePath, ftpDir, fileName,
		catalog, state, year, month, seg,
		remoteSize, nullableTime(remoteModUTC),
	)
	return err
}

// RecordAttempt inserts a download attempt row and, on success, updates the registry.
func (s *Store) RecordAttempt(ctx context.Context,
	fileID uuid.UUID,
	started, finished time.Time,
	ftpHost, remotePath string,
	remoteSize int64, remoteModUTC sql.NullTime,
	triggerReason string,
	success bool,
	errMsg *string,
	localPath *string,
	localSize *int64,
	// success-only registry update:
	ftpDir, fileName string,
	catalog, state string, year, month int, segment *string,
	sha256hex string, // used only when success is true
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const ins = `
INSERT INTO dbo.LOG_DATASUS_DBC_DOWNLOAD (
  ID_LOG_DATASUS_DBC_FILE,
  DT_ATTEMPT_STARTED_UTC,
  DT_ATTEMPT_FINISHED_UTC,
  DC_FTP_HOST,
  EX_FTP_REMOTE_PATH,
  QT_REMOTE_SIZE_BYTES,
  DT_REMOTE_MODIFIED_UTC,
  DC_TRIGGER_REASON,
  IC_SUCCESS,
  DC_ERROR_MESSAGE,
  DC_LOCAL_PATH,
  QT_LOCAL_SIZE_BYTES
) VALUES (
  @p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11, @p12
)`
	var errStr interface{}
	if errMsg != nil {
		errStr = *errMsg
	} else {
		errStr = nil
	}
	var lp interface{}
	if localPath != nil {
		lp = *localPath
	}
	var ls interface{}
	if localSize != nil {
		ls = *localSize
	}
	_, err = tx.ExecContext(ctx, ins,
		fileID.String(),
		started, finished,
		ftpHost, remotePath,
		remoteSize, nullableTime(remoteModUTC),
		triggerReason,
		success,
		errStr,
		lp,
		ls,
	)
	if err != nil {
		return fmt.Errorf("insert log: %w", err)
	}

	if success {
		var seg interface{}
		if segment != nil && *segment != "" {
			seg = *segment
		} else {
			seg = nil
		}
		const upd = `
UPDATE dbo.LOG_DATASUS_DBC_FILE SET
  DC_FTP_DIRECTORY = @p2,
  DC_FILE_NAME = @p3,
  CD_CATALOG = @p4,
  CD_STATE = @p5,
  NR_FILE_YEAR = @p6,
  NR_FILE_MONTH = @p7,
  CD_SEGMENT = @p8,
  QT_REMOTE_SIZE_BYTES = @p9,
  DT_REMOTE_MODIFIED_UTC = @p10,
  DC_LOCAL_PATH = @p11,
  QT_LOCAL_SIZE_BYTES = @p12,
  BN_LOCAL_SHA256_HEX = @p13,
  DT_LAST_DOWNLOAD_UTC = @p14,
  DC_LAST_DOWNLOAD_STATUS = N'success',
  NR_DOWNLOAD_COUNT = NR_DOWNLOAD_COUNT + 1,
  DT_UPDATED_UTC = SYSUTCDATETIME()
WHERE ID_LOG_DATASUS_DBC_FILE = @p1`
		_, err = tx.ExecContext(ctx, upd,
			fileID.String(),
			ftpDir, fileName,
			catalog, state, year, month, seg,
			remoteSize, nullableTime(remoteModUTC),
			*localPath, *localSize,
			sha256hex,
			finished.UTC(),
		)
		if err != nil {
			return fmt.Errorf("update registry: %w", err)
		}
	}

	return tx.Commit()
}

func nullableTime(t sql.NullTime) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC()
}
