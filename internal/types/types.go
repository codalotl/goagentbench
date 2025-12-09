package types

import "time"

type SystemInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

type RunStart struct {
	RunID        string     `json:"run_id"`
	Scenario     string     `json:"scenario"`
	Workspace    string     `json:"workspace"`
	Agent        string     `json:"agent"`
	AgentVersion string     `json:"agent_version"`
	Model        string     `json:"model,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	System       SystemInfo `json:"system"`
}

type TokenUsage struct {
	Input            int `json:"input"`
	CachedInput      int `json:"cached_input"`
	WriteCachedInput int `json:"write_cached_input"`
	Output           int `json:"output"`
	Total            int `json:"total"`
}

type AgentMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type RunProgress struct {
	RunID           string     `json:"run_id"`
	Scenario        string     `json:"scenario"`
	Agent           string     `json:"agent"`
	AgentVersion    string     `json:"agent_version"`
	Model           string     `json:"model,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Session         string     `json:"session,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	DurationSeconds float64    `json:"duration_seconds"`
	TokenUsage      TokenUsage `json:"token_usage"`
	Transcripts     []string   `json:"transcripts,omitempty"`
	Notes           string     `json:"notes,omitempty"`
}

type TestResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type VerificationReport struct {
	RunID        string       `json:"run_id"`
	Scenario     string       `json:"scenario"`
	Agent        string       `json:"agent"`
	AgentVersion string       `json:"agent_version"`
	Model        string       `json:"model,omitempty"`
	StartedAt    *time.Time   `json:"started_at,omitempty"`
	Progress     *RunProgress `json:"progress,omitempty"`
	VerifiedAt   time.Time    `json:"verified_at"`
	Success      bool         `json:"success"`
	PartialScore *float64     `json:"partial_score,omitempty"`
	Tests        []TestResult `json:"tests"`
	PartialTests []TestResult `json:"partial_tests,omitempty"`
}
