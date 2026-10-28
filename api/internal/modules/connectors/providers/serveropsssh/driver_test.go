package serveropsssh

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

const testPassphrase = "correct horse battery staple"

// testPrivateKey renders a bounded, PEM/OpenSSH-shaped fixture. The adapter
// never parses key material; the catalog contract validates text shape only.
func testPrivateKey() string {
	return strings.Join([]string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW",
		"QyNTUxOQAAACBmaXh0dXJlLWtleS1tYXRlcmlhbC1ub3QtYS1yZWFsLXNlY3JldA==",
		"-----END OPENSSH PRIVATE KEY-----",
	}, "\n") + "\n"
}

func testFingerprint() string {
	sum := sha256.Sum256([]byte("serveropsssh-adapter-test-host-key"))
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func testValues() map[string]string {
	return map[string]string{
		fieldHost:          "server.example.internal",
		fieldPort:          "22",
		fieldUsername:      "deploy",
		fieldPrivateKey:    testPrivateKey(),
		fieldPassphrase:    testPassphrase,
		fieldHostKeySHA256: testFingerprint(),
	}
}

func testScope() providers.ResourceScope {
	return providers.ResourceScope{
		OrganizationID:     uuid.New(),
		OwnerAccountID:     uuid.New(),
		ConnectionID:       uuid.New(),
		CredentialVersion:  1,
		BindingID:          uuid.New(),
		BindingRevision:    1,
		LogicalOperationID: uuid.NewString(),
		Target: core.TargetRef{
			Type:              core.TargetTypeDigitalHuman,
			ID:                uuid.NewString(),
			Scope:             core.ScopeDraft,
			BindingInstanceID: uuid.NewString(),
		},
	}
}

// fakeService is the injected, network-free capability seam. It records the
// request-scoped Connection it observed, including an independent string copy of
// the secret bytes so assertions survive the adapter's zeroing.
type fakeService struct {
	validateCalls int
	snapshotCalls int
	statusCalls   int
	journalCalls  int
	changeCalls   int

	lastConnection     serverops.Connection
	observedPassword   string
	observedPrivateKey string
	observedPassphrase string
	lastServiceName    string
	lastOptions        serverops.JournalOptions
	lastAction         string

	profile  *serverops.ValidationProfile
	snapshot *serverops.SystemSnapshot
	status   *serverops.ServiceStatus
	journal  *serverops.JournalTail
	change   *serverops.ServiceChange

	validateErr error
	snapshotErr error
	statusErr   error
	journalErr  error
	changeErr   error
}

func (f *fakeService) observe(connection serverops.Connection) {
	f.lastConnection = connection
	f.observedPassword = string(connection.Password)
	f.observedPrivateKey = string(connection.PrivateKey)
	f.observedPassphrase = string(connection.Passphrase)
}

func (f *fakeService) ValidateConnection(_ context.Context, connection serverops.Connection) (*serverops.ValidationProfile, error) {
	f.validateCalls++
	f.observe(connection)
	return f.profile, f.validateErr
}

func (f *fakeService) SystemSnapshot(_ context.Context, connection serverops.Connection) (*serverops.SystemSnapshot, error) {
	f.snapshotCalls++
	f.observe(connection)
	return f.snapshot, f.snapshotErr
}

func (f *fakeService) ServiceStatus(_ context.Context, connection serverops.Connection, serviceName string) (*serverops.ServiceStatus, error) {
	f.statusCalls++
	f.observe(connection)
	f.lastServiceName = serviceName
	return f.status, f.statusErr
}

func (f *fakeService) JournalTail(_ context.Context, connection serverops.Connection, serviceName string, options serverops.JournalOptions) (*serverops.JournalTail, error) {
	f.journalCalls++
	f.observe(connection)
	f.lastServiceName = serviceName
	f.lastOptions = options
	return f.journal, f.journalErr
}

func (f *fakeService) ChangeService(_ context.Context, connection serverops.Connection, action, serviceName string) (*serverops.ServiceChange, error) {
	f.changeCalls++
	f.observe(connection)
	f.lastAction = action
	f.lastServiceName = serviceName
	return f.change, f.changeErr
}

func (f *fakeService) totalCalls() int {
	return f.validateCalls + f.snapshotCalls + f.statusCalls + f.journalCalls + f.changeCalls
}

func newTestClient(t *testing.T, service Service) *Client {
	t.Helper()
	client, err := New(service)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func failureOf(t *testing.T, err error) *providers.Failure {
	t.Helper()
	var failure *providers.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %v (%T), want *providers.Failure", err, err)
	}
	return failure
}

func allZero(value []byte) bool {
	for _, b := range value {
		if b != 0 {
			return false
		}
	}
	return true
}

func TestNewRejectsNilServiceAndPinsConnectorID(t *testing.T) {
	t.Parallel()

	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) accepted a missing service")
	} else if code := failureOf(t, err).Code; code != core.ErrorCodeInvalidInput {
		t.Fatalf("New(nil) code = %q, want %q", code, core.ErrorCodeInvalidInput)
	}

	client := newTestClient(t, &fakeService{})
	if client.ConnectorID() != catalog.ServerOpsSSHConnectorID {
		t.Fatalf("ConnectorID() = %q, want %q", client.ConnectorID(), catalog.ServerOpsSSHConnectorID)
	}
}

