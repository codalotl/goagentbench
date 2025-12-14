package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/report"
)

func TestPublishReportWritesFilesAndUpdatesReadme(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	readme := "# demo\n\n## Results\n\n" + beginResultsMarker + "\nold\n" + endResultsMarker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644))

	rep := &report.Report{
		IncludeTokens: false,
		Rows: []report.Row{
			{
				Agent:          "codex",
				Model:          "gpt-5",
				SuccessRate:    0.23,
				AvgCost:        1.234,
				AvgTimeSeconds: 63.2,
			},
		},
	}
	at := time.Date(2025, 12, 14, 10, 11, 12, 0, time.Local)
	stamp := at.Format("2006-01-02_15-04-05")
	dateOnly := at.Format("2006-01-02")

	summaryRel, err := publishReport(root, rep, "goagentbench report --publish", at)
	require.NoError(t, err)
	require.Equal(t, filepath.Join("result_summaries", "summary_"+stamp), summaryRel)

	summaryDir := filepath.Join(root, summaryRel)
	reportCSVPath := filepath.Join(summaryDir, "report.csv")
	commandPath := filepath.Join(summaryDir, "command")

	var wantCSV bytes.Buffer
	require.NoError(t, rep.WriteCSV(&wantCSV))
	gotCSV, err := os.ReadFile(reportCSVPath)
	require.NoError(t, err)
	require.Equal(t, wantCSV.String(), string(gotCSV))

	gotCmd, err := os.ReadFile(commandPath)
	require.NoError(t, err)
	require.Equal(t, "goagentbench report --publish\n", string(gotCmd))

	updated, err := os.ReadFile(filepath.Join(root, "README.md"))
	require.NoError(t, err)
	require.NotContains(t, string(updated), "\nold\n")
	require.Contains(t, string(updated), "| Agent | Model | Success | Avg Cost | Avg Time |")
	require.Contains(t, string(updated), "| codex | gpt-5 | 23% | $1.23 | 1m 3s |")
	require.Contains(t, string(updated), "Results as of "+dateOnly+". See [result_summaries/summary_"+stamp+"](result_summaries/summary_"+stamp+").")
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	require.Equal(t, "simple", shellQuote("simple"))
	require.Equal(t, "''", shellQuote(""))
	require.Equal(t, "'has space'", shellQuote("has space"))
	require.Equal(t, "'a'\\''b'", shellQuote("a'b"))
	require.Equal(t, "goagentbench report '--scenarios=a b'", formatCommandForPublish([]string{"/tmp/gnarly/goagentbench", "report", "--scenarios=a b"}))
	require.True(t, strings.Contains(formatCommandForPublish([]string{"/tmp/gnarly/goagentbench", "report", "--scenarios=a b"}), "'--scenarios=a b'"))
}
