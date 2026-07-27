package exa

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestActionSchemasAreValid(t *testing.T) {
	for _, action := range Actions() {
		if action.NameI18n[integrations.LocaleSimplifiedChinese] == "" || action.DescriptionI18n[integrations.LocaleSimplifiedChinese] == "" {
			t.Errorf("action %s is missing simplified Chinese metadata", action.ID)
		}
		assertDeclaredLabelsLocalized(t, "action "+action.ID+" scope", action.RequiredScopes, action.ScopeLabelsI18n)
		if err := tools.ValidateJSONSchema(action.InputSchema); err != nil {
			t.Errorf("action %s input schema: %v", action.ID, err)
		}
		if err := tools.ValidateJSONSchema(action.OutputSchema); err != nil {
			t.Errorf("action %s output schema: %v", action.ID, err)
		}
		properties, _ := action.InputSchema["properties"].(map[string]interface{})
		for field, rawSchema := range properties {
			property, _ := rawSchema.(map[string]interface{})
			titles, _ := property["title_i18n"].(integrations.LocalizedText)
			if titles[integrations.LocaleEnglishUS] == "" || titles[integrations.LocaleSimplifiedChinese] == "" {
				t.Errorf("action %s input %s is missing localized title metadata: %#v", action.ID, field, property["title_i18n"])
			}
			if _, hasEnum := property["enum"]; hasEnum {
				labels, _ := property["enum_labels_i18n"].(map[string]map[string]string)
				if len(labels[integrations.LocaleEnglishUS]) == 0 || len(labels[integrations.LocaleSimplifiedChinese]) == 0 {
					t.Errorf("action %s input %s is missing localized enum metadata: %#v", action.ID, field, property["enum_labels_i18n"])
				}
			}
		}
		if !action.DataEgress || action.ExternalDestination != "api.exa.ai" {
			t.Errorf("action %s egress contract = %v/%q", action.ID, action.DataEgress, action.ExternalDestination)
		}
	}
}

func TestProviderDefinitionHasLocalizedCatalogMetadata(t *testing.T) {
	definition := ProviderDefinition("auto")
	if definition.NameI18n[integrations.LocaleSimplifiedChinese] != "网页搜索" || definition.DescriptionI18n[integrations.LocaleSimplifiedChinese] == "" {
		t.Fatalf("provider localization = %#v / %#v", definition.NameI18n, definition.DescriptionI18n)
	}
	if definition.DocumentationURLI18n[integrations.LocaleSimplifiedChinese] == "" {
		t.Fatalf("localized documentation urls = %#v", definition.DocumentationURLI18n)
	}
	assertDeclaredLabelsLocalized(t, "tag", definition.Tags, definition.TagLabelsI18n)
	assertDeclaredLabelsLocalized(t, "category", definition.Categories, definition.CategoryLabelsI18n)
	sources := map[integrations.ConnectionCredentialSource]bool{}
	for _, auth := range definition.AuthMethods {
		if auth.Type == integrations.AuthMethodTypePlatform || auth.CredentialSource == integrations.ConnectionCredentialSourcePlatform {
			t.Fatalf("provider exposed legacy platform authentication: %#v", auth)
		}
		if auth.IdentityKind != integrations.AuthIdentityKindApplication ||
			auth.AcquisitionStrategy != integrations.AuthAcquisitionStrategyManualForm ||
			auth.LifecycleStrategy != integrations.AuthLifecycleStrategyStatic ||
			auth.RequestAuthStrategy != integrations.RequestAuthStrategyAPIKeyHeader ||
			auth.SetupGuide == nil || len(auth.SetupGuide.Steps) != 4 ||
			!strings.HasPrefix(auth.SetupGuide.ConsoleURL, "https://") ||
			!strings.HasPrefix(auth.SetupGuide.DocumentationURL, "https://") {
			t.Fatalf("provider authentication strategy = %#v", auth)
		}
		if auth.SetupGuide.Steps[0].Action != integrations.AuthSetupStepActionOpenConsole ||
			auth.SetupGuide.Steps[2].Action != integrations.AuthSetupStepActionOpenDocumentation {
			t.Fatalf("Exa setup step actions = %#v", auth.SetupGuide.Steps)
		}
		sources[auth.CredentialSource] = true
		if auth.LabelI18n[integrations.LocaleSimplifiedChinese] == "" || auth.DescriptionI18n[integrations.LocaleSimplifiedChinese] == "" {
			t.Errorf("auth method %s is missing simplified Chinese metadata", auth.ID)
		}
	}
	if !sources[integrations.ConnectionCredentialSourceOrganization] || !sources[integrations.ConnectionCredentialSourceAccount] {
		t.Fatalf("provider auth sources = %#v, want organization and account", sources)
	}
}

