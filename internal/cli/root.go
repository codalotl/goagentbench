package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codalotl/goagentbench/internal/agents"
	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/setup"
	"github.com/codalotl/goagentbench/internal/types"
	"github.com/codalotl/goagentbench/internal/verify"
	"github.com/codalotl/goagentbench/internal/workspace"
)

// These function variables allow tests to stub external dependencies.
var (
	agentRunner  = agents.Run
	verifyRunner = verify.Run
)

// Execute runs the CLI.
func Execute() error {
	root := silenceUsageAndErrors(&cobra.Command{
		Use:   "goagentbench",
		Short: "Benchmark AI coding agents on Go coding tasks.",
	})
	workspacePath := workspace.Path()

	root.AddCommand(newValidateCmd())
	root.AddCommand(newSetupCmd(workspacePath))
	root.AddCommand(newRunAgentCmd(workspacePath))
	root.AddCommand(newExecCmd(workspacePath))
	root.AddCommand(newVerifyCmd(workspacePath))
	executed, err := root.ExecuteC()
	if err != nil {
		maybePrintUsage(executed, root, err)
	}
	return err
}

func newValidateCmd() *cobra.Command {
	cmd := silenceUsageAndErrors(&cobra.Command{
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
			formatted, err := json.MarshalIndent(sc, "", "  ")
			if err != nil {
				return fmt.Errorf("format scenario: %w", err)
			}
			fmt.Println(string(formatted))
			fmt.Println("valid")
			return nil
		},
	})
	return cmd
}

func newSetupCmd(workspacePath string) *cobra.Command {
	cmd := silenceUsageAndErrors(&cobra.Command{
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
			printer := output.NewPrinter(os.Stdout)
			return setup.Run(ctx, printer, scenarioName, workspacePath, sc)
		},
	})
	return cmd
}

func newRunAgentCmd(workspacePath string) *cobra.Command {
	var agentName string
	var modelName string
	var onlyStart bool
	cmd := silenceUsageAndErrors(&cobra.Command{
		Use:   "run-agent --agent=<agent> [--model=<model>] <scenario>",
		Short: "Run an agent on a prepared scenario",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if agentName == "" {
				return fmt.Errorf("--agent is required")
			}
			printer := output.NewPrinter(os.Stdout)
			scenarioName, err := workspace.CleanScenario(args[0])
			if err != nil {
				return err
			}
			rootDir, _ := os.Getwd()
			registry, err := agents.LoadRegistry(rootDir)
			if err != nil {
				return err
			}
			agentDef, llmDef, err := registry.ValidateAgentModel(agentName, modelName)
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
			return runAgent(ctx, printer, workspacePath, scenarioName, agentDef, modelName, llmDef, sc, onlyStart)
		},
	})
	cmd.Flags().StringVar(&agentName, "agent", "", "agent to run (required)")
	cmd.Flags().StringVar(&modelName, "model", "", "model to use")
	cmd.Flags().BoolVar(&onlyStart, "only-start", false, "only create .run-start.json without running agent")
	return cmd
}

func newExecCmd(workspacePath string) *cobra.Command {
	var agentName string
	var modelName string
	cmd := silenceUsageAndErrors(&cobra.Command{
		Use:   "exec --agent=<agent> [--model=<model>] <scenario>",
		Short: "Validate, set up, run, and verify a scenario",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if agentName == "" {
				return fmt.Errorf("--agent is required")
			}
			printer := output.NewPrinter(os.Stdout)
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
			if err := printer.App("Scenario validated."); err != nil {
				return err
			}
			if err := setup.Run(ctx, printer, scenarioName, workspacePath, sc); err != nil {
				return err
			}
			if err := printer.App("Scenario setup complete."); err != nil {
				return err
			}
			rootDir, _ := os.Getwd()
			registry, err := agents.LoadRegistry(rootDir)
			if err != nil {
				return err
			}
			agentDef, llmDef, err := registry.ValidateAgentModel(agentName, modelName)
			if err != nil {
				return err
			}
			if err := runAgent(ctx, printer, workspacePath, scenarioName, agentDef, modelName, llmDef, sc, false); err != nil {
				return err
			}
			if _, err := verifyRunner(ctx, verify.Options{
				ScenarioName:  scenarioName,
				WorkspacePath: workspacePath,
				RootPath:      rootDir,
				Printer:       printer,
			}, sc); err != nil {
				return err
			}
			return printer.App("Verification complete.")
		},
	})
	cmd.Flags().StringVar(&agentName, "agent", "", "agent to run (required)")
	cmd.Flags().StringVar(&modelName, "model", "", "model to use")
	return cmd
}

