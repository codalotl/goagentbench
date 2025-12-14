package cli

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codalotl/goagentbench/internal/report"
)

const (
	beginResultsMarker = "<!-- BEGIN_RESULTS -->"
	endResultsMarker   = "<!-- END_RESULTS -->"
)

func publishReport(rootDir string, rep *report.Report, command string, at time.Time) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", errors.New("rootDir is required")
	}
	if rep == nil {
		return "", errors.New("report is nil")
	}

	stamp := at.In(time.Local).Format("2006-01-02_15-04-05")
	summaryRel := filepath.Join("result_summaries", "summary_"+stamp)
	summaryDir := filepath.Join(rootDir, summaryRel)
	if err := os.MkdirAll(summaryDir, 0o755); err != nil {
		return "", err
	}

	var csvBuf bytes.Buffer
	if err := rep.WriteCSV(&csvBuf); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(summaryDir, "report.csv"), csvBuf.Bytes(), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(summaryDir, "command"), []byte(strings.TrimSpace(command)+"\n"), 0o644); err != nil {
		return "", err
	}

	table := reportMarkdownTable(rep)
	summaryLink := filepath.ToSlash(summaryRel)
	dateOnly := at.In(time.Local).Format("2006-01-02")
	resultsLine := fmt.Sprintf("Results as of %s. See [%s](%s).", dateOnly, summaryLink, summaryLink)
	replacement := strings.TrimRight(table, "\n") + "\n\n" + resultsLine + "\n"
	if err := updateReadmeResults(rootDir, replacement); err != nil {
		return "", err
	}

	return summaryRel, nil
}

func reportMarkdownTable(rep *report.Report) string {
	var b strings.Builder
	b.WriteString("| Agent | Model | Success | Avg Cost | Avg Time |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, row := range rep.Rows {
		successPct := int(math.Round(row.SuccessRate * 100))
		cost := math.Round(row.AvgCost*100) / 100
		avgCost := fmt.Sprintf("$%.2f", cost)
		avgTime := formatDurationSeconds(row.AvgTimeSeconds)
		b.WriteString(fmt.Sprintf("| %s | %s | %d%% | %s | %s |\n", row.Agent, row.Model, successPct, avgCost, avgTime))
	}
	return b.String()
}

func formatDurationSeconds(seconds float64) string {
	if seconds <= 0 {
		return "0s"
	}
	total := int64(math.Round(seconds))
	h := total / 3600
	total %= 3600
	m := total / 60
	s := total % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func updateReadmeResults(rootDir string, replacement string) error {
	readmePath := filepath.Join(rootDir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	updated, err := replaceBetweenMarkers(string(data), beginResultsMarker, endResultsMarker, replacement)
	if err != nil {
		return err
	}
	return os.WriteFile(readmePath, []byte(updated), 0o644)
}

func replaceBetweenMarkers(doc, beginMarker, endMarker, replacement string) (string, error) {
	beginIdx := strings.Index(doc, beginMarker)
	if beginIdx < 0 {
		return "", fmt.Errorf("missing marker %q", beginMarker)
	}
	beginLineEnd := strings.Index(doc[beginIdx:], "\n")
	if beginLineEnd < 0 {
		return "", errors.New("begin marker line missing newline")
	}
	insertStart := beginIdx + beginLineEnd + 1

	endIdx := strings.Index(doc, endMarker)
	if endIdx < 0 {
		return "", fmt.Errorf("missing marker %q", endMarker)
	}
	if endIdx < insertStart {
		return "", errors.New("end marker precedes begin marker")
	}

	return doc[:insertStart] + replacement + doc[endIdx:], nil
}

func formatCommandForPublish(args []string) string {
	parts := []string{"goagentbench"}
	if len(args) > 1 {
		for _, arg := range args[1:] {
			parts = append(parts, shellQuote(arg))
		}
	}
	return strings.Join(parts, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	safe := true
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == ',' || r == '=':
		default:
			safe = false
		}
		if !safe {
			break
		}
	}
	if safe {
		return arg
	}
	// POSIX shell single-quote escaping: close, escape, reopen.
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}