func assertDeclaredLabelsLocalized(t *testing.T, kind string, values []string, labels integrations.LocalizedLabelMap) {
	t.Helper()
	for _, value := range values {
		localized := labels[value]
		if localized[integrations.LocaleEnglishUS] == "" || localized[integrations.LocaleSimplifiedChinese] == "" {
			t.Errorf("%s %q is missing en-US or zh-Hans labels: %#v", kind, value, localized)
		}
	}
}

func TestSearchSchemaRejectsWhitespaceOnlyQuery(t *testing.T) {
	for _, action := range Actions() {
		if action.ID != integrations.ActionWebSearch {
			continue
		}
		if err := tools.ValidateJSONSchemaValue(action.InputSchema, map[string]interface{}{"query": "   "}); err == nil {
			t.Fatal("search schema accepted a whitespace-only query")
		}
		return
	}
	t.Fatal("search action not found")
}

func TestConfiguredDefaultSearchTypeIsUsedBySchemaAndRequest(t *testing.T) {
	actions := Actions("instant")
	searchType := actions[0].InputSchema["properties"].(map[string]interface{})["search_type"].(map[string]interface{})
	if searchType["default"] != "instant" {
		t.Fatalf("search schema default = %#v, want instant", searchType["default"])
	}

	config := testConfig("exa-default-search-type")
	config.DefaultSearchType = "instant"
	adapter := newTestAdapter(t, config, func(w http.ResponseWriter, r *http.Request) {
		var request searchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode search request: %v", err)
		}
		if request.Type != "instant" {
			t.Errorf("search request type = %q, want instant", request.Type)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"requestId":          "req-default-type",
			"resolvedSearchType": "instant",
			"results":            []interface{}{},
		})
	})
	if _, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input:    map[string]interface{}{"query": "current information"},
	}); err != nil {
		t.Fatalf("search with configured default: %v", err)
	}
}

func TestSearchDropsEmptyHighlightsAndCapsHighlightItems(t *testing.T) {
	highlights := make([]string, 0, maxHighlightCandidatesPerResult+10)
	for index := 0; index < 5; index++ {
		highlights = append(highlights, "   ")
	}
	for index := 0; index < maxHighlightCandidatesPerResult+5; index++ {
		highlights = append(highlights, "highlight")
	}
	adapter := newTestAdapter(t, testConfig("exa-highlight-cap"), func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"requestId":          "req-highlight-cap",
			"resolvedSearchType": "auto",
			"results": []map[string]interface{}{{
				"title":      "Bounded result",
				"url":        "https://example.com/bounded",
				"highlights": highlights,
			}},
		})
	})
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input:    map[string]interface{}{"query": "bounded highlights"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	item := outputResults(t, result)[0].(map[string]interface{})
	got := item["highlights"].([]interface{})
	if len(got) != maxHighlightsPerResult {
		t.Fatalf("highlight count = %d, want %d", len(got), maxHighlightsPerResult)
	}
	for _, value := range got {
		if strings.TrimSpace(value.(string)) == "" {
			t.Fatalf("normalized highlights retained an empty item: %#v", got)
		}
	}
	assertOutputMatchesActionSchema(t, integrations.ActionWebSearch, result.Output)
}

