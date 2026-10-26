package container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	digitalhumantask "github.com/zgiai/zgi/api/internal/modules/automation/digitalhuman"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationrepo "github.com/zgiai/zgi/api/internal/modules/automation/repository"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationruntime "github.com/zgiai/zgi/api/internal/modules/automation/service/runtime"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	socialtools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/socialoperations"
)

// This fixture exercises the real Automation task/action runtime and the real
// social_operations tool provider. The runner's tool choices and the native
// platform responses are deterministic test fixtures; it is not an LLM or a
// live social account.
type socialAutomationDispatcher struct{ executor *automationruntime.Executor }

func (d *socialAutomationDispatcher) DispatchTaskRun(ctx context.Context, id string) error {
	if d.executor == nil {
		return fmt.Errorf("automation executor unavailable")
	}
	return d.executor.ExecuteTaskRun(ctx, id)
}

type socialAutomationResolver struct {
	scope       socialops.Scope
	gateway     *socialOperationsGateway
	fixture     *socialIntegrationFixture
	inspections *int
}

func (r socialAutomationResolver) ResolveSocialOperations(context.Context, socialtools.InvocationIdentity) (socialops.Scope, socialops.PublicationGateway, error) {
	return r.scope, r.gateway, nil
}

func (r socialAutomationResolver) InspectSocialImage(ctx context.Context, _ socialtools.InvocationIdentity, id, logoID uuid.UUID) (map[string]any, error) {
	if r.fixture == nil || r.inspections == nil {
		return nil, fmt.Errorf("image inspector unavailable")
	}
	artifact, err := r.fixture.service.GetArtifact(ctx, r.scope, id)
	if err != nil || logoID == uuid.Nil {
		return nil, fmt.Errorf("restore inspected image: %w", err)
	}
	if _, err = r.fixture.service.ReadArtifact(ctx, r.scope, id); err != nil {
		return nil, err
	}
	if _, err = r.fixture.service.ReadArtifact(ctx, r.scope, logoID); err != nil {
		return nil, err
	}
	*r.inspections++
	cfg, _, err := r.fixture.service.LoadCurrentConfiguration(ctx, r.scope)
	if err != nil {
		return nil, err
	}
	reviewJSON, _ := json.Marshal(map[string]any{"artifact_id": id, "digest": artifact.Digest, "configuration_revision": cfg.Revision,
		"approved": true, "issues": []string{}, "description": "Controlled fixture inspected the exact composed pixels.",
		"provider": "controlled-fixture", "model": "deterministic-vision", "observed_at": r.fixture.now.UTC()})
	review, err := r.fixture.service.PutArtifact(ctx, r.scope, "application/json", reviewJSON)
	if err != nil {
		return nil, err
	}
	if err = r.fixture.service.RecordImageInspection(ctx, r.scope, id, review.ID, true); err != nil {
		return nil, err
	}
	return map[string]any{"approved": true, "issues": []any{}, "review_artifact_id": review.ID}, nil
}

type socialAutomationRunner struct {
	provider       *socialtools.Provider
	fixture        *socialIntegrationFixture
	conversationID string
	messageID      string
	logoID         uuid.UUID
	publicationID  uuid.UUID
	stages         []string
	toolCalls      []string
}

func (r *socialAutomationRunner) RunDigitalHuman(ctx context.Context, request automationaction.DigitalHumanRunRequest) (*automationaction.DigitalHumanRunResult, error) {
	if request.MaxTaskTokens != 100000 {
		return nil, fmt.Errorf("restored task token budget = %d", request.MaxTaskTokens)
	}
	var err error
	switch request.Name {
	case "daily_content":
		err = r.dailyContent(ctx)
	case "frequent_inbox":
		err = r.inbox(ctx)
	case "daily_seven_day_samples":
		err = r.metrics(ctx)
	case "weekly_review":
		err = r.weekly(ctx)
	default:
		err = fmt.Errorf("unexpected scripted task %q", request.Name)
	}
	if err != nil {
		return nil, err
	}
	return &automationaction.DigitalHumanRunResult{RunID: uuid.NewString()}, nil
}

