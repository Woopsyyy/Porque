package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// createArchiveFromDir walking srcDir, tar-gzipping it to dstPath, and returning the SHA-256 hash.
func createArchiveFromDir(srcDir, dstPath string) (sha string, err error) {
	f, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("create archive: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	// gzip writes compressed bytes to both the file and the hash, so the digest
	// is over the on-disk .tar.gz.
	gz := gzip.NewWriter(io.MultiWriter(f, h))
	tw := tar.NewWriter(gz)

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}

		// Skip backup files themselves if they are stored in the server directory
		if rel == "backups" || strings.HasPrefix(rel, "backups/") || strings.HasPrefix(rel, "backups\\") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip session.lock files which are exclusively locked by the running Minecraft server on Windows
		if info.Name() == "session.lock" {
			return nil
		}

		rel = strings.ReplaceAll(rel, "\\", "/")

		header, hdrErr := tar.FileInfoHeader(info, info.Name())
		if hdrErr != nil {
			return hdrErr
		}

		header.Name = "data/" + rel

		if writeErr := tw.WriteHeader(header); writeErr != nil {
			return writeErr
		}

		if info.Mode().IsDir() {
			return nil
		}

		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer file.Close()

		_, copyErr := io.Copy(tw, file)
		return copyErr
	})

	if err != nil {
		return "", fmt.Errorf("walk dir: %w", err)
	}

	// Close writers to ensure everything is flushed to the hash
	if err := tw.Close(); err != nil {
		return "", fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("close gzip: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyDirFiltered recursively copies srcDir into dstDir, applying the same
// exclusions as the archiver (the backups/ subtree and session.lock files).
// It is used to take a fast, point-in-time snapshot of a world while saving is
// frozen, so saving can be re-enabled before the slow compression step.
func copyDirFiltered(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dstDir, 0o755)
		}
		if rel == "backups" || strings.HasPrefix(rel, "backups/") || strings.HasPrefix(rel, "backups\\") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == "session.lock" {
			return nil
		}

		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil // skip symlinks / special files
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// extractArchiveToDir decompresses and extracts the .tar.gz back to destDir, stripping "data/" prefix.
func extractArchiveToDir(srcFile, destDir string) error {
	f, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip header: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar header: %w", err)
		}

		name := hdr.Name
		if strings.HasPrefix(name, "data/") {
			name = strings.TrimPrefix(name, "data/")
		}
		if name == "" {
			continue
		}

		target := filepath.Join(destDir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("mkdir parent: %w", err)
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return fmt.Errorf("create file: %w", err)
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return fmt.Errorf("write file: %w", err)
			}
			outFile.Close()
		}
	}
	return nil
}

// validateArchive verifies that path is a readable gzip stream containing a
// well-formed tar, by walking every entry. It returns a descriptive error for
// a corrupt archive.
func validateArchive(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip header: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	entries := 0
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar header: %w", err)
		}
		// Drain entry bytes to surface truncation/corruption mid-stream.
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return fmt.Errorf("tar body: %w", err)
		}
		entries++
	}
	if entries == 0 {
		return fmt.Errorf("archive contains no entries")
	}
	return nil
}

// sha256File computes the SHA-256 of a file on disk.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
