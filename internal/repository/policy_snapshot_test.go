package repository

import "testing"

func TestPolicySnapshot_Allows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snap     PolicySnapshot
		catalog  string
		year     int
		month    int
		expected bool
	}{
		{
			name:     "no selection denies all",
			snap:     PolicySnapshot{},
			catalog:  "RD",
			year:     2024,
			month:    1,
			expected: false,
		},
		{
			name: "catalog mismatch",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "SP", year: 2024, month: 1,
			expected: false,
		},
		{
			name: "year matches catalog",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "RD", year: 2024, month: 7,
			expected: true,
		},
		{
			name: "month matches catalog",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				years:        map[int]struct{}{},
				months:       map[int]map[int]struct{}{2024: {3: {}}},
			},
			catalog: "RD", year: 2024, month: 3,
			expected: true,
		},
		{
			name: "month out of selection",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				years:        map[int]struct{}{},
				months:       map[int]map[int]struct{}{2024: {3: {}}},
			},
			catalog: "RD", year: 2024, month: 4,
			expected: false,
		},
		{
			name: "lowercase catalog accepted via normalization",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "rd", year: 2024, month: 1,
			expected: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.snap.Allows(tc.catalog, tc.year, tc.month)
			if got != tc.expected {
				t.Fatalf("Allows(%q, %d, %d) = %v, want %v",
					tc.catalog, tc.year, tc.month, got, tc.expected)
			}
		})
	}
}
