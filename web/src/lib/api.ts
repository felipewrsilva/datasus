import type {
  DatFile,
  DashboardInsights,
  StateSizeBucket,
  FailureReasonCount,
  FilesResponse,
  FileFacetsResponse,
  FileFilters,
  StagesResponse,
  Stats,
  ScanStatus,
  TriggerScanResponse,
  PoliciesResponse,
  BatchResult,
  BatchPreview,
} from "./types";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

function pick<T>(obj: Record<string, unknown>, ...keys: string[]): T | undefined {
  for (const key of keys) {
    if (obj[key] !== undefined && obj[key] !== null) {
      return obj[key] as T;
    }
  }
  return undefined;
}

function normalizeFile(raw: unknown): DatFile {
  const r = (raw ?? {}) as Record<string, unknown>;
  return {
    id: pick<string>(r, "id", "ID") ?? "",
    filename: pick<string>(r, "filename", "Filename") ?? "",
    catalog: pick<string>(r, "catalog", "Catalog") ?? "",
    state: pick<string>(r, "state", "State") ?? "",
    year: Number(pick<number | string>(r, "year", "Year") ?? 0),
    month: Number(pick<number | string>(r, "month", "Month") ?? 0),
    ftp_dir: pick<string>(r, "ftp_dir", "FTPDir") ?? "",
    ftp_path: pick<string>(r, "ftp_path", "FTPPath") ?? "",
    size_bytes: (pick<number | null>(r, "size_bytes", "SizeBytes") ?? null) as number | null,
    remote_checksum: (pick<string | null>(r, "remote_checksum", "RemoteChecksum") ?? null) as string | null,
    remote_timestamp: (pick<string | null>(r, "remote_timestamp", "RemoteTimestamp") ?? null) as string | null,
    local_hash: (pick<string | null>(r, "local_hash", "LocalHash") ?? null) as string | null,
    root_path: pick<string>(r, "root_path", "RootPath") ?? "",
    dbc_path: (pick<string | null>(r, "dbc_path", "DBCPath") ?? null) as string | null,
    csv_path: (pick<string | null>(r, "csv_path", "CSVPath") ?? null) as string | null,
    parquet_path: (pick<string | null>(r, "parquet_path", "ParquetPath") ?? null) as string | null,
    overall_status: pick<DatFile["overall_status"]>(r, "overall_status", "OverallStatus") ?? "pending",
    created_at: pick<string>(r, "created_at", "CreatedAt") ?? "",
    updated_at: pick<string>(r, "updated_at", "UpdatedAt") ?? "",
    last_seen_at: pick<string>(r, "last_seen_at", "LastSeenAt") ?? "",
  };
}

function normalizeFilesResponse(raw: unknown): FilesResponse {
  const r = (raw ?? {}) as Record<string, unknown>;
  const items = (pick<unknown[]>(r, "items", "Items") ?? []).map(normalizeFile);
  return {
    total: Number(pick<number | string>(r, "total", "Total") ?? items.length),
    items,
  };
}

function normalizeStateSizeBuckets(raw: unknown): StateSizeBucket[] {
  const items = Array.isArray(raw) ? raw : [];
  return items.map((item) => {
    const r = (item ?? {}) as Record<string, unknown>;
    return {
      key: String(pick<string>(r, "key", "Key") ?? ""),
      count: Number(pick<number | string>(r, "count", "Count") ?? 0),
      total_size_bytes: Number(
        pick<number | string>(r, "total_size_bytes", "TotalSizeBytes") ?? 0,
      ),
      avg_size_bytes: Number(
        pick<number | string>(r, "avg_size_bytes", "AvgSizeBytes") ?? 0,
      ),
    };
  });
}

function normalizeFailureReasons(raw: unknown): FailureReasonCount[] {
  const items = Array.isArray(raw) ? raw : [];
  return items.map((item) => {
    const r = (item ?? {}) as Record<string, unknown>;
    return {
      stage: String(pick<string>(r, "stage", "Stage") ?? ""),
      reason: String(pick<string>(r, "reason", "Reason") ?? "Unknown error"),
      count: Number(pick<number | string>(r, "count", "Count") ?? 0),
    };
  });
}

