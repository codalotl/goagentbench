package agents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunResultsToProgress_ScalesDurationSeconds(t *testing.T) {
	started := time.Unix(0, 0)
	ended := started.Add(10 * time.Second)

	progress := runResultsToProgress(
		"model-a",
		RunContext{
			ScenarioName: "scenario",
			Agent:        Definition{Name: "codex", Version: "0.0.0"},
		},
		started,
		ended,
		RunResults{ScaleDuration: 1.5},
	)

	require.InDelta(t, 15.0, progress.DurationSeconds, 1e-9)
}

func TestRunResultsToProgress_UnscaledWhenZero(t *testing.T) {
	started := time.Unix(0, 0)
	ended := started.Add(10 * time.Second)

	progress := runResultsToProgress(
		"model-a",
		RunContext{
			ScenarioName: "scenario",
			Agent:        Definition{Name: "codex", Version: "0.0.0"},
		},
		started,
		ended,
		RunResults{ScaleDuration: 0},
	)

	require.InDelta(t, 10.0, progress.DurationSeconds, 1e-9)
}
