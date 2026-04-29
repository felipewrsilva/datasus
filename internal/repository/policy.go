package repository

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"datasus/internal/domain"
	"datasus/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PolicyRepository stores global download policy selections.
type PolicyRepository struct {
	db *pgxpool.Pool
}

var knownPolicyCatalogs = []string{
	"AD", "RD", "SP", "AQ", "AM", "BI", "RJ", "PA", "AR", "ER", "PS", "AN",
}

func NewPolicyRepository(db *pgxpool.Pool) *PolicyRepository {
	return &PolicyRepository{db: db}
}

func (r *PolicyRepository) ensureGlobalPolicyTables(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS download_policy_catalogs (
			catalog CHAR(2) PRIMARY KEY
		)`); err != nil {
		return fmt.Errorf("ensure download_policy_catalogs: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS download_policy_years (
			year SMALLINT PRIMARY KEY CHECK (year >= 0 AND year <= 9999)
		)`); err != nil {
		return fmt.Errorf("ensure download_policy_years: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS download_policy_months (
			year SMALLINT NOT NULL CHECK (year >= 0 AND year <= 9999),
			month SMALLINT NOT NULL CHECK (month >= 1 AND month <= 12),
			PRIMARY KEY (year, month)
		)`); err != nil {
		return fmt.Errorf("ensure download_policy_months: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS processing_policy_config (
			id SMALLINT PRIMARY KEY CHECK (id = 1),
			enable_download BOOLEAN NOT NULL DEFAULT TRUE,
			enable_csv BOOLEAN NOT NULL DEFAULT TRUE,
			enable_parquet BOOLEAN NOT NULL DEFAULT TRUE,
			download_dir TEXT NULL,
			csv_dir TEXT NULL,
			parquet_dir TEXT NULL
		)`); err != nil {
		return fmt.Errorf("ensure processing_policy_config: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE processing_policy_config
			ADD COLUMN IF NOT EXISTS download_dir TEXT NULL,
			ADD COLUMN IF NOT EXISTS csv_dir TEXT NULL,
			ADD COLUMN IF NOT EXISTS parquet_dir TEXT NULL
	`); err != nil {
		return fmt.Errorf("ensure processing_policy_config columns: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO processing_policy_config (id, enable_download, enable_csv, enable_parquet)
		VALUES (1, TRUE, TRUE, TRUE)
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed processing_policy_config: %w", err)
	}
	return nil
}

// YearMonth is a calendar month in policy period selections.
type YearMonth struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

// PolicyPeriods stores global period selections.
type PolicyPeriods struct {
	Years  []int       `json:"years"`
	Months []YearMonth `json:"months"`
}

type AvailablePeriods struct {
	Years  []int       `json:"years"`
	Months []YearMonth `json:"months"`
}

// ProcessingStages defines which pipeline stages are enabled globally.
type ProcessingStages struct {
	EnableDownload bool `json:"enable_download"`
	EnableCSV      bool `json:"enable_csv"`
	EnableParquet  bool `json:"enable_parquet"`
}

type ProcessingDirectories struct {
	DownloadDir *string `json:"download_dir,omitempty"`
	CSVDir      *string `json:"csv_dir,omitempty"`
	ParquetDir  *string `json:"parquet_dir,omitempty"`
}

// GlobalPolicy is the API shape for processing policy configuration.
type GlobalPolicy struct {
	AvailableCatalogs []string              `json:"available_catalogs"`
	AvailablePeriods  AvailablePeriods      `json:"available_periods"`
	SelectedCatalogs  []string              `json:"selected_catalogs"`
	SelectedPeriods   PolicyPeriods         `json:"selected_periods"`
	Processing        ProcessingStages      `json:"processing"`
	Directories       ProcessingDirectories `json:"directories"`
}

func normalizeCatalog(c string) string {
	return strings.ToUpper(strings.TrimSpace(c))
}

func (r *PolicyRepository) listAvailableCatalogs(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c FROM (
			SELECT DISTINCT UPPER(TRIM(f.catalog::text))::text AS c FROM files f
			UNION
			SELECT UPPER(TRIM(c.catalog::text))::text FROM download_policy_catalogs c
		) t
		WHERE c IS NOT NULL AND btrim(c) != ''
		ORDER BY 1`)
	if err != nil {
		return nil, fmt.Errorf("list available catalogs: %w", err)
	}
	var catalogs []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return nil, err
		}
		catalogs = append(catalogs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(catalogs))
	for _, c := range catalogs {
		seen[c] = struct{}{}
	}
	for _, known := range knownPolicyCatalogs {
		if _, ok := seen[known]; ok {
			continue
		}
		catalogs = append(catalogs, known)
	}
	slices.Sort(catalogs)
	return catalogs, nil
}

