const INVALID_LABEL = "Data desconhecida";
const LEFT_TO_RIGHT_MARK = /\u200e/g;

function parseDate(value: string | Date | null | undefined): Date | null {
  if (!value) return null;
  const parsed = value instanceof Date ? new Date(value.getTime()) : new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed;
}

function normalizeFormattedDateTime(value: string): string {
  return value.replace(LEFT_TO_RIGHT_MARK, "").replace(",", "");
}

const dateOnlyFormatter = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
});

const dateTimeFormatter = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

const dateTimeSecondsFormatter = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

const timeOnlySecondsFormatter = new Intl.DateTimeFormat("pt-BR", {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

export function formatDateBR(value: string | Date | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return INVALID_LABEL;
  return dateOnlyFormatter.format(parsed);
}

export function formatDateTimeBR(value: string | Date | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return INVALID_LABEL;
  return normalizeFormattedDateTime(dateTimeFormatter.format(parsed));
}

export function formatDateTimeSecondsBR(value: string | Date | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return INVALID_LABEL;
  return normalizeFormattedDateTime(dateTimeSecondsFormatter.format(parsed));
}

export function formatTimeOnlySecondsBR(value: string | Date | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return "";
  return timeOnlySecondsFormatter.format(parsed);
}
