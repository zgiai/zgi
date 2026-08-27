package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	// DirectSkillExaWebSearch deliberately lives outside the shared Agent Skill
	// catalog. It is an MVP capability-completion test owned by DigitalHuman.
	DirectSkillExaWebSearch = "digital-human-exa-web-search"
	exaWebSearchToolName    = "exa_web_search"
	exaSearchEndpoint       = "https://api.exa.ai/search"
	exaCredentialAADID      = "digital-human-direct-skill/exa-web-search"
	maxExaTokenRunes        = 2048
	maxExaResponseBytes     = 4 << 20
	maxExaResultTextRunes   = 4000
)

var errExaSearchFailed = errors.New("exa web search failed")

// DirectSkillConfiguration is the secret-free settings view exposed to the UI.
type DirectSkillConfiguration struct {
	SkillID    string    `json:"skill_id"`
	Configured bool      `json:"configured"`
	Enabled    bool      `json:"enabled"`
	Revision   int64     `json:"revision"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// UpdateDirectSkillConfigurationRequest changes an experimental direct Skill.
// An empty token preserves the existing encrypted token during later updates.
type UpdateDirectSkillConfigurationRequest struct {
	APIToken         string `json:"api_token"`
	Enabled          bool   `json:"enabled"`
	ExpectedRevision int64  `json:"expected_revision"`
}

// DirectSkillPreparation contains only runtime-facing tools and guidance.
type DirectSkillPreparation struct {
	SystemPromptAddendum string
	Tools                []skillloop.RuntimeTool
	destroy              func()
}

// Destroy removes the request-scoped decrypted credential as soon as the run
// completes. Callers must defer it immediately after Prepare succeeds.
func (preparation *DirectSkillPreparation) Destroy() {
	if preparation != nil && preparation.destroy != nil {
		preparation.destroy()
		preparation.destroy = nil
	}
}

// DirectSkillRuntimeProvider is the DigitalHuman-owned boundary for temporary
// direct capabilities that are not resolved through the shared connector.
type DirectSkillRuntimeProvider interface {
	Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (*DirectSkillPreparation, error)
	Candidates() ([]ResourceCandidate, error)
	GetConfiguration(context.Context, Scope, uuid.UUID, string) (*DirectSkillConfiguration, error)
	UpdateConfiguration(context.Context, Scope, uuid.UUID, string, UpdateDirectSkillConfigurationRequest) (*DirectSkillConfiguration, error)
}

// ExaDirectSkillProvider implements a temporary DigitalHuman-only web search
// path. TODO(digital-human-connectors): replace credential resolution with a
// connector-backed lease once the shared connector is reduced to credential
// management. Keep the Skill ID and tool contract stable during that migration.
type ExaDirectSkillProvider struct {
	repository *GormRepository
	cipher     integrations.CredentialCipher
	httpClient *http.Client
	endpoint   string
	now        func() time.Time
}

func NewExaDirectSkillProvider(repository *GormRepository, cipher integrations.CredentialCipher, httpClient *http.Client) *ExaDirectSkillProvider {
	if repository == nil || cipher == nil {
		panic("DigitalHuman Exa direct Skill requires repository and credential cipher")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &ExaDirectSkillProvider{repository: repository, cipher: cipher, httpClient: httpClient, endpoint: exaSearchEndpoint, now: func() time.Time { return time.Now().UTC() }}
}

func (provider *ExaDirectSkillProvider) Candidates() ([]ResourceCandidate, error) {
	revision, err := hashRevision(struct {
		SkillID string `json:"skill_id"`
		Tool    string `json:"tool"`
		Schema  int    `json:"schema"`
	}{SkillID: DirectSkillExaWebSearch, Tool: exaWebSearchToolName, Schema: 1})
	if err != nil {
		return nil, err
	}
	return []ResourceCandidate{{
		Type: resourcebindings.BindingTypeSkill, ResourceID: DirectSkillExaWebSearch,
		DisplayName: "Exa Web Search", Description: "Search the public web and read relevant page content with Exa.",
		ResourceRevision: revision, AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeExecute},
		Metadata: map[string]any{
			"source": "digital_human_direct", "runtime_type": "tool", "has_tools": true,
			"direct_configuration": "exa-web-search",
			"display": map[string]any{
				"icon": "search", "category": "research",
				"label": map[string]string{"zh_Hans": "Exa 网页搜索", "en_US": "Exa Web Search"},
				"description": map[string]string{
					"zh_Hans": "搜索公开网页并读取相关页面内容，用于获取最新信息。",
					"en_US":   "Search the public web and read relevant page content for current information.",
				},
				"when_to_use": map[string]string{
					"zh_Hans": "需要核验最新事实、查找公开来源或补充外部证据时使用。",
					"en_US":   "Use for current facts, public sources, or external evidence.",
				},
			},
		},
		Status: "available", Available: true,
	}}, nil
}

func (provider *ExaDirectSkillProvider) GetConfiguration(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, skillID string) (*DirectSkillConfiguration, error) {
	if provider == nil || !validScope(scope) || digitalHumanID == uuid.Nil || normalizeDirectSkillID(skillID) != DirectSkillExaWebSearch {
		return nil, ErrInvalidRequest
	}
	if _, err := provider.repository.Definition(ctx, scope, digitalHumanID); err != nil {
		return nil, err
	}
	stored, err := provider.repository.DirectSkillCredential(ctx, scope, digitalHumanID, DirectSkillExaWebSearch, false)
	if errors.Is(err, errDirectSkillCredentialNotFound) {
		return &DirectSkillConfiguration{SkillID: DirectSkillExaWebSearch}, nil
	}
	if err != nil {
		return nil, err
	}
	return directSkillConfigurationView(stored), nil
}

func (provider *ExaDirectSkillProvider) UpdateConfiguration(
	ctx context.Context,
	scope Scope,
	digitalHumanID uuid.UUID,
	skillID string,
	request UpdateDirectSkillConfigurationRequest,
) (*DirectSkillConfiguration, error) {
	token := strings.TrimSpace(request.APIToken)
	if provider == nil || !validScope(scope) || digitalHumanID == uuid.Nil ||
		normalizeDirectSkillID(skillID) != DirectSkillExaWebSearch || request.ExpectedRevision < 0 ||
		!utf8.ValidString(token) || utf8.RuneCountInString(token) > maxExaTokenRunes {
		return nil, ErrInvalidRequest
	}
	var updated *DirectSkillCredential
	err := provider.repository.Transaction(ctx, func(tx *GormRepository) error {
		if _, err := tx.LockDefinition(ctx, scope, digitalHumanID); err != nil {
			return err
		}
		stored, err := tx.DirectSkillCredential(ctx, scope, digitalHumanID, DirectSkillExaWebSearch, true)
		if errors.Is(err, errDirectSkillCredentialNotFound) {
			if request.ExpectedRevision != 0 || token == "" {
				return ErrRevisionConflict
			}
			now, id := provider.now(), uuid.New()
			envelope, encryptErr := provider.cipher.EncryptCredentials(
				map[string]string{"api_token": token}, directSkillCredentialAAD(scope.OrganizationID, id, 1),
			)
			if encryptErr != nil {
				return fmt.Errorf("encrypt DigitalHuman direct Skill credential: %w", encryptErr)
			}
			updated = &DirectSkillCredential{
				ID: id, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
				SkillID: DirectSkillExaWebSearch, EncryptedCredentials: envelope, CredentialVersion: 1,
				Enabled: request.Enabled, Revision: 1, CreatedBy: scope.ActorID, UpdatedBy: scope.ActorID, CreatedAt: now, UpdatedAt: now,
			}
			return tx.CreateDirectSkillCredential(ctx, updated)
		}
		if err != nil {
			return err
		}
		if request.ExpectedRevision != stored.Revision {
			return ErrRevisionConflict
		}
		next := *stored
		if token != "" {
			next.CredentialVersion++
			envelope, encryptErr := provider.cipher.EncryptCredentials(
				map[string]string{"api_token": token}, directSkillCredentialAAD(scope.OrganizationID, next.ID, next.CredentialVersion),
			)
			if encryptErr != nil {
				return fmt.Errorf("encrypt DigitalHuman direct Skill credential: %w", encryptErr)
			}
			next.EncryptedCredentials = envelope
		}
		next.Enabled = request.Enabled
		next.Revision++
		next.UpdatedBy = scope.ActorID
		next.UpdatedAt = provider.now()
		if err := tx.UpdateDirectSkillCredential(ctx, &next, stored.Revision); err != nil {
			return err
		}
		updated = &next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return directSkillConfigurationView(updated), nil
}

func (provider *ExaDirectSkillProvider) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, _ invocation.Event) (*DirectSkillPreparation, error) {
	if provider == nil || run == nil || resolved == nil || !executionHasSkill(resolved, DirectSkillExaWebSearch) {
		return nil, nil
	}
	scope := Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID}
	stored, err := provider.repository.DirectSkillCredential(ctx, scope, run.DigitalHumanID, DirectSkillExaWebSearch, false)
	if errors.Is(err, errDirectSkillCredentialNotFound) {
		return unavailableExaPreparation("Exa web search is bound but has no API token configured. Do not claim to have searched the web."), nil
	}
	if err != nil {
		return nil, err
	}
	if !stored.Enabled {
		return unavailableExaPreparation("Exa web search is bound but currently disabled. Do not claim to have searched the web."), nil
	}
	credentials, err := provider.cipher.DecryptCredentials(stored.EncryptedCredentials, directSkillCredentialAAD(stored.OrganizationID, stored.ID, stored.CredentialVersion))
	if err != nil {
		return unavailableExaPreparation("Exa web search could not load its credential. Do not claim to have searched the web."), nil
	}
	token := strings.TrimSpace(credentials["api_token"])
	if token == "" {
		destroyDirectSkillCredentials(credentials)
		return unavailableExaPreparation("Exa web search has an invalid credential configuration. Do not claim to have searched the web."), nil
	}
	preparation := &DirectSkillPreparation{
		SystemPromptAddendum: strings.Join([]string{
			"Exa web search is available for current or externally verifiable information.",
			"Use it only when web evidence materially improves the answer. Base web claims on returned sources and include useful source links in the final answer.",
			"If the tool reports a credential, quota, rate-limit, or availability error, explain the capability limitation briefly and do not invent search results.",
		}, "\n"),
		Tools: []skillloop.RuntimeTool{provider.searchTool(credentials)},
	}
	preparation.destroy = func() { destroyDirectSkillCredentials(credentials) }
	return preparation, nil
}

func (provider *ExaDirectSkillProvider) searchTool(credentials map[string]string) skillloop.RuntimeTool {
	definition := adapter.Tool{Type: "function", Function: adapter.Function{
		Name:        exaWebSearchToolName,
		Description: "Search the public web with Exa and return relevant source URLs plus bounded page text. Use for current facts or external evidence, not for questions already answerable from the conversation.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]any{"type": "string", "description": "A focused web search query."},
				"num_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "description": "Number of results; defaults to 5."},
			},
			"required": []string{"query"}, "additionalProperties": false,
		},
	}}
	handler := func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
		args, err := decodeExaSearchArguments(call.Function.Arguments)
		if err != nil {
			return exaToolFailure("invalid_arguments", true)
		}
		response, code, recoverable, err := provider.search(ctx, credentials["api_token"], args)
		if err != nil {
			return exaToolFailure(code, recoverable)
		}
		return skillloop.RuntimeToolResult{
			Status: "success", Arguments: map[string]any{"tool": exaWebSearchToolName},
			Result:      map[string]any{"result_count": len(response.Results), "request_id": response.RequestID},
			ModelResult: map[string]any{"status": "success", "results": response.Results},
		}
	}
	return skillloop.RuntimeTool{Definition: definition, SkillID: DirectSkillExaWebSearch, Handler: handler}
}

type exaSearchArguments struct {
	Query      string `json:"query"`
	NumResults int    `json:"num_results"`
}

type exaSearchResponse struct {
	RequestID string            `json:"requestId"`
	Results   []exaSearchResult `json:"results"`
}

type exaSearchResult struct {
	Title         string   `json:"title,omitempty"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate,omitempty"`
	Author        string   `json:"author,omitempty"`
	Text          string   `json:"text,omitempty"`
	Highlights    []string `json:"highlights,omitempty"`
}

