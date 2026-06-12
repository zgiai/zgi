package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestKnowledgeSystemSkillsExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillInternalKnowledge, SkillAgentKnowledge})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	internal, ok := resolved.Get(SkillInternalKnowledge)
	if !ok {
		t.Fatalf("internal knowledge skill was not resolved")
	}
	if got := toolNames(internal.Tools); !sameStrings(got, []string{"list_accessible_knowledge_bases", "retrieve_knowledge"}) {
		t.Fatalf("internal knowledge tools = %v", got)
	}
	if internal.Metadata.MaxCallsPerTurn != 20 {
		t.Fatalf("internal knowledge max calls = %d, want 20", internal.Metadata.MaxCallsPerTurn)
	}
	if internal.Metadata.Display.Label["zh_Hans"] != "内部知识库" {
		t.Fatalf("internal knowledge zh label = %q", internal.Metadata.Display.Label["zh_Hans"])
	}
	agent, ok := resolved.Get(SkillAgentKnowledge)
	if !ok {
		t.Fatalf("agent knowledge skill was not resolved")
	}
	if got := toolNames(agent.Tools); !sameStrings(got, []string{"retrieve_agent_knowledge"}) {
		t.Fatalf("agent knowledge tools = %v", got)
	}
	if agent.Metadata.MaxCallsPerTurn != 20 {
		t.Fatalf("agent knowledge max calls = %d, want 20", agent.Metadata.MaxCallsPerTurn)
	}
	if agent.Metadata.Display.Label["zh_Hans"] != "智能体知识库" {
		t.Fatalf("agent knowledge zh label = %q", agent.Metadata.Display.Label["zh_Hans"])
	}
	if strings.Contains(agent.Metadata.Display.Description["zh_Hans"], "�") || strings.Contains(agent.Metadata.Display.Description["zh_Hans"], "?") {
		t.Fatalf("agent knowledge zh description looks corrupted: %q", agent.Metadata.Display.Description["zh_Hans"])
	}
}

func TestDatabaseSystemSkillsExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillInternalDatabase, SkillAgentDatabase})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	expectedTools := []string{
		"list_accessible_databases",
		"list_database_tables",
		"describe_database_table",
		"query_table_records",
		"insert_table_records",
		"update_table_records",
		"delete_table_records",
	}
	internal, ok := resolved.Get(SkillInternalDatabase)
	if !ok {
		t.Fatalf("internal database skill was not resolved")
	}
	if got := toolNames(internal.Tools); !sameStrings(got, expectedTools) {
		t.Fatalf("internal database tools = %v", got)
	}
	if internal.Metadata.MaxCallsPerTurn != 40 {
		t.Fatalf("internal database max calls = %d, want 40", internal.Metadata.MaxCallsPerTurn)
	}
	agent, ok := resolved.Get(SkillAgentDatabase)
	if !ok {
		t.Fatalf("agent database skill was not resolved")
	}
	if got := toolNames(agent.Tools); !sameStrings(got, expectedTools) {
		t.Fatalf("agent database tools = %v", got)
	}
	if agent.Metadata.MaxCallsPerTurn != 40 {
		t.Fatalf("agent database max calls = %d, want 40", agent.Metadata.MaxCallsPerTurn)
	}
}

func TestAgentWorkflowSystemSkillExposeExpectedTools(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillAgentWorkflow})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	agent, ok := resolved.Get(SkillAgentWorkflow)
	if !ok {
		t.Fatalf("agent workflow skill was not resolved")
	}
	expectedTools := []string{"get_workflow_run_status", "list_agent_workflows", "run_agent_workflow"}
	if got := toolNames(agent.Tools); !sameStrings(got, expectedTools) {
		t.Fatalf("agent workflow tools = %v", got)
	}
	if !sameStrings(agent.Metadata.SupportedCallers, []string{SkillCallerAgent}) {
		t.Fatalf("supported callers = %#v, want agent", agent.Metadata.SupportedCallers)
	}
	if !sameStrings(agent.Metadata.RequiredConfig, []string{SkillRequiredConfigAgentWorkflow}) {
		t.Fatalf("required config = %#v, want agent_workflow", agent.Metadata.RequiredConfig)
	}
	if !IsHiddenSystemSkill(SkillAgentWorkflow) {
		t.Fatal("agent-workflow should be hidden")
	}
	if got := ExpectedSkillToolArguments(SkillAgentWorkflow, "run_agent_workflow"); got == nil {
		t.Fatal("run_agent_workflow contract missing")
	}
}

func TestWorkReportSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillWorkReport})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillWorkReport)
	if !ok {
		t.Fatalf("work report skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeHybrid {
		t.Fatalf("runtime type = %q, want hybrid", doc.Metadata.RuntimeType)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "周报月报生成" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "当用户需要生成周报、月报、工作总结、项目进展汇报或管理汇报时使用。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"周报", "月报", "工作总结"}) {
		t.Fatalf("zh tags = %v", got)
	}
}

func TestSchedulePlannerSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillSchedulePlanner})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillSchedulePlanner)
	if !ok {
		t.Fatalf("schedule planner skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "日程规划" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "用于规划每日安排、每周计划、任务排期、会议议程、学习计划或工作负载。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"日程", "计划", "效率"}) {
		t.Fatalf("zh tags = %v", got)
	}
}

func TestContentSummarySystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillContentSummary})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillContentSummary)
	if !ok {
		t.Fatalf("content summary skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected content summary not to have scripts")
	}
	if !strings.Contains(doc.Metadata.Description, "TL;DR") || !strings.Contains(doc.Metadata.Description, "action items") {
		t.Fatalf("description does not include expected summary triggers: %q", doc.Metadata.Description)
	}
	if !strings.Contains(doc.Metadata.WhenToUse, "already available") || !strings.Contains(doc.Metadata.WhenToUse, "file-generator") {
		t.Fatalf("when_to_use does not include routing boundaries: %q", doc.Metadata.WhenToUse)
	}
	if !strings.Contains(doc.Instructions, "Read, parse, extract, or inspect uploaded files directly.") {
		t.Fatalf("instructions missing uploaded-file boundary")
	}
	if !strings.Contains(doc.Instructions, "If the user uploads a file and asks to summarize") {
		t.Fatalf("instructions missing uploaded-file summary boundary")
	}
	if !strings.Contains(doc.Instructions, "Language Rules") || !strings.Contains(doc.Instructions, "For Chinese requests, do not use English structural labels") {
		t.Fatalf("instructions missing language consistency rules")
	}
	if len(doc.Metadata.References) != 5 {
		t.Fatalf("references = %#v, want 5 content summary references", doc.Metadata.References)
	}
	for _, path := range []string{"general-summary.md", "action-items.md", "risks-conclusions.md", "meeting-notes.md", "requirements-summary.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestSensitiveRedactionSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillSensitiveRedaction})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillSensitiveRedaction)
	if !ok {
		t.Fatalf("sensitive redaction skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeHybrid {
		t.Fatalf("runtime type = %q, want hybrid", doc.Metadata.RuntimeType)
	}
	if got := toolNames(doc.Tools); !sameStrings(got, []string{"redact_text"}) {
		t.Fatalf("tools = %v, want redact_text", got)
	}
	tool, ok := findSkillTool(*doc, "redact_text")
	if !ok {
		t.Fatalf("expected redact_text tool")
	}
	if tool.ProviderType != "builtin" || tool.ProviderID != "sensitive_redaction" {
		t.Fatalf("tool provider = %s/%s, want builtin/sensitive_redaction", tool.ProviderType, tool.ProviderID)
	}
	for _, trigger := range []string{"PII redaction", "手机号脱敏", "Token 脱敏", "password redaction", "privacy cleanup"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	for _, required := range []string{
		"Do not directly read, parse, extract, OCR, or inspect uploaded files or images.",
		"call `redact_text`",
		"Do not show complete original sensitive values in the final answer.",
		"pass only the redacted content to `file-generator`",
		"rule-based redaction may miss context-dependent",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 5 {
		t.Fatalf("references = %#v, want 5 sensitive redaction references", doc.Metadata.References)
	}
	for _, path := range []string{"personal-identifiers.md", "business-identifiers.md", "secrets-technical.md", "redaction-strategies.md", "document-export.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestEmailWritingSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillEmailWriting})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillEmailWriting)
	if !ok {
		t.Fatalf("email writing skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected email writing not to have scripts")
	}
	for _, trigger := range []string{"business emails", "customer follow-up emails", "meeting invitations", "reminders", "email polishing"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	for _, boundary := range []string{"Do not send emails", "generate attachments", "invent commitments"} {
		if !strings.Contains(doc.Metadata.WhenToUse, boundary) {
			t.Fatalf("when_to_use missing boundary %q: %q", boundary, doc.Metadata.WhenToUse)
		}
	}
	for _, required := range []string{
		"call `request_user_input` instead of writing a normal Markdown clarification",
		"Use placeholders for missing non-critical facts instead of inventing them.",
		"Do not claim the email has been sent, scheduled, saved, or attached.",
		"Do not invent prices, discounts, refunds, compensation, deadlines, legal statements, contract terms, delivery commitments, or approval status.",
		"Do not summarize away user-provided requirements.",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 8 {
		t.Fatalf("references = %#v, want 8 email writing references", doc.Metadata.References)
	}
	for _, path := range []string{"business-email.md", "follow-up-email.md", "meeting-invitation.md", "reminder-email.md", "apology-explanation.md", "announcement-email.md", "report-delivery.md", "polish-existing-draft.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestDecisionSupportSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillDecisionSupport})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillDecisionSupport)
	if !ok {
		t.Fatalf("decision support skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected decision support not to have scripts")
	}
	for _, trigger := range []string{"technical choice", "prioritization", "risk-benefit", "option comparison", "decision review"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	for _, boundary := range []string{"does not read, write, update, delete, or manage agent memory itself", "structured decision support only"} {
		if !strings.Contains(doc.Metadata.WhenToUse, boundary) {
			t.Fatalf("when_to_use missing boundary %q: %q", boundary, doc.Metadata.WhenToUse)
		}
	}
	for _, required := range []string{
		"call `request_user_input` instead of writing a normal Markdown clarification",
		"Agent memory is a lower-layer capability.",
		"Do not read, write, update, delete, or manage memory through this skill.",
		"Do not invent market data, customer commitments, costs, dates, team capacity, legal terms, financial numbers, security facts, or contract clauses.",
		"route that downstream task to the appropriate skill",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 6 {
		t.Fatalf("references = %#v, want 6 decision support references", doc.Metadata.References)
	}
	for _, path := range []string{"product-feature-decision.md", "technical-choice.md", "project-prioritization.md", "customer-solution.md", "management-decision.md", "decision-review.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestMultiDocumentCompareSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillMultiDocumentCompare})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillMultiDocumentCompare)
	if !ok {
		t.Fatalf("multi-document compare skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected multi-document compare not to have scripts")
	}
	for _, trigger := range []string{"合同对比", "PRD对比", "供应商方案比较", "新增删除修改", "conflict clauses"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	if !strings.Contains(doc.Metadata.WhenToUse, "system document parser") || !strings.Contains(doc.Metadata.WhenToUse, "file-generator") {
		t.Fatalf("when_to_use does not include parser/export boundaries: %q", doc.Metadata.WhenToUse)
	}
	for _, required := range []string{
		"Directly read, parse, extract, or inspect uploaded files or file bytes.",
		"system document parser",
		"Do not replace legal review",
		"Every comparison conclusion must be grounded in document source text",
		"Language Rules",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 6 {
		t.Fatalf("references = %#v, want 6 multi-document compare references", doc.Metadata.References)
	}
	for _, path := range []string{"generic-comparison.md", "version-diff.md", "clause-conflict.md", "vendor-comparison.md", "requirements-diff.md", "policy-comparison.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestResumeScreeningSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillResumeScreening})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillResumeScreening)
	if !ok {
		t.Fatalf("resume screening skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected resume screening not to have scripts")
	}
	for _, trigger := range []string{"简历初筛", "JD匹配", "岗位匹配", "面试问题", "resume screening"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	if !strings.Contains(doc.Metadata.WhenToUse, "system document parser") || !strings.Contains(doc.Metadata.WhenToUse, "file-generator") {
		t.Fatalf("when_to_use does not include parser/export boundaries: %q", doc.Metadata.WhenToUse)
	}
	for _, required := range []string{
		"Directly read, parse, extract, or inspect uploaded resume files or file bytes.",
		"Make final hiring, rejection, compensation, title, or ranking decisions.",
		"Do not use gender, age, marital or childbearing status",
		"If no JD is provided, do not output job-fit level",
		"Mark missing or unclear resume information as `简历未体现`",
		"Every screening conclusion must be grounded in resume source text",
		"Language Rules",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 5 {
		t.Fatalf("references = %#v, want 5 resume screening references", doc.Metadata.References)
	}
	for _, path := range []string{"resume-summary.md", "jd-match.md", "screening-criteria.md", "interview-questions.md", "talent-pool-entry.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestImageGeneratorSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillImageGenerator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillImageGenerator)
	if !ok {
		t.Fatalf("image generator skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeTool {
		t.Fatalf("runtime type = %q, want tool", doc.Metadata.RuntimeType)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected image generator not to have scripts")
	}
	if got := toolNames(doc.Tools); !sameStrings(got, []string{"generate_image", "edit_image"}) {
		t.Fatalf("tools = %v, want generate_image and edit_image", got)
	}
	for _, tool := range doc.Tools {
		if tool.ProviderType != "builtin" || tool.ProviderID != "image_generator" {
			t.Fatalf("tool provider = %s/%s, want builtin/image_generator", tool.ProviderType, tool.ProviderID)
		}
	}
	for _, trigger := range []string{"generate image", "text to image", "image variant", "image edit", "background change"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	for _, required := range []string{
		"request_user_input",
		"specific living person's likeness",
		"copyright, trademark, portrait rights, and brand compliance",
		"Do not handle OCR, image recognition, table extraction, or screenshot diagnosis",
		"prompt-plus-reference-URL regeneration",
		"prompt-professionalizer",
		"Direct tool calls are allowed only when the prompt and key parameters are already complete",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 7 {
		t.Fatalf("references = %#v, want 7 image generator references", doc.Metadata.References)
	}
	for _, path := range []string{"text-to-image.md", "reference-variant.md", "image-edit.md", "marketing-material.md", "poster-concept.md", "product-scene.md", "style-guide.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestTicketRoutingSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillTicketRouting})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillTicketRouting)
	if !ok {
		t.Fatalf("ticket routing skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected ticket routing not to have scripts")
	}
	for _, trigger := range []string{"ticket routing", "ticket triage", "issue classification", "urgency", "department routing", "workspace routing"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	if !strings.Contains(doc.Metadata.WhenToUse, "routing recommendations only") || !strings.Contains(doc.Metadata.WhenToUse, "file-generator") {
		t.Fatalf("when_to_use does not include routing/export boundaries: %q", doc.Metadata.WhenToUse)
	}
	for _, required := range []string{
		"does not perform real ticket creation, dispatch",
		"Do not invent workspace IDs",
		"request_user_input",
		"需人工确认",
		"信息不足，需补充确认",
		"Do not claim a ticket was dispatched",
		"Language Rules",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 6 {
		t.Fatalf("references = %#v, want 6 ticket routing references", doc.Metadata.References)
	}
	for _, path := range []string{"generic-routing.md", "property-service.md", "enterprise-department-routing.md", "urgency-level.md", "customer-reply.md", "workspace-mapping.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestPromptProfessionalizerSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillPromptProfessionalizer})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillPromptProfessionalizer)
	if !ok {
		t.Fatalf("prompt professionalizer skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypePrompt {
		t.Fatalf("runtime type = %q, want prompt", doc.Metadata.RuntimeType)
	}
	if len(doc.Tools) != 0 {
		t.Fatalf("tools = %v, want none", doc.Tools)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected prompt professionalizer not to have scripts")
	}
	for _, trigger := range []string{"optimize prompt", "professional prompt", "image prompt", "video prompt", "architecture diagram prompt", "data visualization prompt", "chart prompt"} {
		if !strings.Contains(doc.Metadata.Description, trigger) {
			t.Fatalf("description missing trigger %q: %q", trigger, doc.Metadata.Description)
		}
	}
	if !strings.Contains(doc.Metadata.WhenToUse, "does not directly generate images") || !strings.Contains(doc.Metadata.WhenToUse, "corresponding skill or tool") || !strings.Contains(doc.Metadata.WhenToUse, "required preflight step before professional generation tools") {
		t.Fatalf("when_to_use does not include downstream tool boundaries: %q", doc.Metadata.WhenToUse)
	}
	for _, required := range []string{
		"Preflight Requirement",
		"image-generator",
		"architecture-diagram-generator",
		"chart-generator",
		"request_user_input",
		"Do not directly call downstream generation tools",
		"Before professional generation tools, use this skill as the required preflight",
		"Do not invent facts, data fields, metrics, dimensions, technical modules",
		"默认假设",
		"Do not simply polish wording",
		"Language Rules",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 6 {
		t.Fatalf("references = %#v, want 6 prompt professionalizer references", doc.Metadata.References)
	}
	for _, path := range []string{"image-prompt.md", "video-prompt.md", "architecture-diagram-prompt.md", "data-visualization-prompt.md", "general-prompt-optimization.md", "clarification-rules.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestArchitectureDiagramSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillArchitectureDiagram})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillArchitectureDiagram)
	if !ok {
		t.Fatalf("architecture diagram skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeTool {
		t.Fatalf("runtime type = %q, want tool", doc.Metadata.RuntimeType)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected architecture diagram not to have scripts")
	}
	if got := toolNames(doc.Tools); !sameStrings(got, []string{"generate_architecture_diagram"}) {
		t.Fatalf("tools = %v, want generate_architecture_diagram", got)
	}
	tool, ok := findSkillTool(*doc, "generate_architecture_diagram")
	if !ok {
		t.Fatalf("expected generate_architecture_diagram tool")
	}
	if tool.ProviderType != "builtin" || tool.ProviderID != "architecture_diagram_generator" {
		t.Fatalf("tool provider = %s/%s, want builtin/architecture_diagram_generator", tool.ProviderType, tool.ProviderID)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "架构图生成器" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.Description["zh_Hans"]; got != "根据自然语言或结构化数据生成 SVG 和 HTML 技术架构图。" {
		t.Fatalf("zh description = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "当回答需要生成技术架构图文件时使用。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"架构图", "技术图", "可视化"}) {
		t.Fatalf("zh tags = %v", got)
	}
	for _, trigger := range []string{"system diagram", "AI Agent architecture", "data flow diagram", "sequence diagram", "ER diagram"} {
		if !strings.Contains(doc.Metadata.Description, trigger) && !strings.Contains(doc.Metadata.WhenToUse, trigger) {
			t.Fatalf("metadata missing trigger %q: description=%q when_to_use=%q", trigger, doc.Metadata.Description, doc.Metadata.WhenToUse)
		}
	}
	for _, required := range []string{
		"request_user_input",
		"Read exactly one reference document",
		"Generate SVG and HTML artifacts only",
		"Do not promise PNG, PDF",
		"Unsupported diagram types must be reported as unsupported",
		"prompt-professionalizer",
		"Direct tool calls are allowed only when the diagram type, content, and key rendering requirements are already complete",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 8 {
		t.Fatalf("references = %#v, want 8 architecture diagram references", doc.Metadata.References)
	}
	for _, path := range []string{"diagram-system-architecture.md", "diagram-agent-architecture.md", "diagram-data-flow.md", "diagram-flowchart.md", "diagram-comparison-matrix.md", "diagram-sequence.md", "diagram-state.md", "diagram-er.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestChartGeneratorSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillChartGenerator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillChartGenerator)
	if !ok {
		t.Fatalf("chart generator skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeTool {
		t.Fatalf("runtime type = %q, want tool", doc.Metadata.RuntimeType)
	}
	if doc.Metadata.HasScripts {
		t.Fatalf("expected chart generator not to have scripts")
	}
	if doc.Metadata.ScriptsSupported {
		t.Fatalf("scripts supported = true for builtin chart generator")
	}
	tool, ok := findSkillTool(*doc, "generate_chart")
	if !ok {
		t.Fatalf("expected generate_chart tool")
	}
	if tool.ProviderType != "builtin" || tool.ProviderID != "chart_generator" {
		t.Fatalf("tool provider = %s/%s, want builtin/chart_generator", tool.ProviderType, tool.ProviderID)
	}
	if got := doc.Metadata.Display.Label["zh_Hans"]; got != "图表生成器" {
		t.Fatalf("zh label = %q", got)
	}
	if got := doc.Metadata.Display.WhenToUse["zh_Hans"]; got != "当回答需要生成图表文件时使用。" {
		t.Fatalf("zh when_to_use = %q", got)
	}
	if got := doc.Metadata.Display.Tags["zh_Hans"]; !sameStrings(got, []string{"图表", "可视化", "数据"}) {
		t.Fatalf("zh tags = %v", got)
	}
	for _, required := range []string{
		"request_user_input",
		"Read exactly one reference document",
		"Generate SVG artifacts only",
		"Unsupported chart types must be reported as unsupported",
		"prompt-professionalizer",
		"Direct tool calls are allowed only when the chart type, data mapping, title or purpose, and key rendering requirements are already complete",
	} {
		if !strings.Contains(doc.Instructions, required) {
			t.Fatalf("instructions missing %q", required)
		}
	}
	if len(doc.Metadata.References) != 7 {
		t.Fatalf("references = %#v, want 7 chart references", doc.Metadata.References)
	}
	for _, path := range []string{"chart-radar.md", "chart-bar.md", "chart-line.md", "chart-pie.md", "chart-doughnut.md", "chart-scatter.md", "chart-score-distribution.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestIntentRouterSystemSkillMetadata(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillIntentRouter})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillIntentRouter)
	if !ok {
		t.Fatalf("intent router skill was not resolved")
	}
	if doc.Metadata.RuntimeType != SkillRuntimeTypeHybrid {
		t.Fatalf("runtime type = %q, want hybrid", doc.Metadata.RuntimeType)
	}
	tool, ok := findSkillTool(*doc, "route_intent")
	if !ok {
		t.Fatalf("expected route_intent tool")
	}
	if tool.ProviderType != "builtin" || tool.ProviderID != "intent_router" {
		t.Fatalf("tool provider = %s/%s, want builtin/intent_router", tool.ProviderType, tool.ProviderID)
	}
	if len(doc.Metadata.References) != 4 {
		t.Fatalf("references = %#v, want 4 intent router references", doc.Metadata.References)
	}
	for _, path := range []string{"taxonomy.md", "routing-rules.md", "clarification-rules.md", "payload-examples.md"} {
		if !hasReference(doc.Metadata.References, path) {
			t.Fatalf("references = %#v, missing %s", doc.Metadata.References, path)
		}
	}
}

func TestProfessionalGenerationSkillsAutoIncludePromptProfessionalizer(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	for _, skillID := range []string{SkillImageGenerator, SkillArchitectureDiagram, SkillChartGenerator} {
		t.Run(skillID, func(t *testing.T) {
			resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{skillID})
			if err != nil {
				t.Fatalf("ResolveEnabledSkills() error = %v", err)
			}
			if _, ok := resolved.Get(skillID); !ok {
				t.Fatalf("requested skill %s was not resolved", skillID)
			}
			if _, ok := resolved.Get(SkillPromptProfessionalizer); !ok {
				t.Fatalf("%s did not auto-include %s", skillID, SkillPromptProfessionalizer)
			}
		})
	}
}

func TestProfessionalGenerationToolsRequirePromptProfessionalizerPreflight(t *testing.T) {
	for _, tt := range []struct {
		skillID  string
		toolName string
	}{
		{SkillImageGenerator, "generate_image"},
		{SkillImageGenerator, "edit_image"},
		{SkillArchitectureDiagram, "generate_architecture_diagram"},
		{SkillChartGenerator, "generate_chart"},
	} {
		t.Run(tt.skillID+"/"+tt.toolName, func(t *testing.T) {
			if !RequiresPromptProfessionalizerPreflight(tt.skillID, tt.toolName) {
				t.Fatalf("RequiresPromptProfessionalizerPreflight() = false, want true")
			}
		})
	}
	if RequiresPromptProfessionalizerPreflight(SkillCalculator, "evaluate_expression") {
		t.Fatalf("calculator should not require prompt professionalizer preflight")
	}
}

func TestAgentMemorySystemSkillIsNotLoadable(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	_, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillAgentMemory})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("ResolveEnabledSkills(agent-memory) error = %v, want ErrSkillNotFound", err)
	}
	for _, toolName := range []string{"read_agent_memory", "update_agent_memory", "clear_agent_memory"} {
		if got := ExpectedSkillToolArguments(SkillAgentMemory, toolName); got != nil {
			t.Fatalf("ExpectedSkillToolArguments(agent-memory/%s) = %#v, want nil", toolName, got)
		}
	}
}

func TestUserMemorySystemSkillIsNotLoadable(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	_, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillUserMemory})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("ResolveEnabledSkills(user-memory) error = %v, want ErrSkillNotFound", err)
	}
	for _, toolName := range []string{"read_user_memory", "add_user_memory", "update_user_memory", "delete_user_memory", "list_temporary_memories"} {
		if got := ExpectedSkillToolArguments(SkillUserMemory, toolName); got != nil {
			t.Fatalf("ExpectedSkillToolArguments(user-memory/%s) = %#v, want nil", toolName, got)
		}
	}
}

func TestCustomSkillCannotDeclareTools(t *testing.T) {
	root := t.TempDir()
	content := `---
name: custom-tool-skill
description: Invalid custom skill.
when_to_use: Never.
provider_type: builtin
provider_id: knowledge
runtime_type: prompt
tools:
  - retrieve_knowledge
---

# Invalid

This custom skill should not be allowed to declare tools.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadCustomSkillDocument(root); err == nil {
		t.Fatalf("LoadCustomSkillDocument() error = nil, want custom tool declaration rejection")
	}
}

func TestCalculatorMetaToolArgumentsExposeRequiredExpressionSchema(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillCalculator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	doc, ok := resolved.Get(SkillCalculator)
	if !ok {
		t.Fatalf("calculator skill was not resolved")
	}
	if doc.Metadata.MaxCallsPerTurn != 50 {
		t.Fatalf("calculator max calls = %d, want 50", doc.Metadata.MaxCallsPerTurn)
	}
	metaTools := MetaToolsForSkillState(resolved, map[string]struct{}{SkillCalculator: {}})
	callTool := findMetaTool(metaTools, MetaToolCallSkillTool)
	if callTool == nil {
		t.Fatalf("call_skill_tool meta tool not found")
	}
	params, ok := callTool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", callTool.Function.Parameters)
	}
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters.properties missing")
	}
	arguments, ok := properties["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("arguments schema missing")
	}
	oneOf, ok := arguments["oneOf"].([]interface{})
	if !ok || len(oneOf) == 0 {
		t.Fatalf("arguments.oneOf = %#v, want calculator tool schemas", arguments["oneOf"])
	}
	expressionSchema := findSchemaWithRequired(oneOf, "expression")
	if expressionSchema == nil {
		t.Fatalf("evaluate_expression schema requiring expression not found in %#v", oneOf)
	}
	expressionProperties, _ := expressionSchema["properties"].(map[string]interface{})
	if _, ok := expressionProperties["expression"]; !ok {
		t.Fatalf("expression property missing from %#v", expressionSchema)
	}
}

func TestRequestUserInputMetaToolIsAlwaysExposed(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), []string{SkillCalculator})
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	metaTools := MetaToolsForSkillState(resolved, map[string]struct{}{})
	tool := findMetaTool(metaTools, MetaToolRequestUserInput)
	if tool == nil {
		t.Fatalf("request_user_input meta tool not found")
	}
	params, ok := tool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", tool.Function.Parameters)
	}
	required, _ := params["required"].([]string)
	if len(required) != 2 || required[0] != "message" || required[1] != "questions" {
		t.Fatalf("required = %#v, want message and questions", params["required"])
	}
	properties, _ := params["properties"].(map[string]interface{})
	if _, ok := properties["message"]; !ok {
		t.Fatalf("message property missing from %#v", properties)
	}
	if _, ok := properties["questions"]; !ok {
		t.Fatalf("questions property missing from %#v", properties)
	}
}

func TestExpectedSkillToolArgumentsForCalculator(t *testing.T) {
	expected := ExpectedSkillToolArguments(SkillCalculator, "evaluate_expression")
	if expected == nil {
		t.Fatalf("ExpectedSkillToolArguments() = nil")
	}
	schema, ok := expected["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema type = %T, want map[string]interface{}", expected["schema"])
	}
	if !hasRequired(schema, "expression") {
		t.Fatalf("schema does not require expression: %#v", schema)
	}
	example, ok := expected["example"].(map[string]interface{})
	if !ok || example["expression"] == "" {
		t.Fatalf("example missing expression: %#v", expected["example"])
	}
}

func TestSystemToolSkillsExposeArgumentContracts(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	skillIDs := []string{
		SkillAgentKnowledge,
		SkillAgentDatabase,
		SkillCalculator,
		SkillFileGenerator,
		SkillSensitiveRedaction,
		SkillChartGenerator,
		SkillIntentRouter,
		SkillArchitectureDiagram,
		SkillImageGenerator,
		SkillWorkReport,
		SkillInternalDatabase,
		SkillInternalKnowledge,
		SkillTime,
	}
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), skillIDs)
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	for _, doc := range resolved.Skills {
		for _, tool := range doc.Tools {
			if _, ok := SkillToolArgumentContractFor(doc.Metadata.ID, tool.Name); !ok {
				t.Fatalf("missing argument contract for %s/%s", doc.Metadata.ID, tool.Name)
			}
		}
	}
}

func TestExpectedSkillToolArgumentsForBuiltInRequiredTools(t *testing.T) {
	tests := []struct {
		skillID  string
		toolName string
		required []string
	}{
		{SkillFileGenerator, "generate_file", []string{"content", "format"}},
		{SkillFileGenerator, "generate_docx", []string{"document"}},
		{SkillFileGenerator, "generate_pdf", []string{"html"}},
		{SkillFileGenerator, "generate_pptx", []string{"presentation"}},
		{SkillSensitiveRedaction, "redact_text", []string{"text"}},
		{SkillChartGenerator, "generate_chart", []string{"chart_type", "data"}},
		{SkillIntentRouter, "route_intent", []string{"user_input", "intent_id", "task_type", "confidence", "recommended_action", "evidence", "normalized_request"}},
		{SkillArchitectureDiagram, "generate_architecture_diagram", []string{"diagram_type", "data"}},
		{SkillImageGenerator, "generate_image", []string{"prompt"}},
		{SkillImageGenerator, "edit_image", []string{"image", "edit_instruction"}},
		{SkillWorkReport, "generate_file", []string{"content", "format"}},
		{SkillInternalKnowledge, "retrieve_knowledge", []string{"query", "dataset_ids"}},
		{SkillAgentKnowledge, "retrieve_agent_knowledge", []string{"query"}},
		{SkillInternalDatabase, "query_table_records", []string{"data_source_id", "table_id"}},
		{SkillAgentDatabase, "insert_table_records", []string{"data_source_id", "table_id", "records"}},
		{SkillTime, "date_calculate", []string{"operation"}},
	}
	for _, tt := range tests {
		t.Run(tt.skillID+"/"+tt.toolName, func(t *testing.T) {
			expected := ExpectedSkillToolArguments(tt.skillID, tt.toolName)
			if expected == nil {
				t.Fatalf("ExpectedSkillToolArguments() = nil")
			}
			schema, ok := expected["schema"].(map[string]interface{})
			if !ok {
				t.Fatalf("schema type = %T, want map[string]interface{}", expected["schema"])
			}
			for _, required := range tt.required {
				if !hasRequired(schema, required) {
					t.Fatalf("schema does not require %s: %#v", required, schema)
				}
			}
			example, ok := expected["example"].(map[string]interface{})
			if !ok || len(example) == 0 {
				t.Fatalf("example missing: %#v", expected["example"])
			}
		})
	}
}

func TestChartGeneratorContractSupportsBarAndLinePayloads(t *testing.T) {
	expected := ExpectedSkillToolArguments(SkillChartGenerator, "generate_chart")
	if expected == nil {
		t.Fatalf("ExpectedSkillToolArguments() = nil")
	}
	schema, ok := expected["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema type = %T, want map[string]interface{}", expected["schema"])
	}
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema.properties missing")
	}
	dataSchema, ok := properties["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data schema missing")
	}
	if hasRequired(dataSchema, "dimensions") {
		t.Fatalf("top-level data schema should not require dimensions: %#v", dataSchema)
	}
	branches, ok := dataSchema["anyOf"].([]interface{})
	if !ok || len(branches) < 7 {
		t.Fatalf("data anyOf = %#v, want radar/bar/line/pie/scatter/distribution branches", dataSchema["anyOf"])
	}
	for _, required := range []string{"dimensions", "categories", "x_axis", "items", "points", "bands"} {
		if findSchemaWithRequired(branches, required) == nil {
			t.Fatalf("data schema branch requiring %s not found: %#v", required, branches)
		}
	}
	if findSchemaWithRequired(branches, "scores") == nil {
		t.Fatalf("data schema branch requiring scores not found: %#v", branches)
	}
	for _, rawBranch := range branches {
		branch, ok := rawBranch.(map[string]interface{})
		if !ok || !hasRequired(branch, "bands") {
			continue
		}
		if branchAllowsLabelOnlyBands(branch) {
			t.Fatalf("score distribution bands schema allows label-only bands: %#v", branch)
		}
	}
}

func TestMetaToolArgumentsExposeAllLoadedSystemToolContracts(t *testing.T) {
	runtime := NewRuntimeWithCatalog(nil, nil, "catalog")
	skillIDs := []string{
		SkillAgentKnowledge,
		SkillAgentDatabase,
		SkillCalculator,
		SkillFileGenerator,
		SkillSensitiveRedaction,
		SkillChartGenerator,
		SkillIntentRouter,
		SkillArchitectureDiagram,
		SkillImageGenerator,
		SkillWorkReport,
		SkillInternalDatabase,
		SkillInternalKnowledge,
		SkillTime,
	}
	resolved, err := runtime.ResolveEnabledSkills(context.Background(), skillIDs)
	if err != nil {
		t.Fatalf("ResolveEnabledSkills() error = %v", err)
	}
	loaded := map[string]struct{}{}
	for _, id := range skillIDs {
		loaded[id] = struct{}{}
	}
	metaTools := MetaToolsForSkillState(resolved, loaded)
	callTool := findMetaTool(metaTools, MetaToolCallSkillTool)
	if callTool == nil {
		t.Fatalf("call_skill_tool meta tool not found")
	}
	params, ok := callTool.Function.Parameters.(map[string]interface{})
	if !ok {
		t.Fatalf("parameters type = %T, want map[string]interface{}", callTool.Function.Parameters)
	}
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters.properties missing")
	}
	arguments, ok := properties["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("arguments schema missing")
	}
	if _, hasOneOf := arguments["oneOf"]; hasOneOf {
		t.Fatalf("arguments.oneOf should not be used when optional-only contracts are loaded: %#v", arguments)
	}
	anyOf, ok := arguments["anyOf"].([]interface{})
	if !ok || len(anyOf) < 7 {
		t.Fatalf("arguments.anyOf = %#v, want built-in tool schemas", arguments["anyOf"])
	}
	for _, required := range []string{"content", "query", "operation"} {
		if findSchemaWithRequired(anyOf, required) == nil {
			t.Fatalf("schema requiring %s not found in %#v", required, anyOf)
		}
	}
}

func toolNames(tools []SkillToolDefinition) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name)
	}
	return out
}

func findMetaTool(metaTools []llmadapter.Tool, name string) *llmadapter.Tool {
	for idx := range metaTools {
		if metaTools[idx].Function.Name == name {
			return &metaTools[idx]
		}
	}
	return nil
}

func findSchemaWithRequired(schemas []interface{}, required string) map[string]interface{} {
	for _, raw := range schemas {
		schema, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if hasRequired(schema, required) {
			return schema
		}
	}
	return nil
}

func branchAllowsLabelOnlyBands(branch map[string]interface{}) bool {
	properties, ok := branch["properties"].(map[string]interface{})
	if !ok {
		return false
	}
	bands, ok := properties["bands"].(map[string]interface{})
	if !ok {
		return false
	}
	items, ok := bands["items"].(map[string]interface{})
	if !ok {
		return false
	}
	required, ok := items["required"].([]string)
	if !ok {
		values, ok := items["required"].([]interface{})
		if !ok {
			return true
		}
		required = make([]string, 0, len(values))
		for _, value := range values {
			text, _ := value.(string)
			if text != "" {
				required = append(required, text)
			}
		}
	}
	return len(required) == 1 && required[0] == "label"
}

func hasReference(references []SkillReference, path string) bool {
	for _, reference := range references {
		if reference.Path == path {
			return true
		}
	}
	return false
}

func hasRequired(schema map[string]interface{}, required string) bool {
	values, ok := schema["required"].([]string)
	if ok {
		for _, value := range values {
			if value == required {
				return true
			}
		}
	}
	rawValues, ok := schema["required"].([]interface{})
	if ok {
		for _, value := range rawValues {
			if value == required {
				return true
			}
		}
	}
	return false
}

func hasAnyOfRequired(schema map[string]interface{}, required string) bool {
	rawBranches, ok := schema["anyOf"].([]interface{})
	if !ok {
		return false
	}
	for _, rawBranch := range rawBranches {
		branch, ok := rawBranch.(map[string]interface{})
		if !ok {
			continue
		}
		if hasRequired(branch, required) {
			return true
		}
	}
	return false
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
