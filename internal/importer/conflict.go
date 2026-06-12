package importer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

// ErrIdentical is returned by CheckConflict when the destination file already
// exists and its content is byte-for-byte identical to the incoming content.
// It is a signal value (like io.EOF), not a real error — callers should handle
// it explicitly rather than propagating it as a failure.
var ErrIdentical = errors.New("identical content")

type ConflictError struct {
	Path string
}

// If this message changes, update the import-specific hint in internal/cli/import.go (errors.As wrap).
func (e ConflictError) Error() string {
	return fmt.Sprintf("%s already exists with different content; use --overwrite to replace or --name to save as a new item", e.Path)
}

type AmbiguousError struct {
	Name    string
	Sources []string
}

func (e AmbiguousError) Error() string {
	return fmt.Sprintf("%s: found in multiple sources %v; use --name to specify which one to import", e.Name, e.Sources)
}

// CheckConflict returns ConflictError if path exists with different content and overwrite is false.
// Returns nil when path doesn't exist, content matches, or overwrite is true.
func CheckConflict(path string, incoming []byte, overwrite bool) error {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if bytes.Equal(existing, incoming) {
		return ErrIdentical
	}
	if overwrite {
		return nil
	}
	return ConflictError{Path: path}
}
