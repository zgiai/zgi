package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
	workflowbuiltin "github.com/zgiai/zgi/api/internal/modules/tools/builtin/workflow"
)

func TestRunnerAllowsBatchRecoverableSkillToolFailures(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "limited-calculator", `---
name: limited-calculator
description: Calculate with a low per-turn success limit.
when_to_use: Use when testing tool call limits.
provider_type: builtin
provider_id: calculator
runtime_type: tool
tools:
  - evaluate_expression
max_calls_per_turn: 20
---

# Limited Calculator

Use the calculator tool.
`)
	toolCalls := make([]adapter.ToolCall, 0, 10)
	for i := 0; i < 10; i++ {
		toolCalls = append(toolCalls, runnerTestSkillToolCall(
			fmt.Sprintf("call_bad_%d", i),
			"limited-calculator",
			"evaluate_expression",
			map[string]interface{}{"expression": "1/"},
		))
	}
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"limited-calculator"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:      "assistant",
						ToolCalls: toolCalls,
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "replanned after batch failures"},
				}},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatalf("register calculator provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"limited-calculator"})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "calculate several expressions"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want batch failures to be returned before replanning", err)
	}
	if answer != "replanned after batch failures" {
		t.Fatalf("answer = %q, want final answer after batch failure round", answer)
	}
	if fakeLLM.appChatCalls != 3 {
		t.Fatalf("AppChat calls = %d, want 3", fakeLLM.appChatCalls)
	}
}

func TestRunnerForwardsAgentWorkflowEvents(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "agent-workflow", `---
name: agent-workflow
description: Run Agent-bound workflows.
when_to_use: Use when testing Agent workflow event bridging.
provider_type: builtin
provider_id: workflow
runtime_type: tool
tools:
  - run_agent_workflow
---

# Agent Workflow

Use the workflow tool.
`)
	fakeLLM := &runnerTestLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_load",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: `{"skill_id":"agent-workflow"}`,
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{
							runnerTestSkillToolCall("call_workflow", "agent-workflow", "run_agent_workflow", map[string]interface{}{
								"binding_id": "approval-flow",
								"inputs":     map[string]interface{}{"query": "run workflow"},
							}),
						},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "workflow done"},
				}},
			},
		},
	}
	workflowRunner := &runnerTestWorkflowRunner{
		events: []automationaction.WorkflowRunEvent{
			{
				Type: EventWorkflowStarted,
				Payload: map[string]interface{}{
					"workflow_run_id": "run-1",
					"status":          "running",
				},
			},
			{
				Type: EventWorkflowNodeStarted,
				Payload: map[string]interface{}{
					"workflow_run_id": "run-1",
					"node_id":         "node-1",
					"status":          "running",
				},
			},
		},
	}
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(workflowbuiltin.NewProvider(func() automationaction.AutomationWorkflowRunner {
		return workflowRunner
	})); err != nil {
		t.Fatalf("register workflow provider: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(manager), manager, catalogDir)
	resolved, err := runtime.ResolveEnabledSkills(ctx, []string{"agent-workflow"})
	if err != nil {
		t.Fatalf("resolve skills: %v", err)
	}

	var events []Event
	runner := &Runner{
		LLMClient:    fakeLLM,
		SkillRuntime: runtime,
		AppContext:   &llmclient.AppContext{},
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat("conv-1", "msg-1", "", "auto", &adapter.ChatRequest{
		Messages: []adapter.Message{{Role: "user", Content: "run workflow"}},
	})

	answer, _, err := runner.Run(ctx, RunRequest{
		Prepared: prepared,
		Resolved: resolved,
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: "org-1",
			UserID:         "account-1",
			ConversationID: "conv-1",
			MessageID:      "msg-1",
			InvokeFrom:     tools.ToolInvokeFromAgent,
			RuntimeParameters: map[string]interface{}{
				"organization_id": "org-1",
				"workspace_id":    "workspace-1",
				"workflow_bindings": []map[string]interface{}{
					{
						"binding_id":       "approval-flow",
						"label":            "Approval flow",
						"agent_id":         "agent-1",
						"workflow_id":      "workflow-1",
						"version_strategy": "latest_published",
						"timeout_seconds":  60,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "workflow done" {
		t.Fatalf("answer = %q, want workflow done", answer)
	}
	workflowStarted := findRunnerTestEvent(events, EventWorkflowStarted)
	if workflowStarted == nil {
		t.Fatalf("events = %#v, want workflow_started", events)
	}
	if workflowStarted.Payload["conversation_id"] != "conv-1" || workflowStarted.Payload["message_id"] != "msg-1" {
		t.Fatalf("workflow_started payload = %#v, want conversation/message ids", workflowStarted.Payload)
	}
	nodeStarted := findRunnerTestEvent(events, EventWorkflowNodeStarted)
	if nodeStarted == nil || nodeStarted.Payload["node_id"] != "node-1" {
		t.Fatalf("events = %#v, want node_started node-1", events)
	}
}

func writeRunnerTestSkill(t *testing.T, catalogDir string, skillID string, content string) {
	t.Helper()

	root := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func runnerTestSkillToolCall(callID string, skillID string, toolName string, arguments map[string]interface{}) adapter.ToolCall {
	payload, _ := json.Marshal(map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"arguments": arguments,
	})
	return adapter.ToolCall{
		ID:   callID,
		Type: "function",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolCallSkillTool,
			Arguments: string(payload),
		},
	}
}

type runnerTestLLMClient struct {
	appChatResponses []*adapter.ChatResponse
	appChatCalls     int
}

type runnerTestWorkflowRunner struct {
	events []automationaction.WorkflowRunEvent
}

func (f *runnerTestWorkflowRunner) RunAutomationWorkflow(ctx context.Context, req automationaction.WorkflowRunRequest) (*automationaction.WorkflowRunResult, error) {
	_ = ctx
	for _, event := range f.events {
		if req.EventSink != nil {
			req.EventSink(event)
		}
	}
	return &automationaction.WorkflowRunResult{
		WorkflowRunID: "run-1",
		WorkflowID:    req.WorkflowRef.WorkflowID,
		AgentID:       req.WorkflowRef.AgentID,
		Status:        "succeeded",
		Outputs:       map[string]interface{}{},
	}, nil
}

func findRunnerTestEvent(events []Event, eventType string) *Event {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}

func (f *runnerTestLLMClient) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	if f.appChatCalls >= len(f.appChatResponses) {
		return nil, errors.New("unexpected AppChat call")
	}
	resp := f.appChatResponses[f.appChatCalls]
	f.appChatCalls++
	return resp, nil
}

func (f *runnerTestLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *runnerTestLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}
