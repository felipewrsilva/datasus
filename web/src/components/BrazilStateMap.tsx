"use client";

import { useEffect, useMemo, useState, useRef, useCallback } from "react";
import brazilMap from "@svg-maps/brazil";
import { stateNamePtBR } from "@/lib/stateLabels";
import { formatBytes, formatSizeAndFileCount } from "@/lib/formatBytes";
import { filesPath } from "@/lib/dashboardFileDrill";
import {
  mapFillPalette,
  sizeToSextileLevel,
  sortedSizesFromBuckets,
  sextileBinBoundaries,
} from "@/lib/dashboardSizeScale";

type StateSize = {
  key: string;
  count: number;
  total_size_bytes: number;
  avg_size_bytes: number;
};

type Props = {
  states: StateSize[];
};

const TOOLTIP_OFFSET_X = 8;
const TOOLTIP_OFFSET_Y = 6;

export function BrazilStateMap({ states }: Props) {
  const [hovered, setHovered] = useState<string | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [isDark, setIsDark] = useState(false);
  const tooltipRef = useRef<HTMLDivElement | null>(null);
  const pointerPosRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });
  const hasPointerRef = useRef(false);
  const activeTooltipRef = useRef(false);
  const frameRef = useRef<number | null>(null);

  useEffect(() => {
    const root = document.documentElement;
    const updateTheme = () => setIsDark(root.classList.contains("dark"));
    updateTheme();
    const observer = new MutationObserver(updateTheme);
    observer.observe(root, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, []);

  const byState = useMemo(() => {
    const map = new Map<string, StateSize>();
    for (const entry of states) {
      map.set(entry.key.toUpperCase(), {
        ...entry,
        total_size_bytes: Math.max(0, entry.total_size_bytes ?? 0),
      });
    }
    return map;
  }, [states]);

  const locationUFs = useMemo(
    () => new Set<string>(brazilMap.locations.map((location: { id: string }) => location.id.toUpperCase())),
    [],
  );

  const mapStatesForScale = useMemo(() => {
    const list: StateSize[] = [];
    for (const uf of locationUFs) {
      const row = byState.get(uf);
      if (row) list.push(row);
    }
    return list;
  }, [byState, locationUFs]);

  const sizeValues = useMemo(
    () => sortedSizesFromBuckets(mapStatesForScale),
    [mapStatesForScale],
  );

  const maxSize = Math.max(...sizeValues, 1);
  const binRanges = useMemo(() => sextileBinBoundaries(sizeValues), [sizeValues]);
  const palette = mapFillPalette(isDark);

  const activeUF = hovered ?? focused;
  const levelForSize = (totalSize: number) => sizeToSextileLevel(sizeValues, totalSize);

  const stateStroke = isDark ? "rgba(226, 232, 240, 0.34)" : "rgba(15, 23, 42, 0.62)";
  const activeFill = isDark ? "#bae6fd" : "#172554";
  const activeStroke = isDark ? "rgba(248, 250, 252, 0.98)" : "rgba(15, 23, 42, 0.98)";
  const activeShadow = isDark ? "drop-shadow(0 0 10px rgba(56, 189, 248, 0.48))" : "drop-shadow(0 0 10px rgba(8, 81, 156, 0.34))";

  const formatBytesRounded = (value: number) => {
    if (!Number.isFinite(value) || value <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let size = Math.round(value);
    let index = 0;
    while (size >= 1024 && index < units.length - 1) {
      size /= 1024;
      index += 1;
    }
    return `${Math.round(size)} ${units[index]}`;
  };

  const formatRange = (lo: number, hi: number) => {
    if (lo === hi) return formatBytesRounded(lo);
    return `${formatBytesRounded(lo)} a ${formatBytesRounded(hi)}`;
  };

  const activeStateData = activeUF ? byState.get(activeUF) : null;
  const activeTotalSize = activeStateData?.total_size_bytes ?? 0;
  const activeCount = activeStateData?.count ?? 0;
  const activeSizeLabel = Number.isFinite(activeTotalSize) && activeTotalSize > 0 ? formatBytes(activeTotalSize) : "0 GB";
  const normalizedCount = Number.isFinite(activeCount) && activeCount > 0 ? Math.trunc(activeCount) : 0;
  const activeCountLabel = `(${normalizedCount.toLocaleString("pt-BR")} arquivos)`;

  const applyTooltipPosition = useCallback((tooltipEl: HTMLDivElement) => {
    const x = pointerPosRef.current.x + TOOLTIP_OFFSET_X;
    const y = pointerPosRef.current.y - TOOLTIP_OFFSET_Y;
    tooltipEl.style.transform = `translate3d(${x}px, ${y}px, 0) translateY(-100%)`;
  }, []);

  const renderTooltipFrame = useCallback(() => {
    frameRef.current = null;
    const tooltipEl = tooltipRef.current;
    if (!tooltipEl || !activeTooltipRef.current || !hasPointerRef.current) return;
    applyTooltipPosition(tooltipEl);
  }, [applyTooltipPosition]);

  const scheduleTooltipFrame = useCallback(() => {
    if (frameRef.current !== null) return;
    frameRef.current = window.requestAnimationFrame(renderTooltipFrame);
  }, [renderTooltipFrame]);

  useEffect(() => {
    activeTooltipRef.current = Boolean(activeUF);
    if (!activeTooltipRef.current) return;
    if (!hasPointerRef.current) return;
    const tooltipEl = tooltipRef.current;
    if (tooltipEl) applyTooltipPosition(tooltipEl);
    scheduleTooltipFrame();
  }, [activeUF, applyTooltipPosition, scheduleTooltipFrame]);

  useEffect(() => {
    if (!activeUF) return;
    const handlePointerMove = (event: PointerEvent) => {
      pointerPosRef.current = { x: event.clientX, y: event.clientY };
      hasPointerRef.current = true;
      scheduleTooltipFrame();
    };
    window.addEventListener("pointermove", handlePointerMove, { passive: true });
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
    };
  }, [activeUF, scheduleTooltipFrame]);

  useEffect(() => () => {
    if (frameRef.current !== null) {
      window.cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    }
  }, []);

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-[2.1fr_1fr]">
      <div className="rounded-2xl border border-sky-200/90 bg-sky-100/70 p-3 dark:border-[var(--border)] dark:bg-[var(--accent-soft)]/15">
        <svg
          viewBox={brazilMap.viewBox}
          className="h-auto w-full"
          role="img"
          aria-label="Mapa do Brasil por tamanho total de arquivos por estado"
          onPointerLeave={() => {
            setHovered(null);
          }}
        >
          {brazilMap.locations.map((location: { id: string; path: string }) => {
            const uf = location.id.toUpperCase();
            const stateData = byState.get(uf);
            const totalSize = stateData?.total_size_bytes ?? 0;
            const count = stateData?.count ?? 0;
            const level = levelForSize(totalSize);
            const isActive = activeUF === uf;
            const label = `${stateNamePtBR(uf)}: ${formatSizeAndFileCount(totalSize, count)}`;

            return (
              <a key={location.id} href={filesPath({ state: uf })}>
                <path
                  d={location.path}
                  tabIndex={0}
                  aria-label={label}
                  fill={isActive ? activeFill : palette[level]}
                  stroke={isActive ? activeStroke : stateStroke}
                  strokeWidth={isActive ? 2.2 : 1}
                  style={{
                    transition: "fill 180ms ease, stroke 180ms ease, stroke-width 180ms ease",
                    cursor: "pointer",
                    filter: isActive ? activeShadow : undefined,
                  }}
                  onPointerEnter={(event) => {
                    pointerPosRef.current = { x: event.clientX, y: event.clientY };
                    hasPointerRef.current = true;
                    setHovered(uf);
                    const tooltipEl = tooltipRef.current;
                    if (tooltipEl) applyTooltipPosition(tooltipEl);
                    scheduleTooltipFrame();
                  }}
                  onFocus={() => {
                    setFocused(uf);
                    scheduleTooltipFrame();
                  }}
                  onBlur={() => setFocused(null)}
                />
              </a>
            );
          })}
        </svg>
        <div
          ref={tooltipRef}
          className="pointer-events-none fixed left-0 top-0 z-50 max-w-[220px] rounded-lg border border-[var(--border)] bg-[var(--card-strong)] px-3 py-2 shadow-xl backdrop-blur-sm transition-opacity duration-75"
          style={{
            willChange: "transform",
            transform: "translate3d(0, 0, 0)",
            opacity: activeUF ? 1 : 0,
            visibility: activeUF ? "visible" : "hidden",
          }}
          role="status"
          aria-live="polite"
          aria-hidden={!activeUF}
        >
          <p className="mb-0.5 text-xs text-[var(--muted)]">{activeUF ? stateNamePtBR(activeUF) : ""}</p>
          <p className="text-sm font-semibold tabular-nums leading-tight text-[var(--foreground)]">
            {activeUF ? activeSizeLabel : ""}
          </p>
          <p className="mt-0.5 text-xs tabular-nums text-[var(--muted)]">{activeUF ? activeCountLabel : ""}</p>
        </div>
      </div>

      <div className="space-y-3">
        <div className="glass-card rounded-2xl p-5">
          <p className="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--muted)]">Tamanho total por estado</p>
          <div className="space-y-2">
            {binRanges.length === 0 ? (
              <p className="text-xs text-[var(--muted)]">Sem dados de tamanho por estado para montar a escala.</p>
            ) : (
              binRanges.map(([lo, hi], idx) => (
                <div
                  key={idx}
                  className="grid grid-cols-[12px_minmax(0,1fr)] items-center gap-x-2 text-xs"
                >
                  <span
                    className="inline-block h-3 w-3 rounded-sm border border-[var(--border)]"
                    style={{ backgroundColor: palette[idx] }}
                  />
                  <span className="min-w-0 tabular-nums text-[var(--foreground)]">{formatRange(lo, hi)}</span>
                </div>
              ))
            )}
          </div>
          <div className="mt-4 flex items-center justify-between gap-2 text-xs text-[var(--muted)]">
            <span className="tabular-nums">
              Mínimo: <span className="tabular-nums text-[var(--foreground)]">{formatBytesRounded(sizeValues[0] ?? 0)}</span>
            </span>
            <span className="text-right tabular-nums">
              Máximo: <span className="tabular-nums text-[var(--foreground)]">{formatBytesRounded(maxSize)}</span>
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
