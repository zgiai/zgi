package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/capabilities/contentparse"
	"github.com/zgiai/ginext/internal/contracts"
	"gopkg.in/yaml.v3"
)

type manifest struct {
	Version  int              `yaml:"version"`
	Name     string           `yaml:"name"`
	Defaults manifestDefaults `yaml:"defaults"`
	Cases    []benchCase      `yaml:"cases"`
}

type manifestDefaults struct {
	Intent  string `yaml:"intent"`
	Profile string `yaml:"profile"`
	Engine  string `yaml:"engine"`
}

type benchCase struct {
	ID          string       `yaml:"id"`
	Description string       `yaml:"description"`
	File        string       `yaml:"file"`
	Intent      string       `yaml:"intent"`
	Profile     string       `yaml:"profile"`
	Engine      string       `yaml:"engine"`
	Requires    requirements `yaml:"requires"`
	Expect      expectation  `yaml:"expect"`
}

type requirements struct {
	Tools    []string `yaml:"tools"`
	AnyTools []string `yaml:"any_tools"`
	Env      []string `yaml:"env"`
}

type expectation struct {
	Status        string   `yaml:"status"`
	MinElements   int      `yaml:"min_elements"`
	ContainsText  []string `yaml:"contains_text"`
	MaxDurationMS int64    `yaml:"max_duration_ms"`
}

type caseReport struct {
	ID         string         `json:"id"`
	File       string         `json:"file"`
	Status     string         `json:"status"`
	Passed     bool           `json:"passed"`
	Skipped    bool           `json:"skipped"`
	SkipReason string         `json:"skip_reason,omitempty"`
	DurationMS int64          `json:"duration_ms"`
	Artifact   artifactReport `json:"artifact"`
	Error      string         `json:"error,omitempty"`
}

type artifactReport struct {
	Status       string         `json:"status"`
	QualityLevel string         `json:"quality_level"`
	EngineUsed   string         `json:"engine_used"`
	FallbackUsed bool           `json:"fallback_used"`
	TextLength   int            `json:"text_length"`
	MarkdownLen  int            `json:"markdown_length"`
	ElementCount int            `json:"element_count"`
	Diagnostics  map[string]any `json:"diagnostics,omitempty"`
}

