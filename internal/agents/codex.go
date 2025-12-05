package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type codexAgent struct {
	ctx     context.Context
	version string
}

func newCodexAgent(ctx context.Context, version string) Agent {
	return &codexAgent{
		ctx:     ctx,
		version: version,
	}
}

func (c *codexAgent) Version() string {
	return c.version
}

func (c *codexAgent) Run(cwd string, llm LLMDefinition, session string, instructions string) RunResults {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return RunResults{Err: errors.New("instructions are required for codex")}
	}
	if strings.TrimSpace(llm.Model) == "" {
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
	args = append(args, "--model", llm.Model, "--", trimmedInstructions)
	cmd := exec.CommandContext(c.ctx, "codex", args...)
	cmd.Dir = cwd

	output, err := cmd.CombinedOutput()
	transcripts, usage := parseCodexOutput(output)

	result := RunResults{
		Transcript:        strings.TrimSpace(strings.Join(transcripts, "\n")),
		InputTokens:       usage.inputTokens,
		CachedInputTokens: usage.cachedTokens,
		OutputTokens:      usage.outputTokens,
		Session:           session,
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

func parseCodexOutput(raw []byte) ([]string, codexUsage) {
	reader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	// Allow long JSON lines.
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var transcripts []string
	var usage codexUsage
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
	return transcripts, usage
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
