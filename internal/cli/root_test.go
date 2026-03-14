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
	"github.com/codalotl/goagentbench/internal/workspace"
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

func TestDiscoverExecAllScenariosSkipsSmokeAndSorts(t *testing.T) {
	root := t.TempDir()
	t.Setenv(workspace.EnvVarScenarioRoot, root)

	for _, rel := range []string{
		filepath.Join("zeta", "last", "scenario.yml"),
		filepath.Join("alpha", "first", "scenario.yml"),
		filepath.Join("smoke", "quick", "scenario.yml"),
		filepath.Join("smoke", "nested", "case", "scenario.yml"),
	} {
		abs := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte("name: demo\nrepo: demo\ncommit: deadbeef\nclassification:\n  type: demo\nsetup: null\nagent:\n  instructions: hi\nverify:\n  tests: []\n"), 0o644))
	}

	names, err := discoverExecAllScenarios(nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join("alpha", "first"),
		filepath.Join("zeta", "last"),
	}, names)
}

func TestDiscoverExecAllScenariosFiltersSelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv(workspace.EnvVarScenarioRoot, root)

	for _, rel := range []string{
		filepath.Join("zeta", "last", "scenario.yml"),
		filepath.Join("alpha", "first", "scenario.yml"),
		filepath.Join("beta", "middle", "scenario.yml"),
	} {
		abs := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte("name: demo\nrepo: demo\ncommit: deadbeef\nclassification:\n  type: demo\nsetup: null\nagent:\n  instructions: hi\nverify:\n  tests: []\n"), 0o644))
	}

	names, err := discoverExecAllScenarios([]string{
		filepath.Join("zeta", "last"),
		filepath.Join("alpha", "first"),
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join("alpha", "first"),
		filepath.Join("zeta", "last"),
	}, names)
}

func TestDiscoverExecAllScenariosErrorsOnMissingSelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv(workspace.EnvVarScenarioRoot, root)

	abs := filepath.Join(root, "alpha", "first", "scenario.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte("name: demo\nrepo: demo\ncommit: deadbeef\nclassification:\n  type: demo\nsetup: null\nagent:\n  instructions: hi\nverify:\n  tests: []\n"), 0o644))

	_, err := discoverExecAllScenarios([]string{
		filepath.Join("alpha", "first"),
		filepath.Join("missing", "scenario"),
	})
	require.EqualError(t, err, "scenario(s) not found: missing/scenario")
}

func TestParseExecAllScenarioFilter(t *testing.T) {
	names, err := parseExecAllScenarioFilter(" zeta/last , alpha/first , zeta/last ")
	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join("zeta", "last"),
		filepath.Join("alpha", "first"),
	}, names)
}

func TestParseExecAllScenarioFilterRejectsSmoke(t *testing.T) {
	_, err := parseExecAllScenarioFilter("smoke/quick")
	require.EqualError(t, err, `scenario "smoke/quick" in --scenario must not be under smoke/`)
}

func TestParseExecAllScenarioFilterRejectsEmptyEntry(t *testing.T) {
	_, err := parseExecAllScenarioFilter("alpha/first,")
	require.EqualError(t, err, "invalid empty scenario in --scenario")
}

