package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const maxProviderResponseBytes = 1024 * 1024

const (
	runtimeAgentBootstrapTokenPath = "/run/zgi-agent/bootstrap-token"
	runtimeAgentUser               = "zgi-runtime"
)

type controlClient struct {
	apiURL     string
	domain     string
	apiKey     string
	httpClient *http.Client
	sandboxURL func(port uint16, sandboxID string, domain string) string
}

type sandboxConnection struct {
	SandboxID          string `json:"sandboxID"`
	Domain             string `json:"domain"`
	EnvdAccessToken    string `json:"envdAccessToken"`
	TrafficAccessToken string `json:"trafficAccessToken"`
}

type createSandboxRequest struct {
	TemplateID      string            `json:"templateID"`
	Metadata        map[string]string `json:"metadata"`
	Environment     map[string]string `json:"envVars"`
	TimeoutSeconds  int               `json:"timeout"`
	AllowInternet   bool              `json:"allow_internet_access"`
	AutoPause       bool              `json:"autoPause"`
	AutoPauseMemory bool              `json:"autoPauseMemory"`
	AutoResume      autoResumeConfig  `json:"autoResume"`
	Network         networkConfig     `json:"network"`
}

type autoResumeConfig struct {
	Enabled bool `json:"enabled"`
}

type networkConfig struct {
	DenyOut            []string `json:"denyOut"`
	AllowPublicTraffic bool     `json:"allowPublicTraffic"`
}

func (c *controlClient) create(ctx context.Context, request createSandboxRequest) (sandboxConnection, error) {
	var result sandboxConnection
	if err := c.doControlJSON(ctx, http.MethodPost, "/v2/sandboxes", request, &result, http.StatusCreated); err != nil {
		return sandboxConnection{}, err
	}
	return c.validateConnection(result)
}

func (c *controlClient) listByAttempt(ctx context.Context, attemptID string) ([]string, error) {
	var ids []string
	var nextToken string
	seen := make(map[string]struct{})
	for page := 0; page < 10; page++ {
		metadata := url.Values{"zgi.attempt_id": {attemptID}}.Encode()
		query := url.Values{"metadata": {metadata}, "limit": {"100"}}
		if nextToken != "" {
			query.Set("nextToken", nextToken)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/v2/sandboxes?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("X-API-KEY", c.apiKey)
		request.Header.Set("Accept", "application/json")
		response, err := c.httpClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("%w: list E2B allocation attempts", agentexecution.ErrBackendUnavailable)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, providerStatusError(response.StatusCode)
		}
		var listed []struct {
			SandboxID string            `json:"sandboxID"`
			Metadata  map[string]string `json:"metadata"`
		}
		decodeErr := decodeLimitedJSON(response.Body, &listed)
		next := strings.TrimSpace(response.Header.Get("X-Next-Token"))
		closeErr := response.Body.Close()
		if err := errors.Join(decodeErr, closeErr); err != nil {
			return nil, fmt.Errorf("%w: decode E2B allocation list: %v", agentexecution.ErrBackendUnavailable, err)
		}
		for _, item := range listed {
			if item.Metadata["zgi.attempt_id"] != attemptID || strings.TrimSpace(item.SandboxID) == "" {
				return nil, fmt.Errorf("%w: E2B allocation list did not match the attempt", agentexecution.ErrBackendUnavailable)
			}
			if _, duplicate := seen[item.SandboxID]; !duplicate {
				seen[item.SandboxID] = struct{}{}
				ids = append(ids, item.SandboxID)
			}
		}
		if next == "" {
			return ids, nil
		}
		if next == nextToken {
			return nil, fmt.Errorf("%w: E2B allocation pagination did not advance", agentexecution.ErrBackendUnavailable)
		}
		nextToken = next
	}
	return nil, fmt.Errorf("%w: E2B allocation list exceeds page limit", agentexecution.ErrBackendUnavailable)
}

func (c *controlClient) connect(ctx context.Context, sandboxID string, timeoutSeconds int) (sandboxConnection, error) {
	var result sandboxConnection
	requestPath := "/v2/sandboxes/" + url.PathEscape(sandboxID) + "/connect"
	if err := c.doControlJSON(ctx, http.MethodPost, requestPath, map[string]int{"timeout": timeoutSeconds}, &result, http.StatusOK, http.StatusCreated); err != nil {
		return sandboxConnection{}, err
	}
	return c.validateConnection(result)
}

func (c *controlClient) pause(ctx context.Context, sandboxID string) error {
	requestPath := "/sandboxes/" + url.PathEscape(sandboxID) + "/pause"
	err := c.doControlJSON(ctx, http.MethodPost, requestPath, map[string]bool{"memory": true}, nil, http.StatusOK, http.StatusNoContent, http.StatusConflict)
	return err
}

