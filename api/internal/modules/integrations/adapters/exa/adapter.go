package exa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	maxHighlightsPerResult          = 10
	maxHighlightCandidatesPerResult = 50
)

type Config struct {
	// APIKey is retained only for backwards-compatible direct adapter tests.
	// Production execution supplies credentials through the resolved connection.
	APIKey               string
	Timeout              time.Duration
	MaxResults           int
	DefaultSearchType    string
	MaxFetchURLs         int
	MaxContentCharacters int
}

type Adapter struct {
	config Config
	client *client
}

func New(config Config, httpClient *http.Client) (*Adapter, error) {
	return newWithBaseURL(config, httpClient, officialBaseURL)
}

func newWithBaseURL(config Config, httpClient *http.Client, baseURL string) (*Adapter, error) {
	if config.MaxResults <= 0 || config.MaxFetchURLs <= 0 || config.MaxContentCharacters <= 0 {
		return nil, fmt.Errorf("Exa result limits must be positive")
	}
	if config.MaxResults > 10 {
		config.MaxResults = 10
	}
	config.DefaultSearchType = strings.ToLower(strings.TrimSpace(config.DefaultSearchType))
	if config.DefaultSearchType == "" {
		config.DefaultSearchType = "auto"
	}
	switch config.DefaultSearchType {
	case "auto", "fast", "instant":
	default:
		return nil, fmt.Errorf("Exa default search type must be auto, fast, or instant")
	}
	if config.MaxFetchURLs > 5 {
		config.MaxFetchURLs = 5
	}
	if config.MaxContentCharacters > 20000 {
		config.MaxContentCharacters = 20000
	}
	if config.Timeout <= 0 {
		config.Timeout = 20 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}
	httpClientCopy := *httpClient
	httpClientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	httpClient = &httpClientCopy
	return &Adapter{config: config, client: newClient(baseURL, httpClient)}, nil
}

func (a *Adapter) DriverID() string { return integrations.DriverExa }

// ValidateConnection performs the smallest supported Exa search. Exa does not
// expose a free credential-introspection endpoint, so callers must surface that
// this validation can incur the provider's normal request cost.
func (a *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	result, err := a.Execute(ctx, integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input: map[string]interface{}{
			"query":       "ZGI connection test",
			"num_results": 1,
			"search_type": "instant",
		},
		Connection: connection,
	})
	if err != nil {
		return nil, err
	}
	return &integrations.ConnectionProfile{
		DisplayName:       "Exa",
		ProviderRequestID: result.ProviderRequestID,
		CostUSD:           result.CostUSD,
	}, nil
}

func (a *Adapter) Execute(ctx context.Context, req integrations.ActionRequest) (*integrations.ActionResult, error) {
	apiKey := a.apiKey(req.Connection)
	if apiKey == "" {
		return nil, integrations.NewError(integrations.ErrorCodeAuthInvalid, "external integration credentials are unavailable", nil)
	}
	callCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	switch req.ActionID {
	case integrations.ActionWebSearch:
		return a.search(callCtx, apiKey, req.Input)
	case integrations.ActionWebFetch:
		return a.fetch(callCtx, apiKey, req.Input)
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "unknown Exa action", nil)
	}
}

func (a *Adapter) apiKey(connection *integrations.ResolvedConnection) string {
	if connection != nil {
		if key := strings.TrimSpace(connection.Credentials["api_key"]); key != "" {
			return key
		}
	}
	return strings.TrimSpace(a.config.APIKey)
}

