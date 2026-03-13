package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAgentModel_DefaultsToFirstSupported(t *testing.T) {
	reg := &Registry{
		Agents: map[string]Definition{
			"agent1": {
				Name:         "agent1",
				SupportsLLMs: []string{"llm-a", "llm-b"},
			},
		},
		LLMs: map[string]LLMDefinition{
			"llm-a": {Name: "llm-a"},
			"llm-b": {Name: "llm-b"},
		},
	}

	agent, llm, err := reg.ValidateAgentModel("agent1", "")
	require.NoError(t, err)
	require.Equal(t, "agent1", agent.Name)
	require.NotNil(t, llm)
	require.Equal(t, "llm-a", llm.Name)
}

func TestValidateAgentModel_DefaultMissingLLM(t *testing.T) {
	reg := &Registry{
		Agents: map[string]Definition{
			"agent1": {
				Name:         "agent1",
				SupportsLLMs: []string{"llm-a"},
			},
		},
		LLMs: map[string]LLMDefinition{},
	}

	_, _, err := reg.ValidateAgentModel("agent1", "")
	require.Error(t, err)
}

func TestValidateAgentModel_PerAgentOverride(t *testing.T) {
	reg := &Registry{
		Agents: map[string]Definition{
			"agent1": {
				Name:         "agent1",
				SupportsLLMs: []string{"llm-a"},
			},
		},
		LLMs: map[string]LLMDefinition{
			"llm-a": {
				Name:     "llm-a",
				Model:    "shared-model",
				PerAgent: map[string]string{"agent1": "agent1-model"},
			},
		},
	}

	_, llm, err := reg.ValidateAgentModel("agent1", "llm-a")
	require.NoError(t, err)
	require.NotNil(t, llm)
	require.Equal(t, "agent1-model", llm.Model)
}

func TestLoadRegistry_LLMCostsFromYAML(t *testing.T) {
	root := t.TempDir()

	err := os.WriteFile(filepath.Join(root, "agents.yml"), []byte(`
agents:
  - name: codex
    version: "1.0.0"
    supports-llms:
      - gpt-5.4-high
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(root, "llms.yml"), []byte(`
model-costs:
  gpt-5.4-costs: &gpt54_costs
    input-cost: 2.5
    cached-input-cost: 0.25
    cache-write-input-cost: 6.25
    output-cost: 15
llms:
  - name: gpt-5.4-high
    model: gpt-5.4
    costs: *gpt54_costs
`), 0o644)
	require.NoError(t, err)

	reg, err := LoadRegistry(root)
	require.NoError(t, err)

	llm, ok := reg.LLM("gpt-5.4-high")
	require.True(t, ok)
	require.NotNil(t, llm.Costs)
	require.True(t, llm.Costs.IsComplete())
	require.NotNil(t, llm.Costs.InputCost)
	require.NotNil(t, llm.Costs.CachedInputCost)
	require.NotNil(t, llm.Costs.CacheWriteInputCost)
	require.NotNil(t, llm.Costs.OutputCost)
	require.InDelta(t, 2.5, *llm.Costs.InputCost, 1e-9)
	require.InDelta(t, 0.25, *llm.Costs.CachedInputCost, 1e-9)
	require.InDelta(t, 6.25, *llm.Costs.CacheWriteInputCost, 1e-9)
	require.InDelta(t, 15.0, *llm.Costs.OutputCost, 1e-9)
}
