package container

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodels "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
)

type socialVisionModelStub struct{}

func (socialVisionModelStub) ResolveUseCase(context.Context, string, llmmodel.UseCase, *string, *string) (*defaultmodels.ResolvedModel, error) {
	return &defaultmodels.ResolvedModel{Provider: "fixture", Model: "vision"}, nil
}
func (socialVisionModelStub) ResolveModelType(context.Context, string, *string, *string, sharedmodel.ModelType) (*defaultmodels.ResolvedModel, error) {
	return nil, nil
}

type socialVisionClientStub struct {
	t       *testing.T
	scope   socialops.Scope
	calls   int
	content string
}

func (c *socialVisionClientStub) AppChat(_ context.Context, app *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	c.calls++
	if app.OrganizationID != c.scope.OrganizationID.String() || app.AccountID != c.scope.OwnerID.String() || app.WorkspaceID != c.scope.WorkspaceID.String() || !app.SuppressInvocationContent {
		c.t.Fatal("vision scope not restored", app)
	}
	parts, ok := req.Messages[0].Content.([]adapter.MessageContentPart)
	if !ok || len(parts) != 3 || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,") || !strings.HasPrefix(parts[2].ImageURL.URL, "data:image/png;base64,") || req.MaxTokens == nil || *req.MaxTokens > 1200 {
		c.t.Fatal("actual image and approved logo not supplied with bounded output")
	}
	return &adapter.ChatResponse{Choices: []adapter.Choice{{FinishReason: "stop", Message: adapter.Message{Content: c.content}}}}, nil
}
func TestSocialImageInspectionUsesScopedActualPixelsAndPersistsFindings(t *testing.T) {
	f := newSocialIntegrationFixture(t)
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	a, err := f.service.PutArtifact(t.Context(), f.scope, "image/png", b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg := socialops.Configuration{Scope: f.scope, Revision: 1, EmployeeVersionID: uuid.New(), Product: socialops.ProductConfiguration{Name: "Product", Description: "Description"}, Audience: "Developers", PublicationLanguage: "en-US", InternalReviewLanguage: "zh-Hans", Sources: []socialops.SourceReference{{URL: "https://example.com"}}, BrandArtifactRefs: []uuid.UUID{a.ID}, Platforms: []socialops.PlatformBinding{{Platform: "x", ConnectorID: "x-social", ConnectionID: f.connection, AccountID: "acct-7", MaxDailyPublications: 1, Capabilities: []socialops.CapabilityRequirement{{Capability: socialops.CapabilityImages, Mode: socialops.RequirementRequired}}}}, Timezone: "UTC", Cadence: socialops.Cadence{DailyContentCron: "0 9 * * *", InboxCron: "0 * * * *", SevenDaySampleCron: "0 10 * * *", WeeklyReviewCron: "0 11 * * 1"}, TeamHandoff: socialops.TeamHandoff{ConnectorID: "dingtalk", ConnectionID: uuid.New(), OperationID: "dingtalk.messages.send_direct", Destination: "user-1"}}
	cfg.Research = socialops.ResearchBinding{ConnectorID: "exa", ConnectionID: uuid.New()}
	for _, capability := range []socialops.Capability{socialops.CapabilityAccount, socialops.CapabilityPost, socialops.CapabilityReadback, socialops.CapabilityComment, socialops.CapabilityInbox, socialops.CapabilityMetrics} {
		cfg.Platforms[0].Capabilities = append(cfg.Platforms[0].Capabilities, socialops.CapabilityRequirement{Capability: capability, Mode: socialops.RequirementOptional})
	}
	encoded, _ := json.Marshal(cfg)
	configArtifact, err := f.service.PutArtifact(t.Context(), f.scope, "application/json", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.SaveRecord(t.Context(), f.scope, "configuration", "current", 0, configArtifact.ID); err != nil {
		t.Fatal(err)
	}
	client := &socialVisionClientStub{t: t, scope: f.scope, content: `{"approved":true,"issues":[],"description":"Readable image; approved logo matches."}`}
	result, err := inspectSocialImage(t.Context(), f.service, f.scope, "conversation", a.ID, uuid.Nil, socialVisionModelStub{}, client)
	if err != nil || result["approved"] != true || client.calls != 1 {
		t.Fatal(result, err, client.calls)
	}
	reviewID, ok := result["review_artifact_id"].(uuid.UUID)
	if !ok {
		t.Fatal("missing durable review")
	}
	if _, err = f.service.ReadArtifact(t.Context(), f.scope, reviewID); err != nil {
		t.Fatal(err)
	}
	wrong := f.scope
	wrong.OwnerID = uuid.New()
	if _, err = inspectSocialImage(t.Context(), f.service, wrong, "conversation", a.ID, uuid.Nil, socialVisionModelStub{}, client); err == nil || client.calls != 1 {
		t.Fatal("foreign image reached vision")
	}
	client.content = `{"approved":true,"issues":["clipped text"],"description":"An issue remains."}`
	result, err = inspectSocialImage(t.Context(), f.service, f.scope, "conversation", a.ID, uuid.Nil, socialVisionModelStub{}, client)
	if err != nil || result["approved"] != false {
		t.Fatal("issues were approved", result, err)
	}
}
