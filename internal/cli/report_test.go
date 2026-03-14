package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/types"
)

func TestReportCommandFiltersByVersionFlag(t *testing.T) {
	root := t.TempDir()
	writeReportFixture(t, root, "demo", "run_1", "codex", "0.1.0", "gpt", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	writeReportFixture(t, root, "demo", "run_2", "codex", "0.2.0", "gpt", time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))

	restoreWD := chdirForTest(t, root)
	defer restoreWD()

	output := captureStdout(t, func() error {
		cmd := newReportCmd()
		cmd.SetArgs([]string{"--version=0.1.0"})
		return cmd.Execute()
	})

	require.Equal(t, "agent,model,agent_version,unique_scenarios,count,success,partial_success_score,success_rate,partial_success_rate,avg_cost,avg_time\ncodex,gpt,0.1.0,1,1,1,1,1,1,1,10\n", output)
}

func writeReportFixture(t *testing.T, root, scenario, runID, agent, version, model string, verifiedAt time.Time) {
	t.Helper()

	dir := filepath.Join(root, "results", scenario)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	rep := types.VerificationReport{
		RunID:        runID,
		Scenario:     scenario,
		Agent:        agent,
		AgentVersion: version,
		Model:        model,
		VerifiedAt:   verifiedAt,
		Success:      true,
		Progress: &types.RunProgress{
			DurationSeconds: 10,
			TokenUsage: types.TokenUsage{
				Cost:  1,
				Input: 1,
				Total: 1,
			},
		},
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, runID+".verify.json"), data, 0o644))
}

func chdirForTest(t *testing.T, dir string) func() {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	return func() {
		require.NoError(t, os.Chdir(wd))
	}
}

func captureStdout(t *testing.T, run func() error) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	runErr := run()

	require.NoError(t, w.Close())
	os.Stdout = origStdout

	data, readErr := io.ReadAll(r)
	require.NoError(t, readErr)
	require.NoError(t, r.Close())
	require.NoError(t, runErr)
	return string(data)
}
