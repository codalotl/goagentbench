package agents

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/codalotl/goagentbench/internal/output"
)

type codalotlAgent struct {
	ctx     context.Context
	printer *output.Printer
}

func codalotlExecArgs(model string, instructions string, opts RunOptions) []string {
	args := []string{"exec", "-y"}
	if pkg := strings.TrimSpace(opts.Package); pkg != "" {
		args = append(args, fmt.Sprintf("--package=%s", pkg))
	}
	args = append(args,
		fmt.Sprintf("--model=%s", model),
		"--",
		instructions,
	)
	return args
}

func newCodalotlAgent(ctx context.Context, printer *output.Printer) Agent {
	return &codalotlAgent{
		ctx:     ctx,
		printer: printer,
	}
}

func (c *codalotlAgent) Version() (string, error) {
	return codalotlVersion(c.ctx)
}

func (c *codalotlAgent) Run(cwd string, llm LLMDefinition, session string, instructions string, opts RunOptions) RunResults {
	trimmedInstructions := strings.TrimSpace(instructions)
	if trimmedInstructions == "" {
		return RunResults{Err: errors.New("instructions are required for codalotl")}
	}

	model := strings.TrimSpace(llm.Model)
	if model == "" {
		return RunResults{Err: errors.New("model is required for codalotl")}
	}

	// Codalotl currently does not support resuming sessions. Ignore any provided session.
	_ = session

	args := codalotlExecArgs(model, trimmedInstructions, opts)

	var outputBytes []byte
	var err error
	if c.printer != nil {
		outputBytes, err = c.printer.RunCommandStreaming(c.ctx, cwd, "codalotl", args...)
	} else {
		cmd := exec.CommandContext(c.ctx, "codalotl", args...)
		cmd.Dir = cwd
		outputBytes, err = cmd.CombinedOutput()
	}

	transcript, usage := parseCodalotlOutput(outputBytes)
	cost := calculateCodexCost(model, usage.inputTokens, usage.cachedInputTokens, usage.outputTokens)
	res := RunResults{
		Transcript:        transcript,
		InputTokens:       usage.inputTokens,
		CachedInputTokens: usage.cachedInputTokens,
		OutputTokens:      usage.outputTokens,
		Cost:              cost,
		Session:           "",
	}
	if err != nil {
		res.Err = err
	}
	return res
}

type codalotlUsage struct {
	inputTokens       int
	cachedInputTokens int
	outputTokens      int
	totalTokens       int
}

var codalotlTokensPattern = regexp.MustCompile(`Tokens:\s*input=(\d+)\s+cached_input=(\d+)\s+output=(\d+)\s+total=(\d+)`)

func parseCodalotlOutput(raw []byte) (string, codalotlUsage) {
	// Keep the full transcript (other agents keep raw output as well), but scan
	// from the end to find the final token usage line.
	out := string(raw)
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if m := codalotlTokensPattern.FindStringSubmatch(line); len(m) == 5 {
			input, _ := strconv.Atoi(m[1])
			cached, _ := strconv.Atoi(m[2])
			outputTokens, _ := strconv.Atoi(m[3])
			total, _ := strconv.Atoi(m[4])
			return out, codalotlUsage{
				inputTokens:       input,
				cachedInputTokens: cached,
				outputTokens:      outputTokens,
				totalTokens:       total,
			}
		}
	}
	return out, codalotlUsage{}
}

var codalotlVersionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:[-\w\.]+)?)`)

func codalotlVersion(ctx context.Context) (string, error) {
	attempts := [][]string{
		{"version"},
		{"--version"},
	}
	var failures []string
	for _, args := range attempts {
		cmd := exec.CommandContext(ctx, "codalotl", args...)
		output, err := cmd.CombinedOutput()
		trimmed := strings.TrimSpace(string(output))
		if v := parseCodalotlVersion(trimmed); v != "" {
			return v, nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("codalotl %s: %v", strings.Join(args, " "), err))
		} else if trimmed != "" {
			failures = append(failures, fmt.Sprintf("codalotl %s: unexpected output %q", strings.Join(args, " "), trimmed))
		} else {
			failures = append(failures, fmt.Sprintf("codalotl %s: no version output", strings.Join(args, " ")))
		}
	}
	if len(failures) == 0 {
		return "", errors.New("could not determine codalotl version")
	}
	return "", fmt.Errorf("could not determine codalotl version: %s", strings.Join(failures, "; "))
}

func parseCodalotlVersion(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	if match := codalotlVersionPattern.FindStringSubmatch(output); len(match) > 1 {
		return match[1]
	}
	fields := strings.Fields(output)
	if len(fields) == 1 {
		return strings.TrimPrefix(fields[0], "v")
	}
	return ""
}
