package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type providerBoundaryStore struct {
	*channelStoreStub
	credential *ChannelInstallationCredential
}

func (store *providerBoundaryStore) CreateChannelInstallationWithCredential(
	_ context.Context,
	installation *channelruntime.Installation,
	credential *ChannelInstallationCredential,
) error {
	store.installations[installation.ID] = cloneInstallation(installation)
	copyCredential := *credential
	store.credential = &copyCredential
	return nil
}

func (store *providerBoundaryStore) ChannelInstallationCredentialByID(
	_ context.Context,
	organizationID, installationID uuid.UUID,
) (*ChannelInstallationCredential, error) {
	if store.credential == nil || store.credential.OrganizationID != organizationID || store.credential.InstallationID != installationID {
		return nil, ErrChannelCredentialNotFound
	}
	copyCredential := *store.credential
	return &copyCredential, nil
}

func (*providerBoundaryStore) ReplaceChannelInstallationCredential(
	context.Context,
	Scope,
	*channelruntime.Installation,
	*ChannelInstallationCredential,
	int64,
) error {
	return ErrChannelConflict
}

type boundaryProvider struct {
	deliveries []channelruntime.DeliveryRequest
}

func (*boundaryProvider) Provider() string { return "acme-chat" }

func (*boundaryProvider) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{}
}

func (*boundaryProvider) PrepareCredentials(current, updates map[string]string) (map[string]string, error) {
	credentials := cloneChannelCredentials(current)
	for key, value := range updates {
		if key != "client_token" && key != "callback_secret" {
			destroyChannelCredentials(credentials)
			return nil, ErrInvalidRequest
		}
		credentials[key] = strings.TrimSpace(value)
	}
	if credentials["client_token"] == "" || credentials["callback_secret"] == "" {
		destroyChannelCredentials(credentials)
		return nil, ErrInvalidRequest
	}
	return credentials, nil
}

func (*boundaryProvider) ResolveIdentity(
	_ context.Context,
	credential channelruntime.ResolvedChannelCredential,
) (channelruntime.InstallationIdentity, error) {
	if credential.Values["client_token"] != "acme-token" {
		return channelruntime.InstallationIdentity{}, ErrChannelNotReady
	}
	return channelruntime.InstallationIdentity{
		ExternalTenantID: "acme-workspace",
		ExternalSelfID:   "acme-helper",
	}, nil
}

func (*boundaryProvider) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{
		IntegrationID: "acme-vault",
		DriverID:      "acme-http",
		AuthMethodID:  "acme-secret",
	}
}

func (*boundaryProvider) Verify(
	_ context.Context,
	request channelruntime.WebhookRequest,
	credential channelruntime.ResolvedChannelCredential,
) (*channelruntime.VerifiedWebhook, error) {
	if request.Headers.Get("X-Acme-Signature") != credential.Values["callback_secret"] {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	if !json.Valid(request.Body) {
		return nil, channelruntime.ErrInvalidWebhook
	}
	return &channelruntime.VerifiedWebhook{Body: append([]byte(nil), request.Body...)}, nil
}

func (*boundaryProvider) Normalize(
	_ context.Context,
	verified *channelruntime.VerifiedWebhook,
	normalization channelruntime.NormalizationContext,
) (*channelruntime.InboundMessage, error) {
	var payload struct {
		EventID       string `json:"event"`
		MessageID     string `json:"message"`
		Conversation  string `json:"room"`
		Sender        string `json:"from"`
		Text          string `json:"body"`
		DirectMessage bool   `json:"direct"`
	}
	if verified == nil || json.Unmarshal(verified.Body, &payload) != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	conversationType := channelruntime.ConversationTypeGroup
	wakeReason := channelruntime.WakeReasonMention
	if payload.DirectMessage {
		conversationType = channelruntime.ConversationTypeDirect
		wakeReason = channelruntime.WakeReasonDirectMessage
	}
	return &channelruntime.InboundMessage{
		Provider:               "acme-chat",
		ProviderEventID:        payload.EventID,
		ProviderMessageID:      payload.MessageID,
		ExternalTenantID:       normalization.Identity.ExternalTenantID,
		ExternalConversationID: payload.Conversation,
		ConversationType:       conversationType,
		SenderExternalID:       payload.Sender,
		Text:                   payload.Text,
		WakeReason:             wakeReason,
		OccurredAt:             time.Now().UTC(),
	}, nil
}

func (*boundaryProvider) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{
		StatusCode: http.StatusAccepted,
		Headers:    http.Header{"X-Acme-Acknowledged": []string{"true"}},
		Body:       []byte(`{"accepted":true}`),
	}
}

