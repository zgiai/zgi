package skills

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Run the real Python package over an authenticated file-API test double.
// This verifies transport and orchestration, not Linux network isolation or
// a real model's clinical accuracy.
func modelSandbox(t *testing.T, script ...string) (*SandboxScriptRunner, string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	workspace := t.TempDir()
	entry, err := filepath.Abs("../../../examples/private-skills/clinical-text-analysis/scripts/run.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(script) == 0 {
		requireLocalClinicalSkill(t)
	}
	var filesMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-sandbox-key" {
			http.Error(w, "unauthorized", 401)
			return
		}
		var result interface{} = map[string]interface{}{}
		switch r.URL.Path {
		case "/v1/files/upload":
			var payload struct {
				SandboxID    string `json:"sandbox_id"`
				Organization string `json:"organization_id"`
				Path         string `json:"path"`
				Content      string `json:"content"`
			}
			if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.SandboxID != "box" || payload.Organization != "org" {
				http.Error(w, "invalid scope", 400)
				return
			}
			data, err := base64.StdEncoding.DecodeString(payload.Content)
			if err != nil {
				http.Error(w, "invalid encoding", 400)
				return
			}
			filesMu.Lock()
			path := filepath.Join(workspace, payload.Path)
			err = os.MkdirAll(filepath.Dir(path), 0700)
			if err == nil {
				err = os.WriteFile(path, data, 0600)
			}
			filesMu.Unlock()
			if err != nil {
				http.Error(w, "write failed", 500)
				return
			}
		case "/v1/files/download":
			if r.URL.Query().Get("organization_id") != "org" || r.URL.Query().Get("sandbox_id") != "box" {
				http.Error(w, "invalid scope", 400)
				return
			}
			filesMu.Lock()
			data, err := os.ReadFile(filepath.Join(workspace, r.URL.Query().Get("path")))
			filesMu.Unlock()
			if err != nil {
				http.Error(w, "missing", 404)
				return
			}
			if r.URL.Query().Get("max_bytes") != "262144" || len(data) > scriptModelRequestLimit {
				http.Error(w, "invalid download limit", 400)
				return
			}
			result = map[string]interface{}{"content": base64.StdEncoding.EncodeToString(data), "encoding": "base64", "max_bytes": scriptModelRequestLimit}
		case "/v1/exec/skill":
			var payload struct {
				Stdin string            `json:"stdin"`
				Env   map[string]string `json:"env"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid", 400)
				return
			}
			command := exec.CommandContext(r.Context(), "python3", entry)
			if len(script) > 0 {
				command = exec.CommandContext(r.Context(), "python3", "-c", script[0])
			}
			command.Dir = workspace
			command.Stdin = strings.NewReader(payload.Stdin)
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "PYTHONDONTWRITEBYTECODE=1"}
			for key, value := range payload.Env {
				command.Env = append(command.Env, key+"="+value)
			}
			output, err := command.Output()
			exitCode := 0
			if err != nil {
				exitCode = 1
			}
			result = sandboxSkillRunResult{Command: sandboxCommandResult{Stdout: string(output), ExitCode: exitCode}}
		default:
			http.Error(w, "unexpected route", 404)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "data": result})
	}))
	t.Cleanup(server.Close)
	return NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL, APIKey: "test-sandbox-key"}), workspace
}

func TestClinicalTextModelChannelWorkflow(t *testing.T) {
	for _, tc := range []struct {
		name, input  string
		replies      []string
		calls, limit int
		failed       string
	}{
		{"complete", `{"text":"患者主诉失眠","question":"有什么症状？"}`, []string{`{"symptoms":["失眠"]}`, `{"items":[{"subject":"患者","predicate":"reports","object":"失眠","evidence":"患者主诉失眠"}]}`, `{"answer":"患者主诉失眠。"}`}, 3, 3, ""},
		{"no_question", `{"text":"患者主诉失眠"}`, []string{`{"symptoms":["失眠"]}`, `{"items":[]}`}, 2, 3, ""},
		{"invalid_input", `{"text":""}`, nil, 0, 3, "input"},
		{"invalid_json", `{"text":"患者主诉失眠"}`, []string{"not json"}, 1, 3, "extraction"},
		{"relation_failure", `{"text":"患者主诉失眠"}`, []string{`{"symptoms":["失眠"]}`, "FAIL"}, 2, 3, "relation"},
		{"answer_failure", `{"text":"患者主诉失眠","question":"有什么症状？"}`, []string{`{"symptoms":["失眠"]}`, `{"items":[]}`, "FAIL"}, 3, 3, "answer"},
		{"quota", `{"text":"患者主诉失眠"}`, []string{`{"symptoms":["失眠"]}`}, 1, 1, "relation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, workspace := modelSandbox(t)
			calls := 0
			channel := &ScriptModelChannel{Model: "configured-test-model", Complete: func(ctx context.Context, system, user string) (string, error) {
				calls++
				if calls > len(tc.replies) {
					return "", fmt.Errorf("unexpected call")
				}
				if calls == 2 && !strings.Contains(user, `"symptoms": ["失眠"]`) {
					t.Error("extraction missing from relation prompt")
				}
				if tc.name == "complete" && calls == 3 && !strings.Contains(user, `"predicate": "reports"`) {
					t.Error("relations missing from answer prompt")
				}
				if tc.replies[calls-1] == "FAIL" {
					return "", fmt.Errorf("private upstream credentials")
				}
				return tc.replies[calls-1], nil
			}}
			result, err := runner.runSkillWithModel(context.Background(), "box", tc.input, ExecutionContext{OrganizationID: "org", UserID: "user", ScriptModel: channel}, skillScriptManifest{TimeoutMS: 10000, ModelCalls: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			if calls != tc.calls {
				t.Fatalf("calls=%d want=%d", calls, tc.calls)
			}
			var output map[string]interface{}
			if err := json.Unmarshal([]byte(result.Command.Stdout), &output); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(result.Command.Stdout, "private upstream") {
				t.Fatal("upstream error leaked")
			}
			if tc.failed == "" {
				if output["status"] != "completed" || result.Command.ExitCode != 0 {
					t.Fatalf("result=%v", output)
				}
				if output["model_version"] != channel.Model {
					t.Fatal("wrong model identity")
				}
			} else if output["failed_step"] != tc.failed || result.Command.ExitCode == 0 {
				t.Fatalf("result=%v", output)
			}
			artifact, err := os.ReadFile(filepath.Join(workspace, "artifacts/clinical-text-analysis-result.json"))
			if err != nil || strings.TrimSpace(string(artifact)) != strings.TrimSpace(result.Command.Stdout) {
				t.Fatal("artifact differs from workflow output", err)
			}
		})
	}
}

func TestScriptModelRequestValidation(t *testing.T) {
	for _, raw := range []string{`{"sequence":1,"system":"s","user":"u","model":"other"}`, `{"sequence":-1}`, `{"sequence":1}`, `{} {}`, strings.Repeat("x", scriptModelRequestLimit+1)} {
		if _, err := decodeScriptModelRequest([]byte(raw)); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestScriptModelCancellation(t *testing.T) {
	runner, _ := modelSandbox(t)
	called := false
	channel := &ScriptModelChannel{Model: "test", Complete: func(ctx context.Context, _, _ string) (string, error) {
		called = true
		<-ctx.Done()
		return "", ctx.Err()
	}}
	start := time.Now()
	_, err := runner.runSkillWithModel(context.Background(), "box", `{"text":"失眠"}`, ExecutionContext{OrganizationID: "org", UserID: "user", ScriptModel: channel}, skillScriptManifest{TimeoutMS: 1000, ModelCalls: 3})
	if err == nil || !called || time.Since(start) > 3*time.Second {
		t.Fatalf("cancellation failed: %v", err)
	}
}

func TestClinicalTextPrivateSkillPackage(t *testing.T) {
	requireLocalClinicalSkill(t)
	root := "../../../examples/private-skills/clinical-text-analysis"
	doc, err := LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Metadata.HasScripts || len(doc.Tools) != 1 || !SkillSupportsCaller(doc.Metadata.SupportedCallers, SkillCallerAgent) {
		t.Fatal("not an Agent script skill")
	}
	if err := validateSkillToolArguments(doc.Metadata.ID, doc.Tools[0], map[string]interface{}{"text": "失眠"}); err != nil {
		t.Fatal(err)
	}
	if err := validateSkillToolArguments(doc.Metadata.ID, doc.Tools[0], map[string]interface{}{"text": "失眠", "model": "other"}); err == nil {
		t.Fatal("model override accepted")
	}
	manifest, err := prepareSkillScriptManifest(root, 240, false)
	if err != nil || manifest.Manifest.ModelCalls != 3 {
		t.Fatal("invalid manifest", err)
	}
}

func TestScriptModelDoesNotReplayAndRejectsSkippedSequence(t *testing.T) {
	runner, _ := modelSandbox(t, `
import json, os, pathlib, time
p = pathlib.Path(os.environ['ZGI_MODEL_CHANNEL'])
def publish(n):
    f = p / 'pending'
    f.write_text(json.dumps({'sequence':n, 'system':'s', 'user':'u'}))
    f.replace(p / 'request.json')
publish(1)
while not (p / 'response.json').exists(): time.sleep(.05)
time.sleep(.7)
publish(3)
time.sleep(10)
`)
	calls := 0
	channel := &ScriptModelChannel{Model: "test", Complete: func(context.Context, string, string) (string, error) { calls++; return `{}`, nil }}
	_, err := runner.runSkillWithModel(context.Background(), "box", `{}`, ExecutionContext{OrganizationID: "org", UserID: "user", ScriptModel: channel}, skillScriptManifest{TimeoutMS: 5000, ModelCalls: 3})
	if err == nil || !strings.Contains(err.Error(), "sequence") || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestScriptExitCancelsPendingModel(t *testing.T) {
	runner, _ := modelSandbox(t, `
import json, os, pathlib, time
p = pathlib.Path(os.environ['ZGI_MODEL_CHANNEL'])
f = p / 'pending'
f.write_text(json.dumps({'sequence':1, 'system':'s', 'user':'u'}))
f.replace(p / 'request.json')
time.sleep(.5)
`)
	cancelled := false
	channel := &ScriptModelChannel{Model: "test", Complete: func(ctx context.Context, _, _ string) (string, error) {
		<-ctx.Done()
		cancelled = true
		return "", ctx.Err()
	}}
	start := time.Now()
	_, err := runner.runSkillWithModel(context.Background(), "box", `{}`, ExecutionContext{OrganizationID: "org", UserID: "user", ScriptModel: channel}, skillScriptManifest{TimeoutMS: 5000, ModelCalls: 3})
	if err != nil || !cancelled || time.Since(start) > 3*time.Second {
		t.Fatalf("cancelled=%v err=%v", cancelled, err)
	}
}

func TestModelEnabledSkillRequiresServerCapability(t *testing.T) {
	requireLocalClinicalSkill(t)
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: "http://127.0.0.1:1"})
	doc, err := LoadCustomSkillDocument("../../../examples/private-skills/clinical-text-analysis")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.RunSkillScript(context.Background(), doc, map[string]interface{}{"text": "test"}, ExecutionContext{OrganizationID: "org", UserID: "user"}, "test")
	if err == nil || !strings.Contains(err.Error(), "model channel is unavailable") {
		t.Fatalf("expected rejection before network activity: %v", err)
	}
}

// The clinical package is distributed separately from the platform code.
// Package acceptance tests remain available locally; transport tests with
// embedded fixtures run independently in a platform-only checkout.
func requireLocalClinicalSkill(t *testing.T) {
	t.Helper()
	_, err := os.Stat("../../../examples/private-skills/clinical-text-analysis/scripts/run.py")
	if os.IsNotExist(err) {
		t.Skip("separately distributed clinical Skill package is not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
}