func decodeExaSearchArguments(raw string) (exaSearchArguments, error) {
	var args exaSearchArguments
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return exaSearchArguments{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return exaSearchArguments{}, ErrInvalidRequest
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.NumResults == 0 {
		args.NumResults = 5
	}
	if args.Query == "" || !utf8.ValidString(args.Query) || utf8.RuneCountInString(args.Query) > 1000 || args.NumResults < 1 || args.NumResults > 10 {
		return exaSearchArguments{}, ErrInvalidRequest
	}
	return args, nil
}

func (provider *ExaDirectSkillProvider) search(ctx context.Context, token string, args exaSearchArguments) (*exaSearchResponse, string, bool, error) {
	body, err := json.Marshal(map[string]any{
		"query": args.Query, "type": "auto", "numResults": args.NumResults,
		"contents": map[string]any{"text": true, "highlights": true},
	})
	if err != nil {
		return nil, "invalid_request", false, errExaSearchFailed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "invalid_request", false, errExaSearchFailed
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", strings.TrimSpace(token))
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return nil, "unavailable", true, errExaSearchFailed
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		code, recoverable := exaHTTPError(response.StatusCode)
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxExaResponseBytes))
		return nil, code, recoverable, errExaSearchFailed
	}
	var decoded exaSearchResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxExaResponseBytes))
	if err := decoder.Decode(&decoded); err != nil {
		return nil, "invalid_response", true, errExaSearchFailed
	}
	if len(decoded.Results) > args.NumResults {
		decoded.Results = decoded.Results[:args.NumResults]
	}
	for index := range decoded.Results {
		decoded.Results[index] = sanitizeExaResult(decoded.Results[index])
	}
	return &decoded, "", false, nil
}

