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
	Printer      *output.Printer
}

type RunOutcome struct {
	Progress *types.RunProgress
	Manual   bool
}

// AgentVersion returns the actual version reported by the agent harness, if any.
// The boolean indicates whether a harness exists (manual agents return false).
func AgentVersion(ctx context.Context, def Definition) (string, bool, error) {
	agent, ok := buildAgent(ctx, def, nil)
	if !ok {
		return "", false, nil
	}
	version, err := agent.Version()
	if err != nil {
		return "", true, err
	}
	if strings.TrimSpace(version) == "" {
		return "", true, fmt.Errorf("agent %q returned an empty version", def.Name)
	}
	return version, true, nil
}

// Run invokes the harness for the given agent. Some agents are manual and
// simply return a stub progress file instructing the user to run the agent.
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
		results := agent.Run(rc.ScenarioPath, *llm, rc.Session, rc.Instructions)
		ended := time.Now()
		progress := runResultsToProgress(modelName, rc, started, ended, results)
		return &RunOutcome{Progress: progress, Manual: false}, errorFromRunResults(results)
	}

	switch rc.Agent.Name {
	case "codalotl":
		return runManual(rc, "Run the codalotl agent manually in the scenario directory.")
	default:
		return runManual(rc, fmt.Sprintf("No harness for agent %q; please run manually.", rc.Agent.Name))
	}
}

func buildAgent(ctx context.Context, def Definition, printer *output.Printer) (Agent, bool) {
	switch def.Name {
	case "codex":
		return newCodexAgent(ctx, printer), true
	case "cursor-agent":
		return newCursorAgent(ctx, printer), true
	case "claude":
		return newClaudeAgent(ctx, printer), true
	default:
		return nil, false
	}
}

func runManual(rc RunContext, note string) (*RunOutcome, error) {
	modelName := rc.ModelName
	if modelName == "" && rc.LLM != nil {
		modelName = rc.LLM.Name
	}
	now := time.Now()
	progress := &types.RunProgress{
		RunID:           "",
		Scenario:        rc.ScenarioName,
		Agent:           rc.Agent.Name,
		AgentVersion:    rc.Agent.Version,
		Model:           modelName,
		StartedAt:       now,
		UpdatedAt:       now,
		EndedAt:         &now,
		Session:         rc.Session,
		DurationSeconds: 0,
		TokenUsage:      types.TokenUsage{},
		Notes:           note,
		Transcripts:     []string{note},
	}
	return &RunOutcome{Progress: progress, Manual: true}, nil
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
	if session == "" {
		session = strings.TrimSpace(rc.Session)
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
		DurationSeconds: ended.Sub(started).Seconds(),
		TokenUsage: types.TokenUsage{
			Input:            results.InputTokens,
			CachedInput:      results.CachedInputTokens,
			WriteCachedInput: results.WriteCachedInputTokens,
			Output:           completionTokens,
			Total:            promptTokens + completionTokens,
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
