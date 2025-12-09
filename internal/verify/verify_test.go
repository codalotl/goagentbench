package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTestArgs_NormalizesRelativeTargets(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workdir, "internal/agent"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(workdir, "pkg"), 0o755))

	tests := []struct {
		name  string
		entry string
		want  []string
	}{
		{
			name:  "addsDotSlash",
			entry: "internal/agent",
			want:  []string{"./internal/agent"},
		},
		{
			name:  "keepsExistingDotSlash",
			entry: "./internal/agent",
			want:  []string{"./internal/agent"},
		},
		{
			name:  "withRunPattern",
			entry: "internal/agent -run TestThing",
			want:  []string{"./internal/agent", "-run", "TestThing"},
		},
		{
			name:  "packagePathUnchanged",
			entry: "github.com/example/repo/internal/agent",
			want:  []string{"github.com/example/repo/internal/agent"},
		},
		{
			name:  "ellipsisPattern",
			entry: "pkg/...",
			want:  []string{"./pkg/..."},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			args, err := parseTestArgs(workdir, tt.entry)
			require.NoError(t, err)
			require.Equal(t, tt.want, args)
		})
	}
}
