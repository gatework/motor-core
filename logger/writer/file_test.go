package writer

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReportsWriteFailure(t *testing.T) {
	w, err := New(WithPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("line")); err == nil {
		t.Fatal("Write reported success on closed file")
	}
}
func TestFileCloseAndRotation(t *testing.T) {
	dir := t.TempDir()
	w, err := New(WithPath(dir), WithCap(3))
	if err != nil {
		t.Fatal(err)
	}
	closer, ok := any(w).(io.Closer)
	if !ok {
		_ = w.file.Close()
		t.Fatal("writer has no close operation")
	}
	defer closer.Close()
	data := []byte("first")
	if n, err := w.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	copy(data, "other")
	if _, err := w.Write([]byte("second")); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("late")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Write after Close: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content += string(data)
	}
	if content != "firstsecond" {
		t.Fatalf("written %q", content)
	}
}
