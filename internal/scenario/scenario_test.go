package scenario_test

import (
	"testing"

	"github.com/codalotl/goagentbench/internal/scenario"
	"github.com/stretchr/testify/require"
)

func TestScenarioTestTargets_Valid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want scenario.TestTarget
	}{
		{
			name: "package",
			raw:  "some/pkg",
			want: scenario.TestTarget{Target: "some/pkg"},
		},
		{
			name: "recursive pattern",
			raw:  "./other/...",
			want: scenario.TestTarget{Target: "./other/..."},
		},
		{
			name: "glob",
			raw:  "internal/app/golden_*_test.go",
			want: scenario.TestTarget{Target: "internal/app/golden_*_test.go"},
		},
		{
			name: "file",
			raw:  "internal/app/some_test.go",
			want: scenario.TestTarget{Target: "internal/app/some_test.go"},
		},
		{
			name: "run space separated",
			raw:  "./mypkg -run TestImportant",
			want: scenario.TestTarget{Target: "./mypkg", Run: "TestImportant"},
		},
		{
			name: "run equals",
			raw:  "./mypkg -run=TestImportant",
			want: scenario.TestTarget{Target: "./mypkg", Run: "TestImportant"},
		},
		{
			name: "run quoted double",
			raw:  "./mypkg -run \"TestImportant|TestThing\"",
			want: scenario.TestTarget{Target: "./mypkg", Run: "TestImportant|TestThing"},
		},
		{
			name: "run quoted single",
			raw:  "./mypkg -run 'TestImportant/^(Sub1|Sub2)$'",
			want: scenario.TestTarget{Target: "./mypkg", Run: "TestImportant/^(Sub1|Sub2)$"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sc := scenario.Scenario{
				Verify: scenario.VerifyConfig{
					Tests: scenario.StringList{tc.raw},
				},
			}
			got, err := sc.TestTargets()
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, tc.want, got[0])
		})
	}
}

func TestScenarioPartialTestTargets_Valid(t *testing.T) {
	sc := scenario.Scenario{
		Verify: scenario.VerifyConfig{
			PartialTests: scenario.StringList{
				"internal/app/some_test.go",
			},
		},
	}

	got, err := sc.PartialTestTargets()
	require.NoError(t, err)
	want := []scenario.TestTarget{
		{Target: "internal/app/some_test.go"},
	}
	require.Equal(t, want, got)

	sc.Verify.PartialTests = nil
	got, err = sc.PartialTestTargets()
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestScenarioTestTargets_Invalid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "absolute path", raw: "/abs/path"},
		{name: "double dash run", raw: "./pkg --run Test"},
		{name: "missing run pattern", raw: "./pkg -run"},
		{name: "missing run pattern with space", raw: "./pkg -run "},
		{name: "multiple run", raw: "./pkg -run Test -run Another"},
		{name: "package ellipsis with run", raw: "./... -run Test"},
		{name: "glob without suffix", raw: "some/pkg/*_test"},
		{name: "target ending slash", raw: "some/pkg/"},
		{name: "mismatched quotes", raw: "./pkg -run 'TestImportant"},
		{name: "target starts with dash", raw: "-pkg"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sc := scenario.Scenario{
				Verify: scenario.VerifyConfig{
					Tests: scenario.StringList{tc.raw},
				},
			}
			_, err := sc.TestTargets()
			require.Error(t, err)
		})
	}
}

func TestValidate_DoesNotRequireVerifyTests(t *testing.T) {
	t.Setenv("GOAGENTBENCH_SKIP_REMOTE", "1")
	base := scenario.Scenario{
		Name:           "demo",
		Repo:           "github.com/example/repo",
		Commit:         "1234567",
		Classification: scenario.Classification{Type: "build-package"},
		Agent:          scenario.AgentConfig{Instructions: "do the thing"},
	}

	t.Run("empty verify", func(t *testing.T) {
		sc := base
		err := scenario.Validate(&sc, t.TempDir())
		require.NoError(t, err)
	})

	t.Run("partial tests only", func(t *testing.T) {
		sc := base
		sc.Verify.PartialTests = scenario.StringList{"./pkg -run TestImportant"}
		err := scenario.Validate(&sc, t.TempDir())
		require.NoError(t, err)
	})
}
