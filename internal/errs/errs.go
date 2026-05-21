package errs

import (
	"fmt"
	"os"
)

// Fatal writes "error: <msg>" to stderr and exits with code 1.
func Fatal(msg string) {
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	os.Exit(1)
}

// Fatalf writes a formatted "error: ..." to stderr and exits with code 1.
func Fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
