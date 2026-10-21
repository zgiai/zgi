package serveropsssh

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// probeProfile builds a secret-free capture profile for the probe tests.
func probeProfile(requestID, hostname, fingerprint string) *serverops.ValidationProfile {
	return &serverops.ValidationProfile{
		Attempt:            serverops.Attempt{RequestID: requestID, AttemptCount: 1},
		Hostname:           hostname,
		HostKeyFingerprint: fingerprint,
	}
}

// TestProbeHostKeyCapturesWhenFingerprintAbsent pins the discovery path: an
// omitted fingerprint opts the request-scoped Connection into capture mode with
// an empty pinned value, and the observed canonical fingerprint is returned.
func TestProbeHostKeyCapturesWhenFingerprintAbsent(t *testing.T) {
	t.Parallel()

	values := testValues()
	delete(values, fieldHostKeySHA256)
	fake := &fakeService{profile: probeProfile("req-capture", "remote-host.internal", testFingerprint())}
	client := newTestClient(t, fake)

	observation, err := client.ProbeHostKey(context.Background(), values)
	if err != nil {
		t.Fatalf("ProbeHostKey() error = %v", err)
	}
	if fake.validateCalls != 1 || fake.totalCalls() != 1 {
		t.Fatalf("capability calls = validate:%d total:%d, want a single validate probe", fake.validateCalls, fake.totalCalls())
	}
	if !fake.lastConnection.CaptureHostKey || fake.lastConnection.HostKeySHA256 != "" {
		t.Fatalf("capture connection = %+v", fake.lastConnection)
	}
	if observation.Fingerprint != testFingerprint() || observation.Hostname != "remote-host.internal" || observation.RequestID != "req-capture" {
		t.Fatalf("observation = %+v", observation)
	}
	if fake.observedPrivateKey != testPrivateKey() || fake.observedPassphrase != testPassphrase {
		t.Fatal("probe did not receive the submitted secret material")
	}
	if !allZero(fake.lastConnection.PrivateKey) || !allZero(fake.lastConnection.Passphrase) {
		t.Fatal("temporary secret copies were not zeroed after the probe")
	}
}

// TestProbeHostKeyUsesStrictModeWhenFingerprintSupplied pins the verification
// path: a supplied fingerprint pins the Connection and is compared instead of
// being captured, so no observed fingerprint is surfaced.
func TestProbeHostKeyUsesStrictModeWhenFingerprintSupplied(t *testing.T) {
	t.Parallel()

	fake := &fakeService{profile: probeProfile("req-strict", "remote-host.internal", "")}
	client := newTestClient(t, fake)

	observation, err := client.ProbeHostKey(context.Background(), testValues())
	if err != nil {
		t.Fatalf("ProbeHostKey() error = %v", err)
	}
	if fake.lastConnection.CaptureHostKey || fake.lastConnection.HostKeySHA256 != testFingerprint() {
		t.Fatalf("strict connection = %+v", fake.lastConnection)
	}
	if observation.Fingerprint != "" {
		t.Fatalf("strict observation surfaced a fingerprint: %+v", observation)
	}
	if observation.RequestID != "req-strict" || observation.Hostname != "remote-host.internal" {
		t.Fatalf("observation = %+v", observation)
	}
}

func TestProbeHostKeySupportsPasswordAuthentication(t *testing.T) {
	t.Parallel()

	values := testValues()
	delete(values, fieldPrivateKey)
	delete(values, fieldPassphrase)
	delete(values, fieldHostKeySHA256)
	values[fieldPassword] = "server-password"
	fake := &fakeService{profile: probeProfile("req-password", "remote-host.internal", testFingerprint())}

	observation, err := newTestClient(t, fake).ProbeHostKey(context.Background(), values)
	if err != nil {
		t.Fatalf("ProbeHostKey() error = %v", err)
	}
	if observation.Fingerprint != testFingerprint() || fake.observedPassword != "server-password" {
		t.Fatalf("observation = %+v, observed password = %q", observation, fake.observedPassword)
	}
	if !allZero(fake.lastConnection.Password) || len(fake.lastConnection.PrivateKey) != 0 {
		t.Fatal("temporary password material was not zeroed or private key was unexpectedly set")
	}
}

