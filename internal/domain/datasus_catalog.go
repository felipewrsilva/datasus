package domain

import (
	"fmt"
	"slices"
	"time"
)

type CatalogCode string

type CatalogDefinition struct {
	Code  CatalogCode
	Label string
}

const (
	CatalogSIH     CatalogCode = "SIH"
	CatalogSIA     CatalogCode = "SIA"
	CatalogCNES    CatalogCode = "CNES"
	CatalogSIM     CatalogCode = "SIM"
	CatalogSINASC  CatalogCode = "SINASC"
	CatalogSINAN   CatalogCode = "SINAN"
	CatalogPNI     CatalogCode = "PNI"
	CatalogESUSAPS CatalogCode = "ESUS_APS"
	CatalogCIHA    CatalogCode = "CIHA"
)

var KnownCatalogs = []CatalogDefinition{
	{Code: CatalogSIH, Label: "Hospital Information System"},
	{Code: CatalogSIA, Label: "Ambulatory Information System"},
	{Code: CatalogCNES, Label: "National Registry of Health Facilities"},
	{Code: CatalogSIM, Label: "Mortality Information System"},
	{Code: CatalogSINASC, Label: "Live Birth Information System"},
	{Code: CatalogSINAN, Label: "Notifiable Diseases Information System"},
	{Code: CatalogPNI, Label: "National Immunization Program"},
	{Code: CatalogESUSAPS, Label: "e-SUS / APS"},
	{Code: CatalogCIHA, Label: "Hospitalization Communication System"},
}

func CurrentYear() int {
	return time.Now().Year()
}

func CatalogCodes() []CatalogCode {
	out := make([]CatalogCode, 0, len(KnownCatalogs))
	for _, catalog := range KnownCatalogs {
		out = append(out, catalog.Code)
	}
	return out
}

func IsKnownCatalog(code CatalogCode) bool {
	for _, catalog := range KnownCatalogs {
		if catalog.Code == code {
			return true
		}
	}
	return false
}

func YearRange(startYear int, endYear int) []int {
	if endYear < startYear {
		return nil
	}
	years := make([]int, 0, endYear-startYear+1)
	for year := startYear; year <= endYear; year++ {
		years = append(years, year)
	}
	return years
}

func AvailableYears() []int {
	return YearRange(0, CurrentYear())
}

func AvailableMonths() []int {
	months := make([]int, 12)
	for i := range 12 {
		months[i] = i + 1
	}
	return months
}

func YearSelectionKey(catalog CatalogCode, year int) string {
	return fmt.Sprintf("%s:%04d", catalog, year)
}

func MonthSelectionKey(catalog CatalogCode, year int, month int) string {
	return fmt.Sprintf("%s:%04d:%02d", catalog, year, month)
}

func SortCatalogCodes(codes []CatalogCode) {
	slices.Sort(codes)
}
