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

func TestCalculateLLMCost_UsesYAMLPricing(t *testing.T) {
	inputCost := 2.5
	cachedInputCost := 0.25
	cacheWriteInputCost := 6.25
	outputCost := 15.0
	llm := LLMDefinition{
		Name:  "gpt-5.4-high",
		Model: "gpt-5.4",
		Costs: &LLMCostDefinition{
			InputCost:           &inputCost,
			CachedInputCost:     &cachedInputCost,
			CacheWriteInputCost: &cacheWriteInputCost,
			OutputCost:          &outputCost,
		},
	}

	cost := calculateLLMCost(llm, 2_000_000, 1_000_000, 500_000, 500_000)

	require.InDelta(t, 15.875, cost, 1e-6)
}

func TestCalculateLLMCost_RequiresYAMLPricing(t *testing.T) {
	llm := LLMDefinition{
		Name:  "gpt-5.2-high",
		Model: "gpt-5.2",
	}

	cost := calculateLLMCost(llm, 2_000_000, 1_000_000, 500_000, 500_000)

	require.Zero(t, cost)
}

func TestCodexScaleDurationFromLoginStatusOutput(t *testing.T) {
	require.InDelta(t, 1.8, codexScaleDurationFromLoginStatusOutput("Logged in using ChatGPT\n"), 1e-9)
	require.Zero(t, codexScaleDurationFromLoginStatusOutput("Not logged in\n"))
	require.Zero(t, codexScaleDurationFromLoginStatusOutput(""))
}
