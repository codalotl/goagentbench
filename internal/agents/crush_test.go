package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCrushVersion(t *testing.T) {
	require.Equal(t, "0.23.0", parseCrushVersion("crush v0.23.0"))
	require.Equal(t, "0.23.0", parseCrushVersion("████████\ncrush v0.23.0 (linux)\n████████"))
	require.Equal(t, "0.23.0-alpha.1", parseCrushVersion("welcome to crush v0.23.0-alpha.1!"))
	require.Equal(t, "0.23.0", parseCrushVersion("0.23.0"))
}

func TestParseCrushUsageCSV(t *testing.T) {
	in, out, cost, ok := parseCrushUsageCSV("16336,16418,0.04162375\n")
	require.True(t, ok)
	require.Equal(t, 16336, in)
	require.Equal(t, 16418, out)
	require.InDelta(t, 0.04162375, cost, 1e-9)

	in, out, cost, ok = parseCrushUsageCSV("5,6\n")
	require.True(t, ok)
	require.Equal(t, 5, in)
	require.Equal(t, 6, out)
	require.Zero(t, cost)

	_, _, _, ok = parseCrushUsageCSV("not,a,number")
	require.False(t, ok)
}

func TestCrushProviderMapCoversSupportedLLMs(t *testing.T) {
	root := findRepoRoot(t)
	reg, err := LoadRegistry(root)
	require.NoError(t, err)
	def, ok := reg.Agent("crush")
	require.True(t, ok)
	require.NotEmpty(t, def.SupportsLLMs)
	for _, llmName := range def.SupportsLLMs {
		provider, ok := crushProviderForLLM[llmName]
		require.Truef(t, ok, "missing provider mapping for crush llm %s", llmName)
		require.NotEmpty(t, provider)
		require.Truef(t, provider == "openai" || provider == "xai", "unexpected provider %s for crush llm %s", provider, llmName)
	}
}

func TestCrushReasoningEffortForLLM(t *testing.T) {
	unsupported := LLMDefinition{
		Name:           "grok-4-1-fast-reasoning",
		Model:          "grok-4-1-fast-reasoning",
		ReasoningLevel: "high",
	}
	require.Empty(t, crushReasoningEffortForLLM(unsupported))

	supportedDefault := LLMDefinition{
		Name:  "grok-4",
		Model: "grok-4",
	}
	require.Equal(t, "high", crushReasoningEffortForLLM(supportedDefault))

	supportedExplicit := LLMDefinition{
		Name:           "gpt-5.1-codex-high",
		Model:          "gpt-5.1-codex-high",
		ReasoningLevel: "medium",
	}
	require.Equal(t, "medium", crushReasoningEffortForLLM(supportedExplicit))
}

func TestWriteCrushConfigOmitsReasoningEffortWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeCrushConfig(dir, "xai", "grok-4-1-fast-reasoning", ""))
	b, err := os.ReadFile(filepath.Join(dir, ".crush.json"))
	require.NoError(t, err)
	require.NotContains(t, string(b), "reasoning_effort")
}

func findRepoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if fileExists(filepath.Join(dir, "agents.yml")) && fileExists(filepath.Join(dir, "llms.yml")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root from %s", dir)
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
