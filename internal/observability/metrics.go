package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	JobsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasus_jobs_processed_total",
		Help: "Total jobs processed, by stage and outcome.",
	}, []string{"stage", "status"}) // status: success | failure

	JobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "datasus_job_duration_seconds",
		Help:    "Processing time per job, by stage.",
		Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"stage"})

	QueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasus_queue_depth",
		Help: "Current number of jobs in the queue, by stage and status.",
	}, []string{"stage", "status"})

	FTPScanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "datasus_ftp_scan_duration_seconds",
		Help:    "Duration of FTP directory scans.",
		Buckets: []float64{5, 15, 30, 60, 120, 300},
	})

	FTPScanFilesFound = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_ftp_scan_files_found_total",
		Help: "Total .dbc files found during FTP scans.",
	})

	FTPScanFilesEnqueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_ftp_scan_files_enqueued_total",
		Help: "Total files enqueued for download after FTP scans.",
	})
)
