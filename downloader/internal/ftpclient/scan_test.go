package ftpclient

import "testing"

func TestFtpPathJoin(t *testing.T) {
	tests := []struct {
		parent, child, want string
	}{
		{"/a/b", "c", "/a/b/c"},
		{"/a/b/", "c", "/a/b/c"},
		{"", "c", "c"},
		{"/a", "", "/a"},
		{"/a/b", "/c", "/a/b/c"},
	}
	for _, tc := range tests {
		got := FtpPathJoin(tc.parent, tc.child)
		if got != tc.want {
			t.Errorf("FtpPathJoin(%q,%q)=%q want %q", tc.parent, tc.child, got, tc.want)
		}
	}
}
