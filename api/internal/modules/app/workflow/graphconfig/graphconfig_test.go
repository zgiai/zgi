package graphconfig

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func TestParseNodeTypeSupportsQuestionAnswer(t *testing.T) {
	got, err := ParseNodeType("question-answer")
	if err != nil {
		t.Fatalf("ParseNodeType() error = %v", err)
	}
	if got != shared.QuestionAnswer {
		t.Fatalf("ParseNodeType() = %q, want %q", got, shared.QuestionAnswer)
	}
}
