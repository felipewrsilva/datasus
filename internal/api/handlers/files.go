package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"datasus/internal/domain"
	"datasus/internal/repository"
)

// FilesHandler serves read-only file and stage data.
type FilesHandler struct {
	fileRepo   *repository.FileRepository
	stageRepo  *repository.StageRepository
	logRepo    *repository.LogRepository
	policyRepo *repository.PolicyRepository
}

func NewFilesHandler(
	fileRepo *repository.FileRepository,
	stageRepo *repository.StageRepository,
	logRepo *repository.LogRepository,
	policyRepo *repository.PolicyRepository,
) *FilesHandler {
	return &FilesHandler{fileRepo: fileRepo, stageRepo: stageRepo, logRepo: logRepo, policyRepo: policyRepo}
}

func (h *FilesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := intQuery(q.Get("limit"), 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	f := repository.ListFilters{
		Catalog:  strings.ToUpper(strings.TrimSpace(q.Get("catalog"))),
		Catalogs: parseListQuery(q["catalog"]),
		State:    strings.ToUpper(strings.TrimSpace(q.Get("state"))),
		States:   parseListQuery(q["state"]),
		FTPDir:   q.Get("ftp_dir"),
		Filename: q.Get("filename"),
		SortBy:   q.Get("sort_by"),
		SortDir:  q.Get("sort_dir"),
		Limit:    limit,
		Offset:   max(0, intQuery(q.Get("offset"), 0)),
	}
	if s := q.Get("status"); s != "" {
		f.OverallStatus = domain.OverallStatus(strings.ToLower(strings.TrimSpace(s)))
	}
	if statuses := parseStatusListQuery(q["status"]); len(statuses) > 0 {
		f.OverallStatuses = make([]domain.OverallStatus, 0, len(statuses))
		for _, item := range statuses {
			f.OverallStatuses = append(f.OverallStatuses, domain.OverallStatus(item))
		}
	}
	if y := q.Get("year"); y != "" {
		yr, _ := strconv.Atoi(y)
		f.Year = &yr
	}
	if m := q.Get("month"); m != "" {
		mo, _ := strconv.Atoi(m)
		f.Month = &mo
	}
	if y := q.Get("period_from_year"); y != "" {
		yr, _ := strconv.Atoi(y)
		f.PeriodFromYear = &yr
	}
	if m := q.Get("period_from_month"); m != "" {
		mo, _ := strconv.Atoi(m)
		f.PeriodFromMonth = &mo
	}
	if y := q.Get("period_to_year"); y != "" {
		yr, _ := strconv.Atoi(y)
		f.PeriodToYear = &yr
	}
	if m := q.Get("period_to_month"); m != "" {
		mo, _ := strconv.Atoi(m)
		f.PeriodToMonth = &mo
	}
	// policy_match: pending | ignored — backward compatibility with old links.
	if pm := strings.ToLower(strings.TrimSpace(q.Get("policy_match"))); pm == "pending" || pm == "ignored" {
		f.PolicyMatch = pm
	}

	if truthyQuery(q.Get("pipeline_completed")) {
		requireDownload := true
		requireCSV := true
		requireParquet := true
		if h.policyRepo != nil {
			processing, err := h.policyRepo.ProcessingStages(r.Context())
			if err != nil {
				jsonError(w, err, 500)
				return
			}
			requireDownload = processing.EnableDownload
			requireCSV = processing.EnableCSV
			requireParquet = processing.EnableParquet
		}
		f.PipelineCompleted = true
		f.RequireDownload = requireDownload
		f.RequireCSV = requireCSV
		f.RequireParquet = requireParquet
	}

	files, total, err := h.fileRepo.List(r.Context(), f)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, map[string]any{"total": total, "items": files})
}

func (h *FilesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		jsonError(w, errors.New("invalid file id"), http.StatusBadRequest)
		return
	}
	file, err := h.fileRepo.GetByID(r.Context(), id)
	if err == domain.ErrNotFound {
		jsonError(w, err, 404)
		return
	}
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, file)
}

func (h *FilesHandler) Facets(w http.ResponseWriter, r *http.Request) {
	facets, err := h.fileRepo.ListFacets(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, facets)
}

func (h *FilesHandler) GetStages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		jsonError(w, errors.New("invalid file id"), http.StatusBadRequest)
		return
	}
	stages, err := h.stageRepo.ListByFile(r.Context(), id)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	logs, _ := h.logRepo.ListByFile(r.Context(), id, 100)
	jsonOK(w, map[string]any{"stages": stages, "logs": logs})
}

func (h *FilesHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.fileRepo.Stats(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, stats)
}

