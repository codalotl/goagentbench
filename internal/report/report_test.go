package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/types"
)

func TestRunAppliesLimitAndDedup(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scenario := "demo"

	write := func(runID string, verifiedAt time.Time) {
		t.Helper()
		rep := types.VerificationReport{
			RunID:        runID,
			Scenario:     scenario,
			Agent:        "codex",
			AgentVersion: "0.1.0",
			Model:        "gpt",
			VerifiedAt:   verifiedAt,
			Success:      true,
			Progress: &types.RunProgress{
				DurationSeconds: 10,
				TokenUsage:      types.TokenUsage{Cost: 1, Input: 5, Total: 5},
			},
		}
		writeReportFile(t, filepath.Join(root, "results", scenario), runID+"-"+verifiedAt.Format(time.RFC3339)+".verify.json", rep)
	}

	now := time.Now()
	write("run_1", now.Add(-3*time.Hour))
	write("run_2", now.Add(-2*time.Hour))
	write("run_3", now.Add(-1*time.Hour))
	// Duplicate run_id should be ignored (keep latest).
	write("run_3", now.Add(-30*time.Minute))

	rep, err := Run(Options{
		RootPath: root,
		Limit:    2,
	})
	require.NoError(t, err)
	require.Len(t, rep.Rows, 1)
	require.Equal(t, 2, rep.Rows[0].Count)
}

func TestRunDefaultsToLatestAgentVersionUnlessAll(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scenario := "demo"

	now := time.Now()
	write := func(version, runID string, verifiedAt time.Time) {
		t.Helper()
		rep := types.VerificationReport{
			RunID:        runID,
			Scenario:     scenario,
			Agent:        "codex",
			AgentVersion: version,
			Model:        "gpt",
			VerifiedAt:   verifiedAt,
			Success:      true,
			Progress: &types.RunProgress{
				DurationSeconds: 10,
				TokenUsage:      types.TokenUsage{Cost: 1, Input: 1, Total: 1},
			},
		}
		writeReportFile(t, filepath.Join(root, "results", scenario), runID+".verify.json", rep)
	}

	write("0.1.0", "run_1", now.Add(-2*time.Hour))
	write("0.2.0", "run_2", now.Add(-1*time.Hour))

	latestOnly, err := Run(Options{RootPath: root, Limit: 10})
	require.NoError(t, err)
	require.Len(t, latestOnly.Rows, 1)
	require.Equal(t, "0.2.0", latestOnly.Rows[0].AgentVersion)
	require.Equal(t, 1, latestOnly.Rows[0].Count)

	all, err := Run(Options{RootPath: root, Limit: 10, AllAgentVersions: true})
	require.NoError(t, err)
	require.Len(t, all.Rows, 1)
	require.Equal(t, "0.1.0,0.2.0", all.Rows[0].AgentVersion)
	require.Equal(t, 2, all.Rows[0].Count)
}

func TestAveragesExcludeZeroAsMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scenario := "demo"

	now := time.Now()
	rep1 := types.VerificationReport{
		RunID:        "run_1",
		Scenario:     scenario,
		Agent:        "codex",
		AgentVersion: "0.1.0",
		Model:        "gpt",
		VerifiedAt:   now.Add(-2 * time.Hour),
		Success:      true,
		Progress: &types.RunProgress{
			DurationSeconds: 10,
			TokenUsage:      types.TokenUsage{Cost: 2.5, Input: 10, Total: 10},
		},
	}
	rep2 := types.VerificationReport{
		RunID:        "run_2",
		Scenario:     scenario,
		Agent:        "codex",
		AgentVersion: "0.1.0",
		Model:        "gpt",
		VerifiedAt:   now.Add(-1 * time.Hour),
		Success:      false,
		Progress: &types.RunProgress{
			DurationSeconds: 0,
			TokenUsage:      types.TokenUsage{Cost: 0, Input: 0, Total: 0},
		},
	}

	dir := filepath.Join(root, "results", scenario)
	writeReportFile(t, dir, "one.verify.json", rep1)
	writeReportFile(t, dir, "two.verify.json", rep2)

	out, err := Run(Options{RootPath: root, Limit: 10})
	require.NoError(t, err)
	require.Len(t, out.Rows, 1)
	require.Equal(t, 2, out.Rows[0].Count)
	require.InEpsilon(t, 2.5, out.Rows[0].AvgCost, 1e-9)
	require.InEpsilon(t, 10.0, out.Rows[0].AvgTimeSeconds, 1e-9)
}

func TestWriteCSVHeaders(t *testing.T) {
	t.Parallel()

	r := &Report{
		IncludeTokens: false,
		Rows: []Row{
			{Agent: "a", Model: "m", AgentVersion: "1.0.0", UniqueScenarios: 1, Count: 1},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, r.WriteCSV(&buf))
	cr := csv.NewReader(bytes.NewReader(buf.Bytes()))
	records, err := cr.ReadAll()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(records), 1)
	require.Equal(t, []string{"agent", "model", "agent_version", "unique_scenarios", "count", "success", "partial_success_score", "success_rate", "partial_success_rate", "avg_cost", "avg_time"}, records[0])
}

