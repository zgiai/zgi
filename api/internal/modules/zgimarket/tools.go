package zgimarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
)

// toolCatalogTTL is how long the tool list of a bound listing is reused before the gateway is
// asked again.
const toolCatalogTTL = time.Minute

// secretConnectorID is the connector identity the binding secrets are sealed under.
const secretConnectorID = "zgi-market"

// SecretSealer seals and opens the credential the market issued for a binding.
type SecretSealer interface {
	Protect(subject credentials.Subject, plaintext []byte) (string, error)
	Open(subject credentials.Subject, envelope string) ([]byte, error)
}

// Tools lets organization administrators bind activated market MCP servers to their agents and
// digital humans, and lets the runtime call the bound tools at the market gateway with the
// credential the market issued for that binding.
type Tools struct {
	proxy   *Proxy
	store   toolBindingStore
	sealer  SecretSealer
	gateway *gatewayClient
	now     func() time.Time

	mu      sync.Mutex
	catalog map[string]cachedCatalog
}

type cachedCatalog struct {
	tools   []GatewayTool
	expires time.Time
}

// NewTools builds the market tools. gateway is nil when no gateway address is configured: binding
// still works, but no tool reaches the runtime. The proxy that reaches the console is attached by
// Proxy.WithTools.
func NewTools(db *gorm.DB, sealer SecretSealer, gateway *GatewayConfig) (*Tools, error) {
	tools := &Tools{
		store: toolBindingStore{db: db}, sealer: sealer, now: time.Now,
		catalog: map[string]cachedCatalog{},
	}
	if gateway != nil && gateway.URL != "" {
		client, err := newGatewayClient(*gateway)
		if err != nil {
			return nil, err
		}
		tools.gateway = client
	}
	return tools, nil
}

// ToolCandidate is an activated MCP listing of the organization that may be bound.
type ToolCandidate struct {
	ListingID          uint64              `json:"listing_id"`
	Slug               string              `json:"slug"`
	Title              map[string]string   `json:"title"`
	SubscriptionID     uint64              `json:"subscription_id"`
	Tools              []CandidateTool     `json:"tools"`
	RequiredConnectors []RequiredConnector `json:"required_connectors"`
}

// CandidateTool is one tool of a candidate listing.
type CandidateTool struct {
	Name        string            `json:"name"`
	Description map[string]string `json:"description"`
	RiskLevel   string            `json:"risk_level"`
}

// RequiredConnector is a connector whose connection the organization needs for a listing.
type RequiredConnector struct {
	Slug     string            `json:"slug"`
	AuthType string            `json:"auth_type"`
	Title    map[string]string `json:"title"`
}

// Connection is a connection of the organization at the market.
type Connection struct {
	ID            uint64 `json:"id"`
	ConnectorSlug string `json:"connector_slug"`
	Name          string `json:"name"`
	Status        string `json:"status"`
}

// ToolCandidates is what a subject may be given.
type ToolCandidates struct {
	Listings    []ToolCandidate `json:"listings"`
	Connections []Connection    `json:"connections"`
}

// BindingView is the binding of one subject as the console shows it.
type BindingView struct {
	SubjectType string            `json:"subject_type"`
	SubjectID   string            `json:"subject_id"`
	Items       []ToolBindingItem `json:"items"`
	UpdatedAt   *time.Time        `json:"updated_at"`
	// GatewayConfigured reports whether bound tools reach the runtime on this instance.
	GatewayConfigured bool `json:"gateway_configured"`
}

// BindInput names the listings a subject may use, each with an optional connection.
type BindInput struct {
	Items []struct {
		ListingID    uint64  `json:"listing_id"`
		ConnectionID *uint64 `json:"connection_id"`
	} `json:"items"`
}

// marketAnswerError carries an answer of the market that the caller should see as it is.
type marketAnswerError struct{ answer *answer }

func (e *marketAnswerError) Error() string {
	return fmt.Sprintf("the market answered %d", e.answer.status)
}