// GetPolicies returns the global policy and available catalogs for selection.
func (r *PolicyRepository) GetPolicies(ctx context.Context) (GlobalPolicy, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return GlobalPolicy{}, err
	}
	availableCatalogs, err := r.listAvailableCatalogs(ctx)
	if err != nil {
		return GlobalPolicy{}, err
	}
	availablePeriods, err := r.listAvailablePeriods(ctx)
	if err != nil {
		return GlobalPolicy{}, err
	}

	selectedCatalogRows, err := r.db.Query(ctx, `
		SELECT catalog::text FROM download_policy_catalogs ORDER BY catalog`)
	if err != nil {
		return GlobalPolicy{}, fmt.Errorf("list selected catalogs: %w", err)
	}
	selectedCatalogs := make([]string, 0)
	for selectedCatalogRows.Next() {
		var catalog string
		if err := selectedCatalogRows.Scan(&catalog); err != nil {
			selectedCatalogRows.Close()
			return GlobalPolicy{}, err
		}
		selectedCatalogs = append(selectedCatalogs, strings.ToUpper(strings.TrimSpace(catalog)))
	}
	selectedCatalogRows.Close()
	if err := selectedCatalogRows.Err(); err != nil {
		return GlobalPolicy{}, err
	}

	yearRows, err := r.db.Query(ctx, `
		SELECT year FROM download_policy_years ORDER BY year`)
	if err != nil {
		return GlobalPolicy{}, fmt.Errorf("list selected years: %w", err)
	}
	selectedYears := make([]int, 0)
	for yearRows.Next() {
		var year int
		if err := yearRows.Scan(&year); err != nil {
			yearRows.Close()
			return GlobalPolicy{}, err
		}
		selectedYears = append(selectedYears, year)
	}
	yearRows.Close()
	if err := yearRows.Err(); err != nil {
		return GlobalPolicy{}, err
	}

	monthRows, err := r.db.Query(ctx, `
		SELECT year, month FROM download_policy_months ORDER BY year, month`)
	if err != nil {
		return GlobalPolicy{}, fmt.Errorf("list selected months: %w", err)
	}
	selectedMonths := make([]YearMonth, 0)
	for monthRows.Next() {
		var ym YearMonth
		if err := monthRows.Scan(&ym.Year, &ym.Month); err != nil {
			monthRows.Close()
			return GlobalPolicy{}, err
		}
		selectedMonths = append(selectedMonths, ym)
	}
	monthRows.Close()
	if err := monthRows.Err(); err != nil {
		return GlobalPolicy{}, err
	}
	var processing ProcessingStages
	var directories ProcessingDirectories
	if err := r.db.QueryRow(ctx, `
		SELECT enable_download, enable_csv, enable_parquet, download_dir, csv_dir, parquet_dir
		FROM processing_policy_config
		WHERE id = 1`,
	).Scan(
		&processing.EnableDownload,
		&processing.EnableCSV,
		&processing.EnableParquet,
		&directories.DownloadDir,
		&directories.CSVDir,
		&directories.ParquetDir,
	); err != nil {
		return GlobalPolicy{}, fmt.Errorf("read processing policy config: %w", err)
	}

	return GlobalPolicy{
		AvailableCatalogs: availableCatalogs,
		AvailablePeriods:  availablePeriods,
		SelectedCatalogs:  selectedCatalogs,
		SelectedPeriods: PolicyPeriods{
			Years:  selectedYears,
			Months: selectedMonths,
		},
		Processing:  processing,
		Directories: directories,
	}, nil
}