func (*socialAutomationRunner) GetDigitalHumanRun(context.Context, string, string, string, string) (*automationaction.DigitalHumanRunState, error) {
	return &automationaction.DigitalHumanRunState{Status: "running"}, nil
}
func (*socialAutomationRunner) CancelDigitalHumanRun(context.Context, string, string, string, string) error {
	return nil
}

func (r *socialAutomationRunner) invoke(ctx context.Context, name string, input any) (map[string]any, error) {
	tool, err := r.provider.GetTool(name)
	if err != nil {
		return nil, err
	}
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: r.fixture.scope.OrganizationID.String(), InvokeFrom: tools.ToolInvokeFromAgent})
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	app := r.fixture.scope.EmployeeID.String()
	messages, err := tool.Invoke(ctx, r.fixture.scope.OwnerID.String(), map[string]any{"input": string(raw)}, &r.conversationID, &app, &r.messageID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(messages) != 1 || messages[0].Type != tools.ToolInvokeMessageTypeJSON {
		return nil, fmt.Errorf("%s returned invalid result", name)
	}
	r.toolCalls = append(r.toolCalls, name)
	return messages[0].Data, nil
}

func nested(result map[string]any, key string) map[string]any {
	value, _ := result[key].(map[string]any)
	return value
}
func resultString(result map[string]any, key string) string {
	value, _ := result[key].(string)
	return value
}
func resultInt64(result map[string]any, key string) int64 {
	switch value := result[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	}
	return 0
}

func (r *socialAutomationRunner) save(ctx context.Context, kind, key, content, mime string) (map[string]any, error) {
	return r.invoke(ctx, "save_record", map[string]any{"kind": kind, "key": key, "content": content, "mime": mime, "expected_revision": 0})
}

func (r *socialAutomationRunner) dailyContent(ctx context.Context) error {
	for _, item := range []struct{ kind, key, content string }{
		{"source", "launch_source", "https://example.com/source — verified release notes"},
		{"product_fact", "launch_fact", "The release adds durable scoped social operations."},
		{"topic", "launch_topic", "Explain durable review and verified readback."},
	} {
		if _, err := r.save(ctx, item.kind, item.key, item.content, "text/plain"); err != nil {
			return err
		}
		r.stages = append(r.stages, item.kind)
	}
	composed, err := r.invoke(ctx, "compose_image", map[string]any{"logo_artifact_id": r.logoID, "headline": "Durable social operations", "body": "Reviewed, verified, and archived."})
	if err != nil {
		return err
	}
	imageID := resultString(nested(composed, "artifact"), "id")
	if imageID == "" {
		return fmt.Errorf("compose_image returned no artifact")
	}
	inspection, err := r.invoke(ctx, "inspect_image", map[string]any{"artifact_id": imageID, "logo_artifact_id": r.logoID})
	if err != nil || inspection["approved"] != true || resultString(inspection, "review_artifact_id") == "" {
		return fmt.Errorf("inspect composed image: %w", err)
	}
	r.stages = append(r.stages, "compose")
	bundle, _ := json.Marshal(socialops.DraftBundle{Action: "post", English: "automation", Chinese: "自动化社交运营审核稿", Images: []uuid.UUID{uuid.MustParse(imageID)}})
	draft, err := r.save(ctx, "draft", "automation_post", string(bundle), "application/json")
	if err != nil {
		return err
	}
	artifact := nested(draft, "artifact")
	prepared, err := r.invoke(ctx, "prepare_publication", map[string]any{"connection_id": r.fixture.connection, "account_id": "acct-7", "content_key": "automation_post", "platform": "x", "draft_artifact_id": resultString(artifact, "id")})
	if err != nil {
		return err
	}
	publication := nested(prepared, "publication")
	r.publicationID = uuid.MustParse(resultString(publication, "id"))
	reviewed, err := r.invoke(ctx, "review_publication", map[string]any{"publication_id": r.publicationID, "expected_revision": resultInt64(publication, "revision"), "digest": resultString(artifact, "digest")})
	if err != nil || resultString(nested(reviewed, "publication"), "state") != "reviewed" {
		return fmt.Errorf("review publication: %w", err)
	}
	r.stages = append(r.stages, "review")
	if _, err = r.invoke(ctx, "dispatch_publication", map[string]any{"publication_id": r.publicationID}); err != nil {
		return err
	}
	r.stages = append(r.stages, "publish")
	if _, err = r.invoke(ctx, "write_daily_archive", map[string]any{"date": "2026-10-20"}); err != nil {
		return err
	}
	r.stages = append(r.stages, "archive")
	return nil
}

