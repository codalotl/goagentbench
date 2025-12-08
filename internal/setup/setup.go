package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codalotl/goagentbench/internal/fsutil"
	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/workspace"
)

// Run performs the setup for a scenario: clone repo at commit and apply setup copy steps.
func Run(ctx context.Context, printer *output.Printer, scenarioName, workspacePath string, sc *scenario.Scenario) error {
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
	if err := printer.Appf("Cloning %s into %s", repoURL, targetDir); err != nil {
		return err
	}
	if _, err := printer.RunCommand(ctx, "", "git", "clone", repoURL, targetDir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	if err := printer.Appf("Checking out %s", sc.Commit); err != nil {
		return err
	}
	if _, err := printer.RunCommand(ctx, targetDir, "git", "checkout", sc.Commit); err != nil {
		return fmt.Errorf("git checkout %s failed: %w", sc.Commit, err)
	}
	if sc.Setup != nil {
		for _, c := range sc.Setup.Copy {
			if err := printer.Appf("Copying %s to %s", c.From, c.To); err != nil {
				return err
			}
			if err := applyCopy(targetDir, scenarioDir, c); err != nil {
				return err
			}
		}
		for _, p := range sc.Setup.Patch {
			patch := strings.TrimSpace(p)
			if err := printer.Appf("Applying patch %s", patch); err != nil {
				return err
			}
			if err := applyPatch(ctx, printer, targetDir, scenarioDir, patch); err != nil {
				return err
			}
		}
	}
	if err := printer.App("Setup complete."); err != nil {
		return err
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
	var destDir string
	if info.IsDir() {
		destDir = dst
		dst = filepath.Join(dst, filepath.Base(src))
	} else {
		destDir = filepath.Dir(dst)
	}
	undo, err := fsutil.CopyToDir(src, destDir, false)
	_ = undo
	return err
}

func applyPatch(ctx context.Context, printer *output.Printer, targetDir, scenarioDir, patch string) error {
	if patch == "" {
		return fmt.Errorf("patch name cannot be empty")
	}
	patchPath, err := fsutil.SafeJoin(scenarioDir, patch)
	if err != nil {
		return err
	}
	absPatchPath, err := filepath.Abs(patchPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPatchPath); err != nil {
		return err
	}
	if _, err := printer.RunCommand(ctx, targetDir, "git", "apply", absPatchPath); err != nil {
		return fmt.Errorf("git apply %s failed: %w", absPatchPath, err)
	}
	return nil
}
