package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codalotl/goagentbench/internal/output"
	"github.com/codalotl/goagentbench/internal/types"
)

type RunContext struct {
	ScenarioName string
	ScenarioPath string
	ModelName    string
	LLM          *LLMDefinition
	Agent        Definition
	Instructions string
	Session      string
	Options      RunOptions
	Printer      *output.Printer
}

type RunOutcome struct {
	Progress *types.RunProgress
}

// AgentVersion returns the actual version reported by the agent harness.
func AgentVersion(ctx context.Context, def Definition) (string, error) {
	agent, ok := buildAgent(ctx, def, nil)
	if !ok {
		return "", fmt.Errorf("no harness for agent %q", def.Name)
	}
	version, err := agent.Version()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(version) == "" {
		return "", fmt.Errorf("agent %q returned an empty version", def.Name)
	}
	return version, nil
}

// Run invokes the harness for the given agent.
func Run(ctx context.Context, rc RunContext) (*RunOutcome, error) {
	modelName := rc.ModelName
	if modelName == "" && rc.LLM != nil {
		modelName = rc.LLM.Name
	}
	var llm *LLMDefinition
	if rc.LLM != nil {
		llmCopy := rc.LLM.resolvedForAgent(rc.Agent.Name)
		llm = &llmCopy
	}
	if agent, ok := buildAgent(ctx, rc.Agent, rc.Printer); ok {
		if llm == nil {
			return nil, fmt.Errorf("model is required for agent %q", rc.Agent.Name)
		}
		started := time.Now()
		results := agent.Run(rc.ScenarioPath, *llm, rc.Session, rc.Instructions, rc.Options)
		ended := time.Now()
		progress := runResultsToProgress(modelName, rc, started, ended, results)
		return &RunOutcome{Progress: progress}, errorFromRunResults(results)
	}
	return nil, fmt.Errorf("no harness for agent %q", rc.Agent.Name)
}

func buildAgent(ctx context.Context, def Definition, printer *output.Printer) (Agent, bool) {
	switch def.Name {
	case "codex":
		return newCodexAgent(ctx, printer), true
	case "codalotl":
		return newCodalotlAgent(ctx, printer), true
	case "cursor-agent":
		return newCursorAgent(ctx, printer), true
	case "claude":
		return newClaudeAgent(ctx, printer), true
	case "crush":
		return newCrushAgent(ctx, printer), true
	default:
		return nil, false
	}
}

func runResultsToProgress(modelName string, rc RunContext, started time.Time, ended time.Time, results RunResults) *types.RunProgress {
	promptTokens := results.InputTokens + results.CachedInputTokens + results.WriteCachedInputTokens
	completionTokens := results.OutputTokens
	transcript := strings.TrimSpace(results.Transcript)
	var transcripts []string
	if transcript != "" {
		transcripts = append(transcripts, transcript)
	}
	session := strings.TrimSpace(results.Session)
	if session == "" && rc.Agent.Name != "codalotl" {
		session = strings.TrimSpace(rc.Session)
	}

	durationSeconds := ended.Sub(started).Seconds()
	if results.ScaleDuration > 0 {
		durationSeconds *= results.ScaleDuration
	}

	progress := &types.RunProgress{
		RunID:           "",
		Scenario:        rc.ScenarioName,
		Agent:           rc.Agent.Name,
		AgentVersion:    rc.Agent.Version,
		Model:           modelName,
		StartedAt:       started,
		UpdatedAt:       ended,
		EndedAt:         &ended,
		Session:         session,
		DurationSeconds: durationSeconds,
		TokenUsage: types.TokenUsage{
			Input:            results.InputTokens,
			CachedInput:      results.CachedInputTokens,
			WriteCachedInput: results.WriteCachedInputTokens,
			Output:           completionTokens,
			Total:            promptTokens + completionTokens,
			Cost:             results.Cost,
		},
		Transcripts: transcripts,
	}
	if results.Err != nil {
		progress.Notes = strings.TrimSpace(results.Err.Error())
	}

	return progress
}

func errorFromRunResults(res RunResults) error {
	return res.Err
}
