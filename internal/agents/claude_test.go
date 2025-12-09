package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseClaudeOutput_UsesModelUsage(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"09d8c476-46e2-45cc-a86b-3f3d3d90cdb5","model":"claude-sonnet-4-5"}`,
		`{"type":"result","subtype":"success","session_id":"09d8c476-46e2-45cc-a86b-3f3d3d90cdb5","usage":{"input_tokens":5,"cache_creation_input_tokens":10,"cache_read_input_tokens":15,"output_tokens":20},"modelUsage":{"claude-haiku-4-5-20251001":{"inputTokens":21880,"outputTokens":5458,"cacheReadInputTokens":267521,"cacheCreationInputTokens":46502},"claude-sonnet-4-5":{"inputTokens":69,"outputTokens":14708,"cacheReadInputTokens":2262209,"cacheCreationInputTokens":47359},"claude-sonnet-4-5-20250929":{"inputTokens":1000,"outputTokens":206,"cacheReadInputTokens":0,"cacheCreationInputTokens":0}},"total_cost_usd":1.0831759499999999}`,
	}, "\n")

	transcript, usage, session, cost := parseClaudeOutput([]byte(raw), "claude-sonnet-4-5")

	require.Equal(t, raw, transcript)
	require.Equal(t, "09d8c476-46e2-45cc-a86b-3f3d3d90cdb5", session)
	require.Equal(t, 1069, usage.inputTokens) // 69 + 1000 (matching model variants)
	require.Equal(t, 2262209, usage.cacheReadTokens)
	require.Equal(t, 47359, usage.cacheWriteTokens)
	require.Equal(t, 14914, usage.outputTokens)
	require.InDelta(t, 1.0831759499999999, cost, 1e-9)
}

func TestParseClaudeOutput_FallbackToLegacyUsage(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"09d8c476-46e2-45cc-a86b-3f3d3d90cdb5","model":"claude-sonnet-4-5-20250929"}`,
		`{"type":"result","subtype":"success","session_id":"09d8c476-46e2-45cc-a86b-3f3d3d90cdb5","usage":{"input_tokens":1712,"cache_creation_input_tokens":28152,"cache_read_input_tokens":125992,"output_tokens":1574}}`,
	}, "\n")

	transcript, usage, session, cost := parseClaudeOutput([]byte(raw), "")

	require.Equal(t, raw, transcript)
	require.Equal(t, "09d8c476-46e2-45cc-a86b-3f3d3d90cdb5", session)
	require.Equal(t, 1712, usage.inputTokens)
	require.Equal(t, 28152, usage.cacheWriteTokens)
	require.Equal(t, 125992, usage.cacheReadTokens)
	require.Equal(t, 1574, usage.outputTokens)
	require.Zero(t, cost)
}

func TestParseClaudeOutput_FallbackWhenNonJSON(t *testing.T) {
	raw := "plain output line"

	transcript, usage, session, cost := parseClaudeOutput([]byte(raw), "")

	require.Equal(t, raw, transcript)
	require.Zero(t, usage.inputTokens)
	require.Zero(t, usage.cacheReadTokens)
	require.Zero(t, usage.cacheWriteTokens)
	require.Zero(t, usage.outputTokens)
	require.Empty(t, session)
	require.Zero(t, cost)
}

func TestParseClaudeVersion(t *testing.T) {
	require.Equal(t, "2.0.62", parseClaudeVersion("2.0.62 (Claude Code)"))
	require.Equal(t, "2.0.62", parseClaudeVersion("claude version 2.0.62"))
}
