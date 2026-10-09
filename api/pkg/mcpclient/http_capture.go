package mcpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// prepareRequest rewrites only the two protocol fields whose presence cannot
// be represented by the SDK's string/typed parameters: empty cursors and raw
// multi-round-trip responses. It does not change tool arguments or schemas.
func (t *httpBoundary) prepareRequest(req *http.Request) (string, error) {
	if req.Method != http.MethodPost || req.GetBody == nil {
		return "", nil
	}
	reader, err := req.GetBody()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, t.limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > t.limit {
		return "", &MCPError{Kind: ErrLimitExceeded, Operation: "write HTTP body", Cause: errors.New("MCP request exceeds configured size")}
	}
	if len(body) == 0 {
		return "", nil
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return "", nil
	}
	var method string
	_ = json.Unmarshal(envelope["method"], &method)
	var params map[string]json.RawMessage
	_ = json.Unmarshal(envelope["params"], &params)
	if params == nil {
		params = make(map[string]json.RawMessage)
	}
	changed := false
	if cursor, ok := req.Context().Value(cursorKey{}).(*string); ok && cursor != nil {
		encoded, _ := json.Marshal(*cursor)
		params["cursor"] = encoded
		changed = true
	}
	if continuation, ok := req.Context().Value(continuationKey{}).(*Continuation); ok && continuation != nil {
		if continuation.RequestState.Present {
			params["requestState"] = cloneRaw(continuation.RequestState.JSON)
		}
		if continuation.InputResponses != nil {
			encoded, _ := json.Marshal(continuation.InputResponses)
			params["inputResponses"] = encoded
		}
		changed = true
	}
	if changed {
		encoded, _ := json.Marshal(params)
		envelope["params"] = encoded
		body, _ = json.Marshal(envelope)
		if int64(len(body)) > t.limit {
			return method, &MCPError{Kind: ErrLimitExceeded, Operation: "write HTTP body"}
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	return method, nil
}

func (t *httpBoundary) captureJSON(method string, body []byte, slot *captureSlot) []byte {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		invalid := &MCPError{Kind: ErrProtocol, Operation: method, Cause: errors.New("invalid JSON-RPC response")}
		if slot != nil {
			slot.setBoundaryError(invalid)
		} else {
			t.setProtocolError(invalid)
		}
		return body
	}
	if raw, ok := envelope["result"]; ok {
		if slot != nil {
			slot.mu.Lock()
			slot.raw = cloneRaw(raw)
			slot.mu.Unlock()
		}
		if method == "server/discover" || method == "initialize" {
			t.mu.Lock()
			t.discovery = cloneRaw(raw)
			t.mu.Unlock()
		}
		if cacheableMethod(method) {
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) == nil {
				if _, present := fields["ttlMs"]; present {
					delete(fields, "ttlMs")
					envelope["result"], _ = json.Marshal(fields)
					if updated, err := json.Marshal(envelope); err == nil {
						return updated
					}
				}
			}
		}
	}
	return body
}
func cacheableMethod(method string) bool {
	switch method {
	case "tools/list", "resources/list", "resources/templates/list", "prompts/list", "resources/read":
		return true
	}
	return false
}

func (t *httpBoundary) captureEvent(method string, event []byte, slot *captureSlot) {
	lines := strings.Split(strings.ReplaceAll(string(event), "\r\n", "\n"), "\n")
	var data []string
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if len(data) == 0 {
		return
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal([]byte(strings.Join(data, "\n")), &envelope) != nil {
		invalid := &MCPError{Kind: ErrProtocol, Operation: method, Cause: errors.New("invalid JSON-RPC SSE event")}
		if slot != nil {
			slot.setBoundaryError(invalid)
		} else {
			t.setProtocolError(invalid)
		}
		return
	}
	if raw, ok := envelope["result"]; ok {
		if slot != nil {
			slot.mu.Lock()
			slot.raw = cloneRaw(raw)
			slot.mu.Unlock()
		}
		if method == "server/discover" || method == "initialize" {
			t.mu.Lock()
			t.discovery = cloneRaw(raw)
			t.mu.Unlock()
		}
	}
	if rawMethod, ok := envelope["method"]; ok && t.onNotification != nil {
		var name string
		if json.Unmarshal(rawMethod, &name) == nil {
			t.onNotification(name, cloneRaw(envelope["params"]))
		}
	}
}