func (h *FilesHandler) Insights(w http.ResponseWriter, r *http.Request) {
	stats, err := h.fileRepo.Stats(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	totalFiles := sumStatusCounts(stats)

	byCatalog, err := h.fileRepo.SizeByCatalog(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	byState, err := h.fileRepo.SizeByState(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}

	reasons, err := h.stageRepo.TopFailureReasons(r.Context(), intQuery(r.URL.Query().Get("fail_limit"), 10))
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	policyCounts := map[string]int64{}
	requireDownload := true
	requireCSV := true
	requireParquet := true
	if h.policyRepo != nil {
		pendingPolicy, ignoredPolicy, err := h.policyRepo.PendingAndIgnoredCounts(r.Context())
		if err != nil {
			jsonError(w, err, 500)
			return
		}
		policyCounts["pending"] = pendingPolicy
		policyCounts["ignored"] = ignoredPolicy

		processing, err := h.policyRepo.ProcessingStages(r.Context())
		if err != nil {
			jsonError(w, err, 500)
			return
		}
		requireDownload = processing.EnableDownload
		requireCSV = processing.EnableCSV
		requireParquet = processing.EnableParquet
	}

	expectedTerminalStatus := expectedCompletedStatus(requireDownload, requireCSV, requireParquet)
	pipelineConsistency, err := h.stageRepo.PipelineConsistency(
		r.Context(),
		expectedTerminalStatus,
		requireDownload,
		requireCSV,
		requireParquet,
	)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	byCatalogTotal := sumStateSizeBuckets(byCatalog)
	byStateTotal := sumStateSizeBuckets(byState)

	jsonOK(w, map[string]any{
		"total_files":                 totalFiles,
		"status_counts":               stats,
		"policy_counts":               policyCounts,
		"stats":                       stats,
		"by_catalog":                  byCatalog,
		"by_state":                    byState,
		"failure_reasons":             reasons,
		"pipeline_completed_count":    pipelineConsistency.PipelineCompletedCount,
		"status_stage_mismatch_count": pipelineConsistency.StatusStageMismatchCount,
		"by_catalog_total_mismatch":   byCatalogTotal - totalFiles,
		"by_state_total_mismatch":     byStateTotal - totalFiles,
	})
}

func (h *FilesHandler) Bottlenecks(w http.ResponseWriter, r *http.Request) {
	items, err := h.stageRepo.Bottlenecks(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, map[string]any{"items": items})
}

func (h *FilesHandler) FailureReasons(w http.ResponseWriter, r *http.Request) {
	reasons, err := h.stageRepo.TopFailureReasons(r.Context(), intQuery(r.URL.Query().Get("limit"), 20))
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, map[string]any{"items": reasons})
}

func (h *FilesHandler) Alerts(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	currentFrom := now.Add(-1 * time.Hour)
	previousFrom := now.Add(-2 * time.Hour)
	current, err := h.stageRepo.FailureCountBetween(r.Context(), currentFrom, now)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	previous, err := h.stageRepo.FailureCountBetween(r.Context(), previousFrom, currentFrom)
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	oldestPendingSeconds, err := h.stageRepo.PendingOldestAgeSeconds(r.Context())
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	alerts := make([]map[string]any, 0, 2)
	if current > previous*2 && current > 10 {
		alerts = append(alerts, map[string]any{
			"type":     "failure_spike",
			"message":  "Aumento de falhas na ultima hora",
			"current":  current,
			"previous": previous,
		})
	}
	if oldestPendingSeconds > 1800 {
		alerts = append(alerts, map[string]any{
			"type":                   "queue_stalled",
			"message":                "Fila com item pendente antigo",
			"oldest_pending_seconds": oldestPendingSeconds,
		})
	}
	jsonOK(w, map[string]any{"items": alerts})
}

func (h *FilesHandler) ManualActions(w http.ResponseWriter, r *http.Request) {
	items, err := h.logRepo.ListManualActions(r.Context(), intQuery(r.URL.Query().Get("limit"), 50))
	if err != nil {
		jsonError(w, err, 500)
		return
	}
	jsonOK(w, map[string]any{"items": items})
}

// helpers

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func intQuery(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseListQuery(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		parts := strings.Split(item, ",")
		for _, part := range parts {
			v := strings.ToUpper(strings.TrimSpace(part))
			if v == "" {
				continue
			}
			out = append(out, v)
		}
	}
	return out
}

func parseStatusListQuery(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		parts := strings.Split(item, ",")
		for _, part := range parts {
			v := strings.ToLower(strings.TrimSpace(part))
			if v == "" {
				continue
			}
			out = append(out, v)
		}
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truthyQuery(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func sumStatusCounts(statusCounts map[string]int64) int64 {
	var total int64
	for _, count := range statusCounts {
		total += count
	}
	return total
}

func sumCountBuckets(items []repository.CountBucket) int64 {
	var total int64
	for _, item := range items {
		total += item.Count
	}
	return total
}

func sumStateSizeBuckets(items []repository.StateSizeBucket) int64 {
	var total int64
	for _, item := range items {
		total += item.Count
	}
	return total
}

func expectedCompletedStatus(requireDownload, requireCSV, requireParquet bool) domain.OverallStatus {
	switch {
	case requireParquet:
		return domain.StatusParquetReady
	case requireCSV:
		return domain.StatusCSVReady
	case requireDownload:
		return domain.StatusDownloaded
	default:
		return domain.StatusPending
	}
}
