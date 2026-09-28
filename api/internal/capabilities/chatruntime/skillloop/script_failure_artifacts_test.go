package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type diagnosticFailureScriptRunner struct{}

func (diagnosticFailureScriptRunner) Configured() bool { return true }
func (diagnosticFailureScriptRunner) RunSkillScript(_ context.Context, _ skills.SkillDocument, _ map[string]interface{}, _ skills.ExecutionContext, _ string) (*skills.ToolInvocationResult, error) {
	return &skills.ToolInvocationResult{
		Trace: skills.SkillTrace{Kind: "tool_call", SkillID: "diagnostic-script", ToolName: skills.SkillScriptToolRun, Status: "error", Error: "script exited with code 1", Result: map[string]interface{}{"exit_code": 1, "package_sha256": "verified-package"}},
		Messages: []tools.ToolInvokeMessage{
			{Type: tools.ToolInvokeMessageTypeJSON, Data: map[string]interface{}{"status": "failed", "diagnostics": map[string]interface{}{"error_type": "ValueError", "message": "input rejected"}}},
			{Type: tools.ToolInvokeMessageTypeFile, Text: "https://files.example.test/diagnostic.json", Meta: map[string]interface{}{"file": map[string]interface{}{"id": "diagnostic-file", "name": "diagnostic.json", "download_url": "https://files.example.test/diagnostic.json", "size": 40}}},
		},
	}, errors.New("script exited with code 1")
}

func TestFailedScriptPreservesDiagnosticsAndArtifactsWithoutSuccess(t *testing.T) {
	var artifacts []map[string]interface{}
	var events []Event
	r := &Runner{SkillRuntime: skills.NewRuntime(nil, nil).WithScriptRunner(diagnosticFailureScriptRunner{}), OnArtifact: func(x map[string]interface{}) { artifacts = append(artifacts, x) }, OnEvent: func(e Event) error { events = append(events, e); return nil }}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{Metadata: skills.SkillMetadata{ID: "diagnostic-script", ScriptsSupported: true}, Tools: []skills.SkillToolDefinition{{Name: skills.SkillScriptToolRun}}}}}
	step := r.handleCallSkillTool(context.Background(), NewPreparedChat("conversation", "message", "provider", "auto", nil), resolved, "call-failure", map[string]interface{}{"skill_id": "diagnostic-script", "tool_name": skills.SkillScriptToolRun, "arguments": map[string]interface{}{}, "plan_phase_id": "execute"}, skills.ExecutionContext{}, nil, nil)
	if !step.recoverable || step.trace.Status != "error" {
		t.Fatalf("failure became success: %#v", step.trace)
	}
	if step.trace.Result["package_sha256"] != "verified-package" || step.trace.Arguments["plan_phase_id"] != "execute" {
		t.Fatalf("lost runtime evidence: %#v", step.trace)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(step.toolMessage.Content.(string)), &payload); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload["execution_output"])
	if !strings.Contains(string(encoded), "input rejected") || !strings.Contains(string(encoded), "diagnostic-file") {
		t.Fatalf("missing execution diagnostics: %s", encoded)
	}
	if len(artifacts) != 1 {
		t.Fatalf("recorded %d artifacts, want 1", len(artifacts))
	}
	count := 0
	for _, e := range events {
		if e.Type == EventSkillArtifactCreated {
			count++
		}
		if e.Type == EventSkillCallEnd {
			t.Fatal("failed execution emitted success completion")
		}
	}
	if count != 1 {
		t.Fatalf("artifact events=%d, want 1", count)
	}
}

type nonExecutionScriptFailure struct{ diagnosticFailureScriptRunner }

func (s nonExecutionScriptFailure) RunSkillScript(ctx context.Context, doc skills.SkillDocument, args map[string]interface{}, execCtx skills.ExecutionContext, callID string) (*skills.ToolInvocationResult, error) {
	invocation, err := s.diagnosticFailureScriptRunner.RunSkillScript(ctx, doc, args, execCtx, callID)
	delete(invocation.Trace.Result, "exit_code")
	return invocation, err
}

func TestScriptErrorWithoutExecutionEvidenceDoesNotExposeOutput(t *testing.T) {
	artifacts := 0
	r := &Runner{SkillRuntime: skills.NewRuntime(nil, nil).WithScriptRunner(nonExecutionScriptFailure{}), OnArtifact: func(map[string]interface{}) { artifacts++ }}
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{Metadata: skills.SkillMetadata{ID: "diagnostic-script", ScriptsSupported: true}, Tools: []skills.SkillToolDefinition{{Name: skills.SkillScriptToolRun}}}}}
	step := r.handleCallSkillTool(context.Background(), NewPreparedChat("conversation", "message", "provider", "auto", nil), resolved, "call-validation", map[string]interface{}{"skill_id": "diagnostic-script", "tool_name": skills.SkillScriptToolRun, "arguments": map[string]interface{}{}}, skills.ExecutionContext{}, nil, nil)
	if artifacts != 0 || strings.Contains(step.toolMessage.Content.(string), "execution_output") {
		t.Fatal("non-execution failure exposed script output")
	}
	if step.trace.Status != "error" {
		t.Fatalf("status=%s, want error", step.trace.Status)
	}
}
