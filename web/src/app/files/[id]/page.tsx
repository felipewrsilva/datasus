"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { getFile, getFileStages } from "@/lib/api";
import type { DatFile, Stage, LogEntry } from "@/lib/types";
import { StageCard } from "@/components/StageCard";
import { OverallStatusBadge } from "@/components/StageStatusBadge";
import { GlobalActions } from "@/components/ActionButtons";
import { stateNamePtBR } from "@/lib/stateLabels";
import { formatCatalogLabel } from "@/lib/catalogLabels";
import { formatDateTimeBR, formatDateTimeSecondsBR } from "@/lib/dateFormat";

const POLL_MS = 5_000;

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

export default function FileDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [file, setFile] = useState<DatFile | null>(null);
  const [stages, setStages] = useState<Stage[]>([]);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [fetching, setFetching] = useState(false);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const ago = useSecondsAgo(lastUpdated);
  const failedStage = stages.find((s) => s.status === "failed")?.stage ?? null;
  const hasFailure = file?.overall_status === "failed" || failedStage !== null;
  const reprocessStage = failedStage ?? "download";
  const latestStageError = stages.find((s) => s.status === "failed" && s.error_message)?.error_message ?? null;
  const latestFailedLog = logs.find((l) => l.event_type === "failed")?.message ?? null;
  const failureReason = latestStageError || latestFailedLog || "Sem motivo explícito registrado.";

  const load = useCallback(async () => {
    if (!id || !/^[0-9a-fA-F-]{36}$/.test(id)) {
      setError("ID de arquivo inválido.");
      setFetching(false);
      return;
    }
    setFetching(true);
    setError(null);
    try {
      const [f, s] = await Promise.all([getFile(id), getFileStages(id)]);
      setFile(f);
      setStages(s.stages ?? []);
      setLogs(s.logs ?? []);
      setLastUpdated(new Date());
    } catch (err) {
      const message = err instanceof Error ? err.message : "Falha ao carregar detalhes do arquivo.";
      setError(message);
    } finally {
      setFetching(false);
    }
  }, [id]);

  useEffect(() => {
    const boot = setTimeout(() => { void load(); }, 0);
    const t = setInterval(load, POLL_MS);
    return () => { clearTimeout(boot); clearInterval(t); };
  }, [load]);

  if (!file) {
    return (
      <main className="min-h-screen flex items-center justify-center">
        <div className="glass-card-strong max-w-lg rounded-2xl p-6 text-center">
          {error ? (
            <>
              <p className="mb-3 text-sm text-[var(--status-danger-fg)]">{error}</p>
              <button
                type="button"
                onClick={() => void load()}
                className="rounded-lg border border-[var(--border)] px-3 py-2 text-sm text-[var(--foreground)] hover:bg-white/5"
              >
                Tentar novamente
              </button>
            </>
          ) : (
            <p className="text-[var(--muted)]">Carregando...</p>
          )}
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen p-6">
      <div className="mx-auto max-w-4xl">
        {/* Header */}
        <div className="flex items-start justify-between gap-3 mb-6">
          <div className="flex items-center gap-3 min-w-0">
            <Link href="/files" className="secondary-link-chip shrink-0 text-xs">Voltar para arquivos</Link>
            <span className="text-[var(--muted)]">/</span>
            <h1 className="font-mono text-xl font-bold text-[var(--foreground)] truncate">{file.filename}</h1>
            <OverallStatusBadge status={file.overall_status} />
          </div>
          {ago && (
            <span className="shrink-0 flex items-center gap-1.5 text-xs text-[var(--muted)]">
              <span className={`h-1.5 w-1.5 rounded-full ${fetching ? "bg-blue-400 animate-pulse" : "bg-emerald-500"}`} />
              Atualizado {ago}
            </span>
          )}
        </div>

        {error && (
          <div className="mb-6 rounded-xl border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-[var(--status-danger-fg)]">
            {error}
          </div>
        )}

        {hasFailure && (
          <div className="glass-card-strong mb-6 rounded-2xl p-4">
            <h2 className="mb-3 text-sm font-medium text-[var(--foreground)]">Ações gerais</h2>
            <GlobalActions failedStage={reprocessStage} onAction={load} />
          </div>
        )}

        {/* Metadata */}
        <div className="glass-card-strong mb-6 rounded-2xl p-4">
          <h2 className="mb-3 text-sm font-medium text-[var(--foreground)]">Metadados</h2>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <dt className="text-[var(--muted)]">Catálogo</dt><dd>{formatCatalogLabel(file.catalog)}</dd>
            <dt className="text-[var(--muted)]">Estado</dt><dd>{stateNamePtBR(file.state)}</dd>
            <dt className="text-[var(--muted)]">Ano / Mês</dt>
            <dd>{file.year} / {String(file.month).padStart(2, "0")}</dd>
            <dt className="text-[var(--muted)]">Parte</dt>
            <dd className="font-mono">{file.segment ?? "—"}</dd>
            <dt className="text-[var(--muted)]">Caminho</dt>
            <dd className="font-mono text-xs break-all">{file.ftp_path}</dd>
            <dt className="text-[var(--muted)]">Tamanho</dt>
            <dd>{file.size_bytes != null ? `${(file.size_bytes / 1024 / 1024).toFixed(1)} MB` : "—"}</dd>
            <dt className="text-[var(--muted)]">Hash local</dt>
            <dd className="font-mono text-xs">{file.local_hash?.slice(0, 16) ?? "—"}</dd>
            <dt className="text-[var(--muted)]">Última modificação no FTP</dt>
            <dd>{formatDateTimeBR(file.remote_timestamp)}</dd>
          </dl>
        </div>

        <div className="glass-card-strong mb-6 rounded-2xl p-4">
          <h2 className="mb-3 text-sm font-medium text-[var(--foreground)]">Motivo da falha</h2>
          <p className={`text-sm break-all ${file.overall_status === "failed" ? "text-[var(--status-danger-fg)]" : "text-[var(--muted)]"}`}>
            {failureReason}
          </p>
        </div>

        {/* Artifact paths */}
        <div className="glass-card-strong mb-6 rounded-2xl p-4">
          <h2 className="mb-3 text-sm font-medium text-[var(--foreground)]">Caminhos dos artefatos</h2>
          <dl className="grid grid-cols-1 gap-y-2 text-sm">
            {[
              { label: ".dbc", path: file.dbc_path },
              { label: ".csv", path: file.csv_path },
              { label: ".parquet", path: file.parquet_path },
            ].map(({ label, path }) => (
              <div key={label} className="flex items-start gap-3">
                <dt className="w-16 shrink-0 pt-0.5 font-mono text-xs text-[var(--muted)]">{label}</dt>
                <dd className={`font-mono text-xs break-all ${path ? "text-[var(--foreground)]" : "text-[var(--muted)]"}`}>
                  {path ?? "ainda não gerado"}
                </dd>
              </div>
            ))}
          </dl>
        </div>

        {/* Stage cards */}
        <div className="mb-6">
          <h2 className="mb-3 text-sm font-medium text-[var(--foreground)]">Etapas do pipeline</h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            {stages.map((s) => <StageCard key={s.id} stage={s} />)}
          </div>
        </div>

        {/* Logs */}
        <div>
          <h2 className="mb-3 text-sm font-medium text-[var(--foreground)]">Logs de processamento</h2>
          <div className="max-h-96 space-y-1 overflow-y-auto rounded-2xl border border-[var(--border)] bg-slate-950/90 p-4 font-mono text-xs text-slate-300">
            {logs.length === 0 ? (
              <p className="text-slate-500">Sem logs ainda</p>
            ) : (
              logs.map((l) => (
                <div key={l.id} className="flex gap-3">
                  {l.created_at && (
                    <span className="shrink-0 text-slate-600">{formatDateTimeSecondsBR(l.created_at)}</span>
                  )}
                  <span className="shrink-0 text-slate-500">[{l.stage.slice(0, 3)}]</span>
                  <span className={l.event_type === "failed" ? "text-rose-300" : l.event_type === "completed" ? "text-emerald-300" : "text-slate-300"}>
                    {l.event_type}: {l.message}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </main>
  );
}
