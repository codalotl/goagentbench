package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codalotl/goagentbench/internal/agents"
	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/types"
	"github.com/codalotl/goagentbench/internal/verify"
	"github.com/codalotl/goagentbench/internal/workspace"
)

type execAllScenario struct {
	Name       string
	Definition *scenario.Scenario
}

type execAllConfig struct {
	RootPath         string
	WorkspacePath    string
	Agent            agents.Definition
	ModelName        string
	LLM              *agents.LLMDefinition
	Runs             int
	MaxInterruptions int
	Scenarios        []execAllScenario
}

type execAllRunStatus string

const (
	execAllStatusSuccess                execAllRunStatus = "success"
	execAllStatusPartial                execAllRunStatus = "partial"
	execAllStatusFailed                 execAllRunStatus = "failed"
	execAllStatusInterrupted            execAllRunStatus = "interrupted"
	execAllStatusSkippedAfterInterrupts execAllRunStatus = "skipped-after-interruptions"
)

type execAllRunRecord struct {
	ScenarioName      string
	RunNumber         int
	Status            execAllRunStatus
	RunID             string
	ReportPath        string
	InterruptionsUsed int
	LastError         string
}

type execAllSummary struct {
	AgentName            string
	ModelName            string
	ScenarioCount        int
	Runs                 int
	MaxInterruptions     int
	CompletedRuns        int
	InterruptionAttempts int
	InterruptedScenarios int
	Records              []execAllRunRecord
}

func newExecAllCmd(workspacePath string) *cobra.Command {
	var agentName string
	var modelName string
	var scenarioFilter string
	var runs int
	var maxInterruptions int

	cmd := silenceUsageAndErrors(&cobra.Command{
		Use:   "exec-all --agent=<agent> [--model=<model>] [--scenario=<scenario1,scenario2>] [--runs=3] [--max-interruptions=2]",
		Short: "Run every non-smoke scenario in sequence",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if agentName == "" {
				return fmt.Errorf("--agent is required")
			}
			if runs < 1 {
				return fmt.Errorf("invalid argument %d for --runs: must be >= 1", runs)
			}
			if maxInterruptions < 0 {
				return fmt.Errorf("invalid argument %d for --max-interruptions: must be >= 0", maxInterruptions)
			}

			printer := output.NewPrinter(os.Stdout)
			rootDir, _ := os.Getwd()
			registry, err := agents.LoadRegistry(rootDir)
			if err != nil {
				return err
			}
			agentDef, llmDef, err := registry.ValidateAgentModel(agentName, modelName)
			if err != nil {
				return err
			}

			selectedScenarios, err := parseExecAllScenarioFilter(scenarioFilter)
			if err != nil {
				return err
			}
			scenarioNames, err := discoverExecAllScenarios(selectedScenarios)
			if err != nil {
				return err
			}
			if len(scenarioNames) == 0 {
				return fmt.Errorf("no scenarios found under %s (excluding smoke)", filepath.Clean(workspace.ScenarioDir(".")))
			}
			scenarios, err := loadExecAllScenarios(scenarioNames)
			if err != nil {
				return err
			}
			if err := printer.Appf("Validated %d scenarios.", len(scenarios)); err != nil {
				return err
			}

			summary, runErr := runExecAll(ctx, printer, execAllConfig{
				RootPath:         rootDir,
				WorkspacePath:    workspacePath,
				Agent:            agentDef,
				ModelName:        resolvedModelName(modelName, llmDef),
				LLM:              llmDef,
				Runs:             runs,
				MaxInterruptions: maxInterruptions,
				Scenarios:        scenarios,
			})
			if summary != nil {
				if err := printer.App(summary.String()); err != nil {
					return err
				}
			}
			return runErr
		},
	})

	cmd.Flags().StringVar(&agentName, "agent", "", "agent to run (required)")
	cmd.Flags().StringVar(&modelName, "model", "", "model to use")
	cmd.Flags().StringVar(&scenarioFilter, "scenario", "", "comma-separated scenario subset to run")
	cmd.Flags().StringVar(&scenarioFilter, "scenarios", "", "comma-separated scenario subset to run")
	cmd.Flags().IntVar(&runs, "runs", 1, "completed runs to collect for each scenario")
	cmd.Flags().IntVar(&maxInterruptions, "max-interruptions", 2, "maximum interruptions permitted per scenario before giving up")
	return cmd
}

