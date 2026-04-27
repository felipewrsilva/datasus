import type { CatalogCode } from "./datasusCatalog";
import { monthSelectionKey, yearSelectionKey } from "./datasusCatalog";

export type SelectionState = {
  catalogsAll: Set<CatalogCode>;
  yearsAll: Set<string>;
  months: Set<string>;
};

export type SerializableSelectionState = {
  catalogsAll: CatalogCode[];
  yearsAll: string[];
  months: string[];
};

export function createSelectionState(
  initial?: Partial<SerializableSelectionState>,
): SelectionState {
  return {
    catalogsAll: new Set(initial?.catalogsAll ?? []),
    yearsAll: new Set(initial?.yearsAll ?? []),
    months: new Set(initial?.months ?? []),
  };
}

export function selectCatalog(
  state: SelectionState,
  catalog: CatalogCode,
): SelectionState {
  state.catalogsAll.add(catalog);
  return state;
}

export function selectYear(
  state: SelectionState,
  catalog: CatalogCode,
  year: number,
): SelectionState {
  state.yearsAll.add(yearSelectionKey(catalog, year));
  return state;
}

export function selectMonth(
  state: SelectionState,
  catalog: CatalogCode,
  year: number,
  month: number,
): SelectionState {
  state.months.add(monthSelectionKey(catalog, year, month));
  return state;
}

export function toSerializableSelectionState(
  state: SelectionState,
): SerializableSelectionState {
  return {
    catalogsAll: [...state.catalogsAll].sort(),
    yearsAll: [...state.yearsAll].sort(),
    months: [...state.months].sort(),
  };
}
