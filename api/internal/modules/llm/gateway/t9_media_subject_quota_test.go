package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

const t9VideoTaskPricing = `{"video_generation":{"currency":"USD","billing_unit":"task","rates":[{"resolution":"720p","price_per_task":1.2}]}}`

// newT9PrivateMediaService builds a private-lane gateway wired with a single
// route so every media operation runs the shared subject pre-deduct and settle
// chain (music and lyrics already cover the same chain in music_test.go).
func newT9PrivateMediaService(t *testing.T, organizationID uuid.UUID, provider, modelName string, updates map[string]any) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), provider, modelName)
	if len(updates) > 0 {
		if err := db.Model(&llmmodel.LLMModel{}).Where("name = ?", modelName).Updates(updates).Error; err != nil {
			t.Fatalf("update %q capabilities: %v", modelName, err)
		}
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: provider,
		IsEnabled:       true,
		Models:          []string{modelName},
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  provider,
			APIKeyCiphertext: "ciphertext",
			IsActive:         true,
		},
	}
	return &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: adapter.GlobalFactory,
		healthTracker:  NewChannelHealthTracker(nil),
		channelRouter: &ChannelRouter{
			db:                      db,
			organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{route}},
			cryptoService:           stubCryptoService{},
			strategyFactory:         NewStrategyFactory(),
			privateModels:           &fakePrivateModelLookup{},
		},
	}
}

func installT9Adapter(t *testing.T, service *llmGatewayServiceImpl, provider string, impl adapter.LLMProviderAdapter) {
	t.Helper()
	factory := adapter.NewDefaultAdapterFactory()
	factory.Register(provider, func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return impl, nil
	})
	service.adapterFactory = factory
}

func t9APIKey(organizationID uuid.UUID) *apikeymodel.TenantAPIKey {
	return &apikeymodel.TenantAPIKey{ID: uuid.NewString(), OrganizationID: organizationID.String()}
}

type t9ImageAdapter struct {
	chatTraceSuccessAdapter
	billing              *fakeBillingProvider
	preDeductCallsAtCall int
	resp                 *adapter.ImageResponse
	err                  error
}

func (a *t9ImageAdapter) CreateImage(context.Context, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	if a.billing != nil {
		a.preDeductCallsAtCall = a.billing.preDeductCalls
	}
	if a.err != nil {
		return nil, a.err
	}
	if a.resp != nil {
		return a.resp, nil
	}
	return &adapter.ImageResponse{Data: []adapter.ImageItem{{URL: "https://example.test/t9.png"}}}, nil
}

type t9SpeechAdapter struct {
	chatTraceSuccessAdapter
	billing              *fakeBillingProvider
	preDeductCallsAtCall int
	payload              []byte
	err                  error
}

func (a *t9SpeechAdapter) GenerateSpeech(_ context.Context, _ *adapter.SpeechRequest, dst io.Writer) error {
	if a.billing != nil {
		a.preDeductCallsAtCall = a.billing.preDeductCalls
	}
	if a.err != nil {
		return a.err
	}
	if len(a.payload) > 0 {
		if _, err := dst.Write(a.payload); err != nil {
			return err
		}
	}
	return nil
}

type t9TranscriptionAdapter struct {
	chatTraceSuccessAdapter
	billing              *fakeBillingProvider
	preDeductCallsAtCall int
	text                 string
	err                  error
}

func (a *t9TranscriptionAdapter) Transcribe(_ context.Context, req *adapter.TranscriptionRequest) (*adapter.TranscriptionResponse, error) {
	if a.billing != nil {
		a.preDeductCallsAtCall = a.billing.preDeductCalls
	}
	if a.err != nil {
		return nil, a.err
	}
	if req != nil && req.Audio != nil {
		if _, err := io.Copy(io.Discard, req.Audio); err != nil {
			return nil, err
		}
	}
	return &adapter.TranscriptionResponse{RequestID: req.RequestID, Text: a.text}, nil
}

type t9VideoAdapter struct {
	chatTraceSuccessAdapter
	createResp *adapter.VideoResponse
	createErr  error
	taskResp   *adapter.VideoResponse
	taskErr    error
}

func (a *t9VideoAdapter) CreateVideo(context.Context, *adapter.VideoRequest) (*adapter.VideoResponse, error) {
	if a.createErr != nil {
		return nil, a.createErr
	}
	return a.createResp, nil
}

func (a *t9VideoAdapter) GetVideoTask(context.Context, *adapter.VideoTaskRequest) (*adapter.VideoResponse, error) {
	if a.taskErr != nil {
		return nil, a.taskErr
	}
	return a.taskResp, nil
}

