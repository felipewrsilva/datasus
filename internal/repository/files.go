package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"datasus/internal/domain"
)

type FileRepository struct {
	db *pgxpool.Pool
}

type CountBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type StateSizeBucket struct {
	Key            string `json:"key"`
	Count          int64  `json:"count"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
	AvgSizeBytes   int64  `json:"avg_size_bytes"`
}

type FilePeriod struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

type FileFacets struct {
	Catalogs []string     `json:"catalogs"`
	States   []string     `json:"states"`
	Statuses []string     `json:"statuses"`
	Periods  []FilePeriod `json:"periods"`
}

func NewFileRepository(db *pgxpool.Pool) *FileRepository {
	return &FileRepository{db: db}
}

// ListFilters holds optional filter criteria for listing files.
type ListFilters struct {
	Catalog         string
	Catalogs        []string
	State           string
	States          []string
	Year            *int
	Month           *int
	PeriodFromYear  *int
	PeriodFromMonth *int
	PeriodToYear    *int
	PeriodToMonth   *int
	FTPDir          string
	Filename        string
	OverallStatus   domain.OverallStatus
	OverallStatuses []domain.OverallStatus
	// PolicyMatch is kept for backward compatibility with old dashboard links.
	// pending/ignored now map directly to overall_status.
	PolicyMatch string
	// PipelineCompleted, when true, keeps only files whose stages satisfy the same
	// predicate as dashboard pipeline_completed_count (see pipelineStageFlagsAndEvalCTEs).
	PipelineCompleted bool
	RequireDownload   bool
	RequireCSV        bool
	RequireParquet    bool
	StageStatus       domain.StageStatus
	StageName         domain.StageName
	SortBy            string
	SortDir           string
	Limit             int
	Offset            int
}

func (r *FileRepository) GetByID(ctx context.Context, id string) (*domain.File, error) {
	const q = `
		SELECT id, filename, catalog, state, year, month, ftp_dir, ftp_path,
		       size_bytes, remote_checksum, remote_timestamp, local_hash,
		       root_path, dbc_path, csv_path, parquet_path,
		       overall_status, created_at, updated_at, last_seen_at
		FROM files WHERE id = $1`

	row := r.db.QueryRow(ctx, q, id)
	f, err := scanFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return f, err
}

func (r *FileRepository) GetByFilename(ctx context.Context, filename string) (*domain.File, error) {
	const q = `
		SELECT id, filename, catalog, state, year, month, ftp_dir, ftp_path,
		       size_bytes, remote_checksum, remote_timestamp, local_hash,
		       root_path, dbc_path, csv_path, parquet_path,
		       overall_status, created_at, updated_at, last_seen_at
		FROM files WHERE filename = $1`

	row := r.db.QueryRow(ctx, q, filename)
	f, err := scanFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return f, err
}

func (r *FileRepository) List(ctx context.Context, f ListFilters) ([]*domain.File, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}

	args := []any{}
	where := "WHERE 1=1"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND %s = $%d", cond, len(args))
	}
	addFilenameGlob := func(col, v string) {
		pat := FilenameGlobToILike(v)
		args = append(args, pat)
		where += fmt.Sprintf(" AND %s ILIKE $%d ESCAPE '%c'", col, len(args), LIKEEscapeChar)
	}

	// Prefer multi-value catalogs only; duplicate Catalog + Catalogs breaks AND.
	if len(f.Catalogs) > 0 {
		args = append(args, f.Catalogs)
		where += fmt.Sprintf(" AND catalog = ANY($%d)", len(args))
	} else if f.Catalog != "" {
		add("catalog", f.Catalog)
	}
	if len(f.States) > 0 {
		args = append(args, f.States)
		where += fmt.Sprintf(" AND state = ANY($%d)", len(args))
	} else if f.State != "" {
		add("state", f.State)
	}
	if f.Year != nil {
		add("year", *f.Year)
	}
	if f.Month != nil {
		add("month", *f.Month)
	}
	if f.PeriodFromYear != nil && f.PeriodFromMonth != nil {
		args = append(args, *f.PeriodFromYear, *f.PeriodFromMonth)
		where += fmt.Sprintf(" AND (year > $%d OR (year = $%d AND month >= $%d))", len(args)-1, len(args)-1, len(args))
	}
	if f.PeriodToYear != nil && f.PeriodToMonth != nil {
		args = append(args, *f.PeriodToYear, *f.PeriodToMonth)
		where += fmt.Sprintf(" AND (year < $%d OR (year = $%d AND month <= $%d))", len(args)-1, len(args)-1, len(args))
	}
	if f.FTPDir != "" {
		add("ftp_dir", f.FTPDir)
	}
	if f.Filename != "" {
		addFilenameGlob("filename", f.Filename)
	}
	if len(f.OverallStatuses) > 0 {
		statuses := make([]string, 0, len(f.OverallStatuses))
		for _, s := range f.OverallStatuses {
			if s != "" {
				statuses = append(statuses, string(s))
			}
		}
		if len(statuses) > 0 {
			args = append(args, statuses)
			where += fmt.Sprintf(" AND overall_status = ANY($%d)", len(args))
		}
	} else if f.OverallStatus != "" {
		add("overall_status", string(f.OverallStatus))
	}

	switch strings.ToLower(strings.TrimSpace(f.PolicyMatch)) {
	case "pending":
		add("overall_status", string(domain.StatusPending))
	case "ignored":
		add("overall_status", string(domain.StatusIgnored))
	}

	if f.PipelineCompleted {
		p1 := len(args) + 1
		p2 := len(args) + 2
		p3 := len(args) + 3
		args = append(args, f.RequireDownload, f.RequireCSV, f.RequireParquet)
		where += fmt.Sprintf(
			` AND id IN (WITH %s SELECT id FROM eval WHERE pipeline_completed)`,
			pipelineStageFlagsAndEvalCTEs(p1, p2, p3),
		)
	}

	sortDir := "DESC"
	if f.SortDir == "asc" || f.SortDir == "ASC" {
		sortDir = "ASC"
	}
	// Matches UI: remote FTP time when set, else last ingestion time (see web displayFileTimestamp).
	orderBy := fmt.Sprintf("COALESCE(remote_timestamp, last_seen_at) %s", sortDir)
	switch f.SortBy {
	case "year_month":
		orderBy = fmt.Sprintf("year %s, month %s", sortDir, sortDir)
	case "filename", "catalog", "state", "year", "month", "overall_status", "updated_at", "created_at":
		orderBy = f.SortBy + " " + sortDir
	}

	// Count
	var total int
	countQ := "SELECT COUNT(*) FROM files " + where
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count files: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	listQ := fmt.Sprintf(`
		SELECT id, filename, catalog, state, year, month, ftp_dir, ftp_path,
		       size_bytes, remote_checksum, remote_timestamp, local_hash,
		       root_path, dbc_path, csv_path, parquet_path,
		       overall_status, created_at, updated_at, last_seen_at
		FROM files %s
		ORDER BY %s, id ASC
		LIMIT $%d OFFSET $%d`, where, orderBy, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	var files []*domain.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, 0, err
		}
		files = append(files, f)
	}
	return files, total, rows.Err()
}

func (r *FileRepository) FindByIDs(ctx context.Context, ids []string) ([]*domain.File, error) {
	if len(ids) == 0 {
		return []*domain.File{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, filename, catalog, state, year, month, ftp_dir, ftp_path,
		       size_bytes, remote_checksum, remote_timestamp, local_hash,
		       root_path, dbc_path, csv_path, parquet_path,
		       overall_status, created_at, updated_at, last_seen_at
		FROM files WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("find by ids: %w", err)
	}
	defer rows.Close()

	var files []*domain.File
	for rows.Next() {
		f, scanErr := scanFile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (r *FileRepository) FindByFilters(ctx context.Context, f ListFilters, maxRows int) ([]*domain.File, error) {
	if maxRows <= 0 {
		maxRows = 5000
	}
	offset := 0
	limit := 500
	out := make([]*domain.File, 0, min(maxRows, 512))
	for {
		f.Offset = offset
		f.Limit = limit
		page, _, err := r.List(ctx, f)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return out, nil
		}
		for _, item := range page {
			if len(out) >= maxRows {
				return out, nil
			}
			out = append(out, item)
		}
		offset += len(page)
	}
}

func (r *FileRepository) ListFailedSince(ctx context.Context, since time.Time, stage domain.StageName) ([]*domain.File, error) {
	rows, err := r.db.Query(ctx, `
		SELECT f.id, f.filename, f.catalog, f.state, f.year, f.month, f.ftp_dir, f.ftp_path,
		       f.size_bytes, f.remote_checksum, f.remote_timestamp, f.local_hash,
		       f.root_path, f.dbc_path, f.csv_path, f.parquet_path,
		       f.overall_status, f.created_at, f.updated_at, f.last_seen_at
		FROM files f
		JOIN file_stages s ON s.file_id = f.id
		WHERE s.status='failed' AND s.stage=$1 AND s.updated_at >= $2
		ORDER BY s.updated_at DESC
		LIMIT 1000`, stage, since)
	if err != nil {
		return nil, fmt.Errorf("list failed since: %w", err)
	}
	defer rows.Close()

	files := make([]*domain.File, 0, 128)
	for rows.Next() {
		item, scanErr := scanFile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		files = append(files, item)
	}
	return files, rows.Err()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// UpsertFromFTP inserts or updates a file discovered during an FTP scan.
// Returns (file, changed) where changed=true means the remote content differs.
func (r *FileRepository) UpsertFromFTP(ctx context.Context, params UpsertFTPParams) (*domain.File, bool, error) {
	const q = `
		INSERT INTO files (filename, catalog, state, year, month, ftp_dir, ftp_path,
		                   size_bytes, remote_checksum, remote_timestamp,
		                   root_path, last_seen_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())
		ON CONFLICT (filename) DO UPDATE
		    SET ftp_path          = EXCLUDED.ftp_path,
		        size_bytes        = EXCLUDED.size_bytes,
		        remote_checksum   = EXCLUDED.remote_checksum,
		        remote_timestamp  = EXCLUDED.remote_timestamp,
		        last_seen_at      = now(),
		        updated_at        = now()
		RETURNING id, filename, catalog, state, year, month, ftp_dir, ftp_path,
		          size_bytes, remote_checksum, remote_timestamp, local_hash,
		          root_path, dbc_path, csv_path, parquet_path,
		          overall_status, created_at, updated_at, last_seen_at,
		          (xmax = 0) AS is_new,
		          (remote_checksum IS DISTINCT FROM $9 OR remote_timestamp IS DISTINCT FROM $10) AS content_changed`

	var isNew, contentChanged bool
	row := r.db.QueryRow(ctx, q,
		params.Filename, params.Catalog, params.State, params.Year, params.Month,
		params.FTPDir, params.FTPPath, params.SizeBytes,
		params.RemoteChecksum, params.RemoteTimestamp, params.RootPath,
	)

	// scanFile doesn't handle the extra boolean columns, so scan manually
	f := &domain.File{}
	var sizeBytes *int64
	var remoteChecksum, localHash *string
	var remoteTimestamp *time.Time
	var dbcPath, csvPath, parquetPath *string

	err := row.Scan(
		&f.ID, &f.Filename, &f.Catalog, &f.State, &f.Year, &f.Month,
		&f.FTPDir, &f.FTPPath, &sizeBytes, &remoteChecksum, &remoteTimestamp,
		&localHash, &f.RootPath, &dbcPath, &csvPath, &parquetPath,
		&f.OverallStatus, &f.CreatedAt, &f.UpdatedAt, &f.LastSeenAt,
		&isNew, &contentChanged,
	)
	if err != nil {
		return nil, false, fmt.Errorf("upsert file: %w", err)
	}
	f.SizeBytes = sizeBytes
	f.RemoteChecksum = remoteChecksum
	f.RemoteTimestamp = remoteTimestamp
	f.LocalHash = localHash
	f.DBCPath = dbcPath
	f.CSVPath = csvPath
	f.ParquetPath = parquetPath

	return f, isNew || contentChanged, nil
}

type UpsertFTPParams struct {
	Filename        string
	Catalog         string
	State           string
	Year            int
	Month           int
	FTPDir          string
	FTPPath         string
	SizeBytes       *int64
	RemoteChecksum  *string
	RemoteTimestamp *time.Time
	RootPath        string
}

func (r *FileRepository) UpdateStatus(ctx context.Context, id string, status domain.OverallStatus) error {
	_, err := r.db.Exec(ctx,
		`UPDATE files SET overall_status=$2, updated_at=now() WHERE id=$1`, id, string(status))
	return err
}

func (r *FileRepository) UpdatePaths(ctx context.Context, id string, dbc, csv, parquet *string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE files
		SET dbc_path=$2, csv_path=$3, parquet_path=$4, updated_at=now()
		WHERE id=$1`, id, dbc, csv, parquet)
	return err
}

