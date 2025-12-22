package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCodalotlOutput_ExtractsUsageFromLastLine(t *testing.T) {
	raw := strings.Join([]string{
		"some response text",
		"more text",
		`\u2022 Agent finished the turn. Tokens: input=10042 cached_input=32000 output=1043 total=43085`,
	}, "\n")
	raw = strings.ReplaceAll(raw, `\u2022`, "\u2022")

	transcript, usage := parseCodalotlOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Equal(t, 10042, usage.inputTokens)
	require.Equal(t, 32000, usage.cachedInputTokens)
	require.Equal(t, 1043, usage.outputTokens)
	require.Equal(t, 43085, usage.totalTokens)
}

func TestParseCodalotlOutput_NoUsageLine(t *testing.T) {
	raw := "some output without usage"

	_, usage := parseCodalotlOutput([]byte(raw))

	require.Equal(t, 0, usage.inputTokens)
	require.Equal(t, 0, usage.cachedInputTokens)
	require.Equal(t, 0, usage.outputTokens)
	require.Equal(t, 0, usage.totalTokens)
}

func TestParseCodalotlVersion(t *testing.T) {
	version := parseCodalotlVersion("codalotl version v0.12.3")
	require.Equal(t, "0.12.3", version)

	version = parseCodalotlVersion("v0.12.3-alpha.1")
	require.Equal(t, "0.12.3-alpha.1", version)

	version = parseCodalotlVersion("0.12.3")
	require.Equal(t, "0.12.3", version)
}

func TestCodalotlExecArgs_IncludesPackage(t *testing.T) {
	args := codalotlExecArgs("gpt-5", "do it", RunOptions{Package: "internal/cli"})
	require.Contains(t, args, "--package=internal/cli")
}

func TestCodalotlExecArgs_TrimsPackage(t *testing.T) {
	args := codalotlExecArgs("gpt-5", "do it", RunOptions{Package: "  internal/cli  "})
	require.Contains(t, args, "--package=internal/cli")
	require.NotContains(t, args, "--package=  internal/cli  ")
}