func TestNormalizedSourceURLRemovesUpstreamFragment(t *testing.T) {
	if got := sanitizeSourceURL("https://example.com/article#section"); got != "https://example.com/article" {
		t.Fatalf("sanitizeSourceURL() = %q, want fragment removed", got)
	}
}

func TestNewRejectsUnsupportedDefaultSearchType(t *testing.T) {
	config := testConfig("exa-invalid-default")
	config.DefaultSearchType = "deep"
	if _, err := New(config, nil); err == nil {
		t.Fatal("New() error = nil, want unsupported default search type failure")
	}
}

func TestSearchRequestAndStandardOutput(t *testing.T) {
	const apiKey = "exa-test-secret"
	adapter := newTestAdapter(t, testConfig(apiKey), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Errorf("request = %s %s, want POST /search", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != apiKey {
			t.Errorf("x-api-key = %q, want configured key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var request searchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode search request: %v", err)
		}
		if request.Query != "current ZGI release" || request.NumResults != 7 || request.Type != "fast" {
			t.Errorf("unexpected search request: %+v", request)
		}
		if !reflect.DeepEqual(request.IncludeDomains, []string{"example.com"}) || !reflect.DeepEqual(request.ExcludeDomains, []string{"spam.example"}) {
			t.Errorf("unexpected domain filters: include=%v exclude=%v", request.IncludeDomains, request.ExcludeDomains)
		}
		if request.StartPublishedDate != "2026-07-01T00:00:00Z" || request.EndPublishedDate != "2026-07-20T00:00:00Z" {
			t.Errorf("unexpected published date range: %+v", request)
		}
		highlights, ok := request.Contents["highlights"].(map[string]interface{})
		if !ok || highlights["query"] != "current ZGI release" || highlights["maxCharacters"] != float64(2000) {
			t.Errorf("unexpected highlights request: %#v", request.Contents)
		}

		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"requestId":          "req-search-1",
			"resolvedSearchType": "fast",
			"costDollars":        map[string]interface{}{"total": 0.007},
			"results": []map[string]interface{}{
				{
					"title":         " ZGI release notes ",
					"url":           " https://example.com/zgi ",
					"publishedDate": "2026-07-19T12:00:00Z",
					"author":        " Example Author ",
					"highlights":    []string{"Relevant release details"},
				},
			},
		})
	})

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input: map[string]interface{}{
			"query":                " current ZGI release ",
			"num_results":          7,
			"search_type":          "fast",
			"include_domains":      []interface{}{"example.com"},
			"exclude_domains":      []interface{}{"spam.example"},
			"start_published_date": "2026-07-01T00:00:00Z",
			"end_published_date":   "2026-07-20T00:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	assertOutputMatchesActionSchema(t, integrations.ActionWebSearch, result.Output)
	assertResultMetadata(t, result, "req-search-1", 0.007, 1, 1)
	if got := result.Output["schema_version"]; got != "zgi.web_search.v1" {
		t.Fatalf("schema_version = %#v", got)
	}
	if got := result.Output["provider"]; got != integrations.DriverExa {
		t.Fatalf("provider = %#v", got)
	}
	if got := result.Output["resolved_search_type"]; got != "fast" {
		t.Fatalf("resolved_search_type = %#v", got)
	}
	if got := result.Output["cost_usd"]; got != 0.007 {
		t.Fatalf("cost_usd = %#v, want number 0.007", got)
	}
	item := outputResults(t, result)[0].(map[string]interface{})
	if item["title"] != "ZGI release notes" || item["url"] != "https://example.com/zgi" || item["author"] != "Example Author" {
		t.Fatalf("standardized search item = %#v", item)
	}
}

