import type { Stage } from "@/lib/types";
import { StageStatusBadge } from "./StageStatusBadge";
import { formatDateTimeBR } from "@/lib/dateFormat";

const STAGE_LABELS: Record<string, string> = {
  download: "Download",
  csv_conversion: "Conversão para CSV",
  parquet_conversion: "Conversão para Parquet",
};

export function StageCard({ stage }: { stage: Stage }) {
  return (
    <div className="glass-card rounded-2xl p-4">
      <div className="flex items-center justify-between mb-2">
        <h3 className="font-medium text-[var(--foreground)]">{STAGE_LABELS[stage.stage] ?? stage.stage}</h3>
        <StageStatusBadge status={stage.status} />
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm text-[var(--muted)]">
        <dt>Tentativas</dt>
        <dd className="font-mono">{stage.attempts}</dd>
        {stage.started_at && (
          <>
            <dt>Início</dt>
            <dd>{formatDateTimeBR(stage.started_at)}</dd>
          </>
        )}
        {stage.finished_at && (
          <>
            <dt>Fim</dt>
            <dd>{formatDateTimeBR(stage.finished_at)}</dd>
          </>
        )}
        {stage.error_message && (
          <>
            <dt className="text-[var(--status-danger-fg)]">Erro</dt>
            <dd className="col-span-2 font-mono text-xs break-all text-[var(--status-danger-fg)]">{stage.error_message}</dd>
          </>
        )}
      </dl>
    </div>
  );
}
