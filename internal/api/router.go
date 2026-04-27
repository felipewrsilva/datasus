package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"datasus/internal/api/handlers"
)

func NewRouter(
	filesHandler *handlers.FilesHandler,
	actionsHandler *handlers.ActionsHandler,
	policiesHandler *handlers.PoliciesHandler,
	log *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestID)
	r.Use(CORS)
	r.Use(Recovery(log))
	r.Use(Logger(log))
	r.Use(middleware.Compress(5))

	r.Route("/api", func(r chi.Router) {
		// Health & metrics
		r.Get("/health", handlers.Health)
		r.Handle("/metrics", promhttp.Handler())

		// Files
		r.Get("/files", filesHandler.List)
		r.Get("/files/facets", filesHandler.Facets)
		r.Get("/files/{id}", filesHandler.Get)
		r.Get("/files/{id}/stages", filesHandler.GetStages)
		r.Get("/stats", filesHandler.Stats)
		r.Get("/dashboard/insights", filesHandler.Insights)
		r.Get("/ops/bottlenecks", filesHandler.Bottlenecks)
		r.Get("/ops/failure-reasons", filesHandler.FailureReasons)
		r.Get("/ops/alerts", filesHandler.Alerts)
		r.Get("/ops/manual-actions", filesHandler.ManualActions)
		r.Get("/policies", policiesHandler.Get)
		r.Put("/policies", policiesHandler.Put)

		// Actions
		r.Post("/scan", actionsHandler.Scan)
		r.Get("/scan/status", actionsHandler.ScanStatus)
		r.Post("/download", actionsHandler.Download)
		r.Post("/download/mask", actionsHandler.DownloadMask)
		r.Post("/convert/csv", actionsHandler.ConvertCSV)
		r.Post("/convert/csv/mask", actionsHandler.ConvertCSVMask)
		r.Post("/convert/parquet", actionsHandler.ConvertParquet)
		r.Post("/convert/parquet/mask", actionsHandler.ConvertParquetMask)
		r.Post("/actions/preview", actionsHandler.PreviewBatch)
		r.Post("/actions/enqueue", actionsHandler.EnqueueBatch)
		r.Post("/actions/reprocess/failures", actionsHandler.ReprocessFailures)
		r.Post("/purge", actionsHandler.Purge)
	})

	return r
}