func (r *socialAutomationRunner) inbox(ctx context.Context) error {
	body, _ := json.Marshal(socialops.OutboundMessage{Text: "Thanks for the feedback.", Chinese: "感谢反馈。"})
	record, err := r.save(ctx, "feedback", "inbox_feedback", string(body), "application/json")
	if err != nil {
		return err
	}
	artifact := nested(record, "artifact")
	prepared, err := r.invoke(ctx, "prepare_outbound", map[string]any{"work_key": "inbox_dm_1", "kind": "dm", "connection_id": r.fixture.connection, "account_id": "acct-7", "conversation_id": "conversation-1", "body_artifact_id": resultString(artifact, "id")})
	if err != nil {
		return err
	}
	out := nested(prepared, "outbound")
	if _, err = r.invoke(ctx, "review_outbound", map[string]any{"outbound_id": resultString(out, "id"), "expected_revision": resultInt64(out, "revision"), "digest": resultString(artifact, "digest")}); err != nil {
		return err
	}
	if _, err = r.invoke(ctx, "dispatch_outbound", map[string]any{"outbound_id": resultString(out, "id")}); err != nil {
		return err
	}
	r.stages = append(r.stages, "inbox_outbox")
	return nil
}

func (r *socialAutomationRunner) metrics(ctx context.Context) error {
	*r.fixture.now = r.fixture.now.Add(7 * 24 * time.Hour)
	if _, err := r.invoke(ctx, "due_metrics", map[string]any{"limit": 20}); err != nil {
		return err
	}
	if _, err := r.invoke(ctx, "capture_metrics", map[string]any{"publication_id": r.publicationID}); err != nil {
		return err
	}
	r.stages = append(r.stages, "seven_day_metrics")
	return nil
}

func (r *socialAutomationRunner) weekly(ctx context.Context) error {
	if _, err := r.invoke(ctx, "list_publications", map[string]any{"since": "2026-10-19T00:00:00Z", "until": "2026-10-28T00:00:00Z", "limit": 20}); err != nil {
		return err
	}
	for _, item := range []struct{ kind, key, content string }{
		{"weekly_review", "2026-W43", "The verified post received nine likes after seven days."},
		{"topic", "next_topic", "Explain recovery from uncertain external writes."},
		{"writing_rule", "current", "Keep public claims tied to verified sources and platform evidence."},
	} {
		if _, err := r.save(ctx, item.kind, item.key, item.content, "text/plain"); err != nil {
			return err
		}
	}
	r.stages = append(r.stages, "weekly_topic_writing_rule")
	return nil
}

