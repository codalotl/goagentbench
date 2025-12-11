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
	"sort"
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

const resultsEnvVar = "GOAGENTBENCH_RESULTS"

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
	problems, err := checkModificationRules(sc, workspaceDir)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		report := &types.VerificationReport{
			RunID:        runID(runStart, progress),
			Scenario:     opts.ScenarioName,
			Agent:        agentName(runStart, progress),
			AgentVersion: agentVersion(runStart, progress),
			Model:        modelName(runStart, progress),
			StartedAt:    startedAt(runStart),
			Progress:     progress,
			VerifiedAt:   time.Now(),
			Success:      false,
			Tests: []types.TestResult{
				{
					Name:   "verify.modification-rules",
					Passed: false,
					Error:  strings.Join(problems, "\n"),
				},
			},
		}
		if !opts.OnlyReport {
			if err := writeReport(opts, report); err != nil {
				return nil, err
			}
		}
		printSummary(printer, report)
		return &Result{Report: report}, nil
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

var verifyIgnoredFiles = map[string]struct{}{
	".run-start.json":    {},
	".run-progress.json": {},
}

func checkModificationRules(sc *scenario.Scenario, workspaceDir string) ([]string, error) {
	changes, err := listWorkspaceChanges(workspaceDir)
	if err != nil {
		return nil, err
	}
	changes = filterIgnoredChanges(changes)
	if len(changes) == 0 {
		if len(sc.Verify.MustModify) == 0 {
			return nil, nil
		}
		return []string{"workspace has no changes but verify.must-modify requires modifications"}, nil
	}

	var problems []string
	for _, path := range changes {
		if matchesPathRule(path, sc.Verify.NoModify, workspaceDir) {
			problems = append(problems, fmt.Sprintf("%s is blocked by verify.no-modify", path))
		}
	}

	if len(sc.Verify.MustModify) > 0 {
		for _, rule := range sc.Verify.MustModify {
			if !anyChangeMatchesRule(changes, rule, workspaceDir) {
				problems = append(problems, fmt.Sprintf("%s in verify.must-modify was not modified", rule))
			}
		}
	}

	if len(problems) == 0 {
		return nil, nil
	}
	sort.Strings(problems)
	return problems, nil
}

