package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/codalotl/goagentbench/internal/agents"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/setup"
	"github.com/codalotl/goagentbench/internal/types"
	"github.com/codalotl/goagentbench/internal/verify"
	"github.com/codalotl/goagentbench/internal/workspace"
)

// Execute runs the CLI.
func Execute() error {
	root := &cobra.Command{
		Use:   "goagentbench",
		Short: "Benchmark AI coding agents on Go coding tasks.",
	}
	var workspacePath string
	root.PersistentFlags().StringVar(&workspacePath, "workspace", "workspace", "workspace directory for scenarios")

	root.AddCommand(newValidateCmd(&workspacePath))
	root.AddCommand(newSetupCmd(&workspacePath))
	root.AddCommand(newRunAgentCmd(&workspacePath))
	root.AddCommand(newVerifyCmd(&workspacePath))
	return root.Execute()
}

func newValidateCmd(workspacePath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-scenario <scenario>",
		Short: "Validate a scenario definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scenarioName, err := workspace.CleanScenario(args[0])
			if err != nil {
				return err
			}
			scenarioPath := workspace.ScenarioFile(scenarioName)
			sc, err := scenario.Load(scenarioPath)
			if err != nil {
				return err
			}
			if err := scenario.Validate(sc, workspace.ScenarioDir(scenarioName)); err != nil {
				return err
			}
			fmt.Println("valid")
			return nil
		},
	}
	return cmd
}

func newSetupCmd(workspacePath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup <scenario>",
		Short: "Prepare the scenario workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			scenarioName, err := workspace.CleanScenario(args[0])
			if err != nil {
				return err
			}
			scenarioPath := workspace.ScenarioFile(scenarioName)
			sc, err := scenario.Load(scenarioPath)
			if err != nil {
				return err
			}
			return setup.Run(ctx, scenarioName, *workspacePath, sc)
		},
	}
	return cmd
}

func newRunAgentCmd(workspacePath *string) *cobra.Command {
	var agentName string
	var modelName string
	var onlyStart bool
	cmd := &cobra.Command{
		Use:   "run-agent --agent=<agent> [--model=<model>] <scenario>",
		Short: "Run an agent on a prepared scenario",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if agentName == "" {
				return fmt.Errorf("--agent is required")
			}
			scenarioName, err := workspace.CleanScenario(args[0])
			if err != nil {
				return err
			}
			rootDir, _ := os.Getwd()
			registry, err := agents.LoadRegistry(rootDir)
			if err != nil {
				return err
			}
			agentDef, err := registry.ValidateAgentModel(agentName, modelName)
			if err != nil {
				return err
			}
			scenarioPath := workspace.ScenarioFile(scenarioName)
			sc, err := scenario.Load(scenarioPath)
			if err != nil {
				return err
			}
			if err := scenario.Validate(sc, workspace.ScenarioDir(scenarioName)); err != nil {
				return err
			}
			return runAgent(ctx, *workspacePath, scenarioName, agentDef, modelName, onlyStart)
		},
	}
	cmd.Flags().StringVar(&agentName, "agent", "", "agent to run (required)")
	cmd.Flags().StringVar(&modelName, "model", "", "model to use")
	cmd.Flags().BoolVar(&onlyStart, "only-start", false, "only create .run-start.json without running agent")
	return cmd
}

func runAgent(ctx context.Context, workspacePath, scenarioName string, agentDef agents.Definition, model string, onlyStart bool) error {
	workspaceDir := workspace.WorkspaceScenarioDir(workspacePath, scenarioName)
	if _, err := os.Stat(workspaceDir); err != nil {
		return fmt.Errorf("scenario not set up at %s; run setup first", workspaceDir)
	}
	runStartPath := filepath.Join(workspaceDir, ".run-start.json")
	runProgressPath := filepath.Join(workspaceDir, ".run-progress.json")
	if _, err := os.Stat(runStartPath); err == nil {
		return fmt.Errorf("run already exists at %s", runStartPath)
	}
	if _, err := os.Stat(runProgressPath); err == nil {
		return fmt.Errorf("run already in progress at %s", runProgressPath)
	}
	runID := fmt.Sprintf("run_%d", time.Now().Unix())
	now := time.Now()
	start := types.RunStart{
		RunID:        runID,
		Scenario:     scenarioName,
		Workspace:    workspacePath,
		Agent:        agentDef.Name,
		AgentVersion: agentDef.Version,
		Model:        model,
		StartedAt:    now,
		System: types.SystemInfo{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			GoVersion: runtime.Version(),
		},
	}
	if err := writeJSON(runStartPath, start); err != nil {
		return err
	}
	if onlyStart {
		fmt.Printf("Wrote %s\n", runStartPath)
		return nil
	}

	outcome, err := agents.Run(ctx, agents.RunContext{
		ScenarioName: scenarioName,
		ScenarioPath: workspaceDir,
		Model:        model,
		Agent:        agentDef,
	})
	if err != nil {
		fmt.Printf("Agent run error: %v\n", err)
	}
	if outcome == nil || outcome.Progress == nil {
		return fmt.Errorf("agent runner returned no progress")
	}
	progress := outcome.Progress
	progress.RunID = runID
	progress.Scenario = scenarioName
	progress.Agent = agentDef.Name
	progress.AgentVersion = agentDef.Version
	progress.Model = model
	progress.StartedAt = start.StartedAt
	now = time.Now()
	progress.UpdatedAt = now
	if progress.EndedAt == nil {
		progress.EndedAt = &now
	}
	if progress.DurationSeconds == 0 && progress.EndedAt != nil {
		progress.DurationSeconds = progress.EndedAt.Sub(progress.StartedAt).Seconds()
	}
	if err := writeJSON(runProgressPath, progress); err != nil {
		return err
	}
	if outcome.Manual {
		fmt.Println("Manual agent; progress file recorded. Please run the agent manually if needed.")
	}
	fmt.Printf("Run complete. Start: %s, progress: %s\n", runStartPath, runProgressPath)
	return nil
}

func newVerifyCmd(workspacePath *string) *cobra.Command {
	var onlyReport bool
	cmd := &cobra.Command{
		Use:   "verify <scenario>",
		Short: "Verify an agent run for a scenario",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			scenarioName, err := workspace.CleanScenario(args[0])
			if err != nil {
				return err
			}
			scenarioPath := workspace.ScenarioFile(scenarioName)
			sc, err := scenario.Load(scenarioPath)
			if err != nil {
				return err
			}
			rootDir, _ := os.Getwd()
			opts := verify.Options{
				ScenarioName:  scenarioName,
				WorkspacePath: *workspacePath,
				RootPath:      rootDir,
				OnlyReport:    onlyReport,
			}
			_, err = verify.Run(ctx, opts, sc)
			return err
		},
	}
	cmd.Flags().BoolVar(&onlyReport, "only-report", false, "print report without writing results file")
	return cmd
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
