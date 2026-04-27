package domain

import "slices"

// SelectionIntent is the persisted selection state.
// It supports additive composition at catalog, year and month levels.
type SelectionIntent struct {
	CatalogsAll []CatalogCode `json:"catalogs_all"`
	YearsAll    []string      `json:"years_all"`
	Months      []string      `json:"months"`
}

func (s SelectionIntent) Normalize() SelectionIntent {
	out := SelectionIntent{
		CatalogsAll: dedupeCatalogs(s.CatalogsAll),
		YearsAll:    dedupeStrings(s.YearsAll),
		Months:      dedupeStrings(s.Months),
	}
	SortCatalogCodes(out.CatalogsAll)
	slices.Sort(out.YearsAll)
	slices.Sort(out.Months)
	return out
}

func SelectCatalog(s SelectionIntent, catalog CatalogCode) SelectionIntent {
	s.CatalogsAll = append(s.CatalogsAll, catalog)
	return s.Normalize()
}

func SelectYear(s SelectionIntent, catalog CatalogCode, year int) SelectionIntent {
	s.YearsAll = append(s.YearsAll, YearSelectionKey(catalog, year))
	return s.Normalize()
}

func SelectMonth(s SelectionIntent, catalog CatalogCode, year int, month int) SelectionIntent {
	s.Months = append(s.Months, MonthSelectionKey(catalog, year, month))
	return s.Normalize()
}

func dedupeCatalogs(in []CatalogCode) []CatalogCode {
	seen := make(map[CatalogCode]struct{}, len(in))
	out := make([]CatalogCode, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
