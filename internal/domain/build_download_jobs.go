package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type DownloadJob struct {
	Catalog CatalogCode `json:"catalog"`
	Year    int         `json:"year"`
	Month   int         `json:"month"`
	Key     string      `json:"key"`
}

type InvalidSelection struct {
	Level  string `json:"level"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type BuildDownloadJobsResult struct {
	Jobs    []DownloadJob      `json:"jobs"`
	Invalid []InvalidSelection `json:"invalid,omitempty"`
}

func BuildDownloadJobs(selection SelectionIntent) BuildDownloadJobsResult {
	normalized := selection.Normalize()
	validYears := make(map[int]struct{})
	for _, year := range AvailableYears() {
		validYears[year] = struct{}{}
	}

	jobMap := map[string]DownloadJob{}
	invalid := make([]InvalidSelection, 0)

	addJob := func(catalog CatalogCode, year int, month int) {
		key := MonthSelectionKey(catalog, year, month)
		jobMap[key] = DownloadJob{
			Catalog: catalog,
			Year:    year,
			Month:   month,
			Key:     key,
		}
	}

	for _, catalog := range normalized.CatalogsAll {
		if !IsKnownCatalog(catalog) {
			invalid = append(invalid, InvalidSelection{Level: "catalog", Value: string(catalog), Reason: "unknown catalog"})
			continue
		}
		for year := range validYears {
			for month := 1; month <= 12; month++ {
				addJob(catalog, year, month)
			}
		}
	}

	for _, yearSelectionKey := range normalized.YearsAll {
		catalog, year, err := parseYearSelection(yearSelectionKey)
		if err != nil {
			invalid = append(invalid, InvalidSelection{Level: "year", Value: yearSelectionKey, Reason: err.Error()})
			continue
		}
		if !IsKnownCatalog(catalog) {
			invalid = append(invalid, InvalidSelection{Level: "year", Value: yearSelectionKey, Reason: "unknown catalog"})
			continue
		}
		if _, ok := validYears[year]; !ok {
			invalid = append(invalid, InvalidSelection{Level: "year", Value: yearSelectionKey, Reason: "year out of range"})
			continue
		}
		for month := 1; month <= 12; month++ {
			addJob(catalog, year, month)
		}
	}

	for _, monthSelectionKey := range normalized.Months {
		catalog, year, month, err := parseMonthSelection(monthSelectionKey)
		if err != nil {
			invalid = append(invalid, InvalidSelection{Level: "month", Value: monthSelectionKey, Reason: err.Error()})
			continue
		}
		if !IsKnownCatalog(catalog) {
			invalid = append(invalid, InvalidSelection{Level: "month", Value: monthSelectionKey, Reason: "unknown catalog"})
			continue
		}
		if _, ok := validYears[year]; !ok {
			invalid = append(invalid, InvalidSelection{Level: "month", Value: monthSelectionKey, Reason: "year out of range"})
			continue
		}
		if month < 1 || month > 12 {
			invalid = append(invalid, InvalidSelection{Level: "month", Value: monthSelectionKey, Reason: "month out of range"})
			continue
		}
		addJob(catalog, year, month)
	}

	jobs := make([]DownloadJob, 0, len(jobMap))
	for _, job := range jobMap {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Catalog != jobs[j].Catalog {
			return jobs[i].Catalog < jobs[j].Catalog
		}
		if jobs[i].Year != jobs[j].Year {
			return jobs[i].Year < jobs[j].Year
		}
		return jobs[i].Month < jobs[j].Month
	})

	return BuildDownloadJobsResult{
		Jobs:    jobs,
		Invalid: invalid,
	}
}

func parseYearSelection(key string) (CatalogCode, int, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid key format")
	}
	year, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid year")
	}
	return CatalogCode(parts[0]), year, nil
}

func parseMonthSelection(key string) (CatalogCode, int, int, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("invalid key format")
	}
	year, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("invalid year")
	}
	month, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, 0, fmt.Errorf("invalid month")
	}
	return CatalogCode(parts[0]), year, month, nil
}
