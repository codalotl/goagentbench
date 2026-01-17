package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/agents"
	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/codalotl/goagentbench/internal/types"
	"github.com/codalotl/goagentbench/internal/verify"
)

var runnerStubMu sync.Mutex

func TestRunAgentFailsWithoutRunningVerify(t *testing.T) {
	t.Parallel()
	runnerStubMu.Lock()
	t.Cleanup(runnerStubMu.Unlock)

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
	origAgentVersionChecker := agentVersionChecker
	origVerifyRunner := verifyRunner
	t.Cleanup(func() {
		agentRunner = origAgentRunner
		agentVersionChecker = origAgentVersionChecker
		verifyRunner = origVerifyRunner
	})

	agentVersionChecker = func(ctx context.Context, def agents.Definition) (string, error) {
		return def.Version, nil
	}
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

func TestRunAgentPersistsScaledDurationSeconds(t *testing.T) {
	t.Parallel()
	runnerStubMu.Lock()
	t.Cleanup(runnerStubMu.Unlock)

	workspacePath := t.TempDir()
	scenarioName := "demo-scenario"
	workspaceDir := filepath.Join(workspacePath, scenarioName)
	require.NoError(t, os.MkdirAll(workspaceDir, 0o755))

	sc := &scenario.Scenario{
		Agent: scenario.AgentConfig{
			Instructions: "do something",
		},
	}
	agentDef := agents.Definition{
		Name:    "dummy",
		Version: "v0.0.1",
	}

	origAgentRunner := agentRunner
	origAgentVersionChecker := agentVersionChecker
	origVerifyRunner := verifyRunner
	t.Cleanup(func() {
		agentRunner = origAgentRunner
		agentVersionChecker = origAgentVersionChecker
		verifyRunner = origVerifyRunner
	})

	agentVersionChecker = func(ctx context.Context, def agents.Definition) (string, error) {
		return def.Version, nil
	}
	verifyRunner = func(ctx context.Context, opts verify.Options, sc *scenario.Scenario) (*verify.Result, error) {
		t.Fatalf("verify should not run when multi-turn mode is disabled")
		return nil, nil
	}
	agentRunner = func(ctx context.Context, rc agents.RunContext) (*agents.RunOutcome, error) {
		data, err := os.ReadFile(filepath.Join(rc.ScenarioPath, ".run-start.json"))
		require.NoError(t, err)
		var start types.RunStart
		require.NoError(t, json.Unmarshal(data, &start))

		startedAt := start.StartedAt
		endedAt := startedAt.Add(10 * time.Second)
		unscaled := endedAt.Sub(startedAt).Seconds()

		return &agents.RunOutcome{
			Progress: &types.RunProgress{
				Scenario:        rc.ScenarioName,
				Agent:           rc.Agent.Name,
				AgentVersion:    rc.Agent.Version,
				Model:           rc.ModelName,
				StartedAt:       startedAt,
				UpdatedAt:       endedAt,
				EndedAt:         &endedAt,
				DurationSeconds: unscaled * 1.8,
				TokenUsage: types.TokenUsage{
					Input: 1,
					Total: 1,
				},
			},
		}, nil
	}

	printer := output.NewPrinter(io.Discard)
	err := runAgent(context.Background(), printer, workspacePath, scenarioName, agentDef, "test-model", nil, sc, false)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(workspaceDir, ".run-progress.json"))
	require.NoError(t, err)
	var progress types.RunProgress
	require.NoError(t, json.Unmarshal(data, &progress))
	require.InDelta(t, 18.0, progress.DurationSeconds, 1e-9)
}
