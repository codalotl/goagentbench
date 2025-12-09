package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCodexOutput_RawTranscriptAndMetadata(t *testing.T) {
	raw := strings.Join([]string{
		`{"thread_id":"thread-123","usage":{"input_tokens":12,"cached_input_tokens":3,"output_tokens":7}}`,
		`{"type":"message","message":{"text":"hello"}}`,
	}, "\n")

	transcript, usage, thread := parseCodexOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Equal(t, "thread-123", thread)
	require.Equal(t, 12, usage.inputTokens)
	require.Equal(t, 3, usage.cachedTokens)
	require.Equal(t, 7, usage.outputTokens)
}

func TestParseCodexOutput_RawWhenNonJSON(t *testing.T) {
	raw := "some non json line"

	transcript, usage, thread := parseCodexOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Empty(t, thread)
	require.Zero(t, usage.inputTokens)
	require.Zero(t, usage.cachedTokens)
	require.Zero(t, usage.outputTokens)
}

func TestCalculateCodexCost(t *testing.T) {
	nonCached := 2_000_000
	cached := 1_000_000
	output := 500_000

	cost := calculateCodexCost(nonCached, cached, output)

	require.InDelta(t, 7.625, cost, 1e-6)
}

func TestCalculateCodexCostZero(t *testing.T) {
	cost := calculateCodexCost(0, 0, 0)
	require.Zero(t, cost)
}