func (t *Tools) market(ctx context.Context, who caller, method, path string, body any) (*answer, error) {
	var payload []byte
	headers := http.Header{}
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = encoded
		headers.Set("Content-Type", "application/json")
	}
	return t.proxy.call(ctx, who, method, path, payload, headers, answerBodyLimit)
}

// marketData calls the market and decodes the data member of a successful answer.
func (t *Tools) marketData(ctx context.Context, who caller, method, path string, body any, into any) error {
	result, err := t.market(ctx, who, method, path, body)
	if err != nil {
		return err
	}
	if result.status < 200 || result.status > 299 {
		return &marketAnswerError{answer: result}
	}
	if into == nil {
		return nil
	}
	envelope := struct {
		Data json.RawMessage `json:"data"`
	}{}
	if err := json.Unmarshal(result.content, &envelope); err != nil {
		return fmt.Errorf("decode the market answer: %w", err)
	}
	return json.Unmarshal(envelope.Data, into)
}

// Candidates lists the organization's activated MCP listings and its connections.
func (t *Tools) Candidates(ctx context.Context, who caller) (*ToolCandidates, error) {
	var subscriptions []struct {
		ID          uint64 `json:"id"`
		ListingID   uint64 `json:"listing_id"`
		ListingKind string `json:"listing_kind"`
		ListingSlug string `json:"listing_slug"`
		Status      string `json:"status"`
	}
	if err := t.marketData(ctx, who, http.MethodGet, "subscriptions?per_page=100", nil, &subscriptions); err != nil {
		return nil, err
	}
	result := &ToolCandidates{Listings: []ToolCandidate{}, Connections: []Connection{}}
	for _, subscription := range subscriptions {
		if subscription.ListingKind != "mcp" || subscription.Status != "active" || len(result.Listings) >= 100 {
			continue
		}
		var listing struct {
			ID   uint64 `json:"id"`
			Card struct {
				Title map[string]string `json:"title"`
			} `json:"card"`
			MCP *struct {
				Tools []CandidateTool `json:"tools"`
			} `json:"mcp"`
			RequiredConnectors []RequiredConnector `json:"required_connectors"`
		}
		err := t.marketData(ctx, who, http.MethodGet, "listings/mcp/"+subscription.ListingSlug, nil, &listing)
		var refused *marketAnswerError
		if errors.As(err, &refused) && refused.answer.status == http.StatusNotFound {
			// Delisted since it was activated: it cannot be bound any more.
			continue
		}
		if err != nil {
			return nil, err
		}
		candidate := ToolCandidate{
			ListingID: subscription.ListingID, Slug: subscription.ListingSlug, Title: listing.Card.Title,
			SubscriptionID: subscription.ID, Tools: []CandidateTool{}, RequiredConnectors: listing.RequiredConnectors,
		}
		if listing.MCP != nil {
			candidate.Tools = listing.MCP.Tools
		}
		if candidate.RequiredConnectors == nil {
			candidate.RequiredConnectors = []RequiredConnector{}
		}
		result.Listings = append(result.Listings, candidate)
	}
	var connections []Connection
	if err := t.marketData(ctx, who, http.MethodGet, "connections", nil, &connections); err != nil {
		return nil, err
	}
	if connections != nil {
		result.Connections = connections
	}
	return result, nil
}

func (t *Tools) subject(ctx context.Context, who caller, subjectType, rawID string) (uuid.UUID, uuid.UUID, error) {
	organizationID, err := uuid.Parse(who.organizationID)
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrSubjectNotFound
	}
	subjectID, err := uuid.Parse(rawID)
	if err != nil || (subjectType != SubjectAgent && subjectType != SubjectDigitalHuman) {
		return uuid.Nil, uuid.Nil, ErrSubjectNotFound
	}
	exists, err := t.store.subjectExists(ctx, organizationID, subjectType, subjectID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if !exists {
		return uuid.Nil, uuid.Nil, ErrSubjectNotFound
	}
	return organizationID, subjectID, nil
}

