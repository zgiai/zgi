package workflowtest

import (
	"context"
	"fmt"
	"strings"
)

type RecognizeScenariosRequest struct {
	Context  string `json:"context,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	CaseMode string `json:"case_mode,omitempty"`
	Model    *Model `json:"model,omitempty"`
}

type ScenarioRecognitionInput struct {
	AgentID           string
	Context           string
	WorkflowContext   string
	Prompt            string
	CaseMode          string
	Model             *Model
	Cases             []Case
	ExistingScenarios []Scenario
}

type Model struct {
	Provider string `json:"provider,omitempty"`
	Name     string `json:"name,omitempty"`
}

type RecognizedScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RecognizedCaseAssignment struct {
	CaseID       string `json:"case_id"`
	ScenarioName string `json:"scenario_name"`
}

type ScenarioRecognitionResult struct {
	Scenarios   []RecognizedScenario       `json:"scenarios,omitempty"`
	Assignments []RecognizedCaseAssignment `json:"assignments,omitempty"`
	Cases       []Case                     `json:"cases,omitempty"`
}

func (r *ScenarioRecognitionResult) RecognizedCount() int {
	if r == nil {
		return 0
	}
	return len(r.Scenarios)
}

func (r *ScenarioRecognitionResult) AssignedCaseCount() int {
	if r == nil {
		return 0
	}
	return len(r.Assignments)
}

type ScenarioRecognizer interface {
	RecognizeScenarios(ctx context.Context, req ScenarioRecognitionInput) (*ScenarioRecognitionResult, error)
}

func normalizeRecognizedScenarioName(name string) string {
	return strings.TrimSpace(name)
}

func normalizeScenarioRecognitionResult(result *ScenarioRecognitionResult) (*ScenarioRecognitionResult, error) {
	if result == nil {
		return nil, fmt.Errorf("scenario recognition result is empty")
	}
	scenarios := make([]RecognizedScenario, 0, len(result.Scenarios))
	seen := map[string]struct{}{}
	for _, scenario := range result.Scenarios {
		name := normalizeRecognizedScenarioName(scenario.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		scenarios = append(scenarios, RecognizedScenario{
			Name:        name,
			Description: strings.TrimSpace(scenario.Description),
		})
	}
	assignments := make([]RecognizedCaseAssignment, 0, len(result.Assignments))
	for _, assignment := range result.Assignments {
		caseID := strings.TrimSpace(assignment.CaseID)
		scenarioName := normalizeRecognizedScenarioName(assignment.ScenarioName)
		if caseID == "" || scenarioName == "" {
			continue
		}
		assignments = append(assignments, RecognizedCaseAssignment{
			CaseID:       caseID,
			ScenarioName: scenarioName,
		})
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("scenario recognition result is empty")
	}
	return &ScenarioRecognitionResult{Scenarios: scenarios, Assignments: assignments}, nil
}
