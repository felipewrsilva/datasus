import { describe, expect, it } from "vitest";
import {
  formatDateBR,
  formatDateTimeBR,
  formatDateTimeSecondsBR,
  formatTimeOnlySecondsBR,
} from "@/lib/dateFormat";

describe("dateFormat", () => {
  const sample = "2026-04-28T13:14:15Z";

  it("formata data como dd/MM/yyyy", () => {
    const value = formatDateBR(sample);
    expect(value).toMatch(/^\d{2}\/\d{2}\/\d{4}$/);
  });

  it("formata data e hora como dd/MM/yyyy HH:mm sem vírgula", () => {
    const value = formatDateTimeBR(sample);
    expect(value).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/);
    expect(value).not.toContain(",");
  });

  it("formata data e hora com segundos como dd/MM/yyyy HH:mm:ss", () => {
    const value = formatDateTimeSecondsBR(sample);
    expect(value).toMatch(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}:\d{2}$/);
    expect(value).not.toContain(",");
  });

  it("retorna fallback para entradas inválidas", () => {
    expect(formatDateBR("invalid")).toBe("Data desconhecida");
    expect(formatDateTimeBR(null)).toBe("Data desconhecida");
    expect(formatDateTimeSecondsBR(undefined)).toBe("Data desconhecida");
    expect(formatTimeOnlySecondsBR("invalid")).toBe("");
  });
});
