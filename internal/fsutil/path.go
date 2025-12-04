package fsutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeJoin ensures the resulting path stays within base.
func SafeJoin(base, rel string) (string, error) {
	clean := filepath.Clean(rel)
	target := filepath.Join(base, clean)
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTarget, absBase) {
		return "", fmt.Errorf("path %q escapes base directory", rel)
	}
	return target, nil
}
