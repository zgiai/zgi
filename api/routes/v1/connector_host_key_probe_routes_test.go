package v1

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorproviders "github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	connectorserveropsssh "github.com/zgiai/zgi/api/internal/modules/connectors/providers/serveropsssh"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const hostKeyProbeFingerprint = "SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU"

const hostKeyProbePath = "/console/api/connectors/" + catalog.ServerOpsSSHConnectorID + "/host-key-tests"

// connectorHostKeyProbeStub is the injected, network-free probe seam. It keeps
// the request map reference so a test can prove the route destroyed it, and an
// independent snapshot to prove which values reached the seam.
type connectorHostKeyProbeStub struct {
	calls       int
	values      map[string]string
	snapshot    map[string]string
	observation connectorserveropsssh.HostKeyObservation
	err         error
}

func (stub *connectorHostKeyProbeStub) ProbeHostKey(_ context.Context, values map[string]string) (connectorserveropsssh.HostKeyObservation, error) {
	stub.calls++
	stub.values = values
	stub.snapshot = make(map[string]string, len(values))
	for name, value := range values {
		stub.snapshot[name] = value
	}
	return stub.observation, stub.err
}

func hostKeyProbeRouteRouter(t *testing.T, probe connectorserveropsssh.HostKeyProbe, organizationID, ownerID uuid.UUID) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		if organizationID != uuid.Nil {
			util.SetOrganizationID(c, organizationID.String())
		}
		if ownerID != uuid.Nil {
			c.Set("account_id", ownerID.String())
		}
		c.Next()
	})
	definitions, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), core.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(definitions)
	if err != nil {
		t.Fatal(err)
	}
	handler := &connectorHandler{catalog: credentialSetupRouteCatalog(t), hostKeyProbe: probe, errorProjector: projector}
	handler.register(group)
	return router
}

func hostKeyProbeCaptureValues() map[string]string {
	values := sshFixtureValues(sshFixturePrivateKey())
	delete(values, "host_key_sha256")
	return values
}

// TestHostKeyProbeRouteReturnsObservedFingerprint pins the read-only success
// contract: no Idempotency-Key is required, the response is uncacheable, the
// observation is returned unmodified, and the submitted secret is destroyed.
func TestHostKeyProbeRouteReturnsObservedFingerprint(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	probe := &connectorHostKeyProbeStub{observation: connectorserveropsssh.HostKeyObservation{
		Fingerprint: hostKeyProbeFingerprint, Hostname: "remote-host.internal", RequestID: "req-route-1",
	}}
	router := hostKeyProbeRouteRouter(t, probe, organizationID, ownerID)

	response := connectorRouteRequest(t, router, http.MethodPost, hostKeyProbePath,
		credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("probe status/body = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("probe response is cacheable")
	}
	for _, want := range []string{hostKeyProbeFingerprint, "remote-host.internal", "req-route-1"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("probe body %s does not contain %q", response.Body.String(), want)
		}
	}
	if probe.calls != 1 {
		t.Fatalf("probe calls = %d, want 1", probe.calls)
	}
	if probe.snapshot["host"] != "server.example.internal" || probe.snapshot["port"] != "22" || probe.snapshot["username"] != "deploy" {
		t.Fatalf("probe received %#v", probe.snapshot)
	}
	if _, present := probe.snapshot["host_key_sha256"]; present {
		t.Fatalf("capture probe received a fingerprint: %#v", probe.snapshot)
	}
	if len(probe.values) != 0 {
		t.Fatalf("route retained probe values after the handler returned: %#v", probe.values)
	}
	assertHostKeyProbeBodyIsSecretFree(t, response.Body.String())
}

// TestHostKeyProbeRouteRejectsNonSSHConnectors pins the surface gate: only the
// exact SSH Server Operations dialect reaches the probe.
func TestHostKeyProbeRouteRejectsNonSSHConnectors(t *testing.T) {
	probe := &connectorHostKeyProbeStub{}
	router := hostKeyProbeRouteRouter(t, probe, uuid.New(), uuid.New())

	for _, connectorID := range []string{"dingtalk", "feishu", "github", "unknown"} {
		t.Run(connectorID, func(t *testing.T) {
			response := connectorRouteRequest(t, router, http.MethodPost,
				"/console/api/connectors/"+connectorID+"/host-key-tests",
				credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil), nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("non-SSH probe status/body = %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("non-SSH probe response is cacheable")
			}
			assertHostKeyProbeBodyIsSecretFree(t, response.Body.String())
		})
	}
	if probe.calls != 0 {
		t.Fatalf("non-SSH probe reached the seam %d times", probe.calls)
	}
}

