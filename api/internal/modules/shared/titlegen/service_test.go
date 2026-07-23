package titlegen

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/model"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
)

func TestGenerateRetriesFallbackCandidateAfterBadModelOutput(t *testing.T) {
	organizationID := uuid.New()
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "openai",
			Model:    "bad-json-model",
			Source:   llmdefaultservice.SourceExplicit,
		},
		resolved: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "openai",
				Model:    "bad-json-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
			{
				UseCase:  string(llmmodelmodel.UseCaseVision),
				Provider: "openai",
				Model:    "fast-chat-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"bad-json-model":  titleResponse("not json"),
			"fast-chat-model": titleResponse(`{"title":"Weekend Travel"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: organizationID,
		AccountID:      uuid.New(),
		AppID:          "app-1",
		AppType:        "aichat",
		ConversationID: "conversation-1",
		Messages:       []Message{{Role: "user", Content: "Help me plan weekend travel"}},
		FallbackTitle:  "Help me plan weekend travel",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "Weekend Travel" {
		t.Fatalf("result = %#v, want model title", result)
	}
	if got, want := llm.calls, []string{"bad-json-model", "fast-chat-model"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
	if len(llm.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(llm.requests))
	}
	for _, req := range llm.requests {
		if req.Stream {
			t.Fatalf("title request should not stream: %#v", req)
		}
		if req.MaxTokens == nil || *req.MaxTokens > 64 {
			t.Fatalf("MaxTokens = %#v, want <= 64", req.MaxTokens)
		}
		if len(req.Tools) > 0 || len(req.Functions) > 0 {
			t.Fatalf("title request should not use tools/functions: %#v", req)
		}
	}
}

func TestGenerateSkipsReasoningModelCandidates(t *testing.T) {
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "openai",
			Model:    "o1-preview",
			Source:   llmdefaultservice.SourceExplicit,
		},
		resolved: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseVision),
				Provider: "openai",
				Model:    "gpt-4o-mini",
				Source:   llmdefaultservice.SourceExplicit,
			},
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"o1-preview":  titleResponse(`{"title":"Should Not Call"}`),
			"gpt-4o-mini": titleResponse(`{"title":"Fast Title"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "Summarize a title quickly"}},
		FallbackTitle:  "Summarize a title quickly",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "Fast Title" {
		t.Fatalf("result = %#v, want non-reasoning model title", result)
	}
	if got, want := llm.calls, []string{"gpt-4o-mini"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateSkipsModelResolvedForReasoningUseCase(t *testing.T) {
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "openai",
			Model:    "dual-use-model",
			Source:   llmdefaultservice.SourceExplicit,
		},
		resolved: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseReasoning),
				Provider: "openai",
				Model:    "dual-use-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
			{
				UseCase:  string(llmmodelmodel.UseCaseVision),
				Provider: "openai",
				Model:    "fast-chat-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"dual-use-model":  titleResponse(`{"title":"Should Not Call"}`),
			"fast-chat-model": titleResponse(`{"title":"Backup Title"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "Name this conversation"}},
		FallbackTitle:  "Name this conversation",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "Backup Title" {
		t.Fatalf("result = %#v, want fallback candidate title", result)
	}
	if got, want := llm.calls, []string{"fast-chat-model"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateReturnsFallbackWhenAllCandidatesFail(t *testing.T) {
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "openai",
			Model:    "broken-model",
			Source:   llmdefaultservice.SourceExplicit,
		},
	}
	llm := &fakeTitleLLM{errByModel: map[string]error{"broken-model": errors.New("upstream failed")}}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "Explain project setup"}},
		FallbackTitle:  "Explain project setup",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceFallback || result.Title != "Explain project setup" {
		t.Fatalf("result = %#v, want fallback", result)
	}
}

func TestGenerateAcceptsRawTitleWhenJsonParsingFails(t *testing.T) {
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "qwen",
			Model:    "qwen3-max",
			Source:   llmdefaultservice.SourceExplicit,
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"qwen3-max": titleResponse("胖猫生成"),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "生产一个胖胖的猫咪"}},
		FallbackTitle:  "Conversation 2026-06-26 17:16:15",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "胖猫生成" {
		t.Fatalf("result = %#v, want raw model title", result)
	}
}

func TestGenerateSkipsImageGenDefaultCandidates(t *testing.T) {
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseImageGen),
			Provider: "openai",
			Model:    "image-generator",
			Source:   llmdefaultservice.SourceExplicit,
		},
		resolved: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseImageGen),
				Provider: "openai",
				Model:    "image-generator",
				Source:   llmdefaultservice.SourceExplicit,
			},
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "openai",
				Model:    "fast-chat-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"image-generator": titleResponse(`{"title":"Should Not Use Image Model"}`),
			"fast-chat-model": titleResponse(`{"title":"Image Prompt Title"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "Create an image of a product mockup"}},
		FallbackTitle:  "New chat",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "Image Prompt Title" {
		t.Fatalf("result = %#v, want text model title", result)
	}
	if got, want := llm.calls, []string{"fast-chat-model"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateUsesPreferredModelWhenDefaultsHaveNoCandidates(t *testing.T) {
	defaults := &fakeDefaultModelService{}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"qwen3-max": titleResponse(`{"title":"代码审查问题"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID:    uuid.New(),
		AccountID:         uuid.New(),
		Messages:          []Message{{Role: "user", Content: "你帮我看看我的代码也没有什么问题"}},
		FallbackTitle:     "New chat",
		PreferredProvider: "qwen",
		PreferredModel:    "qwen3-max",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "代码审查问题" {
		t.Fatalf("result = %#v, want preferred model title", result)
	}
	if got, want := llm.calls, []string{"qwen3-max"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateUsesRouteModelWhenDefaultsHaveNoCandidates(t *testing.T) {
	defaults := &fakeDefaultModelService{}
	routes := &fakeRouteModelProvider{
		models: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "qwen",
				Model:    "qwen-image-2.0",
				Source:   llmdefaultservice.SourceAuto,
			},
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "qwen",
				Model:    "qwen3-max",
				Source:   llmdefaultservice.SourceAuto,
			},
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"qwen-image-2.0": titleResponse(`{"title":"Should Not Use Image Model"}`),
			"qwen3-max":      titleResponse(`{"title":"胖猫生成"}`),
		},
	}
	svc := NewServiceWithRouteModelProvider(llm, defaults, routes)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "生产一个胖胖的猫咪"}},
		FallbackTitle:  "生产一个胖胖的猫咪",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "胖猫生成" {
		t.Fatalf("result = %#v, want route model title", result)
	}
	if got, want := llm.calls, []string{"qwen3-max"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateRotatesRouteModelsWhenAttemptFails(t *testing.T) {
	defaults := &fakeDefaultModelService{}
	routes := &fakeRouteModelProvider{
		models: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "qwen",
				Model:    "qwen-plus",
				Source:   llmdefaultservice.SourceAuto,
			},
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "qwen",
				Model:    "qwen3-max",
				Source:   llmdefaultservice.SourceAuto,
			},
		},
	}
	llm := &fakeTitleLLM{
		errByModel: map[string]error{
			"qwen-plus": errors.New("gateway failed"),
		},
		responses: map[string]*adapter.ChatResponse{
			"qwen3-max": titleResponse(`{"title":"备用模型标题"}`),
		},
	}
	svc := NewServiceWithRouteModelProvider(llm, defaults, routes)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "给这个问题起标题"}},
		FallbackTitle:  "给这个问题起标题",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "备用模型标题" {
		t.Fatalf("result = %#v, want rotated route model title", result)
	}
	if got, want := llm.calls, []string{"qwen-plus", "qwen3-max"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateRejectsPreferredImageModelForTitle(t *testing.T) {
	defaults := &fakeDefaultModelService{}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"image-model": titleResponse(`{"title":"Should Not Use Image Model"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID:    uuid.New(),
		AccountID:         uuid.New(),
		Messages:          []Message{{Role: "user", Content: "生成一张图"}},
		FallbackTitle:     "New chat",
		PreferredProvider: "openai",
		PreferredModel:    "image-model",
		PreferredUseCase:  string(llmmodelmodel.UseCaseImageGen),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceFallback || result.Title != "New chat" {
		t.Fatalf("result = %#v, want fallback", result)
	}
	if len(llm.calls) != 0 {
		t.Fatalf("calls = %#v, want no image model title call", llm.calls)
	}
}

func TestGenerateRemembersSuccessfulModelForSameAccount(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	defaults := &fakeDefaultModelService{}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"qwen3-max": titleResponse(`{"title":"第一次标题"}`),
		},
	}
	svc := NewService(llm, defaults)

	first, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID:    organizationID,
		AccountID:         accountID,
		Messages:          []Message{{Role: "user", Content: "第一次对话"}},
		FallbackTitle:     "New chat",
		PreferredProvider: "qwen",
		PreferredModel:    "qwen3-max",
	})
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	if first.Source != SourceModel {
		t.Fatalf("first result = %#v, want model title", first)
	}

	llm.responses["qwen3-max"] = titleResponse(`{"title":"第二次标题"}`)
	second, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: organizationID,
		AccountID:      accountID,
		Messages:       []Message{{Role: "user", Content: "第二次对话"}},
		FallbackTitle:  "New chat",
	})
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if second.Source != SourceModel || second.Title != "第二次标题" {
		t.Fatalf("second result = %#v, want cached model title", second)
	}
	if got, want := llm.calls, []string{"qwen3-max", "qwen3-max"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateRemembersSuccessfulModelAcrossServiceInstances(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	req := GenerateRequest{OrganizationID: organizationID, AccountID: accountID}
	successfulTitleModelCache.Delete(titleModelCacheKey(req))
	t.Cleanup(func() {
		successfulTitleModelCache.Delete(titleModelCacheKey(req))
	})

	firstLLM := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"qwen3-max": titleResponse(`{"title":"First Title"}`),
		},
	}
	firstSvc := NewService(firstLLM, &fakeDefaultModelService{})
	first, err := firstSvc.Generate(context.Background(), GenerateRequest{
		OrganizationID:    organizationID,
		AccountID:         accountID,
		Messages:          []Message{{Role: "user", Content: "First conversation"}},
		FallbackTitle:     "New chat",
		PreferredProvider: "qwen",
		PreferredModel:    "qwen3-max",
	})
	if err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	if first.Source != SourceModel {
		t.Fatalf("first result = %#v, want model title", first)
	}

	secondLLM := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"qwen3-max": titleResponse(`{"title":"Workflow Image Title"}`),
		},
	}
	secondSvc := NewService(secondLLM, &fakeDefaultModelService{})
	second, err := secondSvc.Generate(context.Background(), GenerateRequest{
		OrganizationID: organizationID,
		AccountID:      accountID,
		Messages:       []Message{{Role: "user", Content: "Generate an image of a storefront"}},
		FallbackTitle:  "Conversation 2026-06-26 16:34:25",
	})
	if err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	if second.Source != SourceModel || second.Title != "Workflow Image Title" {
		t.Fatalf("second result = %#v, want cached model title", second)
	}
	if got, want := secondLLM.calls, []string{"qwen3-max"}; !equalStrings(got, want) {
		t.Fatalf("second calls = %#v, want %#v", got, want)
	}
}

