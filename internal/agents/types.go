package agents

type RunOptions struct {
	// Package is an optional Go package path (relative to the workspace root)
	// that an agent may use to scope work (ex: "internal/cli").
	Package string
}

// RunResults contains the details returned by an Agent Run invocation.
type RunResults struct {
	// Transcript is the full transcript
	Transcript string

	InputTokens            int     // number of uncached input tokens. This is essentially context used
	CachedInputTokens      int     // number of cached input tokens. As a convo is renetered with tool call results, the old parts accumulate here.
	WriteCachedInputTokens int     // number of tokens spent writing content to the cache
	OutputTokens           int     // number of reasoning/output tokens
	Cost                   float64 // total cost for the run (if available)

	// ScaleDuration, when >0, scales the wall-clock DurationSeconds recorded in RunProgress.
	// A value of 0 means "unscaled" (use the measured elapsed time).
	// The purpose of this is to, for example, adjust ChatGPT Pro's Priority Processing back to "apples-to-apples" times.
	// A better solution might be to just not use ChatGPT Pro's auth, but that would cost more money/time to run.
	ScaleDuration float64

	// If an agent supports it, this is the session ID (or resume ID). We can pass this ID to future Run calls to continue.
	Session string

	// Error is any error produced by trying to run the agent.
	Err error
}

type Agent interface {
	Version() (string, error)
	Run(cwd string, llm LLMDefinition, session string, instructions string, opts RunOptions) RunResults
}
