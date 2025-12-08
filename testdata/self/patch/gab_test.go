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

func TestRun_GABAppliesPatches(t *testing.T) {

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
@@ -0,0 +1 @@q
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

	runGit := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
		return string(out)
	}

	writeFile := func(t *testing.T, path, content string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	readFile := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		return string(data)
	}

	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")
	ctx := context.Background()

	createRepo := func(t *testing.T) (string, string) {
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

	type testCase struct {
		name       string
		patchFiles map[string]string
		patches    scenario.StringList
		expectErr  bool
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
			expectErr: true,
		},
		{
			name: "patch fails to apply",
			patchFiles: map[string]string{
				"bad.patch": badPatch,
			},
			patches:   scenario.StringList{"bad.patch"},
			expectErr: true,
		},
		{
			name:      "blank patch entry",
			patches:   scenario.StringList{"   "},
			expectErr: true,
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
				_ = os.RemoveAll(scenarioDir)
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
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			targetDir := workspace.WorkspaceScenarioDir(workspacePath, scenarioName)
			require.DirExists(t, targetDir)
			tc.verify(t, targetDir)
		})
	}
}
