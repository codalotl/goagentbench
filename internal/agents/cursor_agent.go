package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"github.com/codalotl/goagentbench/internal/output"
)

type cursorAgent struct {
	ctx     context.Context
	printer *output.Printer
}

func newCursorAgent(ctx context.Context, printer *output.Printer) Agent {
	return &cursorAgent{
		ctx:     ctx,
		printer: printer,
	}
}

func (c *cursorAgent) Version() (string, error) {
	return cursorAgentVersion(c.ctx)
}

func (c *cursorAgent) Run(cwd string, llm LLMDefinition, session string, instructions string) RunResults {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return RunResults{Err: errors.New("instructions are required for cursor-agent")}
	}
	session = strings.TrimSpace(session)
	model := strings.TrimSpace(llm.Model)
	if session == "" && model == "" {
		return RunResults{Err: errors.New("model is required for cursor-agent")}
	}

	args := []string{
		"-p",
		"-f",
		"--output-format=stream-json",
		"--stream-partial-output",
	}
	if session != "" {
		args = append(args, "--resume", session)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, trimmedInstructions)

	var outputBytes []byte
	var err error
	if c.printer != nil {
		outputBytes, err = c.printer.RunCommandStreaming(c.ctx, cwd, "cursor-agent", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "cursor-agent", args...)
		cmd.Dir = cwd
		outputBytes, err = cmd.CombinedOutput()
	}
	transcript, parsedSession := parseCursorAgentOutput(outputBytes)

	res := RunResults{
		Transcript: transcript,
		Session:    session,
	}
	if res.Session == "" && parsedSession != "" {
		res.Session = parsedSession
	}
	if err != nil {
		res.Err = err
	}
	return res
}

func parseCursorAgentOutput(raw []byte) (string, string) {
	reader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

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
			session = extractCursorSessionID(payload)
		}
	}
	return string(raw), session
}

func extractCursorSessionID(payload map[string]any) string {
	if sid, ok := payload["session_id"].(string); ok && strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid)
	}
	if sid, ok := payload["sessionId"].(string); ok && strings.TrimSpace(sid) != "" {
		return strings.TrimSpace(sid)
	}
	return ""
}

var cursorAgentVersionPattern = regexp.MustCompile(`[0-9]{4}\.[0-9]{2}\.[0-9]{2}[-\w\.]+`)

func cursorAgentVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "cursor-agent", "-v")
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if version := parseCursorAgentVersion(trimmed); version != "" {
		return version, nil
	}
	if err != nil {
		return "", err
	}
	if trimmed == "" {
		return "", errors.New("cursor-agent -v returned no output")
	}
	return "", errors.New("could not parse cursor-agent version")
}

func parseCursorAgentVersion(output string) string {
	if output == "" {
		return ""
	}
	if match := cursorAgentVersionPattern.FindString(output); match != "" {
		return match
	}
	fields := strings.Fields(output)
	if len(fields) == 1 {
		return fields[0]
	}
	return ""
}
