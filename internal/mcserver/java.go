package mcserver

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/woopsy/porque/internal/winproc"
)

// javaVersionRe matches the version string printed by `java -version`, e.g.
// `openjdk version "21.0.2"` or `java version "1.8.0_401"`.
var javaVersionRe = regexp.MustCompile(`version "(\d+)(?:\.(\d+))?`)

// detectJavaMajor returns the major version of the `java` on PATH (e.g. 21, 17,
// 8). `java -version` prints to stderr, so we read combined output. Java 8 and
// earlier report as "1.8" — that maps to major 8.
func detectJavaMajor() (int, error) {
	cmd := exec.Command("java", "-version")
	winproc.Hide(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("java not found on PATH: %w", err)
	}
	m := javaVersionRe.FindStringSubmatch(string(out))
	if m == nil {
		return 0, fmt.Errorf("could not parse java version from: %s", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(m[1])
	if major == 1 && m[2] != "" {
		// Legacy "1.X" scheme (Java 8 and earlier).
		major, _ = strconv.Atoi(m[2])
	}
	return major, nil
}

// requiredJavaMajor returns the minimum Java major version needed to run the
// given Minecraft version.
func requiredJavaMajor(mcVersion string) int {
	switch {
	case versionAtLeast(mcVersion, 1, 20, 5), versionAtLeast(mcVersion, 1, 21, 0):
		return 21
	case versionAtLeast(mcVersion, 1, 18, 0):
		return 17
	case versionAtLeast(mcVersion, 1, 17, 0):
		return 16
	default:
		return 8
	}
}

// versionAtLeast reports whether a dotted Minecraft version string is >= the
// given major.minor.patch. Unparseable versions are treated as "modern" (true)
// so we err toward requiring a newer Java rather than silently crashing.
func versionAtLeast(v string, major, minor, patch int) bool {
	parts := strings.Split(strings.TrimSpace(v), ".")
	nums := make([]int, 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return true
		}
		nums[i] = n
	}
	target := []int{major, minor, patch}
	for i := 0; i < 3; i++ {
		if nums[i] != target[i] {
			return nums[i] > target[i]
		}
	}
	return true
}

// checkJavaForVersion verifies a compatible Java is installed for mcVersion. It
// returns a human-readable error suitable for surfacing in the UI when the
// installed Java is too old (or missing).
func checkJavaForVersion(mcVersion string) error {
	required := requiredJavaMajor(mcVersion)
	have, err := detectJavaMajor()
	if err != nil {
		return fmt.Errorf("Java is required to run Minecraft %s (needs Java %d) but no Java was found. Install Adoptium Temurin %d and restart Porque.", mcVersion, required, required)
	}
	if have < required {
		return fmt.Errorf("Minecraft %s needs Java %d, but Java %d was found. Install Adoptium Temurin %d.", mcVersion, required, have, required)
	}
	return nil
}
