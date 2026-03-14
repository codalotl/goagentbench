package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

type codexAgent struct {
	ctx     context.Context
	printer *output.Printer
}

const (
	// Determined empirically on 2026/01/15, by comparing durations from several runs with/without priority processing.
	codexChatGPTDurationScale = 1.8
	codexChatGPTPlanTypePro   = "pro"
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

	scaleDuration := codexScaleDuration(c.ctx, cwd)

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
	cost := calculateLLMCost(llm, nonCachedInputTokens, usage.cachedTokens, 0, usage.outputTokens)

	result := RunResults{
		Transcript:        transcript,
		InputTokens:       nonCachedInputTokens,
		CachedInputTokens: usage.cachedTokens,
		OutputTokens:      usage.outputTokens,
		Cost:              cost,
		ScaleDuration:     scaleDuration,
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

func codexScaleDuration(ctx context.Context, cwd string) float64 {
	cmd := exec.CommandContext(ctx, "codex", "login", "status")
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	planType := codexChatGPTPlanType()
	return codexScaleDurationFromLoginStatusOutput(string(out), planType)
}

func codexScaleDurationFromLoginStatusOutput(output string, planType string) float64 {
	// When Codex is logged in via ChatGPT, it can execute slower than wall-clock measurements suggest
	// for the same token usage; only apply the empirical normalization for ChatGPT Pro accounts.
	if strings.Contains(output, "Logged in using ChatGPT") && strings.EqualFold(strings.TrimSpace(planType), codexChatGPTPlanTypePro) {
		return codexChatGPTDurationScale
	}
	return 0
}

type codexAuthFile struct {
	Tokens codexAuthTokens `json:"tokens"`
}

type codexAuthTokens struct {
	IDToken string `json:"id_token"`
}

type codexIDTokenClaims struct {
	OpenAIAuth codexOpenAIAuthClaims `json:"https://api.openai.com/auth"`
}

type codexOpenAIAuthClaims struct {
	ChatGPTPlanType string `json:"chatgpt_plan_type"`
}

func codexChatGPTPlanType() string {
	authPath, err := codexAuthPath()
	if err != nil {
		return ""
	}
	planType, err := codexChatGPTPlanTypeFromAuthFile(authPath)
	if err != nil {
		return ""
	}
	return planType
}

func codexAuthPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

func codexChatGPTPlanTypeFromAuthFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var auth codexAuthFile
	if err := json.Unmarshal(raw, &auth); err != nil {
		return "", err
	}
	return codexChatGPTPlanTypeFromIDToken(auth.Tokens.IDToken)
}

func codexChatGPTPlanTypeFromIDToken(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", errors.New("invalid id token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var claims codexIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}
	return strings.TrimSpace(claims.OpenAIAuth.ChatGPTPlanType), nil
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
