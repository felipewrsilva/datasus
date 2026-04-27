export type CatalogCode =
  | "SIH"
  | "SIA"
  | "CNES"
  | "SIM"
  | "SINASC"
  | "SINAN"
  | "PNI"
  | "ESUS_APS"
  | "CIHA";

export type CatalogDefinition = {
  code: CatalogCode;
  label: string;
};

export const FIRST_AVAILABLE_YEAR = 0;

export const DATASUS_CATALOGS: readonly CatalogDefinition[] = [
  { code: "SIH", label: "Hospital Information System" },
  { code: "SIA", label: "Ambulatory Information System" },
  { code: "CNES", label: "National Registry of Health Facilities" },
  { code: "SIM", label: "Mortality Information System" },
  { code: "SINASC", label: "Live Birth Information System" },
  { code: "SINAN", label: "Notifiable Diseases Information System" },
  { code: "PNI", label: "National Immunization Program" },
  { code: "ESUS_APS", label: "e-SUS / APS" },
  { code: "CIHA", label: "Hospitalization Communication System" },
];

export function currentYear(now: Date = new Date()): number {
  return now.getUTCFullYear();
}

export function availableYears(now: Date = new Date()): number[] {
  const endYear = currentYear(now);
  const years: number[] = [];
  for (let year = FIRST_AVAILABLE_YEAR; year <= endYear; year += 1) {
    years.push(year);
  }
  return years;
}

export function availableMonths(): number[] {
  return [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12];
}

export function isKnownCatalog(code: string): code is CatalogCode {
  return DATASUS_CATALOGS.some((catalog) => catalog.code === code);
}

export function yearSelectionKey(catalog: CatalogCode, year: number): string {
  return `${catalog}:${String(year).padStart(4, "0")}`;
}

export function monthSelectionKey(
  catalog: CatalogCode,
  year: number,
  month: number,
): string {
  return `${catalog}:${String(year).padStart(4, "0")}:${String(month).padStart(2, "0")}`;
}
