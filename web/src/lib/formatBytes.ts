/** Human-readable byte size: base 1024, B through TB. */
export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  const decimals = size >= 10 || index === 0 ? 0 : 1;
  return `${size.toFixed(decimals)} ${units[index]}`;
}

/** Tamanho em destaque, quantidade entre parênteses (pt-BR). */
type FormatSizeAndFileCountOptions = {
  locale?: string;
  zeroSizeUnit?: "B" | "GB";
};

export function formatSizeAndFileCount(
  totalSizeBytes: number,
  count: number,
  options: string | FormatSizeAndFileCountOptions = "pt-BR",
): string {
  const locale = typeof options === "string" ? options : (options.locale ?? "pt-BR");
  const zeroSizeUnit = typeof options === "string" ? "B" : (options.zeroSizeUnit ?? "B");
  const normalizedCount = Number.isFinite(count) && count > 0 ? Math.trunc(count) : 0;
  const sizeLabel =
    Number.isFinite(totalSizeBytes) && totalSizeBytes > 0
      ? formatBytes(totalSizeBytes)
      : zeroSizeUnit === "GB"
        ? "0 GB"
        : "0 B";

  return `${sizeLabel} (${normalizedCount.toLocaleString(locale)} arquivos)`;
}
