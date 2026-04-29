"use client";

import { Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { getFileFacets, getFiles } from "@/lib/api";
import type { DatFile, FileFacetsResponse, FileFilters, FilePolicyMatch, OverallStatus } from "@/lib/types";
import { OverallStatusBadge } from "@/components/StageStatusBadge";
import { stateNamePtBR } from "@/lib/stateLabels";
import { formatCatalogLabel } from "@/lib/catalogLabels";
import { overallStatusLabel } from "@/lib/statusLabels";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { areSearchParamsEquivalent, canonicalSearchParamsString } from "@/lib/searchParamsCanonical";
import { formatDateTimeBR } from "@/lib/dateFormat";

const POLL_MS = 15_000;
const DEBOUNCE_MS = 250;
const PAGE_SIZE = 100;

/** Same value the API uses when sorting by last_seen_at: COALESCE(remote_timestamp, last_seen_at). */
function displayFileTimestamp(f: DatFile): string | null {
  return f.remote_timestamp ?? f.last_seen_at ?? null;
}

function periodKey(year: number, month: number): string {
  return `${year}-${String(month).padStart(2, "0")}`;
}

function periodLabel(key: string): string {
  const [year, month] = key.split("-");
  return `${month}/${year}`;
}

function normalizePolicyMatchParam(raw: string | null): FilePolicyMatch | "" {
  const v = (raw ?? "").trim().toLowerCase();
  if (v === "pending" || v === "ignored") return v;
  return "";
}

function normalizePipelineCompletedParam(raw: string | null): boolean {
  const v = (raw ?? "").trim().toLowerCase();
  return v === "1" || v === "true" || v === "yes" || v === "on";
}

function parseCatalogsFromSearchParams(sp: { get: (k: string) => string | null; getAll: (k: string) => string[] }): string[] {
  const fromQuery = sp.getAll("catalog").map((item) => item.trim().toUpperCase()).filter(Boolean);
  if (fromQuery.length > 0) return fromQuery;
  return (sp.get("catalogs") ?? "").split(",").map((item) => item.trim().toUpperCase()).filter(Boolean);
}

function useDebouncedValue<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(t);
  }, [value, delay]);
  return debounced;
}

