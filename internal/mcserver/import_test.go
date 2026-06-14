package mcserver

import "testing"

func TestStripPrefixForNames(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  string
	}{
		{"single wrapper dir", []string{"myworld/server.properties", "myworld/world/level.dat"}, "myworld/"},
		{"files at root", []string{"server.properties", "eula.txt"}, ""},
		{"mixed roots", []string{"a/x.txt", "b/y.txt"}, ""},
		{"root file alongside dir", []string{"top/x.txt", "readme.md"}, ""},
		{"dir entries ignored", []string{"srv/", "srv/server.jar"}, "srv/"},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		if got := stripPrefixForNames(c.names); got != c.want {
			t.Errorf("%s: stripPrefixForNames(%v) = %q, want %q", c.name, c.names, got, c.want)
		}
	}
}

func TestNormalizeZipName(t *testing.T) {
	if got := normalizeZipName(`myworld\world\level.dat`); got != "myworld/world/level.dat" {
		t.Errorf("normalizeZipName backslash = %q", got)
	}
	if got := normalizeZipName("already/forward.txt"); got != "already/forward.txt" {
		t.Errorf("normalizeZipName forward = %q", got)
	}
}