func TestVerifyCredentialsUsesFixedProbeAndStableIdentity(t *testing.T) {
	t.Parallel()

	fake := &fakeService{profile: &serverops.ValidationProfile{
		Attempt:  serverops.Attempt{RequestID: "req-verify", AttemptCount: 1},
		Hostname: "remote-host.internal",
	}}
	client := newTestClient(t, fake)

	verification, err := client.VerifyCredentials(context.Background(), providers.CredentialInput{Values: testValues()})
	if err != nil {
		t.Fatalf("VerifyCredentials() error = %v", err)
	}
	if fake.validateCalls != 1 || fake.totalCalls() != 1 {
		t.Fatalf("capability calls = validate:%d total:%d, want a single validate probe", fake.validateCalls, fake.totalCalls())
	}
	if verification.AccountID != "deploy@server.example.internal:22" {
		t.Fatalf("AccountID = %q", verification.AccountID)
	}
	if verification.DisplayName != "remote-host.internal" {
		t.Fatalf("DisplayName = %q", verification.DisplayName)
	}
	if verification.RequestID != "req-verify" {
		t.Fatalf("RequestID = %q", verification.RequestID)
	}
	wantOperations := []string{
		serverops.ActionSystemSnapshot, serverops.ActionServiceStatus, serverops.ActionJournalTail,
		serverops.ActionServiceRestart, serverops.ActionServiceStart, serverops.ActionServiceStop, serverops.ActionServiceReload,
	}
	if !reflect.DeepEqual(verification.Operations, wantOperations) {
		t.Fatalf("Operations = %#v, want %#v", verification.Operations, wantOperations)
	}
	wantEvidence := &core.ProviderEvidence{ServerOps: &core.ServerOpsEvidence{
		Version: 1, Identity: "deploy@server.example.internal:22", Privilege: "",
	}}
	if !reflect.DeepEqual(verification.Evidence, wantEvidence) {
		t.Fatalf("Evidence = %#v, want %#v", verification.Evidence, wantEvidence)
	}
	if fake.lastConnection.Privilege != serverops.PrivilegeNone {
		t.Fatalf("observed privilege = %q, want none", fake.lastConnection.Privilege)
	}

	// The probe received the pinned request-scoped connection, not a persisted one.
	if fake.lastConnection.Host != "server.example.internal" || fake.lastConnection.Port != 22 ||
		fake.lastConnection.Username != "deploy" || fake.lastConnection.HostKeySHA256 != testFingerprint() {
		t.Fatalf("observed connection = %+v", fake.lastConnection)
	}
	if fake.observedPrivateKey != testPrivateKey() || fake.observedPassphrase != testPassphrase {
		t.Fatal("probe did not receive the submitted secret material")
	}
	if !allZero(fake.lastConnection.PrivateKey) || !allZero(fake.lastConnection.Passphrase) {
		t.Fatal("temporary secret copies were not zeroed after the probe")
	}
}

func TestVerifyCredentialsSupportsPasswordAuthentication(t *testing.T) {
	t.Parallel()

	values := testValues()
	delete(values, fieldPrivateKey)
	delete(values, fieldPassphrase)
	values[fieldPassword] = "server-password"
	fake := &fakeService{profile: &serverops.ValidationProfile{
		Attempt:  serverops.Attempt{RequestID: "req-password", AttemptCount: 1},
		Hostname: "remote-host.internal",
	}}

	verification, err := newTestClient(t, fake).VerifyCredentials(
		context.Background(),
		providers.CredentialInput{Values: values},
	)
	if err != nil {
		t.Fatalf("VerifyCredentials() error = %v", err)
	}
	if verification.RequestID != "req-password" || fake.observedPassword != "server-password" {
		t.Fatalf("verification = %+v, observed password = %q", verification, fake.observedPassword)
	}
	if !allZero(fake.lastConnection.Password) || len(fake.lastConnection.PrivateKey) != 0 {
		t.Fatal("temporary password material was not zeroed or private key was unexpectedly set")
	}
}

// verifyAccountIdentity runs the fixed probe for one credential set and returns
// the account identity the adapter reports.
func verifyAccountIdentity(t *testing.T, values map[string]string) string {
	t.Helper()
	fake := &fakeService{profile: &serverops.ValidationProfile{
		Attempt:  serverops.Attempt{RequestID: "req-identity", AttemptCount: 1},
		Hostname: "remote-host.internal",
	}}
	verification, err := newTestClient(t, fake).VerifyCredentials(context.Background(), providers.CredentialInput{Values: values})
	if err != nil {
		t.Fatalf("VerifyCredentials() error = %v", err)
	}
	return verification.AccountID
}

