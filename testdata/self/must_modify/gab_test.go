package verify_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/verify"
)

func TestRunEnforcesModificationRules(t *testing.T) {
	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")

	tests := []struct {
		name        string
		apply       func(t *testing.T, repo string)
		wantSuccess bool
	}{
		{
			name: "requiresMustModifyChanges",
			apply: func(t *testing.T, repo string) {
				// keep repo clean so must-modify fails
			},
			wantSuccess: false,
		},
		{
			name: "failsOnNoModifyChanges",
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/base.txt", "changed")
				writeFile(t, repo, "forbidden/secret.txt", "leaked")
			},
			wantSuccess: false,
		},
		{
			name: "mustModifyRuleNotSatisfied",
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "other/change.txt", "update")
			},
			wantSuccess: false,
		},
		{
			name: "directoryRuleIsNotRecursive",
			apply: func(t *testing.T, repo string) {
				require.NoError(t, os.MkdirAll(filepath.Join(repo, "allowed/sub"), 0o755))
				writeFile(t, repo, "allowed/sub/nested.txt", "nested change")
			},
			wantSuccess: false,
		},
		{
			name: "passesWithAllowedChangesAndIgnoresMetadata",
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/base.txt", "changed")
				writeFile(t, repo, ".run-start.json", "{}")
				writeFile(t, repo, ".run-progress.json", "{}")
			},
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			scenarioName := "integration-scenario"
			repo := initIntegrationRepo(t, workspaceRoot, scenarioName)
			tt.apply(t, repo)

			opts := verify.Options{
				ScenarioName:  scenarioName,
				WorkspacePath: workspaceRoot,
				RootPath:      workspaceRoot,
				OnlyReport:    true,
				Printer:       output.NewPrinter(nil),
			}

			res, err := verify.Run(context.Background(), opts, baseScenario(scenarioName))
			require.NoError(t, err)
			require.NotNil(t, res)
			require.NotNil(t, res.Report)
			require.Equal(t, tt.wantSuccess, res.Report.Success)

			if tt.wantSuccess {
				require.Empty(t, res.Report.Tests)
				return
			}

			require.Len(t, res.Report.Tests, 1)
			require.False(t, res.Report.Tests[0].Passed)
			require.NotEmpty(t, res.Report.Tests[0].Error)
		})
	}
}

func baseScenario(name string) *scenario.Scenario {
	return &scenario.Scenario{
		Name:   name,
		Repo:   "github.com/example/repo",
		Commit: "1234567",
		Classification: scenario.Classification{
			Type: "build-package",
		},
		Agent: scenario.AgentConfig{
			Instructions: "do work",
		},
		Verify: scenario.VerifyConfig{
			MustModify: scenario.StringList{"allowed"},
			NoModify:   []string{"forbidden/secret.txt"},
		},
	}
}

func initIntegrationRepo(t *testing.T, workspaceRoot, scenarioName string) string {
	t.Helper()
	dir := filepath.Join(workspaceRoot, scenarioName)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")

	writeFile(t, dir, "allowed/base.txt", "original")
	writeFile(t, dir, "allowed/sub/sub1.txt", "original")
	writeFile(t, dir, "forbidden/secret.txt", "keep")

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func writeFile(t *testing.T, dir, relative, content string) {
	t.Helper()
	target := filepath.Join(dir, relative)
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte(content), 0o644))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
