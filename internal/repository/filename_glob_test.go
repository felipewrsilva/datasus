package repository

import "testing"

func TestFilenameGlobToILike(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", "%"},
		{"FOO.DBC", "%FOO.DBC%"},
		{"FOO*DBC", "FOO%DBC"},
		{"%_%", "%#%#_#%%"},
		{"*A*", "%A%"},
		{"SPT#01", "%SPT##01%"},
	}
	for _, tc := range cases {
		got := FilenameGlobToILike(tc.in)
		if got != tc.want {
			t.Errorf("FilenameGlobToILike(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
