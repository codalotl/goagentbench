package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/agents"
	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/types"
	"github.com/codalotl/goagentbench/internal/verify"
)

func TestRunAgentFailsWithoutRunningVerify(t *testing.T) {
	t.Parallel()

	workspacePath := t.TempDir()
	scenarioName := "demo-scenario"
	workspaceDir := filepath.Join(workspacePath, scenarioName)
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	sc := &scenario.Scenario{
		Agent: scenario.AgentConfig{
			Instructions:                     "do something",
			AllowMultipleTurnsOnFailedVerify: true,
		},
	}
	agentDef := agents.Definition{
		Name:    "dummy",
		Version: "v0.0.1",
	}

	start := time.Now()

	origAgentRunner := agentRunner
	origVerifyRunner := verifyRunner
	t.Cleanup(func() {
		agentRunner = origAgentRunner
		verifyRunner = origVerifyRunner
	})

	agentRunner = func(ctx context.Context, rc agents.RunContext) (*agents.RunOutcome, error) {
		ended := time.Now()
		return &agents.RunOutcome{
			Progress: &types.RunProgress{
				Scenario:        rc.ScenarioName,
				Agent:           rc.Agent.Name,
				AgentVersion:    rc.Agent.Version,
				Model:           rc.ModelName,
				StartedAt:       start,
				UpdatedAt:       ended,
				EndedAt:         &ended,
				DurationSeconds: ended.Sub(start).Seconds(),
				TokenUsage: types.TokenUsage{
					Input: 1,
					Total: 1,
				},
			},
		}, errors.New("agent boom")
	}
	verifyRunner = func(ctx context.Context, opts verify.Options, sc *scenario.Scenario) (*verify.Result, error) {
		t.Fatalf("verify should not run when agent fails")
		return nil, nil
	}

	printer := output.NewPrinter(io.Discard)
	err := runAgent(context.Background(), printer, workspacePath, scenarioName, agentDef, "test-model", nil, sc, false)

	require.Error(t, err)
	require.ErrorContains(t, err, "agent run failed")

	_, statErr := os.Stat(filepath.Join(workspaceDir, ".run-progress.json"))
	require.NoError(t, statErr)
}
