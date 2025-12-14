package report

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codalotl/goagentbench/internal/types"
)

const resultsEnvVar = "GOAGENTBENCH_RESULTS"

type Options struct {
	RootPath         string
	Scenarios        []string
	Agents           []string
	Models           []string
	Limit            int
	After            *time.Time
	AllAgentVersions bool
	IncludeTokens    bool
}

type Row struct {
	Agent              string
	Model              string
	AgentVersion       string
	UniqueScenarios    int
	Count              int
	Success            int
	PartialScoreSum    float64
	SuccessRate        float64
	PartialSuccessRate float64
	AvgCost            float64
	AvgTimeSeconds     float64
	AvgTokInput        float64
	AvgTokCachedInput  float64
	AvgTokWriteCached  float64
	AvgTokOutput       float64
	AvgTokTotal        float64
}

type Report struct {
	IncludeTokens bool
	Rows          []Row
}

func Run(opts Options) (*Report, error) {
	if strings.TrimSpace(opts.RootPath) == "" {
		return nil, errors.New("RootPath is required")
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 1
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be >= 1, got %d", limit)
	}

	entries, err := loadResults(resultsDir(opts.RootPath))
	if err != nil {
		return nil, err
	}

	scenarioSet := sliceToSet(opts.Scenarios)
	agentSet := sliceToSet(opts.Agents)
	modelSet := sliceToSet(opts.Models)

	filtered := make([]resultEntry, 0, len(entries))
	for _, e := range entries {
		sc := strings.TrimSpace(e.Scenario)
		agent := strings.TrimSpace(e.Agent)
		model := strings.TrimSpace(e.Model)
		if scenarioSet != nil && !scenarioSet[sc] {
			continue
		}
		if agentSet != nil && !agentSet[agent] {
			continue
		}
		if modelSet != nil && !modelSet[model] {
			continue
		}
		if opts.After != nil && e.VerifiedAt.Before(*opts.After) {
			continue
		}
		filtered = append(filtered, e)
	}

	filtered = dedupByRunIDKeepLatest(filtered)
	if !opts.AllAgentVersions {
		filtered = filterToLatestVersionPerAgentModel(filtered)
	}
	filtered = applyLimitPerScenarioAgentModel(filtered, limit)

	grouped := map[string][]resultEntry{}
	for _, e := range filtered {
		key := agentModelKey(e.Agent, e.Model)
		grouped[key] = append(grouped[key], e)
	}

	rows := make([]Row, 0, len(grouped))
	for _, group := range grouped {
		row, ok := buildRow(group, opts.AllAgentVersions)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].SuccessRate != rows[j].SuccessRate {
			return rows[i].SuccessRate > rows[j].SuccessRate
		}
		if rows[i].PartialSuccessRate != rows[j].PartialSuccessRate {
			return rows[i].PartialSuccessRate > rows[j].PartialSuccessRate
		}
		if rows[i].Agent != rows[j].Agent {
			return rows[i].Agent < rows[j].Agent
		}
		return rows[i].Model < rows[j].Model
	})

	return &Report{
		IncludeTokens: opts.IncludeTokens,
		Rows:          rows,
	}, nil
}