func (c *controlClient) destroy(ctx context.Context, sandboxID string) error {
	requestPath := "/sandboxes/" + url.PathEscape(sandboxID)
	return c.doControlJSON(ctx, http.MethodDelete, requestPath, nil, nil, http.StatusOK, http.StatusNoContent, http.StatusNotFound)
}

func (c *controlClient) environment(ctx context.Context, connection sandboxConnection, envdPort uint16) (map[string]string, error) {
	requestURL := c.sandboxURL(envdPort, connection.SandboxID, connection.Domain)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(requestURL, "/")+"/envs", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Access-Token", connection.EnvdAccessToken)
	request.Header.Set("E2b-Sandbox-Id", connection.SandboxID)
	request.Header.Set("E2b-Sandbox-Port", fmt.Sprint(envdPort))
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: E2B envd request failed", agentexecution.ErrBackendUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, providerStatusError(response.StatusCode)
	}
	var result map[string]string
	if err := decodeLimitedJSON(response.Body, &result); err != nil {
		return nil, fmt.Errorf("%w: decode E2B envd response", agentexecution.ErrBackendUnavailable)
	}
	return result, nil
}

func (c *controlClient) uploadBootstrapToken(ctx context.Context, connection sandboxConnection, envdPort uint16, token string) error {
	requestURL := strings.TrimRight(c.sandboxURL(envdPort, connection.SandboxID, connection.Domain), "/") + "/files"
	query := url.Values{
		"path":     []string{runtimeAgentBootstrapTokenPath},
		"username": []string{runtimeAgentUser},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL+"?"+query.Encode(), strings.NewReader(token))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Access-Token", connection.EnvdAccessToken)
	request.Header.Set("E2b-Sandbox-Id", connection.SandboxID)
	request.Header.Set("E2b-Sandbox-Port", fmt.Sprint(envdPort))
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: E2B bootstrap upload failed", agentexecution.ErrBackendUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return providerStatusError(response.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxProviderResponseBytes))
	return nil
}

func (c *controlClient) doControlJSON(ctx context.Context, method string, requestPath string, body any, target any, allowed ...int) error {
	var reader io.Reader
	if body != nil {
		value, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(value)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.apiURL+requestPath, reader)
	if err != nil {
		return err
	}
	request.Header.Set("X-API-KEY", c.apiKey)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: E2B control request failed", agentexecution.ErrBackendUnavailable)
	}
	defer response.Body.Close()
	for _, status := range allowed {
		if response.StatusCode == status {
			if target == nil || response.StatusCode == http.StatusNoContent {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxProviderResponseBytes))
				return nil
			}
			if err := decodeLimitedJSON(response.Body, target); err != nil {
				return fmt.Errorf("%w: decode E2B control response", agentexecution.ErrBackendUnavailable)
			}
			return nil
		}
	}
	return providerStatusError(response.StatusCode)
}

func (c *controlClient) validateConnection(connection sandboxConnection) (sandboxConnection, error) {
	connection.SandboxID = strings.TrimSpace(connection.SandboxID)
	connection.Domain = strings.TrimSpace(connection.Domain)
	connection.EnvdAccessToken = strings.TrimSpace(connection.EnvdAccessToken)
	connection.TrafficAccessToken = strings.TrimSpace(connection.TrafficAccessToken)
	if connection.Domain == "" {
		connection.Domain = c.domain
	}
	if connection.SandboxID == "" || connection.EnvdAccessToken == "" || connection.TrafficAccessToken == "" {
		return sandboxConnection{}, fmt.Errorf("%w: E2B returned incomplete secured connection data", agentexecution.ErrBackendUnavailable)
	}
	return connection, nil
}

func providerStatusError(status int) error {
	switch status {
	case http.StatusNotFound:
		return agentexecution.ErrNotFound
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: E2B rejected the runtime request", agentexecution.ErrInvalidRequest)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: E2B authentication was rejected", agentexecution.ErrBackendUnavailable)
	case http.StatusTooManyRequests, http.StatusRequestTimeout, http.StatusConflict:
		return fmt.Errorf("%w: E2B is temporarily unable to serve the request", agentexecution.ErrBackendUnavailable)
	default:
		if status >= 500 {
			return fmt.Errorf("%w: E2B service returned HTTP %d", agentexecution.ErrBackendUnavailable, status)
		}
		return fmt.Errorf("%w: E2B returned HTTP %d", agentexecution.ErrBackendUnavailable, status)
	}
}

func decodeLimitedJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, maxProviderResponseBytes+1))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
