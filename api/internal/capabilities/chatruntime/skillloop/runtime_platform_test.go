package skillloop

import (
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
)

func TestRuntimePlatformToolsUseLoadedScopeLivePolicyAndParentBudget(t *testing.T) {
	manager := tools.NewToolManager(nil)
	if err := manager.RegisterProvider(calculator.NewProvider()); err != nil {
		t.Fatal(err)
	}
	runtime := skills.NewRuntime(tools.NewToolEngine(manager), manager)
	resolved := nativeCalculatorResolvedSkills()
	set := runtime.BuildNativeToolSet(t.Context(), resolved, skills.NativeToolSetOptions{BudgetChars: 10000})
	loaded := map[string]struct{}{skills.SkillCalculator: {}}
	allowed := true
	req := RunRequest{ExecutionContext: skills.ExecutionContext{OrganizationID: "org-1", UserID: "user-1"}, AuthorizeSkillStep: func(context.Context, string) (bool, error) { return allowed, nil }}
	prepared := NewPreparedChat("conversation-1", "message-1", "", "auto", &adapter.ChatRequest{})
	count := 0
	skillCounts := map[string]int{}
	var successful []SkillToolCallRef
	var traces []skills.SkillTrace
	runner := &Runner{SkillRuntime: runtime}
	services := runner.runtimePlatformServices(req, prepared, resolved, loaded, func() *skills.NativeToolSet { return &set }, &count, skillCounts, &successful, &traces)
	result, err := services.Tool(t.Context(), "cli-call-1", "calculate", json.RawMessage(`{"operation":"multiply","left":6,"right":7}`))
	if err != nil || !json.Valid(result.Data) || !strings.Contains(string(result.Data), "42") || count != 1 || skillCounts[skills.SkillCalculator] != 1 || len(traces) != 1 || len(successful) != 1 {
		t.Fatalf("result=%s count=%d traces=%d err=%v", result.Data, count, len(traces), err)
	}
	allowed = false
	if _, err := services.Tool(t.Context(), "cli-call-2", "calculate", json.RawMessage(`{}`)); err == nil || count != 1 {
		t.Fatal("revoked tool executed")
	}
	allowed = true
	delete(loaded, skills.SkillCalculator)
	if _, err := services.Tool(t.Context(), "cli-call-3", "calculate", json.RawMessage(`{}`)); err == nil || count != 1 {
		t.Fatal("unloaded tool executed")
	}
	for _, name := range []string{"shell_exec", "load_skill", "activate_skills", "run_script", "admin.call"} {
		if _, err := services.Tool(t.Context(), "denied", name, json.RawMessage(`{}`)); err == nil {
			t.Fatalf("unexposed tool %q executed", name)
		}
	}
}

func TestRuntimePlatformGovernedToolReturnsAuthoritativeApproval(t *testing.T) {
	catalogDir := t.TempDir()
	writeRunnerTestSkill(t, catalogDir, "cli-governed-files", `---
name: cli-governed-files
description: Test governed platform call.
when_to_use: Use for platform approval tests.
provider_type: builtin
provider_id: files
runtime_type: tool
tools:
  - delete_file
tool_governance:
  delete_file:
    tool_id: file.delete
    skill_id: cli-governed-files
    domain: files
    effect: delete
    asset_type: file
    risk_level: high
    requires_asset_resolution: true
    permission_scopes:
      - file:manage
    default_approval_policy: always_ask
    allowed_permission_tiers:
      - basic
    audit_required: true
    idempotency_required: false
---
Ask before deleting files.
`)
	// There is deliberately no engine: approval must happen before executing
	// any provider. Use the actual existing policy gateway, not a fake receipt.
	runtime := skills.NewRuntimeWithCatalog(nil, nil, catalogDir).WithToolGovernanceGateway(skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy()))
	resolved, err := runtime.ResolveEnabledSkills(t.Context(), []string{"cli-governed-files"})
	if err != nil {
		t.Fatal(err)
	}
	req := RunRequest{AuthorizeSkillStep: func(context.Context, string) (bool, error) { return true, nil }, ExecutionContext: skills.ExecutionContext{OrganizationID: "org-1", UserID: "user-1", ConversationID: "conversation-1", MessageID: "message-1", RuntimeParameters: map[string]interface{}{"tool_governance": map[string]interface{}{"permission_tier": "basic", "assets": []map[string]interface{}{{"id": "file-1", "type": "file", "name": "report.pdf"}}}}}}
	prepared := NewPreparedChat("conversation-1", "message-1", "", "auto", &adapter.ChatRequest{})
	count := 0
	var successful []SkillToolCallRef
	var traces []skills.SkillTrace
	services := (&Runner{SkillRuntime: runtime}).runtimePlatformServices(req, prepared, resolved, map[string]struct{}{"cli-governed-files": {}}, func() *skills.NativeToolSet { return nil }, &count, map[string]int{}, &successful, &traces)
	result, err := services.Tool(t.Context(), "platform:job-1:delete-1", "delete_file", json.RawMessage(`{"file_id":"file-1"}`))
	if err != nil || result.PendingGovernance["correlation_id"] == "" || !result.NeedsContinuation() || len(successful) != 0 {
		t.Fatalf("pending=%+v successful=%d err=%v", result.PendingGovernance, len(successful), err)
	}
	req.ExecutionContext.RuntimeParameters["tool_governance_caller_type"] = skills.SkillCallerAgent
	req.ExecutionContext.RuntimeParameters["agent_runtime_source"] = "console"
	req.RuntimeTools = []RuntimeTool{{PlatformInteractiveApproval: true, BindPlatform: func(RuntimePlatformServices) {}}}
	services = (&Runner{SkillRuntime: runtime}).runtimePlatformServices(req, prepared, resolved, map[string]struct{}{"cli-governed-files": {}}, func() *skills.NativeToolSet { return nil }, &count, map[string]int{}, &successful, &traces)
	result, err = services.Tool(t.Context(), "platform:job-2:delete-1", "delete_file", json.RawMessage(`{"file_id":"file-1"}`))
	if err != nil || !result.NeedsContinuation() || req.ExecutionContext.PlatformInteractiveApproval {
		t.Fatalf("console CLI pending=%+v err=%v; parent capability must remain false", result, err)
	}
	req.RuntimeTools = nil
	// Even a mistakenly pre-set parent context cannot grant CLI approval.
	req.ExecutionContext.PlatformInteractiveApproval = true
	services = (&Runner{SkillRuntime: runtime}).runtimePlatformServices(req, prepared, resolved, map[string]struct{}{"cli-governed-files": {}}, func() *skills.NativeToolSet { return nil }, &count, map[string]int{}, &successful, &traces)
	result, err = services.Tool(t.Context(), "platform:job-3:delete-1", "delete_file", json.RawMessage(`{"file_id":"file-1"}`))
	if err == nil || result.NeedsContinuation() {
		t.Fatalf("ordinary Agent gained interactive approval: %+v err=%v", result, err)
	}
}

func TestRuntimePlatformPendingUsesExistingContinuation(t *testing.T) {
	pending := map[string]interface{}{"correlation_id": "approval-1"}
	step := handleRuntimeToolCall(t.Context(), adapter.ToolCall{ID: "shell-1"}, RuntimeTool{Handler: func(context.Context, adapter.ToolCall) RuntimeToolResult {
		return RuntimeToolResult{Status: "awaiting_approval", PendingGovernance: pending}
	}})
	if step.pendingGovernance["correlation_id"] != "approval-1" {
		t.Fatal("governance continuation lost")
	}
}