// TestHostKeyProbeRouteRejectsMalformedBodyAndFields pins the strict decode
// contract and the raw mail_usage/target presence checks.
func TestHostKeyProbeRouteRejectsMalformedBodyAndFields(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	oversized := `{"values":{"private_key":"` + strings.Repeat("A", 64*1024) + `"}}`

	cases := map[string]string{
		"unknown top-level field": credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"unexpected": true}),
		"two json objects":        credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil) + credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil),
		"target populated":        credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"target": map[string]any{"type": "agent", "id": "agent-1"}}),
		"target null":             credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"target": nil}),
		"target blank":            credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"target": "  "}),
		"mail_usage populated":    credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"mail_usage": "read_send"}),
		"mail_usage null":         credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"mail_usage": nil}),
		"mail_usage blank":        credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), map[string]any{"mail_usage": ""}),
		"oversized body":          oversized,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			probe := &connectorHostKeyProbeStub{}
			router := hostKeyProbeRouteRouter(t, probe, organizationID, ownerID)
			response := connectorRouteRequest(t, router, http.MethodPost, hostKeyProbePath, body, nil)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("malformed probe status/body = %d %s", response.Code, response.Body.String())
			}
			if probe.calls != 0 {
				t.Fatalf("malformed probe reached the seam %d times", probe.calls)
			}
			assertHostKeyProbeBodyIsSecretFree(t, response.Body.String())
		})
	}
}

// TestHostKeyProbeRouteRequiresOwnerScope pins the authenticated owner boundary.
func TestHostKeyProbeRouteRequiresOwnerScope(t *testing.T) {
	probe := &connectorHostKeyProbeStub{}
	for name, actor := range map[string][2]uuid.UUID{
		"missing actor": {uuid.Nil, uuid.Nil},
		"missing owner": {uuid.New(), uuid.Nil},
	} {
		t.Run(name, func(t *testing.T) {
			router := hostKeyProbeRouteRouter(t, probe, actor[0], actor[1])
			response := connectorRouteRequest(t, router, http.MethodPost, hostKeyProbePath,
				credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil), nil)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated probe status/body = %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unauthenticated probe response is cacheable")
			}
			assertHostKeyProbeBodyIsSecretFree(t, response.Body.String())
		})
	}
	if probe.calls != 0 {
		t.Fatalf("unauthenticated probe reached the seam %d times", probe.calls)
	}
}

// TestHostKeyProbeRouteReportsUnavailableSeam pins the 503 fallback when the
// probe dependency is not wired.
func TestHostKeyProbeRouteReportsUnavailableSeam(t *testing.T) {
	router := hostKeyProbeRouteRouter(t, nil, uuid.New(), uuid.New())
	response := connectorRouteRequest(t, router, http.MethodPost, hostKeyProbePath,
		credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil), nil)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired probe status/body = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unwired probe response is cacheable")
	}
	assertHostKeyProbeBodyIsSecretFree(t, response.Body.String())
}

// TestHostKeyProbeRouteNeverEchoesSecretsOrProviderText pins the failure
// projection: neither a classified provider failure nor an unstructured cause
// that echoes a submitted secret can reach the response body.
func TestHostKeyProbeRouteNeverEchoesSecretsOrProviderText(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()

	cases := map[string]struct {
		err    error
		status int
	}{
		"invalid input": {err: connectorproviders.Invalid(), status: http.StatusBadRequest},
		"provider unavailable": {
			err:    &connectorproviders.Failure{Code: core.ErrorCodeProviderUnavailable, Retryable: true},
			status: http.StatusServiceUnavailable,
		},
		"unstructured cause": {
			err:    fmt.Errorf("remote echoed %s and %s", sshFixturePrivateKey(), "fixture-passphrase"),
			status: http.StatusServiceUnavailable,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			probe := &connectorHostKeyProbeStub{err: testCase.err}
			router := hostKeyProbeRouteRouter(t, probe, organizationID, ownerID)
			response := connectorRouteRequest(t, router, http.MethodPost, hostKeyProbePath,
				credentialSetupRequestBody(t, hostKeyProbeCaptureValues(), nil), nil)
			if response.Code != testCase.status {
				t.Fatalf("probe failure status/body = %d %s", response.Code, response.Body.String())
			}
			assertHostKeyProbeBodyIsSecretFree(t, response.Body.String())
		})
	}
}

func assertHostKeyProbeBodyIsSecretFree(t *testing.T, body string) {
	t.Helper()
	assertConnectorRouteBodyIsSecretFree(t, body)
	for _, forbidden := range []string{credentialSetupSecretSentinel, "fixture-passphrase", "remote echoed"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("probe response exposed forbidden value %q: %s", forbidden, body)
		}
	}
}