// Binding returns the market tools of one subject.
func (t *Tools) Binding(ctx context.Context, who caller, subjectType, rawID string) (*BindingView, error) {
	organizationID, subjectID, err := t.subject(ctx, who, subjectType, rawID)
	if err != nil {
		return nil, err
	}
	view := &BindingView{
		SubjectType: subjectType, SubjectID: subjectID.String(), Items: []ToolBindingItem{},
		GatewayConfigured: t.gateway != nil,
	}
	binding, err := t.store.find(ctx, organizationID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	if binding != nil {
		view.Items, view.UpdatedAt = binding.Items, &binding.UpdatedAt
	}
	return view, nil
}

// Bind replaces the market tools of one subject. The market binding follows: it is created with
// the first listing, its items replaced afterwards, and deleted with the last listing.
func (t *Tools) Bind(ctx context.Context, who caller, subjectType, rawID string, input BindInput) (*BindingView, error) {
	organizationID, subjectID, err := t.subject(ctx, who, subjectType, rawID)
	if err != nil {
		return nil, err
	}
	accountID, err := uuid.Parse(who.accountID)
	if err != nil {
		return nil, ErrSubjectNotFound
	}
	if len(input.Items) > maxBoundListings {
		return nil, fmt.Errorf("%w: at most %d listings", ErrListingNotBindable, maxBoundListings)
	}
	items, err := t.resolveItems(ctx, who, input)
	if err != nil {
		return nil, err
	}
	existing, err := t.store.find(ctx, organizationID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if existing != nil {
			if err := t.marketData(ctx, who, http.MethodDelete, "bindings/"+strconv.FormatUint(existing.MarketBindingID, 10), nil, nil); err != nil && !isNotFound(err) {
				return nil, err
			}
			if err := t.store.delete(ctx, existing.ID); err != nil {
				return nil, err
			}
			t.forget(existing.ID)
		}
		return t.Binding(ctx, who, subjectType, rawID)
	}
	marketItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{"subscription_id": item.SubscriptionID}
		if item.ConnectionID != nil {
			entry["connection_id"] = *item.ConnectionID
		}
		marketItems = append(marketItems, entry)
	}
	if existing != nil {
		var replaced struct {
			Version int `json:"version"`
		}
		err := t.marketData(ctx, who, http.MethodPut, "bindings/"+strconv.FormatUint(existing.MarketBindingID, 10)+"/items",
			map[string]any{"expected_version": existing.MarketVersion, "items": marketItems}, &replaced)
		switch {
		case err == nil:
			existing.Items, existing.MarketVersion, existing.UpdatedBy = items, replaced.Version, accountID
			if err := t.store.save(ctx, existing); err != nil {
				return nil, err
			}
			t.forget(existing.ID)
			return t.Binding(ctx, who, subjectType, rawID)
		case isNotFound(err) || isConflict(err):
			// Deleted or changed at the market since: start a new binding with a new credential.
			if err := t.marketData(ctx, who, http.MethodDelete, "bindings/"+strconv.FormatUint(existing.MarketBindingID, 10), nil, nil); err != nil && !isNotFound(err) {
				return nil, err
			}
			if err := t.store.delete(ctx, existing.ID); err != nil {
				return nil, err
			}
			t.forget(existing.ID)
		default:
			return nil, err
		}
	}
	var created struct {
		Binding struct {
			ID      uint64 `json:"id"`
			Version int    `json:"version"`
		} `json:"binding"`
		SecretKey string `json:"secret_key"`
	}
	name := "pre " + subjectType + " " + subjectID.String()[:8]
	err = t.marketData(ctx, who, http.MethodPost, "bindings", map[string]any{
		"consumer_type": subjectType, "consumer_ref": consumerRef(subjectType, subjectID), "name": name, "items": marketItems,
	}, &created)
	if err != nil {
		return nil, err
	}
	binding := &ToolBinding{
		ID: uuid.New(), OrganizationID: organizationID, SubjectType: subjectType, SubjectID: subjectID,
		MarketBindingID: created.Binding.ID, MarketVersion: created.Binding.Version, Items: items,
		CreatedBy: accountID, UpdatedBy: accountID,
	}
	sealed, err := t.sealer.Protect(secretSubject(binding), []byte(created.SecretKey))
	if err != nil {
		// The credential cannot be kept: end the market binding so it is not left behind.
		_ = t.marketData(ctx, who, http.MethodDelete, "bindings/"+strconv.FormatUint(created.Binding.ID, 10), nil, nil)
		return nil, fmt.Errorf("seal the market credential: %w", err)
	}
	binding.SecretCiphertext = sealed
	if err := t.store.save(ctx, binding); err != nil {
		_ = t.marketData(ctx, who, http.MethodDelete, "bindings/"+strconv.FormatUint(created.Binding.ID, 10), nil, nil)
		return nil, err
	}
	return t.Binding(ctx, who, subjectType, rawID)
}