func (r *FileRepository) UpdateLocalHash(ctx context.Context, id string, hash string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE files SET local_hash=$2, updated_at=now() WHERE id=$1`, id, hash)
	return err
}

func (r *FileRepository) MarkPurged(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE files
		SET overall_status='purged', dbc_path=NULL, csv_path=NULL, parquet_path=NULL,
		    updated_at=now()
		WHERE id=$1`, id)
	return err
}

// FindByPattern matches user glob (only * is special); see [FilenameGlobToILike].
func (r *FileRepository) FindByPattern(ctx context.Context, userPattern string) ([]*domain.File, error) {
	pat := FilenameGlobToILike(userPattern)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, filename, catalog, state, year, month, ftp_dir, ftp_path,
		       size_bytes, remote_checksum, remote_timestamp, local_hash,
		       root_path, dbc_path, csv_path, parquet_path,
		       overall_status, created_at, updated_at, last_seen_at
		FROM files WHERE filename ILIKE $1 ESCAPE '%c' ORDER BY filename`, LIKEEscapeChar), pat)
	if err != nil {
		return nil, fmt.Errorf("find by pattern: %w", err)
	}
	defer rows.Close()

	var files []*domain.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// Stats returns aggregate counts for the dashboard.
func (r *FileRepository) Stats(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT overall_status, COUNT(*) FROM files GROUP BY overall_status`)
	if err != nil {
		return nil, fmt.Errorf("stats query: %w", err)
	}
	defer rows.Close()

	m := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		m[status] = count
	}
	return m, rows.Err()
}

