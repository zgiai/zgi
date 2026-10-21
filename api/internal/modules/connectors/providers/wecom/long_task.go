package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type cliEnvelope struct {
	Result *string `json:"result"`
	Error  *struct {
		Code providers.OptionalCode `json:"code"`
	} `json:"error"`
	TaskID   string `json:"taskid"`
	PollMode *int   `json:"poll_mode"`
	Poll     *struct {
		Done     *bool `json:"done"`
		Timeout  *int  `json:"task_timeout"`
		Interval *int  `json:"polling_interval_ms"`
	} `json:"long_task_poll"`
}

func responseUnrecognized() error {
	return providers.ResponseFailure("operation", core.ErrorCodeProviderResponseUnrecognized, false, 0)
}
func cliCode(code providers.OptionalCode) error {
	if !code.Present || code.Value == 0 {
		return nil
	}
	kind := core.ErrorCodePermissionMissing
	if code.Value == 853004 {
		kind = core.ErrorCodeInvalidAppCredentials
	}
	return providers.ResponseFailure("operation", kind, false, code.Value)
}
func (c *Client) readEnvelope(ctx context.Context, path, token string, body any, taskID string) (cliEnvelope, error) {
	headers := http.Header{"Authorization": {"Bearer " + token}}
	if taskID != "" {
		headers.Set("X-Long-Poll-TaskId", taskID)
	}
	var outer struct {
		Code    providers.OptionalCode `json:"errcode"`
		Results string                 `json:"results_json"`
	}
	var inner cliEnvelope
	_, err := c.http.WithResponseLimit(1<<20).JSON(providers.WithDiagnosticPhase(ctx, "operation"), "POST", "/cli"+path, nil, headers, body, &outer)
	if err != nil {
		f := *providers.FailureOf(err)
		f.Unknown = false
		return inner, &f
	}
	if err = cliCode(outer.Code); err != nil {
		return inner, err
	}
	if !providers.ValidJSONObject([]byte(outer.Results)) || json.Unmarshal([]byte(outer.Results), &inner) != nil {
		return inner, responseUnrecognized()
	}
	if inner.Error != nil {
		if err = cliCode(inner.Error.Code); err != nil {
			return inner, err
		}
	}
	return inner, nil
}
func decodeBusiness(inner cliEnvelope) (map[string]any, error) {
	if inner.Result == nil || !providers.ValidJSONObject([]byte(*inner.Result)) {
		return nil, responseUnrecognized()
	}
	var result map[string]any
	var code struct {
		Code providers.OptionalCode `json:"errcode"`
	}
	if json.Unmarshal([]byte(*inner.Result), &result) != nil || json.Unmarshal([]byte(*inner.Result), &code) != nil {
		return nil, responseUnrecognized()
	}
	if err := cliCode(code.Code); err != nil {
		return nil, err
	}
	return result, nil
}
func (c *Client) readBusiness(ctx context.Context, path string, in providers.CredentialInput, token *string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, providers.Invalid()
	}
	defer clear(raw)
	request := map[string]string{"payload": string(raw)}
	inner, err := c.readEnvelope(ctx, path, *token, request, "")
	if err != nil && providers.FailureOf(err).Code == core.ErrorCodeInvalidAppCredentials {
		*token, err = c.token(ctx, in)
		if err != nil {
			return nil, err
		}
		inner, err = c.readEnvelope(ctx, path, *token, request, "")
	}
	if err != nil {
		return nil, err
	}
	if inner.TaskID == "" {
		return decodeBusiness(inner)
	}
	if path != "/doc/contents/get" || len(inner.TaskID) > 4096 || strings.ContainsAny(inner.TaskID, "\r\n\x00") {
		return nil, responseUnrecognized()
	}
	mode := 0
	if inner.PollMode != nil {
		mode = *inner.PollMode
	}
	if mode != 0 && mode != 1 {
		return nil, responseUnrecognized()
	}
	taskID := inner.TaskID
	deadline := time.Now().Add(120 * time.Second)
	interval := 500 * time.Millisecond
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for polls := 0; polls < 240; polls++ {
		if inner.Poll != nil {
			if inner.Poll.Timeout != nil && *inner.Poll.Timeout > 0 && *inner.Poll.Timeout <= 120 {
				shorter := time.Now().Add(time.Duration(*inner.Poll.Timeout) * time.Second)
				if shorter.Before(deadline) {
					deadline = shorter
				}
			}
			if inner.Poll.Interval != nil && *inner.Poll.Interval > 0 && *inner.Poll.Interval <= 120000 {
				interval = max(500*time.Millisecond, time.Duration(*inner.Poll.Interval)*time.Millisecond)
			}
		}
		if !time.Now().Add(interval).Before(deadline) {
			return nil, failure(core.ErrorCodeProviderUnavailable)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, failure(core.ErrorCodeProviderUnavailable)
		case <-timer.C:
		}
		pollPath, pollHeader := path, taskID
		var pollBody any = map[string]any{}
		if mode == 0 {
			pollPath = "/task/query"
			pollHeader = ""
			payload, _ := json.Marshal(map[string]string{"taskid": taskID})
			pollBody = map[string]string{"method": "PollClawLongTask", "payload": string(payload)}
		}
		pollContext, done := context.WithDeadline(ctx, deadline)
		inner, err = c.readEnvelope(pollContext, pollPath, *token, pollBody, pollHeader)
		done()
		if err != nil {
			return nil, err
		}
		if inner.Poll == nil || inner.Poll.Done == nil || inner.TaskID != "" && inner.TaskID != taskID {
			return nil, responseUnrecognized()
		}
		if *inner.Poll.Done {
			return decodeBusiness(inner)
		}
	}
	return nil, failure(core.ErrorCodeProviderUnavailable)
}
