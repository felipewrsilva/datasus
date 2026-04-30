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
		state    string
		expected bool
	}{
		{
			name:     "no selection denies all",
			snap:     PolicySnapshot{},
			catalog:  "RD",
			state:    "SP",
			year:     2024,
			month:    1,
			expected: false,
		},
		{
			name: "catalog mismatch",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				states:       map[string]struct{}{"SP": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "SP", state: "SP", year: 2024, month: 1,
			expected: false,
		},
		{
			name: "year matches catalog and state",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				states:       map[string]struct{}{"SP": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "RD", state: "SP", year: 2024, month: 7,
			expected: true,
		},
		{
			name: "state mismatch denies",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				states:       map[string]struct{}{"SP": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "RD", state: "RJ", year: 2024, month: 7,
			expected: false,
		},
		{
			name: "month matches catalog and state",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				states:       map[string]struct{}{"SP": {}},
				years:        map[int]struct{}{},
				months:       map[int]map[int]struct{}{2024: {3: {}}},
			},
			catalog: "RD", state: "SP", year: 2024, month: 3,
			expected: true,
		},
		{
			name: "month out of selection",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				states:       map[string]struct{}{"SP": {}},
				years:        map[int]struct{}{},
				months:       map[int]map[int]struct{}{2024: {3: {}}},
			},
			catalog: "RD", state: "SP", year: 2024, month: 4,
			expected: false,
		},
		{
			name: "lowercase catalog and state accepted via normalization",
			snap: PolicySnapshot{
				HasSelection: true,
				catalogs:     map[string]struct{}{"RD": {}},
				states:       map[string]struct{}{"SP": {}},
				years:        map[int]struct{}{2024: {}},
				months:       map[int]map[int]struct{}{},
			},
			catalog: "rd", state: "sp", year: 2024, month: 1,
			expected: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.snap.Allows(tc.catalog, tc.state, tc.year, tc.month)
			if got != tc.expected {
				t.Fatalf("Allows(%q, %q, %d, %d) = %v, want %v",
					tc.catalog, tc.state, tc.year, tc.month, got, tc.expected)
			}
		})
	}
}