func exaHTTPError(status int) (string, bool) {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request", false
	case http.StatusUnauthorized, http.StatusForbidden:
		return "invalid_token", false
	case http.StatusPaymentRequired:
		return "quota_exceeded", false
	case http.StatusTooManyRequests:
		return "rate_limited", true
	default:
		return "unavailable", status >= http.StatusInternalServerError
	}
}

func sanitizeExaResult(result exaSearchResult) exaSearchResult {
	result.Title = truncateExaRunes(strings.TrimSpace(result.Title), 500)
	result.URL = truncateExaRunes(strings.TrimSpace(result.URL), 2048)
	result.Author = truncateExaRunes(strings.TrimSpace(result.Author), 500)
	result.PublishedDate = truncateExaRunes(strings.TrimSpace(result.PublishedDate), 128)
	result.Text = truncateExaRunes(strings.TrimSpace(result.Text), maxExaResultTextRunes)
	if len(result.Highlights) > 5 {
		result.Highlights = result.Highlights[:5]
	}
	for index := range result.Highlights {
		result.Highlights[index] = truncateExaRunes(strings.TrimSpace(result.Highlights[index]), 1000)
	}
	return result
}

func truncateExaRunes(value string, maximum int) string {
	if maximum <= 0 || utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximum]) + "…"
}