function normalizeDashboardInsights(raw: unknown): DashboardInsights {
  const r = (raw ?? {}) as Record<string, unknown>;
  const statusCounts = (
    pick<Record<string, number>>(r, "status_counts", "StatusCounts") ??
    pick<Record<string, number>>(r, "stats", "Stats") ??
    {}
  ) as Record<string, number>;
  const policyRaw = (pick<Record<string, unknown>>(r, "policy_counts", "PolicyCounts") ?? {});
  return {
    total_files: Number(
      pick<number | string>(r, "total_files", "TotalFiles") ??
      Object.values(statusCounts).reduce((sum, count) => sum + Number(count ?? 0), 0),
    ),
    status_counts: statusCounts,
    policy_counts: {
      pending: Number(pick<number | string>(policyRaw, "pending", "Pending") ?? 0),
      ignored: Number(pick<number | string>(policyRaw, "ignored", "Ignored") ?? 0),
    },
    stats: statusCounts,
    by_catalog: normalizeStateSizeBuckets(pick<unknown[]>(r, "by_catalog", "ByCatalog")),
    by_state: normalizeStateSizeBuckets(pick<unknown[]>(r, "by_state", "ByState")),
    failure_reasons: normalizeFailureReasons(pick<unknown[]>(r, "failure_reasons", "FailureReasons")),
    pipeline_completed_count: Number(
      pick<number | string>(r, "pipeline_completed_count", "PipelineCompletedCount") ?? 0,
    ),
    status_stage_mismatch_count: Number(
      pick<number | string>(r, "status_stage_mismatch_count", "StatusStageMismatchCount") ?? 0,
    ),
    by_catalog_total_mismatch: Number(
      pick<number | string>(r, "by_catalog_total_mismatch", "ByCatalogTotalMismatch") ?? 0,
    ),
    by_state_total_mismatch: Number(
      pick<number | string>(r, "by_state_total_mismatch", "ByStateTotalMismatch") ?? 0,
    ),
  };
}

function normalizeFileFacetsResponse(raw: unknown): FileFacetsResponse {
  const r = (raw ?? {}) as Record<string, unknown>;
  const periodsRaw = pick<unknown[]>(r, "periods", "Periods") ?? [];
  return {
    catalogs: (pick<unknown[]>(r, "catalogs", "Catalogs") ?? []).map((x) => String(x)),
    states: (pick<unknown[]>(r, "states", "States") ?? []).map((x) => String(x)),
    statuses: (pick<unknown[]>(r, "statuses", "Statuses") ?? []).map((x) => String(x) as FileFacetsResponse["statuses"][number]),
    periods: periodsRaw.map((item) => {
      const row = (item ?? {}) as Record<string, unknown>;
      return {
        year: Number(pick<number | string>(row, "year", "Year") ?? 0),
        month: Number(pick<number | string>(row, "month", "Month") ?? 0),
      };
    }).filter((item) => Number.isFinite(item.year) && Number.isFinite(item.month) && item.month >= 1 && item.month <= 12),
  };
}

function normalizeStage(raw: unknown): StagesResponse["stages"][number] {
  const r = (raw ?? {}) as Record<string, unknown>;
  return {
    id: pick<string>(r, "id", "ID") ?? "",
    file_id: pick<string>(r, "file_id", "FileID") ?? "",
    stage: pick<StagesResponse["stages"][number]["stage"]>(r, "stage", "Stage") ?? "download",
    status: pick<StagesResponse["stages"][number]["status"]>(r, "status", "Status") ?? "pending",
    attempts: Number(pick<number | string>(r, "attempts", "Attempts") ?? 0),
    started_at: (pick<string | null>(r, "started_at", "StartedAt") ?? null) as string | null,
    finished_at: (pick<string | null>(r, "finished_at", "FinishedAt") ?? null) as string | null,
    error_message: (pick<string | null>(r, "error_message", "ErrorMessage") ?? null) as string | null,
    updated_at: pick<string>(r, "updated_at", "UpdatedAt") ?? "",
  };
}

function normalizeLogEntry(raw: unknown): StagesResponse["logs"][number] {
  const r = (raw ?? {}) as Record<string, unknown>;
  return {
    id: Number(pick<number | string>(r, "id", "ID") ?? 0),
    file_id: pick<string>(r, "file_id", "FileID") ?? "",
    stage: pick<StagesResponse["logs"][number]["stage"]>(r, "stage", "Stage") ?? "download",
    event_type: pick<string>(r, "event_type", "EventType") ?? "",
    message: pick<string>(r, "message", "Message") ?? "",
    payload_json: (pick<Record<string, unknown> | null>(r, "payload_json", "PayloadJSON") ?? null) as Record<string, unknown> | null,
    created_at: (pick<string | null>(r, "created_at", "CreatedAt") ?? undefined) as string | undefined,
  };
}

