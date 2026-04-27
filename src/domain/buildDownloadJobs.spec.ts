import { describe, expect, it } from "vitest";

import { buildDownloadJobs } from "./buildDownloadJobs";
import { availableYears, currentYear, monthSelectionKey, yearSelectionKey } from "./datasusCatalog";
import { createSelectionState, selectCatalog, selectMonth, selectYear } from "./selectionModel";

describe("buildDownloadJobs", () => {
  it("selecting catalog selects all years and months", () => {
    const state = createSelectionState();
    selectCatalog(state, "SIH");

    const result = buildDownloadJobs(state);
    const years = availableYears();

    expect(result.invalid).toHaveLength(0);
    expect(result.jobs).toHaveLength(years.length * 12);
    expect(result.jobs[0]).toMatchObject({ catalog: "SIH", year: years[0], month: 1 });
    expect(result.jobs[result.jobs.length - 1]).toMatchObject({
      catalog: "SIH",
      year: currentYear(),
      month: 12,
    });
  });

  it("selecting year selects all months", () => {
    const state = createSelectionState();
    selectYear(state, "SIA", 2024);

    const result = buildDownloadJobs(state);
    expect(result.invalid).toHaveLength(0);
    expect(result.jobs).toHaveLength(12);
    expect(result.jobs[0]).toMatchObject({ catalog: "SIA", year: 2024, month: 1 });
    expect(result.jobs[11]).toMatchObject({ catalog: "SIA", year: 2024, month: 12 });
  });

  it("supports partial selections and deduplicates overlaps", () => {
    const state = createSelectionState();
    selectMonth(state, "SIM", 2025, 7);
    state.months.add(monthSelectionKey("SIM", 2025, 7));
    state.yearsAll.add(yearSelectionKey("SIM", 2025));

    const result = buildDownloadJobs(state);
    expect(result.invalid).toHaveLength(0);
    expect(result.jobs).toHaveLength(12);
    expect(result.jobs[0].key).toBe("SIM:2025:01");
    expect(result.jobs[11].key).toBe("SIM:2025:12");
  });

  it("is deterministic and idempotent for same input", () => {
    const state = createSelectionState({
      catalogsAll: ["CNES", "CNES"],
      yearsAll: ["SINAN:2024", "SINAN:2024"],
      months: ["PNI:2023:01", "PNI:2023:01"],
    });

    const first = buildDownloadJobs(state);
    const second = buildDownloadJobs(state);
    expect(second).toEqual(first);
  });

  it("reports invalid selection values", () => {
    const state = createSelectionState({
      catalogsAll: ["SIH"],
      yearsAll: ["SIA:2019", "broken"],
      months: ["SIM:2024:13", "bad"],
    });
    // simulate runtime invalid catalog input from persisted data
    state.catalogsAll.add("UNKNOWN" as never);

    const result = buildDownloadJobs(state);
    expect(result.invalid.length).toBeGreaterThanOrEqual(5);
  });
});