func (a *Adapter) search(ctx context.Context, apiKey string, input map[string]interface{}) (*integrations.ActionResult, error) {
	query := strings.TrimSpace(stringValue(input, "query"))
	numResults := integerValue(input, "num_results", 5)
	if numResults > a.config.MaxResults {
		numResults = a.config.MaxResults
	}
	searchType := strings.TrimSpace(stringValue(input, "search_type"))
	if searchType == "" {
		searchType = a.config.DefaultSearchType
	}
	request := searchRequest{
		Query:              query,
		NumResults:         numResults,
		Type:               searchType,
		IncludeDomains:     stringValues(input, "include_domains"),
		ExcludeDomains:     stringValues(input, "exclude_domains"),
		StartPublishedDate: stringValue(input, "start_published_date"),
		EndPublishedDate:   stringValue(input, "end_published_date"),
		Contents: map[string]interface{}{
			"highlights": map[string]interface{}{"query": query, "maxCharacters": 2000},
		},
	}
	if err := validateDateRange(request.StartPublishedDate, request.EndPublishedDate); err != nil {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "published date range is invalid", err)
	}
	var upstream response
	attempts, requestID, err := a.client.post(ctx, apiKey, "/search", request, &upstream)
	if err != nil {
		return &integrations.ActionResult{
			ProviderRequestID:   requestID,
			ProviderDiagnostics: integrations.ProviderDiagnosticsFromError(err),
			AttemptCount:        attempts,
		}, err
	}
	resultLimit := minInt(len(upstream.Results), minInt(a.config.MaxResults, numResults))
	results := make([]interface{}, 0, resultLimit)
	remainingHighlights := 12000
	for _, item := range upstream.Results[:resultLimit] {
		sourceURL := sanitizeSourceURL(item.URL)
		if sourceURL == "" {
			continue
		}
		highlights := make([]interface{}, 0, len(item.Highlights))
		for index, highlight := range item.Highlights {
			if index >= maxHighlightCandidatesPerResult || len(highlights) >= maxHighlightsPerResult {
				break
			}
			if remainingHighlights <= 0 {
				break
			}
			value := truncateText(highlight, minInt(2000, remainingHighlights))
			if value == "" {
				continue
			}
			remainingHighlights -= len([]rune(value))
			highlights = append(highlights, value)
		}
		results = append(results, map[string]interface{}{
			"title":        truncateText(item.Title, 500),
			"url":          sourceURL,
			"published_at": truncateText(item.PublishedDate, 64),
			"author":       truncateText(item.Author, 300),
			"highlights":   highlights,
		})
	}
	cost, costValue := parseCost(upstream.CostDollars)
	output := map[string]interface{}{
		"schema_version":       "zgi.web_search.v1",
		"provider":             integrations.DriverExa,
		"request_id":           boundedRequestID(upstream.RequestID),
		"cost_usd":             costValue,
		"resolved_search_type": truncateText(upstream.ResolvedSearchType, 32),
		"results":              results,
	}
	return &integrations.ActionResult{Output: output, ProviderRequestID: boundedRequestID(upstream.RequestID), CostUSD: cost, ResultCount: len(results), AttemptCount: attempts}, nil
}

