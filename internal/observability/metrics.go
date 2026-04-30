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

	FTPScanPhaseDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "datasus_ftp_scan_phase_seconds",
		Help:    "Time spent in each FTP scan phase.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
	}, []string{"phase"})

	FTPScanFilesUnchanged = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_ftp_scan_files_unchanged_total",
		Help: "Total .dbc files seen in FTP scans that already matched the catalog snapshot.",
	})

	FTPScanFilesChanged = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_ftp_scan_files_changed_total",
		Help: "Total .dbc files updated during FTP scans because remote size or timestamp changed.",
	})

	FTPScanFilesInserted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_ftp_scan_files_inserted_total",
		Help: "Total .dbc files inserted into the catalog by FTP scans.",
	})

	FTPScanDBRoundtrips = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_ftp_scan_db_roundtrips_total",
		Help: "Total database round trips performed by FTP scans (snapshot, bulk upsert, batch enqueue, etc.).",
	})

	PolicySkipsByState = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasus_policy_skips_by_state_total",
		Help: "Total files skipped by policy, grouped by state and source.",
	}, []string{"state", "source"})

	PolicySyncRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_policy_sync_runs_total",
		Help: "Total policy-driven local filesystem synchronization runs.",
	})

	PolicySyncFilesFound = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_policy_sync_files_found_total",
		Help: "Total .dbc files found by policy local sync.",
	})

	PolicySyncFilesMapped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_policy_sync_files_mapped_total",
		Help: "Total .dbc files successfully mapped and cataloged by policy local sync.",
	})

	PolicySyncEnqueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasus_policy_sync_jobs_enqueued_total",
		Help: "Total conversion jobs enqueued by policy local sync.",
	})
)