func TestContentsRequestAndStandardOutput(t *testing.T) {
	const apiKey = "exa-contents-secret"
	adapter := newTestAdapter(t, testConfig(apiKey), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/contents" {
			t.Errorf("request = %s %s, want POST /contents", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != apiKey {
			t.Errorf("x-api-key = %q, want configured key", got)
		}
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode contents request: %v", err)
		}
		if got := interfaceStrings(request["urls"]); !reflect.DeepEqual(got, []string{"https://example.com/a", "https://example.com/b"}) {
			t.Errorf("urls = %v", got)
		}
		if request["maxAgeHours"] != float64(-1) {
			t.Errorf("maxAgeHours = %#v, want -1", request["maxAgeHours"])
		}
		textOptions, ok := request["text"].(map[string]interface{})
		if !ok || textOptions["maxCharacters"] != float64(1234) {
			t.Errorf("text options = %#v", request["text"])
		}
		if _, exists := request["highlights"]; exists {
			t.Errorf("text request unexpectedly included highlights: %#v", request)
		}

		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"requestId":   "req-contents-1",
			"costDollars": 0.012,
			"results": []map[string]interface{}{
				{
					"title":         " Example page ",
					"url":           " https://example.com/a ",
					"publishedDate": "2026-07-18T00:00:00Z",
					"author":        " Example ",
					"text":          " Page contents ",
					"highlights":    []string{"Page highlight"},
				},
			},
		})
	})

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebFetch,
		Input: map[string]interface{}{
			"urls":           []string{"https://example.com/a", "https://example.com/b"},
			"content_mode":   "text",
			"max_characters": 1234,
			"freshness":      "cache_only",
		},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	assertOutputMatchesActionSchema(t, integrations.ActionWebFetch, result.Output)
	assertResultMetadata(t, result, "req-contents-1", 0.012, 1, 1)
	if result.Output["schema_version"] != "zgi.web_fetch.v1" || result.Output["cost_usd"] != 0.012 {
		t.Fatalf("unexpected standardized fetch metadata: %#v", result.Output)
	}
	item := outputResults(t, result)[0].(map[string]interface{})
	if item["title"] != "Example page" || item["text"] != "Page contents" || item["status"] != "success" {
		t.Fatalf("standardized fetch item = %#v", item)
	}
}

func TestContentsHighlightsRequestContract(t *testing.T) {
	adapter := newTestAdapter(t, testConfig("highlights-secret"), func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode contents request: %v", err)
		}
		highlights, ok := request["highlights"].(map[string]interface{})
		if !ok || highlights["query"] != "release date" || highlights["maxCharacters"] != float64(321) {
			t.Errorf("highlights options = %#v", request["highlights"])
		}
		if _, exists := request["text"]; exists {
			t.Errorf("highlights request unexpectedly included text: %#v", request)
		}
		if _, exists := request["maxAgeHours"]; exists {
			t.Errorf("prefer_cache should omit maxAgeHours: %#v", request)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-highlight", "results": []interface{}{}})
	})

	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebFetch,
		Input: map[string]interface{}{
			"urls":            []string{"https://example.com"},
			"content_mode":    "highlights",
			"highlight_query": "release date",
			"max_characters":  321,
		},
	})
	if err != nil {
		t.Fatalf("fetch highlights: %v", err)
	}
}

func TestContentsPartialFailureIsRepresentedWithoutUpstreamErrorText(t *testing.T) {
	const upstreamError = "secret upstream diagnostic"
	adapter := newTestAdapter(t, testConfig("partial-secret"), func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"requestId": "req-partial",
			"results": []map[string]interface{}{
				{"title": "Available", "url": "https://example.com/ok", "text": "content"},
			},
			"statuses": []map[string]interface{}{
				{"id": "https://example.com/ok", "status": "success"},
				{"id": "https://example.com/failed", "status": "error", "error": map[string]interface{}{"tag": "FETCH_DOCUMENT_ERROR", "message": upstreamError, "httpStatusCode": 422}},
			},
		})
	})

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebFetch,
		Input: map[string]interface{}{
			"urls": []string{"https://example.com/ok", "https://example.com/failed"},
		},
	})
	if err != nil {
		t.Fatalf("fetch partial result: %v", err)
	}
	items := outputResults(t, result)
	if len(items) != 2 || items[0].(map[string]interface{})["status"] != "success" || items[1].(map[string]interface{})["status"] != "failed" {
		t.Fatalf("partial fetch output = %#v", items)
	}
	encoded, _ := json.Marshal(result.Output)
	if strings.Contains(string(encoded), upstreamError) {
		t.Fatalf("partial fetch output leaked upstream diagnostic: %s", encoded)
	}
	assertOutputMatchesActionSchema(t, integrations.ActionWebFetch, result.Output)
}

