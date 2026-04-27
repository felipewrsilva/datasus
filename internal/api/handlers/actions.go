package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"datasus/internal/domain"
	"datasus/internal/ftp"
	"datasus/internal/queue"
	"datasus/internal/repository"
	"datasus/internal/storage"
)

// ActionsHandler handles all mutation/trigger endpoints.
type ActionsHandler struct {
	scanManager    *ftp.ScanManager
	fileRepo       *repository.FileRepository
	stageRepo      *repository.StageRepository
	logRepo        *repository.LogRepository
	queue          *queue.PostgresQueue
	policy         *repository.PolicyRepository
	pauseDownloads bool
}

func NewActionsHandler(
	scanManager *ftp.ScanManager,
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	logRepo *repository.LogRepository,
	q *queue.PostgresQueue,
	policy *repository.PolicyRepository,
	pauseDownloads bool,
) *ActionsHandler {
	return &ActionsHandler{
		scanManager:    scanManager,
		fileRepo:       fileRepo,
		stageRepo:      stageRepo,
		logRepo:        logRepo,
		queue:          q,
		policy:         policy,
		pauseDownloads: pauseDownloads,
	}
}

func (h *ActionsHandler) isDownloadPaused(stage domain.StageName) bool {
	return h.pauseDownloads && stage == domain.StageDownload
}

func (h *ActionsHandler) policyAllows(ctx context.Context, f *domain.File) (bool, error) {
	if h.policy == nil {
		return true, nil
	}
	return h.policy.PolicyAllows(ctx, f.Catalog, f.Year, f.Month)
}

func (h *ActionsHandler) stageEnabled(ctx context.Context, stage domain.StageName) (bool, error) {
	if h.policy == nil {
		return true, nil
	}
	processing, err := h.policy.ProcessingStages(ctx)
	if err != nil {
		return false, err
	}
	switch stage {
	case domain.StageDownload:
		return processing.EnableDownload, nil
	case domain.StageCSVConversion:
		return processing.EnableCSV, nil
	case domain.StageParquetConversion:
		return processing.EnableParquet, nil
	default:
		return true, nil
	}
}

func (h *ActionsHandler) requirePolicySelection(w http.ResponseWriter, r *http.Request) bool {
	if h.policy == nil {
		return true
	}
	complete, err := h.policy.PolicySelectionComplete(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return false
	}
	if !complete {
		jsonError(w, errPolicyIncomplete(), 403)
		return false
	}
	return true
}

// POST /api/scan
func (h *ActionsHandler) Scan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	snapshot, err := h.scanManager.Trigger("manual", "ui", body.Paths)
	if errors.Is(err, ftp.ErrScanAlreadyRunning) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accepted": false,
			"message":  "scan already running",
			"status":   snapshot,
		})
		return
	}
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": true,
		"message":  "scan scheduled",
		"status":   snapshot,
	})
}

// GET /api/scan/status
func (h *ActionsHandler) ScanStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.scanManager.Snapshot())
}

// POST /api/download
func (h *ActionsHandler) Download(w http.ResponseWriter, r *http.Request) {
	if !h.requirePolicySelection(w, r) {
		return
	}
	if h.isDownloadPaused(domain.StageDownload) {
		jsonError(w, errDownloadPaused(), 403)
		return
	}
	enabled, err := h.stageEnabled(r.Context(), domain.StageDownload)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !enabled {
		jsonError(w, errStageDisabled(domain.StageDownload), 403)
		return
	}
	var body struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Filename == "" {
		jsonError(w, errBadRequest("filename required"), 400)
		return
	}

	file, err := h.fileRepo.GetByFilename(r.Context(), strings.ToUpper(body.Filename))
	if err == domain.ErrNotFound {
		jsonError(w, err, 404)
		return
	}
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	allow, err := h.policyAllows(r.Context(), file)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !allow {
		jsonError(w, errPolicyDenied(), 403)
		return
	}

	if err := h.stageRepo.ResetForRetry(r.Context(), file.ID, domain.StageDownload); err != nil {
		jsonError(w, err, 500)
		return
	}
	if err := h.queue.Enqueue(r.Context(), file.ID, domain.StageDownload, time.Now()); err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"enqueued": file.Filename})
}