func (provider *boundaryProvider) Deliver(
	_ context.Context,
	request channelruntime.DeliveryRequest,
) (*channelruntime.DeliveryReceipt, error) {
	provider.deliveries = append(provider.deliveries, request)
	return &channelruntime.DeliveryReceipt{ProviderMessageID: "acme-reply"}, nil
}

func (*boundaryProvider) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}

func (*boundaryProvider) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}

func TestRegisteredProviderRunsEndToEndWithoutDigitalHumanProviderBranch(t *testing.T) {
	provider := &boundaryProvider{}
	registry, err := channelruntime.NewRegistry(channelruntime.Provider{Adapter: provider, Provisioner: provider})
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := integrations.NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	store := &providerBoundaryStore{channelStoreStub: newChannelStoreStub()}
	credentials := NewIntegrationChannelCredentialResolver(store, &connectionResolverStub{}, cipher, registry)
	scope := testScope()
	digitalHumanID := uuid.New()
	runtime := &channelRuntimeStub{}
	service := NewChannelService(
		store,
		channelSubjectStub{definition: &Definition{
			ID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
			Name: "Provider boundary", Status: StatusActive,
		}},
		runtime,
		registry,
		credentials,
		&channelTransientDispatcherStub{},
	)
	installation, err := service.CreateInstallation(t.Context(), scope, digitalHumanID, CreateChannelInstallationRequest{
		Provider: "acme-chat",
		Name:     "Acme helper",
		Credentials: map[string]string{
			"client_token": "acme-token", "callback_secret": "acme-signature",
		},
	})
	if err != nil {
		t.Fatalf("CreateInstallation() error = %v", err)
	}
	if installation.ExternalTenantID != "acme-workspace" || installation.ExternalBotID != "acme-helper" {
		t.Fatalf("installation identity = %q/%q", installation.ExternalTenantID, installation.ExternalBotID)
	}
	if readiness := credentials.ContextReadiness(t.Context(), installation); readiness.Status != ChannelContextReadinessUnavailable {
		t.Fatalf("context readiness = %#v, want unavailable for a provider without ContextReader", readiness)
	}
	payload := []byte(`{"event":"evt-1","message":"msg-1","room":"dm-1","from":"member-1","body":"hello","direct":true}`)
	result, err := service.HandleWebhook(t.Context(), "acme-chat", installation.ID, channelruntime.WebhookRequest{
		Body: payload,
		Headers: http.Header{
			"X-Acme-Signature": []string{"acme-signature"},
		},
		ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if !result.Accepted || result.Response.StatusCode != http.StatusAccepted || string(result.Response.Body) != `{"accepted":true}` {
		t.Fatalf("webhook result = %#v", result)
	}
	if runtime.calls != 1 || runtime.lastQuery != "hello" {
		t.Fatalf("runtime calls = %d, query = %q", runtime.calls, runtime.lastQuery)
	}
	store.transientTarget = &ChannelTransientTarget{
		OrganizationID:         scope.OrganizationID,
		WorkspaceID:            scope.WorkspaceID,
		ActorID:                scope.ActorID,
		InstallationID:         installation.ID,
		ConnectionID:           installation.ConnectionID,
		SubjectType:            runtimeidentity.SubjectTypeDigitalHuman,
		SubjectID:              digitalHumanID,
		Provider:               "acme-chat",
		ExternalConversationID: "dm-1",
	}
	if err := service.DeliverTransient(t.Context(), ChannelTransientDeliveryRequest{
		RunID: runtime.runID, Kind: channelruntime.DeliveryKindProgress,
		DeliveryKey: "progress:1", Sequence: 1, Text: "working",
	}); err != nil {
		t.Fatalf("DeliverTransient() error = %v", err)
	}
	if len(provider.deliveries) != 1 || provider.deliveries[0].Text != "working" {
		t.Fatalf("provider deliveries = %#v", provider.deliveries)
	}
}

func TestChannelWebhookRejectsProviderInstallationMismatch(t *testing.T) {
	service, _, _, _, scope, digitalHumanID := newChannelTestService(t)
	installation, _ := createChannelTestRoute(t, service, scope, digitalHumanID, channelruntime.ConversationTypeDirect)
	if _, err := service.HandleWebhook(t.Context(), "acme-chat", installation.ID, channelruntime.WebhookRequest{
		Body: []byte(`{"safe":true}`),
	}); !errors.Is(err, ErrChannelInstallationNotFound) {
		t.Fatalf("HandleWebhook() mismatch error = %v", err)
	}
}