func TestAdapterCapsAndTruncatesUpstreamResults(t *testing.T) {
	t.Run("search respects requested result count", func(t *testing.T) {
		adapter := newTestAdapter(t, testConfig("search-request-secret"), func(w http.ResponseWriter, r *http.Request) {
			results := make([]map[string]interface{}, 8)
			for index := range results {
				results[index] = map[string]interface{}{"title": "result", "url": "https://example.com"}
			}
			writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-search-requested", "results": results})
		})
		result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: integrations.ActionWebSearch,
			Input:    map[string]interface{}{"query": "requested", "num_results": 2},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if got := len(outputResults(t, result)); got != 2 {
			t.Fatalf("search result count = %d, want requested count 2", got)
		}
	})

	t.Run("fetch respects requested URL count", func(t *testing.T) {
		adapter := newTestAdapter(t, testConfig("fetch-request-secret"), func(w http.ResponseWriter, r *http.Request) {
			results := make([]map[string]interface{}, 5)
			for index := range results {
				results[index] = map[string]interface{}{"title": "result", "url": "https://example.com"}
			}
			writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-fetch-requested", "results": results})
		})
		result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: integrations.ActionWebFetch,
			Input:    map[string]interface{}{"urls": []string{"https://example.com/1", "https://example.com/2"}},
		})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if got := len(outputResults(t, result)); got != 2 {
			t.Fatalf("fetch result count = %d, want requested count 2", got)
		}
	})

	t.Run("search count and highlight budget", func(t *testing.T) {
		adapter := newTestAdapter(t, Config{
			APIKey:               "search-cap-secret",
			Timeout:              2 * time.Second,
			MaxResults:           99,
			MaxFetchURLs:         5,
			MaxContentCharacters: 20000,
		}, func(w http.ResponseWriter, r *http.Request) {
			var request searchRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode search request: %v", err)
			}
			if request.NumResults != 10 {
				t.Errorf("numResults = %d, want platform cap 10", request.NumResults)
			}
			results := make([]map[string]interface{}, 12)
			for index := range results {
				results[index] = map[string]interface{}{
					"title":      "result",
					"url":        "https://example.com",
					"highlights": []string{strings.Repeat("界", 3000)},
				}
			}
			writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-search-cap", "results": results})
		})

		result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: integrations.ActionWebSearch,
			Input:    map[string]interface{}{"query": "cap", "num_results": 100},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		items := outputResults(t, result)
		if len(items) != 10 || result.ResultCount != 10 {
			t.Fatalf("search result count = %d/%d, want 10", len(items), result.ResultCount)
		}
		totalHighlights := 0
		for _, rawItem := range items {
			item := rawItem.(map[string]interface{})
			for _, rawHighlight := range item["highlights"].([]interface{}) {
				length := len([]rune(rawHighlight.(string)))
				if length > 2000 {
					t.Fatalf("individual highlight length = %d, want <= 2000", length)
				}
				totalHighlights += length
			}
		}
		if totalHighlights != 12000 {
			t.Fatalf("total highlight length = %d, want 12000", totalHighlights)
		}
	})

	t.Run("fetch URL and response count", func(t *testing.T) {
		adapter := newTestAdapter(t, Config{
			APIKey:               "fetch-cap-secret",
			Timeout:              2 * time.Second,
			MaxResults:           10,
			MaxFetchURLs:         99,
			MaxContentCharacters: 20000,
		}, func(w http.ResponseWriter, r *http.Request) {
			var request contentsRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode contents request: %v", err)
			}
			if len(request.URLs) != 5 {
				t.Errorf("outbound URL count = %d, want platform cap 5", len(request.URLs))
			}
			results := make([]map[string]interface{}, 7)
			for index := range results {
				results[index] = map[string]interface{}{"title": "result", "url": "https://example.com", "text": ""}
			}
			writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-fetch-cap", "results": results})
		})
		urls := make([]string, 7)
		for index := range urls {
			urls[index] = "https://example.com/page"
		}
		result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: integrations.ActionWebFetch,
			Input:    map[string]interface{}{"urls": urls},
		})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if got := len(outputResults(t, result)); got != 5 || result.ResultCount != 5 {
			t.Fatalf("fetch result count = %d/%d, want 5", got, result.ResultCount)
		}
	})

	t.Run("fetch per-page and total text budget", func(t *testing.T) {
		adapter := newTestAdapter(t, Config{
			APIKey:               "fetch-text-secret",
			Timeout:              2 * time.Second,
			MaxResults:           10,
			MaxFetchURLs:         5,
			MaxContentCharacters: 99999,
		}, func(w http.ResponseWriter, r *http.Request) {
			var request map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode contents request: %v", err)
			}
			textOptions := request["text"].(map[string]interface{})
			if textOptions["maxCharacters"] != float64(20000) {
				t.Errorf("maxCharacters = %#v, want platform cap 20000", textOptions["maxCharacters"])
			}
			results := make([]map[string]interface{}, 5)
			for index := range results {
				results[index] = map[string]interface{}{
					"title": "result",
					"url":   "https://example.com",
					"text":  strings.Repeat("文", 25000),
				}
			}
			writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-fetch-text", "results": results})
		})
		result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: integrations.ActionWebFetch,
			Input: map[string]interface{}{
				"urls":           []string{"https://example.com/1", "https://example.com/2", "https://example.com/3"},
				"max_characters": 99999,
			},
		})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		items := outputResults(t, result)
		if len(items) != 2 {
			t.Fatalf("bounded fetch result count = %d, want 2", len(items))
		}
		totalText := 0
		for _, rawItem := range items {
			length := len([]rune(rawItem.(map[string]interface{})["text"].(string)))
			if length != 20000 {
				t.Fatalf("page text length = %d, want 20000", length)
			}
			totalText += length
		}
		if totalText != 40000 {
			t.Fatalf("total text length = %d, want 40000", totalText)
		}
	})
}

