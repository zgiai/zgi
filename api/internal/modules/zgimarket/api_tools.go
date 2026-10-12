package zgimarket

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The kinds of market listing a subject may be given.
const (
	ItemKindMCP = "mcp"
	ItemKindAPI = "api"
)

// Why an operation of an activated API offering cannot become a tool.
const (
	ReasonNoInputSchema        = "no_input_schema"
	ReasonInputSchemaNotObject = "input_schema_not_object"
)

const (
	gatewayOfferingPath = "/v1/gateway/"
	marketListingPath   = "/v1/market/listings/api/"
	// apiProviderPrefix marks the provider of an API offering; an MCP provider names its listing.
	apiProviderPrefix = "api."
	// httpProtocolFamily is the one protocol family whose operations are declared and can be tools.
	httpProtocolFamily = "http"
	// maxToolNameLength is the longest tool name the runtime and the models accept.
	maxToolNameLength = 64
	// maxOfferingOperations bounds the operations of one offering that become tools.
	maxOfferingOperations = 64
	// truncatedMarker ends a tool result that was cut at maxToolResultChars.
	truncatedMarker = "\n[truncated]"
)

var (
	pathParameterPattern = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	nameSeparators       = regexp.MustCompile(`[^A-Za-z0-9]+`)
	methodPattern        = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE)$`)
)

// BoundOperation is one operation of an API offering as it was described when it was bound. The
// runtime reads the offering again from the market on use, so a new version of the listing is
// followed; this copy is what is shown, and what is used when the market cannot be read.
type BoundOperation struct {
	// Key is the method and the path pattern, the identity of the operation within its offering.
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Method      string         `json:"method"`
	Path        string         `json:"path"`
	Description string         `json:"description,omitempty"`
	RiskLevel   string         `json:"risk_level"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// CandidateOperation is one operation of an activated API offering, bindable or not.