func validateYear(year int) error {
	if year < 0 || year > 9999 {
		return fmt.Errorf("invalid year: %d", year)
	}
	return nil
}

func validateYearMonth(ym YearMonth) error {
	if err := validateYear(ym.Year); err != nil {
		return err
	}
	if ym.Month < 1 || ym.Month > 12 {
		return fmt.Errorf("invalid month: %d", ym.Month)
	}
	return nil
}

func validateProcessingStages(stages ProcessingStages) error {
	if stages.EnableCSV && !stages.EnableDownload {
		return fmt.Errorf("invalid processing policy: csv requires download enabled")
	}
	if stages.EnableParquet && !stages.EnableDownload {
		return fmt.Errorf("invalid processing policy: parquet requires download enabled")
	}
	return nil
}

// ReplacePolicies overwrites global selected catalogs and periods in one transaction.
func (r *PolicyRepository) ReplacePolicies(ctx context.Context, in GlobalPolicy) error {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return err
	}
	if err := validateProcessingStages(in.Processing); err != nil {
		return err
	}
	normalizedDirs, err := storage.NormalizePolicyDirectories(storage.ProcessingDirectories{
		DownloadDir: in.Directories.DownloadDir,
		CSVDir:      in.Directories.CSVDir,
		ParquetDir:  in.Directories.ParquetDir,
	})
	if err != nil {
		return err
	}
	for name, ptr := range map[string]*string{
		"download_dir": normalizedDirs.DownloadDir,
		"csv_dir":      normalizedDirs.CSVDir,
		"parquet_dir":  normalizedDirs.ParquetDir,
	} {
		if ptr == nil {
			continue
		}
		if err := storage.ValidateDirectoryAccess(*ptr, true); err != nil {
			return fmt.Errorf("%s: access check failed: %w", name, err)
		}
	}
	availablePeriods, err := r.listAvailablePeriods(ctx)
	if err != nil {
		return err
	}
	availableYearSet := make(map[int]struct{}, len(availablePeriods.Years))
	for _, year := range availablePeriods.Years {
		availableYearSet[year] = struct{}{}
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM download_policy_months`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM download_policy_years`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM download_policy_catalogs`); err != nil {
		return err
	}

	seenCatalogs := map[string]struct{}{}
	for _, rawCatalog := range in.SelectedCatalogs {
		catalog := normalizeCatalog(rawCatalog)
		if len(catalog) != 2 {
			return fmt.Errorf("invalid catalog: %q", rawCatalog)
		}
		if _, exists := seenCatalogs[catalog]; exists {
			continue
		}
		seenCatalogs[catalog] = struct{}{}
		if _, err := tx.Exec(ctx, `
			INSERT INTO download_policy_catalogs (catalog) VALUES ($1::char(2))`,
			catalog,
		); err != nil {
			return err
		}
	}

	seenYears := map[int]struct{}{}
	for _, year := range in.SelectedPeriods.Years {
		if err := validateYear(year); err != nil {
			return err
		}
		if _, ok := availableYearSet[year]; !ok {
			return fmt.Errorf("year not in periods discovered from stored files: %d", year)
		}
		if _, exists := seenYears[year]; exists {
			continue
		}
		seenYears[year] = struct{}{}
		if _, err := tx.Exec(ctx, `
			INSERT INTO download_policy_years (year) VALUES ($1)`,
			year,
		); err != nil {
			return err
		}
	}

	seenMonths := map[string]struct{}{}
	for _, ym := range in.SelectedPeriods.Months {
		if err := validateYearMonth(ym); err != nil {
			return err
		}
		if _, ok := availableYearSet[ym.Year]; !ok {
			return fmt.Errorf("month year not in periods discovered from stored files: %d-%02d", ym.Year, ym.Month)
		}
		key := fmt.Sprintf("%d-%02d", ym.Year, ym.Month)
		if _, exists := seenMonths[key]; exists {
			continue
		}
		seenMonths[key] = struct{}{}
		if _, err := tx.Exec(ctx, `
			INSERT INTO download_policy_months (year, month) VALUES ($1, $2)`,
			ym.Year, ym.Month,
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE processing_policy_config
		SET enable_download = $1,
		    enable_csv = $2,
		    enable_parquet = $3,
		    download_dir = $4,
		    csv_dir = $5,
		    parquet_dir = $6
		WHERE id = 1`,
		in.Processing.EnableDownload, in.Processing.EnableCSV, in.Processing.EnableParquet,
		normalizedDirs.DownloadDir, normalizedDirs.CSVDir, normalizedDirs.ParquetDir,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if _, _, err = r.ReconcileIgnoredStatuses(ctx); err != nil {
		return err
	}
	_, err = r.EnqueuePendingDownloadsByPolicy(ctx)
	return err
}

