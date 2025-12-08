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
	MustModify   StringList `yaml:"must-modify"`
	NoModify     []string   `yaml:"no-modify"`
	Copy         []CopyStep `yaml:"copy"`
	Tests        StringList `yaml:"tests"`
	PartialTests StringList `yaml:"partial-tests"`
}

// TestTarget represents a go test target and optional -run pattern.
type TestTarget struct {
	Target string
	Run    string
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
	if _, err := sc.TestTargets(); err != nil {
		return err
	}
	if _, err := sc.PartialTestTargets(); err != nil {
		return err
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
	if err := validateMustModify(sc.Verify.MustModify); err != nil {
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

func validateMustModify(entries StringList) error {
	// Allow empty slice.
	for _, v := range entries {
		if strings.TrimSpace(v) == "" {
			return errors.New("verify.must-modify entries cannot be empty")
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
	if strings.HasPrefix(repo, "http://") ||
		strings.HasPrefix(repo, "https://") ||
		strings.HasPrefix(repo, "git@") ||
		strings.HasPrefix(repo, "file://") ||
		filepath.IsAbs(repo) {
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

// TestTargets returns parsed verify.tests entries.
func (s Scenario) TestTargets() ([]TestTarget, error) {
	return parseTestTargets("verify.tests", s.Verify.Tests)
}

// PartialTestTargets returns parsed verify.partial-tests entries.
func (s Scenario) PartialTestTargets() ([]TestTarget, error) {
	return parseTestTargets("verify.partial-tests", s.Verify.PartialTests)
}

func parseTestTargets(field string, entries StringList) ([]TestTarget, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	targets := make([]TestTarget, 0, len(entries))
	for _, raw := range entries {
		target, err := parseTestTarget(raw)
		if err != nil {
			return nil, fmt.Errorf("%s entry %q: %w", field, raw, err)
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func parseTestTarget(raw string) (TestTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TestTarget{}, errors.New("entry cannot be empty")
	}
	if strings.ContainsAny(raw, "\r\n") {
		return TestTarget{}, errors.New("entry cannot contain newlines")
	}

	runIdx := findRunIndex(raw)
	if runIdx == 0 {
		return TestTarget{}, errors.New("target must appear before -run")
	}
	if runIdx > 0 {
		if strings.Contains(raw[runIdx+1:], " -run") {
			return TestTarget{}, errors.New("only one -run flag is allowed")
		}
	}

	target := raw
	run := ""
	if runIdx > 0 {
		target = strings.TrimSpace(raw[:runIdx])
		runPart := strings.TrimSpace(raw[runIdx:])
		if !strings.HasPrefix(runPart, "-run") {
			return TestTarget{}, fmt.Errorf("invalid -run segment %q", runPart)
		}
		if strings.HasPrefix(runPart, "--run") {
			return TestTarget{}, errors.New("-run must not be prefixed with additional '-'")
		}
		rest := runPart[len("-run"):]
		if rest == "" {
			return TestTarget{}, errors.New("missing pattern after -run")
		}
		switch rest[0] {
		case '=':
			run = strings.TrimSpace(rest[1:])
		case ' ', '\t':
			run = strings.TrimSpace(rest[1:])
		default:
			return TestTarget{}, errors.New("pattern must follow -run using space or '='")
		}
		if run == "" {
			return TestTarget{}, errors.New("missing pattern after -run")
		}
		cleaned, err := stripOptionalQuotes(run)
		if err != nil {
			return TestTarget{}, err
		}
		run = cleaned
	}

	if err := validateTestTarget(target, run != ""); err != nil {
		return TestTarget{}, err
	}

	return TestTarget{Target: target, Run: run}, nil
}

func findRunIndex(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			continue
		}
		if !strings.HasPrefix(s[i:], "-run") {
			continue
		}
		if i == 0 {
			return 0
		}
		prev := s[i-1]
		if prev == ' ' || prev == '\t' {
			return i
		}
	}
	return -1
}

func stripOptionalQuotes(s string) (string, error) {
	if len(s) < 2 {
		return s, nil
	}
	first := s[0]
	last := s[len(s)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return s[1 : len(s)-1], nil
	}
	if first == '"' || first == '\'' || last == '"' || last == '\'' {
		return "", errors.New("mismatched quotes in -run pattern")
	}
	return s, nil
}

func validateTestTarget(target string, hasRun bool) error {
	if target == "" {
		return errors.New("target is required")
	}
	if strings.ContainsAny(target, " \t\r\n") {
		return fmt.Errorf("target %q cannot contain whitespace", target)
	}
	if filepath.IsAbs(target) {
		return fmt.Errorf("target %q must be relative", target)
	}
	if strings.HasPrefix(target, "-") {
		return fmt.Errorf("target %q cannot start with '-'", target)
	}
	if strings.HasSuffix(target, "/") {
		return fmt.Errorf("target %q cannot end with '/'", target)
	}
	if strings.ContainsAny(target, "\"'") {
		return fmt.Errorf("target %q must not include quotes", target)
	}

	if hasGlob(target) {
		if !strings.HasSuffix(target, "_test.go") {
			return fmt.Errorf("glob target %q must end with _test.go", target)
		}
		if hasRun {
			return fmt.Errorf("glob target %q cannot be combined with -run", target)
		}
		return nil
	}

	if strings.HasSuffix(target, ".go") {
		if !strings.HasSuffix(target, "_test.go") {
			return fmt.Errorf("file target %q must be a *_test.go file", target)
		}
		return nil
	}

	if strings.Contains(target, "...") {
		if !strings.HasSuffix(target, "...") {
			return fmt.Errorf("package pattern %q must end with ...", target)
		}
		if hasRun {
			return fmt.Errorf("package pattern %q cannot be combined with -run", target)
		}
		return nil
	}

	return nil
}

func hasGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
