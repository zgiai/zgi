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

	mu         sync.Mutex
	catalog    map[string]cachedCatalog
	operations map[string]cachedOperations
}

type cachedCatalog struct {
	tools   []GatewayTool
	expires time.Time
}

type cachedOperations struct {
	operations []BoundOperation
	expires    time.Time
}

// NewTools builds the market tools. gateway is nil when no gateway address is configured: binding
// still works, but no tool reaches the runtime. The proxy that reaches the console is attached by
// Proxy.WithTools.
func NewTools(db *gorm.DB, sealer SecretSealer, gateway *GatewayConfig) (*Tools, error) {
	tools := &Tools{
		store: toolBindingStore{db: db}, sealer: sealer, now: time.Now,
		catalog: map[string]cachedCatalog{}, operations: map[string]cachedOperations{},
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

// ToolCandidate is an activated listing of the organization that may be bound: an MCP server, or
// one http offering of an API listing.
type ToolCandidate struct {
	// Kind is ItemKindMCP or ItemKindAPI.
	Kind               string              `json:"kind"`
	ListingID          uint64              `json:"listing_id"`
	Slug               string              `json:"slug"`
	Title              map[string]string   `json:"title"`
	SubscriptionID     uint64              `json:"subscription_id"`
	Tools              []CandidateTool     `json:"tools"`
	RequiredConnectors []RequiredConnector `json:"required_connectors"`
	// OfferingSlug and Operations describe an API offering. Tools then lists its bindable
	// operations by tool name.
	OfferingSlug string               `json:"offering_slug,omitempty"`
	Operations   []CandidateOperation `json:"operations,omitempty"`
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

// BindInput names the listings a subject may use, each with an optional connection. An item with
// an offering slug is an API offering; its operations are operation keys, or empty for all.
type BindInput struct {
	Items []BindItem `json:"items"`
}

// BindItem is one requested listing of a subject.
type BindItem struct {
	ListingID    uint64   `json:"listing_id"`
	ConnectionID *uint64  `json:"connection_id"`
	OfferingSlug string   `json:"offering_slug"`
	Operations   []string `json:"operations"`
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

// Candidates lists what the organization activated and may bind, and its connections: its MCP
// servers, and the http offerings of its API listings with their operations. An offering of a
// model protocol family has no declared operations and is not listed.
func (t *Tools) Candidates(ctx context.Context, who caller) (*ToolCandidates, error) {
	var subscriptions []struct {
		ID           uint64 `json:"id"`
		ListingID    uint64 `json:"listing_id"`
		ListingKind  string `json:"listing_kind"`
		ListingSlug  string `json:"listing_slug"`
		OfferingSlug string `json:"offering_slug"`
		Status       string `json:"status"`
	}
	if err := t.marketData(ctx, who, http.MethodGet, "subscriptions?per_page=100", nil, &subscriptions); err != nil {
		return nil, err
	}
	result := &ToolCandidates{Listings: []ToolCandidate{}, Connections: []Connection{}}
	for _, subscription := range subscriptions {
		if (subscription.ListingKind != ItemKindMCP && subscription.ListingKind != ItemKindAPI) ||
			subscription.Status != "active" || len(result.Listings) >= 100 {
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
			Offerings          []marketOffering    `json:"offerings"`
			RequiredConnectors []RequiredConnector `json:"required_connectors"`
		}
		err := t.marketData(ctx, who, http.MethodGet, "listings/"+subscription.ListingKind+"/"+subscription.ListingSlug, nil, &listing)
		var refused *marketAnswerError
		if errors.As(err, &refused) && refused.answer.status == http.StatusNotFound {
			// Delisted since it was activated: it cannot be bound any more.
			continue
		}
		if err != nil {
			return nil, err
		}
		candidate := ToolCandidate{
			Kind: subscription.ListingKind, ListingID: subscription.ListingID, Slug: subscription.ListingSlug,
			Title: listing.Card.Title, SubscriptionID: subscription.ID, Tools: []CandidateTool{},
			RequiredConnectors: listing.RequiredConnectors,
		}
		if candidate.RequiredConnectors == nil {
			candidate.RequiredConnectors = []RequiredConnector{}
		}
		if subscription.ListingKind == ItemKindAPI {
			index := slices.IndexFunc(listing.Offerings, func(offering marketOffering) bool {
				return offering.Slug == subscription.OfferingSlug
			})
			if index < 0 || listing.Offerings[index].ProtocolFamily != httpProtocolFamily {
				continue
			}
			candidate.OfferingSlug = subscription.OfferingSlug
			candidate.Operations = candidateOperations(
				subscription.ListingSlug, listing.Offerings[index], soleHTTPOffering(listing.Offerings), firstText(listing.Card.Title),
			)
			for _, operation := range candidate.Operations {
				if operation.Bindable {
					text := map[string]string{"zh-CN": operation.Description, "en-US": operation.Description}
					candidate.Tools = append(candidate.Tools, CandidateTool{Name: operation.Name, Description: text, RiskLevel: operation.RiskLevel})
				}
			}
		} else if listing.MCP != nil {
			candidate.Tools = listing.MCP.Tools
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
		view.Items, view.UpdatedAt = viewItems(binding.Items), &binding.UpdatedAt
	}
	return view, nil
}

// viewItems are the items as a page shows them: without the schemas kept for the runtime.
func viewItems(items []ToolBindingItem) []ToolBindingItem {
	result := make([]ToolBindingItem, 0, len(items))
	for _, item := range items {
		if len(item.Operations) > 0 {
			operations := make([]BoundOperation, 0, len(item.Operations))
			for _, operation := range item.Operations {
				operation.InputSchema = nil
				operations = append(operations, operation)
			}
			item.Operations = operations
		}
		result = append(result, item)
	}
	return result
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
		wantsAPI := requested.OfferingSlug != ""
		index := slices.IndexFunc(candidates.Listings, func(candidate ToolCandidate) bool {
			if wantsAPI {
				return candidate.Kind == ItemKindAPI && candidate.ListingID == requested.ListingID && candidate.OfferingSlug == requested.OfferingSlug
			}
			return candidate.Kind == ItemKindMCP && candidate.ListingID == requested.ListingID
		})
		if index < 0 {
			return nil, ErrListingNotBindable
		}
		candidate := candidates.Listings[index]
		// One subscription is one item of the market binding.
		if slices.ContainsFunc(items, func(item ToolBindingItem) bool { return item.SubscriptionID == candidate.SubscriptionID }) {
			return nil, ErrListingNotBindable
		}
		item := ToolBindingItem{
			Kind: candidate.Kind, ListingID: candidate.ListingID, Slug: candidate.Slug, Title: firstText(candidate.Title),
			SubscriptionID: candidate.SubscriptionID, ConnectionID: requested.ConnectionID, Tools: []ToolInfo{},
		}
		if wantsAPI {
			operations, all, err := boundOperations(candidate, requested.Operations)
			if err != nil {
				return nil, err
			}
			item.OfferingSlug, item.Operations, item.AllOperations = candidate.OfferingSlug, operations, all
			for _, operation := range operations {
				item.Tools = append(item.Tools, ToolInfo{Name: operation.Name, RiskLevel: operation.RiskLevel})
			}
		} else {
			for _, tool := range candidate.Tools {
				item.Tools = append(item.Tools, ToolInfo{Name: tool.Name, RiskLevel: tool.RiskLevel})
			}
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
	for key := range t.operations {
		if strings.HasPrefix(key, prefix) {
			delete(t.operations, key)
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

// parseProviderName reads the subject and the listing of a provider name. The listing is the slug
// of an MCP server, or apiProviderPrefix and the slug of an API offering.
func parseProviderName(name string) (string, uuid.UUID, string, bool) {
	parts := strings.SplitN(name, ":", 3)
	if len(parts) != 3 || (parts[0] != SubjectAgent && parts[0] != SubjectDigitalHuman) ||
		!marketSlugPattern.MatchString(strings.TrimPrefix(parts[2], apiProviderPrefix)) {
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
	taken := map[string]bool{}
	for _, item := range binding.Items {
		if item.IsAPI() {
			continue
		}
		listed, err := t.listingTools(ctx, binding, item.Slug)
		if err != nil {
			continue
		}
		for _, tool := range listed {
			taken[tool.Name] = true
			result = append(result, SubjectTool{
				ProviderID: providerName(subjectType, subjectID, item.Slug), Name: tool.Name,
				Description: tool.Description, InputSchema: tool.InputSchema, RiskLevel: riskOf(item, tool.Name),
			})
		}
	}
	// The operations of the bound API offerings. A short name that another tool of the subject
	// already has, a vendor's MCP tool or an operation of another offering, gives way to the
	// operation's full name, which names its offering and request and which a call always finds.
	for _, item := range binding.Items {
		if !item.IsAPI() {
			continue
		}
		for _, operation := range t.liveOperations(ctx, binding, item) {
			name, free := freeOperationName(taken, item.OfferingSlug, operation)
			if !free {
				continue
			}
			result = append(result, SubjectTool{
				ProviderID: providerName(subjectType, subjectID, apiProviderPrefix+item.OfferingSlug), Name: name,
				Description: operation.Description, InputSchema: operation.InputSchema, RiskLevel: operation.RiskLevel,
			})
		}
	}
	return result, nil
}

// freeOperationName picks the name an operation is listed under among the names a subject
// already has, and takes it: its short name, or its full name when the short one is taken. It
// reports false when both are taken, which leaves the operation out.
func freeOperationName(taken map[string]bool, offeringSlug string, operation BoundOperation) (string, bool) {
	for _, name := range []string{operation.Name, OperationToolName(offeringSlug, operation.Method, operation.Path)} {
		if name != "" && !taken[name] {
			taken[name] = true
			return name, true
		}
	}
	return "", false
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
	if offeringSlug, isAPI := strings.CutPrefix(slug, apiProviderPrefix); isAPI {
		return t.callOperation(ctx, binding, offeringSlug, tool, arguments)
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

// callOperation calls the operation of a bound API offering that the tool name stands for.
// Arguments that do not fit the operation are answered as a tool error; nothing is called.
func (t *Tools) callOperation(ctx context.Context, binding *ToolBinding, offeringSlug, tool string, arguments map[string]any) (*GatewayResult, error) {
	item, bound := binding.offering(offeringSlug)
	if !bound {
		return nil, ErrListingNotBindable
	}
	operations := t.liveOperations(ctx, binding, item)
	// A call names the operation as it was listed: by its short name, or by its full name, which
	// is what a name that was taken falls back to and what a conversation begun earlier still holds.
	index := slices.IndexFunc(operations, func(operation BoundOperation) bool {
		return operation.Name == tool || OperationToolName(offeringSlug, operation.Method, operation.Path) == tool
	})
	if index < 0 {
		return nil, ErrListingNotBindable
	}
	request, err := buildOperationRequest(operations[index].Method, operations[index].Path, arguments)
	if err != nil {
		return &GatewayResult{Text: err.Error(), IsError: true}, nil
	}
	secret, err := t.secret(binding)
	if err != nil {
		return nil, err
	}
	return t.gateway.CallOperation(ctx, offeringSlug, secret, request)
}