// TestVerifyCredentialsRendersUnambiguousAccountIdentity pins the exact identity
// rendering for every host family and for a username that carries the account
// separator. An IPv6 literal must be bracketed exactly as net.JoinHostPort
// brackets the dial address, and an "@" in the username must not blur the
// account/host boundary.
func TestVerifyCredentialsRendersUnambiguousAccountIdentity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		host     string
		port     string
		username string
		want     string
	}{
		{"dns host", "server.example.internal", "22", "deploy", "deploy@server.example.internal:22"},
		{"ipv4 host", "203.0.113.7", "2222", "deploy", "deploy@203.0.113.7:2222"},
		{"ipv6 host", "2001:db8::1", "22", "deploy", "deploy@[2001:db8::1]:22"},
		{"ipv6 loopback", "::1", "2200", "deploy", "deploy@[::1]:2200"},
		{"username with at sign", "server.example.internal", "22", "deploy@corp", `"deploy@corp"@server.example.internal:22`},
		{"username with at sign and ipv6 host", "2001:db8::1", "22", "ops@corp", `"ops@corp"@[2001:db8::1]:22`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			values := testValues()
			values[fieldHost] = testCase.host
			values[fieldPort] = testCase.port
			values[fieldUsername] = testCase.username

			identity := verifyAccountIdentity(t, values)
			if identity != testCase.want {
				t.Fatalf("AccountID = %q, want %q", identity, testCase.want)
			}
			if strings.Contains(identity, testPassphrase) || strings.Contains(identity, testPrivateKey()) {
				t.Fatalf("AccountID leaked a credential: %q", identity)
			}
		})
	}
}

// TestVerifyCredentialsIdentityIsStableAndInjective pins the reconnect
// invariant: the same username, host, and port always yield the same identity,
// a changed endpoint does not, and a username whose literal text already spells
// another username's quoted rendering stays distinct.
func TestVerifyCredentialsIdentityIsStableAndInjective(t *testing.T) {
	t.Parallel()

	values := testValues()
	values[fieldHost] = "2001:db8::1"
	values[fieldUsername] = "ops@corp"
	first := verifyAccountIdentity(t, values)
	for attempt := 0; attempt < 3; attempt++ {
		if next := verifyAccountIdentity(t, values); next != first {
			t.Fatalf("identity changed for identical credentials: %q -> %q", first, next)
		}
	}

	otherEndpoint := testValues()
	otherEndpoint[fieldHost] = "2001:db8::2"
	otherEndpoint[fieldUsername] = "ops@corp"
	if other := verifyAccountIdentity(t, otherEndpoint); other == first {
		t.Fatalf("distinct endpoints share identity %q", first)
	}

	separatorUser := testValues()
	separatorUser[fieldUsername] = `"ops@corp"`
	if got := verifyAccountIdentity(t, separatorUser); got == first {
		t.Fatalf("quoted and literal usernames collide on identity %q", got)
	}
}

func TestVerifyCredentialsRejectsInvalidValuesWithoutProbe(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"missing host", func(v map[string]string) { delete(v, fieldHost) }},
		{"missing port", func(v map[string]string) { delete(v, fieldPort) }},
		{"missing username", func(v map[string]string) { delete(v, fieldUsername) }},
		{"missing key", func(v map[string]string) { delete(v, fieldPrivateKey) }},
		{"missing fingerprint", func(v map[string]string) { delete(v, fieldHostKeySHA256) }},
		{"blank key", func(v map[string]string) { v[fieldPrivateKey] = "  \n" }},
		{"non-canonical port", func(v map[string]string) { v[fieldPort] = "022" }},
		{"out-of-range port", func(v map[string]string) { v[fieldPort] = "65536" }},
		{"extra command", func(v map[string]string) { v["command"] = "id" }},
		{"fingerprint without prefix", func(v map[string]string) { v[fieldHostKeySHA256] = "AAAA" }},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			fake := &fakeService{}
			client := newTestClient(t, fake)
			values := testValues()
			testCase.mutate(values)

			_, err := client.VerifyCredentials(context.Background(), providers.CredentialInput{Values: values})
			if code := failureOf(t, err).Code; code != core.ErrorCodeInvalidInput {
				t.Fatalf("code = %q, want %q", code, core.ErrorCodeInvalidInput)
			}
			if fake.totalCalls() != 0 {
				t.Fatalf("invalid credential reached the capability: %d calls", fake.totalCalls())
			}
		})
	}
}

