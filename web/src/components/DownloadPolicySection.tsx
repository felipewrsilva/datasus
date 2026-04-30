"use client";

import { useEffect, useMemo, useState } from "react";
import { getPolicies, putPolicies } from "@/lib/api";
import type { YearMonth } from "@/lib/types";
import { formatCatalogLabel } from "@/lib/catalogLabels";
import { POLICY_STATES } from "@/lib/stateLabels";
import { Button } from "@/components/ui/button";

const MONTHS = [
  "Janeiro",
  "Fevereiro",
  "Marco",
  "Abril",
  "Maio",
  "Junho",
  "Julho",
  "Agosto",
  "Setembro",
  "Outubro",
  "Novembro",
  "Dezembro",
] as const;

function keyFor(year: number, month: number): string {
  return `${year}-${String(month).padStart(2, "0")}`;
}

function normalizePolicyPathInput(raw: string): string {
  const value = raw.trim();
  if (!value) return "";
  const looksWindows =
    /^[a-zA-Z]:/.test(value) || value.startsWith("//") || value.startsWith("\\\\") || value.includes("\\");
  if (!looksWindows) return value;
  let normalized = value.replaceAll("/", "\\");
  if (normalized.startsWith("\\\\")) {
    const rest = normalized.replace(/^\\+/, "");
    normalized = `\\\\${rest}`;
  }
  normalized = normalized.replace(/\\{2,}/g, "\\");
  if (value.startsWith("//") || value.startsWith("\\\\")) {
    normalized = `\\\\${normalized.replace(/^\\+/, "")}`;
  }
  const isDriveRoot = /^[a-zA-Z]:\\$/.test(normalized);
  if (!isDriveRoot) normalized = normalized.replace(/\\+$/g, "");
  return normalized;
}

