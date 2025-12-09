package agents

// RunResults contains the details returned by an Agent Run invocation.
type RunResults struct {
	// Transcript is the full transcript
	Transcript string

	InputTokens            int // number of uncached input tokens. This is essentially context used
	CachedInputTokens      int // number of cached input tokens. As a convo is renetered with tool call results, the old parts accumulate here.
	WriteCachedInputTokens int // number of tokens spent writing content to the cache
	OutputTokens           int // number of reasoning/output tokens

	// If an agent supports it, this is the session ID (or resume ID). We can pass this ID to future Run calls to continue.
	Session string

	// Error is any error produced by trying to run the agent.
	Err error
}

type Agent interface {
	Version() (string, error)
	Run(cwd string, llm LLMDefinition, session string, instructions string) RunResults
}