func TestSocialOperationsAutomationRunNowExecutesTenStageProviderScript(t *testing.T) {
	f := newSocialIntegrationFixture(t)
	if err := f.db.AutoMigrate(&automationmodel.AutomationTask{}, &automationmodel.AutomationTaskAction{}, &automationmodel.AutomationTaskRun{}, &automationmodel.AutomationActionRun{}, &automationmodel.AutomationTaskActivity{}); err != nil {
		t.Fatal(err)
	}
	var logo bytes.Buffer
	imageData := image.NewRGBA(image.Rect(0, 0, 320, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 320; x++ {
			imageData.SetRGBA(x, y, color.RGBA{20, 110, 180, 255})
		}
	}
	if err := png.Encode(&logo, imageData); err != nil {
		t.Fatal(err)
	}
	logoArtifact, err := f.service.PutArtifact(t.Context(), f.scope, "image/png", logo.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	requirements := make([]socialops.CapabilityRequirement, 0, 7)
	for _, capability := range []socialops.Capability{socialops.CapabilityAccount, socialops.CapabilityPost, socialops.CapabilityReadback, socialops.CapabilityComment, socialops.CapabilityInbox, socialops.CapabilityMetrics, socialops.CapabilityImages} {
		requirements = append(requirements, socialops.CapabilityRequirement{Capability: capability, Mode: socialops.RequirementOptional})
	}
	cfg := socialops.Configuration{Scope: f.scope, EmployeeVersionID: f.gateway.restored.Run.DigitalHumanVersionID,
		Product: socialops.ProductConfiguration{Name: "Product", Description: "Description"}, Audience: "Operators", PublicationLanguage: "English", InternalReviewLanguage: "Chinese",
		Sources: []socialops.SourceReference{{URL: "https://example.com/source"}}, BrandArtifactRefs: []uuid.UUID{logoArtifact.ID},
		Research:  socialops.ResearchBinding{ConnectorID: "exa", ConnectionID: uuid.New()},
		Platforms: []socialops.PlatformBinding{{Platform: "x", ConnectorID: "x-social", ConnectionID: f.connection, AccountID: "acct-7", MaxDailyPublications: 2, Capabilities: requirements}},
		Timezone:  "Asia/Shanghai", Cadence: socialops.Cadence{DailyContentCron: "0 9 * * *", InboxCron: "0 * * * *", SevenDaySampleCron: "0 10 * * *", WeeklyReviewCron: "0 11 * * 1"},
		TeamHandoff: socialops.TeamHandoff{ConnectorID: "dingtalk", ConnectionID: uuid.New(), OperationID: "dingtalk.messages.send_direct", Destination: "user-1"}}
	cfg, record, err := f.service.SaveCurrentConfiguration(t.Context(), f.scope, 0, cfg)
	if err != nil {
		t.Fatal(err)
	}
	inspections := 0
	provider := socialtools.NewProvider(func() (*socialops.Service, error) { return f.service, nil }, socialAutomationResolver{scope: f.scope, gateway: f.gateway, fixture: f, inspections: &inspections})
	if len(provider.GetTools()) != 21 {
		t.Fatalf("social_operations tools = %d, want 21", len(provider.GetTools()))
	}
	runner := &socialAutomationRunner{provider: provider, fixture: f, conversationID: f.gateway.restored.Run.ConversationID.String(), messageID: f.gateway.restored.Run.MessageID.String(), logoID: logoArtifact.ID}
	dispatcher := &socialAutomationDispatcher{}
	taskRepo, actionRepo, runRepo := automationrepo.NewTaskRepository(f.db), automationrepo.NewActionRepository(f.db), automationrepo.NewRunRepository(f.db)
	dispatcher.executor = automationruntime.NewExecutorWithActionExecutors(f.db, taskRepo, actionRepo, runRepo, automationaction.NewRunDigitalHumanExecutor(func() automationaction.AutomationDigitalHumanRunner { return runner }))
	taskService := digitalhumantask.NewService(f.db, dispatcher, func() automationaction.AutomationDigitalHumanRunner { return runner })
	access := digitalhuman.TaskAccessContext{OrganizationID: f.scope.OrganizationID, WorkspaceID: f.scope.WorkspaceID, DigitalHumanID: f.scope.EmployeeID,
		Principal: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: f.scope.OwnerID.String()}, Owner: digitalhuman.SharedTaskOwner(), Surface: digitalhuman.SurfaceConsole}

	for _, key := range []string{"daily_content", "frequent_inbox", "daily_seven_day_samples", "weekly_review"} {
		metadata := &digitalhuman.SocialOperationsTaskMetadata{Scope: digitalhuman.SocialOperationsTaskScope{OrganizationID: f.scope.OrganizationID, WorkspaceID: f.scope.WorkspaceID, OwnerID: f.scope.OwnerID, EmployeeID: f.scope.EmployeeID},
			EmployeeVersionID: cfg.EmployeeVersionID, ConfigurationRevision: cfg.Revision, ConfigurationRecordID: record.ID, ConfigurationArtifactID: record.ArtifactID,
			TaskKey: key, Stages: []string{key}, Budget: digitalhuman.SocialOperationsTaskBudget{MaxPages: 20, MaxRecords: 100, MaxConnectorCalls: 80, MaxTokens: 100000, MaxSafeReadAttempts: 8}}
		mutation := digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: f.scope.OwnerID.String(), OperationID: uuid.New(), Source: "social_operations", SocialOperations: metadata}
		task, createErr := taskService.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{Name: key, Goal: "Execute the bounded social operations stage", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleManual, Timezone: "Asia/Shanghai"}}, mutation)
		if createErr != nil {
			t.Fatalf("create %s: %v", key, createErr)
		}
		mutation.OperationID = uuid.New()
		if _, runErr := taskService.ApplyTaskAction(t.Context(), access, task.ID, digitalhuman.TaskActionRunNow, digitalhuman.ApplyTaskActionRequest{ExpectedRevision: task.Revision}, mutation); runErr != nil {
			t.Fatalf("run %s: %v", key, runErr)
		}
	}

	wantStages := []string{"source", "product_fact", "topic", "compose", "review", "publish", "archive", "inbox_outbox", "seven_day_metrics", "weekly_topic_writing_rule"}
	if fmt.Sprint(runner.stages) != fmt.Sprint(wantStages) {
		t.Fatalf("stages = %#v", runner.stages)
	}
	if inspections != 1 {
		t.Fatalf("controlled vision inspections = %d, want 1", inspections)
	}
	for _, recordKey := range []struct{ kind, key string }{{"source", "launch_source"}, {"product_fact", "launch_fact"}, {"topic", "launch_topic"}, {"feedback", "inbox_feedback"}, {"archive", "2026-10-20"}, {"weekly_review", "2026-W43"}, {"topic", "next_topic"}, {"writing_rule", "current"}} {
		if _, err := f.service.GetRecord(t.Context(), f.scope, recordKey.kind, recordKey.key); err != nil {
			t.Fatalf("missing %s/%s: %v", recordKey.kind, recordKey.key, err)
		}
	}
	publication, err := f.service.GetPublication(t.Context(), f.scope, runner.publicationID)
	if err != nil || publication.State != "archived" || publication.ReviewedDigest == "" {
		t.Fatalf("publication evidence = %#v, %v", publication, err)
	}
	var completedOutbounds, metrics, actionRuns int64
	if err = f.db.Model(&socialops.Outbound{}).Where("state = ? AND receipt_id <> ''", "completed").Count(&completedOutbounds).Error; err != nil || completedOutbounds != 1 {
		t.Fatalf("outbound evidence = %d, %v", completedOutbounds, err)
	}
	if err = f.db.Model(&socialops.Metric{}).Where("publication_id = ?", publication.ID).Count(&metrics).Error; err != nil || metrics != 1 {
		t.Fatalf("metric evidence = %d, %v", metrics, err)
	}
	if err = f.db.Model(&automationmodel.AutomationActionRun{}).Where("status = ? AND external_run_type = ?", automationmodel.AutomationActionRunStatusRunning, "digital_human").Count(&actionRuns).Error; err != nil || actionRuns != 4 {
		t.Fatalf("automation action runs = %d, %v", actionRuns, err)
	}
	for operation, want := range map[string]int{"x-social.post.create": 1, "x-social.post.get": 1, "x-social.message.send": 1, "x-social.metrics.get": 1} {
		if got := countSocialOperation(f.dispatcher.calls, operation); got != want {
			t.Fatalf("native %s calls = %d, want %d", operation, got, want)
		}
	}
}
