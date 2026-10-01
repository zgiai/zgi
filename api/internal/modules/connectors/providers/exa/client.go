// Package exa implements the Connector's search and webpage content protocol.
package exa

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type Client struct {
	http    *providers.HTTP
	schemas *providers.Schemas
}

func New(client *http.Client) (*Client, error) {
	schemas, err := providers.NewSchemas("web-search")
	if err != nil {
		return nil, err
	}
	return &Client{http: providers.NewHTTP(client, "https://api.exa.ai"), schemas: schemas}, nil
}
func (*Client) ConnectorID() string { return "web-search" }
func (client *Client) Verify(ctx context.Context, key string) (providers.Verification, error) {
	result, err := client.Invoke(ctx, "web.search", key, map[string]any{"query": "ZGI connection test", "num_results": 1, "search_type": "instant"})
	if err != nil {
		return providers.Verification{}, err
	}
	return providers.Verification{DisplayName: "Exa", Operations: []string{"web.search", "web.fetch"}, RequestID: result.RequestID, CostUSD: result.CostUSD}, nil
}

type response struct {
	RequestID  string          `json:"requestId"`
	SearchType string          `json:"resolvedSearchType"`
	Cost       json.RawMessage `json:"costDollars"`
	Results    []struct {
		URL, Title, Author, Text string
		Published                string `json:"publishedDate"`
		Highlights               []string
	} `json:"results"`
	Statuses []struct {
		ID, Status string
		Error      json.RawMessage `json:"error"`
	} `json:"statuses"`
}

func (client *Client) Invoke(ctx context.Context, operation, key string, input map[string]any) (providers.Response, error) {
	args, err := client.schemas.Input(operation, input)
	if err != nil || strings.TrimSpace(key) == "" {
		return providers.Response{}, providers.Invalid()
	}
	request := map[string]any{}
	path := "/search"
	limit, budget := providers.Number(args, "num_results"), 12000
	urls := []string{}
	if operation == "web.search" {
		request["query"] = providers.Text(args, "query")
		request["numResults"] = limit
		request["type"] = providers.Text(args, "search_type")
		request["contents"] = map[string]any{"highlights": map[string]any{"query": args["query"], "maxCharacters": 2000}}
		for source, destination := range map[string]string{"include_domains": "includeDomains", "exclude_domains": "excludeDomains", "start_published_date": "startPublishedDate", "end_published_date": "endPublishedDate"} {
			if value, ok := args[source]; ok {
				request[destination] = value
			}
		}
		var start, end time.Time
		if text := providers.Text(args, "start_published_date"); text != "" {
			start, err = time.Parse(time.RFC3339, text)
			if err != nil {
				return providers.Response{}, providers.Invalid()
			}
		}
		if text := providers.Text(args, "end_published_date"); text != "" {
			end, err = time.Parse(time.RFC3339, text)
			if err != nil {
				return providers.Response{}, providers.Invalid()
			}
		}
		if !start.IsZero() && !end.IsZero() && start.After(end) {
			return providers.Response{}, providers.Invalid()
		}
	} else {
		path = "/contents"
		urls = providers.TextList(args, "urls")
		limit = len(urls)
		budget = 40000
		for _, address := range urls {
			if !safeAddress(address) {
				return providers.Response{}, providers.Invalid()
			}
		}
		request["urls"] = urls
		maximum := providers.Number(args, "max_characters")
		if providers.Text(args, "content_mode") == "highlights" {
			request["highlights"] = map[string]any{"query": providers.Text(args, "highlight_query"), "maxCharacters": maximum}
		} else {
			request["text"] = map[string]any{"maxCharacters": maximum}
		}
		switch providers.Text(args, "freshness") {
		case "force_live":
			request["maxAgeHours"] = 0
		case "cache_only":
			request["maxAgeHours"] = -1
		}
	}
	var upstream response
	header, err := client.http.JSON(ctx, http.MethodPost, path, nil, http.Header{"X-Api-Key": []string{key}}, request, &upstream)
	if err != nil {
		return providers.Response{}, err
	}
	requestID := providers.SafeRequestID(upstream.RequestID, key)
	if requestID == "" {
		requestID = providers.SafeRequestID(header.Get("X-Request-Id"), key)
	}
	if requestID == "" {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	}
	results := []any{}
	seen := map[string]bool{}
	for _, row := range upstream.Results {
		if len(results) >= limit {
			break
		}
		address := providers.PublicURL(row.URL)
		if address == "" || !safeAddress(address) || seen[address] {
			continue
		}
		if operation == "web.fetch" && !requested(urls, address) {
			continue
		}
		seen[address] = true
		item := map[string]any{"url": address, "title": providers.SafeText(row.Title, 500), "published_at": providers.SafeText(row.Published, 64), "author": providers.SafeText(row.Author, 300)}
		if operation == "web.fetch" {
			text := providers.SafeText(row.Text, min(providers.Number(args, "max_characters"), max(0, budget)))
			item["text"] = text
			budget -= len([]rune(text))
			item["status"] = "success"
		}
		highlights := []any{}
		for _, raw := range row.Highlights[:min(50, len(row.Highlights))] {
			if len(highlights) >= 10 || budget <= 0 {
				break
			}
			text := providers.SafeText(raw, min(2000, budget))
			if text != "" {
				highlights = append(highlights, text)
				budget -= len([]rune(text))
			}
		}
		item["highlights"] = highlights
		results = append(results, item)
	}
	if operation == "web.fetch" {
		for _, status := range upstream.Statuses {
			address := providers.PublicURL(status.ID)
			if len(results) >= limit {
				break
			}
			if status.Status == "success" || seen[address] || !requested(urls, address) {
				continue
			}
			seen[address] = true
			item := map[string]any{"url": address, "title": "", "published_at": "", "author": "", "text": "", "highlights": []any{}, "status": "failed"}
			// Only the structured bounded tag is public; upstream error messages are
			// never included in diagnostics or user-facing errors.
			var upstreamError struct {
				Tag string `json:"tag"`
			}
			_ = json.Unmarshal(status.Error, &upstreamError)
			if tag := providers.SafeRequestID(upstreamError.Tag, key); tag != "" {
				item["error_code"] = tag
			}
			results = append(results, item)
		}
	}
	var cost *float64
	if len(upstream.Cost) > 0 {
		var total float64
		if json.Unmarshal(upstream.Cost, &total) == nil && total >= 0 {
			cost = &total
		} else {
			var object struct {
				Total *float64 `json:"total"`
			}
			if json.Unmarshal(upstream.Cost, &object) == nil && object.Total != nil && *object.Total >= 0 {
				cost = object.Total
			}
		}
	}
	var publicCost any
	if cost != nil {
		publicCost = *cost
	}
	version := "zgi.web_fetch.v1"
	if operation == "web.search" {
		version = "zgi.web_search.v1"
	}
	output := map[string]any{"schema_version": version, "provider": "exa", "request_id": requestID, "cost_usd": publicCost, "results": results}
	if operation == "web.search" {
		output["resolved_search_type"] = providers.SafeText(upstream.SearchType, 32)
	}
	providers.Redact(output, key)
	if client.schemas.Output(operation, output) != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	}
	return providers.Response{Value: output, RequestID: requestID, CostUSD: cost}, nil
}

func requested(urls []string, address string) bool {
	for _, candidate := range urls {
		if providers.PublicURL(candidate) == address {
			return true
		}
	}
	return false
}
func safeAddress(raw string) bool {
	if providers.PublicURL(raw) == "" {
		return false
	}
	address, _ := url.Parse(raw)
	host := strings.ToLower(address.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
	}
	if !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return false
	}
	return true
}