func TestInvokeCredentialsRejectsInvalidScopeWithoutCapabilityCall(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(*providers.ResourceScope)
	}{
		{"nil organization", func(s *providers.ResourceScope) { s.OrganizationID = uuid.Nil }},
		{"nil owner", func(s *providers.ResourceScope) { s.OwnerAccountID = uuid.Nil }},
		{"nil connection", func(s *providers.ResourceScope) { s.ConnectionID = uuid.Nil }},
		{"zero credential version", func(s *providers.ResourceScope) { s.CredentialVersion = 0 }},
		{"nil binding", func(s *providers.ResourceScope) { s.BindingID = uuid.Nil }},
		{"zero binding revision", func(s *providers.ResourceScope) { s.BindingRevision = 0 }},
		{"empty logical operation", func(s *providers.ResourceScope) { s.LogicalOperationID = "" }},
		{"blank logical operation", func(s *providers.ResourceScope) { s.LogicalOperationID = "   " }},
		{"chat target", func(s *providers.ResourceScope) {
			s.Target = core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}
		}},
		{"digital human without instance", func(s *providers.ResourceScope) {
			s.Target = core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: core.ScopeDraft}
		}},
		{"published without version", func(s *providers.ResourceScope) {
			s.Target = core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: core.ScopePublished, BindingInstanceID: uuid.NewString()}
		}},
		{"empty target id", func(s *providers.ResourceScope) {
			s.Target = core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: "  ", Scope: core.ScopeDraft, BindingInstanceID: uuid.NewString()}
		}},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			fake := &fakeService{}
			client := newTestClient(t, fake)
			scope := testScope()
			testCase.mutate(&scope)

			_, err := client.InvokeCredentials(context.Background(), serverops.ActionSystemSnapshot,
				providers.CredentialInput{Values: testValues(), ResourceScope: scope}, nil)
			if code := failureOf(t, err).Code; code != core.ErrorCodeInvalidInput {
				t.Fatalf("code = %q, want %q", code, core.ErrorCodeInvalidInput)
			}
			if fake.totalCalls() != 0 {
				t.Fatalf("invalid scope reached the capability: %d calls", fake.totalCalls())
			}
		})
	}
}

func TestInvokeCredentialsRejectsUnknownOperationAndInvalidArgumentsWithoutCapabilityCall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		operation string
		args      map[string]any
	}{
		{"unknown operation", "server.command.run", nil},
		{"snapshot with extra command", serverops.ActionSystemSnapshot, map[string]any{"command": "id"}},
		{"snapshot with empty extra key", serverops.ActionSystemSnapshot, map[string]any{"": "x"}},
		{"status missing service name", serverops.ActionServiceStatus, nil},
		{"status empty service name", serverops.ActionServiceStatus, map[string]any{fieldServiceName: ""}},
		{"status bare unit name", serverops.ActionServiceStatus, map[string]any{fieldServiceName: "nginx"}},
		{"status shell injection", serverops.ActionServiceStatus, map[string]any{fieldServiceName: "nginx.service; rm -rf /"}},
		{"status extra command", serverops.ActionServiceStatus, map[string]any{fieldServiceName: "nginx.service", "command": "id"}},
		{"journal missing service name", serverops.ActionJournalTail, nil},
		{"journal lines zero", serverops.ActionJournalTail, map[string]any{fieldServiceName: "nginx.service", fieldLines: 0}},
		{"journal lines above cap", serverops.ActionJournalTail, map[string]any{fieldServiceName: "nginx.service", fieldLines: 201}},
		{"journal lines wrong type", serverops.ActionJournalTail, map[string]any{fieldServiceName: "nginx.service", fieldLines: "100"}},
		{"journal since zero", serverops.ActionJournalTail, map[string]any{fieldServiceName: "nginx.service", fieldSinceMinutes: 0}},
		{"journal since above cap", serverops.ActionJournalTail, map[string]any{fieldServiceName: "nginx.service", fieldSinceMinutes: 1441}},
		{"journal extra key", serverops.ActionJournalTail, map[string]any{fieldServiceName: "nginx.service", "command": "id"}},
		{"restart missing service name", serverops.ActionServiceRestart, nil},
		{"restart bare unit name", serverops.ActionServiceRestart, map[string]any{fieldServiceName: "nginx"}},
		{"start shell injection", serverops.ActionServiceStart, map[string]any{fieldServiceName: "nginx.service; reboot"}},
		{"stop extra verb", serverops.ActionServiceStop, map[string]any{fieldServiceName: "nginx.service", "verb": "mask"}},
		{"reload extra privilege", serverops.ActionServiceReload, map[string]any{fieldServiceName: "nginx.service", fieldPrivilege: "sudo"}},
		{"unknown write action", "server.service.enable", map[string]any{fieldServiceName: "nginx.service"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := &fakeService{}
			client := newTestClient(t, fake)

			_, err := client.InvokeCredentials(context.Background(), testCase.operation,
				providers.CredentialInput{Values: testValues(), ResourceScope: testScope()}, testCase.args)
			if code := failureOf(t, err).Code; code != core.ErrorCodeInvalidInput {
				t.Fatalf("code = %q, want %q", code, core.ErrorCodeInvalidInput)
			}
			if fake.totalCalls() != 0 {
				t.Fatalf("invalid argument reached the capability: %d calls", fake.totalCalls())
			}
		})
	}
}