func TestAdapterMapsAuthenticationAndBudgetStatusesWithoutLeakingAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantCode string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantCode: integrations.ErrorCodeAuthInvalid},
		{name: "payment required", status: http.StatusPaymentRequired, wantCode: integrations.ErrorCodeBudgetExceeded},
		{name: "forbidden", status: http.StatusForbidden, wantCode: integrations.ErrorCodeAccessDenied},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const apiKey = "exa-never-leak-this-secret"
			adapter := newTestAdapter(t, testConfig(apiKey), func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, test.status, map[string]interface{}{
					"requestId": "req-status",
					"error":     "upstream echoed " + apiKey,
				})
			})
			result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
				ActionID: integrations.ActionWebSearch,
				Input:    map[string]interface{}{"query": "status mapping"},
			})
			if err == nil {
				t.Fatal("expected status error")
			}
			if got := integrations.ErrorCode(err); got != test.wantCode {
				t.Fatalf("error code = %q, want %q (error %v)", got, test.wantCode, err)
			}
			if strings.Contains(err.Error(), apiKey) {
				t.Fatalf("returned error leaked API key: %v", err)
			}
			if result == nil || result.ProviderRequestID != "req-status" || result.AttemptCount != 1 {
				t.Fatalf("error result metadata = %+v", result)
			}
		})
	}
}