// resolveItems checks every requested listing against the organization's candidates.
func (t *Tools) resolveItems(ctx context.Context, who caller, input BindInput) ([]ToolBindingItem, error) {
	if len(input.Items) == 0 {
		return nil, nil
	}
	candidates, err := t.Candidates(ctx, who)
	if err != nil {
		return nil, err
	}
	items := make([]ToolBindingItem, 0, len(input.Items))
	for _, requested := range input.Items {
		index := slices.IndexFunc(candidates.Listings, func(candidate ToolCandidate) bool {
			return candidate.ListingID == requested.ListingID
		})
		if index < 0 || slices.ContainsFunc(items, func(item ToolBindingItem) bool { return item.ListingID == requested.ListingID }) {
			return nil, ErrListingNotBindable
		}
		candidate := candidates.Listings[index]
		item := ToolBindingItem{
			ListingID: candidate.ListingID, Slug: candidate.Slug, Title: firstText(candidate.Title),
			SubscriptionID: candidate.SubscriptionID, ConnectionID: requested.ConnectionID, Tools: []ToolInfo{},
		}
		for _, tool := range candidate.Tools {
			item.Tools = append(item.Tools, ToolInfo{Name: tool.Name, RiskLevel: tool.RiskLevel})
		}
		items = append(items, item)
	}
	return items, nil
}

func firstText(text map[string]string) string {
	for _, key := range []string{"zh-CN", "en-US"} {
		if value := strings.TrimSpace(text[key]); value != "" {
			return value
		}
	}
	return ""
}

func isNotFound(err error) bool {
	var refused *marketAnswerError
	return errors.As(err, &refused) && refused.answer.status == http.StatusNotFound
}

func isConflict(err error) bool {
	var refused *marketAnswerError
	return errors.As(err, &refused) && refused.answer.status == http.StatusConflict
}

func secretSubject(binding *ToolBinding) credentials.Subject {
	return credentials.Subject{
		Purpose: credentials.MarketBindingSecret, OrganizationID: binding.OrganizationID,
		ID: binding.ID, ConnectorID: secretConnectorID, Version: 1,
	}
}

func (t *Tools) forget(bindingID uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	prefix := bindingID.String() + "/"
	for key := range t.catalog {
		if strings.HasPrefix(key, prefix) {
			delete(t.catalog, key)
		}
	}
}

// SubjectTool is one market tool of a subject as the runtime exposes it.
type SubjectTool struct {
	// ProviderID names the subject and the listing; the tool manager resolves it back.
	ProviderID  string
	Name        string
	Description string
	InputSchema map[string]any
	// RiskLevel is read, write, or destructive, as the market stated it when the listing was
	// bound. A tool the market did not describe then counts as write.
	RiskLevel string
}

// providerName encodes the subject and the listing of a market tool provider.
func providerName(subjectType string, subjectID uuid.UUID, slug string) string {
	return subjectType + ":" + subjectID.String() + ":" + slug
}

