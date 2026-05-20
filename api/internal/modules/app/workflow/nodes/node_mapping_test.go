package nodes

import (
	"testing"

	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestGetNodeFactorySupportsQuestionAnswer(t *testing.T) {
	factory, err := GetNodeFactory(shared.QuestionAnswer, LatestVersion)
	if err != nil {
		t.Fatalf("GetNodeFactory() error = %v", err)
	}
	if factory == nil {
		t.Fatal("GetNodeFactory() returned nil factory")
	}
}
