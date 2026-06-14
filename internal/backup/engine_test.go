package backup

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTar builds a small uncompressed tar with the given files.
func makeTar(t *testing.T, files map[string]string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "src-*.tar")
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestWriteAndValidateArchive_Valid(t *testing.T) {
	src := makeTar(t, map[string]string{"data/server.properties": "motd=hi", "data/world/level.dat": "x"})
	defer src.Close()

	out := filepath.Join(t.TempDir(), "backup.tar.gz")
	sha, err := writeArchive(out, src)
	if err != nil {
		t.Fatalf("writeArchive: %v", err)
	}
	if len(sha) != 64 {
		t.Errorf("sha length = %d, want 64", len(sha))
	}
	if err := validateArchive(out); err != nil {
		t.Errorf("validateArchive on good archive: %v", err)
	}
	// sha256File over the same file must match the digest returned by writeArchive.
	if got, _ := sha256File(out); got != sha {
		t.Errorf("sha256File = %s, want %s", got, sha)
	}
}

func TestValidateArchive_CorruptGzip(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "corrupt.tar.gz")
	if err := os.WriteFile(bad, []byte("this is definitely not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchive(bad); err == nil {
		t.Fatal("expected error for corrupt gzip, got nil")
	}
}

func TestValidateArchive_TruncatedTar(t *testing.T) {
	src := makeTar(t, map[string]string{"data/world/region/r.0.0.mca": strings.Repeat("A", 4096)})
	defer src.Close()
	out := filepath.Join(t.TempDir(), "trunc.tar.gz")
	if _, err := writeArchive(out, src); err != nil {
		t.Fatal(err)
	}
	// Truncate the gzip file mid-stream to simulate a partial/corrupt backup.
	data, _ := os.ReadFile(out)
	if err := os.WriteFile(out, data[:len(data)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchive(out); err == nil {
		t.Fatal("expected error for truncated archive, got nil")
	}
}

// sanity: gzip reader rejects an empty file.
func TestValidateArchive_Empty(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.tar.gz")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchive(empty); err == nil {
		t.Fatal("expected error for empty archive, got nil")
	}
}