func TestInvokeCredentialsSystemSnapshotMapsOutputSchema(t *testing.T) {
	t.Parallel()

	fake := &fakeService{snapshot: &serverops.SystemSnapshot{
		Attempt:         serverops.Attempt{RequestID: "req-snapshot", AttemptCount: 1},
		Hostname:        "remote-host",
		OperatingSystem: "Linux",
		KernelVersion:   "6.1.0",
		UptimeSeconds:   12345,
		LoadAverage1m:   0.5,
		Memory:          serverops.MemoryInfo{TotalBytes: 1000, AvailableBytes: 400, UsedBytes: 600},
		Disks: []serverops.DiskInfo{{
			MountPoint: "/", Filesystem: "/dev/vda1", TotalBytes: 2000, AvailableBytes: 500, UsedBytes: 1500,
		}},
	}}
	client := newTestClient(t, fake)

	response, err := client.InvokeCredentials(context.Background(), serverops.ActionSystemSnapshot,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()}, nil)
	if err != nil {
		t.Fatalf("InvokeCredentials() error = %v", err)
	}
	if fake.snapshotCalls != 1 || fake.totalCalls() != 1 {
		t.Fatalf("snapshot calls = %d", fake.snapshotCalls)
	}
	if response.RequestID != "req-snapshot" {
		t.Fatalf("RequestID = %q", response.RequestID)
	}
	want := map[string]any{
		"provider":         catalog.ServerOpsSSHConnectorID,
		"operation":        serverops.ActionSystemSnapshot,
		"hostname":         "remote-host",
		"operating_system": "Linux",
		"kernel_version":   "6.1.0",
		"uptime_seconds":   int64(12345),
		"load_average_1m":  0.5,
		"memory": map[string]any{
			"total_bytes":     int64(1000),
			"available_bytes": int64(400),
			"used_bytes":      int64(600),
		},
		"disks": []any{map[string]any{
			"filesystem":      "/dev/vda1",
			"mount_point":     "/",
			"total_bytes":     int64(2000),
			"available_bytes": int64(500),
			"used_bytes":      int64(1500),
		}},
	}
	if !reflect.DeepEqual(response.Value, want) {
		t.Fatalf("Value = %#v, want %#v", response.Value, want)
	}
}