// TestProbeHostKeyPassphraseIsOptionalByOmission pins the documented
// unencrypted-key form: an absent passphrase builds a key-only Connection.
func TestProbeHostKeyPassphraseIsOptionalByOmission(t *testing.T) {
	t.Parallel()

	values := testValues()
	delete(values, fieldPassphrase)
	fake := &fakeService{profile: probeProfile("req-nopass", "remote-host.internal", testFingerprint())}
	client := newTestClient(t, fake)

	if _, err := client.ProbeHostKey(context.Background(), values); err != nil {
		t.Fatalf("ProbeHostKey() error = %v", err)
	}
	if fake.observedPassphrase != "" {
		t.Fatalf("omitted passphrase reached the capability as %q", fake.observedPassphrase)
	}
	if fake.lastConnection.CaptureHostKey {
		t.Fatal("supplied fingerprint was treated as capture")
	}
}

// TestProbeHostKeyRejectsInvalidValuesWithoutProbe pins fail-closed input
// validation: unknown fields, blank optional supplied values, and malformed
// target/port/key/fingerprint are rejected before the capability is called.
func TestProbeHostKeyRejectsInvalidValuesWithoutProbe(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"missing host", func(v map[string]string) { delete(v, fieldHost) }},
		{"missing port", func(v map[string]string) { delete(v, fieldPort) }},
		{"missing username", func(v map[string]string) { delete(v, fieldUsername) }},
		{"missing key", func(v map[string]string) { delete(v, fieldPrivateKey) }},
		{"blank key", func(v map[string]string) { v[fieldPrivateKey] = "  \n" }},
		{"host with surrounding space", func(v map[string]string) { v[fieldHost] = " server.example.internal" }},
		{"non-canonical port", func(v map[string]string) { v[fieldPort] = "022" }},
		{"out-of-range port", func(v map[string]string) { v[fieldPort] = "65536" }},
		{"extra command", func(v map[string]string) { v["command"] = "id" }},
		{"extra target", func(v map[string]string) { v["target"] = "agent-1" }},
		{"blank supplied passphrase", func(v map[string]string) { v[fieldPassphrase] = "" }},
		{"whitespace supplied passphrase", func(v map[string]string) { v[fieldPassphrase] = "  " }},
		{"blank supplied fingerprint", func(v map[string]string) { v[fieldHostKeySHA256] = "" }},
		{"fingerprint without prefix", func(v map[string]string) { v[fieldHostKeySHA256] = "AAAA" }},
		{"fingerprint with bad digest", func(v map[string]string) { v[fieldHostKeySHA256] = "SHA256:AAAA" }},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			fake := &fakeService{}
			client := newTestClient(t, fake)
			values := testValues()
			testCase.mutate(values)

			_, err := client.ProbeHostKey(context.Background(), values)
			if code := failureOf(t, err).Code; code != core.ErrorCodeInvalidInput {
				t.Fatalf("code = %q, want %q", code, core.ErrorCodeInvalidInput)
			}
			if fake.totalCalls() != 0 {
				t.Fatalf("invalid probe value reached the capability: %d calls", fake.totalCalls())
			}
		})
	}
}

// TestProbeHostKeyCaptureRequiresObservedFingerprint pins the fail-closed
// discovery result: a capture probe whose profile carries no fingerprint is a
// provider response failure, never a successful empty observation.
func TestProbeHostKeyCaptureRequiresObservedFingerprint(t *testing.T) {
	t.Parallel()

	values := testValues()
	delete(values, fieldHostKeySHA256)
	fake := &fakeService{profile: probeProfile("req-empty", "remote-host.internal", "")}
	client := newTestClient(t, fake)

	observation, err := client.ProbeHostKey(context.Background(), values)
	failure := failureOf(t, err)
	if failure.Code != core.ErrorCodeProviderResponseUnrecognized {
		t.Fatalf("code = %q, want %q", failure.Code, core.ErrorCodeProviderResponseUnrecognized)
	}
	if observation != (HostKeyObservation{}) {
		t.Fatalf("failed capture returned an observation: %+v", observation)
	}
}

