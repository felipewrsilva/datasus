import {
  availableMonths,
  availableYears,
  isKnownCatalog,
  monthSelectionKey,
  type CatalogCode,
} from "./datasusCatalog";
import type { SelectionState } from "./selectionModel";

export type DownloadJob = {
  catalog: CatalogCode;
  year: number;
  month: number;
  key: string;
};

export type InvalidSelection = {
  level: "catalog" | "year" | "month";
  value: string;
  reason: string;
};

export type BuildDownloadJobsResult = {
  jobs: DownloadJob[];
  invalid: InvalidSelection[];
};

export function buildDownloadJobs(selection: SelectionState): BuildDownloadJobsResult {
  const validYears = new Set(availableYears());
  const jobMap = new Map<string, DownloadJob>();
  const invalid: InvalidSelection[] = [];

  const addJob = (catalog: CatalogCode, year: number, month: number): void => {
    const key = monthSelectionKey(catalog, year, month);
    jobMap.set(key, { catalog, year, month, key });
  };

  for (const catalog of selection.catalogsAll) {
    if (!isKnownCatalog(catalog)) {
      invalid.push({ level: "catalog", value: catalog, reason: "unknown catalog" });
      continue;
    }
    for (const year of validYears) {
      for (const month of availableMonths()) {
        addJob(catalog, year, month);
      }
    }
  }

  for (const yearKey of selection.yearsAll) {
    const parsed = parseYearKey(yearKey);
    if (!parsed.ok) {
      invalid.push({ level: "year", value: yearKey, reason: parsed.reason });
      continue;
    }
    const { catalog, year } = parsed;
    if (!isKnownCatalog(catalog)) {
      invalid.push({ level: "year", value: yearKey, reason: "unknown catalog" });
      continue;
    }
    if (!validYears.has(year)) {
      invalid.push({ level: "year", value: yearKey, reason: "year out of range" });
      continue;
    }
    for (const month of availableMonths()) {
      addJob(catalog, year, month);
    }
  }

  for (const monthKey of selection.months) {
    const parsed = parseMonthKey(monthKey);
    if (!parsed.ok) {
      invalid.push({ level: "month", value: monthKey, reason: parsed.reason });
      continue;
    }
    const { catalog, year, month } = parsed;
    if (!isKnownCatalog(catalog)) {
      invalid.push({ level: "month", value: monthKey, reason: "unknown catalog" });
      continue;
    }
    if (!validYears.has(year)) {
      invalid.push({ level: "month", value: monthKey, reason: "year out of range" });
      continue;
    }
    if (month < 1 || month > 12) {
      invalid.push({ level: "month", value: monthKey, reason: "month out of range" });
      continue;
    }
    addJob(catalog, year, month);
  }

  const jobs = [...jobMap.values()].sort((a, b) => {
    if (a.catalog !== b.catalog) return a.catalog.localeCompare(b.catalog);
    if (a.year !== b.year) return a.year - b.year;
    return a.month - b.month;
  });

  return { jobs, invalid };
}

function parseYearKey(value: string):
  | { ok: true; catalog: string; year: number }
  | { ok: false; reason: string } {
  const parts = value.split(":");
  if (parts.length !== 2) return { ok: false, reason: "invalid key format" };
  const year = Number(parts[1]);
  if (!Number.isInteger(year)) return { ok: false, reason: "invalid year" };
  return { ok: true, catalog: parts[0], year };
}

function parseMonthKey(value: string):
  | { ok: true; catalog: string; year: number; month: number }
  | { ok: false; reason: string } {
  const parts = value.split(":");
  if (parts.length !== 3) return { ok: false, reason: "invalid key format" };
  const year = Number(parts[1]);
  const month = Number(parts[2]);
  if (!Number.isInteger(year)) return { ok: false, reason: "invalid year" };
  if (!Number.isInteger(month)) return { ok: false, reason: "invalid month" };
  return { ok: true, catalog: parts[0], year, month };
}