func TestT9ImagePrivatePreDeductsSubjectBeforeDispatch(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-image"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{"image_generation": true})
	local := &fakeBillingProvider{checkBalanceResult: true}
	imageAdapter := &t9ImageAdapter{billing: local}
	installT9Adapter(t, service, "minimax", imageAdapter)
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{imageQuote: PricingQuote{TotalCredits: 11}}

	resp, err := service.CreateImageWithAppContext(t.Context(), t9APIKey(orgID), nil, &adapter.ImageRequest{Model: modelName, Prompt: "cat"})
	if err != nil {
		t.Fatalf("CreateImageWithAppContext() error = %v", err)
	}
	if resp == nil {
		t.Fatal("CreateImageWithAppContext() response = nil")
	}
	if imageAdapter.preDeductCallsAtCall != 1 {
		t.Fatalf("adapter dispatched with %d subject pre-deducts, want 1", imageAdapter.preDeductCallsAtCall)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 {
		t.Fatalf("local billing pre-deduct/settle = %d/%d, want 1/1", local.preDeductCalls, local.settleCalls)
	}
	if local.lastPreDeduct.QuotaSubjectType != quotaSubjectTypeAPIKey || local.lastPreDeduct.QuotaSubjectID == "" {
		t.Fatalf("subject = %s/%s, want api key subject", local.lastPreDeduct.QuotaSubjectType, local.lastPreDeduct.QuotaSubjectID)
	}
	if local.lastSettle == nil || local.lastSettle.PricingOperation != PricingOperationImage || local.lastSettle.ActualCredits != 11 {
		t.Fatalf("settlement = %#v, want image op with 11 credits", local.lastSettle)
	}
}

func TestT9SpeechPrivatePreDeductsSubjectBeforeStreaming(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-speech"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{
		"speech_generation": true,
		"use_cases":         llmmodel.StringArray{string(llmmodel.UseCaseTextToSpeech)},
	})
	local := &fakeBillingProvider{checkBalanceResult: true}
	speechAdapter := &t9SpeechAdapter{billing: local, payload: []byte("MP3-T9")}
	installT9Adapter(t, service, "minimax", speechAdapter)
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{meteredQuote: PricingQuote{TotalCredits: 7}}

	var audio bytes.Buffer
	err := service.GenerateSpeech(t.Context(), t9APIKey(orgID), &SpeechRequest{
		Model: modelName, Input: "hello", Voice: "t9-voice", ResponseFormat: "mp3",
	}, &audio)
	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}
	if got := audio.String(); got != "MP3-T9" {
		t.Fatalf("streamed audio = %q, want MP3-T9", got)
	}
	if speechAdapter.preDeductCallsAtCall != 1 {
		t.Fatalf("speech adapter dispatched with %d subject pre-deducts, want 1", speechAdapter.preDeductCallsAtCall)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 {
		t.Fatalf("local billing pre-deduct/settle = %d/%d, want 1/1", local.preDeductCalls, local.settleCalls)
	}
	if local.lastSettle.PricingOperation != PricingOperationSpeech || local.lastSettle.ActualCredits != 7 {
		t.Fatalf("settlement = %#v, want speech op with 7 credits", local.lastSettle)
	}
}

// The streaming failure path must settle the reservation as a rollback instead
// of leaving the hold open.
func TestT9SpeechStreamingFailureRollsBackReservation(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-speech-fail"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{
		"speech_generation": true,
		"use_cases":         llmmodel.StringArray{string(llmmodel.UseCaseTextToSpeech)},
	})
	local := &fakeBillingProvider{checkBalanceResult: true}
	upstreamErr := errors.New("t9 upstream speech failure")
	speechAdapter := &t9SpeechAdapter{billing: local, err: upstreamErr}
	installT9Adapter(t, service, "minimax", speechAdapter)
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{meteredQuote: PricingQuote{TotalCredits: 7}}

	err := service.GenerateSpeech(t.Context(), t9APIKey(orgID), &SpeechRequest{
		Model: modelName, Input: "hello", Voice: "t9-voice", ResponseFormat: "mp3",
	}, &bytes.Buffer{})
	if !errors.Is(err, upstreamErr) {
		t.Fatalf("GenerateSpeech() error = %v, want upstream failure", err)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 {
		t.Fatalf("local billing pre-deduct/settle = %d/%d, want 1/1", local.preDeductCalls, local.settleCalls)
	}
	if local.lastSettle == nil || local.lastSettle.Status != billingContextStatusError || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("rollback settlement = %#v, want error status with 0 credits", local.lastSettle)
	}
}

