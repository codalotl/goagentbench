package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codalotl/goagentbench/internal/report"
)

func newReportCmd() *cobra.Command {
	var scenarios string
	var agents string
	var models string
	var version string
	var limit int
	var after string
	var before string
	var latestAgentVersionOnly bool
	var allAgentVersions bool
	var includeTokens bool
	var publish bool

	cmd := silenceUsageAndErrors(&cobra.Command{
		Use:   "report",
		Short: "Aggregate results into a CSV report",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootDir, _ := os.Getwd()
			afterTime, err := parseReportDateFlag("after", after)
			if err != nil {
				return err
			}
			beforeTime, err := parseReportDateFlag("before", before)
			if err != nil {
				return err
			}

			rep, err := report.Run(report.Options{
				RootPath:               rootDir,
				Scenarios:              splitCommaList(scenarios),
				Agents:                 splitCommaList(agents),
				Models:                 splitCommaList(models),
				Version:                strings.TrimSpace(version),
				Limit:                  limit,
				After:                  afterTime,
				Before:                 beforeTime,
				LatestAgentVersionOnly: latestAgentVersionOnly && !allAgentVersions,
				IncludeTokens:          includeTokens,
			})
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := rep.WriteCSV(&buf); err != nil {
				return err
			}
			if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
				return err
			}
			if publish {
				command := formatCommandForPublish(os.Args)
				_, err := publishReport(rootDir, rep, command, time.Now())
				return err
			}
			return nil
		},
	})

	cmd.Flags().StringVar(&scenarios, "scenarios", "", "comma-separated scenario list (default: all)")
	cmd.Flags().StringVar(&agents, "agents", "", "comma-separated agent list (default: all)")
	cmd.Flags().StringVar(&models, "models", "", "comma-separated model list (default: all)")
	cmd.Flags().StringVar(&version, "version", "", "only include results with the exact agent version")
	cmd.Flags().IntVar(&limit, "limit", 1, "most recent N results per {scenario,agent,model}")
	cmd.Flags().StringVar(&after, "after", "", "only include results on/after YYYY-MM-DD (local time)")
	cmd.Flags().StringVar(&before, "before", "", "only include results on/before YYYY-MM-DD (local time)")
	cmd.Flags().BoolVar(&latestAgentVersionOnly, "latest-agent-version-only", false, "only include the newest agent version per {agent,model}")
	cmd.Flags().BoolVar(&allAgentVersions, "all-agent-versions", false, "deprecated no-op; coalescing versions is now the default")
	_ = cmd.Flags().MarkDeprecated("all-agent-versions", "coalescing versions is now the default; use --latest-agent-version-only to restore the old behavior")
	cmd.Flags().BoolVar(&includeTokens, "include-tokens", false, "include token columns in output")
	cmd.Flags().BoolVar(&publish, "publish", false, "publish report summary to result_summaries and update README.md")

	return cmd
}

func splitCommaList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseReportDateFlag(name, value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, fmt.Errorf("invalid --%s (expected YYYY-MM-DD): %w", name, err)
	}
	return &parsed, nil
}
