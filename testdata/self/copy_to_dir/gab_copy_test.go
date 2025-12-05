package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codalotl/goagentbench/internal/fsutil"
)

func TestCopyToDirCopiesFileAndUndo(t *testing.T) {
	srcDir := t.TempDir()
	dstRoot := t.TempDir()

	srcPath := filepath.Join(srcDir, "file.txt")
	writeFile(t, srcPath, "hello")

	dstDir := filepath.Join(dstRoot, "dest")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	undo, err := fsutil.CopyToDir(srcPath, dstDir, false)
	if err != nil {
		t.Fatalf("CopyToDir returned error: %v", err)
	}

	copied := filepath.Join(dstDir, "file.txt")
	got := readFile(t, copied)
	if got != "hello" {
		t.Fatalf("copied file content = %q, want %q", got, "hello")
	}

	undo()
	if _, err := os.Stat(copied); !os.IsNotExist(err) {
		t.Fatalf("copied file still exists after undo; err=%v", err)
	}
	if _, err := os.Stat(dstDir); err != nil {
		t.Fatalf("destination dir removed after undo; err=%v", err)
	}
}

func TestCopyToDirCreatesMissingDestination(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "file.txt")
	writeFile(t, srcPath, "data")

	base := t.TempDir()
	dstDir := filepath.Join(base, "nested", "dest")

	undo, err := fsutil.CopyToDir(srcPath, dstDir, false)
	if err != nil {
		t.Fatalf("CopyToDir returned error: %v", err)
	}

	copied := filepath.Join(dstDir, "file.txt")
	if _, err := os.Stat(copied); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	undo()
	if _, err := os.Stat(dstDir); !os.IsNotExist(err) {
		t.Fatalf("destination dir still exists after undo; err=%v", err)
	}
}

func TestCopyToDirCopiesDirectory(t *testing.T) {
	srcRoot := t.TempDir()
	writeFile(t, filepath.Join(srcRoot, "a.txt"), "root")
	if err := os.MkdirAll(filepath.Join(srcRoot, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(srcRoot, "sub", "b.txt"), "nested")

	dstRoot := t.TempDir()
	dstDir := filepath.Join(dstRoot, filepath.Base(srcRoot))

	undo, err := fsutil.CopyToDir(srcRoot, dstDir, false)
	if err != nil {
		t.Fatalf("CopyToDir returned error: %v", err)
	}

	check := func(path, want string) {
		got := readFile(t, filepath.Join(dstDir, path))
		if got != want {
			t.Fatalf("copied %s content = %q, want %q", path, got, want)
		}
	}
	check("a.txt", "root")
	check(filepath.Join("sub", "b.txt"), "nested")

	undo()
	if _, err := os.Stat(dstDir); !os.IsNotExist(err) {
		t.Fatalf("copied directory still exists after undo; err=%v", err)
	}
}

func TestCopyToDirNoOverwrite(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "file.txt")
	writeFile(t, srcPath, "new")

	dstRoot := t.TempDir()
	dstDir := filepath.Join(dstRoot, "dest")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	destFile := filepath.Join(dstDir, "file.txt")
	writeFile(t, destFile, "old")

	undo, err := fsutil.CopyToDir(srcPath, dstDir, false)
	if err == nil {
		t.Fatalf("CopyToDir expected error when overwrite disabled")
	}
	undo()
	if got := readFile(t, destFile); got != "old" {
		t.Fatalf("existing file changed to %q, want %q", got, "old")
	}
}

func TestCopyToDirOverwriteRestore(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "file.txt")
	writeFile(t, srcPath, "replacement")

	dstRoot := t.TempDir()
	dstDir := filepath.Join(dstRoot, "dest")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	destFile := filepath.Join(dstDir, "file.txt")
	writeFile(t, destFile, "original")

	undo, err := fsutil.CopyToDir(srcPath, dstDir, true)
	if err != nil {
		t.Fatalf("CopyToDir returned error: %v", err)
	}

	if got := readFile(t, destFile); got != "replacement" {
		t.Fatalf("file content after copy = %q, want %q", got, "replacement")
	}

	undo()
	if got := readFile(t, destFile); got != "original" {
		t.Fatalf("file content after undo = %q, want %q", got, "original")
	}
}

func TestCopyToDirDestinationConstraints(t *testing.T) {
	t.Run("destinationIsFile", func(t *testing.T) {
		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "file.txt")
		writeFile(t, srcPath, "data")

		dstRoot := t.TempDir()
		dstDir := filepath.Join(dstRoot, "notdir")
		writeFile(t, dstDir, "existing file")

		undo, err := fsutil.CopyToDir(srcPath, dstDir, false)
		if err == nil {
			t.Fatalf("expected error")
		}
		undo()
		if got := readFile(t, dstDir); got != "existing file" {
			t.Fatalf("destination file changed to %q", got)
		}
	})

	t.Run("intermediateSegmentIsFile", func(t *testing.T) {
		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "file.txt")
		writeFile(t, srcPath, "data")

		base := t.TempDir()
		segment := filepath.Join(base, "segment")
		writeFile(t, segment, "file segment")

		dstDir := filepath.Join(segment, "child")

		undo, err := fsutil.CopyToDir(srcPath, dstDir, true)
		if err == nil {
			t.Fatalf("expected error")
		}
		undo()
		if got := readFile(t, segment); got != "file segment" {
			t.Fatalf("segment file changed to %q", got)
		}
	})
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}