func exaToolFailure(code string, recoverable bool) skillloop.RuntimeToolResult {
	return skillloop.RuntimeToolResult{
		Status: "failed", Arguments: map[string]any{"tool": exaWebSearchToolName},
		Result: map[string]any{"error_code": code},
		ModelResult: map[string]any{
			"status": "failed", "error_code": code,
			"next_action": "Briefly explain that web search is unavailable for this request. Do not invent results or expose internal credentials.",
		},
		Error: errExaSearchFailed, Recoverable: recoverable,
	}
}

func executionHasSkill(resolved *ResolvedExecution, skillID string) bool {
	if resolved == nil {
		return false
	}
	for _, binding := range resolved.Snapshot.Bindings {
		if binding.Type == resourcebindings.BindingTypeSkill && strings.EqualFold(strings.TrimSpace(binding.ResourceID), strings.TrimSpace(skillID)) {
			return true
		}
	}
	return false
}

func unavailableExaPreparation(message string) *DirectSkillPreparation {
	return &DirectSkillPreparation{SystemPromptAddendum: strings.TrimSpace(message)}
}

func directSkillConfigurationView(stored *DirectSkillCredential) *DirectSkillConfiguration {
	if stored == nil {
		return &DirectSkillConfiguration{SkillID: DirectSkillExaWebSearch}
	}
	return &DirectSkillConfiguration{
		SkillID: stored.SkillID, Configured: strings.TrimSpace(stored.EncryptedCredentials) != "",
		Enabled: stored.Enabled, Revision: stored.Revision, UpdatedAt: stored.UpdatedAt,
	}
}

func directSkillCredentialAAD(organizationID, credentialID uuid.UUID, version int) integrations.CredentialAAD {
	return integrations.CredentialAAD{OrganizationID: organizationID, ConnectionID: credentialID, IntegrationID: exaCredentialAADID, CredentialVersion: version}
}

func destroyDirectSkillCredentials(credentials map[string]string) {
	for key := range credentials {
		credentials[key] = ""
		delete(credentials, key)
	}
}

func normalizeDirectSkillID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// GetDirectSkillConfiguration exposes only secret-free state to the console.
func (s *RuntimeService) GetDirectSkillConfiguration(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, skillID string) (*DirectSkillConfiguration, error) {
	if s == nil || s.directSkills == nil {
		return nil, ErrInvalidRequest
	}
	return s.directSkills.GetConfiguration(ctx, scope, digitalHumanID, skillID)
}

// UpdateDirectSkillConfiguration stores a DigitalHuman-owned credential
// without routing it through the shared connector management subsystem.
func (s *RuntimeService) UpdateDirectSkillConfiguration(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, skillID string, request UpdateDirectSkillConfigurationRequest) (*DirectSkillConfiguration, error) {
	if s == nil || s.directSkills == nil {
		return nil, ErrInvalidRequest
	}
	return s.directSkills.UpdateConfiguration(ctx, scope, digitalHumanID, skillID, request)
}
