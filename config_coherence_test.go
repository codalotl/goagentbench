package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codalotl/goagentbench/internal/agents"
)

func TestAgentAndLLMConfigsAreCoherent(t *testing.T) {
	root, err := os.Getwd()
	require.NoError(t, err)

	registry, err := agents.LoadRegistry(root)
	require.NoError(t, err)
	require.NotEmpty(t, registry.Agents, "agents.yml must define at least one agent")
	require.NotEmpty(t, registry.LLMs, "llms.yml must define at least one llm")

	for agentName, def := range registry.Agents {
		require.NotEmpty(t, strings.TrimSpace(def.Version), "agent %s must have a version", agentName)
		require.NotEmpty(t, def.SupportsLLMs, "agent %s must list supported llms", agentName)

		for _, llmName := range def.SupportsLLMs {
			llm, ok := registry.LLM(llmName)
			require.True(t, ok, "agent %s references unknown llm %s", agentName, llmName)
			resolvedModel := strings.TrimSpace(llm.Model)
			if override := strings.TrimSpace(llm.PerAgent[agentName]); override != "" {
				resolvedModel = override
			}
			require.NotEmpty(t, resolvedModel, "agent %s llm %s must resolve to a model string", agentName, llmName)
		}
	}

	for llmName, llm := range registry.LLMs {
		require.NotEmpty(t, strings.TrimSpace(llm.Model), "llm %s must declare a model", llmName)
		for agentKey := range llm.PerAgent {
			_, ok := registry.Agent(agentKey)
			require.True(t, ok, "llm %s has per-agent override for unknown agent %s", llmName, agentKey)
		}
	}
}
