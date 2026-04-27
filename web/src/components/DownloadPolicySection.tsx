"use client";

import { useEffect, useMemo, useState } from "react";
import { getPolicies, putPolicies } from "@/lib/api";
import type { YearMonth } from "@/lib/types";
import { formatCatalogLabel } from "@/lib/catalogLabels";
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

export function DownloadPolicySection({ onSaved }: { onSaved?: () => void }) {
  const [enableDownload, setEnableDownload] = useState(true);
  const [enableCSV, setEnableCSV] = useState(true);
  const [enableParquet, setEnableParquet] = useState(true);
  const [availablePeriodYears, setAvailablePeriodYears] = useState<number[]>([]);
  const [availablePeriodMonths, setAvailablePeriodMonths] = useState<YearMonth[]>([]);
  const [availableCatalogs, setAvailableCatalogs] = useState<string[]>([]);
  const [selectedCatalogs, setSelectedCatalogs] = useState<Set<string>>(new Set());
  const [selectedYears, setSelectedYears] = useState<Set<number>>(new Set());
  const [selectedMonths, setSelectedMonths] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");
  const [expandedYears, setExpandedYears] = useState<Record<number, boolean>>({});
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

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
    void (async () => {
      try {
        const p = await getPolicies();
        if (cancelled) return;
        setAvailablePeriodYears((p.available_periods?.years ?? []).map((y) => Number(y)).filter((y) => Number.isFinite(y)));
        const payloadAvailableMonths = (p.available_periods?.months ?? []).filter((ym) =>
            Number.isFinite(ym.year) && Number.isFinite(ym.month) && ym.month >= 1 && ym.month <= 12,
          );
        setAvailablePeriodMonths(payloadAvailableMonths);
        setAvailableCatalogs((p.available_catalogs ?? []).map((c) => c.toUpperCase()));
        setSelectedCatalogs(new Set((p.selected_catalogs ?? []).map((c) => c.toUpperCase())));
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
      } catch (e) {
        if (cancelled) return;
        setLoadErr(e instanceof Error ? e.message : "Falha ao carregar políticas");
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
    if (!checked) {
      setEnableCSV(false);
      setEnableParquet(false);
    }
  };

  const toggleCSV = (checked: boolean) => {
    if (!enableDownload && checked) return;
    setEnableCSV(checked);
    if (!checked) {
      setEnableParquet(false);
    }
  };

  const toggleParquet = (checked: boolean) => {
    if ((!enableDownload || !enableCSV) && checked) return;
    setEnableParquet(checked);
  };

  const save = async () => {
    setSaving(true);
    setLoadErr(null);
    try {
      const selectedCatalogsList = Array.from(selectedCatalogs).sort();
      const selectedYearsList = Array.from(selectedYears).sort((a, b) => a - b);
      const selectedMonthsList: YearMonth[] = Array.from(selectedMonths)
        .map((key) => {
          const [year, month] = key.split("-").map(Number);
          return { year, month };
        })
        .sort((a, b) => (a.year === b.year ? a.month - b.month : a.year - b.year));
      const updated = await putPolicies({
        selected_catalogs: selectedCatalogsList,
        selected_periods: {
          years: selectedYearsList,
          months: selectedMonthsList,
        },
        processing: {
          enable_download: enableDownload,
          enable_csv: enableCSV,
          enable_parquet: enableParquet,
        },
      });
      const updatedAvailableMonths = (updated.available_periods?.months ?? []).filter((ym) =>
        Number.isFinite(ym.year) && Number.isFinite(ym.month) && ym.month >= 1 && ym.month <= 12,
      );
      setAvailablePeriodYears((updated.available_periods?.years ?? []).map((y) => Number(y)).filter((y) => Number.isFinite(y)));
      setAvailablePeriodMonths(updatedAvailableMonths);
      setAvailableCatalogs((updated.available_catalogs ?? []).map((c) => c.toUpperCase()));
      setSelectedCatalogs(new Set((updated.selected_catalogs ?? []).map((c) => c.toUpperCase())));
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
  const hasSelectedPeriod = selectedYears.size > 0 || selectedMonths.size > 0;
  const canSave = !saving;

  const selectedCatalogLabels = Array.from(selectedCatalogs)
    .sort()
    .map((catalog) => formatCatalogLabel(catalog));
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
            Defina quais etapas do processamento ficam ativas e o escopo por catálogo e período.
          </p>
        </div>
        <Button
          type="button"
          disabled={!canSave}
          onClick={() => void save()}
          size="sm"
        >
          {saving ? "Salvando..." : "Salvar configuração"}
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
            Escolha quais etapas o sistema pode executar. O encadeamento segue Download, CSV, Parquet.
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
                disabled={!enableDownload}
                onChange={(e) => toggleCSV(e.target.checked)}
              />
            </label>
            <label className="flex cursor-pointer items-center justify-between rounded-xl border border-[var(--border)]/70 bg-[var(--background)]/20 px-3 py-2 text-sm">
              <span className="text-[var(--foreground)]">Gerar Parquet</span>
              <input
                type="checkbox"
                className="rounded"
                checked={enableParquet}
                disabled={!enableDownload || !enableCSV}
                onChange={(e) => toggleParquet(e.target.checked)}
              />
            </label>
          </div>
        </section>
        <section className="rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4">
          <h3 className="text-base font-semibold text-[var(--foreground)]">2. Catálogos</h3>
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
          <h3 className="text-base font-semibold text-[var(--foreground)]">3. Período</h3>
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
      </div>

      <section className="mt-5 rounded-2xl border border-[var(--border)] bg-[var(--border)]/20 p-4">
        <h3 className="text-base font-semibold text-[var(--foreground)]">4. Resumo</h3>
        <div className="mt-2 space-y-2 text-sm text-[var(--muted)]">
          <p>
            Etapas ativas: {enableDownload ? "download" : "download desativado"}
            {enableCSV ? ", csv" : ""}
            {enableParquet ? ", parquet" : ""}
          </p>
          <p>
            Modo atual:{" "}
            {selectedCatalogs.size === 0 || !hasSelectedPeriod
              ? "nenhum processamento: falta catálogo ou período"
              : "somente catálogos e períodos selecionados"}
          </p>
          <p>
            Catálogos selecionados: {selectedCatalogLabels.length > 0 ? selectedCatalogLabels.join(", ") : "nenhum"}
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
            Enquanto faltar catálogo ou período, nenhum download nem conversão é enfileirado.
          </p>
        </div>
      </section>
    </section>
  );
}
