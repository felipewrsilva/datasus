import type { StageStatus } from "@/lib/types";

const OVERALL_LABELS: Record<string, string> = {
  pending: "Pendente",
  ignored: "Ignorado",
  downloading: "Baixando",
  downloaded: "Baixado",
  converting_csv: "Convertendo para CSV",
  csv_ready: "CSV pronto",
  converting_parquet: "Convertendo para Parquet",
  parquet_ready: "Parquet pronto",
  failed: "Falhou",
  purged: "Removido",
  unknown: "Desconhecido",
};

const STAGE_LABELS: Record<StageStatus, string> = {
  pending: "Pendente",
  running: "Em execução",
  done: "Concluído",
  failed: "Falhou",
  purged: "Removido",
};

export function overallStatusLabel(status: string | null | undefined): string {
  const key = typeof status === "string" && status.length > 0 ? status : "unknown";
  return OVERALL_LABELS[key] ?? key.replace(/_/g, " ");
}

export function stageStatusLabel(status: StageStatus): string {
  return STAGE_LABELS[status] ?? status;
}