func (a *Adapter) fetch(ctx context.Context, apiKey string, input map[string]interface{}) (*integrations.ActionResult, error) {
	urls := stringValues(input, "urls")
	if len(urls) > a.config.MaxFetchURLs {
		urls = urls[:a.config.MaxFetchURLs]
	}
	maxCharacters := integerValue(input, "max_characters", 10000)
	if maxCharacters > a.config.MaxContentCharacters {
		maxCharacters = a.config.MaxContentCharacters
	}
	mode := strings.TrimSpace(stringValue(input, "content_mode"))
	if mode == "" {
		mode = "text"
	}
	request := contentsRequest{URLs: urls}
	switch freshness := stringValue(input, "freshness"); freshness {
	case "", "prefer_cache":
	case "force_live":
		maxAge := 0
		request.MaxAgeHours = &maxAge
	case "cache_only":
		maxAge := -1
		request.MaxAgeHours = &maxAge
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "unsupported webpage freshness policy", nil)
	}
	if mode == "highlights" {
		request.Highlights = map[string]interface{}{"query": stringValue(input, "highlight_query"), "maxCharacters": maxCharacters}
	} else {
		request.Text = map[string]interface{}{"maxCharacters": maxCharacters}
	}
	var upstream response
	attempts, requestID, err := a.client.post(ctx, apiKey, "/contents", request, &upstream)
	if err != nil {
		return &integrations.ActionResult{
			ProviderRequestID:   requestID,
			ProviderDiagnostics: integrations.ProviderDiagnosticsFromError(err),
			AttemptCount:        attempts,
		}, err
	}
	requestedLimit := minInt(len(urls), a.config.MaxFetchURLs)
	resultLimit := minInt(len(upstream.Results), requestedLimit)
	results := make([]interface{}, 0, resultLimit)
	seenURLs := make(map[string]struct{}, requestedLimit)
	remaining := a.config.MaxContentCharacters * 2
	for _, item := range upstream.Results[:resultLimit] {
		sourceURL := sanitizeSourceURL(item.URL)
		if sourceURL == "" {
			continue
		}
		seenURLs[sourceURL] = struct{}{}
		limit := minInt(maxCharacters, remaining)
		text := truncateText(item.Text, limit)
		remaining -= len([]rune(text))
		highlights := make([]interface{}, 0, len(item.Highlights))
		for index, highlight := range item.Highlights {
			if index >= maxHighlightCandidatesPerResult || len(highlights) >= maxHighlightsPerResult {
				break
			}
			if remaining <= 0 {
				break
			}
			value := truncateText(highlight, minInt(2000, remaining))
			if value == "" {
				continue
			}
			remaining -= len([]rune(value))
			highlights = append(highlights, value)
		}
		results = append(results, map[string]interface{}{
			"title":        truncateText(item.Title, 500),
			"url":          sourceURL,
			"published_at": truncateText(item.PublishedDate, 64),
			"author":       truncateText(item.Author, 300),
			"text":         text,
			"highlights":   highlights,
			"status":       "success",
		})
		if remaining <= 0 {
			break
		}
	}
	for _, item := range upstream.Statuses {
		if len(results) >= requestedLimit {
			break
		}
		if strings.EqualFold(strings.TrimSpace(item.Status), "success") {
			continue
		}
		sourceURL := sanitizeSourceURL(item.ID)
		if sourceURL == "" {
			continue
		}
		if _, exists := seenURLs[sourceURL]; exists {
			continue
		}
		seenURLs[sourceURL] = struct{}{}
		failedResult := map[string]interface{}{
			"title":        "",
			"url":          sourceURL,
			"published_at": "",
			"author":       "",
			"text":         "",
			"highlights":   []interface{}{},
			"status":       "failed",
		}
		if errorCode := exaStatusErrorCode(item.Error); errorCode != "" {
			failedResult["error_code"] = errorCode
		}
		results = append(results, failedResult)
	}
	cost, costValue := parseCost(upstream.CostDollars)
	output := map[string]interface{}{
		"schema_version": "zgi.web_fetch.v1",
		"provider":       integrations.DriverExa,
		"request_id":     boundedRequestID(upstream.RequestID),
		"cost_usd":       costValue,
		"results":        results,
	}
	return &integrations.ActionResult{Output: output, ProviderRequestID: boundedRequestID(upstream.RequestID), CostUSD: cost, ResultCount: len(results), AttemptCount: attempts}, nil
}

func exaStatusErrorCode(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var envelope struct {
		Tag string `json:"tag"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return ""
	}
	return boundedProviderCode(envelope.Tag)
}

func parseCost(raw json.RawMessage) (*float64, interface{}) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		if value, err := number.Float64(); err == nil && value >= 0 {
			return &value, value
		}
	}
	var object struct {
		Total json.Number `json:"total"`
	}
	if err := json.Unmarshal(raw, &object); err == nil && object.Total.String() != "" {
		if value, err := object.Total.Float64(); err == nil && value >= 0 {
			return &value, value
		}
	}
	return nil, nil
}

func validateDateRange(start, end string) error {
	var startTime, endTime time.Time
	var err error
	if strings.TrimSpace(start) != "" {
		startTime, err = time.Parse(time.RFC3339, start)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(end) != "" {
		endTime, err = time.Parse(time.RFC3339, end)
		if err != nil {
			return err
		}
	}
	if !startTime.IsZero() && !endTime.IsZero() && startTime.After(endTime) {
		return fmt.Errorf("start date is after end date")
	}
	return nil
}

func stringValue(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func stringValues(input map[string]interface{}, key string) []string {
	values := integrationsStringSlice(input[key])
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func integrationsStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func integerValue(input map[string]interface{}, key string, fallback int) int {
	value, ok := optionalIntegerValue(input, key)
	if !ok {
		return fallback
	}
	return value
}

func optionalIntegerValue(input map[string]interface{}, key string) (int, bool) {
	value, ok := input[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed == float64(int(typed))
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	default:
		return 0, false
	}
}

func truncateText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func boundedRequestID(value string) string {
	return truncateText(value, 128)
}

func sanitizeSourceURL(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) == 0 || len(value) > 2048 {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	parsed.Fragment = ""
	if integrations.ValidatePublicWebURL(parsed.String()) != nil {
		return ""
	}
	return parsed.String()
}
