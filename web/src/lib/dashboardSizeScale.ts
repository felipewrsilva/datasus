/**
 * Escala do painel: 6 níveis (3 categorias × 2 tons) por distribuição de tamanho total,
 * mesma semântica em light e dark (menor = mais leve, maior = mais destaque).
 */

export const SEXTILE_COUNT = 6;

/** Ordena tamanhos totais não negativos para cálculo de sextis. */
export function sortedSizesFromBuckets(buckets: { total_size_bytes: number }[]): number[] {
  return buckets.map((b) => Math.max(0, b.total_size_bytes ?? 0)).sort((a, b) => a - b);
}

/**
 * Nível 0..5 a partir da distribuição: sextis por posto (equivalente a 3 terços × 2 metades).
 * Tamanho zero → nível 0. Todos os tamanhos iguais e positivos → nível 5.
 */
export function sizeToSextileLevel(sortedAsc: number[], size: number): number {
  const s = Math.max(0, size);
  if (sortedAsc.length === 0 || s <= 0) return 0;
  const n = sortedAsc.length;
  const minv = sortedAsc[0]!;
  const maxv = sortedAsc[n - 1]!;
  if (maxv <= 0) return 0;
  if (minv === maxv) return 5;

  const less = sortedAsc.filter((x) => x < s).length;
  const eq = sortedAsc.filter((x) => x === s).length;
  const pr = (less + 0.5 * eq) / n;
  return Math.min(SEXTILE_COUNT - 1, Math.floor(pr * SEXTILE_COUNT));
}

/** Limites [mín, máx] por bin para legenda (valores reais da amostra). */
export function sextileBinBoundaries(sortedAsc: number[]): [number, number][] {
  const n = sortedAsc.length;
  if (n === 0) return [];
  return Array.from({ length: SEXTILE_COUNT }, (_, b) => {
    const lo = Math.floor((b * n) / SEXTILE_COUNT);
    const hi = Math.min(n - 1, Math.ceil(((b + 1) * n) / SEXTILE_COUNT) - 1);
    return [sortedAsc[lo]!, sortedAsc[hi]!] as [number, number];
  });
}

/** Rótulo semântico por nível para leitura rápida da legenda. */
export const SEXTILE_LEVEL_LABELS = [
  "Baixo",
  "Baixo",
  "Médio",
  "Médio",
  "Alto",
  "Alto",
] as const;

/** Preenchimento SVG: menor = mais claro / menos saturado, maior = mais presença. */
export const MAP_FILL_LIGHT = [
  "#dbeafe",
  "#c7ddff",
  "#adcfff",
  "#93c5fd",
  "#3b82f6",
  "#1e40af",
] as const;

/** Dark: menor = discreto no fundo escuro, maior = mais luminoso e saturado. */
export const MAP_FILL_DARK = [
  "#93c5fd",
  "#60a5fa",
  "#3b82f6",
  "#2563eb",
  "#1d4ed8",
  "#1e3a8a",
] as const;

export function mapFillPalette(isDark: boolean): readonly string[] {
  return isDark ? MAP_FILL_DARK : MAP_FILL_LIGHT;
}

/**
 * Barras horizontais nas listas: mesma progressão (Tailwind + dark:).
 * Cores literais para evitar purge.
 */
export const SIZE_BAR_CLASSES = [
  "bg-slate-300/70 dark:bg-slate-500/30",
  "bg-slate-400/80 dark:bg-slate-400/40",
  "bg-sky-300/90 dark:bg-sky-500/35",
  "bg-sky-400 dark:bg-sky-400/55",
  "bg-blue-500 dark:bg-blue-400/80",
  "bg-blue-600 dark:bg-blue-300",
] as const;

export function barClassForLevel(level: number): string {
  const i = Math.max(0, Math.min(SIZE_BAR_CLASSES.length - 1, level));
  return SIZE_BAR_CLASSES[i]!;
}