// POST /api/download/mask
func (h *ActionsHandler) DownloadMask(w http.ResponseWriter, r *http.Request) {
	h.triggerMask(w, r, domain.StageDownload)
}

// POST /api/convert/csv
func (h *ActionsHandler) ConvertCSV(w http.ResponseWriter, r *http.Request) {
	h.triggerSingle(w, r, domain.StageCSVConversion)
}

// POST /api/convert/csv/mask
func (h *ActionsHandler) ConvertCSVMask(w http.ResponseWriter, r *http.Request) {
	h.triggerMask(w, r, domain.StageCSVConversion)
}

// POST /api/convert/parquet
func (h *ActionsHandler) ConvertParquet(w http.ResponseWriter, r *http.Request) {
	h.triggerSingle(w, r, domain.StageParquetConversion)
}

// POST /api/convert/parquet/mask
func (h *ActionsHandler) ConvertParquetMask(w http.ResponseWriter, r *http.Request) {
	h.triggerMask(w, r, domain.StageParquetConversion)
}

// POST /api/actions/preview
func (h *ActionsHandler) PreviewBatch(w http.ResponseWriter, r *http.Request) {
	if !h.requirePolicySelection(w, r) {
		return
	}
	stage, files, err := h.resolveBatchTargets(r)
	if err != nil {
		jsonError(w, err, 400)
		return
	}
	eligible, blockedPolicy, blockedPurged := 0, 0, 0
	for _, file := range files {
		if file.OverallStatus == domain.StatusPurged {
			blockedPurged++
			continue
		}
		allow, perr := h.policyAllows(r.Context(), file)
		if perr != nil {
			jsonError(w, perr, 500)
			return
		}
		if !allow {
			blockedPolicy++
			continue
		}
		eligible++
	}
	jsonOK(w, map[string]any{
		"stage":             stage,
		"total_matched":     len(files),
		"eligible":          eligible,
		"blocked_by_policy": blockedPolicy,
		"blocked_purged":    blockedPurged,
	})
}

// POST /api/actions/enqueue
func (h *ActionsHandler) EnqueueBatch(w http.ResponseWriter, r *http.Request) {
	if !h.requirePolicySelection(w, r) {
		return
	}
	stage, files, err := h.resolveBatchTargets(r)
	if err != nil {
		jsonError(w, err, 400)
		return
	}
	if h.isDownloadPaused(stage) {
		jsonError(w, errDownloadPaused(), 403)
		return
	}
	enabled, err := h.stageEnabled(r.Context(), stage)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !enabled {
		jsonError(w, errStageDisabled(stage), 403)
		return
	}
	enqueued, skipped, skippedByPolicy := 0, 0, 0
	for _, file := range files {
		if file.OverallStatus == domain.StatusPurged {
			skipped++
			continue
		}
		allow, perr := h.policyAllows(r.Context(), file)
		if perr != nil {
			jsonError(w, perr, 500)
			return
		}
		if !allow {
			skippedByPolicy++
			continue
		}
		_ = h.stageRepo.ResetForRetry(r.Context(), file.ID, stage)
		if err := h.queue.Enqueue(r.Context(), file.ID, stage, time.Now()); err != nil {
			skipped++
			continue
		}
		enqueued++
	}
	_ = h.logRepo.InsertManualAction(r.Context(), "enqueue_batch", &stage, "ui", map[string]any{
		"matched":           len(files),
		"enqueued":          enqueued,
		"skipped":           skipped,
		"skipped_by_policy": skippedByPolicy,
	})
	jsonOK(w, map[string]int{
		"enqueued":          enqueued,
		"skipped":           skipped,
		"skipped_by_policy": skippedByPolicy,
	})
}