func TestRunExecAllRecordsInterruptionsAndContinues(t *testing.T) {
	t.Parallel()
	runnerStubMu.Lock()
	t.Cleanup(runnerStubMu.Unlock)

	workspacePath := t.TempDir()
	rootPath := t.TempDir()

	origAgentRunner := agentRunner
	origAgentVersionChecker := agentVersionChecker
	origVerifyRunner := verifyRunner
	origSetupRunner := setupRunner
	t.Cleanup(func() {
		agentRunner = origAgentRunner
		agentVersionChecker = origAgentVersionChecker
		verifyRunner = origVerifyRunner
		setupRunner = origSetupRunner
	})

	setupCalls := map[string]int{}
	setupRunner = func(ctx context.Context, printer *output.Printer, scenarioName, workspacePath string, sc *scenario.Scenario) error {
		setupCalls[scenarioName]++
		targetDir := filepath.Join(workspacePath, scenarioName)
		if err := os.RemoveAll(targetDir); err != nil {
			return err
		}
		return os.MkdirAll(targetDir, 0o755)
	}

	agentVersionChecker = func(ctx context.Context, def agents.Definition) (string, error) {
		return def.Version, nil
	}

	agentCalls := map[string]int{}
	agentRunner = func(ctx context.Context, rc agents.RunContext) (*agents.RunOutcome, error) {
		agentCalls[rc.ScenarioName]++
		call := agentCalls[rc.ScenarioName]
		started := time.Unix(1_700_000_000+int64(call), 0)
		ended := started.Add(2 * time.Second)
		outcome := &agents.RunOutcome{
			Progress: &types.RunProgress{
				Scenario:        rc.ScenarioName,
				Agent:           rc.Agent.Name,
				AgentVersion:    rc.Agent.Version,
				Model:           rc.ModelName,
				StartedAt:       started,
				UpdatedAt:       ended,
				EndedAt:         &ended,
				DurationSeconds: ended.Sub(started).Seconds(),
				TokenUsage: types.TokenUsage{
					Input: 1,
					Total: 1,
				},
			},
		}
		if rc.ScenarioName == "alpha" && call >= 2 {
			return outcome, errors.New("transient provider error")
		}
		return outcome, nil
	}

	verifyCalls := map[string]int{}
	verifyRunner = func(ctx context.Context, opts verify.Options, sc *scenario.Scenario) (*verify.Result, error) {
		verifyCalls[opts.ScenarioName]++
		data, err := os.ReadFile(filepath.Join(opts.WorkspacePath, opts.ScenarioName, ".run-start.json"))
		require.NoError(t, err)

		var start types.RunStart
		require.NoError(t, json.Unmarshal(data, &start))

		report := &types.VerificationReport{
			RunID:        start.RunID,
			Scenario:     opts.ScenarioName,
			Agent:        start.Agent,
			AgentVersion: start.AgentVersion,
			Model:        start.Model,
			VerifiedAt:   time.Unix(1_700_100_000+int64(verifyCalls[opts.ScenarioName]), 0),
		}
		switch opts.ScenarioName {
		case "alpha":
			report.Success = true
		case "beta":
			switch verifyCalls[opts.ScenarioName] {
			case 1:
				partial := 0.5
				report.PartialScore = &partial
			case 2:
				report.Success = false
			default:
				report.Success = true
			}
		default:
			report.Success = true
		}
		return &verify.Result{Report: report}, nil
	}

	cfg := execAllConfig{
		RootPath:         rootPath,
		WorkspacePath:    workspacePath,
		Agent:            agents.Definition{Name: "dummy", Version: "v0.0.1"},
		ModelName:        "test-model",
		Runs:             3,
		MaxInterruptions: 2,
		Scenarios: []execAllScenario{
			{
				Name: "alpha",
				Definition: &scenario.Scenario{
					Agent: scenario.AgentConfig{Instructions: "solve alpha"},
				},
			},
			{
				Name: "beta",
				Definition: &scenario.Scenario{
					Agent: scenario.AgentConfig{Instructions: "solve beta"},
				},
			},
		},
	}

	summary, err := runExecAll(context.Background(), output.NewPrinter(io.Discard), cfg)
	require.Error(t, err)
	require.ErrorContains(t, err, "interruptions in 1 scenario")
	require.NotNil(t, summary)

	require.Equal(t, 2, summary.ScenarioCount)
	require.Equal(t, 3, summary.Runs)
	require.Equal(t, 2, summary.MaxInterruptions)
	require.Equal(t, 4, summary.CompletedRuns)
	require.Equal(t, 2, summary.InterruptionAttempts)
	require.Equal(t, 1, summary.InterruptedScenarios)
	require.Len(t, summary.Records, 6)

	require.Equal(t, execAllStatusSuccess, summary.Records[0].Status)
	require.Equal(t, execAllStatusInterrupted, summary.Records[1].Status)
	require.Equal(t, 2, summary.Records[1].InterruptionsUsed)
	require.Contains(t, summary.Records[1].LastError, "transient provider error")
	require.Equal(t, execAllStatusSkippedAfterInterrupts, summary.Records[2].Status)
	require.Equal(t, execAllStatusPartial, summary.Records[3].Status)
	require.Equal(t, execAllStatusFailed, summary.Records[4].Status)
	require.Equal(t, execAllStatusSuccess, summary.Records[5].Status)

	require.NotEmpty(t, summary.Records[0].RunID)
	require.NotEmpty(t, summary.Records[0].ReportPath)
	require.Empty(t, summary.Records[1].ReportPath)
	require.Empty(t, summary.Records[2].ReportPath)
	require.NotEmpty(t, summary.Records[3].ReportPath)

	require.Equal(t, 3, setupCalls["alpha"])
	require.Equal(t, 3, setupCalls["beta"])
	require.Equal(t, 1, verifyCalls["alpha"])
	require.Equal(t, 3, verifyCalls["beta"])

	summaryText := summary.String()
	require.Contains(t, summaryText, "alpha run 2: interrupted")
	require.Contains(t, summaryText, "beta run 1: partial")
	require.Contains(t, summaryText, "beta run 2: failed")
}

func TestNewRunIDIsUnique(t *testing.T) {
	t.Parallel()

	seen := map[string]struct{}{}
	for i := 0; i < 128; i++ {
		id := newRunID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate run id: %s", id)
		}
		seen[id] = struct{}{}
	}
}
