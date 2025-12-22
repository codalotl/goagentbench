package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/codalotl/goagentbench/internal/output"
)

type claudeAgent struct {
	ctx     context.Context
	printer *output.Printer
}

func newClaudeAgent(ctx context.Context, printer *output.Printer) Agent {
	return &claudeAgent{
		ctx:     ctx,
		printer: printer,
	}
}

func (c *claudeAgent) Version() (string, error) {
	return claudeVersion(c.ctx)
}

func (c *claudeAgent) Run(cwd string, llm LLMDefinition, session string, instructions string, _ RunOptions) RunResults {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return RunResults{Err: errors.New("instructions are required for claude")}
	}

	session = strings.TrimSpace(session)
	model := strings.TrimSpace(llm.Model)
	if session == "" && model == "" {
		return RunResults{Err: errors.New("model is required for claude")}
	}

	args := []string{
		"-p",
		"--dangerously-skip-permissions",
		"--output-format=stream-json",
		"--verbose",
	}
	if session != "" {
		args = append(args, fmt.Sprintf("--resume=%s", session))
	}
	if model != "" {
		args = append(args, fmt.Sprintf("--model=%s", model))
	}
	args = append(args, trimmedInstructions)

	envOverride := thinkingEnvOverride(llm.ReasoningLevel)

	var outputBytes []byte
	var err error
	if c.printer != nil {
		outputBytes, err = c.printer.RunCommandStreaming(c.ctx, cwd, "claude", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "claude", args...)
		cmd.Dir = cwd
		if len(envOverride) > 0 {
			cmd.Env = append(os.Environ(), envOverride...)
		}
		outputBytes, err = cmd.CombinedOutput()
	}

	transcript, usage, parsedSession, totalCost := parseClaudeOutput(outputBytes, model)

	res := RunResults{
		Transcript:             transcript,
		InputTokens:            usage.inputTokens,
		CachedInputTokens:      usage.cacheReadTokens,
		WriteCachedInputTokens: usage.cacheWriteTokens,
		OutputTokens:           usage.outputTokens,
		Session:                session,
		Cost:                   totalCost,
	}
	if res.Session == "" && parsedSession != "" {
		res.Session = parsedSession
	}
	if err != nil {
		res.Err = err
	}
	return res
}

type claudeUsage struct {
	inputTokens      int
	cacheReadTokens  int
	cacheWriteTokens int
	outputTokens     int
}

func parseClaudeOutput(raw []byte, desiredModel string) (string, claudeUsage, string, float64) {
	reader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var usage claudeUsage
	var session string
	var totalCost float64
	var usageFromModel bool
	targetModel := normalizeClaudeModel(desiredModel)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}

		if session == "" {
			session = extractClaudeSessionID(payload)
		}

		if targetModel == "" {
			if modelName, ok := payload["model"].(string); ok && strings.TrimSpace(modelName) != "" {
				targetModel = normalizeClaudeModel(modelName)
			}
		}
		if targetModel == "" {
			if modelSlug, ok := payload["model_slug"].(string); ok && strings.TrimSpace(modelSlug) != "" {
				targetModel = normalizeClaudeModel(modelSlug)
			}
		}

		if typ, _ := payload["type"].(string); typ == "result" {
			if costVal, ok := payload["total_cost_usd"]; ok {
				if parsedCost, ok := asFloat(costVal); ok {
					totalCost = parsedCost
				}
			}

			var modelUsage any
			if mu, ok := payload["modelUsage"]; ok {
				modelUsage = mu
			} else if mu, ok := payload["model_usage"]; ok {
				modelUsage = mu
			}
			if muMap, ok := modelUsage.(map[string]any); ok {
				if targetModel == "" {
					for key := range muMap {
						targetModel = normalizeClaudeModel(key)
						if targetModel != "" {
							break
						}
					}
				}
				if targetModel != "" {
					var combined claudeUsage
					var matched bool
					for key, val := range muMap {
						if normalizeClaudeModel(key) != targetModel {
							continue
						}
						entry, ok := val.(map[string]any)
						if !ok {
							continue
						}
						accumulateClaudeModelUsage(&combined, entry)
						matched = true
					}
					if matched {
						usage = combined
						usageFromModel = true
					}
				}
			}
		}

		if !usageFromModel {
			if u, ok := payload["usage"]; ok {
				updateClaudeUsage(&usage, u)
			}
		}
	}

	return string(raw), usage, session, totalCost
}

func extractClaudeSessionID(payload map[string]any) string {
	if sid, ok := payload["session_id"].(string); ok && strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid)
	}
	if sid, ok := payload["sessionId"].(string); ok && strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid)
	}
	return ""
}

func updateClaudeUsage(target *claudeUsage, raw any) {
	m, ok := raw.(map[string]any)
	if !ok {
		return
	}
	if val, ok := asInt(m["input_tokens"]); ok {
		target.inputTokens = val
	}
	if val, ok := asInt(m["cache_read_input_tokens"]); ok {
		target.cacheReadTokens = val
	}
	if val, ok := asInt(m["cache_creation_input_tokens"]); ok {
		target.cacheWriteTokens = val
	}
	if val, ok := asInt(m["cache_write_input_tokens"]); ok {
		target.cacheWriteTokens = val
	}
	if val, ok := asInt(m["output_tokens"]); ok {
		target.outputTokens = val
	}
}

var claudeVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-\w\.]+)?`)
var claudeModelDateSuffix = regexp.MustCompile(`-\d{8}$`)

func normalizeClaudeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	return claudeModelDateSuffix.ReplaceAllString(model, "")
}

func accumulateClaudeModelUsage(target *claudeUsage, raw map[string]any) {
	if val, ok := asInt(raw["inputTokens"]); ok {
		target.inputTokens += val
	}
	if val, ok := asInt(raw["outputTokens"]); ok {
		target.outputTokens += val
	}
	if val, ok := asInt(raw["cacheReadInputTokens"]); ok {
		target.cacheReadTokens += val
	}
	if val, ok := asInt(raw["cacheCreationInputTokens"]); ok {
		target.cacheWriteTokens += val
	}
	if val, ok := asInt(raw["cacheWriteInputTokens"]); ok {
		target.cacheWriteTokens += val
	}
}

func asFloat(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func claudeVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", "-v")
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if version := parseClaudeVersion(trimmed); version != "" {
		return version, nil
	}
	if err != nil {
		return "", err
	}
	if trimmed == "" {
		return "", errors.New("claude -v returned no output")
	}
	return "", fmt.Errorf("could not parse claude version from %q", trimmed)
}

func parseClaudeVersion(output string) string {
	if output == "" {
		return ""
	}
	if match := claudeVersionPattern.FindString(output); match != "" {
		return match
	}
	fields := strings.Fields(output)
	if len(fields) == 1 {
		return fields[0]
	}
	return ""
}

func thinkingEnvOverride(reasoningLevel string) []string {
	if !strings.EqualFold(strings.TrimSpace(reasoningLevel), "high") {
		return nil
	}
	return []string{"MAX_THINKING_TOKENS=10000"}
}