func TestT9SpeechClientWriteFailureRollsBackReservation(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-speech-client"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{
		"speech_generation": true,
		"use_cases":         llmmodel.StringArray{string(llmmodel.UseCaseTextToSpeech)},
	})
	local := &fakeBillingProvider{checkBalanceResult: true}
	speechAdapter := &t9SpeechAdapter{billing: local, payload: []byte("MP3-T9")}
	installT9Adapter(t, service, "minimax", speechAdapter)
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{meteredQuote: PricingQuote{TotalCredits: 7}}

	err := service.GenerateSpeech(t.Context(), t9APIKey(orgID), &SpeechRequest{
		Model: modelName, Input: "hello", Voice: "t9-voice", ResponseFormat: "mp3",
	}, failingSpeechWriter{})
	if !IsClientIOError(err) {
		t.Fatalf("GenerateSpeech() error = %v, want a client IO error", err)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 {
		t.Fatalf("local billing pre-deduct/settle = %d/%d, want 1/1", local.preDeductCalls, local.settleCalls)
	}
	if local.lastSettle == nil || local.lastSettle.Status != billingContextStatusError || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("rollback settlement = %#v, want error status with 0 credits", local.lastSettle)
	}
}

func TestT9TranscriptionPrivatePreDeductsAndRepricesActualUsage(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-transcription"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{
		"transcription": true,
		"use_cases":     llmmodel.StringArray{string(llmmodel.UseCaseSpeechToText)},
	})
	local := &fakeBillingProvider{checkBalanceResult: true}
	transcriptionAdapter := &t9TranscriptionAdapter{billing: local, text: "t9 transcript"}
	installT9Adapter(t, service, "minimax", transcriptionAdapter)
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{meteredQuote: PricingQuote{
		TotalCredits:           5,
		MeteredOperation:       PricingOperationTranscription,
		MeteredMeter:           meterInputAudioDuration,
		MeteredBaseUnit:        baseUnitMillisecond,
		MeteredPriceUSDPerUnit: decimal.RequireFromString("0.000001"),
		MeteredPriceResolved:   true,
		RuleID:                 "t9.transcription",
	}}

	// 32000 bytes at 16kHz mono 16-bit is exactly one second (1000 ms).
	audio := bytes.Repeat([]byte{0}, 32000)
	result, err := service.Transcribe(t.Context(), t9APIKey(orgID), &TranscriptionRequest{
		Model: modelName,
		Audio: bytes.NewReader(audio),
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if result == nil || result.Text != "t9 transcript" {
		t.Fatalf("Transcribe() result = %#v", result)
	}
	if transcriptionAdapter.preDeductCallsAtCall != 1 {
		t.Fatalf("transcription adapter dispatched with %d subject pre-deducts, want 1", transcriptionAdapter.preDeductCallsAtCall)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 {
		t.Fatalf("local billing pre-deduct/settle = %d/%d, want 1/1", local.preDeductCalls, local.settleCalls)
	}
	// 1000 ms x 0.000001 USD = 0.001 USD = 1000 milli-points.
	if local.lastSettle == nil || local.lastSettle.PricingOperation != PricingOperationTranscription || local.lastSettle.ActualCredits != 1000 {
		t.Fatalf("settlement = %#v, want transcription op with 1000 credited milli-points", local.lastSettle)
	}
}

func TestT9TranscriptionTooLongEarlyReturnRollsBackReservation(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-transcription-limit"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{
		"transcription": true,
		"use_cases":     llmmodel.StringArray{string(llmmodel.UseCaseSpeechToText)},
	})
	local := &fakeBillingProvider{checkBalanceResult: true}
	transcriptionAdapter := &t9TranscriptionAdapter{billing: local}
	installT9Adapter(t, service, "minimax", transcriptionAdapter)
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{meteredQuote: PricingQuote{TotalCredits: 5}}

	oversized := bytes.Repeat([]byte{0}, int(transcriptionMaxAudioBytes())+1)
	_, err := service.Transcribe(t.Context(), t9APIKey(orgID), &TranscriptionRequest{
		Model: modelName,
		Audio: bytes.NewReader(oversized),
	})
	if !errors.Is(err, ErrTranscriptionAudioTooLong) {
		t.Fatalf("Transcribe() error = %v, want ErrTranscriptionAudioTooLong", err)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 1 {
		t.Fatalf("local billing pre-deduct/settle = %d/%d, want 1/1", local.preDeductCalls, local.settleCalls)
	}
	if local.lastSettle == nil || local.lastSettle.Status != billingContextStatusError || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("rollback settlement = %#v, want error status with 0 credits", local.lastSettle)
	}
}