// ProcessingStages returns the currently configured global stage toggles.
func (r *PolicyRepository) ProcessingStages(ctx context.Context) (ProcessingStages, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return ProcessingStages{}, err
	}
	var stages ProcessingStages
	if err := r.db.QueryRow(ctx, `
		SELECT enable_download, enable_csv, enable_parquet
		FROM processing_policy_config
		WHERE id = 1`,
	).Scan(&stages.EnableDownload, &stages.EnableCSV, &stages.EnableParquet); err != nil {
		return ProcessingStages{}, err
	}
	return stages, nil
}

// ProcessingDirectories returns the configured optional directories.
func (r *PolicyRepository) ProcessingDirectories(ctx context.Context) (ProcessingDirectories, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return ProcessingDirectories{}, err
	}
	var dirs ProcessingDirectories
	if err := r.db.QueryRow(ctx, `
		SELECT download_dir, csv_dir, parquet_dir
		FROM processing_policy_config
		WHERE id = 1`,
	).Scan(&dirs.DownloadDir, &dirs.CSVDir, &dirs.ParquetDir); err != nil {
		return ProcessingDirectories{}, err
	}
	return dirs, nil
}

// policySelectionSummary returns how many catalogs and period rows are selected in the global policy.
func (r *PolicyRepository) policySelectionSummary(ctx context.Context) (catalogCount int, periodCount int, err error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return 0, 0, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(1) FROM download_policy_catalogs),
			(SELECT (SELECT COUNT(1) FROM download_policy_years) + (SELECT COUNT(1) FROM download_policy_months))`,
	).Scan(&catalogCount, &periodCount)
	if err != nil {
		return 0, 0, err
	}
	return catalogCount, periodCount, nil
}

// PolicySelectionComplete is true when at least one catalog and at least one period (year and/or month row) are configured.
func (r *PolicyRepository) PolicySelectionComplete(ctx context.Context) (bool, error) {
	c, p, err := r.policySelectionSummary(ctx)
	if err != nil {
		return false, err
	}
	return c > 0 && p > 0, nil
}

func (r *PolicyRepository) PolicyAllows(ctx context.Context, catalog string, year, month int) (bool, error) {
	selectedCatalogCount, selectedPeriodCount, err := r.policySelectionSummary(ctx)
	if err != nil {
		return false, err
	}
	if selectedCatalogCount == 0 || selectedPeriodCount == 0 {
		return false, nil
	}

	cat := normalizeCatalog(catalog)
	var catalogSelected bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM download_policy_catalogs WHERE catalog = $1::char(2))`, cat,
	).Scan(&catalogSelected); err != nil {
		return false, err
	}
	if !catalogSelected {
		return false, nil
	}

	var yearSelected bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM download_policy_years WHERE year = $1)`, year,
	).Scan(&yearSelected); err != nil {
		return false, err
	}
	if yearSelected {
		return true, nil
	}

	var monthSelected bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM download_policy_months WHERE year = $1 AND month = $2)`, year, month,
	).Scan(&monthSelected); err != nil {
		return false, err
	}
	return monthSelected, nil
}

// PolicySnapshot is an in-memory copy of the global selection used to evaluate
// policy decisions for many files in a single scan without per-row round trips.
type PolicySnapshot struct {
	HasSelection bool
	catalogs     map[string]struct{}
	years        map[int]struct{}
	months       map[int]map[int]struct{}
}

