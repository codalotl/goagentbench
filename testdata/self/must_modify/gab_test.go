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

func TestRunEnforcesMustModifyRules(t *testing.T) {
	const mustModifyTestName = "verify.must-modify"

	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")

	cases := []struct {
		name       string
		scenario   func(name string) *scenario.Scenario
		apply      func(t *testing.T, repo string)
		shouldPass bool
		wantOutput []string
	}{
		{
			name:       "failsWhenNothingChanged",
			scenario:   baseScenario,
			apply:      func(t *testing.T, repo string) {},
			shouldPass: false,
			wantOutput: []string{"allowed"},
		},
		{
			name:     "failsWhenOnlyNestedChange",
			scenario: baseScenario,
			apply: func(t *testing.T, repo string) {
				require.NoError(t, os.MkdirAll(filepath.Join(repo, "allowed/sub"), 0o755))
				writeFile(t, repo, "allowed/sub/nested.txt", "nested")
			},
			shouldPass: false,
			wantOutput: []string{"allowed"},
		},
		{
			name:     "failsWhenNoModifyHit",
			scenario: baseScenario,
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/base.txt", "changed")
				writeFile(t, repo, "forbidden/secret.txt", "leak")
			},
			shouldPass: false,
			wantOutput: []string{"forbidden/secret.txt"},
		},
		{
			name:     "passesWithMustModifySatisfiedAndOtherChangesAllowed",
			scenario: baseScenario,
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/base.txt", "changed")
				writeFile(t, repo, "other/extra.txt", "extra")
				writeFile(t, repo, ".run-start.json", "{}")
				writeFile(t, repo, ".run-progress.json", "{}")
			},
			shouldPass: true,
		},
		{
			name:     "passesWhenMustModifySatisfiedByUntrackedFile",
			scenario: baseScenario,
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/new.txt", "new")
			},
			shouldPass: true,
		},
		{
			name:     "passesWhenMustModifySatisfiedByDeletion",
			scenario: baseScenario,
			apply: func(t *testing.T, repo string) {
				require.NoError(t, os.Remove(filepath.Join(repo, "allowed/base.txt")))
			},
			shouldPass: true,
		},
		{
			name: "passesWhenMustModifyEntryHasTrailingSlash",
			scenario: func(name string) *scenario.Scenario {
				sc := baseScenario(name)
				sc.Verify.MustModify = scenario.StringList{"allowed/"}
				return sc
			},
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/base.txt", "changed")
			},
			shouldPass: true,
		},
		{
			name: "passesWhenMustModifyEntryIsGlob",
			scenario: func(name string) *scenario.Scenario {
				sc := baseScenario(name)
				sc.Verify.MustModify = scenario.StringList{"allowed/*.txt"}
				return sc
			},
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/base.txt", "changed")
			},
			shouldPass: true,
		},
		{
			name: "failsWhenMustModifyEntryIsGlobAndOnlyNestedChanged",
			scenario: func(name string) *scenario.Scenario {
				sc := baseScenario(name)
				sc.Verify.MustModify = scenario.StringList{"allowed/*.txt"}
				return sc
			},
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, "allowed/sub/nested.txt", "nested")
			},
			shouldPass: false,
			wantOutput: []string{"allowed/*.txt"},
		},
		{
			name: "failsWhenMustModifyEntryIsBookkeepingFileEvenIfChanged",
			scenario: func(name string) *scenario.Scenario {
				sc := baseScenario(name)
				sc.Verify.MustModify = scenario.StringList{".run-start.json"}
				return sc
			},
			apply: func(t *testing.T, repo string) {
				writeFile(t, repo, ".run-start.json", "{}")
			},
			shouldPass: false,
			wantOutput: []string{".run-start.json"},
		},
	}

	for _, tt := range cases {
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

			sc := tt.scenario(scenarioName)
			res, err := verify.Run(context.Background(), opts, sc)
			require.NoError(t, err)
			require.NotNil(t, res)
			require.NotNil(t, res.Report)

			if tt.shouldPass {
				require.True(t, res.Report.Success)
				require.Empty(t, res.Report.Tests)
				return
			}

			require.False(t, res.Report.Success)
			require.Len(t, res.Report.Tests, 1)
			require.Equal(t, mustModifyTestName, res.Report.Tests[0].Name)
			require.False(t, res.Report.Tests[0].Passed)
			require.Empty(t, res.Report.Tests[0].Error)
			for _, want := range tt.wantOutput {
				require.Contains(t, res.Report.Tests[0].Output, want)
			}
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
