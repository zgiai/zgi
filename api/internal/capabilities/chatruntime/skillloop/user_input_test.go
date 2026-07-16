package skillloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRequestUserInputCallTerminatesTurnAndEmitsEvent(t *testing.T) {
	events := make([]Event, 0)
	runner := &Runner{
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "openai", "required", &adapter.ChatRequest{})
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		prepared,
		&skills.ResolvedSkills{},
		adapter.ToolCall{
			ID:   "call_ask",
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolRequestUserInput,
				Arguments: `{"message":"I found multiple candidate sheets and need your choice before editing the file.","questions":[{"id":"sheet","question":"Which sheet should I process?","options":[{"label":"Water"},{"label":"Electricity","description":"Use the electricity sheet"}]},{"question":"Should I include a summary?","options":[{"label":"Yes"},{"label":"No"}]}]}`,
			},
		},
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{},
		map[string]interface{}{},
		1,
		nil,
	)

	if !result.terminal {
		t.Fatalf("terminal = false, want true")
	}
	if result.recoverable || result.fatalErr != nil {
		t.Fatalf("unexpected error state: recoverable=%v fatal=%v", result.recoverable, result.fatalErr)
	}
	if result.trace.Kind != "user_input_request" || result.trace.Message != "I found multiple candidate sheets and need your choice before editing the file." {
		t.Fatalf("trace = %#v, want user input request trace", result.trace)
	}
	if result.answer != "" {
		t.Fatalf("answer = %q, want no premature final answer", result.answer)
	}
	if result.pendingUserInput == nil || result.pendingUserInput["request_id"] != "call_ask" {
		t.Fatalf("pendingUserInput = %#v, want request payload", result.pendingUserInput)
	}
	if len(events) != 1 || events[0].Type != EventUserInputRequested {
		t.Fatalf("events = %#v, want one user_input_requested event", events)
	}
	if events[0].Payload["request_id"] != "call_ask" {
		t.Fatalf("payload = %#v, want request id", events[0].Payload)
	}
	if events[0].Payload["message"] != "I found multiple candidate sheets and need your choice before editing the file." {
		t.Fatalf("payload = %#v, want visible request message", events[0].Payload)
	}
	questions, ok := events[0].Payload["questions"].([]map[string]interface{})
	if !ok || len(questions) != 2 || questions[0]["question"] != "Which sheet should I process?" {
		t.Fatalf("questions = %#v, want normalized questions", events[0].Payload["questions"])
	}
	options, ok := questions[0]["options"].([]map[string]interface{})
	if !ok || len(options) != 2 || options[1]["description"] != "Use the electricity sheet" {
		t.Fatalf("options = %#v, want normalized options", questions[0]["options"])
	}
}

func TestRequestUserInputNestedInCallSkillToolTerminatesTurn(t *testing.T) {
	events := make([]Event, 0)
	runner := &Runner{
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "openai", "required", &adapter.ChatRequest{})
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		prepared,
		&skills.ResolvedSkills{},
		adapter.ToolCall{
			ID:   "call_nested_ask",
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolCallSkillTool,
				Arguments: `{"skill_id":"agent-management","tool_name":"request_user_input","arguments":{"message":"I need you to choose one existing Agent.","questions":[{"id":"agent","question":"Which Agent should I use?","options":[{"label":"Use existing"}]}]}}`,
			},
		},
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{},
		map[string]interface{}{},
		1,
		nil,
	)

	if !result.terminal || result.recoverable || result.fatalErr != nil {
		t.Fatalf("result = %#v, want terminal user input request", result)
	}
	if result.trace.Kind != "user_input_request" || result.trace.Status != "success" {
		t.Fatalf("trace = %#v, want successful user input request", result.trace)
	}
	if result.pendingUserInput == nil || result.pendingUserInput["request_id"] != "call_nested_ask" {
		t.Fatalf("pendingUserInput = %#v, want nested request payload", result.pendingUserInput)
	}
	if len(events) != 1 || events[0].Type != EventUserInputRequested {
		t.Fatalf("events = %#v, want one user_input_requested event", events)
	}
	if events[0].Payload["request_id"] != "call_nested_ask" {
		t.Fatalf("payload = %#v, want nested call id as request id", events[0].Payload)
	}
}