// Allows reports whether (catalog, year, month) is selected by the snapshot.
// Mirrors PolicyAllows: empty selection denies all; year selected covers all
// months of that year; otherwise the (year, month) pair must be selected.
func (s PolicySnapshot) Allows(catalog string, year, month int) bool {
	if !s.HasSelection {
		return false
	}
	cat := normalizeCatalog(catalog)
	if _, ok := s.catalogs[cat]; !ok {
		return false
	}
	if _, ok := s.years[year]; ok {
		return true
	}
	if mset, ok := s.months[year]; ok {
		_, ok := mset[month]
		return ok
	}
	return false
}

// LoadPolicySnapshot loads catalogs/years/months selections in three queries.
// Returned snapshot has HasSelection=false when no catalog or no period is set.
func (r *PolicyRepository) LoadPolicySnapshot(ctx context.Context) (PolicySnapshot, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return PolicySnapshot{}, err
	}
	snap := PolicySnapshot{
		catalogs: map[string]struct{}{},
		years:    map[int]struct{}{},
		months:   map[int]map[int]struct{}{},
	}

	catRows, err := r.db.Query(ctx, `SELECT catalog::text FROM download_policy_catalogs`)
	if err != nil {
		return PolicySnapshot{}, fmt.Errorf("load policy catalogs: %w", err)
	}
	for catRows.Next() {
		var c string
		if err := catRows.Scan(&c); err != nil {
			catRows.Close()
			return PolicySnapshot{}, err
		}
		snap.catalogs[normalizeCatalog(c)] = struct{}{}
	}
	catRows.Close()
	if err := catRows.Err(); err != nil {
		return PolicySnapshot{}, err
	}

	yearRows, err := r.db.Query(ctx, `SELECT year FROM download_policy_years`)
	if err != nil {
		return PolicySnapshot{}, fmt.Errorf("load policy years: %w", err)
	}
	for yearRows.Next() {
		var y int
		if err := yearRows.Scan(&y); err != nil {
			yearRows.Close()
			return PolicySnapshot{}, err
		}
		snap.years[y] = struct{}{}
	}
	yearRows.Close()
	if err := yearRows.Err(); err != nil {
		return PolicySnapshot{}, err
	}

	monthRows, err := r.db.Query(ctx, `SELECT year, month FROM download_policy_months`)
	if err != nil {
		return PolicySnapshot{}, fmt.Errorf("load policy months: %w", err)
	}
	for monthRows.Next() {
		var y, m int
		if err := monthRows.Scan(&y, &m); err != nil {
			monthRows.Close()
			return PolicySnapshot{}, err
		}
		set := snap.months[y]
		if set == nil {
			set = map[int]struct{}{}
			snap.months[y] = set
		}
		set[m] = struct{}{}
	}
	monthRows.Close()
	if err := monthRows.Err(); err != nil {
		return PolicySnapshot{}, err
	}

	hasPeriod := len(snap.years) > 0 || len(snap.months) > 0
	snap.HasSelection = len(snap.catalogs) > 0 && hasPeriod
	return snap, nil
}

// PendingAndIgnoredCounts returns counts from persisted status values.
func (r *PolicyRepository) PendingAndIgnoredCounts(ctx context.Context) (int64, int64, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return 0, 0, err
	}
	const q = `
		SELECT
			COUNT(*) FILTER (WHERE f.overall_status = $1) AS pending_policy,
			COUNT(*) FILTER (WHERE f.overall_status = $2) AS ignored_policy
		FROM files f`
	var pendingPolicy, ignoredPolicy int64
	if err := r.db.QueryRow(ctx, q, domain.StatusPending, domain.StatusIgnored).Scan(&pendingPolicy, &ignoredPolicy); err != nil {
		return 0, 0, err
	}
	return pendingPolicy, ignoredPolicy, nil
}