func (r *Report) WriteCSV(w io.Writer) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	header := []string{
		"agent",
		"model",
		"agent_version",
		"unique_scenarios",
		"count",
		"success",
		"partial_success_score",
		"success_rate",
		"partial_success_rate",
		"avg_cost",
		"avg_time",
	}
	if r.IncludeTokens {
		header = append(header,
			"avg_tok_input",
			"avg_tok_cached_input",
			"avg_tok_write_cached_input",
			"avg_tok_output",
			"avg_tok_total",
		)
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, row := range r.Rows {
		record := []string{
			row.Agent,
			row.Model,
			row.AgentVersion,
			strconv.Itoa(row.UniqueScenarios),
			strconv.Itoa(row.Count),
			strconv.Itoa(row.Success),
			formatFloat(row.PartialScoreSum),
			formatFloat(row.SuccessRate),
			formatFloat(row.PartialSuccessRate),
			formatFloat(row.AvgCost),
			formatFloat(row.AvgTimeSeconds),
		}
		if r.IncludeTokens {
			record = append(record,
				formatFloat(row.AvgTokInput),
				formatFloat(row.AvgTokCachedInput),
				formatFloat(row.AvgTokWriteCached),
				formatFloat(row.AvgTokOutput),
				formatFloat(row.AvgTokTotal),
			)
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

type resultEntry struct {
	RunID      string
	Scenario   string
	Agent      string
	Model      string
	Version    string
	VerifiedAt time.Time
	Success    bool
	Partial    *float64
	Duration   float64
	TokenUsage types.TokenUsage
}

func loadResults(dir string) ([]resultEntry, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var out []resultEntry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if filepath.Clean(path) == filepath.Join(dir, "smoke") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".verify.json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var rep types.VerificationReport
		if err := json.Unmarshal(data, &rep); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		verifiedAt := rep.VerifiedAt
		if verifiedAt.IsZero() {
			info, err := os.Stat(path)
			if err == nil {
				verifiedAt = info.ModTime()
			}
		}

		scenarioName := strings.TrimSpace(rep.Scenario)
		if scenarioName == "" {
			scenarioName = scenarioFromPath(dir, path)
		}
		if scenarioName == "smoke" {
			return nil
		}

		var duration float64
		var usage types.TokenUsage
		if rep.Progress != nil {
			duration = rep.Progress.DurationSeconds
			usage = rep.Progress.TokenUsage
		}

		out = append(out, resultEntry{
			RunID:      strings.TrimSpace(rep.RunID),
			Scenario:   scenarioName,
			Agent:      strings.TrimSpace(rep.Agent),
			Model:      strings.TrimSpace(rep.Model),
			Version:    strings.TrimSpace(rep.AgentVersion),
			VerifiedAt: verifiedAt,
			Success:    rep.Success,
			Partial:    rep.PartialScore,
			Duration:   duration,
			TokenUsage: usage,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func scenarioFromPath(resultsDir, filePath string) string {
	rel, err := filepath.Rel(resultsDir, filePath)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func resultsDir(rootPath string) string {
	if env := strings.TrimSpace(os.Getenv(resultsEnvVar)); env != "" {
		if filepath.IsAbs(env) {
			return filepath.Clean(env)
		}
		return filepath.Join(rootPath, filepath.Clean(env))
	}
	return filepath.Join(rootPath, "results")
}

func sliceToSet(items []string) map[string]bool {
	var out map[string]bool
	for _, s := range items {
		val := strings.TrimSpace(s)
		if val == "" {
			continue
		}
		if out == nil {
			out = map[string]bool{}
		}
		out[val] = true
	}
	return out
}

func agentModelKey(agent, model string) string {
	return strings.TrimSpace(agent) + "\x00" + strings.TrimSpace(model)
}

func scenarioAgentModelKey(sc, agent, model string) string {
	return strings.TrimSpace(sc) + "\x00" + strings.TrimSpace(agent) + "\x00" + strings.TrimSpace(model)
}

func dedupByRunIDKeepLatest(entries []resultEntry) []resultEntry {
	seen := map[string]resultEntry{}
	var noRunID []resultEntry
	for _, e := range entries {
		if e.RunID == "" {
			noRunID = append(noRunID, e)
			continue
		}
		prev, ok := seen[e.RunID]
		if !ok || e.VerifiedAt.After(prev.VerifiedAt) {
			seen[e.RunID] = e
		}
	}
	out := make([]resultEntry, 0, len(seen)+len(noRunID))
	out = append(out, noRunID...)
	for _, e := range seen {
		out = append(out, e)
	}
	return out
}

func applyLimitPerScenarioAgentModel(entries []resultEntry, limit int) []resultEntry {
	grouped := map[string][]resultEntry{}
	for _, e := range entries {
		key := scenarioAgentModelKey(e.Scenario, e.Agent, e.Model)
		grouped[key] = append(grouped[key], e)
	}
	out := make([]resultEntry, 0, len(entries))
	for _, group := range grouped {
		sort.Slice(group, func(i, j int) bool {
			return group[i].VerifiedAt.After(group[j].VerifiedAt)
		})
		if len(group) > limit {
			group = group[:limit]
		}
		out = append(out, group...)
	}
	return out
}

func buildRow(group []resultEntry, allAgentVersions bool) (Row, bool) {
	if len(group) == 0 {
		return Row{}, false
	}
	agent := group[0].Agent
	model := group[0].Model

	selectedVersion := selectLatestVersion(group)
	if !allAgentVersions && selectedVersion != "" {
		filtered := group[:0]
		for _, e := range group {
			if e.Version == selectedVersion {
				filtered = append(filtered, e)
			}
		}
		group = filtered
		if len(group) == 0 {
			return Row{}, false
		}
	}

	uniqueScenarios := map[string]bool{}
	versions := map[string]bool{}

	successCount := 0
	partialSum := 0.0
	var costs []float64
	var times []float64
	var tokIn []float64
	var tokCached []float64
	var tokWriteCached []float64
	var tokOut []float64
	var tokTotal []float64

	for _, e := range group {
		uniqueScenarios[e.Scenario] = true
		if strings.TrimSpace(e.Version) != "" {
			versions[e.Version] = true
		}
		if e.Success {
			successCount++
		}
		partialSum += partialScore(e)

		if e.TokenUsage.Cost != 0 {
			costs = append(costs, e.TokenUsage.Cost)
		}
		if e.Duration != 0 {
			times = append(times, e.Duration)
		}
		if e.TokenUsage.Input != 0 {
			tokIn = append(tokIn, float64(e.TokenUsage.Input))
		}
		if e.TokenUsage.CachedInput != 0 {
			tokCached = append(tokCached, float64(e.TokenUsage.CachedInput))
		}
		if e.TokenUsage.WriteCachedInput != 0 {
			tokWriteCached = append(tokWriteCached, float64(e.TokenUsage.WriteCachedInput))
		}
		if e.TokenUsage.Output != 0 {
			tokOut = append(tokOut, float64(e.TokenUsage.Output))
		}
		if e.TokenUsage.Total != 0 {
			tokTotal = append(tokTotal, float64(e.TokenUsage.Total))
		}
	}

	count := len(group)
	successRate := 0.0
	if count > 0 {
		successRate = float64(successCount) / float64(count)
	}
	partialRate := 0.0
	if count > 0 {
		partialRate = partialSum / float64(count)
	}

	versionList := uniqueVersionsSorted(versions)
	versionValue := ""
	switch {
	case allAgentVersions && len(versionList) > 0:
		versionValue = strings.Join(versionList, ",")
	case !allAgentVersions && strings.TrimSpace(selectedVersion) != "":
		versionValue = selectedVersion
	case !allAgentVersions && len(versionList) > 0:
		versionValue = versionList[len(versionList)-1]
	}

	return Row{
		Agent:              agent,
		Model:              model,
		AgentVersion:       versionValue,
		UniqueScenarios:    len(uniqueScenarios),
		Count:              count,
		Success:            successCount,
		PartialScoreSum:    partialSum,
		SuccessRate:        successRate,
		PartialSuccessRate: partialRate,
		AvgCost:            avgOrZero(costs),
		AvgTimeSeconds:     avgOrZero(times),
		AvgTokInput:        avgOrZero(tokIn),
		AvgTokCachedInput:  avgOrZero(tokCached),
		AvgTokWriteCached:  avgOrZero(tokWriteCached),
		AvgTokOutput:       avgOrZero(tokOut),
		AvgTokTotal:        avgOrZero(tokTotal),
	}, true
}

func partialScore(e resultEntry) float64 {
	if e.Partial != nil {
		return *e.Partial
	}
	if e.Success {
		return 1
	}
	return 0
}

func avgOrZero(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func selectLatestVersion(entries []resultEntry) string {
	var semvers []string
	var stringsOnly []string
	seen := map[string]bool{}
	for _, e := range entries {
		v := strings.TrimSpace(e.Version)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		if isSemverLike(v) {
			semvers = append(semvers, v)
		} else {
			stringsOnly = append(stringsOnly, v)
		}
	}
	if len(semvers) > 0 {
		sort.Slice(semvers, func(i, j int) bool {
			return compareSemver(semvers[i], semvers[j]) < 0
		})
		return semvers[len(semvers)-1]
	}
	if len(stringsOnly) == 0 {
		return ""
	}
	sort.Strings(stringsOnly)
	return stringsOnly[len(stringsOnly)-1]
}

func filterToLatestVersionPerAgentModel(entries []resultEntry) []resultEntry {
	grouped := map[string][]resultEntry{}
	for _, e := range entries {
		key := agentModelKey(e.Agent, e.Model)
		grouped[key] = append(grouped[key], e)
	}
	out := make([]resultEntry, 0, len(entries))
	for _, group := range grouped {
		selected := selectLatestVersion(group)
		if strings.TrimSpace(selected) == "" {
			out = append(out, group...)
			continue
		}
		for _, e := range group {
			if e.Version == selected {
				out = append(out, e)
			}
		}
	}
	return out
}

func uniqueVersionsSorted(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	semvers := make([]string, 0, len(set))
	other := make([]string, 0, len(set))
	for v := range set {
		if isSemverLike(v) {
			semvers = append(semvers, v)
		} else {
			other = append(other, v)
		}
	}
	sort.Slice(semvers, func(i, j int) bool {
		return compareSemver(semvers[i], semvers[j]) < 0
	})
	sort.Strings(other)
	return append(semvers, other...)
}

type parsedVersion struct {
	major  int
	minor  int
	patch  int
	pre    string
	hasPre bool
}

func isSemverLike(v string) bool {
	_, ok := parseSemver(v)
	return ok
}

func compareSemver(a, b string) int {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	if pa.major != pb.major {
		return cmpInt(pa.major, pb.major)
	}
	if pa.minor != pb.minor {
		return cmpInt(pa.minor, pb.minor)
	}
	if pa.patch != pb.patch {
		return cmpInt(pa.patch, pb.patch)
	}
	if pa.hasPre != pb.hasPre {
		if pa.hasPre {
			return -1
		}
		return 1
	}
	if !pa.hasPre {
		return 0
	}
	return strings.Compare(pa.pre, pb.pre)
}

func parseSemver(raw string) (parsedVersion, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return parsedVersion{}, false
	}
	main := s
	pre := ""
	if idx := strings.IndexAny(s, "+-"); idx >= 0 {
		main = s[:idx]
		if s[idx] == '-' {
			rest := s[idx+1:]
			if plus := strings.Index(rest, "+"); plus >= 0 {
				pre = rest[:plus]
			} else {
				pre = rest
			}
		}
	}
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return parsedVersion{}, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	pat, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return parsedVersion{}, false
	}
	p := parsedVersion{major: maj, minor: min, patch: pat}
	if strings.TrimSpace(pre) != "" {
		p.pre = pre
		p.hasPre = true
	}
	return p, true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func formatFloat(v float64) string {
	// Compensate for common binary floating-point representation issues so values
	// like 1.005 reliably round to 1.01 at 2 decimal places.
	rounded := math.Round((v+math.Copysign(1e-9, v))*100) / 100
	if rounded == 0 {
		return "0"
	}
	s := strconv.FormatFloat(rounded, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "-0" {
		return "0"
	}
	return s
}
