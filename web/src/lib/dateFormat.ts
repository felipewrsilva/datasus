const DATE_TIME_ZONE = "America/Sao_Paulo";
const INVALID_LABEL = "Data desconhecida";

function parseDate(value: string | null | undefined): Date | null {
  if (!value) return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed;
}

const dateOnlyFormatter = new Intl.DateTimeFormat("pt-BR", {
  timeZone: DATE_TIME_ZONE,
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
});

const dateTimeFormatter = new Intl.DateTimeFormat("pt-BR", {
  timeZone: DATE_TIME_ZONE,
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

const dateTimeSecondsFormatter = new Intl.DateTimeFormat("pt-BR", {
  timeZone: DATE_TIME_ZONE,
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

const timeOnlySecondsFormatter = new Intl.DateTimeFormat("pt-BR", {
  timeZone: DATE_TIME_ZONE,
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

export function formatDateBR(value: string | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return INVALID_LABEL;
  return dateOnlyFormatter.format(parsed);
}

export function formatDateTimeBR(value: string | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return INVALID_LABEL;
  return dateTimeFormatter.format(parsed);
}

export function formatDateTimeSecondsBR(value: string | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return INVALID_LABEL;
  return dateTimeSecondsFormatter.format(parsed);
}

export function formatTimeOnlySecondsBR(value: string | null | undefined): string {
  const parsed = parseDate(value);
  if (!parsed) return "";
  return timeOnlySecondsFormatter.format(parsed);
}
