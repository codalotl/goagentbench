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
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type AgentMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type RunProgress struct {
	RunID           string         `json:"run_id"`
	Scenario        string         `json:"scenario"`
	Agent           string         `json:"agent"`
	AgentVersion    string         `json:"agent_version"`
	Model           string         `json:"model,omitempty"`
	StartedAt       time.Time      `json:"started_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	EndedAt         *time.Time     `json:"ended_at,omitempty"`
	DurationSeconds float64        `json:"duration_seconds"`
	TokenUsage      TokenUsage     `json:"token_usage"`
	Messages        []AgentMessage `json:"messages,omitempty"`
	Notes           string         `json:"notes,omitempty"`
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
