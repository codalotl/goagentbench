package agents

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/codalotl/goagentbench/internal/output"
)

type crushAgent struct {
	ctx     context.Context
	printer *output.Printer
}

// crushProviderForLLM maps llms.yml names to Crush providers.
var crushProviderForLLM = map[string]string{
	"gpt-5.1-codex-high":      "openai",
	"grok-code-fast-1":        "xai",
	"grok-4-1-fast-reasoning": "xai",
	"grok-4":                  "xai",
}

// crushNoReasoningEffort lists models that error if reasoning_effort is set.
var crushNoReasoningEffort = map[string]struct{}{
	"grok-4-1-fast-reasoning": {},
}

func newCrushAgent(ctx context.Context, printer *output.Printer) Agent {
	return &crushAgent{
		ctx:     ctx,
		printer: printer,
	}
}

func (c *crushAgent) Version() (string, error) {
	return crushVersion(c.ctx)
}

func (c *crushAgent) Run(cwd string, llm LLMDefinition, session string, instructions string) RunResults {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return RunResults{Err: errors.New("instructions are required for crush")}
	}

	provider, ok := crushProviderForLLM[strings.TrimSpace(llm.Name)]
	if !ok || strings.TrimSpace(provider) == "" {
		return RunResults{Err: fmt.Errorf("no crush provider configured for model %q", llm.Name)}
	}

	model := strings.TrimSpace(llm.Model)
	if model == "" {
		return RunResults{Err: errors.New("model is required for crush")}
	}

	reasoning := crushReasoningEffortForLLM(llm)

	if err := writeCrushConfig(cwd, provider, model, reasoning); err != nil {
		return RunResults{Err: err}
	}

	absScenarioDir, absErr := filepath.Abs(cwd)
	if absErr != nil {
		absScenarioDir = cwd
	}
	dataDir := filepath.Join(absScenarioDir, ".crush")

	// NOTE: -y/--yolo doesn't work. It seems run automatically enables auto-approve mode.
	args := []string{"-D", dataDir, "run", "-q", trimmedInstructions}
	var outputBytes []byte
	var err error
	if c.printer != nil {
		outputBytes, err = c.printer.RunCommandStreaming(c.ctx, cwd, "crush", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "crush", args...)
		cmd.Dir = cwd
		outputBytes, err = cmd.CombinedOutput()
	}

	inputTokens, outputTokens, cost := crushReadLatestSessionUsage(c.ctx, cwd)

	res := RunResults{
		Transcript:   string(outputBytes),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Cost:         cost,
		Session:      "",
	}
	if err != nil {
		res.Err = err
	}
	return res
}

type crushConfig struct {
	Models map[string]crushModelConfig `json:"models"`
	LSP    map[string]crushLSPConfig   `json:"lsp"`
}

type crushModelConfig struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type crushLSPConfig struct {
	Command string `json:"command"`
}

func crushReasoningEffortForLLM(llm LLMDefinition) string {
	if !crushSupportsReasoningEffort(llm.Name, llm.Model) {
		return ""
	}
	reasoning := strings.TrimSpace(llm.ReasoningLevel)
	if reasoning == "" {
		reasoning = "high"
	}
	return reasoning
}

func crushSupportsReasoningEffort(name, model string) bool {
	name = strings.TrimSpace(name)
	model = strings.TrimSpace(model)
	if _, ok := crushNoReasoningEffort[name]; ok {
		return false
	}
	if _, ok := crushNoReasoningEffort[model]; ok {
		return false
	}
	return true
}

func writeCrushConfig(cwd, provider, model, reasoning string) error {
	cfg := crushConfig{
		Models: map[string]crushModelConfig{
			"large": {
				Provider:        provider,
				Model:           model,
				ReasoningEffort: reasoning,
			},
			"small": {
				Provider:        provider,
				Model:           model,
				ReasoningEffort: reasoning,
			},
		},
		LSP: map[string]crushLSPConfig{
			"go": {
				Command: "gopls",
			},
		},
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(cwd, ".crush.json")
	return os.WriteFile(path, b, 0o644)
}

func crushReadLatestSessionUsage(ctx context.Context, cwd string) (int, int, float64) {
	dbPath := filepath.Join(cwd, ".crush", "crush.db")
	if _, err := os.Stat(dbPath); err != nil {
		return 0, 0, 0
	}

	query := "select prompt_tokens, completion_tokens, cost from sessions order by updated_at desc limit 1;"
	cmd := exec.CommandContext(ctx, "sqlite3", "-csv", dbPath, query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, 0
	}

	input, output, cost, ok := parseCrushUsageCSV(string(out))
	if !ok {
		return 0, 0, 0
	}
	return input, output, cost
}

func parseCrushUsageCSV(raw string) (int, int, float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, false
	}
	reader := csv.NewReader(strings.NewReader(raw))
	reader.FieldsPerRecord = -1
	record, err := reader.Read()
	if err != nil || len(record) < 2 {
		return 0, 0, 0, false
	}
	inputTokens, err1 := strconv.Atoi(strings.TrimSpace(record[0]))
	outputTokens, err2 := strconv.Atoi(strings.TrimSpace(record[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, 0, false
	}
	var cost float64
	if len(record) > 2 {
		if parsedCost, err := strconv.ParseFloat(strings.TrimSpace(record[2]), 64); err == nil {
			cost = parsedCost
		}
	}
	return inputTokens, outputTokens, cost, true
}

var crushVersionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:[-\w\.]+)?)`)

func crushVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "crush", "-v")
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if version := parseCrushVersion(trimmed); version != "" {
		return version, nil
	}
	if err != nil {
		return "", err
	}
	if trimmed == "" {
		return "", errors.New("crush -v returned no output")
	}
	return "", errors.New("could not parse crush version")
}

func parseCrushVersion(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	if match := crushVersionPattern.FindStringSubmatch(output); len(match) > 1 {
		return match[1]
	}
	fields := strings.Fields(output)
	if len(fields) == 1 {
		return strings.TrimPrefix(fields[0], "v")
	}
	return ""
}