func TestInvokeCredentialsServiceStatusMapsOutputSchema(t *testing.T) {
	t.Parallel()

	fake := &fakeService{status: &serverops.ServiceStatus{
		Attempt:     serverops.Attempt{RequestID: "req-status", AttemptCount: 1},
		ServiceName: "nginx.service",
		LoadState:   "loaded",
		ActiveState: "active",
		SubState:    "running",
		Description: "nginx web server",
	}}
	client := newTestClient(t, fake)

	response, err := client.InvokeCredentials(context.Background(), serverops.ActionServiceStatus,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"})
	if err != nil {
		t.Fatalf("InvokeCredentials() error = %v", err)
	}
	if fake.lastServiceName != "nginx.service" {
		t.Fatalf("service name = %q", fake.lastServiceName)
	}
	want := map[string]any{
		"provider":     catalog.ServerOpsSSHConnectorID,
		"operation":    serverops.ActionServiceStatus,
		"service_name": "nginx.service",
		"load_state":   "loaded",
		"active_state": "active",
		"sub_state":    "running",
		"description":  "nginx web server",
	}
	if !reflect.DeepEqual(response.Value, want) {
		t.Fatalf("Value = %#v, want %#v", response.Value, want)
	}
	if response.RequestID != "req-status" {
		t.Fatalf("RequestID = %q", response.RequestID)
	}
}

func TestInvokeCredentialsJournalTailMapsOutputSchemaAndAppliesDefaults(t *testing.T) {
	t.Parallel()

	fake := &fakeService{journal: &serverops.JournalTail{
		Attempt:     serverops.Attempt{RequestID: "req-journal", AttemptCount: 1},
		ServiceName: "nginx.service",
		Lines:       []string{"line-1", "line-2"},
		Truncated:   true,
	}}
	client := newTestClient(t, fake)

	response, err := client.InvokeCredentials(context.Background(), serverops.ActionJournalTail,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"})
	if err != nil {
		t.Fatalf("InvokeCredentials() error = %v", err)
	}
	if fake.lastOptions != (serverops.JournalOptions{Lines: 100, SinceMinutes: 60}) {
		t.Fatalf("options = %+v, want catalog defaults 100/60", fake.lastOptions)
	}
	want := map[string]any{
		"provider":     catalog.ServerOpsSSHConnectorID,
		"operation":    serverops.ActionJournalTail,
		"service_name": "nginx.service",
		"lines":        []any{"line-1", "line-2"},
		"truncated":    true,
	}
	if !reflect.DeepEqual(response.Value, want) {
		t.Fatalf("Value = %#v, want %#v", response.Value, want)
	}
	if response.RequestID != "req-journal" {
		t.Fatalf("RequestID = %q", response.RequestID)
	}

	// An explicit window is passed through exactly.
	fake.journalCalls = 0
	if _, err = client.InvokeCredentials(context.Background(), serverops.ActionJournalTail,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service", fieldLines: 5, fieldSinceMinutes: 10}); err != nil {
		t.Fatalf("explicit InvokeCredentials() error = %v", err)
	}
	if fake.lastOptions != (serverops.JournalOptions{Lines: 5, SinceMinutes: 10}) {
		t.Fatalf("explicit options = %+v", fake.lastOptions)
	}
}

func TestInvokeCredentialsClassifiesCapabilityErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		code      error
		want      core.ErrorCode
		retryable bool
	}{
		{"invalid input", serverops.ErrInvalidInput, core.ErrorCodeInvalidInput, false},
		{"connection invalid", serverops.ErrConnectionInvalid, core.ErrorCodeInvalidAppCredentials, false},
		{"auth invalid", serverops.ErrAuthInvalid, core.ErrorCodeInvalidAppCredentials, false},
		{"provider rejected", serverops.ErrProviderRejected, core.ErrorCodeProviderUnavailable, false},
		{"response invalid", serverops.ErrResponseInvalid, core.ErrorCodeProviderResponseUnrecognized, false},
		{"timeout", serverops.ErrTimeout, core.ErrorCodeProviderUnavailable, true},
		{"upstream", serverops.ErrUpstream, core.ErrorCodeProviderUnavailable, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fake := &fakeService{snapshotErr: &serverops.Error{Code: testCase.code, RequestID: "req-class", AttemptCount: 1}}
			client := newTestClient(t, fake)

			_, err := client.InvokeCredentials(context.Background(), serverops.ActionSystemSnapshot,
				providers.CredentialInput{Values: testValues(), ResourceScope: testScope()}, nil)
			failure := failureOf(t, err)
			if failure.Code != testCase.want || failure.Retryable != testCase.retryable {
				t.Fatalf("failure = %+v, want code=%q retryable=%v", failure, testCase.want, testCase.retryable)
			}
			if failure.RequestID != "req-class" {
				t.Fatalf("RequestID = %q", failure.RequestID)
			}
			if strings.Contains(err.Error(), testPassphrase) || strings.Contains(err.Error(), testPrivateKey()) {
				t.Fatalf("error leaked a credential: %q", err.Error())
			}
		})
	}

	// An unstructured cause is classified upstream-only and never surfaced.
	t.Run("unstructured cause", func(t *testing.T) {
		fake := &fakeService{snapshotErr: fmt.Errorf("remote rejected: %s / %s", testPassphrase, testPrivateKey())}
		client := newTestClient(t, fake)
		_, err := client.InvokeCredentials(context.Background(), serverops.ActionSystemSnapshot,
			providers.CredentialInput{Values: testValues(), ResourceScope: testScope()}, nil)
		failure := failureOf(t, err)
		if failure.Code != core.ErrorCodeProviderUnavailable {
			t.Fatalf("code = %q, want %q", failure.Code, core.ErrorCodeProviderUnavailable)
		}
		if strings.Contains(err.Error(), testPassphrase) || strings.Contains(err.Error(), testPrivateKey()) {
			t.Fatalf("error leaked a credential: %q", err.Error())
		}
	})
}

func TestVerifyCredentialsClassifiesProbeErrors(t *testing.T) {
	t.Parallel()

	fake := &fakeService{validateErr: &serverops.Error{Code: serverops.ErrAuthInvalid, RequestID: "req-probe", AttemptCount: 1}}
	client := newTestClient(t, fake)

	_, err := client.VerifyCredentials(context.Background(), providers.CredentialInput{Values: testValues()})
	failure := failureOf(t, err)
	if failure.Code != core.ErrorCodeInvalidAppCredentials || failure.RequestID != "req-probe" {
		t.Fatalf("failure = %+v", failure)
	}
	if strings.Contains(err.Error(), testPassphrase) {
		t.Fatalf("probe error leaked a credential: %q", err.Error())
	}
}

func TestInvokeCredentialsRejectsNilCapabilityResults(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, &fakeService{})
	for _, operation := range []string{serverops.ActionSystemSnapshot, serverops.ActionServiceStatus, serverops.ActionJournalTail} {
		args := map[string]any{}
		if operation != serverops.ActionSystemSnapshot {
			args[fieldServiceName] = "nginx.service"
		}
		_, err := client.InvokeCredentials(context.Background(), operation,
			providers.CredentialInput{Values: testValues(), ResourceScope: testScope()}, args)
		if code := failureOf(t, err).Code; code != core.ErrorCodeProviderResponseUnrecognized {
			t.Fatalf("%s code = %q, want %q", operation, code, core.ErrorCodeProviderResponseUnrecognized)
		}
	}
}