// ReconcileIgnoredStatuses updates persisted status based on current policy.
// Returns (toIgnored, toPending, error).
func (r *PolicyRepository) ReconcileIgnoredStatuses(ctx context.Context) (int64, int64, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return 0, 0, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const policyPredicate = `
		EXISTS(
			SELECT 1
			FROM download_policy_catalogs c
			WHERE c.catalog = UPPER(TRIM(files.catalog::text))::char(2)
		)
		AND (
			EXISTS(SELECT 1 FROM download_policy_years y WHERE y.year = files.year)
			OR EXISTS(
				SELECT 1
				FROM download_policy_months m
				WHERE m.year = files.year AND m.month = files.month
			)
		)
	`

	toIgnoredTag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE files
		SET overall_status = $1, updated_at = now()
		WHERE overall_status IN ($2, $3, $4)
		  AND NOT (%s)
	`, policyPredicate), domain.StatusIgnored, domain.StatusPending, domain.StatusDownloaded, domain.StatusCSVReady)
	if err != nil {
		return 0, 0, err
	}

	toPendingTag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE files
		SET overall_status = $1, updated_at = now()
		WHERE overall_status = $2
		  AND (%s)
	`, policyPredicate), domain.StatusPending, domain.StatusIgnored)
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return toIgnoredTag.RowsAffected(), toPendingTag.RowsAffected(), nil
}

// EnqueuePendingDownloadsByPolicy ensures policy-eligible pending files are ready and queued for download.
// Returns how many download jobs were inserted/updated in job_queue.
func (r *PolicyRepository) EnqueuePendingDownloadsByPolicy(ctx context.Context) (int64, error) {
	if err := r.ensureGlobalPolicyTables(ctx); err != nil {
		return 0, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const policyPredicate = `
		EXISTS(
			SELECT 1
			FROM download_policy_catalogs c
			WHERE c.catalog = UPPER(TRIM(files.catalog::text))::char(2)
		)
		AND (
			EXISTS(SELECT 1 FROM download_policy_years y WHERE y.year = files.year)
			OR EXISTS(
				SELECT 1
				FROM download_policy_months m
				WHERE m.year = files.year AND m.month = files.month
			)
		)
	`

	// Ensure stage rows exist for all policy-eligible pending files.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO file_stages (file_id, stage, status)
		SELECT files.id, s.stage::stage_name, 'pending'::stage_status
		FROM files
		CROSS JOIN (VALUES ('download'), ('csv_conversion'), ('parquet_conversion')) AS s(stage)
		WHERE files.overall_status = $1
		  AND (%s)
		ON CONFLICT (file_id, stage) DO NOTHING
	`, policyPredicate), domain.StatusPending); err != nil {
		return 0, err
	}

	// Download must restart from pending when a file is re-eligible by policy.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE file_stages
		SET status='pending', started_at=NULL, finished_at=NULL, error_message=NULL, updated_at=now()
		FROM files
		WHERE file_stages.file_id = files.id
		  AND file_stages.stage = 'download'
		  AND files.overall_status = $1
		  AND (%s)
	`, policyPredicate), domain.StatusPending); err != nil {
		return 0, err
	}

	enqueuedTag, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO job_queue (file_id, stage, status, available_at)
		SELECT files.id, $1, 'pending', now()
		FROM files
		WHERE files.overall_status = $2
		  AND (%s)
		ON CONFLICT (file_id, stage) DO UPDATE
		    SET status='pending', available_at=now(), locked_at=NULL, locked_by=NULL, attempts=0, updated_at=now()
		    WHERE job_queue.status IN ('failed', 'done', 'pending')
	`, policyPredicate), domain.StageDownload, domain.StatusPending)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return enqueuedTag.RowsAffected(), nil
}

func (r *PolicyRepository) listAvailablePeriods(ctx context.Context) (AvailablePeriods, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT year, month
		FROM files
		WHERE year >= 0 AND month BETWEEN 1 AND 12
		ORDER BY year DESC, month DESC`)
	if err != nil {
		return AvailablePeriods{}, fmt.Errorf("list available periods: %w", err)
	}
	defer rows.Close()

	months := make([]YearMonth, 0, 64)
	yearSeen := make(map[int]struct{})
	years := make([]int, 0, 16)
	for rows.Next() {
		var ym YearMonth
		if err := rows.Scan(&ym.Year, &ym.Month); err != nil {
			return AvailablePeriods{}, err
		}
		months = append(months, ym)
		if _, ok := yearSeen[ym.Year]; !ok {
			yearSeen[ym.Year] = struct{}{}
			years = append(years, ym.Year)
		}
	}
	if err := rows.Err(); err != nil {
		return AvailablePeriods{}, err
	}

	return AvailablePeriods{
		Years:  years,
		Months: months,
	}, nil
}
