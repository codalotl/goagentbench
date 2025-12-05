package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvVarWorkspace     = "GOAGENTBENCH_WORKSPACE"
	DefaultWorkspaceDir = "workspace"
)

// Path returns the workspace root directory, using GOAGENTBENCH_WORKSPACE when set.
func Path() string {
	if env := strings.TrimSpace(os.Getenv(EnvVarWorkspace)); env != "" {
		return filepath.Clean(env)
	}
	return DefaultWorkspaceDir
}

// CleanScenario ensures the provided scenario path is safe and normalized.
func CleanScenario(name string) (string, error) {
	if name == "" {
		return "", errors.New("scenario is required")
	}
	if filepath.IsAbs(name) {
		return "", errors.New("scenario must be relative")
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", errors.New("scenario path cannot point outside testdata")
	}
	return clean, nil
}

func ScenarioDir(name string) string {
	return filepath.Join("testdata", name)
}

func ScenarioFile(name string) string {
	return filepath.Join(ScenarioDir(name), "scenario.yml")
}

func WorkspaceScenarioDir(workspacePath, name string) string {
	return filepath.Join(workspacePath, name)
}

// EnsureDir makes sure dir exists.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
