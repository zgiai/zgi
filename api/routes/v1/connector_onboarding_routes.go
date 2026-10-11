package v1

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	connectorproviders "github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"net/http"
	"strconv"
)

type connectorOnboardingService interface {
	Start(context.Context, onboarding.StartCommand) (onboarding.Snapshot, error)
	Get(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (onboarding.Snapshot, error)
	Cancel(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) (onboarding.Snapshot, error)
}

func (h *connectorHandler) registerOnboarding(group *gin.RouterGroup) {
	group.GET("/connectors/pending-setups", h.listPendingSetups)
	group.GET("/connectors/pending-setups/:pendingId", h.getPendingSetup)
	group.DELETE("/connectors/pending-setups/:pendingId", h.cancelPendingSetup)
	group.POST("/connectors/pending-setups/:pendingId/onboarding-flows", h.resumePendingSetup)
	group.POST("/connectors/:connectorId/onboarding-flows", h.startOnboarding)
	group.POST("/connector-connections/:connectionId/onboarding-flows", h.startOnboarding)
	group.GET("/connectors/onboarding-flows/:flowId", h.getOnboarding)
	group.DELETE("/connectors/onboarding-flows/:flowId", h.cancelOnboarding)
	group.POST("/connectors/onboarding-flows/:flowId/actions", h.onboardingAction)
}
func (h *connectorHandler) startAppCredentialSetup(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	d, ok := h.catalog.Get(c.Param("connectorId"))
	if !ok || !d.IsAppCredentials() {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	var request struct {
		connectorKeySetupRequest
		PendingSetupID          string `json:"pending_setup_id"`
		ExpectedPendingRevision int64  `json:"expected_pending_revision"`
	}
	defer func() { core.DestroySecretValues(request.Values) }()
	if decodeConnectorJSON(c, &request) != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if request.PendingSetupID == "" {
		if request.ExpectedPendingRevision != 0 {
			h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
			return
		}
		h.startParsedKeySetup(c, request.connectorKeySetupRequest, org, owner, d.ID)
		return
	}
	id, err := parseConnectorUUID(request.PendingSetupID)
	if err != nil || request.ExpectedPendingRevision < 1 || request.Target != nil || d.ID != "feishu" {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	service, ok := h.onboarding.(connectorPendingService)
	if !ok {
		h.credentialError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	result, err := service.ResumePendingManual(c.Request.Context(), onboarding.ResumeCommand{OrganizationID: org, OwnerAccountID: owner, PendingSetupID: id, ExpectedRevision: request.ExpectedPendingRevision, IdempotencyKey: c.GetHeader("Idempotency-Key"), Values: request.Values})
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, result)
}

type connectorPendingService interface {
	ListPending(context.Context, uuid.UUID, uuid.UUID, string, int, int) (onboarding.PendingList, error)
	GetPending(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (onboarding.PendingSnapshot, error)
	CancelPending(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) (onboarding.PendingSnapshot, error)
	ResumePending(context.Context, onboarding.ResumeCommand) (onboarding.Snapshot, error)
	ResumePendingManual(context.Context, onboarding.ResumeCommand) (onboarding.Snapshot, error)
}

func (h *connectorHandler) listPendingSetups(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	limit, e1 := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, e2 := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if e1 != nil || e2 != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	s, ok := h.onboarding.(connectorPendingService)
	if !ok {
		h.credentialError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	result, err := s.ListPending(c.Request.Context(), org, owner, c.Query("connector_id"), limit, offset)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, result)
}
func (h *connectorHandler) getPendingSetup(c *gin.Context) {
	h.pendingSetupAction(c, "get")
}
func (h *connectorHandler) cancelPendingSetup(c *gin.Context) { h.pendingSetupAction(c, "cancel") }
func (h *connectorHandler) resumePendingSetup(c *gin.Context) { h.pendingSetupAction(c, "resume") }
func (h *connectorHandler) pendingSetupAction(c *gin.Context, action string) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("pendingId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	s, ok := h.onboarding.(connectorPendingService)
	if !ok {
		h.credentialError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	if action == "get" {
		result, err := s.GetPending(c.Request.Context(), org, owner, id)
		if err != nil {
			h.credentialError(c, err)
			return
		}
		connectorSuccess(c, http.StatusOK, result)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if decodeConnectorJSON(c, &request) != nil || request.ExpectedRevision < 1 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if action == "cancel" {
		result, err := s.CancelPending(c.Request.Context(), org, owner, id, request.ExpectedRevision)
		if err != nil {
			h.credentialError(c, err)
			return
		}
		connectorSuccess(c, http.StatusOK, result)
		return
	}
	result, err := s.ResumePending(c.Request.Context(), onboarding.ResumeCommand{OrganizationID: org, OwnerAccountID: owner, PendingSetupID: id, ExpectedRevision: request.ExpectedRevision, IdempotencyKey: c.GetHeader("Idempotency-Key")})
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusAccepted, result)
}
func (h *connectorHandler) startOnboarding(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		ProtocolVersion  int    `json:"protocol_version"`
		Intent           string `json:"intent"`
		Region           string `json:"region"`
		TemplateID       string `json:"template_id"`
		ExpectedRevision int    `json:"expected_revision"`
	}
	if err := decodeConnectorJSON(c, &request); err != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if (request.ProtocolVersion != 0 && request.ProtocolVersion != 1) ||
		(request.ProtocolVersion == 1 && (request.Region != "" || request.TemplateID != "" || request.Intent == "create_app" || request.Intent == "update_app")) ||
		(request.ProtocolVersion == 0 && request.Intent != "create_app" && request.Intent != "update_app") {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	command := onboarding.StartCommand{OrganizationID: org, OwnerAccountID: owner, ConnectorID: c.Param("connectorId"), Region: request.Region, TemplateID: request.TemplateID, ExpectedRevision: request.ExpectedRevision, IdempotencyKey: c.GetHeader("Idempotency-Key"), Intent: request.Intent, ProtocolVersion: request.ProtocolVersion}
	if raw := c.Param("connectionId"); raw != "" {
		id, err := parseConnectorUUID(raw)
		if err != nil || (request.Intent != "update_app" && request.Intent != "reauthorize") || request.Region != "" || request.ExpectedRevision < 1 {
			h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
			return
		}
		command.ConnectionID = id
	} else if (request.Intent != "create_app" && request.Intent != "register_bot" && request.Intent != "login_session") || request.ExpectedRevision != 0 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := h.onboarding.Start(c.Request.Context(), command)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusAccepted, result)
}

func (h *connectorHandler) messagingMethod(id, method string) bool {
	a := catalog.DefaultMessagingAvailability(id)
	if !a.Enabled {
		return false
	}
	for _, m := range a.Methods {
		if m == method {
			return true
		}
	}
	return false
}
func (h *connectorHandler) projectCatalog(d catalog.Definition, connections []core.PersonalConnection) connectorCatalogItemResponse {
	out := projectCatalogItem(d, connections)
	if !d.IsMessagingCredentials() {
		return out
	}
	a := catalog.DefaultMessagingAvailability(d.ID)
	out.SetupAvailability = &a
	return out
}

// startCredentialSetup is the manual credential-setup surface. It admits exactly
// two dialects: the published messaging credential contract and the inert,
// Digital-Human-only Server Operations SSH contract. Every other connector
// dialect (Key, app, mail, and unknown) stays rejected.
func (h *connectorHandler) startCredentialSetup(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	d, ok := h.catalog.Get(c.Param("connectorId"))
	if !ok {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	if d.IsServerOpsSSHCredentials() {
		h.startServerOpsSSHCredentialSetup(c, d)
		return
	}
	if !d.IsMessagingCredentials() {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	h.startKeySetup(c)
}

// connectorServerOpsSSHSetupRequest is the credential-setup body for the SSH
// dialect. MailUsage and Target are json.RawMessage rather than typed fields so
// the handler can distinguish an absent key from an explicitly supplied one: a
// missing key leaves the slice nil, while any supplied JSON value — null, "",
// whitespace, or a populated object — leaves raw bytes behind. Only Values is
// ever forwarded to the shared parsed setup flow.
type connectorServerOpsSSHSetupRequest struct {
	MailUsage json.RawMessage   `json:"mail_usage"`
	Values    map[string]string `json:"values"`
	Target    json.RawMessage   `json:"target"`
}

// startServerOpsSSHCredentialSetup parses one SSH credential-setup body and
// hands only its Values to the existing startParsedKeySetup flow. It reuses
// decodeConnectorJSON, so the strict one-object/unknown-field contract, the
// connectorActor owner scope, the Idempotency-Key rule, and secret destruction
// are unchanged.
//
// It cannot delegate to startKeySetup: that path leaves MailUsage and Target
// admissible, and both are forbidden here. The request DTO keeps those two
// fields as raw JSON so a supplied key is detected exactly — a missing key is
// nil, while null, an empty or whitespace string, or any object is rejected
// outright. A Target must be bound through the generic connection create/attach
// flow, never as a credential-setup side effect, and mail usage does not apply
// to SSH.
func (h *connectorHandler) startServerOpsSSHCredentialSetup(c *gin.Context, d catalog.Definition) {
	// Bound the credential body to 32 KiB before any JSON decoding.
	// decodeConnectorJSON then installs the shared connectorRequestBodyMaxBytes
	// reader on top of this one, but the outer 32 KiB reader still trips
	// first, so this surface's effective streaming ceiling is 32 KiB. Nothing
	// from an oversized body is retained: the decode fails before the parsed
	// Values map is handed anywhere.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	var request connectorServerOpsSSHSetupRequest
	defer func() { core.DestroySecretValues(request.Values) }()
	if decodeConnectorJSON(c, &request) != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	// Presence is tested on the raw JSON bytes, never on decoded values, so
	// target:null and mail_usage:null are rejected exactly like a populated
	// target or a non-blank mail_usage.
	if len(request.Target) > 0 || len(request.MailUsage) > 0 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	// Forward only Values: the shared setup flow never observes a target or
	// mail usage on this surface, not even as a zero value.
	h.startParsedKeySetup(c, connectorKeySetupRequest{Values: request.Values}, org, owner, d.ID)
}
func (h *connectorHandler) onboardingAction(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	var request struct {
		Action            string            `json:"action"`
		Revision          int64             `json:"expected_revision"`
		ChallengeRevision int64             `json:"challenge_revision"`
		Values            map[string]string `json:"values"`
	}
	defer func() { core.DestroySecretValues(request.Values) }()
	if decodeConnectorJSON(c, &request) != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	service, ok := h.onboarding.(interface {
		Action(context.Context, onboarding.ActionCommand) (onboarding.Snapshot, error)
	})
	if !ok {
		h.credentialError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	result, err := service.Action(c.Request.Context(), onboarding.ActionCommand{OrganizationID: org, OwnerAccountID: owner, FlowID: id, IdempotencyKey: c.GetHeader("Idempotency-Key"), Action: request.Action, ExpectedRevision: request.Revision, ChallengeRevision: request.ChallengeRevision, Values: request.Values})
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusAccepted, result)
}
func (h *connectorHandler) getOnboarding(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	result, err := h.onboarding.Get(c.Request.Context(), org, owner, id)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, result)
}
func (h *connectorHandler) cancelOnboarding(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("flowId"))
	if err != nil {
		h.credentialError(c, err)
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if decodeConnectorJSON(c, &request) != nil || request.ExpectedRevision < 1 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := h.onboarding.Cancel(c.Request.Context(), org, owner, id, request.ExpectedRevision)
	if err != nil {
		h.credentialError(c, err)
		return
	}
	connectorSuccess(c, http.StatusOK, result)
}
func (h *connectorHandler) credentialError(c *gin.Context, err error) {
	projectable, ok := core.ApplicationError(err)
	if !ok {
		connectorRouteError(c, err)
		return
	}
	if h.errorProjector == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInternal, false, nil))
		return
	}
	projected := h.errorProjector.ProjectWithRequestID(projectable, apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language")), c.Writer.Header().Get("X-Request-ID")).Presentation
	c.Header(apptransport.HeaderApplicationErrorCode, projected.Code.String())
	c.JSON(projected.HTTPStatus, gin.H{"code": string(core.ErrorCodeOf(err)), "message": projected.Message, "retryable": core.ErrorCodeOf(err) == core.ErrorCodeRateLimited})
}

// connectorHostKeyTestRequest is the host-key probe body. MailUsage and Target
// are json.RawMessage rather than typed fields so the handler can distinguish an
// absent key from an explicitly supplied one: a missing key leaves the slice
// nil, while any supplied JSON value — null, "", whitespace, or a populated
// object — leaves raw bytes behind and is rejected.
type connectorHostKeyTestRequest struct {
	MailUsage json.RawMessage   `json:"mail_usage"`
	Values    map[string]string `json:"values"`
	Target    json.RawMessage   `json:"target"`
}

// connectorHostKeyTestResponse is the non-secret probe result. It carries only
// the observed canonical fingerprint, the redacted/bounded remote hostname, and
// the safe request id.
type connectorHostKeyTestResponse struct {
	HostKeySHA256 string `json:"host_key_sha256"`
	Hostname      string `json:"hostname"`
	RequestID     string `json:"request_id"`
}

// probeConnectorHostKey serves POST /connectors/:connectorId/host-key-tests. It
// is a read-only surface scoped to the exact SSH Server Operations credential
// contract: it never requires an Idempotency-Key and never creates or mutates a
// setup attempt or Connection. The 32 KiB body ceiling, one-object/unknown-field
// decode contract, owner scope, and secret destruction match the strict
// credential-setup route, and the probe seam is only reached after a valid
// request. Failures are projected through credentialError and never echo a
// submitted private key/passphrase or provider error text.
func (h *connectorHandler) probeConnectorHostKey(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	d, ok := h.catalog.Get(c.Param("connectorId"))
	if !ok || !d.IsServerOpsSSHCredentials() {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	if _, _, ok := connectorActor(c); !ok {
		return
	}
	if h.hostKeyProbe == nil {
		h.credentialError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	var request connectorHostKeyTestRequest
	defer func() { core.DestroySecretValues(request.Values) }()
	if decodeConnectorJSON(c, &request) != nil {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	// Presence is tested on the raw JSON bytes, never on decoded values, so
	// target:null and mail_usage:null are rejected exactly like a populated
	// target or a non-blank mail_usage.
	if len(request.Target) > 0 || len(request.MailUsage) > 0 {
		h.credentialError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	observation, err := h.hostKeyProbe.ProbeHostKey(c.Request.Context(), request.Values)
	if err != nil {
		h.credentialError(c, hostKeyProbeError(err))
		return
	}
	connectorSuccess(c, http.StatusOK, connectorHostKeyTestResponse{
		HostKeySHA256: observation.Fingerprint,
		Hostname:      observation.Hostname,
		RequestID:     observation.RequestID,
	})
}

// hostKeyProbeError projects a probe failure onto the Connector error contract.
// It retains only the stable code and retryability and never carries the
// provider cause, which may echo submitted credential text.
func hostKeyProbeError(err error) error {
	var failure *connectorproviders.Failure
	if errors.As(err, &failure) && failure != nil && failure.Code.Valid() {
		return core.NewError(failure.Code, failure.Retryable, nil)
	}
	return core.NewError(core.ErrorCodeProviderUnavailable, false, nil)
}