type CandidateOperation struct {
	Key          string          `json:"key"`
	Name         string          `json:"name"`
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Description  string          `json:"description"`
	RiskLevel    string          `json:"risk_level"`
	Bindable     bool            `json:"bindable"`
	Reason       string          `json:"reason,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// marketOperation is an operation of an http offering as the market's listing states it.
type marketOperation struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// RiskLevel is what the market says the operation does, when it says so. A lookup served by
	// POST is a read, which the method alone cannot tell.
	RiskLevel    string          `json:"risk_level"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

type marketOffering struct {
	ID             uint64            `json:"id"`
	Slug           string            `json:"slug"`
	ProtocolFamily string            `json:"protocol_family"`
	Status         string            `json:"status"`
	Operations     []marketOperation `json:"operations"`
}

// operationKey is the identity of an operation within its offering.
func operationKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

// operationRisk is the risk level of an operation: the one the market declares for it, and
// otherwise a guess from the method, where a GET reads and anything else writes. An unknown
// declared level is not trusted and falls back to the method.
func operationRisk(method, declared string) string {
	switch declared = strings.ToLower(strings.TrimSpace(declared)); declared {
	case RiskRead, RiskWrite, RiskDestructive:
		return declared
	}
	if strings.EqualFold(method, http.MethodGet) {
		return RiskRead
	}
	return RiskWrite
}

// OperationToolName derives the tool name of an operation from its offering, method and path:
// the offering slug and the request, lowercased and joined by underscores, as in
// `amap_web_service_standard__get_v3_geocode_geo`. A name longer than the limit keeps its head
// and ends in a digest of the whole, so two long names never meet.
func OperationToolName(offeringSlug, method, path string) string {
	clean := func(value string) string {
		return strings.Trim(nameSeparators.ReplaceAllString(strings.ToLower(value), "_"), "_")
	}
	name := clean(offeringSlug) + "__" + clean(method)
	if tail := clean(path); tail != "" {
		name += "_" + tail
	}
	if len(name) <= maxToolNameLength {
		return name
	}
	sum := sha256.Sum256([]byte(offeringSlug + " " + operationKey(method, path)))
	suffix := "_" + hex.EncodeToString(sum[:4])
	return strings.TrimRight(name[:maxToolNameLength-len(suffix)], "_") + suffix
}

// describeOperation says what an operation is when its schema does not: the request and the
// listing it belongs to.
func describeOperation(operation marketOperation, schema map[string]any, listingTitle string) string {
	for _, key := range []string{"description", "title"} {
		if text, ok := schema[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	description := strings.ToUpper(operation.Method) + " " + operation.Path
	if listingTitle = strings.TrimSpace(listingTitle); listingTitle != "" {
		description += " (" + listingTitle + ")"
	}
	return description
}

// candidateOperations turns the operations of an http offering into candidates. An operation
// becomes a tool only when it declares an input schema of type object: that is what tells a model
// which arguments to send.
func candidateOperations(offering marketOffering, listingTitle string) []CandidateOperation {
	result := make([]CandidateOperation, 0, len(offering.Operations))
	seen := map[string]bool{}
	for _, operation := range offering.Operations {
		method := strings.ToUpper(strings.TrimSpace(operation.Method))
		key := operationKey(method, operation.Path)
		if !methodPattern.MatchString(method) || !strings.HasPrefix(operation.Path, "/") || seen[key] {
			continue
		}
		seen[key] = true
		candidate := CandidateOperation{
			Key: key, Name: OperationToolName(offering.Slug, method, operation.Path), Method: method, Path: operation.Path,
			RiskLevel: operationRisk(method, operation.RiskLevel), InputSchema: operation.InputSchema, OutputSchema: operation.OutputSchema,
		}
		schema, reason := toolSchema(operation.InputSchema)
		candidate.Bindable, candidate.Reason = reason == "", reason
		candidate.Description = describeOperation(operation, schema, listingTitle)
		result = append(result, candidate)
	}
	return result
}

// toolSchema reads a declared input schema as a tool schema, or says why it cannot be one.
func toolSchema(raw json.RawMessage) (map[string]any, string) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, ReasonNoInputSchema
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || len(schema) == 0 {
		return nil, ReasonNoInputSchema
	}
	if kind, ok := schema["type"].(string); !ok || kind != "object" {
		return schema, ReasonInputSchemaNotObject
	}
	return schema, ""
}

// boundOperations selects the operations of a candidate an administrator asked for. No selection
// means every bindable operation, now and after the listing changes.
func boundOperations(candidate ToolCandidate, selected []string) ([]BoundOperation, bool, error) {
	all := len(selected) == 0
	wanted := map[string]bool{}
	for _, key := range selected {
		wanted[key] = true
	}
	result := make([]BoundOperation, 0, len(candidate.Operations))
	for _, operation := range candidate.Operations {
		if !all && !wanted[operation.Key] {
			continue
		}
		delete(wanted, operation.Key)
		if !operation.Bindable {
			if all {
				continue
			}
			return nil, false, fmt.Errorf("%w: %s has no usable input schema", ErrListingNotBindable, operation.Key)
		}
		schema, _ := toolSchema(operation.InputSchema)
		result = append(result, BoundOperation{
			Key: operation.Key, Name: operation.Name, Method: operation.Method, Path: operation.Path,
			Description: operation.Description, RiskLevel: operation.RiskLevel, InputSchema: schema,
		})
	}
	if len(wanted) > 0 {
		return nil, false, fmt.Errorf("%w: the offering has no such operation", ErrListingNotBindable)
	}
	if len(result) == 0 {
		return nil, false, fmt.Errorf("%w: the offering has no operation with an input schema", ErrListingNotBindable)
	}
	if len(result) > maxOfferingOperations {
		return nil, false, fmt.Errorf("%w: at most %d operations of one offering", ErrListingNotBindable, maxOfferingOperations)
	}
	return result, all, nil
}

// liveOperations are the operations of a bound offering as the market states them now, limited
// to what was selected. When the market cannot be read the copy made at binding time is used.
func (t *Tools) liveOperations(ctx context.Context, binding *ToolBinding, item ToolBindingItem) []BoundOperation {
	key := binding.ID.String() + "/" + strconv.Itoa(binding.MarketVersion) + "/" + apiProviderPrefix + item.OfferingSlug
	t.mu.Lock()
	cached, ok := t.operations[key]
	t.mu.Unlock()
	if ok && t.now().Before(cached.expires) {
		return cached.operations
	}
	operations := item.Operations
	if offering, title, err := t.gateway.Offering(ctx, item.Slug, item.OfferingSlug); err == nil {
		selected := map[string]bool{}
		for _, operation := range item.Operations {
			selected[operation.Key] = true
		}
		fresh := make([]BoundOperation, 0, len(item.Operations))
		for _, candidate := range candidateOperations(*offering, title) {
			if !candidate.Bindable || (!item.AllOperations && !selected[candidate.Key]) || len(fresh) >= maxOfferingOperations {
				continue
			}
			schema, _ := toolSchema(candidate.InputSchema)
			fresh = append(fresh, BoundOperation{
				Key: candidate.Key, Name: candidate.Name, Method: candidate.Method, Path: candidate.Path,
				Description: candidate.Description, RiskLevel: candidate.RiskLevel, InputSchema: schema,
			})
		}
		operations = fresh
	}
	t.mu.Lock()
	t.operations[key] = cachedOperations{operations: operations, expires: t.now().Add(toolCatalogTTL)}
	t.mu.Unlock()
	return operations
}

// operationRequest is the HTTP request one tool call makes at the gateway.
type operationRequest struct {
	Method string
	// Path is the operation path with its parameters filled in, each one escaped.
	Path  string
	Query url.Values
	// Body is the JSON body, nil for a request without one.
	Body []byte
}

// errArguments reports arguments that do not fit the operation.
var errArguments = errors.New("the arguments do not fit the operation")

// buildOperationRequest maps tool arguments onto the request of an operation. A `{name}` of the
// path is taken from the argument of that name. What is left is the query of a GET or DELETE,
// one value per scalar and one per array member, and the JSON body of any other method.
func buildOperationRequest(method, path string, arguments map[string]any) (*operationRequest, error) {
	rest := make(map[string]any, len(arguments))
	for name, value := range arguments {
		rest[name] = value
	}
	var failure error
	filled := pathParameterPattern.ReplaceAllStringFunc(path, func(match string) string {
		name := match[1 : len(match)-1]
		value, present := rest[name]
		text, scalar := scalarText(value)
		if !present || !scalar || text == "" {
			failure = fmt.Errorf("%w: the path needs the argument %q", errArguments, name)
			return match
		}
		delete(rest, name)
		return url.PathEscape(text)
	})
	if failure != nil {
		return nil, failure
	}
	request := &operationRequest{Method: method, Path: filled}
	if method != http.MethodGet && method != http.MethodDelete {
		body, err := json.Marshal(rest)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errArguments, err)
		}
		request.Body = body
		return request, nil
	}
	request.Query = url.Values{}
	names := make([]string, 0, len(rest))
	for name := range rest {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		switch value := rest[name].(type) {
		case nil:
		case []any:
			for _, member := range value {
				if text, scalar := scalarText(member); scalar {
					request.Query.Add(name, text)
					continue
				}
				encoded, _ := json.Marshal(member) //nolint:errcheck // decoded JSON encodes again
				request.Query.Add(name, string(encoded))
			}
		default:
			if text, scalar := scalarText(value); scalar {
				request.Query.Set(name, text)
				continue
			}
			encoded, _ := json.Marshal(value) //nolint:errcheck // decoded JSON encodes again
			request.Query.Set(name, string(encoded))
		}
	}
	return request, nil
}

