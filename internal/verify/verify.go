package verify

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-shellwords"

	"github.com/codalotl/goagentbench/internal/fsutil"
	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/types"
	"github.com/codalotl/goagentbench/internal/workspace"
)

type Options struct {
	ScenarioName  string
	WorkspacePath string
	RootPath      string
	OnlyReport    bool
	Printer       *output.Printer
}

type Result struct {
	Report *types.VerificationReport
}

// Run executes verification: optional copies, go test runs, and writes report.
func Run(ctx context.Context, opts Options, sc *scenario.Scenario) (*Result, error) {
	printer := opts.Printer
	if printer == nil {
		printer = output.NewPrinter(os.Stdout)
	}
	scenarioDir := workspace.ScenarioDir(opts.ScenarioName)
	workspaceDir := workspace.WorkspaceScenarioDir(opts.WorkspacePath, opts.ScenarioName)

	if err := scenario.Validate(sc, scenarioDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(workspaceDir); err != nil {
		return nil, fmt.Errorf("workspace for scenario not found at %s", workspaceDir)
	}
	runStart, _ := readRunStart(filepath.Join(workspaceDir, ".run-start.json"))
	progress, _ := readRunProgress(filepath.Join(workspaceDir, ".run-progress.json"))
	if progress != nil && progress.RunID == "" && runStart != nil {
		progress.RunID = runStart.RunID
	}
	cleanup, err := applyVerifyCopies(sc, scenarioDir, workspaceDir)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	testResults, err := runTestList(ctx, workspaceDir, sc.Verify.Tests, printer)
	if err != nil {
		return nil, err
	}
	partialResults, partialScore, err := runPartial(ctx, workspaceDir, sc.Verify.PartialTests, printer)
	if err != nil {
		return nil, err
	}
	allRequiredPassed := allPassed(testResults)
	partialPassed := partialScore == nil || *partialScore == 1
	success := allRequiredPassed && partialPassed

	report := &types.VerificationReport{
		RunID:        runID(runStart, progress),
		Scenario:     opts.ScenarioName,
		Agent:        agentName(runStart, progress),
		AgentVersion: agentVersion(runStart, progress),
		Model:        modelName(runStart, progress),
		StartedAt:    startedAt(runStart),
		Progress:     progress,
		VerifiedAt:   time.Now(),
		Success:      success,
		PartialScore: partialScore,
		Tests:        testResults,
		PartialTests: partialResults,
	}

	if !opts.OnlyReport {
		if err := writeReport(opts, report); err != nil {
			return nil, err
		}
	}
	printSummary(printer, report)
	return &Result{Report: report}, nil
}

func applyVerifyCopies(sc *scenario.Scenario, scenarioDir, workspaceDir string) (func(), error) {
	if len(sc.Verify.Copy) == 0 {
		return func() {}, nil
	}
	undos := make([]func(), 0, len(sc.Verify.Copy))
	for _, c := range sc.Verify.Copy {
		src := filepath.Join(scenarioDir, c.From)
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		var dstDir string
		if info.IsDir() {
			dstDir, err = fsutil.SafeJoin(workspaceDir, c.To)
			if err != nil {
				return nil, err
			}
		} else {
			dstDir, err = fsutil.SafeJoin(workspaceDir, filepath.Dir(c.To))
			if err != nil {
				return nil, err
			}
		}
		undo, err := fsutil.CopyToDir(src, dstDir, true)
		if err != nil {
			return nil, err
		}
		undos = append(undos, undo)
	}
	return func() {
		for i := len(undos) - 1; i >= 0; i-- {
			if undos[i] != nil {
				undos[i]()
			}
		}
	}, nil
}

func runTestList(ctx context.Context, workdir string, entries scenario.StringList, printer *output.Printer) ([]types.TestResult, error) {
	var results []types.TestResult
	for _, entry := range entries {
		res, err := runGoTest(ctx, workdir, entry, false, printer)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func runPartial(ctx context.Context, workdir string, entries scenario.StringList, printer *output.Printer) ([]types.TestResult, *float64, error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	var results []types.TestResult
	totalTests := 0
	totalPassed := 0
	for _, entry := range entries {
		res, passed, total, err := runGoTestJSON(ctx, workdir, entry, printer)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, res)
		totalTests += total
		totalPassed += passed
	}
	var score *float64
	if totalTests > 0 {
		s := float64(totalPassed) / float64(totalTests)
		score = &s
	} else {
		zero := 0.0
		score = &zero
	}
	return results, score, nil
}

func runGoTest(ctx context.Context, workdir, entry string, forceJSON bool, printer *output.Printer) (types.TestResult, error) {
	args, err := parseTestArgs(entry)
	if err != nil {
		return types.TestResult{Name: entry, Passed: false, Error: err.Error()}, nil
	}
	cmdArgs := []string{"test"}
	if forceJSON {
		cmdArgs = append(cmdArgs, "-json")
	}
	cmdArgs = append(cmdArgs, args...)
	var outputBytes []byte
	if printer != nil {
		outputBytes, err = printer.RunCommandStreaming(ctx, workdir, "go", cmdArgs...)
	} else {
		cmd := exec.CommandContext(ctx, "go", cmdArgs...)
		cmd.Dir = workdir
		var buf bytes.Buffer
		cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
		cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
		err = cmd.Run()
		outputBytes = buf.Bytes()
	}
	result := types.TestResult{
		Name:   entry,
		Passed: err == nil,
		Output: string(outputBytes),
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result, nil
}

func runGoTestJSON(ctx context.Context, workdir, entry string, printer *output.Printer) (types.TestResult, int, int, error) {
	res, err := runGoTest(ctx, workdir, entry, true, printer)
	if err != nil {
		return types.TestResult{}, 0, 0, err
	}
	passed, total := parseJSONCounts(res.Output)
	return res, passed, total, nil
}

func parseTestArgs(entry string) ([]string, error) {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return nil, fmt.Errorf("empty test entry")
	}
	if strings.HasPrefix(trimmed, "go test") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "go test"))
	}
	args, err := shellwords.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("no args parsed from %q", entry)
	}
	return args, nil
}

