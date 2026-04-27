/** Query strings for /files aligned with dashboard KPI semantics (single place to avoid drift). */

export function filesPath(query: Record<string, string>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value) params.set(key, value);
  }
  const q = params.toString();
  return q ? `/files?${q}` : "/files";
}

export function filesPathMultiStatus(statuses: string[]): string {
  const params = new URLSearchParams();
  for (const status of statuses) {
    params.append("status", status);
  }
  return `/files?${params.toString()}`;
}

/** Matches dashboard pipeline_completed_count (stages + processing policy), not overall_status alone. */
export function filesPathPipelineCompleted(): string {
  return "/files?pipeline_completed=1";
}
