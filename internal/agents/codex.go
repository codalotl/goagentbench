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

func newCodexAgent(ctx context.Context, printer *output.Printer) Agent {
	return &codexAgent{
		ctx:     ctx,
		printer: printer,
	}
}

func (c *codexAgent) Version() (string, error) {
	return codexVersion(c.ctx)
}

func (c *codexAgent) Run(cwd string, llm LLMDefinition, session string, instructions string) RunResults {
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
	var output []byte
	var err error
	if c.printer != nil {
		output, err = c.printer.RunCommandStreaming(c.ctx, cwd, "codex", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "codex", args...)
		cmd.Dir = cwd
		output, err = cmd.CombinedOutput()
	}
	transcripts, usage, threadID := parseCodexOutput(output)

	result := RunResults{
		Transcript:        strings.TrimSpace(strings.Join(transcripts, "\n")),
		InputTokens:       usage.inputTokens,
		CachedInputTokens: usage.cachedTokens,
		OutputTokens:      usage.outputTokens,
		Session:           session,
	}
	if session == "" && threadID != "" {
		result.Session = threadID
	}
	if result.Transcript == "" {
		result.Transcript = strings.TrimSpace(string(output))
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

func parseCodexOutput(raw []byte) ([]string, codexUsage, string) {
	reader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	// Allow long JSON lines.
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var transcripts []string
	var usage codexUsage
	var threadID string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			// Non-JSON output still contributes to the transcript.
			transcripts = append(transcripts, line)
			continue
		}
		if threadID == "" {
			threadID = extractThreadID(parsed)
		}
		if u, ok := parsed["usage"]; ok {
			updateUsage(&usage, u)
		}
		if msg := extractCodexMessage(parsed); msg != "" {
			transcripts = append(transcripts, msg)
			continue
		}
		if output := extractCodexOutput(parsed); output != "" {
			transcripts = append(transcripts, output)
		}
	}
	if len(transcripts) == 0 {
		if fallback := strings.TrimSpace(string(raw)); fallback != "" {
			transcripts = []string{fallback}
		}
	}
	return transcripts, usage, threadID
}

func extractCodexMessage(payload map[string]any) string {
	if msg, ok := payload["message"]; ok {
		if text := extractTextValue(msg); text != "" {
			return text
		}
	}
	if content, ok := payload["content"]; ok {
		if text := extractTextFromBlocks(content); text != "" {
			return text
		}
	}
	return ""
}

func extractCodexOutput(payload map[string]any) string {
	if output, ok := payload["output"]; ok {
		return extractOutputText(output)
	}
	return ""
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

func extractTextValue(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case map[string]any:
		if text := extractTextFromBlocks(v["content"]); text != "" {
			return text
		}
		if text, ok := v["text"].(string); ok {
			return text
		}
	}
	return ""
}

func extractTextFromBlocks(raw any) string {
	items, ok := raw.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range items {
		if block, ok := item.(map[string]any); ok {
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

func extractOutputText(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		if output, ok := v["output"]; ok {
			if nested := extractOutputText(output); nested != "" {
				return nested
			}
		}
		if text := extractTextFromBlocks(v["content"]); text != "" {
			return text
		}
		bytes, err := json.Marshal(v)
		if err == nil {
			return string(bytes)
		}
	default:
		if v != nil {
			return fmt.Sprint(v)
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