// Async video: creation reserves once and the later task query settles the same
// attempt without a second local subject pre-deduct.
func TestT9VideoAsyncCreateThenTaskQuerySettlesSameAttempt(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-video"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, nil)
	if err := service.db.Exec("UPDATE llm_models SET pricing = ? WHERE name = ?", t9VideoTaskPricing, modelName).Error; err != nil {
		t.Fatalf("set video pricing: %v", err)
	}
	videoAdapter := &t9VideoAdapter{
		createResp: &adapter.VideoResponse{TaskID: "task-t9", ID: "task-t9", Status: "processing"},
		taskResp:   &adapter.VideoResponse{TaskID: "task-t9", Status: "succeeded"},
	}
	installT9Adapter(t, service, "minimax", videoAdapter)
	local := &fakeBillingProvider{checkBalanceResult: true}
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local

	count := 2
	createResp, err := service.CreateVideoWithAppContext(t.Context(), t9APIKey(orgID), nil, &adapter.VideoRequest{
		Model: modelName, Prompt: "a t9 clip", Resolution: "720p", N: &count,
	})
	if err != nil {
		t.Fatalf("CreateVideoWithAppContext() error = %v", err)
	}
	if createResp == nil || createResp.TaskID != "task-t9" {
		t.Fatalf("create response = %#v", createResp)
	}
	if local.preDeductCalls != 1 || local.settleCalls != 0 {
		t.Fatalf("after create local pre-deduct/settle = %d/%d, want 1/0", local.preDeductCalls, local.settleCalls)
	}
	attemptID := local.lastPreDeduct.AttemptID
	requestID := local.lastPreDeduct.RequestID
	if attemptID == "" || requestID == "" {
		t.Fatalf("create reservation lost its billing identity: %#v", local.lastPreDeduct)
	}

	taskReq := &adapter.VideoTaskRequest{
		Model: modelName, TaskID: "task-t9",
		AdditionalParameters: map[string]interface{}{
			"billing_request_id": requestID,
			"billing_attempt_id": attemptID,
			"resolution":         "720p",
			"count":              count,
		},
	}
	resp, err := service.GetVideoTaskWithAppContext(t.Context(), t9APIKey(orgID), nil, taskReq)
	if err != nil {
		t.Fatalf("GetVideoTaskWithAppContext() error = %v", err)
	}
	if resp == nil || !isSuccessfulVideoStatus(resp.Status) {
		t.Fatalf("task response = %#v", resp)
	}
	if local.preDeductCalls != 1 {
		t.Fatalf("task query reserved again: pre-deduct calls = %d, want 1", local.preDeductCalls)
	}
	if local.settleCalls != 1 || local.lastSettle == nil {
		t.Fatalf("task query settle calls = %d, want 1", local.settleCalls)
	}
	if local.lastSettle.AttemptID != attemptID {
		t.Fatalf("task query settled attempt %q, want the create attempt %q", local.lastSettle.AttemptID, attemptID)
	}
	// 2 tasks x 1.2 USD = 2.4 USD = 2,400,000 milli-points.
	if local.lastSettle.ActualCredits != 2400000 || local.lastSettle.PricingOperation != PricingOperationVideo {
		t.Fatalf("task settlement = %#v, want video op with 2400000 milli-points", local.lastSettle)
	}

	// A repeated query must still never reserve again.
	if _, err := service.GetVideoTaskWithAppContext(t.Context(), t9APIKey(orgID), nil, taskReq); err != nil {
		t.Fatalf("second GetVideoTaskWithAppContext() error = %v", err)
	}
	if local.preDeductCalls != 1 {
		t.Fatalf("second task query reserved again: pre-deduct calls = %d, want 1", local.preDeductCalls)
	}
}

// The media pre-deduct failure must reach the caller as the API-key quota cause
// instead of a generic pre-deduct failure.
func TestT9MediaPredeductErrorSurfacesAPIKeyQuotaCause(t *testing.T) {
	orgID := uuid.New()
	modelName := "t9-image-quota"
	service := newT9PrivateMediaService(t, orgID, "minimax", modelName, map[string]any{"image_generation": true})
	local := &fakeBillingProvider{checkBalanceResult: true, preDeductErr: ErrInsufficientQuota}
	installT9Adapter(t, service, "minimax", &t9ImageAdapter{billing: local})
	service.billing = &fakeBillingProvider{checkBalanceResult: true}
	service.localBilling = local
	service.pricingEngine = &fakePricingEngine{imageQuote: PricingQuote{TotalCredits: 11}}

	_, err := service.CreateImageWithAppContext(t.Context(), t9APIKey(orgID), nil, &adapter.ImageRequest{Model: modelName, Prompt: "cat"})
	var userErr *BillingUserError
	if !errors.As(err, &userErr) || userErr == nil || userErr.Kind != BillingUserErrorKindAPIKeyQuotaInsufficient {
		t.Fatalf("error %v kind = %#v, want %q", err, userErr, BillingUserErrorKindAPIKeyQuotaInsufficient)
	}
}
