package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Definition struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	SupportsLLMs []string `yaml:"supports-llms"`
}

type LLMDefinition struct {
	Name           string            `yaml:"name"`
	Model          string            `yaml:"model"`
	ReasoningLevel string            `yaml:"reasoning-level"`
	PerAgent       map[string]string `yaml:"per-agent"`
}

type registryFile struct {
	Agents []Definition `yaml:"agents"`
}

type llmFile struct {
	LLMs []LLMDefinition `yaml:"llms"`
}

type Registry struct {
	Agents map[string]Definition
	LLMs   map[string]LLMDefinition
}

func LoadRegistry(root string) (*Registry, error) {
	agentPath := filepath.Join(root, "agents.yml")
	llmPath := filepath.Join(root, "llms.yml")
	var af registryFile
	if err := readYAML(agentPath, &af); err != nil {
		return nil, err
	}
	var lf llmFile
	if err := readYAML(llmPath, &lf); err != nil {
		return nil, err
	}
	reg := &Registry{
		Agents: map[string]Definition{},
		LLMs:   map[string]LLMDefinition{},
	}
	for _, a := range af.Agents {
		if a.Name == "" {
			return nil, fmt.Errorf("agent with empty name in %s", agentPath)
		}
		reg.Agents[a.Name] = a
	}
	for _, l := range lf.LLMs {
		if l.Name == "" {
			return nil, fmt.Errorf("llm with empty name in %s", llmPath)
		}
		reg.LLMs[l.Name] = l
	}
	return reg, nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

func (r *Registry) Agent(name string) (Definition, bool) {
	a, ok := r.Agents[name]
	return a, ok
}

func (r *Registry) LLM(name string) (LLMDefinition, bool) {
	l, ok := r.LLMs[name]
	return l, ok
}

// ValidateAgentModel ensures the agent and model exist and the model is supported.
func (r *Registry) ValidateAgentModel(agentName, model string) (Definition, *LLMDefinition, error) {
	agent, ok := r.Agent(agentName)
	if !ok {
		return Definition{}, nil, fmt.Errorf("unknown agent %q", agentName)
	}
	if model == "" {
		if len(agent.SupportsLLMs) == 0 {
			return Definition{}, nil, fmt.Errorf("agent %q has no supported models", agentName)
		}
		defaultModel := agent.SupportsLLMs[0]
		llm, ok := r.LLM(defaultModel)
		if !ok {
			return Definition{}, nil, fmt.Errorf("default model %q for agent %q missing from llms.yml", defaultModel, agentName)
		}
		resolved := llm.resolvedForAgent(agentName)
		return agent, &resolved, nil
	}
	llm, ok := r.LLM(model)
	if !ok {
		return Definition{}, nil, fmt.Errorf("unknown model %q", model)
	}
	for _, m := range agent.SupportsLLMs {
		if m == model {
			resolved := llm.resolvedForAgent(agentName)
			return agent, &resolved, nil
		}
	}
	return Definition{}, nil, fmt.Errorf("agent %q does not support model %q", agentName, model)
}

func (l LLMDefinition) resolvedForAgent(agentName string) LLMDefinition {
	if agentName == "" {
		return l
	}
	if override := strings.TrimSpace(l.perAgentModel(agentName)); override != "" {
		l.Model = override
	}
	return l
}

func (l LLMDefinition) perAgentModel(agentName string) string {
	if len(l.PerAgent) == 0 {
		return ""
	}
	return l.PerAgent[agentName]
}