func TestWriteCSVRoundsHundredths(t *testing.T) {
	t.Parallel()

	r := &Report{
		IncludeTokens: true,
		Rows: []Row{
			{
				Agent:              "a",
				Model:              "m",
				AgentVersion:       "1.0.0",
				UniqueScenarios:    1,
				Count:              3,
				Success:            2,
				PartialScoreSum:    1.234,
				SuccessRate:        2.0 / 3.0, // 0.67
				PartialSuccessRate: 0.125,     // 0.13
				AvgCost:            1.005,     // 1.01
				AvgTimeSeconds:     10.994,    // 10.99
				AvgTokInput:        3.333,     // 3.33
				AvgTokCachedInput:  4.444,     // 4.44
				AvgTokWriteCached:  5.555,     // 5.56
				AvgTokOutput:       6.666,     // 6.67
				AvgTokTotal:        7.777,     // 7.78
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, r.WriteCSV(&buf))
	cr := csv.NewReader(bytes.NewReader(buf.Bytes()))
	records, err := cr.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	row := records[1]

	// Spot-check rounded columns.
	require.Contains(t, row, "1.23") // partial_success_score
	require.Contains(t, row, "0.67") // success_rate
	require.Contains(t, row, "0.13") // partial_success_rate
	require.Contains(t, row, "1.01") // avg_cost
	require.Contains(t, row, "10.99")
	require.Contains(t, row, "5.56")
	require.Contains(t, row, "7.78")
}

func TestFormatFloat_TrimsTrailingZeros(t *testing.T) {
	t.Parallel()

	require.Equal(t, "1", formatFloat(1.0))
	require.Equal(t, "1.2", formatFloat(1.2))
	require.Equal(t, "0.5", formatFloat(0.5))
	require.Equal(t, "0", formatFloat(0.0))
	require.Equal(t, "0", formatFloat(-0.0))
	require.Equal(t, "0", formatFloat(-0.004))
	require.Equal(t, "-1.2", formatFloat(-1.2))
}

func TestRunSkipsSmokeScenario(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Now()

	writeReportFile(t, filepath.Join(root, "results", "smoke"), "smoke.verify.json", types.VerificationReport{
		RunID:        "run_smoke",
		Scenario:     "smoke",
		Agent:        "codex",
		AgentVersion: "0.1.0",
		Model:        "gpt",
		VerifiedAt:   now,
		Success:      true,
	})
	writeReportFile(t, filepath.Join(root, "results", "demo"), "demo.verify.json", types.VerificationReport{
		RunID:        "run_demo",
		Scenario:     "demo",
		Agent:        "codex",
		AgentVersion: "0.1.0",
		Model:        "gpt",
		VerifiedAt:   now,
		Success:      true,
	})

	rep, err := Run(Options{RootPath: root, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rep.Rows, 1)
	require.Equal(t, 1, rep.Rows[0].UniqueScenarios)
	require.Equal(t, 1, rep.Rows[0].Count)
}

func TestRunSortsBySuccessRateDesc(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Now()

	dir := filepath.Join(root, "results", "demo")
	writeReportFile(t, dir, "a1.verify.json", types.VerificationReport{
		RunID:        "run_a1",
		Scenario:     "demo",
		Agent:        "agent-a",
		AgentVersion: "0.1.0",
		Model:        "m",
		VerifiedAt:   now.Add(-2 * time.Hour),
		Success:      true,
	})
	writeReportFile(t, dir, "a2.verify.json", types.VerificationReport{
		RunID:        "run_a2",
		Scenario:     "demo",
		Agent:        "agent-a",
		AgentVersion: "0.1.0",
		Model:        "m",
		VerifiedAt:   now.Add(-1 * time.Hour),
		Success:      false,
	})
	writeReportFile(t, dir, "b1.verify.json", types.VerificationReport{
		RunID:        "run_b1",
		Scenario:     "demo",
		Agent:        "agent-b",
		AgentVersion: "0.1.0",
		Model:        "m",
		VerifiedAt:   now.Add(-30 * time.Minute),
		Success:      true,
	})

	rep, err := Run(Options{RootPath: root, Limit: 10, AllAgentVersions: true})
	require.NoError(t, err)
	require.Len(t, rep.Rows, 2)
	require.Equal(t, "agent-b", rep.Rows[0].Agent)
	require.Equal(t, "agent-a", rep.Rows[1].Agent)
}

func writeReportFile(t *testing.T, dir, name string, rep types.VerificationReport) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.MarshalIndent(rep, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
}