// POST /api/actions/reprocess/failures
func (h *ActionsHandler) ReprocessFailures(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hours int              `json:"hours"`
		Stage domain.StageName `json:"stage"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Hours <= 0 {
		body.Hours = 24
	}
	stage := body.Stage
	if stage == "" {
		stage = domain.StageDownload
	}
	if h.isDownloadPaused(stage) {
		jsonError(w, errDownloadPaused(), 403)
		return
	}
	enabled, err := h.stageEnabled(r.Context(), stage)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !enabled {
		jsonError(w, errStageDisabled(stage), 403)
		return
	}
	if !h.requirePolicySelection(w, r) {
		return
	}
	files, err := h.fileRepo.ListFailedSince(r.Context(), time.Now().Add(-time.Duration(body.Hours)*time.Hour), stage)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	enqueued := 0
	for _, file := range files {
		allow, perr := h.policyAllows(r.Context(), file)
		if perr != nil {
			jsonError(w, perr, 500)
			return
		}
		if !allow {
			continue
		}
		_ = h.stageRepo.ResetForRetry(r.Context(), file.ID, stage)
		if err := h.queue.Enqueue(r.Context(), file.ID, stage, time.Now()); err == nil {
			enqueued++
		}
	}
	_ = h.logRepo.InsertManualAction(r.Context(), "reprocess_failures", &stage, "ui", map[string]any{
		"hours":    body.Hours,
		"matched":  len(files),
		"enqueued": enqueued,
	})
	jsonOK(w, map[string]any{"matched": len(files), "enqueued": enqueued, "hours": body.Hours, "stage": stage})
}

// POST /api/purge
func (h *ActionsHandler) Purge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Filename == "" {
		jsonError(w, errBadRequest("filename required"), 400)
		return
	}

	file, err := h.fileRepo.GetByFilename(r.Context(), strings.ToUpper(body.Filename))
	if err == domain.ErrNotFound {
		jsonError(w, err, 404)
		return
	}
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	if file.OverallStatus == domain.StatusPurged {
		jsonOK(w, map[string]bool{"already_purged": true})
		return
	}

	// Atomic: update DB first, then delete from disk
	if err := h.stageRepo.SetPurged(r.Context(), file.ID); err != nil {
		jsonError(w, err, 500)
		return
	}
	if err := h.fileRepo.MarkPurged(r.Context(), file.ID); err != nil {
		jsonError(w, err, 500)
		return
	}

	_ = h.logRepo.Insert(r.Context(), file.ID, domain.StageDownload, "purged",
		"file purged via API", nil)

	// Best-effort disk removal (do not rollback DB on disk error)
	var diskErrors []string
	for _, path := range []*string{file.DBCPath, file.CSVPath, file.ParquetPath} {
		if path != nil {
			if err := storage.DeleteFile(*path); err != nil {
				diskErrors = append(diskErrors, err.Error())
			}
		}
	}

	resp := map[string]any{"purged": file.Filename}
	if len(diskErrors) > 0 {
		resp["disk_errors"] = diskErrors
	}
	jsonOK(w, resp)
}

func (h *ActionsHandler) triggerSingle(w http.ResponseWriter, r *http.Request, stage domain.StageName) {
	if !h.requirePolicySelection(w, r) {
		return
	}
	if h.isDownloadPaused(stage) {
		jsonError(w, errDownloadPaused(), 403)
		return
	}
	enabled, err := h.stageEnabled(r.Context(), stage)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !enabled {
		jsonError(w, errStageDisabled(stage), 403)
		return
	}
	var body struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Filename == "" {
		jsonError(w, errBadRequest("filename required"), 400)
		return
	}

	file, err := h.fileRepo.GetByFilename(r.Context(), strings.ToUpper(body.Filename))
	if err == domain.ErrNotFound {
		jsonError(w, err, 404)
		return
	}
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	allow, err := h.policyAllows(r.Context(), file)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !allow {
		jsonError(w, errPolicyDenied(), 403)
		return
	}

	_ = h.stageRepo.ResetForRetry(r.Context(), file.ID, stage)
	if err := h.queue.Enqueue(r.Context(), file.ID, stage, time.Now()); err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, map[string]string{"enqueued": file.Filename})
}

func (h *ActionsHandler) triggerMask(w http.ResponseWriter, r *http.Request, stage domain.StageName) {
	if !h.requirePolicySelection(w, r) {
		return
	}
	if h.isDownloadPaused(stage) {
		jsonError(w, errDownloadPaused(), 403)
		return
	}
	enabled, err := h.stageEnabled(r.Context(), stage)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	if !enabled {
		jsonError(w, errStageDisabled(stage), 403)
		return
	}
	var body struct {
		Pattern string `json:"pattern"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Pattern == "" {
		jsonError(w, errBadRequest("pattern required"), 400)
		return
	}

	files, err := h.fileRepo.FindByPattern(r.Context(), body.Pattern)
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	enqueued, skipped, skippedByPolicy := 0, 0, 0
	for _, file := range files {
		if file.OverallStatus == domain.StatusPurged {
			skipped++
			continue
		}
		allow, perr := h.policyAllows(r.Context(), file)
		if perr != nil {
			jsonError(w, perr, 500)
			return
		}
		if !allow {
			skippedByPolicy++
			continue
		}
		_ = h.stageRepo.ResetForRetry(r.Context(), file.ID, stage)
		if err := h.queue.Enqueue(r.Context(), file.ID, stage, time.Now()); err != nil {
			skipped++
		} else {
			enqueued++
		}
	}

	jsonOK(w, map[string]int{
		"enqueued":          enqueued,
		"skipped":           skipped,
		"skipped_by_policy": skippedByPolicy,
	})
}