function validatePolicyPath(value: string): string | null {
  if (!value) return null;
  const windowsLike = /^[a-zA-Z]:/.test(value) || value.startsWith("\\\\");
  if (windowsLike) {
    if (value.includes("/")) return "Use barras invertidas para caminhos Windows.";
    if (/^[a-zA-Z]:[^\\]/.test(value)) return "Formato inválido. Use C:\\pasta.";
    if (value.startsWith("\\\\")) {
      const parts = value.replace(/^\\\\/, "").split("\\");
      if (parts.length < 2 || !parts[0] || !parts[1]) return "Caminho de rede inválido.";
    } else if (!/^[a-zA-Z]:\\/.test(value)) {
      return "Caminho local inválido.";
    }
    if (/[*?"<>|]/.test(value)) return "Caminho contém caracteres inválidos.";
    return null;
  }
  if (!value.startsWith("/")) return "Formato inválido.";
  return null;
}

export function DownloadPolicySection({ onSaved }: { onSaved?: () => void }) {
  const [enableDownload, setEnableDownload] = useState(true);
  const [enableCSV, setEnableCSV] = useState(true);
  const [enableParquet, setEnableParquet] = useState(true);
  const [availablePeriodYears, setAvailablePeriodYears] = useState<number[]>([]);
  const [availablePeriodMonths, setAvailablePeriodMonths] = useState<YearMonth[]>([]);
  const [availableCatalogs, setAvailableCatalogs] = useState<string[]>([]);
  const [availableStates, setAvailableStates] = useState<string[]>([]);
  const [selectedCatalogs, setSelectedCatalogs] = useState<Set<string>>(new Set());
  const [selectedStates, setSelectedStates] = useState<Set<string>>(new Set());
  const [selectedYears, setSelectedYears] = useState<Set<number>>(new Set());
  const [selectedMonths, setSelectedMonths] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");
  const [stateQuery, setStateQuery] = useState("");
  const [expandedYears, setExpandedYears] = useState<Record<number, boolean>>({});
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [downloadDir, setDownloadDir] = useState("");
  const [csvDir, setCSVDir] = useState("");
  const [parquetDir, setParquetDir] = useState("");
  const [pathErrors, setPathErrors] = useState<{ download?: string; csv?: string; parquet?: string }>({});
  const [savedSnapshot, setSavedSnapshot] = useState("");

  const availableMonthSet = useMemo(
    () => new Set(availablePeriodMonths.map((item) => keyFor(item.year, item.month))),
    [availablePeriodMonths],
  );
  const years = useMemo(
    () => [...availablePeriodYears].sort((a, b) => b - a),
    [availablePeriodYears],
  );

  useEffect(() => {
    let cancelled = false;
    const buildSnapshot = (policy: Parameters<typeof putPolicies>[0]) => JSON.stringify(policy);
    const buildPayloadFromState = (p: Awaited<ReturnType<typeof getPolicies>>): Parameters<typeof putPolicies>[0] => ({
      selected_catalogs: [...(p.selected_catalogs ?? [])].map((c) => c.toUpperCase()).sort(),
      selected_states: [...(p.selected_states ?? [])].map((s) => s.toUpperCase()).sort(),
      selected_periods: {
        years: [...(p.selected_periods?.years ?? [])].map((y) => Number(y)).filter((y) => Number.isFinite(y)).sort((a, b) => a - b),
        months: [...(p.selected_periods?.months ?? [])]
          .map((m) => ({ year: Number(m.year), month: Number(m.month) }))
          .filter((m) => Number.isFinite(m.year) && Number.isFinite(m.month) && m.month >= 1 && m.month <= 12)
          .sort((a, b) => (a.year === b.year ? a.month - b.month : a.year - b.year)),
      },
      processing: {
        enable_download: p.processing?.enable_download ?? true,
        enable_csv: p.processing?.enable_csv ?? true,
        enable_parquet: p.processing?.enable_parquet ?? true,
      },
      directories: {
        download_dir: p.directories?.download_dir,
        csv_dir: p.directories?.csv_dir,
        parquet_dir: p.directories?.parquet_dir,
      },
    });
    void (async () => {
      try {
        setLoading(true);
        const p = await getPolicies();
        if (cancelled) return;
        setAvailablePeriodYears((p.available_periods?.years ?? []).map((y) => Number(y)).filter((y) => Number.isFinite(y)));
        const payloadAvailableMonths = (p.available_periods?.months ?? []).filter((ym) =>
            Number.isFinite(ym.year) && Number.isFinite(ym.month) && ym.month >= 1 && ym.month <= 12,
          );
        setAvailablePeriodMonths(payloadAvailableMonths);
        setAvailableCatalogs((p.available_catalogs ?? []).map((c) => c.toUpperCase()));
        setAvailableStates((p.available_states ?? []).map((s) => s.toUpperCase()));
        setSelectedCatalogs(new Set((p.selected_catalogs ?? []).map((c) => c.toUpperCase())));
        setSelectedStates(new Set((p.selected_states ?? []).map((s) => s.toUpperCase())));
        setSelectedYears(
          new Set((p.selected_periods?.years ?? []).map((y) => Number(y)).filter((y) => Number.isFinite(y))),
        );
        setEnableDownload(p.processing?.enable_download ?? true);
        setEnableCSV(p.processing?.enable_csv ?? true);
        setEnableParquet(p.processing?.enable_parquet ?? true);
        const monthKeys = new Set<string>();
        for (const ym of p.selected_periods?.months ?? []) {
          if (Number.isFinite(ym.year) && Number.isFinite(ym.month) && ym.month >= 1 && ym.month <= 12) {
            monthKeys.add(keyFor(ym.year, ym.month));
          }
        }
        setSelectedMonths(monthKeys);
        setDownloadDir(p.directories?.download_dir ?? "");
        setCSVDir(p.directories?.csv_dir ?? "");
        setParquetDir(p.directories?.parquet_dir ?? "");
        setSavedSnapshot(buildSnapshot(buildPayloadFromState(p)));
      } catch (e) {
        if (cancelled) return;
        setLoadErr(e instanceof Error ? e.message : "Falha ao carregar políticas");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const toggleCatalog = (catalog: string, checked: boolean) => {
    setSelectedCatalogs((prev) => {
      const next = new Set(prev);
      if (checked) next.add(catalog);
      else next.delete(catalog);
      return next;
    });
  };

  const toggleState = (state: string, checked: boolean) => {
    setSelectedStates((prev) => {
      const next = new Set(prev);
      if (checked) next.add(state);
      else next.delete(state);
      return next;
    });
  };

  const toggleYear = (year: number, checked: boolean) => {
    const nextYears = new Set(selectedYears);
    const nextMonths = new Set(selectedMonths);
    if (checked) {
      nextYears.add(year);
      for (let m = 1; m <= 12; m += 1) {
        nextMonths.delete(keyFor(year, m));
      }
    } else {
      nextYears.delete(year);
      for (let m = 1; m <= 12; m += 1) {
        nextMonths.delete(keyFor(year, m));
      }
    }
    setSelectedYears(nextYears);
    setSelectedMonths(nextMonths);
  };

  const toggleMonth = (year: number, month: number, checked: boolean) => {
    const key = keyFor(year, month);
    const yearWasSelected = selectedYears.has(year);
    if (yearWasSelected && !checked) {
      const nextYears = new Set(selectedYears);
      nextYears.delete(year);
      const nextMonths = new Set(selectedMonths);
      for (let m = 1; m <= 12; m += 1) {
        if (m !== month) nextMonths.add(keyFor(year, m));
      }
      setSelectedYears(nextYears);
      setSelectedMonths(nextMonths);
      return;
    }

    const nextMonths = new Set(selectedMonths);
    if (checked) nextMonths.add(key);
    else nextMonths.delete(key);

    if (!yearWasSelected && checked) {
      let allTwelve = true;
      for (let m = 1; m <= 12; m += 1) {
        if (!nextMonths.has(keyFor(year, m))) {
          allTwelve = false;
          break;
        }
      }
      if (allTwelve) {
        const nextYears = new Set(selectedYears);
        nextYears.add(year);
        for (let m = 1; m <= 12; m += 1) {
          nextMonths.delete(keyFor(year, m));
        }
        setSelectedYears(nextYears);
        setSelectedMonths(nextMonths);
        return;
      }
    }

    setSelectedMonths(nextMonths);
  };

  const toggleYearExpanded = (year: number) => {
    setExpandedYears((prev) => ({
      ...prev,
      [year]: !prev[year],
    }));
  };

  const toggleDownload = (checked: boolean) => {
    setEnableDownload(checked);
  };

  const toggleCSV = (checked: boolean) => {
    setEnableCSV(checked);
  };

  const toggleParquet = (checked: boolean) => {
    setEnableParquet(checked);
  };

  const save = async () => {
    setSaving(true);
    setLoadErr(null);
    try {
      const selectedCatalogsList = Array.from(selectedCatalogs).sort();
      const selectedStatesList = Array.from(selectedStates).sort();
      const selectedYearsList = Array.from(selectedYears).sort((a, b) => a - b);
      const selectedMonthsList: YearMonth[] = Array.from(selectedMonths)
        .map((key) => {
          const [year, month] = key.split("-").map(Number);
          return { year, month };
        })
        .sort((a, b) => (a.year === b.year ? a.month - b.month : a.year - b.year));
      const updated = await putPolicies({
        selected_catalogs: selectedCatalogsList,
        selected_states: selectedStatesList,
        selected_periods: {
          years: selectedYearsList,
          months: selectedMonthsList,
        },
        processing: {
          enable_download: enableDownload,
          enable_csv: enableCSV,
          enable_parquet: enableParquet,
        },
        directories: {
          download_dir: downloadDir.trim() || undefined,
          csv_dir: csvDir.trim() || undefined,
          parquet_dir: parquetDir.trim() || undefined,
        },
      });
      const updatedAvailableMonths = (updated.available_periods?.months ?? []).filter((ym) =>
        Number.isFinite(ym.year) && Number.isFinite(ym.month) && ym.month >= 1 && ym.month <= 12,
      );
      setAvailablePeriodYears((updated.available_periods?.years ?? []).map((y) => Number(y)).filter((y) => Number.isFinite(y)));
      setAvailablePeriodMonths(updatedAvailableMonths);
      setAvailableCatalogs((updated.available_catalogs ?? []).map((c) => c.toUpperCase()));
      setAvailableStates((updated.available_states ?? []).map((s) => s.toUpperCase()));
      setSelectedCatalogs(new Set((updated.selected_catalogs ?? []).map((c) => c.toUpperCase())));
      setSelectedStates(new Set((updated.selected_states ?? []).map((s) => s.toUpperCase())));
      setSelectedYears(
        new Set((updated.selected_periods?.years ?? []).map((y) => Number(y)).filter((y) => Number.isFinite(y))),
      );
      setSelectedMonths(
        new Set(
          (updated.selected_periods?.months ?? [])
            .filter((month) => Number.isFinite(month.year) && Number.isFinite(month.month) && month.month >= 1 && month.month <= 12)
            .map((month) => keyFor(month.year, month.month)),
        ),
      );
      setEnableDownload(updated.processing?.enable_download ?? true);
      setEnableCSV(updated.processing?.enable_csv ?? true);
      setEnableParquet(updated.processing?.enable_parquet ?? true);
      setDownloadDir(updated.directories?.download_dir ?? "");
      setCSVDir(updated.directories?.csv_dir ?? "");
      setParquetDir(updated.directories?.parquet_dir ?? "");
      setSavedSnapshot(
        JSON.stringify({
          selected_catalogs: (updated.selected_catalogs ?? []).map((c) => c.toUpperCase()).sort(),
          selected_states: (updated.selected_states ?? []).map((s) => s.toUpperCase()).sort(),
          selected_periods: {
            years: (updated.selected_periods?.years ?? []).map((y) => Number(y)).sort((a, b) => a - b),
            months: (updated.selected_periods?.months ?? [])
              .map((m) => ({ year: Number(m.year), month: Number(m.month) }))
              .sort((a, b) => (a.year === b.year ? a.month - b.month : a.year - b.year)),
          },
          processing: {
            enable_download: updated.processing?.enable_download ?? true,
            enable_csv: updated.processing?.enable_csv ?? true,
            enable_parquet: updated.processing?.enable_parquet ?? true,
          },
          directories: {
            download_dir: updated.directories?.download_dir,
            csv_dir: updated.directories?.csv_dir,
            parquet_dir: updated.directories?.parquet_dir,
          },
        }),
      );
      onSaved?.();
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : "Falha ao salvar");
    } finally {
      setSaving(false);
    }
  };

  const filteredCatalogs = availableCatalogs.filter((catalog) =>
    formatCatalogLabel(catalog).toLowerCase().includes(query.trim().toLowerCase()) ||
    catalog.toLowerCase().includes(query.trim().toLowerCase()),
  );
  const allowedStates = new Set(availableStates.length > 0 ? availableStates : POLICY_STATES.map((item) => item.uf));
  const filteredStates = POLICY_STATES.filter((item) => {
    if (!allowedStates.has(item.uf)) return false;
    const term = stateQuery.trim().toLowerCase();
    if (!term) return true;
    return item.uf.toLowerCase().includes(term) || item.name.toLowerCase().includes(term);
  });
  const hasSelectedPeriod = selectedYears.size > 0 || selectedMonths.size > 0;
  const currentPayloadSnapshot = JSON.stringify({
    selected_catalogs: Array.from(selectedCatalogs).sort(),
    selected_states: Array.from(selectedStates).sort(),
    selected_periods: {
      years: Array.from(selectedYears).sort((a, b) => a - b),
      months: Array.from(selectedMonths)
        .map((k) => {
          const [year, month] = k.split("-").map(Number);
          return { year, month };
        })
        .sort((a, b) => (a.year === b.year ? a.month - b.month : a.year - b.year)),
    },
    processing: {
      enable_download: enableDownload,
      enable_csv: enableCSV,
      enable_parquet: enableParquet,
    },
    directories: {
      download_dir: downloadDir.trim() || undefined,
      csv_dir: csvDir.trim() || undefined,
      parquet_dir: parquetDir.trim() || undefined,
    },
  });
  const hasPathErrors = Boolean(pathErrors.download || pathErrors.csv || pathErrors.parquet);
  const isDirty = savedSnapshot !== "" && currentPayloadSnapshot !== savedSnapshot;
  const canSave = !saving && !loading && isDirty && !hasPathErrors;

  const selectedCatalogLabels = Array.from(selectedCatalogs)
    .sort()
    .map((catalog) => formatCatalogLabel(catalog));
  const selectedStateLabels = Array.from(selectedStates)
    .sort()
    .map((state) => POLICY_STATES.find((item) => item.uf === state)?.label ?? state);
  const selectedYearsList = Array.from(selectedYears).sort((a, b) => a - b);
  const selectedMonthsList = Array.from(selectedMonths)
    .map((key) => {
      const [year, month] = key.split("-").map(Number);
      return { year, month };
    })
    .sort((a, b) => (a.year === b.year ? a.month - b.month : a.year - b.year));

  return (
    <section className="glass-card-strong rounded-2xl border border-[var(--border)]/70 p-6 mb-8">
      <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
        <div>
          <h2 className="text-lg font-semibold text-[var(--foreground)]">Configuração da política</h2>
          <p className="text-sm text-[var(--muted)] mt-1 max-w-3xl">
            Defina quais etapas do processamento ficam ativas e o escopo por catálogo, estado e período.
          </p>
        </div>
        <Button
          type="button"
          disabled={!canSave}
          onClick={() => void save()}
          size="sm"
        >
          {saving ? "Salvando..." : loading ? "Carregando..." : "Salvar configuração"}
        </Button>
      </div>
      {loadErr && (
        <p className="mb-3 rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-sm text-[var(--status-danger-fg)]">
          {loadErr}
        </p>
      )}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4 lg:col-span-2">
          <h3 className="text-base font-semibold text-[var(--foreground)]">1. Etapas de processamento</h3>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Escolha quais etapas o sistema pode executar. Cada etapa funciona de forma independente.
          </p>
          <div className="mt-3 grid grid-cols-1 gap-2 md:grid-cols-3">
            <label className="flex cursor-pointer items-center justify-between rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2 text-sm">
              <span className="text-[var(--foreground)]">Download de dados</span>
              <input
                type="checkbox"
                className="rounded"
                checked={enableDownload}
                onChange={(e) => toggleDownload(e.target.checked)}
              />
            </label>
            <label className="flex cursor-pointer items-center justify-between rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2 text-sm">
              <span className="text-[var(--foreground)]">Gerar CSV</span>
              <input
                type="checkbox"
                className="rounded"
                checked={enableCSV}
                onChange={(e) => toggleCSV(e.target.checked)}
              />
            </label>
            <label className="flex cursor-pointer items-center justify-between rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2 text-sm">
              <span className="text-[var(--foreground)]">Gerar Parquet</span>
              <input
                type="checkbox"
                className="rounded"
                checked={enableParquet}
                onChange={(e) => toggleParquet(e.target.checked)}
              />
            </label>
          </div>
        </section>
        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4 lg:col-span-2">
          <h3 className="text-base font-semibold text-[var(--foreground)]">2. Diretórios de armazenamento</h3>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Opcional. Se vazio, o sistema mantém os diretórios padrão atuais.
          </p>
          <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
            <label className="text-sm text-[var(--foreground)]">
              Pasta de Download
              <input
                type="text"
                value={downloadDir}
                onChange={(e) => {
                  setDownloadDir(e.target.value);
                  setPathErrors((prev) => ({ ...prev, download: undefined }));
                }}
                onBlur={() => {
                  const normalized = normalizePolicyPathInput(downloadDir);
                  setDownloadDir(normalized);
                  setPathErrors((prev) => ({ ...prev, download: validatePolicyPath(normalized) ?? undefined }));
                }}
                placeholder="Padrão atual do sistema"
                className="form-control mt-1 w-full rounded-xl px-3 py-2 text-sm"
              />
              {pathErrors.download && <span className="mt-1 block text-xs text-[var(--status-danger-fg)]">{pathErrors.download}</span>}
            </label>
            <label className="text-sm text-[var(--foreground)]">
              Pasta de CSV
              <input
                type="text"
                value={csvDir}
                onChange={(e) => {
                  setCSVDir(e.target.value);
                  setPathErrors((prev) => ({ ...prev, csv: undefined }));
                }}
                onBlur={() => {
                  const normalized = normalizePolicyPathInput(csvDir);
                  setCSVDir(normalized);
                  setPathErrors((prev) => ({ ...prev, csv: validatePolicyPath(normalized) ?? undefined }));
                }}
                placeholder="Padrão atual do sistema"
                className="form-control mt-1 w-full rounded-xl px-3 py-2 text-sm"
              />
              {pathErrors.csv && <span className="mt-1 block text-xs text-[var(--status-danger-fg)]">{pathErrors.csv}</span>}
            </label>
            <label className="text-sm text-[var(--foreground)]">
              Pasta de Parquet
              <input
                type="text"
                value={parquetDir}
                onChange={(e) => {
                  setParquetDir(e.target.value);
                  setPathErrors((prev) => ({ ...prev, parquet: undefined }));
                }}
                onBlur={() => {
                  const normalized = normalizePolicyPathInput(parquetDir);
                  setParquetDir(normalized);
                  setPathErrors((prev) => ({ ...prev, parquet: validatePolicyPath(normalized) ?? undefined }));
                }}
                placeholder="Padrão atual do sistema"
                className="form-control mt-1 w-full rounded-xl px-3 py-2 text-sm"
              />
              {pathErrors.parquet && <span className="mt-1 block text-xs text-[var(--status-danger-fg)]">{pathErrors.parquet}</span>}
            </label>
          </div>
        </section>
        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4">
          <h3 className="text-base font-semibold text-[var(--foreground)]">3. Catálogos</h3>
          <p className="mt-1 text-xs text-[var(--muted)]">Selecione ao menos um catálogo para o processamento poder rodar.</p>
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filtrar por código ou nome"
            className="form-control mt-3 mb-3 w-full rounded-xl px-3 py-2 text-sm"
          />
          <div className="max-h-[22rem] space-y-2 overflow-auto pr-1">
            {filteredCatalogs.length === 0 ? (
              <p className="text-sm text-[var(--muted)]">Nenhum catálogo encontrado.</p>
            ) : (
              filteredCatalogs.map((catalog) => (
                <label
                  key={catalog}
                  className="flex cursor-pointer items-center justify-between rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2 text-sm"
                >
                  <span className="text-[var(--foreground)]">{formatCatalogLabel(catalog)}</span>
                  <input
                    type="checkbox"
                    className="rounded"
                    checked={selectedCatalogs.has(catalog)}
                    onChange={(e) => toggleCatalog(catalog, e.target.checked)}
                  />
                </label>
              ))
            )}
          </div>
        </section>

        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4">
          <h3 className="text-base font-semibold text-[var(--foreground)]">4. Estados</h3>
          <p className="mt-1 text-xs text-[var(--muted)]">Selecione os estados permitidos. Sem seleção, nada é processado.</p>
          <div className="mt-3 mb-3 flex flex-wrap gap-2">
            <button
              type="button"
              className="secondary-link-chip text-xs"
              onClick={() => setSelectedStates(new Set(POLICY_STATES.map((state) => state.uf)))}
            >
              Selecionar todos
            </button>
            <button
              type="button"
              className="secondary-link-chip text-xs"
              onClick={() => setSelectedStates(new Set())}
            >
              Limpar
            </button>
          </div>
          <input
            type="search"
            value={stateQuery}
            onChange={(e) => setStateQuery(e.target.value)}
            placeholder="Filtrar por UF ou nome"
            className="form-control mt-3 mb-3 w-full rounded-xl px-3 py-2 text-sm"
          />
          <div className="max-h-[22rem] space-y-2 overflow-auto pr-1">
            {filteredStates.map((state) => (
              <label
                key={state.uf}
                className="flex cursor-pointer items-center justify-between rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2 text-sm"
              >
                <span className="text-[var(--foreground)]">{state.label}</span>
                <input
                  type="checkbox"
                  className="rounded"
                  checked={selectedStates.has(state.uf)}
                  onChange={(e) => toggleState(state.uf, e.target.checked)}
                />
              </label>
            ))}
          </div>
        </section>
        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4">
          <h3 className="text-base font-semibold text-[var(--foreground)]">5. Período</h3>
          <p className="mt-1 text-xs text-[var(--muted)]">
            Marque o ano para incluir os 12 meses calendário ou escolha meses avulsos. Meses ainda sem arquivo na base
            podem ser marcados para liberar o processamento quando o dado aparecer.
          </p>
          <div className="mt-3 space-y-2 max-h-[22rem] overflow-auto pr-1">
            {years.length === 0 && (
              <p className="text-sm text-[var(--muted)]">
                Nenhum período disponível no momento.
              </p>
            )}
            {years.map((year) => {
              const yearOn = selectedYears.has(year);
              const selectedMonthCount = yearOn
                ? 12
                : MONTHS.reduce((acc, _, idx) => (
                  selectedMonths.has(keyFor(year, idx + 1)) ? acc + 1 : acc
                ), 0);
              const yearPartial = !yearOn && selectedMonthCount > 0 && selectedMonthCount < 12;
              return (
                <div key={year} className="rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2">
                  <div className="flex items-center justify-between gap-2">
                    <label className="inline-flex items-center gap-2 text-sm text-[var(--foreground)]">
                      <input
                        type="checkbox"
                        className="rounded"
                        checked={yearOn}
                        ref={(el) => {
                          if (el) el.indeterminate = yearPartial;
                        }}
                        onChange={(e) => toggleYear(year, e.target.checked)}
                      />
                      <span>{year}</span>
                    </label>
                    <button
                      type="button"
                      className="secondary-link-chip text-xs"
                      onClick={() => toggleYearExpanded(year)}
                    >
                      {expandedYears[year] ? "Ocultar meses" : "Mostrar meses"}
                    </button>
                  </div>
                  {expandedYears[year] && (
                    <div className="mt-2 grid grid-cols-3 gap-2 sm:grid-cols-4">
                      {MONTHS.map((label, idx) => {
                        const month = idx + 1;
                        const monthKey = keyFor(year, month);
                        const hasFileInDb = availableMonthSet.has(monthKey);
                        const checked = yearOn || selectedMonths.has(monthKey);
                        return (
                          <label
                            key={`${year}-${month}`}
                            title={hasFileInDb ? undefined : "Sem arquivo catalogado neste mês ainda"}
                            className={`inline-flex items-center gap-1.5 rounded-lg border border-[var(--border)]/70 px-2 py-1 text-xs ${
                              hasFileInDb ? "text-[var(--foreground)]" : "text-[var(--muted)] opacity-80"
                            }`}
                          >
                            <input
                              type="checkbox"
                              className="rounded"
                              checked={checked}
                              onChange={(e) => toggleMonth(year, month, e.target.checked)}
                            />
                            <span>{label}</span>
                          </label>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </section>
        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4">
          <h3 className="text-base font-semibold text-[var(--foreground)]">6. Resumo</h3>
          <div className="mt-2 space-y-2 text-sm text-[var(--muted)]">
            <p>
              Etapas ativas: {enableDownload ? "download" : "download desativado"}
              {enableCSV ? ", csv" : ""}
              {enableParquet ? ", parquet" : ""}
            </p>
            <p>
              Modo atual:{" "}
              {selectedCatalogs.size === 0 || selectedStates.size === 0 || !hasSelectedPeriod
                ? "nenhum processamento: falta catálogo, estado ou período"
                : "somente catálogos, estados e períodos selecionados"}
            </p>
            <p>
              Catálogos selecionados: {selectedCatalogLabels.length > 0 ? selectedCatalogLabels.join(", ") : "nenhum"}
            </p>
            <p>
              Estados selecionados: {selectedStateLabels.length > 0 ? selectedStateLabels.join(", ") : "nenhum"}
            </p>
            <p>
              Anos selecionados: {selectedYearsList.length > 0 ? selectedYearsList.join(", ") : "nenhum"}
            </p>
            <p>
              Meses selecionados: {selectedMonthsList.length > 0
                ? selectedMonthsList.map((m) => `${MONTHS[m.month - 1]} de ${m.year}`).join(", ")
                : "nenhum"}
            </p>
            <p className="text-xs text-[var(--muted)]">
              Enquanto faltar catálogo, estado ou período, nenhum download nem conversão é enfileirado.
            </p>
          </div>
        </section>
      </div>
    </section>
  );
}
