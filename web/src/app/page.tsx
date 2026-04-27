"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import Link from "next/link";
import { getDashboardInsights, getFiles } from "@/lib/api";
import type { DashboardInsights, DatFile } from "@/lib/types";
import { ScanButton } from "@/components/ActionButtons";
import { OverallStatusBadge } from "@/components/StageStatusBadge";
import { BrazilStateMap } from "@/components/BrazilStateMap";
import { stateNamePtBR } from "@/lib/stateLabels";
import { formatCatalogLabel } from "@/lib/catalogLabels";
import { formatBytes } from "@/lib/formatBytes";
import { barClassForLevel, sizeToSextileLevel, sortedSizesFromBuckets } from "@/lib/dashboardSizeScale";
import {
  filesPath,
  filesPathMultiStatus,
  filesPathPipelineCompleted,
} from "@/lib/dashboardFileDrill";

const POLL_MS = 10_000;

function formatDateSafe(value: string | null | undefined): string {
  if (!value) return "Data desconhecida";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "Data desconhecida";
  return parsed.toLocaleString();
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

export default function DashboardPage() {
  const [insights, setInsights] = useState<DashboardInsights | null>(null);
  const [recent, setRecent] = useState<DatFile[]>([]);
  const [failed, setFailed] = useState<DatFile[]>([]);
  const [initialized, setInitialized] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const ago = useSecondsAgo(lastUpdated);

  const load = useCallback(async () => {
    setFetching(true);
    setError(null);
    try {
      const [i, r, f] = await Promise.all([
        getDashboardInsights(),
        getFiles({ limit: 10 }),
        getFiles({ status: "failed", limit: 10 }),
      ]);
      setInsights(i);
      setRecent(r.items ?? []);
      setFailed(f.items ?? []);
      setLastUpdated(new Date());
      setInitialized(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao carregar dados do painel.");
    } finally {
      setFetching(false);
    }
  }, []);

  useEffect(() => {
    const boot = setTimeout(() => { void load(); }, 0);
    const t = setInterval(load, POLL_MS);
    return () => { clearTimeout(boot); clearInterval(t); };
  }, [load]);

  const stats = insights?.status_counts ?? insights?.stats ?? {};
  const total = insights?.total_files ?? Object.values(stats).reduce((a, b) => a + b, 0);
  const policyCounts = insights?.policy_counts ?? { pending: 0, ignored: 0 };
  const maxCatalogSize = Math.max(...(insights?.by_catalog.map((x) => x.total_size_bytes) ?? [1]));
  const maxStateSize = Math.max(...(insights?.by_state.map((x) => x.total_size_bytes) ?? [1]));

  const catalogSizeSorted = useMemo(
    () => sortedSizesFromBuckets(insights?.by_catalog ?? []),
    [insights?.by_catalog],
  );
  const stateSizeSorted = useMemo(
    () => sortedSizesFromBuckets(insights?.by_state ?? []),
    [insights?.by_state],
  );

  const kpiCards = [
    { label: "Arquivos totais", value: total, color: "text-[var(--foreground)]", href: "/files" },
    {
      label: "Processados",
      value: insights?.pipeline_completed_count ?? 0,
      color: "text-[var(--status-success-fg)]",
      href: filesPathPipelineCompleted(),
    },
    {
      label: "Em processamento",
      value: (stats?.downloading ?? 0) + (stats?.converting_csv ?? 0) + (stats?.converting_parquet ?? 0),
      color: "text-[var(--status-info-fg)]",
      href: filesPathMultiStatus(["downloading", "converting_csv", "converting_parquet"]),
    },
    { label: "Falhas", value: stats?.failed ?? 0, color: "text-[var(--status-danger-fg)]", href: filesPath({ status: "failed" }) },
    {
      label: "Pendentes",
      value: policyCounts.pending,
      color: "text-[var(--status-warning-fg)]",
      href: filesPath({ status: "pending" }),
    },
    {
      label: "Ignorados",
      value: policyCounts.ignored,
      color: "text-[var(--status-neutral-fg)]",
      href: filesPath({ status: "ignored" }),
    },
  ];

  return (
    <main className="min-h-screen p-6">
      <div className="mx-auto max-w-7xl">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold text-[var(--foreground)]">Painel</h1>
            <p className="mt-1 text-sm text-[var(--muted)]">Acompanhe e opere o pipeline ETL do DATASUS</p>
          </div>
          <div className="flex flex-col items-end gap-2">
            <ScanButton onScan={load} />
            <Link href="/policies" className="secondary-link-chip text-xs">
              Abrir política de processamento
            </Link>
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

        {/* KPI cards */}
        {!initialized ? (
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6 mb-8">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="animate-pulse bg-[var(--border)] rounded-2xl h-24" />
            ))}
          </div>
        ) : (
          <>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6 mb-8">
            {kpiCards.map((card) => (
              <Link key={card.label} href={card.href} className="glass-card rounded-2xl p-4 hover:ring-1 hover:ring-[var(--accent)] transition">
                <p className="text-sm text-[var(--muted)]">{card.label}</p>
                <p className={`text-3xl font-bold ${card.color} mt-1`}>{card.value.toLocaleString()}</p>
              </Link>
            ))}
          </div>
          </>
        )}

        {/* Stage breakdown */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3 mb-8">
          {[
            {
              label: "Downloads concluídos",
              value:
                (stats?.downloaded ?? 0) +
                (stats?.converting_csv ?? 0) +
                (stats?.csv_ready ?? 0) +
                (stats?.converting_parquet ?? 0) +
                (stats?.parquet_ready ?? 0),
              href: filesPathMultiStatus(["downloaded", "converting_csv", "csv_ready", "converting_parquet", "parquet_ready"]),
            },
            {
              label: "CSV pronto",
              value: (stats?.csv_ready ?? 0) + (stats?.converting_parquet ?? 0) + (stats?.parquet_ready ?? 0),
              href: filesPathMultiStatus(["csv_ready", "converting_parquet", "parquet_ready"]),
            },
            { label: "Parquet pronto", value: stats?.parquet_ready ?? 0, href: filesPath({ status: "parquet_ready" }) },
          ].map((s) => (
            <Link key={s.label} href={s.href} className="glass-card rounded-2xl p-4 hover:ring-1 hover:ring-[var(--accent)] transition">
              <p className="text-sm text-[var(--muted)]">{s.label}</p>
              <p className="text-2xl font-semibold text-[var(--foreground)]">{s.value.toLocaleString()}</p>
            </Link>
          ))}
        </div>

        <section className="glass-card-strong rounded-2xl p-4 mb-8">
          <h2 className="mb-3 text-lg font-semibold text-[var(--foreground)]">Mapa do Brasil por tamanho total</h2>
          <BrazilStateMap states={insights?.by_state ?? []} />
        </section>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 mb-8">
          <section className="glass-card-strong rounded-2xl p-4">
            <h2 className="mb-3 text-lg font-semibold text-[var(--foreground)]">Catálogo</h2>
            <ul className="space-y-2">
              {(insights?.by_catalog ?? []).slice(0, 12).map((item) => (
                <li key={item.key}>
                  <Link href={filesPath({ catalog: item.key })} className="block">
                    <div className="mb-1.5 flex flex-wrap items-start justify-between gap-x-3 gap-y-1">
                      <span className="text-sm font-medium text-[var(--foreground)]">{formatCatalogLabel(item.key)}</span>
                      <div className="shrink-0 text-right">
                        <span className="block text-sm font-semibold tabular-nums text-[var(--foreground)]">
                          {formatBytes(item.total_size_bytes)}
                        </span>
                        <span className="block text-xs text-[var(--muted)]">
                          ({item.count.toLocaleString("pt-BR")} arquivos)
                        </span>
                      </div>
                    </div>
                    <div className="h-2 w-full rounded-full bg-[var(--border)]">
                      <div
                        className={`h-2 rounded-full ${barClassForLevel(sizeToSextileLevel(catalogSizeSorted, item.total_size_bytes))}`}
                        style={{ width: `${Math.max(5, (item.total_size_bytes / maxCatalogSize) * 100)}%` }}
                      />
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          </section>

          <section className="glass-card-strong rounded-2xl p-4">
            <h2 className="mb-3 text-lg font-semibold text-[var(--foreground)]">Estado</h2>
            <ul className="space-y-2">
              {(insights?.by_state ?? []).slice(0, 12).map((item) => (
                <li key={item.key}>
                  <Link href={filesPath({ state: item.key })} className="block">
                    <div className="mb-1.5 flex flex-wrap items-start justify-between gap-x-3 gap-y-1">
                      <span className="text-sm font-medium text-[var(--foreground)]">{stateNamePtBR(item.key)}</span>
                      <div className="shrink-0 text-right">
                        <span className="block text-sm font-semibold tabular-nums text-[var(--foreground)]">
                          {formatBytes(item.total_size_bytes)}
                        </span>
                        <span className="block text-xs text-[var(--muted)]">
                          ({item.count.toLocaleString("pt-BR")} arquivos)
                        </span>
                      </div>
                    </div>
                    <div className="h-2 w-full rounded-full bg-[var(--border)]">
                      <div
                        className={`h-2 rounded-full ${barClassForLevel(sizeToSextileLevel(stateSizeSorted, item.total_size_bytes))}`}
                        style={{ width: `${Math.max(5, (item.total_size_bytes / maxStateSize) * 100)}%` }}
                      />
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        </div>

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          {/* Recent failures */}
          <section>
            <h2 className="mb-3 text-lg font-semibold text-[var(--foreground)]">Falhas recentes</h2>
            {!initialized ? (
              <div className="space-y-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div key={i} className="animate-pulse bg-[var(--border)] rounded-2xl h-14" />
                ))}
              </div>
            ) : failed.length === 0 ? (
              <p className="glass-card rounded-2xl p-4 text-sm text-[var(--muted)]">Sem falhas recentes</p>
            ) : (
              <ul className="space-y-2">
                {failed.map((f, idx) => (
                  <li
                    key={`${f.id}-${f.remote_timestamp ?? f.last_seen_at}-${idx}`}
                    className="glass-card rounded-2xl p-3 flex items-center justify-between"
                  >
                    <div>
                      <Link href={`/files/${f.id}`} className="text-sm font-medium text-[var(--accent)] hover:underline">
                        {f.filename}
                      </Link>
                      <p className="text-xs text-[var(--muted)]">
                        {formatCatalogLabel(f.catalog)} · {stateNamePtBR(f.state)} · {f.year}/{String(f.month).padStart(2, "0")}
                      </p>
                    </div>
                    <OverallStatusBadge status={f.overall_status} />
                  </li>
                ))}
              </ul>
            )}
          </section>

          {/* Recent activity */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-lg font-semibold text-[var(--foreground)]">Atividade recente</h2>
              <Link href="/files" className="secondary-link-chip">
                Ver todos os arquivos
              </Link>
            </div>
            {!initialized ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <div key={i} className="animate-pulse bg-[var(--border)] rounded-2xl h-14" />
                ))}
              </div>
            ) : (
              <ul className="space-y-2">
                {recent.map((f, idx) => (
                  <li
                    key={`${f.id}-${f.remote_timestamp ?? f.last_seen_at}-${idx}`}
                    className="glass-card rounded-2xl p-3 flex items-center justify-between"
                  >
                    <div>
                      <Link href={`/files/${f.id}`} className="text-sm font-medium text-[var(--accent)] hover:underline">
                        {f.filename}
                      </Link>
                      <p className="text-xs text-[var(--muted)]">{formatDateSafe(f.remote_timestamp ?? f.last_seen_at)}</p>
                    </div>
                    <OverallStatusBadge status={f.overall_status} />
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      </div>
    </main>
  );
}