func TestInvokeCredentialsDoesNotLeakSecrets(t *testing.T) {
	t.Parallel()

	secretComment := "note " + testPrivateKey() + " and " + testPassphrase
	fake := &fakeService{
		snapshot: &serverops.SystemSnapshot{
			Attempt:         serverops.Attempt{RequestID: "req-leak", AttemptCount: 1},
			Hostname:        "host-" + testPassphrase,
			OperatingSystem: "Linux " + testPrivateKey(),
			KernelVersion:   "6.1.0",
			Memory:          serverops.MemoryInfo{},
			Disks:           []serverops.DiskInfo{{MountPoint: "/", Filesystem: secretComment}},
		},
		journal: &serverops.JournalTail{
			Attempt:     serverops.Attempt{RequestID: "req-leak", AttemptCount: 1},
			ServiceName: "nginx.service",
			Lines:       []string{secretComment, "ok"},
		},
	}
	client := newTestClient(t, fake)

	response, err := client.InvokeCredentials(context.Background(), serverops.ActionSystemSnapshot,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()}, nil)
	if err != nil {
		t.Fatalf("snapshot InvokeCredentials() error = %v", err)
	}
	assertNoSecrets(t, response.Value)

	journal, err := client.InvokeCredentials(context.Background(), serverops.ActionJournalTail,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"})
	if err != nil {
		t.Fatalf("journal InvokeCredentials() error = %v", err)
	}
	assertNoSecrets(t, journal.Value)

	// The verification display hostname is redacted as well.
	probe := &fakeService{profile: &serverops.ValidationProfile{
		Attempt:  serverops.Attempt{RequestID: "req-verify", AttemptCount: 1},
		Hostname: "remote-" + testPassphrase,
	}}
	verification, err := newTestClient(t, probe).VerifyCredentials(context.Background(), providers.CredentialInput{Values: testValues()})
	if err != nil {
		t.Fatalf("VerifyCredentials() error = %v", err)
	}
	if strings.Contains(verification.DisplayName, testPassphrase) || !strings.Contains(verification.DisplayName, "[redacted]") {
		t.Fatalf("DisplayName = %q", verification.DisplayName)
	}
}

// assertNoSecrets rejects any credential material in any string of a response
// value. It walks the value directly because a multiline key would be escaped
// by a JSON or %#v rendering and slip past a substring check.
func assertNoSecrets(t *testing.T, value any) {
	t.Helper()
	switch typed := value.(type) {
	case string:
		if strings.Contains(typed, testPassphrase) || strings.Contains(typed, testPrivateKey()) {
			t.Fatalf("response leaked a credential: %q", typed)
		}
	case []any:
		for _, item := range typed {
			assertNoSecrets(t, item)
		}
	case map[string]any:
		for _, item := range typed {
			assertNoSecrets(t, item)
		}
	}
}

func TestInvokeCredentialsZeroesTemporarySecretCopies(t *testing.T) {
	t.Parallel()

	fake := &fakeService{status: &serverops.ServiceStatus{
		Attempt:     serverops.Attempt{RequestID: "req-zero", AttemptCount: 1},
		ServiceName: "nginx.service",
		LoadState:   "loaded",
		ActiveState: "active",
		SubState:    "running",
		Description: "nginx",
	}}
	client := newTestClient(t, fake)

	if _, err := client.InvokeCredentials(context.Background(), serverops.ActionServiceStatus,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"}); err != nil {
		t.Fatalf("InvokeCredentials() error = %v", err)
	}
	if fake.observedPrivateKey != testPrivateKey() || fake.observedPassphrase != testPassphrase {
		t.Fatal("capability did not receive the submitted secret material")
	}
	if !allZero(fake.lastConnection.PrivateKey) || !allZero(fake.lastConnection.Passphrase) {
		t.Fatal("temporary secret copies were not zeroed after the capability call")
	}
}

func TestVerifyCredentialsRecordsSudoPrivilegeEvidence(t *testing.T) {
	t.Parallel()

	values := testValues()
	values[fieldPrivilege] = "sudo"
	fake := &fakeService{profile: &serverops.ValidationProfile{
		Attempt:  serverops.Attempt{RequestID: "req-sudo", AttemptCount: 1},
		Hostname: "remote-host.internal",
	}}
	verification, err := newTestClient(t, fake).VerifyCredentials(context.Background(), providers.CredentialInput{Values: values})
	if err != nil {
		t.Fatalf("VerifyCredentials() error = %v", err)
	}
	if verification.Evidence == nil || verification.Evidence.ServerOps == nil ||
		verification.Evidence.ServerOps.Privilege != "sudo" || verification.Evidence.ServerOps.Identity != verification.AccountID {
		t.Fatalf("Evidence = %#v", verification.Evidence)
	}
	// The privilege never changes the stable account identity.
	if verification.AccountID != "deploy@server.example.internal:22" || fake.lastConnection.Privilege != serverops.PrivilegeSudo {
		t.Fatalf("AccountID = %q, privilege = %q", verification.AccountID, fake.lastConnection.Privilege)
	}

	for _, privilege := range []string{"", "root", "SUDO"} {
		invalid := testValues()
		invalid[fieldPrivilege] = privilege
		probe := &fakeService{}
		_, err := newTestClient(t, probe).VerifyCredentials(context.Background(), providers.CredentialInput{Values: invalid})
		if code := failureOf(t, err).Code; code != core.ErrorCodeInvalidInput || probe.totalCalls() != 0 {
			t.Fatalf("privilege %q: code = %q, calls = %d", privilege, code, probe.totalCalls())
		}
	}
}

