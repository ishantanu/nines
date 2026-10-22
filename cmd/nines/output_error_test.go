package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type failOnceWriter struct{ err error }

func (w *failOnceWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		err := w.err
		w.err = nil
		return 0, err
	}
	return len(p), nil
}
func TestTableOutputFailureDoesNotSucceed(t *testing.T) {
	errWrite := errors.New("output disconnected")
	// The single-column header flushes before the final Flush call. A later
	// successful write must not hide the first error from the caller/agent.
	if err := render(&failOnceWriter{errWrite}, "healthy", "table", "status"); !errors.Is(err, errWrite) {
		t.Fatalf("lost output error: %v", err)
	}
	if err := tableRow(&failOnceWriter{errWrite}, "%s\n", "healthy"); !errors.Is(err, errWrite) {
		t.Fatalf("lost row error: %v", err)
	}
}
func TestConfigLockReportsCleanupAndActionErrors(t *testing.T) {
	for _, errAction := range []error{nil, io.ErrUnexpectedEOF} {
		path := filepath.Join(t.TempDir(), "private", "config.yaml")
		// Replace the fixture lock with a non-empty directory to make cleanup fail
		// even when tests run as root. No permissions or user files are changed.
		err := withConfigLock(path, func() error {
			if err := os.Remove(path + ".lock"); err != nil {
				return err
			}
			if err := os.Mkdir(path+".lock", 0700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(path+".lock", "entry"), nil, 0600); err != nil {
				return err
			}
			return errAction
		})
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || pathErr.Path != path+".lock" {
			t.Fatalf("lost lock cleanup error: %v", err)
		}
		if errAction != nil && !errors.Is(err, errAction) {
			t.Fatalf("lost original action error: %v", err)
		}
	}
}
