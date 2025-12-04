package fsutil

import (
	"io"
	"os"
	"path/filepath"
)

// CopyPath copies either a file or directory. It returns true if the destination
// path did not exist before the copy.
func CopyPath(src, dst string) (bool, error) {
	info, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(dst)
	existed := err == nil
	if info.IsDir() {
		if err := copyDir(src, dst, info.Mode()); err != nil {
			return !existed, err
		}
		return !existed, nil
	}
	if err := copyFile(src, dst, info.Mode()); err != nil {
		return !existed, err
	}
	return !existed, nil
}

func copyDir(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
