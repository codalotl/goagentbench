package agents

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mattn/go-shellwords"

	"github.com/codalotl/goagentbench/internal/types"
)

type RunContext struct {
	ScenarioName string
	ScenarioPath string
	Model        string
	Agent        Definition
}

type RunOutcome struct {
	Progress *types.RunProgress
	Manual   bool
}

// Run invokes the harness for the given agent. Some agents are manual and
// simply return a stub progress file instructing the user to run the agent.
func Run(ctx context.Context, rc RunContext) (*RunOutcome, error) {
	switch rc.Agent.Name {
	case "codex":
		return runCodex(ctx, rc)
	case "codalotl":
		return runManual(rc, "Run the codalotl agent manually in the scenario directory.")
	default:
		return runManual(rc, fmt.Sprintf("No harness for agent %q; please run manually.", rc.Agent.Name))
	}
}

func runManual(rc RunContext, note string) (*RunOutcome, error) {
	now := time.Now()
	progress := &types.RunProgress{
		RunID:           "",
		Scenario:        rc.ScenarioName,
		Agent:           rc.Agent.Name,
		AgentVersion:    rc.Agent.Version,
		Model:           rc.Model,
		StartedAt:       now,
		UpdatedAt:       now,
		EndedAt:         &now,
		DurationSeconds: 0,
		TokenUsage:      types.TokenUsage{},
		Notes:           note,
		Messages: []types.AgentMessage{
			{
				Role:      "system",
				Content:   note,
				Timestamp: now,
			},
		},
	}
	return &RunOutcome{Progress: progress, Manual: true}, nil
}

func runCodex(ctx context.Context, rc RunContext) (*RunOutcome, error) {
	cmdStr := os.Getenv("GOAGENTBENCH_CODEX_CMD")
	if strings.TrimSpace(cmdStr) == "" {
		return runManual(rc, "GOAGENTBENCH_CODEX_CMD not set; run codex manually.")
	}
	args, err := shellwords.Parse(cmdStr)
	if err != nil {
		return nil, fmt.Errorf("parse GOAGENTBENCH_CODEX_CMD: %w", err)
	}
	if len(args) == 0 {
		return runManual(rc, "GOAGENTBENCH_CODEX_CMD empty; run codex manually.")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = rc.ScenarioPath
	output, err := cmd.CombinedOutput()
	now := time.Now()
	progress := &types.RunProgress{
		RunID:           "",
		Scenario:        rc.ScenarioName,
		Agent:           rc.Agent.Name,
		AgentVersion:    rc.Agent.Version,
		Model:           rc.Model,
		StartedAt:       now,
		UpdatedAt:       now,
		EndedAt:         &now,
		DurationSeconds: 0,
		TokenUsage:      types.TokenUsage{},
		Messages: []types.AgentMessage{
			{
				Role:      "system",
				Content:   strings.TrimSpace(string(output)),
				Timestamp: now,
			},
		},
	}
	if err != nil {
		progress.Notes = fmt.Sprintf("codex command failed: %v", err)
		return &RunOutcome{Progress: progress, Manual: false}, err
	}
	return &RunOutcome{Progress: progress, Manual: false}, nil
}
