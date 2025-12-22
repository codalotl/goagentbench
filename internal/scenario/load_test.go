package scenario_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/stretchr/testify/require"
)

func TestLoad_ParsesAgentPackage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: demo
repo: github.com/example/repo
commit: 1234567
classification:
  type: feature
agent:
  package: internal/cli
  instructions: hello
`), 0o644))

	sc, err := scenario.Load(path)
	require.NoError(t, err)
	require.Equal(t, "internal/cli", sc.Agent.Package)
}

