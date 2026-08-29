package exa

import "encoding/json"

type searchRequest struct {
	Query              string                 `json:"query"`
	NumResults         int                    `json:"numResults"`
	Type               string                 `json:"type"`
	IncludeDomains     []string               `json:"includeDomains,omitempty"`
	ExcludeDomains     []string               `json:"excludeDomains,omitempty"`
	StartPublishedDate string                 `json:"startPublishedDate,omitempty"`
	EndPublishedDate   string                 `json:"endPublishedDate,omitempty"`
	Contents           map[string]interface{} `json:"contents,omitempty"`
}

type contentsRequest struct {
	URLs        []string               `json:"urls"`
	Text        interface{}            `json:"text,omitempty"`
	Highlights  map[string]interface{} `json:"highlights,omitempty"`
	MaxAgeHours *int                   `json:"maxAgeHours,omitempty"`
}

type response struct {
	RequestID          string          `json:"requestId"`
	ResolvedSearchType string          `json:"resolvedSearchType"`
	CostDollars        json.RawMessage `json:"costDollars"`
	Results            []result        `json:"results"`
	Statuses           []status        `json:"statuses"`
}

type result struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	PublishedDate string   `json:"publishedDate"`
	Author        string   `json:"author"`
	Text          string   `json:"text"`
	Highlights    []string `json:"highlights"`
}

type status struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Error  json.RawMessage `json:"error"`
}

type errorResponse struct {
	RequestID string          `json:"requestId"`
	Error     json.RawMessage `json:"error"`
	Tag       string          `json:"tag"`
}
