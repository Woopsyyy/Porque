package mcserver

import "testing"

func TestRequiredJavaMajor(t *testing.T) {
	cases := map[string]int{
		"1.21":   21,
		"1.21.4": 21,
		"1.20.6": 21,
		"1.20.5": 21,
		"1.20.4": 17,
		"1.20.1": 17,
		"1.18.2": 17,
		"1.17.1": 16,
		"1.16.5": 8,
		"1.12.2": 8,
	}
	for v, want := range cases {
		if got := requiredJavaMajor(v); got != want {
			t.Errorf("requiredJavaMajor(%q) = %d, want %d", v, got, want)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	if !versionAtLeast("1.20.5", 1, 20, 5) {
		t.Error("1.20.5 should be >= 1.20.5")
	}
	if versionAtLeast("1.20.4", 1, 20, 5) {
		t.Error("1.20.4 should be < 1.20.5")
	}
	if !versionAtLeast("1.21", 1, 21, 0) {
		t.Error("1.21 should be >= 1.21.0")
	}
}
