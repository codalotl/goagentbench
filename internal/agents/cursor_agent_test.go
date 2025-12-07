package agents

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCursorAgentOutput_ExtractsSessionAndKeepsRawTranscript(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"system","subtype":"init","apiKeySource":"env","cwd":"/home/jonathan/projects/goagentbench","session_id":"6e9b8a14-d612-4674-a9f1-c712061927ef","model":"Grok","permissionMode":"default"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"tell me about this repo"}]},"session_id":"6e9b8a14-d612-4674-a9f1-c712061927ef"}`,
		`{"type":"thinking","subtype":"delta","text":"The","session_id":"6e9b8a14-d612-4674-a9f1-c712061927ef","timestamp_ms":1765042350619}`,
		`{"type":"thinking","subtype":"delta","text":" user","session_id":"6e9b8a14-d612-4674-a9f1-c712061927ef","timestamp_ms":1765042350670}`,
	}, "\n")

	transcript, session := parseCursorAgentOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Equal(t, "6e9b8a14-d612-4674-a9f1-c712061927ef", session)
}

func TestParseCursorAgentOutput_FallbackSessionEmpty(t *testing.T) {
	raw := "some plain output line"

	transcript, session := parseCursorAgentOutput([]byte(raw))

	require.Equal(t, raw, transcript)
	require.Empty(t, session)
}

func TestParseCursorAgentVersion(t *testing.T) {
	version := parseCursorAgentVersion("cursor-agent version 2025.11.25-d5b3271")
	require.Equal(t, "2025.11.25-d5b3271", version)

	version = parseCursorAgentVersion("2025.11.25-d5b3271")
	require.Equal(t, "2025.11.25-d5b3271", version)
}