// TestProbeHostKeyRedactsAndBoundsHostname pins the display boundary: a remote
// hostname that echoes a submitted secret is redacted, and every hostname is
// bounded before it can leave the adapter.
func TestProbeHostKeyRedactsAndBoundsHostname(t *testing.T) {
	t.Parallel()

	fake := &fakeService{profile: probeProfile("req-redact", "remote-"+testPassphrase+"-"+testPrivateKey(), testFingerprint())}
	client := newTestClient(t, fake)
	values := testValues()
	delete(values, fieldHostKeySHA256)

	observation, err := client.ProbeHostKey(context.Background(), values)
	if err != nil {
		t.Fatalf("ProbeHostKey() error = %v", err)
	}
	if strings.Contains(observation.Hostname, testPassphrase) || strings.Contains(observation.Hostname, testPrivateKey()) {
		t.Fatalf("hostname leaked a credential: %q", observation.Hostname)
	}
	if !strings.Contains(observation.Hostname, "[redacted]") {
		t.Fatalf("hostname was not redacted: %q", observation.Hostname)
	}

	bounded := &fakeService{profile: probeProfile("req-bound", strings.Repeat("h", 400), testFingerprint())}
	boundedObservation, err := newTestClient(t, bounded).ProbeHostKey(context.Background(), values)
	if err != nil {
		t.Fatalf("bounded ProbeHostKey() error = %v", err)
	}
	if length := len([]rune(boundedObservation.Hostname)); length != 255 {
		t.Fatalf("bounded hostname length = %d, want 255", length)
	}
}

// TestProbeHostKeyDoesNotLeakSecretsInErrors pins the error boundary: neither a
// classified capability error nor an unstructured cause can echo credential
// material or provider error text.
func TestProbeHostKeyDoesNotLeakSecretsInErrors(t *testing.T) {
	t.Parallel()

	classified := &fakeService{validateErr: &serverops.Error{Code: serverops.ErrAuthInvalid, RequestID: "req-probe", AttemptCount: 1}}
	failure := failureOf(t, mustProbeError(t, classified))
	if failure.Code != core.ErrorCodeInvalidAppCredentials || failure.RequestID != "req-probe" {
		t.Fatalf("classified failure = %+v", failure)
	}

	unstructured := &fakeService{validateErr: fmt.Errorf("remote rejected %s / %s", testPassphrase, testPrivateKey())}
	failure = failureOf(t, mustProbeError(t, unstructured))
	if failure.Code != core.ErrorCodeProviderUnavailable {
		t.Fatalf("unstructured failure = %+v", failure)
	}

	for name, fake := range map[string]*fakeService{"classified": classified, "unstructured": unstructured} {
		_, err := newTestClient(t, fake).ProbeHostKey(context.Background(), testValues())
		if strings.Contains(err.Error(), testPassphrase) || strings.Contains(err.Error(), testPrivateKey()) {
			t.Fatalf("%s error leaked a credential: %q", name, err.Error())
		}
	}
}

func mustProbeError(t *testing.T, fake *fakeService) error {
	t.Helper()
	_, err := newTestClient(t, fake).ProbeHostKey(context.Background(), testValues())
	if err == nil {
		t.Fatal("ProbeHostKey() succeeded, want an error")
	}
	return err
}

// TestProbeHostKeySanitizesRequestID pins the non-secret request-id boundary: an
// unsafe request id from the capability is dropped rather than surfaced.
func TestProbeHostKeySanitizesRequestID(t *testing.T) {
	t.Parallel()

	fake := &fakeService{profile: probeProfile("unsafe request id!", "remote-host.internal", testFingerprint())}
	client := newTestClient(t, fake)
	values := testValues()
	delete(values, fieldHostKeySHA256)

	observation, err := client.ProbeHostKey(context.Background(), values)
	if err != nil {
		t.Fatalf("ProbeHostKey() error = %v", err)
	}
	if observation.RequestID != "" {
		t.Fatalf("unsafe request id surfaced: %q", observation.RequestID)
	}
}