type suiteReport struct {
	Name        string       `json:"name"`
	Manifest    string       `json:"manifest"`
	GeneratedAt time.Time    `json:"generated_at"`
	Total       int          `json:"total"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	Skipped     int          `json:"skipped"`
	Metrics     benchMetrics `json:"metrics"`
	Results     []caseReport `json:"results"`
}

type benchMetrics struct {
	AvgDurationMS     float64 `json:"avg_duration_ms"`
	P95DurationMS     int64   `json:"p95_duration_ms"`
	AvgTextLength     float64 `json:"avg_text_length"`
	AvgElementCount   float64 `json:"avg_element_count"`
	FallbackCount     int     `json:"fallback_count"`
	LowTextLengthDocs int     `json:"low_text_length_docs"`
}

func main() {
	var (
		manifestPath = flag.String("manifest", "", "Path to benchmark manifest YAML")
		jsonOut      = flag.String("json-out", "", "Optional JSON report output path")
		caseFilter   = flag.String("case", "", "Run only cases whose id contains this substring")
	)
	flag.Parse()

	if strings.TrimSpace(*manifestPath) == "" {
		fmt.Fprintln(os.Stderr, "missing required --manifest")
		os.Exit(2)
	}

	mf, manifestDir, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	module := contentparse.NewModule()
	report := suiteReport{
		Name:        mf.Name,
		Manifest:    *manifestPath,
		GeneratedAt: time.Now(),
		Results:     make([]caseReport, 0, len(mf.Cases)),
	}

	for _, item := range mf.Cases {
		if *caseFilter != "" && !strings.Contains(item.ID, *caseFilter) {
			continue
		}
		report.Total++
		result := runCase(module, manifestDir, mf.Defaults, item)
		report.Results = append(report.Results, result)
		switch {
		case result.Skipped:
			report.Skipped++
		case result.Passed:
			report.Passed++
		default:
			report.Failed++
		}
	}

	report.Metrics = summarizeMetrics(report.Results)

	printSummary(report)

	if strings.TrimSpace(*jsonOut) != "" {
		if err := writeJSONReport(*jsonOut, report); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(1)
		}
	}

	if report.Failed > 0 {
		os.Exit(1)
	}
}

func loadManifest(path string) (*manifest, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", err
	}
	var mf manifest
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, "", err
	}
	return &mf, filepath.Dir(abs), nil
}

func runCase(module *contentparse.Module, manifestDir string, defaults manifestDefaults, item benchCase) caseReport {
	result := caseReport{
		ID:   item.ID,
		File: item.File,
	}

	if skip, reason := checkRequirements(item.Requires); skip {
		result.Skipped = true
		result.SkipReason = reason
		return result
	}

	filePath := resolveFixturePath(manifestDir, item.File)
	data, err := os.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("read fixture: %v", err)
		return result
	}

	intent := firstNonEmpty(item.Intent, defaults.Intent, string(contracts.ParseIntentPreview))
	profile := firstNonEmpty(item.Profile, defaults.Profile, string(contracts.ParseProfileDefault))
	engine := firstNonEmpty(item.Engine, defaults.Engine, string(contracts.ParseEngineLocal))

	started := time.Now()
	artifact, err := module.Service.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   filepath.Base(filePath),
		Data:       data,
		Intent:     contracts.ParseIntent(intent),
		Profile:    contracts.ParseProfile(profile),
		EngineHint: contracts.ParseEngine(engine),
	})
	result.DurationMS = time.Since(started).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Artifact = artifactReport{
		Status:       string(artifact.Status),
		QualityLevel: string(artifact.QualityLevel),
		EngineUsed:   string(artifact.EngineUsed),
		FallbackUsed: artifact.FallbackUsed,
		TextLength:   len(artifact.Text),
		MarkdownLen:  len(artifact.Markdown),
		ElementCount: len(artifact.Elements),
		Diagnostics:  artifact.Diagnostics,
	}

	if err := assertArtifact(item.Expect, artifact, result.DurationMS); err != nil {
		result.Error = err.Error()
		result.Status = string(artifact.Status)
		return result
	}

	result.Status = string(artifact.Status)
	result.Passed = true
	return result
}

func assertArtifact(expect expectation, artifact *contracts.ParseArtifact, durationMS int64) error {
	if artifact == nil {
		return fmt.Errorf("artifact is nil")
	}
	if expect.Status != "" && string(artifact.Status) != expect.Status {
		return fmt.Errorf("status=%q want=%q", artifact.Status, expect.Status)
	}
	if expect.MinElements > 0 && len(artifact.Elements) < expect.MinElements {
		return fmt.Errorf("elements=%d want >= %d", len(artifact.Elements), expect.MinElements)
	}
	for _, want := range expect.ContainsText {
		if !strings.Contains(artifact.Markdown, want) && !strings.Contains(artifact.Text, want) {
			return fmt.Errorf("missing expected text fragment %q", want)
		}
	}
	if expect.MaxDurationMS > 0 && durationMS > expect.MaxDurationMS {
		return fmt.Errorf("duration=%dms want <= %dms", durationMS, expect.MaxDurationMS)
	}
	return nil
}

func resolveFixturePath(manifestDir, raw string) string {
	expanded := os.ExpandEnv(strings.TrimSpace(raw))
	if filepath.IsAbs(expanded) {
		return expanded
	}
	return filepath.Clean(filepath.Join(manifestDir, expanded))
}

func checkRequirements(req requirements) (bool, string) {
	for _, key := range req.Env {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			return true, fmt.Sprintf("missing env %s", key)
		}
	}
	for _, tool := range req.Tools {
		if _, err := exec.LookPath(tool); err != nil {
			return true, fmt.Sprintf("missing tool %s", tool)
		}
	}
	if len(req.AnyTools) > 0 {
		for _, tool := range req.AnyTools {
			if _, err := exec.LookPath(tool); err == nil {
				return false, ""
			}
		}
		return true, fmt.Sprintf("missing any of tools: %s", strings.Join(req.AnyTools, ", "))
	}
	return false, ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeJSONReport(path string, report suiteReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printSummary(report suiteReport) {
	fmt.Printf("contentparse benchmark: %s\n", report.Name)
	for _, item := range report.Results {
		switch {
		case item.Skipped:
			fmt.Printf("- %s: SKIP (%s)\n", item.ID, item.SkipReason)
		case item.Passed:
			fmt.Printf("- %s: PASS [%s] %dms elements=%d fallback=%v\n", item.ID, item.Artifact.EngineUsed, item.DurationMS, item.Artifact.ElementCount, item.Artifact.FallbackUsed)
		default:
			fmt.Printf("- %s: FAIL (%s)\n", item.ID, item.Error)
		}
	}
	fmt.Printf("summary: total=%d passed=%d failed=%d skipped=%d\n", report.Total, report.Passed, report.Failed, report.Skipped)
	fmt.Printf("metrics: avg_duration=%.1fms p95_duration=%dms avg_text=%.1f avg_elements=%.1f fallback=%d low_text_docs=%d\n",
		report.Metrics.AvgDurationMS,
		report.Metrics.P95DurationMS,
		report.Metrics.AvgTextLength,
		report.Metrics.AvgElementCount,
		report.Metrics.FallbackCount,
		report.Metrics.LowTextLengthDocs,
	)
}

func summarizeMetrics(results []caseReport) benchMetrics {
	if len(results) == 0 {
		return benchMetrics{}
	}
	durations := make([]int64, 0, len(results))
	totalDuration := int64(0)
	totalText := 0
	totalElements := 0
	fallbackCount := 0
	lowTextDocs := 0
	count := 0

	for _, result := range results {
		if result.Skipped || result.Error != "" {
			continue
		}
		count++
		totalDuration += result.DurationMS
		durations = append(durations, result.DurationMS)
		totalText += result.Artifact.TextLength
		totalElements += result.Artifact.ElementCount
		if result.Artifact.FallbackUsed {
			fallbackCount++
		}
		if result.Artifact.TextLength < 300 {
			lowTextDocs++
		}
	}
	if count == 0 {
		return benchMetrics{}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95Idx := int(float64(len(durations))*0.95) - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	if p95Idx >= len(durations) {
		p95Idx = len(durations) - 1
	}
	return benchMetrics{
		AvgDurationMS:     float64(totalDuration) / float64(count),
		P95DurationMS:     durations[p95Idx],
		AvgTextLength:     float64(totalText) / float64(count),
		AvgElementCount:   float64(totalElements) / float64(count),
		FallbackCount:     fallbackCount,
		LowTextLengthDocs: lowTextDocs,
	}
}
