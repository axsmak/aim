// Package fsutil provides small filesystem helpers shared across AIM's
// write paths.
package fsutil

import (
	"os"
	"path/filepath"
)

// WriteFile atomically writes data to path: it writes to a temporary file
// in the same directory as path, then renames it into place. A reader never
// observes a partially written or truncated file at path — an interruption
// mid-write leaves either the old content or the new content, never a mix.
//
// The temporary file is created in path's directory (not os.TempDir()) so
// the final rename stays within one filesystem; a rename across filesystems
// fails with EXDEV.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".aim-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// os.CreateTemp creates the file with mode 0600 regardless of perm;
	// the target mode must be set explicitly before the rename.
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
