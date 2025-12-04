package scenario

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Scenario represents the scenario.yml file contents.
type Scenario struct {
	Name           string         `yaml:"name"`
	Repo           string         `yaml:"repo"`
	Commit         string         `yaml:"commit"`
	Classification Classification `yaml:"classification"`
	Setup          *SetupConfig   `yaml:"setup"`
	Agent          AgentConfig    `yaml:"agent"`
	Verify         VerifyConfig   `yaml:"verify"`
}

type Classification struct {
	Type             string `yaml:"type"`
	HasSpec          *bool  `yaml:"has-spec"`
	SinglePackage    *bool  `yaml:"single-package"`
	SeesFailingTests *bool  `yaml:"sees-failing-tests"`
}

type SetupConfig struct {
	Copy []CopyStep `yaml:"copy"`
}

type CopyStep struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type AgentConfig struct {
	Instructions                     string `yaml:"instructions"`
	AllowMultipleTurns               bool   `yaml:"allow-multiple-turns"`
	AllowMultipleTurnsOnFailedVerify bool   `yaml:"allow-multiple-turns-on-failed-verify"`
}

type VerifyConfig struct {
	OnlyModify   StringList `yaml:"only-modify"`
	NoModify     []string   `yaml:"no-modify"`
	Copy         []CopyStep `yaml:"copy"`
	Tests        StringList `yaml:"tests"`
	PartialTests StringList `yaml:"partial-tests"`
}

// StringList allows unmarshalling a string or a slice of strings.
type StringList []string

// UnmarshalYAML makes StringList accept a string or a slice.
func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var v string
		if err := value.Decode(&v); err != nil {
			return err
		}
		if v != "" {
			*s = []string{v}
		}
		return nil
	case yaml.SequenceNode:
		var vals []string
		if err := value.Decode(&vals); err != nil {
			return err
		}
		*s = vals
		return nil
	case 0:
		// missing field is fine
		return nil
	default:
		return fmt.Errorf("expected string or list, got %v", value.Kind)
	}
}

// Load reads a scenario from a scenario.yml path.
func Load(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Scenario
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, err
	}
	return &sc, nil
}

// Validate checks required fields and referenced files.
func Validate(sc *Scenario, scenarioDir string) error {
	if sc.Name == "" {
		return errors.New("scenario name is required")
	}
	if sc.Repo == "" {
		return errors.New("scenario repo is required")
	}
	if sc.Commit == "" {
		return errors.New("scenario commit is required")
	}
	if sc.Classification.Type == "" {
		return errors.New("classification.type is required")
	}
	if strings.TrimSpace(sc.Agent.Instructions) == "" {
		return errors.New("agent.instructions is required")
	}
	if len(sc.Verify.Tests) == 0 {
		return errors.New("verify.tests is required")
	}
	if err := validateCopySteps(sc.Setup, scenarioDir); err != nil {
		return err
	}
	if err := validateCopySteps(&SetupConfig{Copy: sc.Verify.Copy}, scenarioDir); err != nil {
		return err
	}
	if err := validateCommitShape(sc.Commit); err != nil {
		return err
	}
	if err := validateOnlyModify(sc.Verify.OnlyModify); err != nil {
		return err
	}
	if err := checkRemoteCommit(sc.Repo, sc.Commit); err != nil {
		return err
	}
	return nil
}

func validateCopySteps(cfg *SetupConfig, scenarioDir string) error {
	if cfg == nil {
		return nil
	}
	for _, c := range cfg.Copy {
		if c.From == "" || c.To == "" {
			return fmt.Errorf("copy steps must include from and to")
		}
		src := filepath.Join(scenarioDir, c.From)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("copy source does not exist: %s", c.From)
		}
	}
	return nil
}

func validateOnlyModify(only StringList) error {
	// Allow empty slice.
	for _, v := range only {
		if strings.TrimSpace(v) == "" {
			return errors.New("verify.only-modify entries cannot be empty")
		}
	}
	return nil
}

func validateCommitShape(commit string) error {
	if commit == "" {
		return errors.New("commit is required")
	}
	isSHA, _ := regexp.MatchString(`^[a-fA-F0-9]{7,}$`, commit)
	if !isSHA {
		return fmt.Errorf("commit %q does not look like a git sha", commit)
	}
	return nil
}

// NormalizeRepoURL returns a cloneable repo URL.
func NormalizeRepoURL(repo string) string {
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "git@") {
		return repo
	}
	return "https://" + repo
}

func checkRemoteCommit(repo, commit string) error {
	if repo == "" || commit == "" {
		return nil
	}
	if os.Getenv("GOAGENTBENCH_SKIP_REMOTE") != "" {
		return nil
	}
	url := NormalizeRepoURL(repo)
	cmd := exec.Command("git", "ls-remote", url, commit)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git ls-remote %s %s: %w", url, commit, err)
	}
	return nil
}
