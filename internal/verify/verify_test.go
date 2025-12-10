package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/types"
)

func TestWriteReportOmitsTranscripts(t *testing.T) {
	tmp := t.TempDir()
	progress := &types.RunProgress{
		Scenario:        "tui_build",
		Agent:           "codex",
		AgentVersion:    "v1",
		Model:           "gpt-4",
		StartedAt:       time.Unix(0, 0),
		UpdatedAt:       time.Unix(1, 0),
		DurationSeconds: 1,
		TokenUsage: types.TokenUsage{
			Input: 10,
		},
		Transcripts: []string{"sensitive transcript"},
	}
	report := &types.VerificationReport{
		RunID:        "run_1",
		Scenario:     "tui_build",
		Agent:        "codex",
		AgentVersion: "v1",
		Model:        "gpt-4",
		VerifiedAt:   time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC),
		Success:      true,
		Tests: []types.TestResult{
			{Name: "unit", Passed: true},
		},
		Progress: progress,
	}

	err := writeReport(Options{ScenarioName: "tui_build", RootPath: tmp}, report)
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(tmp, "results", "tui_build", "*.verify.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.NotContains(t, string(data), "sensitive transcript")

	var written types.VerificationReport
	require.NoError(t, json.Unmarshal(data, &written))
	if written.Progress != nil {
		assert.Empty(t, written.Progress.Transcripts)
	}
	require.Len(t, report.Progress.Transcripts, 1)
}
