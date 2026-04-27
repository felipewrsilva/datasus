import type { StageStatus } from "@/lib/types";
import { overallStatusLabel, stageStatusLabel } from "@/lib/statusLabels";
import { Badge } from "@/components/ui/badge";

type BadgeTone = "info" | "success" | "warning" | "danger" | "neutral";

const config: Record<StageStatus, { tone: BadgeTone; className?: string }> = {
  pending: { tone: "warning" },
  running: { tone: "info", className: "animate-pulse" },
  done: { tone: "success" },
  failed: { tone: "danger" },
  purged: { tone: "neutral", className: "line-through" },
};

export function StageStatusBadge({ status }: { status: StageStatus }) {
  const c = config[status] ?? config.pending;
  return <Badge tone={c.tone} className={c.className}>{stageStatusLabel(status)}</Badge>;
}

export function OverallStatusBadge({ status }: { status?: string | null }) {
  const normalized = typeof status === "string" && status.length > 0 ? status : "unknown";
  const tone: BadgeTone =
    normalized === "parquet_ready" ? "success" :
    normalized === "failed" ? "danger" :
    normalized === "purged" ? "neutral" :
    normalized.includes("converting") || normalized === "downloading" ? "info" :
    "warning";
  const animated = normalized.includes("converting") || normalized === "downloading";

  return <Badge tone={tone} className={animated ? "animate-pulse" : undefined}>{overallStatusLabel(normalized)}</Badge>;
}