func runAgent(ctx context.Context, printer *output.Printer, workspacePath, scenarioName string, agentDef agents.Definition, modelName string, llm *agents.LLMDefinition, sc *scenario.Scenario, onlyStart bool) error {
	if modelName == "" && llm != nil {
		modelName = llm.Name
	}
	rootDir, _ := os.Getwd()
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
	agentVersion := agentDef.Version
	if actualVersion, hasHarness, err := agents.AgentVersion(ctx, agentDef); err != nil {
		return fmt.Errorf("check version for agent %q: %w", agentDef.Name, err)
	} else if hasHarness {
		if actualVersion != agentDef.Version {
			return fmt.Errorf("agent %q version mismatch: expected %s, got %s", agentDef.Name, agentDef.Version, actualVersion)
		}
		agentVersion = actualVersion
		agentDef.Version = actualVersion
	}
	runID := fmt.Sprintf("run_%d", time.Now().Unix())
	now := time.Now()
	start := types.RunStart{
		RunID:        runID,
		Scenario:     scenarioName,
		Workspace:    workspacePath,
		Agent:        agentDef.Name,
		AgentVersion: agentVersion,
		Model:        modelName,
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
		return printer.Appf("Wrote %s", runStartPath)
	}

	allowContinues := sc.Agent.AllowMultipleTurnsOnFailedVerify
	maxContinues := 3
	continuesUsed := 0
	session := ""
	aggTokens := types.TokenUsage{}
	var transcripts []string
	lastNotes := ""
	lastEnded := start.StartedAt
	currentInstructions := strings.TrimSpace(sc.Agent.Instructions)

	for turn := 1; ; turn++ {
		if err := printer.Appf("Running agent %s (model=%s) turn %d", agentDef.Name, modelName, turn); err != nil {
			return err
		}
		outcome, runErr := agentRunner(ctx, agents.RunContext{
			ScenarioName: scenarioName,
			ScenarioPath: workspaceDir,
			ModelName:    modelName,
			LLM:          llm,
			Agent:        agentDef,
			Instructions: currentInstructions,
			Session:      session,
			Printer:      printer,
		})
		if runErr != nil {
			_ = printer.Appf("Agent run error: %v", runErr)
		}
		if outcome == nil || outcome.Progress == nil {
			return fmt.Errorf("agent runner returned no progress")
		}
		if outcome.Manual {
			allowContinues = false
		}

		turnProgress := outcome.Progress
		if s := strings.TrimSpace(turnProgress.Session); s != "" {
			session = s
		}
		aggTokens.Input += turnProgress.TokenUsage.Input
		aggTokens.CachedInput += turnProgress.TokenUsage.CachedInput
		aggTokens.WriteCachedInput += turnProgress.TokenUsage.WriteCachedInput
		aggTokens.Output += turnProgress.TokenUsage.Output
		aggTokens.Cost += turnProgress.TokenUsage.Cost
		aggTokens.Total = aggTokens.Input + aggTokens.CachedInput + aggTokens.WriteCachedInput + aggTokens.Output
		transcripts = append(transcripts, turnProgress.Transcripts...)
		if turnProgress.Notes != "" {
			lastNotes = turnProgress.Notes
		}
		if turnProgress.EndedAt != nil {
			lastEnded = *turnProgress.EndedAt
		} else {
			lastEnded = time.Now()
		}

		now = time.Now()
		ended := lastEnded
		progress := &types.RunProgress{
			RunID:           runID,
			Scenario:        scenarioName,
			Agent:           agentDef.Name,
			AgentVersion:    agentVersion,
			Model:           modelName,
			StartedAt:       start.StartedAt,
			UpdatedAt:       now,
			EndedAt:         &ended,
			Session:         session,
			DurationSeconds: ended.Sub(start.StartedAt).Seconds(),
			TokenUsage:      aggTokens,
			Transcripts:     transcripts,
			Notes:           lastNotes,
		}
		if err := writeJSON(runProgressPath, progress); err != nil {
			return err
		}
		if runErr != nil {
			return fmt.Errorf("agent run failed: %w", runErr)
		}
		if outcome.Manual {
			if err := printer.App("Manual agent; progress file recorded. Please run the agent manually if needed."); err != nil {
				return err
			}
			break
		}
		if !allowContinues {
			break
		}

		verRes, err := verifyRunner(ctx, verify.Options{
			ScenarioName:  scenarioName,
			WorkspacePath: workspacePath,
			RootPath:      rootDir,
			OnlyReport:    true,
			Printer:       printer,
		}, sc)
		if err != nil {
			return err
		}
		var summary string
		var success bool
		if verRes != nil && verRes.Report != nil {
			summary = verify.DetailedString(verRes.Report)
			success = verRes.Report.Success
		}
		if success {
			if err := printer.App("Verification passed; stopping."); err != nil {
				return err
			}
			break
		}
		if continuesUsed >= maxContinues {
			if err := printer.Appf("Verification failed; reached continue limit (%d).", maxContinues); err != nil {
				return err
			}
			break
		}
		continuesUsed++
		if err := printer.Appf("Verification failed; continuing (attempt %d of %d).", continuesUsed, maxContinues); err != nil {
			return err
		}
		nextPrompt := strings.TrimSpace(summary)
		if nextPrompt != "" {
			nextPrompt = fmt.Sprintf("%s\n\nPlease continue until the problem is solved.", nextPrompt)
		} else {
			nextPrompt = "Please continue until the problem is solved."
		}
		currentInstructions = nextPrompt
	}

	return printer.Appf("Run complete. Start: %s, progress: %s", runStartPath, runProgressPath)
}