function useSecondsAgo(date: Date | null): string | null {
  const [now, setNow] = useState(0);
  useEffect(() => {
    if (!date) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [date]);
  if (!date) return null;
  if (now === 0) return "agora";
  const s = Math.floor((now - date.getTime()) / 1000);
  return s < 5 ? "agora" : `há ${s}s`;
}

function parseSortDirFromSearchParams(sp: { get: (k: string) => string | null }): "asc" | "desc" {
  const sd = sp.get("sort_dir");
  return sd === "asc" || sd === "desc" ? sd : "desc";
}

function parsePageFromSearchParams(sp: { get: (k: string) => string | null }): number {
  const pg = sp.get("page");
  return pg ? Math.max(0, parseInt(pg, 10) - 1) : 0;
}

function FilesPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [files, setFiles] = useState<DatFile[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(() => parsePageFromSearchParams(searchParams));
  const [facets, setFacets] = useState<FileFacetsResponse>({ catalogs: [], states: [], statuses: [], periods: [] });
  const [facetsLoaded, setFacetsLoaded] = useState(false);
  const [initialized, setInitialized] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [tableBusy, setTableBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const ago = useSecondsAgo(lastUpdated);

  const [rawFilename, setRawFilename] = useState(() => searchParams.get("filename") ?? "");
  const [selectedCatalogs, setSelectedCatalogs] = useState(() => parseCatalogsFromSearchParams(searchParams));
  const [selectedStates, setSelectedStates] = useState(() =>
    searchParams.getAll("state").map((item) => item.trim().toUpperCase()).filter(Boolean),
  );
  const [selectedStatuses, setSelectedStatuses] = useState<OverallStatus[]>(() =>
    searchParams.getAll("status").map((item) => item.trim().toLowerCase() as OverallStatus).filter(Boolean),
  );
  const [rawPeriodFrom, setRawPeriodFrom] = useState(() => searchParams.get("period_from") ?? "");
  const [rawPeriodTo, setRawPeriodTo] = useState(() => searchParams.get("period_to") ?? "");
  const [policyMatchStatus, setPolicyMatchStatus] = useState<FilePolicyMatch | "">(() =>
    normalizePolicyMatchParam(searchParams.get("policy_match")),
  );
  const [pipelineCompleted, setPipelineCompleted] = useState(() =>
    normalizePipelineCompletedParam(searchParams.get("pipeline_completed")),
  );

  const filename = useDebouncedValue(rawFilename, DEBOUNCE_MS);
  const catalogsDebounced = useDebouncedValue(selectedCatalogs, DEBOUNCE_MS);
  const statesDebounced = useDebouncedValue(selectedStates, DEBOUNCE_MS);
  const statusesDebounced = useDebouncedValue(selectedStatuses, DEBOUNCE_MS);

  const [sortBy, setSortBy] = useState(() => searchParams.get("sort_by") ?? "last_seen_at");
  const [sortDir, setSortDir] = useState<"asc" | "desc">(() => parseSortDirFromSearchParams(searchParams));

  const searchParamsSnapshot = searchParams.toString();
  const urlCanonicalKey = useMemo(
    () => canonicalSearchParamsString(searchParamsSnapshot),
    [searchParamsSnapshot],
  );

  const periodOptions = useMemo(
    () => facets.periods.map((item) => periodKey(item.year, item.month)),
    [facets.periods],
  );
  const periodOptionSet = useMemo(() => new Set(periodOptions), [periodOptions]);
  // Until facets load, keep period_* from the URL in sync; avoid stripping deep links while periodOptionSet is empty.
  const normalizedPeriodFrom =
    !facetsLoaded || rawPeriodFrom === "" ? rawPeriodFrom : periodOptionSet.has(rawPeriodFrom) ? rawPeriodFrom : "";
  const normalizedPeriodTo =
    !facetsLoaded || rawPeriodTo === "" ? rawPeriodTo : periodOptionSet.has(rawPeriodTo) ? rawPeriodTo : "";

  const from = useMemo(() => {
    const [y, m] = normalizedPeriodFrom.split("-").map(Number);
    if (!Number.isFinite(y) || !Number.isFinite(m)) return null;
    return { year: y, month: m };
  }, [normalizedPeriodFrom]);
  const to = useMemo(() => {
    const [y, m] = normalizedPeriodTo.split("-").map(Number);
    if (!Number.isFinite(y) || !Number.isFinite(m)) return null;
    return { year: y, month: m };
  }, [normalizedPeriodTo]);
  const isPeriodRangeValid = useMemo(() => {
    if (!from || !to) return true;
    return (from.year < to.year) || (from.year === to.year && from.month <= to.month);
  }, [from, to]);

  useLayoutEffect(() => {
    /* URL (browser history) is external; hydrate React state before the push useEffect runs. */
    /* eslint-disable react-hooks/set-state-in-effect */
    setRawFilename(searchParams.get("filename") ?? "");
    setSelectedCatalogs(parseCatalogsFromSearchParams(searchParams));
    setSelectedStates(searchParams.getAll("state").map((item) => item.trim().toUpperCase()).filter(Boolean));
    setSelectedStatuses(
      searchParams.getAll("status").map((item) => item.trim().toLowerCase() as OverallStatus).filter(Boolean),
    );
    setRawPeriodFrom(searchParams.get("period_from") ?? "");
    setRawPeriodTo(searchParams.get("period_to") ?? "");
    setPolicyMatchStatus(normalizePolicyMatchParam(searchParams.get("policy_match")));
    setPipelineCompleted(normalizePipelineCompletedParam(searchParams.get("pipeline_completed")));
    setSortBy(searchParams.get("sort_by") ?? "last_seen_at");
    setSortDir(parseSortDirFromSearchParams(searchParams));
    setPage(parsePageFromSearchParams(searchParams));
    /* eslint-enable react-hooks/set-state-in-effect */
    // eslint-disable-next-line react-hooks/exhaustive-deps -- searchParams matches the render for this urlCanonicalKey
  }, [urlCanonicalKey]);

  const filters: FileFilters = useMemo(() => {
    const statusesWithPolicy =
      policyMatchStatus && !statusesDebounced.includes(policyMatchStatus)
        ? [...statusesDebounced, policyMatchStatus]
        : statusesDebounced;

    return {
      ...(filename && { filename }),
      ...(catalogsDebounced.length > 0 && { catalogs: catalogsDebounced }),
      ...(statesDebounced.length > 0 && { states: statesDebounced }),
      ...(statusesWithPolicy.length > 0 && { statuses: statusesWithPolicy }),
      ...(from && to && isPeriodRangeValid && { period_from_year: from.year, period_from_month: from.month, period_to_year: to.year, period_to_month: to.month }),
      ...(policyMatchStatus && { policy_match: policyMatchStatus }),
      ...(pipelineCompleted && { pipeline_completed: true }),
      sort_by: sortBy,
      sort_dir: sortDir,
    };
  }, [
    filename,
    catalogsDebounced,
    statesDebounced,
    statusesDebounced,
    policyMatchStatus,
    pipelineCompleted,
    sortBy,
    sortDir,
    from,
    to,
    isPeriodRangeValid,
  ]);

  const load = useCallback(async (mode: "interactive" | "background" = "interactive") => {
    if (mode === "interactive") setTableBusy(true);
    setFetching(true);
    setError(null);
    try {
      const res = await getFiles({ ...filters, limit: PAGE_SIZE, offset: page * PAGE_SIZE });
      setFiles(res.items ?? []);
      setTotal(res.total);
      setLastUpdated(new Date());
      setInitialized(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao carregar arquivos.");
    } finally {
      setFetching(false);
      if (mode === "interactive") setTableBusy(false);
    }
  }, [filters, page]);

  useEffect(() => {
    const boot = setTimeout(() => {
      void (async () => {
        try {
          const response = await getFileFacets();
          setFacets(response);
        } catch {
          setFacets({ catalogs: [], states: [], statuses: [], periods: [] });
        } finally {
          setFacetsLoaded(true);
        }
      })();
    }, 0);
    return () => clearTimeout(boot);
  }, []);

  useEffect(() => {
    const boot = setTimeout(() => { void load("interactive"); }, 0);
    const t = setInterval(() => {
      if (typeof document !== "undefined" && document.visibilityState !== "visible") return;
      void load("background");
    }, POLL_MS);
    return () => { clearTimeout(boot); clearInterval(t); };
  }, [load]);

  useEffect(() => {
    const params = new URLSearchParams();
    if (rawFilename) params.set("filename", rawFilename);
    for (const catalog of selectedCatalogs) params.append("catalog", catalog);
    for (const state of selectedStates) params.append("state", state);
    if (normalizedPeriodFrom) params.set("period_from", normalizedPeriodFrom);
    if (normalizedPeriodTo) params.set("period_to", normalizedPeriodTo);
    for (const status of selectedStatuses) params.append("status", status);
    if (policyMatchStatus) params.set("policy_match", policyMatchStatus);
    if (pipelineCompleted) params.set("pipeline_completed", "1");
    params.set("sort_by", sortBy);
    params.set("sort_dir", sortDir);
    params.set("page_size", String(PAGE_SIZE));
    if (page > 0) params.set("page", String(page + 1));
    const nextQuery = params.toString();
    if (areSearchParamsEquivalent(nextQuery, urlCanonicalKey)) return;
    router.replace(nextQuery ? `/files?${nextQuery}` : "/files");
  }, [rawFilename, selectedCatalogs, selectedStates, normalizedPeriodFrom, normalizedPeriodTo, selectedStatuses, policyMatchStatus, pipelineCompleted, page, sortBy, sortDir, router, urlCanonicalKey]);

  const toggleCatalog = (value: string) => {
    setSelectedCatalogs((prev) => prev.includes(value) ? prev.filter((item) => item !== value) : [...prev, value]);
    setPage(0);
  };
  const toggleState = (value: string) => {
    setSelectedStates((prev) => prev.includes(value) ? prev.filter((item) => item !== value) : [...prev, value]);
    setPage(0);
  };
  const toggleStatus = (value: OverallStatus) => {
    setSelectedStatuses((prev) => prev.includes(value) ? prev.filter((item) => item !== value) : [...prev, value]);
    setPage(0);
  };
  const clearAllFilters = () => {
    setRawFilename("");
    setSelectedCatalogs([]);
    setSelectedStates([]);
    setSelectedStatuses([]);
    setRawPeriodFrom("");
    setRawPeriodTo("");
    setPolicyMatchStatus("");
    setPipelineCompleted(false);
    setPage(0);
  };
  const onSort = (column: string) => {
    if (sortBy === column) {
      setSortDir((prev) => prev === "asc" ? "desc" : "asc");
      return;
    }
    setSortBy(column);
    setSortDir("asc");
  };

  const activeFilters = [
    ...(rawFilename ? [{ key: "filename", label: `Arquivo: ${rawFilename}`, clear: () => setRawFilename("") }] : []),
    ...selectedCatalogs.map((catalog) => ({ key: `catalog-${catalog}`, label: `Catálogo: ${catalog}`, clear: () => setSelectedCatalogs((prev) => prev.filter((item) => item !== catalog)) })),
    ...selectedStates.map((state) => ({ key: `state-${state}`, label: `Estado: ${state}`, clear: () => setSelectedStates((prev) => prev.filter((item) => item !== state)) })),
    ...(policyMatchStatus
      ? [{ key: "policy-match", label: `Status (política): ${overallStatusLabel(policyMatchStatus)}`, clear: () => { setPolicyMatchStatus(""); setPage(0); } }]
      : []),
    ...(pipelineCompleted
      ? [{
        key: "pipeline-completed",
        label: "Pipeline concluído (política)",
        clear: () => { setPipelineCompleted(false); setPage(0); },
      }]
      : []),
    ...selectedStatuses.map((status) => ({ key: `status-${status}`, label: `Status: ${overallStatusLabel(status)}`, clear: () => setSelectedStatuses((prev) => prev.filter((item) => item !== status)) })),
    ...(normalizedPeriodFrom ? [{ key: "period-from", label: `De: ${periodLabel(normalizedPeriodFrom)}`, clear: () => setRawPeriodFrom("") }] : []),
    ...(normalizedPeriodTo ? [{ key: "period-to", label: `Até: ${periodLabel(normalizedPeriodTo)}`, clear: () => setRawPeriodTo("") }] : []),
  ];

  const endPeriodOptions = useMemo(() => {
    if (!normalizedPeriodFrom) return periodOptions;
    return periodOptions.filter((key) => key >= normalizedPeriodFrom);
  }, [periodOptions, normalizedPeriodFrom]);

  return (
    <main className="min-h-screen p-6">
      <div className="mx-auto max-w-7xl">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-2xl font-bold text-[var(--foreground)]">Arquivos</h1>
            <p className="text-sm text-[var(--muted)]">{total.toLocaleString()} arquivos</p>
          </div>
          <div className="flex flex-col items-end gap-1">
            {ago && (
              <span className="flex items-center gap-1.5 text-xs text-[var(--muted)]">
                <span className={`h-1.5 w-1.5 rounded-full ${fetching ? "bg-blue-400 animate-pulse" : "bg-emerald-500"}`} />
                Atualizado {ago}
              </span>
            )}
          </div>
        </div>
        {error && (
          <p className="mb-4 rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-sm text-[var(--status-danger-fg)]">
            {error}
          </p>
        )}

        <div className="glass-card-strong mb-4 rounded-2xl p-4">
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <label className="space-y-1">
              <span className="text-xs font-medium text-[var(--muted)]">Nome do arquivo</span>
              <input
                placeholder="Busca estilo glob, RDSP2401 ou RDSP*01"
                value={rawFilename}
                className="form-control w-full rounded-xl px-3 py-2 text-sm"
                onChange={(e) => { setRawFilename(e.target.value); setPage(0); }}
              />
              <span className="text-[11px] text-[var(--muted)]">Use apenas *, _ e % são literais.</span>
            </label>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="space-y-1">
                <span className="text-xs font-medium text-[var(--muted)]">Período de</span>
                <Select
                  value={normalizedPeriodFrom || "__all"}
                  onValueChange={(next) => {
                    const value = next === "__all" ? "" : next;
                    setRawPeriodFrom(value);
                    setRawPeriodTo((prev) => (prev && value && prev < value ? "" : prev));
                    setPage(0);
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Selecione" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__all">Selecione</SelectItem>
                  {periodOptions.map((key) => (
                    <SelectItem key={key} value={key}>{periodLabel(key)}</SelectItem>
                  ))}
                  </SelectContent>
                </Select>
              </label>
              <label className="space-y-1">
                <span className="text-xs font-medium text-[var(--muted)]">Período até</span>
                <Select
                  value={normalizedPeriodTo || "__all"}
                  onValueChange={(value) => { setRawPeriodTo(value === "__all" ? "" : value); setPage(0); }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Selecione" />
                  </SelectTrigger>
                  <SelectContent>
                  <SelectItem value="__all">Selecione</SelectItem>
                  {endPeriodOptions.map((key) => (
                    <SelectItem key={key} value={key}>{periodLabel(key)}</SelectItem>
                  ))}
                  </SelectContent>
                </Select>
              </label>
            </div>
          </div>

          {!isPeriodRangeValid && (
            <p className="mt-3 rounded-xl border border-amber-500/35 bg-amber-500/10 px-3 py-2 text-xs text-[var(--status-warning-fg)]">
              O período selecionado não é válido, ajuste o intervalo para continuar.
            </p>
          )}

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <div className="space-y-2">
              <p className="text-xs font-medium text-[var(--muted)]">Catálogos, múltipla escolha</p>
              <div className="flex max-h-36 flex-wrap gap-2 overflow-auto rounded-xl border border-[var(--border)] p-2">
                {facets.catalogs.map((catalog) => {
                  const active = selectedCatalogs.includes(catalog);
                  return (
                    <button
                      key={catalog}
                      type="button"
                      onClick={() => toggleCatalog(catalog)}
                      className={`rounded-full border px-3 py-1 text-xs ${active ? "border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--foreground)]" : "border-[var(--border)] text-[var(--muted)]"}`}
                    >
                      {catalog}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="space-y-2">
              <p className="text-xs font-medium text-[var(--muted)]">Estados, múltipla escolha</p>
              <div className="flex max-h-36 flex-wrap gap-2 overflow-auto rounded-xl border border-[var(--border)] p-2">
                {facets.states.map((state) => {
                  const active = selectedStates.includes(state);
                  return (
                    <button
                      key={state}
                      type="button"
                      onClick={() => toggleState(state)}
                      className={`rounded-full border px-3 py-1 text-xs ${active ? "border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--foreground)]" : "border-[var(--border)] text-[var(--muted)]"}`}
                    >
                      {state}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="space-y-2">
              <p className="text-xs font-medium text-[var(--muted)]">Status, múltipla escolha</p>
              <div className="flex max-h-36 flex-wrap gap-2 overflow-auto rounded-xl border border-[var(--border)] p-2">
                {facets.statuses.map((status) => {
                  const active = selectedStatuses.includes(status);
                  return (
                    <button
                      key={status}
                      type="button"
                      onClick={() => toggleStatus(status)}
                      className={`rounded-full border px-3 py-1 text-xs ${active ? "border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--foreground)]" : "border-[var(--border)] text-[var(--muted)]"}`}
                    >
                      {overallStatusLabel(status)}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            <span className="text-xs text-[var(--muted)]">Filtros ativos</span>
            {activeFilters.length === 0 ? (
              <span className="text-xs text-[var(--muted)]">nenhum</span>
            ) : (
              activeFilters.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  onClick={() => { item.clear(); setPage(0); }}
                  className="rounded-full border border-[var(--border)] px-3 py-1 text-xs text-[var(--foreground)]"
                >
                  {item.label} x
                </button>
              ))
            )}
            {activeFilters.length > 0 && (
              <button type="button" onClick={clearAllFilters} className="rounded-full border border-[var(--border)] px-3 py-1 text-xs text-[var(--muted)]">
                Limpar tudo
              </button>
            )}
          </div>
        </div>

        <div className="glass-card-strong relative overflow-hidden rounded-2xl">
          {tableBusy && initialized ? (
            <div
              className="absolute inset-0 z-10 flex items-center justify-center bg-[var(--background)]/55 backdrop-blur-[1px]"
              aria-live="polite"
            >
              <div className="flex items-center gap-2 rounded-xl border border-[var(--border)] bg-[var(--background)] px-4 py-2 text-sm text-[var(--foreground)] shadow-sm">
                <span
                  className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-[var(--accent)] border-t-transparent"
                  aria-hidden={true}
                />
                Atualizando…
              </div>
            </div>
          ) : null}
          <div
            className={`overflow-x-auto ${tableBusy && initialized ? "pointer-events-none select-none opacity-60" : ""}`}
          >
            <table className="min-w-[860px] w-full text-sm" aria-busy={tableBusy && initialized}>
            <thead className="border-b border-[var(--border)] bg-[var(--accent-soft)]/40">
              <tr>
                {[
                  { label: "Arquivo", sort: "filename" },
                  { label: "Catálogo", sort: "catalog" },
                  { label: "Estado", sort: "state" },
                  { label: "Ano/Mês", sort: "year_month" },
                  { label: "Status", sort: "overall_status" },
                  { label: "Última atualização", sort: "last_seen_at" },
                  { label: "", sort: "" },
                ].map((h) => (
                  <th
                    key={h.label || "details"}
                    className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-[var(--muted)]"
                    aria-sort={h.sort ? (sortBy === h.sort ? (sortDir === "asc" ? "ascending" : "descending") : "none") : undefined}
                  >
                    {h.sort ? (
                      <button
                        type="button"
                        onClick={() => onSort(h.sort)}
                        disabled={tableBusy}
                        className="inline-flex items-center gap-1 hover:text-[var(--foreground)] disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        <span>{h.label}</span>
                        <span className={sortBy === h.sort ? "text-[var(--foreground)]" : ""}>
                          {sortBy === h.sort ? (sortDir === "asc" ? "↑" : "↓") : "↕"}
                        </span>
                      </button>
                    ) : h.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {!initialized ? (
                Array.from({ length: 8 }).map((_, i) => (
                  <tr key={i} className="border-b border-[var(--border)]/70">
                    {Array.from({ length: 7 }).map((_, j) => (
                      <td key={j} className="px-4 py-3">
                        <div className="animate-pulse bg-[var(--border)] rounded h-4 w-full" />
                      </td>
                    ))}
                  </tr>
                ))
              ) : files.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-10 text-center text-[var(--muted)]">
                    <p className="text-sm text-[var(--foreground)]">Nenhum arquivo encontrado para os filtros ativos.</p>
                    <p className="mt-1 text-xs text-[var(--muted)]">Revise os filtros ou volte para uma busca mais ampla.</p>
                    <button type="button" onClick={clearAllFilters} className="mt-3 rounded-full border border-[var(--border)] px-3 py-1 text-xs">
                      Limpar filtros
                    </button>
                  </td>
                </tr>
              ) : (
                files.map((f) => (
                  <tr key={f.id} className="border-b border-[var(--border)]/70 transition hover:bg-[var(--accent-soft)]/40">
                    <td className="px-4 py-3 font-mono font-medium">{f.filename}</td>
                    <td className="px-4 py-3">{formatCatalogLabel(f.catalog)}</td>
                    <td className="px-4 py-3">{stateNamePtBR(f.state)}</td>
                    <td className="px-4 py-3">{f.year}/{String(f.month).padStart(2, "0")}</td>
                    <td className="px-4 py-3"><OverallStatusBadge status={f.overall_status} /></td>
                    <td className="px-4 py-3 text-xs text-[var(--muted)]">{formatDateTimeBR(displayFileTimestamp(f))}</td>
                    <td className="px-4 py-3">
                      <Link href={`/files/${f.id}`} className="text-xs text-[var(--accent)] hover:underline">
                        Detalhes
                      </Link>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
            </table>
          </div>
        </div>

        <div className="mt-4 flex items-center justify-between text-sm text-[var(--muted)]">
          <span>
            Exibindo {total === 0 ? 0 : page * PAGE_SIZE + 1} a {Math.min((page + 1) * PAGE_SIZE, total)} de {total}
          </span>
          <span>
            Página {page + 1} de {Math.max(1, Math.ceil(total / PAGE_SIZE))}
          </span>
          <div className="flex gap-2">
            <Button
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={page === 0 || tableBusy}
              variant="secondary"
              size="sm"
            >
              Página anterior
            </Button>
            <Button
              onClick={() => setPage((p) => p + 1)}
              disabled={(page + 1) * PAGE_SIZE >= total || tableBusy}
              variant="secondary"
              size="sm"
            >
              Próxima página
            </Button>
          </div>
        </div>
      </div>
    </main>
  );
}

function FilesPageFallback() {
  return (
    <main className="min-h-screen p-6">
      <div className="mx-auto max-w-7xl">
        <div className="mb-6 h-8 w-48 animate-pulse rounded-lg bg-[var(--border)]" />
        <div className="glass-card-strong mb-4 h-40 animate-pulse rounded-2xl bg-[var(--border)]/60" />
        <div className="glass-card-strong h-96 animate-pulse rounded-2xl bg-[var(--border)]/60" />
      </div>
    </main>
  );
}

export default function FilesPage() {
  return (
    <Suspense fallback={<FilesPageFallback />}>
      <FilesPageContent />
    </Suspense>
  );
}