type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

func parseJSONCounts(output string) (int, int) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	passed := 0
	total := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev goTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Test == "" {
			continue
		}
		switch ev.Action {
		case "pass":
			passed++
			total++
		case "fail":
			total++
		}
	}
	return passed, total
}

func runID(start *types.RunStart, prog *types.RunProgress) string {
	if start != nil && start.RunID != "" {
		return start.RunID
	}
	if prog != nil && prog.RunID != "" {
		return prog.RunID
	}
	return ""
}

func agentName(start *types.RunStart, prog *types.RunProgress) string {
	if start != nil && start.Agent != "" {
		return start.Agent
	}
	if prog != nil && prog.Agent != "" {
		return prog.Agent
	}
	return ""
}

func agentVersion(start *types.RunStart, prog *types.RunProgress) string {
	if start != nil && start.AgentVersion != "" {
		return start.AgentVersion
	}
	if prog != nil && prog.AgentVersion != "" {
		return prog.AgentVersion
	}
	return ""
}

func modelName(start *types.RunStart, prog *types.RunProgress) string {
	if start != nil && start.Model != "" {
		return start.Model
	}
	if prog != nil && prog.Model != "" {
		return prog.Model
	}
	return ""
}

func startedAt(start *types.RunStart) *time.Time {
	if start == nil {
		return nil
	}
	return &start.StartedAt
}

func readRunStart(path string) (*types.RunStart, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rs types.RunStart
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, err
	}
	return &rs, nil
}

func readRunProgress(path string) (*types.RunProgress, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rp types.RunProgress
	if err := json.Unmarshal(data, &rp); err != nil {
		return nil, err
	}
	return &rp, nil
}

func writeReport(opts Options, report *types.VerificationReport) error {
	filename := fmt.Sprintf("%s-%s-%s-%s.verify.json",
		report.VerifiedAt.Format("2006-01-02"),
		safePart(report.RunID, "run"),
		safePart(report.Agent, "agent"),
		safePart(report.Model, "model"))
	outDir := filepath.Join(opts.RootPath, "results", opts.ScenarioName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(outDir, filename)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644)
}

func safePart(value, fallback string) string {
	val := strings.TrimSpace(value)
	if val == "" {
		return fallback
	}
	val = strings.ReplaceAll(val, string(os.PathSeparator), "_")
	return val
}

func allPassed(results []types.TestResult) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}

func printSummary(printer *output.Printer, report *types.VerificationReport) {
	if report == nil {
		return
	}
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("Verification for %s (agent=%s model=%s)\n", report.Scenario, report.Agent, report.Model))
	for _, t := range report.Tests {
		status := "FAIL"
		if t.Passed {
			status = "PASS"
		}
		builder.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, status))
	}
	for _, t := range report.PartialTests {
		status := "FAIL"
		if t.Passed {
			status = "PASS"
		}
		builder.WriteString(fmt.Sprintf("- partial %s: %s\n", t.Name, status))
	}
	if report.PartialScore != nil && *report.PartialScore < 1 {
		builder.WriteString(fmt.Sprintf("Partial success: %.2f\n", *report.PartialScore))
	}
	if report.Success {
		builder.WriteString("Result: success\n")
	} else {
		builder.WriteString("Result: failure\n")
	}
	if printer == nil {
		fmt.Print(builder.String())
		return
	}
	_ = printer.App(builder.String())
}
