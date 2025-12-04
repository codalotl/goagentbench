package setup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/codalotl/goagentbench/internal/fsutil"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/workspace"
)

// Run performs the setup for a scenario: clone repo at commit and apply setup copy steps.
func Run(ctx context.Context, scenarioName, workspacePath string, sc *scenario.Scenario) error {
	scenarioDir := workspace.ScenarioDir(scenarioName)
	if err := scenario.Validate(sc, scenarioDir); err != nil {
		return err
	}
	targetDir := workspace.WorkspaceScenarioDir(workspacePath, scenarioName)
	if err := os.RemoveAll(targetDir); err != nil {
		return err
	}
	if err := workspace.EnsureDir(filepath.Dir(targetDir)); err != nil {
		return err
	}
	repoURL := scenario.NormalizeRepoURL(sc.Repo)
	if err := runCmd(ctx, "", "git", "clone", repoURL, targetDir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	if err := runCmd(ctx, targetDir, "git", "checkout", sc.Commit); err != nil {
		return fmt.Errorf("git checkout %s failed: %w", sc.Commit, err)
	}
	if sc.Setup != nil {
		for _, c := range sc.Setup.Copy {
			if err := applyCopy(targetDir, scenarioDir, c); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyCopy(targetDir, scenarioDir string, step scenario.CopyStep) error {
	src := filepath.Join(scenarioDir, step.From)
	dst, err := fsutil.SafeJoin(targetDir, step.To)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		dst = filepath.Join(dst, filepath.Base(src))
	}
	_, err = fsutil.CopyPath(src, dst)
	return err
}

func runCmd(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
