package workflowtest

import (
	"context"
	"fmt"
	"strings"
)

const (
	minGeneratedCaseCount = 1
	maxGeneratedCaseCount = 50
)

type GenerateCasesRequest struct {
	Count         int      `json:"count"`
	ScenarioID    string   `json:"scenario_id,omitempty"`
	ScenarioIDs   []string `json:"scenario_ids,omitempty"`
	Context       string   `json:"context,omitempty"`
	QuestionTypes []string `json:"question_types,omitempty"`
	TurnStrategy  string   `json:"turn_strategy,omitempty"`
	Prompt        string   `json:"prompt,omitempty"`
	Model         *Model   `json:"model,omitempty"`
}

type GeneratedCase struct {
	Content        string `json:"content"`
	ExpectedResult string `json:"expected_result"`
	QuestionType   string `json:"question_type"`
}

type GenerateCasesResult struct {
	Cases []GeneratedCase `json:"cases"`
	Items []Case          `json:"items,omitempty"`
}

type CaseGenerator interface {
	GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error)
}

func normalizeGeneratedCases(result *GenerateCasesResult) ([]GeneratedCase, error) {
	if result == nil || len(result.Cases) == 0 {
		return nil, fmt.Errorf("generated cases are empty")
	}
	seen := map[string]struct{}{}
	items := make([]GeneratedCase, 0, len(result.Cases))
	for _, item := range result.Cases {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		if _, exists := seen[content]; exists {
			continue
		}
		seen[content] = struct{}{}
		questionType := strings.TrimSpace(item.QuestionType)
		if questionType == "" {
			questionType = CaseTypeCore
		}
		expectedResult := strings.TrimSpace(item.ExpectedResult)
		items = append(items, GeneratedCase{
			Content:        content,
			ExpectedResult: expectedResult,
			QuestionType:   normalizeGeneratedQuestionType(questionType),
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("generated cases are empty")
	}
	return items, nil
}

func normalizeGeneratedQuestionType(value string) string {
	switch strings.TrimSpace(value) {
	case CaseTypeCore, "核心问题":
		return CaseTypeCore
	case CaseTypeExtension, "扩展问法":
		return CaseTypeExtension
	case CaseTypeFuzzy, "模糊问题":
		return CaseTypeFuzzy
	default:
		return CaseTypeCore
	}
}