func (r *FileRepository) CountByCatalog(ctx context.Context) ([]CountBucket, error) {
	return r.countByColumn(ctx, "catalog")
}

func (r *FileRepository) CountByState(ctx context.Context) ([]CountBucket, error) {
	return r.countByColumn(ctx, "state")
}

func (r *FileRepository) SizeByState(ctx context.Context) ([]StateSizeBucket, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			UPPER(TRIM(state::text)) AS key,
			COUNT(*) AS count,
			COALESCE(SUM(size_bytes), 0)::bigint AS total_size_bytes,
			COALESCE(AVG(size_bytes), 0)::bigint AS avg_size_bytes
		FROM files
		GROUP BY 1
		ORDER BY 3 DESC, 2 DESC, 1 ASC`)
	if err != nil {
		return nil, fmt.Errorf("size by state query: %w", err)
	}
	defer rows.Close()

	out := make([]StateSizeBucket, 0, 32)
	for rows.Next() {
		var b StateSizeBucket
		if err := rows.Scan(&b.Key, &b.Count, &b.TotalSizeBytes, &b.AvgSizeBytes); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SizeByCatalog aggregates file counts and byte sizes per catalog (same semantics as SizeByState).
func (r *FileRepository) SizeByCatalog(ctx context.Context) ([]StateSizeBucket, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			UPPER(TRIM(catalog::text)) AS key,
			COUNT(*) AS count,
			COALESCE(SUM(size_bytes), 0)::bigint AS total_size_bytes,
			COALESCE(AVG(size_bytes), 0)::bigint AS avg_size_bytes
		FROM files
		GROUP BY 1
		ORDER BY 3 DESC, 2 DESC, 1 ASC`)
	if err != nil {
		return nil, fmt.Errorf("size by catalog query: %w", err)
	}
	defer rows.Close()

	out := make([]StateSizeBucket, 0, 32)
	for rows.Next() {
		var b StateSizeBucket
		if err := rows.Scan(&b.Key, &b.Count, &b.TotalSizeBytes, &b.AvgSizeBytes); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *FileRepository) countByColumn(ctx context.Context, column string) ([]CountBucket, error) {
	if column != "catalog" && column != "state" {
		return nil, fmt.Errorf("unsupported count column: %s", column)
	}
	q := fmt.Sprintf(`
		SELECT UPPER(TRIM(%s::text)) AS key, COUNT(*) AS count
		FROM files
		GROUP BY 1
		ORDER BY 2 DESC, 1 ASC`, column)
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("count by %s: %w", column, err)
	}
	defer rows.Close()

	out := make([]CountBucket, 0, 32)
	for rows.Next() {
		var b CountBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *FileRepository) CountsByCatalog(ctx context.Context) ([]CountBucket, error) {
	rows, err := r.db.Query(ctx, `
		SELECT catalog, COUNT(*)
		FROM files
		GROUP BY catalog
		ORDER BY COUNT(*) DESC, catalog ASC`)
	if err != nil {
		return nil, fmt.Errorf("counts by catalog query: %w", err)
	}
	defer rows.Close()

	var out []CountBucket
	for rows.Next() {
		var b CountBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *FileRepository) CountsByState(ctx context.Context) ([]CountBucket, error) {
	rows, err := r.db.Query(ctx, `
		SELECT state, COUNT(*)
		FROM files
		GROUP BY state
		ORDER BY COUNT(*) DESC, state ASC`)
	if err != nil {
		return nil, fmt.Errorf("counts by state query: %w", err)
	}
	defer rows.Close()

	var out []CountBucket
	for rows.Next() {
		var b CountBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *FileRepository) ListFacets(ctx context.Context) (*FileFacets, error) {
	catalogs, err := r.listDistinctUpperValues(ctx, "catalog")
	if err != nil {
		return nil, err
	}
	states, err := r.listDistinctUpperValues(ctx, "state")
	if err != nil {
		return nil, err
	}
	statuses, err := r.listDistinctLowerValues(ctx, "overall_status")
	if err != nil {
		return nil, err
	}
	periods, err := r.listAvailablePeriods(ctx)
	if err != nil {
		return nil, err
	}
	return &FileFacets{
		Catalogs: catalogs,
		States:   states,
		Statuses: statuses,
		Periods:  periods,
	}, nil
}

func (r *FileRepository) listDistinctUpperValues(ctx context.Context, column string) ([]string, error) {
	if column != "catalog" && column != "state" {
		return nil, fmt.Errorf("unsupported distinct upper column: %s", column)
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT UPPER(TRIM(%s::text)) AS value
		FROM files
		WHERE %s IS NOT NULL AND TRIM(%s::text) <> ''
		ORDER BY 1 ASC`, column, column, column)
	return r.scanStringRows(ctx, q)
}

func (r *FileRepository) listDistinctLowerValues(ctx context.Context, column string) ([]string, error) {
	if column != "overall_status" {
		return nil, fmt.Errorf("unsupported distinct lower column: %s", column)
	}
	q := fmt.Sprintf(`
		SELECT DISTINCT LOWER(TRIM(%s::text)) AS value
		FROM files
		WHERE %s IS NOT NULL AND TRIM(%s::text) <> ''
		ORDER BY 1 ASC`, column, column, column)
	return r.scanStringRows(ctx, q)
}

func (r *FileRepository) scanStringRows(ctx context.Context, q string) ([]string, error) {
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]string, 0, 32)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *FileRepository) listAvailablePeriods(ctx context.Context) ([]FilePeriod, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT year, month
		FROM files
		WHERE month BETWEEN 1 AND 12
		ORDER BY year DESC, month DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	periods := make([]FilePeriod, 0, 64)
	for rows.Next() {
		var item FilePeriod
		if err := rows.Scan(&item.Year, &item.Month); err != nil {
			return nil, err
		}
		periods = append(periods, item)
	}
	return periods, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanFile(row scannable) (*domain.File, error) {
	f := &domain.File{}
	var sizeBytes *int64
	var remoteChecksum, localHash *string
	var remoteTimestamp *time.Time
	var dbcPath, csvPath, parquetPath *string

	err := row.Scan(
		&f.ID, &f.Filename, &f.Catalog, &f.State, &f.Year, &f.Month,
		&f.FTPDir, &f.FTPPath, &sizeBytes, &remoteChecksum, &remoteTimestamp,
		&localHash, &f.RootPath, &dbcPath, &csvPath, &parquetPath,
		&f.OverallStatus, &f.CreatedAt, &f.UpdatedAt, &f.LastSeenAt,
	)
	if err != nil {
		return nil, err
	}
	f.SizeBytes = sizeBytes
	f.RemoteChecksum = remoteChecksum
	f.RemoteTimestamp = remoteTimestamp
	f.LocalHash = localHash
	f.DBCPath = dbcPath
	f.CSVPath = csvPath
	f.ParquetPath = parquetPath
	return f, nil
}
