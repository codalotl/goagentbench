package agents

import (
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
