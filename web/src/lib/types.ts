export type OverallStatus =
  | "pending"
  | "ignored"
  | "downloading"
  | "downloaded"
  | "converting_csv"
  | "csv_ready"
  | "converting_parquet"
  | "parquet_ready"
  | "failed"
  | "purged";

export type StageStatus = "pending" | "running" | "done" | "failed" | "purged";
export type StageName = "download" | "csv_conversion" | "parquet_conversion";

export interface DatFile {
  id: string;
  filename: string;
  catalog: string;
  state: string;
  year: number;
  month: number;
  ftp_dir: string;
  ftp_path: string;
  size_bytes: number | null;
  remote_checksum: string | null;
  remote_timestamp: string | null;
  local_hash: string | null;
  root_path: string;
  dbc_path: string | null;
  csv_path: string | null;
  parquet_path: string | null;
  overall_status: OverallStatus;
  created_at: string;
  updated_at: string;
  last_seen_at: string;
}

export interface Stage {
  id: string;
  file_id: string;
  stage: StageName;
  status: StageStatus;
  attempts: number;
  started_at: string | null;
  finished_at: string | null;
  error_message: string | null;
  updated_at: string;
}

export interface LogEntry {
  id: number;
  file_id: string;
  stage: StageName;
  event_type: string;
  message: string;
  payload_json: Record<string, unknown> | null;
  created_at?: string;
}

export interface FilesResponse {
  total: number;
  items: DatFile[];
}

export interface FileFacetPeriod {
  year: number;
  month: number;
}

export interface FileFacetsResponse {
  catalogs: string[];
  states: string[];
  statuses: OverallStatus[];
  periods: FileFacetPeriod[];
}

export interface StagesResponse {
  stages: Stage[];
  logs: LogEntry[];
}

export interface Stats {
  [status: string]: number;
}

export interface CountBucket {
  key: string;
  count: number;
}

export interface StateSizeBucket {
  key: string;
  count: number;
  total_size_bytes: number;
  avg_size_bytes: number;
}

export interface FailureReasonCount {
  stage: string;
  reason: string;
  count: number;
}

/** Global pipeline toggles; same source as dashboard pipeline_completed predicate. */
export interface ProcessingPolicySnapshot {
  enable_download: boolean;
  enable_csv: boolean;
  enable_parquet: boolean;
}

/** Counts of file_stages rows with status=done per stage (one row per file per stage). */
export interface StageDoneCounts {
  download: number;
  csv_conversion: number;
  parquet_conversion: number;
}

export interface DashboardInsights {
  total_files: number;
  status_counts: Stats;
  policy_counts: {
    pending: number;
    ignored: number;
  };
  stats: Stats;
  by_catalog: StateSizeBucket[];
  by_state: StateSizeBucket[];
  failure_reasons: FailureReasonCount[];
  pipeline_completed_count: number;
  status_stage_mismatch_count: number;
  by_catalog_total_mismatch: number;
  by_state_total_mismatch: number;
  /** Present on API v2+; defaults applied in normalize when missing. */
  processing?: ProcessingPolicySnapshot;
  expected_terminal_status?: OverallStatus;
  stage_done_counts?: StageDoneCounts;
}

export interface ScanResult {
  dir: string;
  found: number;
  new: number;
  changed: number;
  skipped: number;
  skipped_by_policy?: number;
  enqueued: number;
  errors: string[];
}

export interface ScanStatus {
  state: "stopped" | "running" | "error";
  running: boolean;
  current_reason?: string;
  last_reason?: string;
  last_actor?: string;
  started_at?: string;
  finished_at?: string;
  last_success_at?: string;
  last_error?: string;
  last_duration_ns: number;
  last_found: number;
  last_enqueued: number;
}

export interface TriggerScanResponse {
  accepted: boolean;
  message: string;
  status: ScanStatus;
}

export interface YearMonth {
  year: number;
  month: number;
}

export interface PoliciesResponse {
  available_catalogs: string[];
  available_periods: {
    years: number[];
    months: YearMonth[];
  };
  selected_catalogs: string[];
  selected_periods: {
    years: number[];
    months: YearMonth[];
  };
  processing: {
    enable_download: boolean;
    enable_csv: boolean;
    enable_parquet: boolean;
  };
  directories: {
    download_dir?: string;
    csv_dir?: string;
    parquet_dir?: string;
  };
}

/** Backward compatibility for old links, prefer status filter. */
export type FilePolicyMatch = "pending" | "ignored";

export interface FileFilters {
  catalog?: string;
  catalogs?: string[];
  state?: string;
  states?: string[];
  year?: number;
  month?: number;
  period_from_year?: number;
  period_from_month?: number;
  period_to_year?: number;
  period_to_month?: number;
  ftp_dir?: string;
  filename?: string;
  status?: OverallStatus;
  statuses?: OverallStatus[];
  /** Backward compatibility with old dashboard links. */
  policy_match?: FilePolicyMatch;
  /** Dashboard "Processados": same predicate as pipeline_completed_count. */
  pipeline_completed?: boolean;
  sort_by?: string;
  sort_dir?: "asc" | "desc";
  limit?: number;
  offset?: number;
}

export interface BatchResult {
  enqueued: number;
  skipped: number;
  skipped_by_policy?: number;
}

export interface BatchPreview {
  stage: string;
  total_matched: number;
  eligible: number;
  blocked_by_policy: number;
  blocked_purged: number;
}