func TestAdapterDoesNotFollowRedirectsWithAPIKeyOrQuery(t *testing.T) {
	var redirectedCalls atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	redirectSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirectSource.Close()
	adapter, err := newWithBaseURL(testConfig("redirect-secret"), redirectSource.Client(), redirectSource.URL)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}

	_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input:    map[string]interface{}{"query": "private query"},
	})
	if got := integrations.ErrorCode(err); got != integrations.ErrorCodeUpstream {
		t.Fatalf("ErrorCode() = %q, want %q (err=%v)", got, integrations.ErrorCodeUpstream, err)
	}
	if redirectedCalls.Load() != 0 {
		t.Fatalf("redirect target received %d requests", redirectedCalls.Load())
	}
}

func TestAdapterRetries429And502UsingRetryAfter(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			adapter := newTestAdapter(t, testConfig("retry-secret"), func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("Retry-After", "0")
					writeJSON(t, w, status, map[string]interface{}{"requestId": "req-retry-first", "error": "retry"})
					return
				}
				writeJSON(t, w, http.StatusOK, map[string]interface{}{"requestId": "req-retry-success", "results": []interface{}{}})
			})
			result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
				ActionID: integrations.ActionWebSearch,
				Input:    map[string]interface{}{"query": "retry"},
			})
			if err != nil {
				t.Fatalf("retry request: %v", err)
			}
			if calls.Load() != 2 || result.AttemptCount != 2 || result.ProviderRequestID != "req-retry-success" {
				t.Fatalf("retry metadata calls=%d result=%+v", calls.Load(), result)
			}
		})
	}
	if got := retryDelay(1, "0"); got != 0 {
		t.Fatalf("Retry-After 0 delay = %v, want 0", got)
	}
	if got := retryDelay(1, "99"); got != 99*time.Second {
		t.Fatalf("Retry-After delay = %v, want 99s", got)
	}
}

func TestAdapterMapsExhaustedRetryStatuses(t *testing.T) {
	tests := []struct {
		status   int
		wantCode string
	}{
		{status: http.StatusTooManyRequests, wantCode: integrations.ErrorCodeRateLimited},
		{status: http.StatusBadGateway, wantCode: integrations.ErrorCodeUpstream},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			var calls atomic.Int32
			adapter := newTestAdapter(t, testConfig("exhausted-secret"), func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				w.Header().Set("Retry-After", "0")
				writeJSON(t, w, test.status, map[string]interface{}{"requestId": "req-attempt-" + string(rune('0'+call)), "error": "retry"})
			})
			result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
				ActionID: integrations.ActionWebSearch,
				Input:    map[string]interface{}{"query": "retry exhaustion"},
			})
			if err == nil || integrations.ErrorCode(err) != test.wantCode {
				t.Fatalf("error = %v, code = %q, want %q", err, integrations.ErrorCode(err), test.wantCode)
			}
			if calls.Load() != 3 || result == nil || result.AttemptCount != 3 || result.ProviderRequestID != "req-attempt-3" {
				t.Fatalf("exhausted retry metadata calls=%d result=%+v", calls.Load(), result)
			}
		})
	}
}

func TestAdapterRetryAfterCannotExceedTotalTimeout(t *testing.T) {
	config := testConfig("timeout-secret")
	config.Timeout = 25 * time.Millisecond
	adapter := newTestAdapter(t, config, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "99")
		writeJSON(t, w, http.StatusTooManyRequests, map[string]interface{}{"error": "retry later"})
	})
	started := time.Now()
	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input:    map[string]interface{}{"query": "timeout"},
	})
	if integrations.ErrorCode(err) != integrations.ErrorCodeTimeout {
		t.Fatalf("ErrorCode() = %q, want %q (err=%v)", integrations.ErrorCode(err), integrations.ErrorCodeTimeout, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("retry exceeded total timeout: %v", elapsed)
	}
}

