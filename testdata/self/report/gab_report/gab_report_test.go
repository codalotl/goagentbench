package gab_report

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReportCLI_Golden(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := findRepoRoot(t, wd)

	cases := []struct {
		name   string
		args   []string
		golden string
	}{
		{
			name:   "default",
			args:   nil,
			golden: "report_default.csv",
		},
		{
			name:   "include_tokens",
			args:   []string{"--include-tokens"},
			golden: "report_include_tokens.csv",
		},
		{
			name:   "all_agent_versions",
			args:   []string{"--all-agent-versions"},
			golden: "report_all_agent_versions.csv",
		},
		{
			name:   "after_date",
			args:   []string{"--after=2025-12-10"},
			golden: "report_after_2025-12-10.csv",
		},
		{
			name: "filtered_limit_2",
			args: []string{
				"--scenarios=self/parse_test_file",
				"--agents=codex",
				"--models=gpt-5.1-codex-high",
				"--limit=2",
			},
			golden: "report_codex_parse_test_limit2.csv",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runReport(t, root, tc.args)
			wantPath := filepath.Join(wd, "testdata", tc.golden)
			wantBytes, err := os.ReadFile(wantPath)
			require.NoError(t, err)
			want := string(wantBytes)
			require.Equal(t, want, got)
		})
	}
}

func runReport(t *testing.T, root string, args []string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmdArgs := []string{"run", ".", "report"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "go", cmdArgs...)
	cmd.Dir = root

	cmd.Env = withEnvOverrides(os.Environ(),
		"GOAGENTBENCH_RESULTS=",
		"TZ=UTC",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoErrorf(t, err, "go run failed: %v\nstderr:\n%s", err, stderr.String())
	return stdout.String()
}

func withEnvOverrides(base []string, overrides ...string) []string {
	replace := map[string]string{}
	for _, kv := range overrides {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		replace[k] = v
	}

	out := make([]string, 0, len(base)+len(replace))
	seen := map[string]bool{}
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		if v, ok := replace[k]; ok {
			out = append(out, k+"="+v)
			seen[k] = true
			continue
		}
		out = append(out, kv)
	}
	for k, v := range replace {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func findRepoRoot(t *testing.T, start string) string {
	t.Helper()

	dir := start
	for i := 0; i < 10; i++ {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	t.Fatalf("could not find repo root from %s", start)
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