// scalarText writes a JSON scalar the way a query or a path carries it.
func scalarText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case json.Number:
		return typed.String(), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	default:
		return "", false
	}
}

// Offering reads one offering of a published API listing from the market's public catalog, which
// is served at the gateway's origin. It sends no credential.
func (g *gatewayClient) Offering(ctx context.Context, listingSlug, offeringSlug string) (*marketOffering, string, error) {
	if !marketSlugPattern.MatchString(listingSlug) || !marketSlugPattern.MatchString(offeringSlug) {
		return nil, "", ErrListingNotBindable
	}
	ctx, cancel := context.WithTimeout(ctx, gatewayConnectTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.origin+marketListingPath+listingSlug, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Accept", "application/json")
	response, err := g.http.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, gatewayMessageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if response.StatusCode != http.StatusOK || len(content) > gatewayMessageBytes {
		return nil, "", fmt.Errorf("the market answered %d for the listing", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			Card struct {
				Title map[string]string `json:"title"`
			} `json:"card"`
			Offerings []marketOffering `json:"offerings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		return nil, "", fmt.Errorf("decode the listing: %w", err)
	}
	index := slices.IndexFunc(envelope.Data.Offerings, func(offering marketOffering) bool { return offering.Slug == offeringSlug })
	if index < 0 || envelope.Data.Offerings[index].ProtocolFamily != httpProtocolFamily {
		return nil, "", ErrListingNotBindable
	}
	return &envelope.Data.Offerings[index], firstText(envelope.Data.Card.Title), nil
}

// CallOperation calls one operation of a bound offering through the gateway. The market bills
// the call. An answer outside 2xx is a tool error that carries the market's error code.
func (g *gatewayClient) CallOperation(ctx context.Context, offeringSlug, secret string, call *operationRequest) (*GatewayResult, error) {
	if !marketSlugPattern.MatchString(offeringSlug) || secret == "" || strings.ContainsAny(secret, "\r\n") ||
		call == nil || !methodPattern.MatchString(call.Method) || !strings.HasPrefix(call.Path, "/") {
		return nil, ErrListingNotBindable
	}
	ctx, cancel := context.WithTimeout(ctx, gatewayCallTimeout)
	defer cancel()
	target := g.origin + gatewayOfferingPath + offeringSlug + call.Path
	if encoded := call.Query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	var body io.Reader
	if call.Body != nil {
		body = bytes.NewReader(call.Body)
	}
	request, err := http.NewRequestWithContext(ctx, call.Method, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	request.Header.Set("Accept", "application/json")
	if call.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := g.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, gatewayMessageBytes+1))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &GatewayResult{Text: marketFailure(response.StatusCode, content), IsError: true}, nil
	}
	return &GatewayResult{Text: capResult(string(content))}, nil
}

// capResult bounds a tool result handed to the model and says so when it was cut.
func capResult(text string) string {
	if len(text) <= maxToolResultChars {
		return text
	}
	cut := maxToolResultChars - len(truncatedMarker)
	// Do not end in the middle of a character.
	for cut > 0 && (text[cut]&0xC0) == 0x80 {
		cut--
	}
	return text[:cut] + truncatedMarker
}

// marketFailure describes an answer outside 2xx by the market's own error code and message.
func marketFailure(status int, content []byte) string {
	var failure struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	text := "ZGI Market answered " + strconv.Itoa(status)
	if json.Unmarshal(content, &failure) == nil && (failure.ErrorCode != "" || failure.Message != "") {
		if failure.ErrorCode != "" {
			text += " " + failure.ErrorCode
		}
		if failure.Message != "" {
			text += ": " + failure.Message
		}
		return text
	}
	if body := strings.TrimSpace(string(content)); body != "" {
		text += ": " + capResult(body)
	}
	return text
}