func TestAdapterMapsSlowResponseBodyToTimeout(t *testing.T) {
	config := testConfig("slow-body-secret")
	config.Timeout = 30 * time.Millisecond
	adapter := newTestAdapter(t, config, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"requestId":"late"}`))
	})

	_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID: integrations.ActionWebSearch,
		Input:    map[string]interface{}{"query": "slow body"},
	})
	if got := integrations.ErrorCode(err); got != integrations.ErrorCodeTimeout {
		t.Fatalf("ErrorCode() = %q, want %q (err=%v)", got, integrations.ErrorCodeTimeout, err)
	}
}

func TestSanitizeSourceURL(t *testing.T) {
	if got := sanitizeSourceURL("https://example.com/article#section"); got != "https://example.com/article" {
		t.Fatalf("sanitizeSourceURL() = %q", got)
	}
	for _, raw := range []string{
		"javascript:alert(1)",
		"http://127.0.0.1/private",
		"https://user:password@example.com/",
		"https://example.com/?access_token=secret",
		"https://example.com/?X-Amz-Credential=credential&X-Amz-Signature=signature",
	} {
		if got := sanitizeSourceURL(raw); got != "" {
			t.Fatalf("sanitizeSourceURL(%q) = %q, want empty", raw, got)
		}
	}
}

func TestAdapterRejectsMalformedAndOversizedResponsesWithoutLeakingAPIKey(t *testing.T) {
	tests := []struct {
		name string
		body func(apiKey string) string
	}{
		{
			name: "malformed JSON",
			body: func(apiKey string) string { return `{"requestId":"req", "echo":"` + apiKey + `"` },
		},
		{
			name: "oversized response",
			body: func(apiKey string) string { return apiKey + strings.Repeat("x", maxResponseSize+1) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const apiKey = "exa-malformed-secret"
			adapter := newTestAdapter(t, testConfig(apiKey), func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(test.body(apiKey)))
			})
			result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
				ActionID: integrations.ActionWebSearch,
				Input:    map[string]interface{}{"query": "invalid response"},
			})
			if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
				t.Fatalf("error = %v, code = %q", err, integrations.ErrorCode(err))
			}
			if strings.Contains(err.Error(), apiKey) {
				t.Fatalf("returned error leaked API key: %v", err)
			}
			if result == nil || result.AttemptCount != 1 {
				t.Fatalf("invalid response metadata = %+v", result)
			}
		})
	}
}

func newTestAdapter(t *testing.T, config Config, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	adapter, err := newWithBaseURL(config, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("create Exa adapter: %v", err)
	}
	return adapter
}

func testConfig(apiKey string) Config {
	return Config{
		APIKey:               apiKey,
		Timeout:              2 * time.Second,
		MaxResults:           10,
		MaxFetchURLs:         5,
		MaxContentCharacters: 20000,
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode test response: %v", err)
	}
}

func assertResultMetadata(t *testing.T, result *integrations.ActionResult, requestID string, cost float64, count, attempts int) {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.ProviderRequestID != requestID || result.Output["request_id"] != requestID {
		t.Fatalf("request ID metadata = %+v output=%#v", result, result.Output)
	}
	if result.CostUSD == nil || math.Abs(*result.CostUSD-cost) > 0.000000001 {
		t.Fatalf("cost = %v, want %v", result.CostUSD, cost)
	}
	if result.ResultCount != count || result.AttemptCount != attempts {
		t.Fatalf("counts = results %d attempts %d, want %d/%d", result.ResultCount, result.AttemptCount, count, attempts)
	}
}

func assertOutputMatchesActionSchema(t *testing.T, actionID string, output map[string]interface{}) {
	t.Helper()
	for _, action := range Actions() {
		if action.ID == actionID {
			if err := tools.ValidateJSONSchemaValue(action.OutputSchema, output); err != nil {
				t.Fatalf("action %s output schema validation: %v\noutput=%#v", actionID, err, output)
			}
			return
		}
	}
	t.Fatalf("action schema %s not found", actionID)
}

func outputResults(t *testing.T, result *integrations.ActionResult) []interface{} {
	t.Helper()
	results, ok := result.Output["results"].([]interface{})
	if !ok {
		t.Fatalf("output results type = %T (%#v)", result.Output["results"], result.Output["results"])
	}
	return results
}

func interfaceStrings(value interface{}) []string {
	raw, _ := value.([]interface{})
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			values = append(values, text)
		}
	}
	return values
}
