package updater

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.2", "v0.1.1", true},
		{"v0.1.10", "v0.1.2", true},
		{"0.2.0", "0.1.9", true},
		{"v0.1.1", "v0.1.1", false},
		{"v0.1.0", "v0.1.1", false},
		{"v1.0.0", "v0.9.9", true},
		{"v0.1.2", "dev", false}, // unparseable current → no update
		{"garbage", "v0.1.1", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
