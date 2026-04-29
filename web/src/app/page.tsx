"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import Link from "next/link";
import { getDashboardInsights, getFiles } from "@/lib/api";
import type { DashboardInsights, DatFile } from "@/lib/types";
import { ScanButton } from "@/components/ActionButtons";
import { ContextualHint } from "@/components/ContextualHint";
import { OverallStatusBadge } from "@/components/StageStatusBadge";
import { BrazilStateMap } from "@/components/BrazilStateMap";
import { stateNamePtBR } from "@/lib/stateLabels";
import { formatCatalogLabel } from "@/lib/catalogLabels";
import { formatBytes } from "@/lib/formatBytes";
import { formatDateTimeBR } from "@/lib/dateFormat";
import { barClassForLevel, sizeToSextileLevel, sortedSizesFromBuckets } from "@/lib/dashboardSizeScale";
import {
  filesPath,
  filesPathMultiStatus,
  filesPathPipelineCompleted,
} from "@/lib/dashboardFileDrill";

const POLL_MS = 10_000;

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

type KpiCard = {
  label: string;
  value: number;
  color: string;
  href: string;
  hint?: string;
  cardClassName?: string;
};

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
    const boot = setTimeout(() => {
      void load();
    }, 0);
    const t = setInterval(load, POLL_MS);
    return () => {
      clearTimeout(boot);
      clearInterval(t);
    };
  }, [load]);

  const stats = useMemo(
    () => insights?.status_counts ?? insights?.stats ?? {},
    [insights?.status_counts, insights?.stats],
  );
  const total = insights?.total_files ?? Object.values(stats).reduce((a, b) => a + b, 0);
  const policyCounts = useMemo(
    () => insights?.policy_counts ?? { pending: 0, ignored: 0 },
    [insights?.policy_counts],
  );
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

  const resumoHint = useMemo(() => {
    if (!insights?.stage_done_counts) return undefined;
    const d = insights.stage_done_counts;
    return `Etapas marcadas como concluídas no registro: download ${d.download.toLocaleString("pt-BR")}, CSV ${d.csv_conversion.toLocaleString("pt-BR")}, Parquet ${d.parquet_conversion.toLocaleString("pt-BR")}.`;
  }, [insights?.stage_done_counts]);

  const kpiCards = useMemo((): KpiCard[] => {
    if (!insights) {
      return [];
    }
    const mismatch = insights.status_stage_mismatch_count ?? 0;
    const catMis = insights.by_catalog_total_mismatch ?? 0;
    const stMis = insights.by_state_total_mismatch ?? 0;
    const bucketMismatch = catMis !== 0 || stMis !== 0;

    const totalHintParts = ["Soma de todas as contagens por status no cadastro."];
    if (bucketMismatch) {
      totalHintParts.push(
        `Diferença entre a soma dos agrupamentos e este total: catálogo ${catMis.toLocaleString("pt-BR")}, estado ${stMis.toLocaleString("pt-BR")}. Comum quando há registros sem UF ou catálogo preenchidos.`,
      );
    }

    const pipelineHint =
      mismatch > 0
        ? `Arquivos que completaram todas as etapas exigidas pela política. Há ${mismatch.toLocaleString("pt-BR")} caso(s) em que o status do arquivo não coincide com as etapas esperadas; use a lista de arquivos para inspecionar.`
        : "Arquivos que completaram todas as etapas exigidas pela política de processamento atual.";

    return [
      {
        label: "Arquivos",
        value: total,
        color: "text-[var(--foreground)]",
        href: "/files",
        hint: totalHintParts.join(" "),
      },
      {
        label: "Concluídos",
        value: insights.pipeline_completed_count ?? 0,
        color: "text-[var(--status-success-fg)]",
        href: filesPathPipelineCompleted(),
        hint: pipelineHint,
        cardClassName: mismatch > 0 ? "ring-1 ring-amber-500/35" : undefined,
      },
      {
        label: "Em processamento",
        value: (stats?.downloading ?? 0) + (stats?.converting_csv ?? 0) + (stats?.converting_parquet ?? 0),
        color: "text-[var(--status-info-fg)]",
        href: filesPathMultiStatus(["downloading", "converting_csv", "converting_parquet"]),
        hint: "Soma dos status em download, conversão CSV e conversão Parquet.",
      },
      {
        label: "Falhas",
        value: stats?.failed ?? 0,
        color: "text-[var(--status-danger-fg)]",
        href: filesPath({ status: "failed" }),
        hint: "Arquivos com status de falha no pipeline.",
      },
      {
        label: "Pendentes",
        value: policyCounts.pending,
        color: "text-[var(--status-warning-fg)]",
        href: filesPath({ status: "pending" }),
        hint: "Arquivos ainda pendentes de processamento.",
      },
      {
        label: "Ignorados",
        value: policyCounts.ignored,
        color: "text-[var(--status-neutral-fg)]",
        href: filesPath({ status: "ignored" }),
        hint: "Arquivos marcados como ignorados pela operação.",
      },
    ];
  }, [insights, total, stats, policyCounts]);

  return (
    <main className="min-h-screen p-6">
      <div className="mx-auto max-w-7xl">
        <header className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h1 className="text-3xl font-bold text-[var(--foreground)]">Painel</h1>
            <p className="mt-1 text-sm text-[var(--muted)]">Resumo operacional do pipeline</p>
          </div>
          <div className="flex flex-col items-stretch gap-2 sm:items-end">
            <ScanButton onScan={load} />
            {ago ? (
              <span className="flex items-center justify-end gap-1.5 text-xs text-[var(--muted)]">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${fetching ? "animate-pulse bg-blue-400" : "bg-emerald-500"}`}
                />
                Atualizado {ago}
              </span>
            ) : null}
          </div>
        </header>

        {error ? (
          <p className="mb-4 rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-sm text-[var(--status-danger-fg)]">
            {error}
          </p>
        ) : null}

        <section aria-labelledby="resumo-heading" className="mb-8">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            {resumoHint ? (
              <ContextualHint text={resumoHint}>
                <h2 id="resumo-heading" className="text-sm font-medium text-[var(--foreground)]">
                  Resumo
                </h2>
              </ContextualHint>
            ) : (
              <h2 id="resumo-heading" className="text-sm font-medium text-[var(--foreground)]">
                Resumo
              </h2>
            )}
          </div>
          {!initialized ? (
            <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="h-24 animate-pulse rounded-2xl bg-[var(--border)]" />
              ))}
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
              {kpiCards.map((card) => (
                card.hint ? (
                  <ContextualHint
                    key={card.label}
                    text={card.hint}
                    ariaLabel={`${card.label}: ${card.value.toLocaleString("pt-BR")}`}
                  >
                    <div
                      className={`glass-card rounded-2xl p-4 transition hover:ring-1 hover:ring-[var(--accent)] ${card.cardClassName ?? ""}`}
                    >
                      <Link href={card.href} className="block" title={card.hint}>
                        <p className="min-w-0 text-sm text-[var(--muted)]" title={card.hint}>
                          {card.label}
                        </p>
                        <p className={`mt-1 text-3xl font-bold tabular-nums ${card.color}`} title={card.hint}>
                          {card.value.toLocaleString("pt-BR")}
                        </p>
                      </Link>
                    </div>
                  </ContextualHint>
                ) : (
                  <div
                    key={card.label}
                    className={`glass-card rounded-2xl p-4 transition hover:ring-1 hover:ring-[var(--accent)] ${card.cardClassName ?? ""}`}
                  >
                    <Link href={card.href} className="block">
                      <p className="min-w-0 text-sm text-[var(--muted)]">{card.label}</p>
                      <p className={`mt-1 text-3xl font-bold tabular-nums ${card.color}`}>
                        {card.value.toLocaleString("pt-BR")}
                      </p>
                    </Link>
                  </div>
                )
              ))}
            </div>
          )}
        </section>

        <section aria-label="Funil de progresso" className="mb-8">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            {[
              {
                label: "Download",
                value:
                  (stats?.downloaded ?? 0) +
                  (stats?.converting_csv ?? 0) +
                  (stats?.csv_ready ?? 0) +
                  (stats?.converting_parquet ?? 0) +
                  (stats?.parquet_ready ?? 0),
                href: filesPathMultiStatus(["downloaded", "converting_csv", "csv_ready", "converting_parquet", "parquet_ready"]),
              },
              {
                label: "CSV",
                value: (stats?.csv_ready ?? 0) + (stats?.converting_parquet ?? 0) + (stats?.parquet_ready ?? 0),
                href: filesPathMultiStatus(["csv_ready", "converting_parquet", "parquet_ready"]),
              },
              {
                label: "Parquet",
                value: stats?.parquet_ready ?? 0,
                href: filesPath({ status: "parquet_ready" }),
              },
            ].map((s) => (
              <Link
                key={s.label}
                href={s.href}
                className="glass-card rounded-2xl p-4 transition hover:ring-1 hover:ring-[var(--accent)]"
              >
                <p className="text-sm text-[var(--muted)]">{s.label}</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums text-[var(--foreground)]">
                  {s.value.toLocaleString("pt-BR")}
                </p>
              </Link>
            ))}
          </div>
        </section>

        <section className="glass-card-strong mb-8 rounded-2xl p-4">
          <h2 className="mb-3 text-lg font-semibold text-[var(--foreground)]">Volume por estado</h2>
          <BrazilStateMap states={insights?.by_state ?? []} />
        </section>

        <div className="mb-8 grid grid-cols-1 gap-6 lg:grid-cols-2">
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
                          {item.count.toLocaleString("pt-BR")} arquivos
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
                          {item.count.toLocaleString("pt-BR")} arquivos
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
          <section aria-labelledby="falhas-heading">
            <h2 id="falhas-heading" className="mb-3 text-lg font-semibold text-[var(--foreground)]">
              Falhas recentes
            </h2>
            {!initialized ? (
              <div className="space-y-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div key={i} className="h-14 animate-pulse rounded-2xl bg-[var(--border)]" />
                ))}
              </div>
            ) : failed.length === 0 ? (
              <p className="glass-card rounded-2xl p-4 text-sm text-[var(--muted)]">Nenhuma falha recente</p>
            ) : (
              <ul className="space-y-2">
                {failed.map((f, idx) => (
                  <li
                    key={`${f.id}-${f.remote_timestamp ?? f.last_seen_at}-${idx}`}
                    className="glass-card flex items-center justify-between rounded-2xl p-3"
                  >
                    <div className="min-w-0">
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

          <section aria-labelledby="atividade-heading">
            <div className="mb-3 flex items-center justify-between gap-2">
              <h2 id="atividade-heading" className="text-lg font-semibold text-[var(--foreground)]">
                Atividade recente
              </h2>
              <Link href="/files" className="secondary-link-chip shrink-0">
                Todos os arquivos
              </Link>
            </div>
            {!initialized ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <div key={i} className="h-14 animate-pulse rounded-2xl bg-[var(--border)]" />
                ))}
              </div>
            ) : (
              <ul className="space-y-2">
                {recent.map((f, idx) => (
                  <li
                    key={`${f.id}-${f.remote_timestamp ?? f.last_seen_at}-${idx}`}
                    className="glass-card flex items-center justify-between rounded-2xl p-3"
                  >
                    <div className="min-w-0">
                      <Link href={`/files/${f.id}`} className="text-sm font-medium text-[var(--accent)] hover:underline">
                        {f.filename}
                      </Link>
                      <p className="text-xs text-[var(--muted)]">{formatDateTimeBR(f.remote_timestamp ?? f.last_seen_at)}</p>
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