func newVerifyCmd(workspacePath string) *cobra.Command {
	var onlyReport bool
	cmd := silenceUsageAndErrors(&cobra.Command{
		Use:   "verify <scenario>",
		Short: "Verify an agent run for a scenario",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			printer := output.NewPrinter(os.Stdout)
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
				WorkspacePath: workspacePath,
				RootPath:      rootDir,
				OnlyReport:    onlyReport,
				Printer:       printer,
			}
			_, err = verify.Run(ctx, opts, sc)
			return err
		},
	})
	cmd.Flags().BoolVar(&onlyReport, "only-report", false, "print report without writing results file")
	return cmd
}

func silenceUsageAndErrors(cmd *cobra.Command) *cobra.Command {
	silenceErrors(cmd)
	cmd.SilenceUsage = true
	return cmd
}

func silenceErrors(cmd *cobra.Command) *cobra.Command {
	cmd.SilenceErrors = true
	return cmd
}

func maybePrintUsage(cmd, root *cobra.Command, err error) {
	if err == nil {
		return
	}
	target := cmd
	if target == nil {
		target = root
	}
	if target == nil {
		return
	}
	if shouldShowUsage(err) {
		_ = target.Usage()
	}
}

func shouldShowUsage(err error) bool {
	msg := strings.ToLower(err.Error())
	if strings.HasPrefix(msg, "unknown command") {
		return true
	}
	if strings.HasPrefix(msg, "unknown flag") || strings.HasPrefix(msg, "unknown shorthand flag") {
		return true
	}
	if strings.Contains(msg, "accepts") && strings.Contains(msg, "arg") {
		return true
	}
	if strings.Contains(msg, "requires at least") && strings.Contains(msg, "arg") {
		return true
	}
	if strings.Contains(msg, "requires at most") && strings.Contains(msg, "arg") {
		return true
	}
	if strings.Contains(msg, "required flag") {
		return true
	}
	if strings.Contains(msg, "flag needs an argument") {
		return true
	}
	if strings.HasPrefix(msg, "invalid argument") {
		return true
	}
	return false
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