func TestGenerateDoesNotUseCachedModelNowMarkedReasoning(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	req := GenerateRequest{OrganizationID: organizationID, AccountID: accountID}
	successfulTitleModelCache.Store(titleModelCacheKey(req), &llmdefaultservice.ResolvedModel{
		UseCase:  string(llmmodelmodel.UseCaseTextChat),
		Provider: "openai",
		Model:    "dual-use-model",
		Source:   llmdefaultservice.SourceAuto,
	})
	t.Cleanup(func() {
		successfulTitleModelCache.Delete(titleModelCacheKey(req))
	})

	defaults := &fakeDefaultModelService{
		resolved: []*llmdefaultservice.ResolvedModel{
			{
				UseCase:  string(llmmodelmodel.UseCaseReasoning),
				Provider: "openai",
				Model:    "dual-use-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
			{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "openai",
				Model:    "fast-chat-model",
				Source:   llmdefaultservice.SourceExplicit,
			},
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"dual-use-model":  titleResponse(`{"title":"Should Not Use Cached Reasoning"}`),
			"fast-chat-model": titleResponse(`{"title":"Current Text Model"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: organizationID,
		AccountID:      accountID,
		Messages:       []Message{{Role: "user", Content: "Name this conversation"}},
		FallbackTitle:  "Name this conversation",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "Current Text Model" {
		t.Fatalf("result = %#v, want current text model title", result)
	}
	if got, want := llm.calls, []string{"fast-chat-model"}; !equalStrings(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestGenerateAcceptsConciseTitleWithPunctuation(t *testing.T) {
	defaults := &fakeDefaultModelService{
		primary: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "openai",
			Model:    "fast-chat-model",
			Source:   llmdefaultservice.SourceExplicit,
		},
	}
	llm := &fakeTitleLLM{
		responses: map[string]*adapter.ChatResponse{
			"fast-chat-model": titleResponse(`{"title":"代码审查：环境变量问题"}`),
		},
	}
	svc := NewService(llm, defaults)

	result, err := svc.Generate(context.Background(), GenerateRequest{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Messages:       []Message{{Role: "user", Content: "帮我检查代码"}},
		FallbackTitle:  "New chat",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Source != SourceModel || result.Title != "代码审查：环境变量问题" {
		t.Fatalf("result = %#v, want punctuation title", result)
	}
}

func TestTenantRouteModelProviderPreservesOfficialProviderModelPairs(t *testing.T) {
	provider := NewTenantRouteModelProvider(&titleRouteRepository{routes: []*channelmodel.LLMRoute{
		{
			Type:            shared.RouteTypeZGICloud,
			IsOfficial:      true,
			ChannelProvider: "zgi-cloud",
			Models:          []string{"flat-chat-model"},
			OfficialProviderModels: []channelmodel.ProviderModel{
				{Provider: "OpenAI", Model: "Case-Sensitive-Chat"},
			},
		},
	}})

	models, err := provider.ListTitleModels(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("ListTitleModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("ListTitleModels() = %#v, want one official candidate", models)
	}
	if gotProvider, gotModel := models[0].Provider, models[0].Model; gotProvider != "OpenAI" || gotModel != "Case-Sensitive-Chat" {
		t.Fatalf("official candidate = %q/%q, want %q/%q", gotProvider, gotModel, "OpenAI", "Case-Sensitive-Chat")
	}
}

func TestTenantRouteModelProviderRetainsPrivateRouteProviderAndModels(t *testing.T) {
	provider := NewTenantRouteModelProvider(&titleRouteRepository{routes: []*channelmodel.LLMRoute{
		{
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "Private-Provider",
			Models:          []string{" Private-Chat ", "Private-Chat"},
			OfficialProviderModels: []channelmodel.ProviderModel{
				{Provider: "Ignored-Official-Provider", Model: "Ignored-Official-Model"},
			},
		},
	}})

	models, err := provider.ListTitleModels(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("ListTitleModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("ListTitleModels() = %#v, want one private candidate", models)
	}
	if gotProvider, gotModel := models[0].Provider, models[0].Model; gotProvider != "Private-Provider" || gotModel != "Private-Chat" {
		t.Fatalf("private candidate = %q/%q, want %q/%q", gotProvider, gotModel, "Private-Provider", "Private-Chat")
	}
}

type titleRouteRepository struct {
	channelrepo.TenantRouteRepository
	routes []*channelmodel.LLMRoute
}

func (r *titleRouteRepository) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	return r.routes, nil
}

type fakeDefaultModelService struct {
	primary  *llmdefaultservice.ResolvedModel
	resolved []*llmdefaultservice.ResolvedModel
}

func (f *fakeDefaultModelService) ResolveModelType(context.Context, string, *string, *string, sharedmodel.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDefaultModelService) ResolveUseCase(context.Context, string, llmmodelmodel.UseCase, *string, *string) (*llmdefaultservice.ResolvedModel, error) {
	return f.primary, nil
}

func (f *fakeDefaultModelService) ListResolved(context.Context, uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	return f.resolved, nil
}

func (f *fakeDefaultModelService) Upsert(context.Context, uuid.UUID, *uuid.UUID, llmmodelmodel.UseCase, string, string, llmsharedtypes.JSONObject) (*defaultmodelmodel.DefaultModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDefaultModelService) Delete(context.Context, uuid.UUID, llmmodelmodel.UseCase) error {
	return errors.New("not implemented")
}

type fakeRouteModelProvider struct {
	models []*llmdefaultservice.ResolvedModel
	err    error
}

func (f *fakeRouteModelProvider) ListTitleModels(context.Context, uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	return f.models, f.err
}

type fakeTitleLLM struct {
	calls      []string
	requests   []*adapter.ChatRequest
	responses  map[string]*adapter.ChatResponse
	errByModel map[string]error
}

func (f *fakeTitleLLM) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("unexpected Chat call")
}

func (f *fakeTitleLLM) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected ChatStream call")
}

func (f *fakeTitleLLM) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected CreateResponse call")
}

func (f *fakeTitleLLM) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected Embed call")
}

func (f *fakeTitleLLM) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected CreateImage call")
}

func (f *fakeTitleLLM) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected Rerank call")
}

func (f *fakeTitleLLM) AppChat(_ context.Context, _ *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.calls = append(f.calls, req.Model)
	f.requests = append(f.requests, req)
	if err := f.errByModel[req.Model]; err != nil {
		return nil, err
	}
	if resp := f.responses[req.Model]; resp != nil {
		return resp, nil
	}
	return nil, errors.New("missing fake response")
}

func (f *fakeTitleLLM) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected AppChatStream call")
}

func (f *fakeTitleLLM) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected AppCreateResponse call")
}

func (f *fakeTitleLLM) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected AppEmbed call")
}

func (f *fakeTitleLLM) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected AppCreateImage call")
}

func (f *fakeTitleLLM) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected AppRerank call")
}

func titleResponse(content string) *adapter.ChatResponse {
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{Message: adapter.Message{Content: content}}},
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
