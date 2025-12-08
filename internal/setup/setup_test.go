package setup_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/setup"
	"github.com/codalotl/goagentbench/internal/workspace"
	"github.com/stretchr/testify/require"
)

const (
	baseFileContent = "Original content\n"
	singlePatch     = `diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-Original content
+Patched content
`
	addNewPatch = `diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+New file content
`
	badPatch = `diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-Nonexistent content
+Should fail
`
)

func createRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")

	writeFile(t, filepath.Join(dir, "file.txt"), baseFileContent)

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial commit")
	out := runGit(t, dir, "rev-parse", "HEAD")
	commit := strings.TrimSpace(out)
	return dir, commit
}

func TestRun_AppliesPatches(t *testing.T) {
	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")
	t.Setenv(workspace.EnvVarScenarioRoot, t.TempDir())
	ctx := context.Background()

	type testCase struct {
		name       string
		patchFiles map[string]string
		patches    scenario.StringList
		expectErr  string
		verify     func(t *testing.T, dir string)
	}

	cases := []testCase{
		{
			name: "single patch",
			patchFiles: map[string]string{
				"single.patch": singlePatch,
			},
			patches: scenario.StringList{"single.patch"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content := readFile(t, filepath.Join(dir, "file.txt"))
				require.Equal(t, "Patched content\n", content)
			},
		},
		{
			name: "multiple patches",
			patchFiles: map[string]string{
				"single.patch":  singlePatch,
				"add-new.patch": addNewPatch,
			},
			patches: scenario.StringList{"single.patch", "add-new.patch"},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content := readFile(t, filepath.Join(dir, "file.txt"))
				require.Equal(t, "Patched content\n", content)
				newContent := readFile(t, filepath.Join(dir, "new.txt"))
				require.Equal(t, "New file content\n", newContent)
			},
		},
		{
			name:      "missing patch file",
			patches:   scenario.StringList{"missing.patch"},
			expectErr: "setup.patch file does not exist",
		},
		{
			name: "patch fails to apply",
			patchFiles: map[string]string{
				"bad.patch": badPatch,
			},
			patches:   scenario.StringList{"bad.patch"},
			expectErr: "git apply",
		},
		{
			name:      "blank patch entry",
			patches:   scenario.StringList{"   "},
			expectErr: "setup.patch entries cannot be empty",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			repoPath, commit := createRepo(t)
			scenarioName := filepath.Join("setup", "patch_case_"+strings.ReplaceAll(strings.ToLower(tc.name), " ", "_"))
			scenarioDir := workspace.ScenarioDir(scenarioName)
			require.NoError(t, os.MkdirAll(scenarioDir, 0o755))
			t.Cleanup(func() {
				cleanupScenarioDir(t, scenarioDir)
			})

			for name, content := range tc.patchFiles {
				writeFile(t, filepath.Join(scenarioDir, name), content)
			}

			printer := output.NewPrinter(io.Discard)
			sc := &scenario.Scenario{
				Name:   "test-scenario",
				Repo:   repoPath,
				Commit: commit,
				Classification: scenario.Classification{
					Type: "build-package",
				},
				Agent: scenario.AgentConfig{
					Instructions: "do stuff",
				},
				Setup: &scenario.SetupConfig{
					Patch: tc.patches,
				},
			}

			workspacePath := filepath.Join(t.TempDir(), "workspace")
			err := setup.Run(ctx, printer, scenarioName, workspacePath, sc)
			if tc.expectErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectErr)
				return
			}
			require.NoError(t, err)
			targetDir := workspace.WorkspaceScenarioDir(workspacePath, scenarioName)
			require.DirExists(t, targetDir)
			tc.verify(t, targetDir)
		})
	}
}

func TestRun_ExecSteps(t *testing.T) {
	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")
	t.Setenv(workspace.EnvVarScenarioRoot, t.TempDir())
	ctx := context.Background()

	repoPath, commit := createRepo(t)
	scenarioName := filepath.Join("setup", "exec_steps")
	scenarioDir := workspace.ScenarioDir(scenarioName)
	require.NoError(t, os.MkdirAll(scenarioDir, 0o755))
	t.Cleanup(func() {
		cleanupScenarioDir(t, scenarioDir)
	})

	writeFile(t, filepath.Join(scenarioDir, "single.patch"), singlePatch)

	printer := output.NewPrinter(io.Discard)
	sc := &scenario.Scenario{
		Name:   "test-scenario",
		Repo:   repoPath,
		Commit: commit,
		Classification: scenario.Classification{
			Type: "build-package",
		},
		Agent: scenario.AgentConfig{
			Instructions: "do stuff",
		},
		Setup: &scenario.SetupConfig{
			Patch: scenario.StringList{"single.patch"},
			Exec: scenario.StringList{
				`grep -q "Patched content" file.txt`,
				"echo exec-ran > exec.log",
			},
		},
	}

	workspacePath := filepath.Join(t.TempDir(), "workspace")
	err := setup.Run(ctx, printer, scenarioName, workspacePath, sc)
	require.NoError(t, err)

	targetDir := workspace.WorkspaceScenarioDir(workspacePath, scenarioName)
	content := readFile(t, filepath.Join(targetDir, "file.txt"))
	require.Equal(t, "Patched content\n", content)
	execLog := readFile(t, filepath.Join(targetDir, "exec.log"))
	require.Equal(t, "exec-ran\n", execLog)
}

func TestRun_ExecStepFailure(t *testing.T) {
	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")
	t.Setenv(workspace.EnvVarScenarioRoot, t.TempDir())
	ctx := context.Background()

	repoPath, commit := createRepo(t)
	scenarioName := filepath.Join("setup", "exec_failure")

	printer := output.NewPrinter(io.Discard)
	sc := &scenario.Scenario{
		Name:   "test-scenario",
		Repo:   repoPath,
		Commit: commit,
		Classification: scenario.Classification{
			Type: "build-package",
		},
		Agent: scenario.AgentConfig{
			Instructions: "do stuff",
		},
		Setup: &scenario.SetupConfig{
			Exec: scenario.StringList{"exit 7"},
		},
	}

	workspacePath := filepath.Join(t.TempDir(), "workspace")
	err := setup.Run(ctx, printer, scenarioName, workspacePath, sc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "setup exec")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
	return string(out)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func cleanupScenarioDir(t *testing.T, dir string) {
	t.Helper()
	_ = os.RemoveAll(dir)
	parent := filepath.Dir(dir)
	if entries, err := os.ReadDir(parent); err == nil && len(entries) == 0 {
		_ = os.Remove(parent)
	}
}