function normalizeStagesResponse(raw: unknown): StagesResponse {
  const r = (raw ?? {}) as Record<string, unknown>;
  return {
    stages: (pick<unknown[]>(r, "stages", "Stages") ?? []).map(normalizeStage),
    logs: (pick<unknown[]>(r, "logs", "Logs") ?? []).map(normalizeLogEntry),
  };
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

export async function getFiles(filters: FileFilters = {}): Promise<FilesResponse> {
  const params = new URLSearchParams();
  if (filters.catalog) params.set("catalog", filters.catalog);
  if (filters.catalogs) {
    for (const catalog of filters.catalogs) params.append("catalog", catalog);
  }
  if (filters.state) params.set("state", filters.state);
  if (filters.states) {
    for (const state of filters.states) params.append("state", state);
  }
  if (filters.year) params.set("year", String(filters.year));
  if (filters.month) params.set("month", String(filters.month));
  if (filters.ftp_dir) params.set("ftp_dir", filters.ftp_dir);
  if (filters.filename) params.set("filename", filters.filename);
  if (filters.status) params.set("status", filters.status);
  if (filters.statuses) {
    for (const status of filters.statuses) params.append("status", status);
  }
  if (filters.policy_match) params.set("policy_match", filters.policy_match);
  if (filters.pipeline_completed) params.set("pipeline_completed", "1");
  if (filters.period_from_year) params.set("period_from_year", String(filters.period_from_year));
  if (filters.period_from_month) params.set("period_from_month", String(filters.period_from_month));
  if (filters.period_to_year) params.set("period_to_year", String(filters.period_to_year));
  if (filters.period_to_month) params.set("period_to_month", String(filters.period_to_month));
  if (filters.sort_by) params.set("sort_by", filters.sort_by);
  if (filters.sort_dir) params.set("sort_dir", filters.sort_dir);
  if (filters.limit) params.set("limit", String(filters.limit));
  if (filters.offset) params.set("offset", String(filters.offset));
  const data = await req<unknown>(`/api/files?${params}`);
  return normalizeFilesResponse(data);
}

export async function getFileFacets(): Promise<FileFacetsResponse> {
  const data = await req<unknown>("/api/files/facets");
  return normalizeFileFacetsResponse(data);
}

export async function getFile(id: string): Promise<DatFile> {
  const data = await req<unknown>(`/api/files/${id}`);
  return normalizeFile(data);
}

export async function getFileStages(id: string): Promise<StagesResponse> {
  const data = await req<unknown>(`/api/files/${id}/stages`);
  return normalizeStagesResponse(data);
}

export async function getStats(): Promise<Stats> {
  return req<Stats>("/api/stats");
}

export async function getDashboardInsights(): Promise<DashboardInsights> {
  const data = await req<unknown>("/api/dashboard/insights");
  return normalizeDashboardInsights(data);
}

function normalizePolicies(raw: unknown): PoliciesResponse {
  const r = (raw ?? {}) as Record<string, unknown>;
  const availablePeriodsRaw = (pick<Record<string, unknown>>(r, "available_periods", "AvailablePeriods") ?? {});
  const availableYearsRaw = pick<unknown[]>(availablePeriodsRaw, "years", "Years") ?? [];
  const availableMonthsRaw = pick<unknown[]>(availablePeriodsRaw, "months", "Months") ?? [];
  const selectedPeriodsRaw = (pick<Record<string, unknown>>(r, "selected_periods", "SelectedPeriods") ?? {});
  const selectedYearsRaw = pick<unknown[]>(selectedPeriodsRaw, "years", "Years") ?? [];
  const selectedMonthsRaw = pick<unknown[]>(selectedPeriodsRaw, "months", "Months") ?? [];
  return {
    available_catalogs: (pick<unknown[]>(r, "available_catalogs", "AvailableCatalogs") ?? []).map((x) => String(x)),
    available_periods: {
      years: availableYearsRaw.map((x) => Number(x)).filter((x) => Number.isFinite(x)),
      months: availableMonthsRaw.map((item) => {
        const row = (item ?? {}) as Record<string, unknown>;
        return {
          year: Number(pick<number | string>(row, "year", "Year") ?? 0),
          month: Number(pick<number | string>(row, "month", "Month") ?? 0),
        };
      }).filter((item) => Number.isFinite(item.year) && Number.isFinite(item.month) && item.month >= 1 && item.month <= 12),
    },
    selected_catalogs: (pick<unknown[]>(r, "selected_catalogs", "SelectedCatalogs") ?? []).map((x) => String(x)),
    selected_periods: {
      years: selectedYearsRaw.map((x) => Number(x)).filter((x) => Number.isFinite(x)),
      months: selectedMonthsRaw.map((item) => {
        const row = (item ?? {}) as Record<string, unknown>;
        return {
          year: Number(pick<number | string>(row, "year", "Year") ?? 0),
          month: Number(pick<number | string>(row, "month", "Month") ?? 0),
        };
      }),
    },
    processing: {
      enable_download: Boolean(
        pick<boolean>(
          pick<Record<string, unknown>>(r, "processing", "Processing") ?? {},
          "enable_download",
          "EnableDownload",
        ) ?? true,
      ),
      enable_csv: Boolean(
        pick<boolean>(
          pick<Record<string, unknown>>(r, "processing", "Processing") ?? {},
          "enable_csv",
          "EnableCSV",
        ) ?? true,
      ),
      enable_parquet: Boolean(
        pick<boolean>(
          pick<Record<string, unknown>>(r, "processing", "Processing") ?? {},
          "enable_parquet",
          "EnableParquet",
        ) ?? true,
      ),
    },
  };
}

/** At least one catalog and at least one period row (ano inteiro e/ou mês) saved in policy. */
export function isPolicySelectionComplete(p: PoliciesResponse): boolean {
  const cats = p.selected_catalogs.length;
  const periodRows = p.selected_periods.years.length + p.selected_periods.months.length;
  return cats > 0 && periodRows > 0;
}

export async function getPolicies(): Promise<PoliciesResponse> {
  const data = await req<unknown>("/api/policies");
  return normalizePolicies(data);
}

export async function putPolicies(input: {
  selected_catalogs: string[];
  selected_periods: { years: number[]; months: Array<{ year: number; month: number }> };
  processing: { enable_download: boolean; enable_csv: boolean; enable_parquet: boolean };
}): Promise<PoliciesResponse> {
  const data = await req<unknown>("/api/policies", {
    method: "PUT",
    body: JSON.stringify(input),
  });
  return normalizePolicies(data);
}

export async function triggerScan(paths?: string[]): Promise<TriggerScanResponse> {
  return req<TriggerScanResponse>("/api/scan", {
    method: "POST",
    body: JSON.stringify({ paths: paths ?? [] }),
  });
}

export async function getScanStatus(): Promise<ScanStatus> {
  return req<ScanStatus>("/api/scan/status");
}

export async function triggerDownload(filename: string): Promise<{ enqueued: string }> {
  return req("/api/download", {
    method: "POST",
    body: JSON.stringify({ filename }),
  });
}

export async function triggerDownloadMask(pattern: string): Promise<{
  enqueued: number;
  skipped: number;
  skipped_by_policy?: number;
}> {
  return req("/api/download/mask", {
    method: "POST",
    body: JSON.stringify({ pattern }),
  });
}

export async function previewBatchAction(input: {
  stage: "download" | "csv_conversion" | "parquet_conversion";
  ids?: string[];
  pattern?: string;
  filters?: FileFilters;
}): Promise<BatchPreview> {
  return req("/api/actions/preview", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function enqueueBatchAction(input: {
  stage: "download" | "csv_conversion" | "parquet_conversion";
  ids?: string[];
  pattern?: string;
  filters?: FileFilters;
}): Promise<BatchResult> {
  return req("/api/actions/enqueue", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function reprocessRecentFailures(input: {
  stage?: "download" | "csv_conversion" | "parquet_conversion";
  hours?: number;
}): Promise<{ matched: number; enqueued: number; hours: number; stage: string }> {
  return req("/api/actions/reprocess/failures", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function getBottlenecks(): Promise<{
  items: Array<{ stage: string; pending_count: number; running_count: number; failed_count: number }>;
}> {
  return req("/api/ops/bottlenecks");
}

export async function getFailureReasons(limit = 20): Promise<{
  items: Array<{ stage: string; reason: string; count: number }>;
}> {
  return req(`/api/ops/failure-reasons?limit=${limit}`);
}

export async function getOpsAlerts(): Promise<{ items: Array<Record<string, unknown>> }> {
  return req("/api/ops/alerts");
}

export async function getManualActions(limit = 50): Promise<{ items: Array<Record<string, unknown>> }> {
  return req(`/api/ops/manual-actions?limit=${limit}`);
}

export async function triggerCSV(filename: string): Promise<{ enqueued: string }> {
  return req("/api/convert/csv", {
    method: "POST",
    body: JSON.stringify({ filename }),
  });
}

export async function triggerCSVMask(pattern: string): Promise<{
  enqueued: number;
  skipped: number;
  skipped_by_policy?: number;
}> {
  return req("/api/convert/csv/mask", {
    method: "POST",
    body: JSON.stringify({ pattern }),
  });
}

export async function triggerParquet(filename: string): Promise<{ enqueued: string }> {
  return req("/api/convert/parquet", {
    method: "POST",
    body: JSON.stringify({ filename }),
  });
}

export async function triggerParquetMask(pattern: string): Promise<{
  enqueued: number;
  skipped: number;
  skipped_by_policy?: number;
}> {
  return req("/api/convert/parquet/mask", {
    method: "POST",
    body: JSON.stringify({ pattern }),
  });
}

export async function purgeFile(filename: string): Promise<{ purged?: string; already_purged?: boolean }> {
  return req("/api/purge", {
    method: "POST",
    body: JSON.stringify({ filename }),
  });
}