func TestRequestUserInputRejectsInvalidArguments(t *testing.T) {
	runner := &Runner{}
	prepared := NewPreparedChat("conv-1", "msg-1", "openai", "auto", &adapter.ChatRequest{})
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		prepared,
		&skills.ResolvedSkills{},
		adapter.ToolCall{
			ID:   "call_ask",
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolRequestUserInput,
				Arguments: `{"questions":[{"question":"","options":[{"label":"A"}]}]}`,
			},
		},
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{},
		map[string]interface{}{},
		1,
		nil,
	)

	if !result.recoverable || result.terminal {
		t.Fatalf("recoverable=%v terminal=%v, want recoverable non-terminal error", result.recoverable, result.terminal)
	}
	if result.trace.Kind != "user_input_request" || result.trace.Status != "error" {
		t.Fatalf("trace = %#v, want failed user input request trace", result.trace)
	}
}

func TestRunnerProtocolOnlyExposesUserInputWithoutBusinessTools(t *testing.T) {
	ctx := context.Background()
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_ask",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolRequestUserInput,
								Arguments: `{"message":"I need one more decision before continuing.","questions":[{"id":"target","question":"Which target should I use?","options":[{"label":"Current page"}]}]}`,
							},
						}},
					},
				}},
			},
		},
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: skills.NewRuntimeWithCatalog(nil, nil, ""),
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "continue the task"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared:          prepared,
		Resolved:          &skills.ResolvedSkills{},
		ProtocolToolsOnly: true,
	})

	var pending *UserInputPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("Run() error = %v, want UserInputPendingError", err)
	}
	if pending.Payload["request_id"] != "call_ask" {
		t.Fatalf("pending payload = %#v, want request id", pending.Payload)
	}
	if answer != "" {
		t.Fatalf("answer = %q, want no premature final answer", answer)
	}
	if pending.Payload["message"] != "I need one more decision before continuing." {
		t.Fatalf("pending payload = %#v, want visible pending message", pending.Payload)
	}
	if len(fakeLLM.appChatRequests) != 1 {
		t.Fatalf("AppChat requests = %d, want 1", len(fakeLLM.appChatRequests))
	}
	exposed := map[string]bool{}
	for _, tool := range fakeLLM.appChatRequests[0].Tools {
		exposed[tool.Function.Name] = true
	}
	if !exposed[skills.MetaToolRequestUserInput] {
		t.Fatalf("tools = %#v, want %s", exposed, skills.MetaToolRequestUserInput)
	}
	for _, forbidden := range []string{skills.MetaToolLoadSkill, skills.MetaToolReadSkillReference, skills.MetaToolCallSkillTool} {
		if exposed[forbidden] {
			t.Fatalf("tools = %#v, protocol-only run must not expose %s", exposed, forbidden)
		}
	}
}

func TestRunnerRejectsEmptyResolvedWithoutProtocolOnlyMode(t *testing.T) {
	runner := &Runner{
		LLMClient:  &runnerTestLLMClient{},
		AppContext: &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "disabled", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared: prepared,
		Resolved: &skills.ResolvedSkills{},
	})
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "no skills available") {
		t.Fatalf("Run() error = %v, want empty discovery rejection", err)
	}
}

func TestRequestUserInputRequiresVisibleMessage(t *testing.T) {
	runner := &Runner{}
	prepared := NewPreparedChat("conv-1", "msg-1", "openai", "auto", &adapter.ChatRequest{})
	result := runner.handleProgressiveSkillCall(
		context.Background(),
		prepared,
		&skills.ResolvedSkills{},
		adapter.ToolCall{
			ID:   "call_ask",
			Type: "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolRequestUserInput,
				Arguments: `{"questions":[{"question":"Which sheet should I process?","options":[{"label":"Water"}]}]}`,
			},
		},
		skills.ExecutionContext{},
		0,
		map[string]int{},
		map[string]struct{}{},
		map[string]interface{}{},
		1,
		nil,
	)

	if !result.recoverable || result.terminal {
		t.Fatalf("recoverable=%v terminal=%v, want recoverable non-terminal error", result.recoverable, result.terminal)
	}
	if result.trace.Kind != "user_input_request" || result.trace.Status != "error" {
		t.Fatalf("trace = %#v, want failed user input request trace", result.trace)
	}
}