func parseProviderName(name string) (string, uuid.UUID, string, bool) {
	parts := strings.SplitN(name, ":", 3)
	if len(parts) != 3 || (parts[0] != SubjectAgent && parts[0] != SubjectDigitalHuman) || !marketSlugPattern.MatchString(parts[2]) {
		return "", uuid.Nil, "", false
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return "", uuid.Nil, "", false
	}
	return parts[0], id, parts[2], true
}

// SubjectTools lists the market tools of one agent or digital human, read from the gateway with
// the subject's credential. A listing whose tools cannot be read is left out, so one unavailable
// server does not take the others away.
func (t *Tools) SubjectTools(ctx context.Context, organizationID uuid.UUID, subjectType string, subjectID uuid.UUID) ([]SubjectTool, error) {
	if t == nil || t.gateway == nil || organizationID == uuid.Nil || subjectID == uuid.Nil {
		return nil, nil
	}
	binding, err := t.store.find(ctx, organizationID, subjectType, subjectID)
	if err != nil || binding == nil {
		return nil, err
	}
	result := make([]SubjectTool, 0)
	for _, item := range binding.Items {
		listed, err := t.listingTools(ctx, binding, item.Slug)
		if err != nil {
			continue
		}
		for _, tool := range listed {
			result = append(result, SubjectTool{
				ProviderID: providerName(subjectType, subjectID, item.Slug), Name: tool.Name,
				Description: tool.Description, InputSchema: tool.InputSchema, RiskLevel: riskOf(item, tool.Name),
			})
		}
	}
	return result, nil
}

func riskOf(item ToolBindingItem, name string) string {
	for _, tool := range item.Tools {
		if tool.Name == name {
			switch tool.RiskLevel {
			case RiskRead, RiskWrite, RiskDestructive:
				return tool.RiskLevel
			}
		}
	}
	return RiskWrite
}

// listingTools returns the tools of a bound listing, from the cache or the gateway.
func (t *Tools) listingTools(ctx context.Context, binding *ToolBinding, slug string) ([]GatewayTool, error) {
	key := binding.ID.String() + "/" + strconv.Itoa(binding.MarketVersion) + "/" + slug
	t.mu.Lock()
	cached, ok := t.catalog[key]
	t.mu.Unlock()
	if ok && t.now().Before(cached.expires) {
		return cached.tools, nil
	}
	secret, err := t.secret(binding)
	if err != nil {
		return nil, err
	}
	listed, err := t.gateway.ListTools(ctx, slug, secret)
	if err != nil {
		return nil, err
	}
	valid := make([]GatewayTool, 0, len(listed))
	for _, tool := range listed {
		if toolNamePattern.MatchString(tool.Name) {
			valid = append(valid, tool)
		}
	}
	t.mu.Lock()
	t.catalog[key] = cachedCatalog{tools: valid, expires: t.now().Add(toolCatalogTTL)}
	t.mu.Unlock()
	return valid, nil
}

func (t *Tools) secret(binding *ToolBinding) (string, error) {
	plaintext, err := t.sealer.Open(secretSubject(binding), binding.SecretCiphertext)
	if err != nil {
		return "", fmt.Errorf("open the market credential: %w", err)
	}
	return string(plaintext), nil
}

// callTool calls one tool of a subject's bound listing at the gateway.
func (t *Tools) callTool(ctx context.Context, organizationID uuid.UUID, name, tool string, arguments map[string]any) (*GatewayResult, error) {
	if t.gateway == nil {
		return nil, ErrMarketToolsUnavailable
	}
	subjectType, subjectID, slug, ok := parseProviderName(name)
	if !ok {
		return nil, ErrListingNotBindable
	}
	binding, err := t.store.find(ctx, organizationID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, ErrListingNotBindable
	}
	if _, bound := binding.item(slug); !bound {
		return nil, ErrListingNotBindable
	}
	secret, err := t.secret(binding)
	if err != nil {
		return nil, err
	}
	return t.gateway.CallTool(ctx, slug, secret, tool, arguments)
}
