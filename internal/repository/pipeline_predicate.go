package repository

import "fmt"

// pipelineStageFlagsAndEvalCTEs returns "stage_flags AS (...), eval AS (...)" for use after WITH.
// Placeholders req1, req2, req3 are 1-based indices for requireDownload, requireCSV, requireParquet (boolean).
func pipelineStageFlagsAndEvalCTEs(req1, req2, req3 int) string {
	return fmt.Sprintf(`stage_flags AS (
		SELECT
			f.id,
			f.overall_status,
			BOOL_OR(fs.stage = 'download' AND fs.status = 'done') AS download_done,
			BOOL_OR(fs.stage = 'csv_conversion' AND fs.status = 'done') AS csv_done,
			BOOL_OR(fs.stage = 'parquet_conversion' AND fs.status = 'done') AS parquet_done
		FROM files f
		LEFT JOIN file_stages fs ON fs.file_id = f.id
		GROUP BY f.id, f.overall_status
	),
	eval AS (
		SELECT
			id,
			overall_status,
			((NOT $%d) OR download_done)
			AND ((NOT $%d) OR csv_done)
			AND ((NOT $%d) OR parquet_done) AS pipeline_completed
		FROM stage_flags
	)`, req1, req2, req3)
}