func parseExecAllScenarioFilter(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	seen := make(map[string]struct{})
	names := make([]string, 0, strings.Count(value, ",")+1)
	for _, part := range strings.Split(value, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			return nil, fmt.Errorf("invalid empty scenario in --scenario")
		}
		name, err := workspace.CleanScenario(candidate)
		if err != nil {
			return nil, fmt.Errorf("invalid scenario %q in --scenario: %w", candidate, err)
		}
		if isSmokeScenario(name) {
			return nil, fmt.Errorf("scenario %q in --scenario must not be under smoke/", name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func discoverExecAllScenarios(selected []string) ([]string, error) {
	root := filepath.Clean(workspace.ScenarioDir("."))
	selectedSet := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		selectedSet[name] = struct{}{}
	}

	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if isSmokeScenario(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "scenario.yml" {
			return nil
		}
		relDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		if isSmokeScenario(relDir) {
			return nil
		}
		name, err := workspace.CleanScenario(relDir)
		if err != nil {
			return err
		}
		if len(selectedSet) > 0 {
			if _, ok := selectedSet[name]; !ok {
				return nil
			}
			delete(selectedSet, name)
		}
		names = append(names, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(selectedSet) > 0 {
		missing := make([]string, 0, len(selectedSet))
		for name := range selectedSet {
			missing = append(missing, name)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("scenario(s) not found: %s", strings.Join(missing, ", "))
	}
	sort.Strings(names)
	return names, nil
}

func loadExecAllScenarios(names []string) ([]execAllScenario, error) {
	scenarios := make([]execAllScenario, 0, len(names))
	for _, name := range names {
		sc, err := scenario.Load(workspace.ScenarioFile(name))
		if err != nil {
			return nil, fmt.Errorf("load scenario %q: %w", name, err)
		}
		if err := scenario.Validate(sc, workspace.ScenarioDir(name)); err != nil {
			return nil, fmt.Errorf("validate scenario %q: %w", name, err)
		}
		scenarios = append(scenarios, execAllScenario{
			Name:       name,
			Definition: sc,
		})
	}
	return scenarios, nil
}

func runExecAll(ctx context.Context, printer *output.Printer, cfg execAllConfig) (*execAllSummary, error) {
	summary := &execAllSummary{
		AgentName:        cfg.Agent.Name,
		ModelName:        cfg.ModelName,
		ScenarioCount:    len(cfg.Scenarios),
		Runs:             cfg.Runs,
		MaxInterruptions: cfg.MaxInterruptions,
		Records:          make([]execAllRunRecord, 0, len(cfg.Scenarios)*cfg.Runs),
	}

	for _, scenarioDef := range cfg.Scenarios {
		interruptionsUsed := 0
		scenarioInterrupted := false

		for runNumber := 1; runNumber <= cfg.Runs; runNumber++ {
			if scenarioInterrupted {
				summary.Records = append(summary.Records, execAllRunRecord{
					ScenarioName: scenarioDef.Name,
					RunNumber:    runNumber,
					Status:       execAllStatusSkippedAfterInterrupts,
				})
				continue
			}

			for attempt := 1; ; attempt++ {
				if err := printer.Appf("Running scenario %s (%d/%d), attempt %d", scenarioDef.Name, runNumber, cfg.Runs, attempt); err != nil {
					return nil, err
				}

				report, attemptErr := runExecAllAttempt(ctx, printer, cfg, scenarioDef)
				if attemptErr == nil {
					summary.CompletedRuns++
					summary.Records = append(summary.Records, execAllRunRecord{
						ScenarioName: scenarioDef.Name,
						RunNumber:    runNumber,
						Status:       execAllStatusForReport(report),
						RunID:        report.RunID,
						ReportPath:   verify.ReportPath(verify.Options{ScenarioName: scenarioDef.Name, RootPath: cfg.RootPath}, report),
					})
					break
				}

				interruptionsUsed++
				summary.InterruptionAttempts++
				if interruptionsUsed >= cfg.MaxInterruptions {
					scenarioInterrupted = true
					summary.InterruptedScenarios++
					summary.Records = append(summary.Records, execAllRunRecord{
						ScenarioName:      scenarioDef.Name,
						RunNumber:         runNumber,
						Status:            execAllStatusInterrupted,
						InterruptionsUsed: interruptionsUsed,
						LastError:         attemptErr.Error(),
					})
					if err := printer.Appf("Scenario %s run %d interrupted after %d interruption(s): %v", scenarioDef.Name, runNumber, interruptionsUsed, attemptErr); err != nil {
						return nil, err
					}
					break
				}
				if err := printer.Appf("Scenario %s run %d interrupted (%d/%d): %v. Retrying.", scenarioDef.Name, runNumber, interruptionsUsed, cfg.MaxInterruptions, attemptErr); err != nil {
					return nil, err
				}
			}
		}
	}

	if summary.InterruptedScenarios > 0 {
		return summary, fmt.Errorf("exec-all completed with interruptions in %d scenario(s)", summary.InterruptedScenarios)
	}
	return summary, nil
}

func runExecAllAttempt(ctx context.Context, printer *output.Printer, cfg execAllConfig, sc execAllScenario) (*types.VerificationReport, error) {
	if err := setupRunner(ctx, printer, sc.Name, cfg.WorkspacePath, sc.Definition); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}
	if err := printer.App("Scenario setup complete."); err != nil {
		return nil, err
	}
	if err := runAgent(ctx, printer, cfg.WorkspacePath, sc.Name, cfg.Agent, cfg.ModelName, cfg.LLM, sc.Definition, false); err != nil {
		return nil, fmt.Errorf("run-agent failed: %w", err)
	}
	result, err := verifyRunner(ctx, verify.Options{
		ScenarioName:  sc.Name,
		WorkspacePath: cfg.WorkspacePath,
		RootPath:      cfg.RootPath,
		Printer:       printer,
	}, sc.Definition)
	if err != nil {
		return nil, fmt.Errorf("verify failed: %w", err)
	}
	if result == nil || result.Report == nil {
		return nil, fmt.Errorf("verify returned no report")
	}
	if err := printer.App("Verification complete."); err != nil {
		return nil, err
	}
	return result.Report, nil
}

func execAllStatusForReport(report *types.VerificationReport) execAllRunStatus {
	if report == nil {
		return execAllStatusFailed
	}
	if report.Success {
		return execAllStatusSuccess
	}
	if report.PartialScore != nil && *report.PartialScore > 0 {
		return execAllStatusPartial
	}
	return execAllStatusFailed
}

func resolvedModelName(flagValue string, llm *agents.LLMDefinition) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	if llm == nil {
		return ""
	}
	return llm.Name
}

func isSmokeScenario(name string) bool {
	if name == "" || name == "." {
		return false
	}
	parts := strings.Split(filepath.Clean(name), string(filepath.Separator))
	return len(parts) > 0 && parts[0] == "smoke"
}

func (s *execAllSummary) String() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Exec-all summary (agent=%s model=%s)\n", s.AgentName, s.ModelName)
	fmt.Fprintf(&b, "- scenarios: %d\n", s.ScenarioCount)
	fmt.Fprintf(&b, "- requested runs: %d\n", s.Runs)
	fmt.Fprintf(&b, "- max interruptions per scenario: %d\n", s.MaxInterruptions)
	fmt.Fprintf(&b, "- completed runs: %d\n", s.CompletedRuns)
	fmt.Fprintf(&b, "- interruption attempts: %d\n", s.InterruptionAttempts)
	for _, record := range s.Records {
		fmt.Fprintf(&b, "- %s run %d: %s", record.ScenarioName, record.RunNumber, record.Status)
		if record.RunID != "" {
			fmt.Fprintf(&b, " run_id=%s", record.RunID)
		}
		if record.ReportPath != "" {
			fmt.Fprintf(&b, " report=%s", record.ReportPath)
		}
		if record.InterruptionsUsed > 0 {
			fmt.Fprintf(&b, " interruptions=%d/%d", record.InterruptionsUsed, s.MaxInterruptions)
		}
		if record.LastError != "" {
			fmt.Fprintf(&b, " error=%s", oneLine(record.LastError))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func oneLine(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\n", " | ")
	return text
}
