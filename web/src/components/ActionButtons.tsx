"use client";

import { useState, useCallback, useEffect } from "react";
import {
  getScanStatus,
  getPolicies,
  isPolicySelectionComplete,
  reprocessRecentFailures,
  triggerScan,
} from "@/lib/api";
import type { StageName } from "@/lib/types";
import { Button } from "@/components/ui/button";

interface Toast {
  id: number;
  text: string;
  isError: boolean;
}

interface GlobalActionsProps {
  onAction?: () => void;
  failedStage: StageName;
}

let toastSeq = 0;

const ACTION_REPROCESS_24H = "Reprocessar falha";

export function GlobalActions({ onAction, failedStage }: GlobalActionsProps) {
  const [loading, setLoading] = useState<string | null>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [policyComplete, setPolicyComplete] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    void getPolicies()
      .then((p) => {
        if (!cancelled) setPolicyComplete(isPolicySelectionComplete(p));
      })
      .catch(() => {
        if (!cancelled) setPolicyComplete(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const addToast = useCallback((text: string, isError: boolean) => {
    const id = ++toastSeq;
    setToasts((prev) => [...prev, { id, text, isError }]);
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 4000);
  }, []);

  const actionsDisabled = policyComplete !== true;
  const stageLabelMap: Record<StageName, string> = {
    download: "download",
    csv_conversion: "CSV",
    parquet_conversion: "Parquet",
  };
  const stageLabel = stageLabelMap[failedStage];

  return (
    <div className="space-y-3">
      {actionsDisabled && (
        <p className="text-xs text-[var(--status-warning-fg)]">
          Reprocessamento exige pelo menos um catálogo e um período em Políticas.
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <ActionBtn
          label={`${ACTION_REPROCESS_24H} (${stageLabel})`}
          loading={loading === ACTION_REPROCESS_24H}
          onClick={() => {
            if (policyComplete !== true) {
              addToast("Defina catálogo e período em Políticas antes de reprocessar.", true);
              return;
            }
            setLoading(ACTION_REPROCESS_24H);
            void reprocessRecentFailures({ hours: 24, stage: failedStage })
              .then((r) => addToast(`Reprocessamento (${stageLabel}): ${r.enqueued}/${r.matched} enfileirados`, false))
              .catch((e) => addToast(`Falha no reprocessamento: ${(e as Error).message}`, true))
              .finally(() => {
                setLoading(null);
                onAction?.();
              });
          }}
          color="danger"
          disabled={actionsDisabled}
        />
      </div>
      {toasts.length > 0 && (
        <div className="space-y-1">
          {toasts.map((t) => (
            <p key={t.id} className={`text-sm ${t.isError ? "text-[var(--status-danger-fg)]" : "text-[var(--status-success-fg)]"}`}>
              {t.text}
            </p>
          ))}
        </div>
      )}
      <span className="w-full text-xs text-[var(--muted)]">Reprocessa as falhas da etapa nos últimos 24h.</span>
    </div>
  );
}

export function ScanButton({ onScan }: { onScan?: () => void }) {
  const [loading, setLoading] = useState(false);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [running, setRunning] = useState(false);

  const addToast = useCallback((text: string, isError: boolean) => {
    const id = ++toastSeq;
    setToasts((prev) => [...prev, { id, text, isError }]);
    setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 4000);
  }, []);

  useEffect(() => {
    let cancelled = false;
    void getScanStatus()
      .then((status) => {
        if (!cancelled) setRunning(status.running);
      })
      .catch(() => {
        if (!cancelled) setRunning(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleScan = async () => {
    setLoading(true);
    try {
      const response = await triggerScan();
      setRunning(response.status.running);
      if (!response.accepted) {
        addToast("Já existe uma varredura em andamento.", true);
        return;
      }
      addToast("Varredura disparada em background.", false);
      onScan?.();
    } catch (e) {
      const msg = (e as Error).message.toLowerCase();
      if (msg.includes("already running")) {
        setRunning(true);
        addToast("Já existe uma varredura em andamento.", true);
      } else {
        addToast(`Falha na varredura: ${(e as Error).message}`, true);
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <Button
        onClick={handleScan}
        disabled={loading || running}
        size="sm"
      >
        {loading ? "Disparando..." : running ? "Varredura em andamento" : "Varrer FTP"}
      </Button>
      {toasts.map((t) => (
        <p key={t.id} className={`text-xs ${t.isError ? "text-[var(--status-danger-fg)]" : "text-[var(--status-success-fg)]"}`}>
          {t.text}
        </p>
      ))}
    </div>
  );
}

function ActionBtn({
  label,
  loading,
  onClick,
  color,
  disabled = false,
}: {
  label: string;
  loading: boolean;
  onClick: () => void;
  color: string;
  disabled?: boolean;
}) {
  const variants: Record<string, "default" | "secondary" | "danger"> = {
    blue: "default",
    indigo: "secondary",
    purple: "secondary",
    red: "danger",
    danger: "danger",
  };

  return (
    <Button
      onClick={onClick}
      disabled={loading || disabled}
      variant={variants[color] ?? "secondary"}
      size="sm"
    >
      {loading ? "…" : label}
    </Button>
  );
}