func TestInvokeCredentialsServiceWriteActionsMapOutputSchema(t *testing.T) {
	t.Parallel()

	for _, action := range []string{
		serverops.ActionServiceRestart, serverops.ActionServiceStart, serverops.ActionServiceStop, serverops.ActionServiceReload,
	} {
		for _, privilege := range []string{"", "sudo"} {
			values := testValues()
			if privilege != "" {
				values[fieldPrivilege] = privilege
			}
			fake := &fakeService{change: &serverops.ServiceChange{
				Attempt:     serverops.Attempt{RequestID: "req-change", AttemptCount: 1},
				Action:      action,
				ServiceName: "nginx.service",
				LoadState:   "loaded",
				ActiveState: "active",
				SubState:    "running",
			}}
			response, err := newTestClient(t, fake).InvokeCredentials(context.Background(), action,
				providers.CredentialInput{Values: values, ResourceScope: testScope()},
				map[string]any{fieldServiceName: "nginx.service"})
			if err != nil {
				t.Fatalf("%s/%q: InvokeCredentials() error = %v", action, privilege, err)
			}
			if fake.changeCalls != 1 || fake.totalCalls() != 1 || fake.lastAction != action || fake.lastServiceName != "nginx.service" {
				t.Fatalf("%s: capability calls = %d/%d action=%q unit=%q", action, fake.changeCalls, fake.totalCalls(), fake.lastAction, fake.lastServiceName)
			}
			if string(fake.lastConnection.Privilege) != privilege {
				t.Fatalf("%s: privilege = %q, want %q", action, fake.lastConnection.Privilege, privilege)
			}
			want := map[string]any{
				"provider":     catalog.ServerOpsSSHConnectorID,
				"operation":    action,
				"service_name": "nginx.service",
				"load_state":   "loaded",
				"active_state": "active",
				"sub_state":    "running",
			}
			if !reflect.DeepEqual(response.Value, want) || response.RequestID != "req-change" {
				t.Fatalf("%s: response = %#v", action, response)
			}
		}
	}
}

func TestInvokeCredentialsServiceWriteFailuresAreClassified(t *testing.T) {
	t.Parallel()

	rejected := &serverops.Error{Code: serverops.ErrProviderRejected, RequestID: "req-rejected", AttemptCount: 1}
	fake := &fakeService{changeErr: rejected}
	_, err := newTestClient(t, fake).InvokeCredentials(context.Background(), serverops.ActionServiceRestart,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"})
	failure := failureOf(t, err)
	if failure.Code != core.ErrorCodeProviderUnavailable || failure.Retryable || failure.RequestID != "req-rejected" {
		t.Fatalf("failure = %#v", failure)
	}

	// A result for another action is never mapped onto this operation.
	mismatched := &fakeService{change: &serverops.ServiceChange{
		Action: serverops.ActionServiceStop, ServiceName: "nginx.service",
		LoadState: "loaded", ActiveState: "inactive", SubState: "dead",
	}}
	_, err = newTestClient(t, mismatched).InvokeCredentials(context.Background(), serverops.ActionServiceStart,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"})
	if code := failureOf(t, err).Code; code != core.ErrorCodeProviderResponseUnrecognized {
		t.Fatalf("mismatched action code = %q", code)
	}
	nilResult := &fakeService{}
	_, err = newTestClient(t, nilResult).InvokeCredentials(context.Background(), serverops.ActionServiceReload,
		providers.CredentialInput{Values: testValues(), ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"})
	if code := failureOf(t, err).Code; code != core.ErrorCodeProviderResponseUnrecognized {
		t.Fatalf("nil result code = %q", code)
	}
}

func TestInvokeCredentialsReadActionsIgnoreSudoPrivilegeSelection(t *testing.T) {
	t.Parallel()

	// The connection carries the privilege, but the capability decides that
	// read actions never use it; the adapter only passes the connection along.
	values := testValues()
	values[fieldPrivilege] = "sudo"
	fake := &fakeService{status: &serverops.ServiceStatus{
		Attempt: serverops.Attempt{RequestID: "req-status", AttemptCount: 1}, ServiceName: "nginx.service",
		LoadState: "loaded", ActiveState: "active", SubState: "running", Description: "nginx",
	}}
	if _, err := newTestClient(t, fake).InvokeCredentials(context.Background(), serverops.ActionServiceStatus,
		providers.CredentialInput{Values: values, ResourceScope: testScope()},
		map[string]any{fieldServiceName: "nginx.service"}); err != nil {
		t.Fatalf("InvokeCredentials() error = %v", err)
	}
	if fake.statusCalls != 1 || fake.changeCalls != 0 {
		t.Fatalf("calls = status:%d change:%d", fake.statusCalls, fake.changeCalls)
	}
}
