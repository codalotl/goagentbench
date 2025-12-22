package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/codalotl/goagentbench/internal/output"
)

type codexAgent struct {
	ctx     context.Context
	printer *output.Printer
}

const (
	codexLegacyInputCostPerToken       = 1.25 / 1_000_000
	codexLegacyCachedInputCostPerToken = 0.125 / 1_000_000
	codexLegacyOutputCostPerToken      = 10.0 / 1_000_000

	codexGPT51InputCostPerToken       = 1.15 / 1_000_000
	codexGPT51CachedInputCostPerToken = 0.13 / 1_000_000
	codexGPT51OutputCostPerToken      = 10.0 / 1_000_000

	codexGPT52InputCostPerToken       = 1.75 / 1_000_000
	codexGPT52CachedInputCostPerToken = 0.18 / 1_000_000
	codexGPT52OutputCostPerToken      = 14.0 / 1_000_000
)

func newCodexAgent(ctx context.Context, printer *output.Printer) Agent {
	return &codexAgent{
		ctx:     ctx,
		printer: printer,
	}
}

func (c *codexAgent) Version() (string, error) {
	return codexVersion(c.ctx)
}

func (c *codexAgent) Run(cwd string, llm LLMDefinition, session string, instructions string, _ RunOptions) RunResults {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return RunResults{Err: errors.New("instructions are required for codex")}
	}
	session = strings.TrimSpace(session)
	if session == "" && strings.TrimSpace(llm.Model) == "" {
		return RunResults{Err: errors.New("model is required for codex")}
	}

	args := []string{
		"exec",
		"--dangerously-bypass-approvals-and-sandbox",
		"--skip-git-repo-check",
		"--json",
	}
	if llm.ReasoningLevel != "" {
		reasoningConfig := fmt.Sprintf("model_reasoning_effort=\"%s\"", llm.ReasoningLevel)
		args = append(args, "--config", reasoningConfig)
	}
	if session != "" {
		args = append(args, "resume", session)
	} else {
		args = append(args, "--model", llm.Model)
	}
	args = append(args, "--", trimmedInstructions)
	var outputBytes []byte
	var err error
	if c.printer != nil {
		outputBytes, err = c.printer.RunCommandStreaming(c.ctx, cwd, "codex", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "codex", args...)
		cmd.Dir = cwd
		outputBytes, err = cmd.CombinedOutput()
	}
	transcript, usage, threadID := parseCodexOutput(outputBytes)
	nonCachedInputTokens := usage.inputTokens - usage.cachedTokens
	if nonCachedInputTokens < 0 {
		nonCachedInputTokens = 0
	}
	cost := calculateCodexCost(llm.Model, nonCachedInputTokens, usage.cachedTokens, usage.outputTokens)

	result := RunResults{
		Transcript:        transcript,
		InputTokens:       nonCachedInputTokens,
		CachedInputTokens: usage.cachedTokens,
		OutputTokens:      usage.outputTokens,
		Cost:              cost,
		Session:           session,
	}
	if session == "" && threadID != "" {
		result.Session = threadID
	}
	if err != nil {
		result.Err = err
	}

	return result
}

type codexUsage struct {
	inputTokens  int
	cachedTokens int
	outputTokens int
}

func parseCodexOutput(raw []byte) (string, codexUsage, string) {
	reader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	// Allow long JSON lines.
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var usage codexUsage
	var threadID string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}
		if threadID == "" {
			threadID = extractThreadID(parsed)
		}
		if u, ok := parsed["usage"]; ok {
			updateUsage(&usage, u)
		}
	}
	return string(raw), usage, threadID
}

func extractThreadID(payload map[string]any) string {
	if t, ok := payload["thread_id"].(string); ok && strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t)
	}
	// Fall back to explicit thread.started event.
	if typ, ok := payload["type"].(string); ok && typ == "thread.started" {
		if t, ok := payload["thread_id"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

func updateUsage(target *codexUsage, raw any) {
	m, ok := raw.(map[string]any)
	if !ok {
		return
	}
	if val, ok := asInt(m["input_tokens"]); ok {
		target.inputTokens = val
	}
	if val, ok := asInt(m["cached_input_tokens"]); ok {
		target.cachedTokens = val
	}
	if val, ok := asInt(m["output_tokens"]); ok {
		target.outputTokens = val
	}
}

func asInt(val any) (int, bool) {
	switch v := val.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i, true
		}
	}
	return 0, false
}

var codexVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-\w\.]+)?`)

func codexVersion(ctx context.Context) (string, error) {
	attempts := [][]string{
		{"--version"},
		{"version"},
	}
	var failures []string
	for _, args := range attempts {
		cmd := exec.CommandContext(ctx, "codex", args...)
		output, err := cmd.CombinedOutput()
		trimmed := strings.TrimSpace(string(output))
		if v := parseCodexVersion(trimmed); v != "" {
			return v, nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("codex %s: %v", strings.Join(args, " "), err))
		} else if trimmed != "" {
			failures = append(failures, fmt.Sprintf("codex %s: unexpected output %q", strings.Join(args, " "), trimmed))
		} else {
			failures = append(failures, fmt.Sprintf("codex %s: no version output", strings.Join(args, " ")))
		}
	}
	if len(failures) == 0 {
		return "", errors.New("could not determine codex version")
	}
	return "", fmt.Errorf("could not determine codex version: %s", strings.Join(failures, "; "))
}

func parseCodexVersion(output string) string {
	if output == "" {
		return ""
	}
	if match := codexVersionPattern.FindString(output); match != "" {
		return match
	}
	fields := strings.Fields(output)
	if len(fields) == 1 {
		return fields[0]
	}
	return ""
}

type codexPricing struct {
	inputCostPerToken       float64
	cachedInputCostPerToken float64
	outputCostPerToken      float64
}

func codexPricingForModel(model string) codexPricing {
	trimmed := strings.TrimSpace(model)
	switch {
	case strings.HasPrefix(trimmed, "gpt-5.2"):
		return codexPricing{
			inputCostPerToken:       codexGPT52InputCostPerToken,
			cachedInputCostPerToken: codexGPT52CachedInputCostPerToken,
			outputCostPerToken:      codexGPT52OutputCostPerToken,
		}
	case strings.HasPrefix(trimmed, "gpt-5.1"):
		return codexPricing{
			inputCostPerToken:       codexGPT51InputCostPerToken,
			cachedInputCostPerToken: codexGPT51CachedInputCostPerToken,
			outputCostPerToken:      codexGPT51OutputCostPerToken,
		}
	default:
		return codexPricing{
			inputCostPerToken:       codexLegacyInputCostPerToken,
			cachedInputCostPerToken: codexLegacyCachedInputCostPerToken,
			outputCostPerToken:      codexLegacyOutputCostPerToken,
		}
	}
}

func calculateCodexCost(model string, nonCached, cached, output int) float64 {
	return calculateCodexCostForPricing(codexPricingForModel(model), nonCached, cached, output)
}

func calculateCodexCostForPricing(pricing codexPricing, nonCached, cached, output int) float64 {
	return float64(nonCached)*pricing.inputCostPerToken +
		float64(cached)*pricing.cachedInputCostPerToken +
		float64(output)*pricing.outputCostPerToken
}
