package toolusage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Identity must come from authenticated runtime metadata and the selected route.
type Identity struct {
	OrganizationID                                  uuid.UUID
	RequestID, AttemptID, Lane, ModelName, Provider string
	UpstreamScope                                   string
	ChannelID                                       uuid.UUID
	ModelID                                         *uuid.UUID
}
type callFact struct{ ID, Tool, Status, SessionID, ContainerSpec string }

// Observer accumulates trusted upstream facts only. It never stores prompt/result content.
type Observer struct {
	identity          Identity
	requested         map[string]bool
	providerRequestID string
	calls             map[string]callFact
	counts            map[string]int64
	seen              map[string]bool
	geminiQueries     map[string]bool
}

func NewObserver(identity Identity, request []byte) *Observer {
	o := &Observer{identity: identity, requested: map[string]bool{}, calls: map[string]callFact{}, counts: map[string]int64{}, seen: map[string]bool{}, geminiQueries: map[string]bool{}}
	var body struct{ Tools []map[string]json.RawMessage }
	_ = json.Unmarshal(request, &body)
	for _, t := range body.Tools {
		var name string
		_ = json.Unmarshal(t["type"], &name)
		if name == "" {
			_ = json.Unmarshal(t["name"], &name)
		}
		for key := range t {
			switch key {
			case "googleSearch", "google_search":
				name = ToolWebSearch
			case "fileSearch", "file_search":
				name = ToolFileSearch
			case "codeExecution", "code_execution":
				name = ToolCodeInterpreter
			}
		}
		if strings.HasPrefix(name, "web_search") {
			name = ToolWebSearch
		}
		if strings.HasPrefix(name, "code_execution") {
			name = ToolCodeInterpreter
		}
		if name == ToolWebSearch || name == ToolFileSearch || name == ToolCodeInterpreter {
			o.requested[name] = true
		}
	}
	return o
}
func (o *Observer) Observe(data []byte) {
	var payload struct {
		ID, Type  string
		Message   json.RawMessage
		Response  json.RawMessage
		Item      json.RawMessage
		Output    []json.RawMessage
		ToolUsage map[string]struct {
			NumRequests *int64 `json:"num_requests"`
		} `json:"tool_usage"`
		Usage struct {
			ServerToolUse struct {
				WebSearchRequests *int64 `json:"web_search_requests"`
			} `json:"server_tool_use"`
		}
		ResponseID string `json:"responseId"`
		Candidates []struct {
			GroundingMetadata struct {
				WebSearchQueries []string `json:"webSearchQueries"`
			} `json:"groundingMetadata"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return
	}
	if len(payload.Message) > 0 {
		o.Observe(payload.Message)
	}
	if len(payload.Response) > 0 {
		o.Observe(payload.Response)
		return
	}
	if payload.ID != "" && (payload.Type == "response" || payload.Type == "message" || len(payload.Output) > 0 || payload.ToolUsage != nil) {
		o.providerRequestID = payload.ID
	}
	if payload.ResponseID != "" {
		o.providerRequestID = payload.ResponseID
	}
	for candidateIndex, candidate := range payload.Candidates {
		for queryIndex, query := range candidate.GroundingMetadata.WebSearchQueries {
			query = strings.TrimSpace(query)
			if query != "" {
				key := fmt.Sprintf("%s:%d:%d:%s", o.providerRequestID, candidateIndex, queryIndex, query)
				o.geminiQueries[key] = true
			}
		}
	}
	if len(o.geminiQueries) > 0 {
		o.counts[ToolWebSearch] = int64(len(o.geminiQueries))
		o.seen[ToolWebSearch] = true
	}
	for tool, v := range payload.ToolUsage {
		if (tool == ToolWebSearch || tool == ToolFileSearch) && v.NumRequests != nil && *v.NumRequests >= 0 {
			o.counts[tool] = *v.NumRequests
			o.seen[tool] = true
		}
	}
	if n := payload.Usage.ServerToolUse.WebSearchRequests; n != nil && *n >= 0 {
		o.counts[ToolWebSearch] = *n
		o.seen[ToolWebSearch] = true
	}
	if len(payload.Item) > 0 {
		o.observeCall(payload.Item)
	}
	for _, item := range payload.Output {
		o.observeCall(item)
	}
}
func (o *Observer) observeCall(data []byte) {
	var v struct {
		ID, Type, Status string
		ContainerID      string `json:"container_id"`
		ContainerSpec    string `json:"container_spec"`
	}
	if json.Unmarshal(data, &v) != nil || v.ID == "" {
		return
	}
	tool := ""
	switch v.Type {
	case "web_search_call":
		tool = ToolWebSearch
	case "file_search_call":
		tool = ToolFileSearch
	case "code_interpreter_call":
		tool = ToolCodeInterpreter
	default:
		return
	}
	status := StatusPending
	switch v.Status {
	case "completed":
		status = StatusSucceeded
	case "failed", "incomplete":
		status = StatusFailed
	}
	previous, ok := o.calls[v.ID]
	if ok && (previous.Status == StatusSucceeded || previous.Status == StatusFailed) {
		return
	}
	o.calls[v.ID] = callFact{ID: v.ID, Tool: tool, Status: status, SessionID: v.ContainerID, ContainerSpec: v.ContainerSpec}
}

// Events emits one summary per search tool, so counters and individual calls are never added twice.
// Code interpreter facts are distinct by successful provider container/session identity.
func (o *Observer) Events(ended bool, failed bool) ([]Event, error) {
	all := map[string]bool{}
	for t := range o.requested {
		all[t] = true
	}
	for t := range o.seen {
		all[t] = true
	}
	for _, f := range o.calls {
		all[f.Tool] = true
	}
	tools := []string{}
	for t := range all {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	result := []Event{}
	base := func(tool, key, callID, session string) Event {
		return Event{SchemaVersion: 1, OrganizationID: o.identity.OrganizationID, RequestID: o.identity.RequestID, AttemptID: o.identity.AttemptID, EventID: key, BillingLane: o.identity.Lane, Tool: tool, ModelID: o.identity.ModelID, ModelName: o.identity.ModelName, ProviderName: o.identity.Provider, ChannelID: &o.identity.ChannelID, UpstreamScope: o.identity.UpstreamScope, ProviderRequestID: o.providerRequestID, ProviderCallID: callID, SessionID: session, DedupeKey: key, Unit: "call", Status: StatusPending, UsageSource: UsageSourceMissing}
	}
	for _, tool := range tools {
		if tool == ToolCodeInterpreter {
			emitted := false
			ids := []string{}
			for id := range o.calls {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				f := o.calls[id]
				if f.Tool != tool || f.SessionID == "" || f.Status != StatusSucceeded {
					continue
				}
				key, err := DedupeKey(o.identity.Provider, o.identity.ChannelID.String(), o.identity.UpstreamScope, tool, o.providerRequestID, f.ID, f.SessionID)
				if err != nil {
					return nil, err
				}
				e := base(tool, key, f.ID, f.SessionID)
				e.Unit = "session"
				e.Quantity = 1
				e.Status = StatusSucceeded
				e.UsageSource = UsageSourceGatewayCounted
				e.ContainerSpec = f.ContainerSpec
				_ = e.SetDetails(map[string]any{"mapping": "responses_completed_container_v1"})
				result = append(result, e)
				emitted = true
			}
			if emitted {
				continue
			}
		}
		key, err := DedupeKey(o.identity.Provider, o.identity.ChannelID.String(), o.identity.UpstreamScope, ToolWebSearch, "attempt:"+o.identity.AttemptID, tool, "")
		if err != nil {
			return nil, err
		}
		if o.providerRequestID != "" && tool != ToolCodeInterpreter {
			key, err = DedupeKey(o.identity.Provider, o.identity.ChannelID.String(), o.identity.UpstreamScope, tool, o.providerRequestID, "summary", "")
			if err != nil {
				return nil, err
			}
		}
		e := base(tool, key, "", "")
		if tool == ToolCodeInterpreter {
			e.Unit = "session"
		}
		count := int64(0)
		callIDs := []string{}
		known := o.seen[tool]
		allFinal := true
		anyFailed := false
		for _, f := range o.calls {
			if f.Tool != tool {
				continue
			}
			known = true
			callIDs = append(callIDs, f.ID)
			if f.Status == StatusSucceeded {
				count++
			}
			if f.Status == StatusPending {
				allFinal = false
			}
			if f.Status == StatusFailed {
				anyFailed = true
			}
		}
		sort.Strings(callIDs)
		if reported, ok := o.counts[tool]; ok {
			count = reported
			allFinal = true
			e.UsageSource = UsageSourceProviderReported
		} else if known {
			e.UsageSource = UsageSourceGatewayCounted
		}
		if tool == ToolCodeInterpreter {
			if anyFailed && allFinal {
				e.Status = StatusFailed
				e.UsageSource = UsageSourceGatewayCounted
			} else {
				e.UsageSource = UsageSourceMissing
			}
		}
		if tool != ToolCodeInterpreter && o.providerRequestID != "" && known && (allFinal || (ended && count > 0)) {
			e.Quantity = count
			e.Status = StatusSucceeded
			if count == 0 && anyFailed {
				e.Status = StatusFailed
			}
		}
		if e.Status == StatusPending && failed {
			e.Status = StatusFailed
			e.UsageSource = UsageSourceMissing
		}
		if e.Status == StatusPending && ended {
			e.Status = StatusFailed
			e.UsageSource = UsageSourceMissing
		}
		reason := ""
		if e.UsageSource == UsageSourceMissing {
			reason = "missing_usage"

			if failed {
				reason = "upstream_failed"
			}
		}
		_ = e.SetDetails(map[string]any{"mapping": "upstream_tools_v1", "call_ids": callIDs, "free_reason": reason})
		result = append(result, e)
	}
	return result, nil
}
func (o *Observer) Record(ctx context.Context, r *Repository, ended, failed bool) error {
	events, err := o.Events(ended, failed)
	if err != nil {
		return err
	}
	for i := range events {
		if _, err := r.Record(ctx, &events[i]); err != nil {
			return fmt.Errorf("record tool fact %s: %w", events[i].Tool, err)
		}
	}
	return nil
}
