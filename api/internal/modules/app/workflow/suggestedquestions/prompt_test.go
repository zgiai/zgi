package suggestedquestions

import (
	"strings"
	"testing"
)

func TestBuildPromptsUseRegisteredTemplates(t *testing.T) {
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(systemPrompt, "Return one valid JSON object only") {
		t.Fatalf("system prompt was not rendered from expected template: %q", systemPrompt)
	}

	userPrompt, err := buildUserPrompt(WorkflowContext{
		Locale:    "zh-Hans",
		AgentName: "Enterprise Assistant Starter",
	}, 3)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"Use Simplified Chinese.",
		"Generate 3 suggested first questions",
		"AI app",
		`"agent_name": "Enterprise Assistant Starter"`,
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestBuildUserPromptRecognizesPersistedChatWorkflowType(t *testing.T) {
	userPrompt, err := buildUserPrompt(WorkflowContext{
		Locale:       "en-US",
		WorkflowType: "chat",
		Conversation: &ConversationSummary{QueryRole: QueryRoleRouteSelector},
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(userPrompt, "conversational workflow web app") {
		t.Fatalf("user prompt did not recognize chat workflow type:\n%s", userPrompt)
	}
	if !strings.Contains(userPrompt, `"query_role": "route_selector"`) {
		t.Fatalf("user prompt missing conversation semantics:\n%s", userPrompt)
	}
}
