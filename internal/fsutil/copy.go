package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// CopyToDir copies src (file or directory) into dstDir. dstDir must refer to a
// directory. When dstDir does not exist, it (and any required parent
// directories) are created; if any segment along the path already exists as a
// file, the copy fails. If overwrite is false, attempting to copy over existing
// files is an error. On success, the returned function reverts the filesystem to
// its original state when invoked. The returned function is always safe to call.
func CopyToDir(src, dstDir string, overwrite bool) (func(), error) {
	noop := func() {}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return noop, err
	}

	txn := &copyTxn{overwrite: overwrite}

	createdDirs, err := trackedMkdirAll(dstDir, 0o755)
	if err != nil {
		return noop, err
	}
	txn.createdDirs = append(txn.createdDirs, createdDirs...)

	dstInfo, err := os.Stat(dstDir)
	if err != nil {
		txn.rollback()
		return noop, err
	}
	if !dstInfo.IsDir() {
		txn.rollback()
		return noop, fmt.Errorf("fsutil: destination %q is not a directory", dstDir)
	}

	var copyErr error
	if srcInfo.IsDir() {
		copyErr = txn.copyDirContents(src, dstDir)
	} else {
		copyErr = txn.copyFile(src, filepath.Join(dstDir, filepath.Base(src)), srcInfo.Mode())
	}
	if copyErr != nil {
		txn.rollback()
		return noop, copyErr
	}

	return txn.commit(), nil
}

type copyTxn struct {
	overwrite    bool
	createdDirs  []string
	createdFiles []string
	overwritten  []overwrittenFile
}

type overwrittenFile struct {
	path   string
	backup string
	mode   os.FileMode
}

func (t *copyTxn) copyDirContents(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return t.ensureDir(target, info.Mode())
		}
		return t.copyFile(path, target, info.Mode())
	})
}

func (t *copyTxn) ensureDir(path string, mode os.FileMode) error {
	created, err := trackedMkdirAll(path, mode)
	if err != nil {
		return err
	}
	t.createdDirs = append(t.createdDirs, created...)
	return nil
}

func (t *copyTxn) copyFile(src, dst string, mode os.FileMode) error {
	if err := t.ensureDir(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	destInfo, err := os.Stat(dst)
	destExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if destExists {
		if destInfo.IsDir() {
			return fmt.Errorf("fsutil: destination %q is a directory", dst)
		}
		if !t.overwrite {
			return fmt.Errorf("fsutil: destination file %q already exists", dst)
		}
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(dst), ".fsutil-copy-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	tmpClosed := false
	renamed := false
	defer func() {
		if !tmpClosed {
			_ = tmpFile.Close()
		}
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmpFile.Chmod(mode); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	tmpClosed = true

	if destExists {
		backupPath, err := createBackup(dst)
		if err != nil {
			return err
		}
		t.overwritten = append(t.overwritten, overwrittenFile{
			path:   dst,
			backup: backupPath,
			mode:   destInfo.Mode(),
		})
	} else {
		t.createdFiles = append(t.createdFiles, dst)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	renamed = true

	return nil
}

func (t *copyTxn) rollback() {
	undoChanges(t.createdFiles, t.createdDirs, t.overwritten)
	t.createdFiles = nil
	t.createdDirs = nil
	t.overwritten = nil
}

func (t *copyTxn) commit() func() {
	createdFiles := append([]string(nil), t.createdFiles...)
	createdDirs := append([]string(nil), t.createdDirs...)
	overwritten := append([]overwrittenFile(nil), t.overwritten...)
	var once sync.Once

	return func() {
		once.Do(func() {
			undoChanges(createdFiles, createdDirs, overwritten)
		})
	}
}

func undoChanges(createdFiles, createdDirs []string, overwritten []overwrittenFile) {
	for i := len(overwritten) - 1; i >= 0; i-- {
		restoreFile(overwritten[i])
		_ = os.Remove(overwritten[i].backup)
	}
	for i := len(createdFiles) - 1; i >= 0; i-- {
		_ = os.Remove(createdFiles[i])
	}
	for i := len(createdDirs) - 1; i >= 0; i-- {
		_ = os.Remove(createdDirs[i])
	}
}

func restoreFile(entry overwrittenFile) {
	dir := filepath.Dir(entry.path)
	tmpFile, err := os.CreateTemp(dir, ".fsutil-undo-*")
	if err != nil {
		return
	}
	tmpName := tmpFile.Name()
	tmpClosed := false
	renamed := false
	defer func() {
		if !tmpClosed {
			_ = tmpFile.Close()
		}
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	srcFile, err := os.Open(entry.backup)
	if err != nil {
		return
	}
	defer srcFile.Close()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		return
	}
	if err := tmpFile.Chmod(entry.mode); err != nil {
		return
	}
	if err := tmpFile.Close(); err != nil {
		return
	}
	tmpClosed = true

	if err := os.Rename(tmpName, entry.path); err != nil {
		return
	}
	renamed = true
}

func createBackup(path string) (string, error) {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".fsutil-backup-*")
	if err != nil {
		return "", err
	}
	name := tmpFile.Name()
	var ok bool
	defer func() {
		if !ok {
			_ = tmpFile.Close()
			_ = os.Remove(name)
		}
	}()

	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	if _, err := io.Copy(tmpFile, src); err != nil {
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}
	ok = true
	return name, nil
}

func trackedMkdirAll(path string, mode os.FileMode) ([]string, error) {
	clean := filepath.Clean(path)
	if clean == "." {
		return nil, nil
	}

	var missing []string
	current := clean
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("fsutil: %q exists and is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	created := make([]string, 0, len(missing))
	for i := len(missing) - 1; i >= 0; i-- {
		dir := missing[i]
		if err := os.Mkdir(dir, mode); err != nil {
			if os.IsExist(err) {
				continue
			}
			for j := len(created) - 1; j >= 0; j-- {
				_ = os.Remove(created[j])
			}
			return nil, err
		}
		created = append(created, dir)
	}
	return created, nil
}