func (h *ActionsHandler) resolveBatchTargets(r *http.Request) (domain.StageName, []*domain.File, error) {
	var body struct {
		Stage   domain.StageName `json:"stage"`
		Pattern string           `json:"pattern"`
		IDs     []string         `json:"ids"`
		Filters map[string]any   `json:"filters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", nil, errBadRequest("invalid json body")
	}
	stage := body.Stage
	if stage == "" {
		stage = domain.StageDownload
	}
	var files []*domain.File
	var err error
	switch {
	case len(body.IDs) > 0:
		files, err = h.fileRepo.FindByIDs(r.Context(), body.IDs)
	case strings.TrimSpace(body.Pattern) != "":
		files, err = h.fileRepo.FindByPattern(r.Context(), body.Pattern)
	default:
		if !hasCatalogOrPeriodFilters(body.Filters) {
			return "", nil, errBadRequest("filters must include catalog and/or period criteria")
		}
		filters := parseListFiltersMap(body.Filters)
		if err := enrichPipelineCompletedFromPolicy(r.Context(), h.policy, &filters); err != nil {
			return "", nil, err
		}
		files, err = h.fileRepo.FindByFilters(r.Context(), filters, 5000)
	}
	if err != nil {
		return "", nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })
	return stage, files, nil
}

func hasCatalogOrPeriodFilters(input map[string]any) bool {
	if input == nil {
		return false
	}
	isNonEmptyString := func(v any) bool {
		s, ok := v.(string)
		return ok && strings.TrimSpace(s) != ""
	}
	if isNonEmptyString(input["catalog"]) {
		return true
	}
	if values, ok := input["catalogs"].([]any); ok {
		for _, value := range values {
			if isNonEmptyString(value) {
				return true
			}
		}
	}
	periodKeys := []string{"period_from_year", "period_from_month", "period_to_year", "period_to_month"}
	for _, key := range periodKeys {
		if _, ok := toInt(input[key]); ok {
			return true
		}
	}
	return false
}

func parseListFiltersMap(input map[string]any) repository.ListFilters {
	f := repository.ListFilters{}
	if input == nil {
		return f
	}
	if v, ok := input["catalog"].(string); ok {
		f.Catalog = strings.ToUpper(strings.TrimSpace(v))
	}
	if v, ok := input["state"].(string); ok {
		f.State = strings.ToUpper(strings.TrimSpace(v))
	}
	if v, ok := input["filename"].(string); ok {
		f.Filename = v
	}
	if v, ok := input["ftp_dir"].(string); ok {
		f.FTPDir = v
	}
	if values, ok := input["catalogs"].([]any); ok {
		for _, value := range values {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				f.Catalogs = append(f.Catalogs, strings.ToUpper(strings.TrimSpace(s)))
			}
		}
	}
	if values, ok := input["statuses"].([]any); ok {
		for _, value := range values {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				f.OverallStatuses = append(f.OverallStatuses, domain.OverallStatus(strings.ToLower(strings.TrimSpace(s))))
			}
		}
	}
	if v, ok := input["status"].(string); ok {
		f.OverallStatus = domain.OverallStatus(strings.ToLower(strings.TrimSpace(v)))
	}
	if y, ok := toInt(input["period_from_year"]); ok {
		f.PeriodFromYear = &y
	}
	if m, ok := toInt(input["period_from_month"]); ok {
		f.PeriodFromMonth = &m
	}
	if y, ok := toInt(input["period_to_year"]); ok {
		f.PeriodToYear = &y
	}
	if m, ok := toInt(input["period_to_month"]); ok {
		f.PeriodToMonth = &m
	}
	if v, ok := input["sort_by"].(string); ok {
		f.SortBy = v
	}
	if v, ok := input["sort_dir"].(string); ok {
		f.SortDir = v
	}
	if v, ok := input["policy_match"].(string); ok {
		pm := strings.ToLower(strings.TrimSpace(v))
		if pm == "pending" || pm == "ignored" {
			f.PolicyMatch = pm
		}
	}
	if jsonTruthy(input["pipeline_completed"]) {
		f.PipelineCompleted = true
	}
	return f
}

func jsonTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case float64:
		return x == 1
	default:
		return false
	}
}

func enrichPipelineCompletedFromPolicy(ctx context.Context, policy *repository.PolicyRepository, f *repository.ListFilters) error {
	if !f.PipelineCompleted {
		return nil
	}
	rd, rc, rp := true, true, true
	if policy != nil {
		proc, err := policy.ProcessingStages(ctx)
		if err != nil {
			return err
		}
		rd, rc, rp = proc.EnableDownload, proc.EnableCSV, proc.EnableParquet
	}
	f.RequireDownload, f.RequireCSV, f.RequireParquet = rd, rc, rp
	return nil
}

func toInt(v any) (int, bool) {
	switch value := v.(type) {
	case float64:
		return int(value), true
	case int:
		return value, true
	default:
		return 0, false
	}
}

type badRequestError string
type policyDeniedError struct{}
type policyIncompleteError struct{}
type downloadPausedError struct{}
type stageDisabledError struct {
	stage domain.StageName
}

func (policyDeniedError) Error() string {
	return "processing policy does not allow this catalog and period"
}
func (policyIncompleteError) Error() string {
	return "select at least one catalog and one period in processing policy"
}
func (downloadPausedError) Error() string {
	return "downloads are paused by configuration"
}
func (e stageDisabledError) Error() string {
	return fmt.Sprintf("stage %q is disabled by processing policy", e.stage)
}

func errPolicyDenied() error                        { return policyDeniedError{} }
func errPolicyIncomplete() error                    { return policyIncompleteError{} }
func errDownloadPaused() error                      { return downloadPausedError{} }
func errStageDisabled(stage domain.StageName) error { return stageDisabledError{stage: stage} }

func errBadRequest(msg string) error    { return badRequestError(msg) }
func (e badRequestError) Error() string { return string(e) }