func listWorkspaceChanges(workspaceDir string) ([]string, error) {
	cmds := [][]string{
		{"git", "diff", "--name-only", "--diff-filter=ACDMRTUXB"},
		{"git", "diff", "--name-only", "--diff-filter=ACDMRTUXB", "--cached"},
		{"git", "ls-files", "--others", "--exclude-standard"},
	}
	paths := map[string]struct{}{}
	for _, args := range cmds {
		out, err := runInWorkspace(workspaceDir, args...)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			p := strings.TrimSpace(scanner.Text())
			if p == "" {
				continue
			}
			paths[filepath.Clean(p)] = struct{}{}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	list := make([]string, 0, len(paths))
	for p := range paths {
		list = append(list, p)
	}
	sort.Strings(list)
	return list, nil
}

func runInWorkspace(workspaceDir string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command provided")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = workspaceDir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", strings.Join(cmd.Args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

func filterIgnoredChanges(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		clean := filepath.Clean(p)
		if _, ok := verifyIgnoredFiles[clean]; ok {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func anyChangeMatchesRule(changes []string, rule, workspaceDir string) bool {
	for _, path := range changes {
		if pathMatchesRule(path, rule, workspaceDir) {
			return true
		}
	}
	return false
}

func matchesPathRule(path string, rules []string, workspaceDir string) bool {
	for _, rule := range rules {
		if pathMatchesRule(path, rule, workspaceDir) {
			return true
		}
	}
	return false
}

func pathMatchesRule(path, rule, workspaceDir string) bool {
	if strings.TrimSpace(rule) == "" {
		return false
	}
	cleanPath := filepath.Clean(path)
	if strings.ContainsAny(rule, "*?[") {
		matched, err := filepath.Match(rule, cleanPath)
		return err == nil && matched
	}
	cleanRule := filepath.Clean(rule)
	if cleanPath == cleanRule {
		return true
	}
	if looksLikeDirRule(rule, workspaceDir) {
		return filepath.Dir(cleanPath) == cleanRule
	}
	return false
}

func looksLikeDirRule(rule, workspaceDir string) bool {
	if strings.HasSuffix(rule, string(filepath.Separator)) {
		return true
	}
	info, err := os.Stat(filepath.Join(workspaceDir, filepath.Clean(rule)))
	return err == nil && info.IsDir()
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
		} else {
			target := filepath.Clean(c.To)
			// If the target looks like a directory (no extension or trailing slash),
			// copy into that directory. Otherwise, treat c.To as a file path and use
			// its parent directory.
			if strings.HasSuffix(c.To, string(filepath.Separator)) || filepath.Ext(target) == "" {
				dstDir, err = fsutil.SafeJoin(workspaceDir, target)
			} else {
				dstDir, err = fsutil.SafeJoin(workspaceDir, filepath.Dir(target))
			}
		}
		if err != nil {
			return nil, err
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
	args, err := parseTestArgs(workdir, entry)
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

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func parseTestArgs(workdir, entry string) ([]string, error) {
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
	args[0] = normalizeTestTargetArg(args[0], workdir)
	return args, nil
}

func normalizeTestTargetArg(target, workdir string) string {
	if target == "" || strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../") || filepath.IsAbs(target) {
		return target
	}

	base := target
	if strings.HasSuffix(base, "/...") {
		base = strings.TrimSuffix(base, "/...")
	}
	if idx := strings.IndexAny(base, "*?["); idx >= 0 {
		base = filepath.Dir(base)
	}
	if base == "" {
		return target
	}
	if pathExists(filepath.Join(workdir, base)) {
		return "./" + target
	}
	return target
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

func progressWithoutTranscripts(progress *types.RunProgress) *types.RunProgress {
	if progress == nil {
		return nil
	}
	clone := *progress
	clone.Transcripts = nil
	return &clone
}

func reportWithoutTranscripts(report *types.VerificationReport) *types.VerificationReport {
	if report == nil {
		return nil
	}
	clone := *report
	clone.Progress = progressWithoutTranscripts(report.Progress)
	return &clone
}

func writeReport(opts Options, report *types.VerificationReport) error {
	cleanReport := reportWithoutTranscripts(report)
	filename := fmt.Sprintf("%s-%s-%s-%s.verify.json",
		cleanReport.VerifiedAt.Format("2006-01-02"),
		safePart(cleanReport.RunID, "run"),
		safePart(cleanReport.Agent, "agent"),
		safePart(cleanReport.Model, "model"))
	outDir := filepath.Join(resultsDir(opts.RootPath), opts.ScenarioName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(outDir, filename)
	data, err := json.MarshalIndent(cleanReport, "", "  ")
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

func resultsDir(rootPath string) string {
	if env := strings.TrimSpace(os.Getenv(resultsEnvVar)); env != "" {
		if filepath.IsAbs(env) {
			return filepath.Clean(env)
		}
		return filepath.Join(rootPath, filepath.Clean(env))
	}
	return filepath.Join(rootPath, "results")
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
	summary := SummaryString(report)
	if summary == "" {
		return
	}
	if printer == nil {
		fmt.Print(summary)
		return
	}
	_ = printer.App(summary)
}

// SummaryString returns a human-readable summary of the verification report.
func SummaryString(report *types.VerificationReport) string {
	if report == nil {
		return ""
	}
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("Verification for %s (agent=%s model=%s)\n", report.Scenario, report.Agent, report.Model))
	appendTest := func(prefix string, t types.TestResult) {
		status := "FAIL"
		if t.Passed {
			status = "PASS"
		}
		builder.WriteString(fmt.Sprintf("- %s%s: %s\n", prefix, t.Name, status))
		if t.Passed {
			return
		}
		if errText := strings.TrimSpace(t.Error); errText != "" {
			for _, line := range strings.Split(errText, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				builder.WriteString("  " + line + "\n")
			}
		}
	}
	for _, t := range report.Tests {
		appendTest("", t)
	}
	for _, t := range report.PartialTests {
		appendTest("partial ", t)
	}
	if report.PartialScore != nil && *report.PartialScore < 1 {
		builder.WriteString(fmt.Sprintf("Partial success: %.2f\n", *report.PartialScore))
	}
	if report.Success {
		builder.WriteString("Result: success\n")
	} else {
		builder.WriteString("Result: failure\n")
	}
	return builder.String()
}

// DetailedString returns the summary plus any available test output/error text.
func DetailedString(report *types.VerificationReport) string {
	summary := SummaryString(report)
	if report == nil {
		return summary
	}
	builder := strings.Builder{}
	if summary != "" {
		builder.WriteString(summary)
		if !strings.HasSuffix(summary, "\n") {
			builder.WriteString("\n")
		}
	}
	appendTests := func(prefix string, tests []types.TestResult) {
		for _, t := range tests {
			if t.Output == "" && t.Error == "" {
				continue
			}
			builder.WriteString(fmt.Sprintf("%s%s output:\n", prefix, t.Name))
			if t.Output != "" {
				builder.WriteString(strings.TrimSpace(t.Output))
				builder.WriteString("\n")
			}
			if t.Error != "" {
				builder.WriteString("Error: ")
				builder.WriteString(strings.TrimSpace(t.Error))
				builder.WriteString("\n")
			}
		}
	}
	appendTests("", report.Tests)
	appendTests("partial ", report.PartialTests)
	return strings.TrimSpace(builder.String())
}
