package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClaudeOutput_ExtractsSessionAndUsage(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"09d8c476-46e2-45cc-a86b-3f3d3d90cdb5","model":"claude-sonnet-4-5-20250929"}`,
		`{"type":"result","subtype":"success","session_id":"09d8c476-46e2-45cc-a86b-3f3d3d90cdb5","usage":{"input_tokens":1712,"cache_creation_input_tokens":28152,"cache_read_input_tokens":125992,"output_tokens":1574}}`,
	}, "\n")

	transcript, usage, session := parseClaudeOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Equal(t, "09d8c476-46e2-45cc-a86b-3f3d3d90cdb5", session)
	require.Equal(t, 1712, usage.inputTokens)
	require.Equal(t, 28152, usage.cacheWriteTokens)
	require.Equal(t, 125992, usage.cacheReadTokens)
	require.Equal(t, 1574, usage.outputTokens)
}

func TestParseClaudeOutput_FallbackWhenNonJSON(t *testing.T) {
	raw := "plain output line"

	transcript, usage, session := parseClaudeOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Zero(t, usage.inputTokens)
	require.Zero(t, usage.cacheReadTokens)
	require.Zero(t, usage.cacheWriteTokens)
	require.Zero(t, usage.outputTokens)
	require.Empty(t, session)
}

func TestParseClaudeVersion(t *testing.T) {
	require.Equal(t, "2.0.62", parseClaudeVersion("2.0.62 (Claude Code)"))
	require.Equal(t, "2.0.62", parseClaudeVersion("claude version 2.0.62"))
}
