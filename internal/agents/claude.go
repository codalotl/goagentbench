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

func (c *claudeAgent) Run(cwd string, llm LLMDefinition, session string, instructions string) RunResults {
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

	var outputBytes []byte
	var err error
	if c.printer != nil {
		outputBytes, err = c.printer.RunCommandStreaming(c.ctx, cwd, "claude", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "claude", args...)
		cmd.Dir = cwd
		outputBytes, err = cmd.CombinedOutput()
	}

	transcript, usage, parsedSession := parseClaudeOutput(outputBytes)

	res := RunResults{
		Transcript:             transcript,
		InputTokens:            usage.inputTokens,
		CachedInputTokens:      usage.cacheReadTokens,
		WriteCachedInputTokens: usage.cacheWriteTokens,
		OutputTokens:           usage.outputTokens,
		Session:                session,
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

func parseClaudeOutput(raw []byte) (string, claudeUsage, string) {
	reader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var usage claudeUsage
	var session string
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

		if u, ok := payload["usage"]; ok {
			updateClaudeUsage(&usage, u)
		}
	}

	return string(raw), usage, session
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
