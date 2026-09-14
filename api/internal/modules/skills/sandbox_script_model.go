package skills

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ScriptModelChannel is an execution-scoped capability supplied by the chat
// runtime. Scripts cannot choose a provider, model, identity, or credentials.
type ScriptModelChannel struct {
	Model    string
	Complete func(context.Context, string, string) (string, error)
}

const scriptModelRequestLimit = 256 * 1024
const scriptModelResponseLimit = 32 * 1024

type scriptModelRequest struct {
	Sequence int    `json:"sequence"`
	System   string `json:"system"`
	User     string `json:"user"`
}

func decodeScriptModelRequest(data []byte) (scriptModelRequest, error) {
	var request scriptModelRequest
	if len(data) > scriptModelRequestLimit {
		return request, fmt.Errorf("skill model request exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("invalid skill model request")
	}
	if decoder.Decode(new(interface{})) != io.EOF {
		return request, fmt.Errorf("invalid skill model request")
	}
	if request.Sequence < 0 || (request.Sequence > 0 && (strings.TrimSpace(request.System) == "" || strings.TrimSpace(request.User) == "")) {
		return request, fmt.Errorf("invalid skill model request")
	}
	return request, nil
}

func (r *SandboxScriptRunner) writeModelFile(ctx context.Context, sandboxID, path string, value interface{}, execCtx ExecutionContext) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.doIdempotentJSON(ctx, http.MethodPost, "/v1/files/upload", map[string]interface{}{
		"sandbox_id": sandboxID, "path": path, "encoding": "base64",
		"content": base64.StdEncoding.EncodeToString(data), "organization_id": execCtx.OrganizationID,
	}, nil, r.timeouts.Artifact)
}

// The existing authenticated sandbox file API carries requests and responses.
// No listener, egress rule, API key, or tenant-selected URL enters the sandbox.
// Each invocation gets its own sandbox and random mailbox, deleted on exit.
func (r *SandboxScriptRunner) runSkillWithModel(parent context.Context, sandboxID, stdin string, execCtx ExecutionContext, manifest skillScriptManifest) (*sandboxSkillRunResult, error) {
	timeout := skillScriptTimeoutSeconds(manifest.TimeoutMS)
	if manifest.ModelCalls == 0 {
		return r.runSkill(parent, sandboxID, stdin, execCtx, timeout)
	}
	if execCtx.ScriptModel == nil || execCtx.ScriptModel.Complete == nil || manifest.ModelCalls < 0 || manifest.ModelCalls > 3 {
		return nil, fmt.Errorf("skill model channel is unavailable")
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()
	modelCtx, cancelModel := context.WithCancel(ctx)
	defer cancelModel()
	execCtx.scriptModelPath = ".zgi-model-" + uuid.NewString()
	requestPath := execCtx.scriptModelPath + "/request.json"
	responsePath := execCtx.scriptModelPath + "/response.json"
	if err := r.writeModelFile(ctx, sandboxID, requestPath, scriptModelRequest{}, execCtx); err != nil {
		return nil, err
	}
	type completion struct {
		result *sandboxSkillRunResult
		err    error
	}
	done := make(chan completion, 1)
	go func() {
		result, err := r.runSkill(ctx, sandboxID, stdin, execCtx, timeout)
		done <- completion{result, err}
		cancelModel() // Stop model work without racing the final file response.
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	sequence := 0
	for {
		select {
		case result := <-done:
			return result.result, result.err
		case <-ctx.Done():
			select {
			case result := <-done:
				return result.result, result.err
			default:
			}
			return nil, ctx.Err()
		case <-ticker.C:
			var content sandboxFileContent
			endpoint := "/v1/files/download?sandbox_id=" + url.QueryEscape(sandboxID) + "&path=" + url.QueryEscape(requestPath) + "&encoding=base64&max_bytes=262144"
			err := r.doIdempotentJSON(ctx, http.MethodGet, withOrganizationQuery(endpoint, execCtx), nil, &content, r.timeouts.Artifact)
			if err != nil {
				return nil, fmt.Errorf("skill model channel read failed")
			}
			if content.MaxBytes != scriptModelRequestLimit || content.Encoding != "base64" {
				return nil, fmt.Errorf("sandbox bounded model channel reads are unavailable")
			}
			if len(content.Content) > base64.StdEncoding.EncodedLen(scriptModelRequestLimit) {
				return nil, fmt.Errorf("skill model request exceeds size limit")
			}
			data, err := base64.StdEncoding.DecodeString(content.Content)
			if err != nil {
				return nil, fmt.Errorf("invalid skill model request encoding")
			}
			request, err := decodeScriptModelRequest(data)
			if err != nil {
				return nil, err
			}
			if request.Sequence == sequence {
				continue
			}
			if request.Sequence != sequence+1 {
				return nil, fmt.Errorf("invalid skill model request sequence")
			}
			sequence = request.Sequence // Charge the attempt before invoking; never replay it.
			response := map[string]interface{}{"sequence": sequence}
			if sequence > manifest.ModelCalls {
				response["error"] = "skill model call limit exceeded"
			} else {
				select {
				case result := <-done:
					return result.result, result.err
				default:
				}
				callCtx, stop := context.WithTimeout(modelCtx, 60*time.Second)
				answer, callErr := execCtx.ScriptModel.Complete(callCtx, request.System, request.User)
				stop()
				select {
				case result := <-done:
					return result.result, result.err
				default:
				}
				if callErr != nil {
					response["error"] = "skill model request failed"
				} else if len(answer) > scriptModelResponseLimit {
					response["error"] = "skill model response exceeds size limit"
				} else {
					response["content"] = answer
				}
			}
			if err := r.writeModelFile(ctx, sandboxID, responsePath, response, execCtx); err != nil {
				return nil, fmt.Errorf("skill model channel write failed")
			}
		}
	}
}
