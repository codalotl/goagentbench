package gab

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

func TestRunSupportsRelativeTestTargets(t *testing.T) {
	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")

	scenarioName := "gab-relative-tests"
	scenarioRoot := t.TempDir()
	t.Setenv("GOAGENTBENCH_SCENARIO_ROOT", scenarioRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(scenarioRoot, scenarioName), 0o755))

	workspaceRoot := t.TempDir()
	repoDir := filepath.Join(workspaceRoot, scenarioName)
	initGoRepo(t, repoDir)

	sc := &scenario.Scenario{
		Name:   scenarioName,
		Repo:   "github.com/example/repo",
		Commit: "1234567",
		Classification: scenario.Classification{
			Type: "build-package",
		},
		Agent: scenario.AgentConfig{
			Instructions: "do work",
		},
		Verify: scenario.VerifyConfig{
			Tests: scenario.StringList{"internal/agent"},
		},
	}
	opts := verify.Options{
		ScenarioName:  scenarioName,
		WorkspacePath: workspaceRoot,
		RootPath:      workspaceRoot,
		OnlyReport:    true,
		Printer:       output.NewPrinter(nil),
	}

	res, err := verify.Run(context.Background(), opts, sc)
	resExists := res != nil && res.Report != nil
	if err != nil || !resExists || !res.Report.Success {
		t.Fatalf("verify.Run failed; err=%v success=%v", err, resExists && res.Report.Success)
	}
}

func initGoRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")

	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/gab\n\ngo 1.24.4\n")
	writeFile(t, filepath.Join(dir, "internal/agent/agent.go"), `package agent

func Add(a, b int) int {
	return a + b
}
`)
	writeFile(t, filepath.Join(dir, "internal/agent/agent_test.go"), `package agent

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatalf("expected 5")
	}
}
`)

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
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
