package serverops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	serviceTestRequestID  = "req-serverops-capability-test"
	serviceTestPassphrase = "service-test-passphrase"
)

// serviceFakeRunner records the exact trusted command it received and returns a
// canned result. It never dials, spawns, or touches the network.
type serviceFakeRunner struct {
	mu      sync.Mutex
	calls   int
	command string
	result  sshRunResult
	err     error
}

func (r *serviceFakeRunner) run(_ context.Context, trustedCommand string) (sshRunResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.command = trustedCommand
	return r.result, r.err
}

func (r *serviceFakeRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *serviceFakeRunner) observedCommand() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.command
}

// serviceFakeFactory is the private synchronous factory seam double. It copies
// the secret slices synchronously so assertions survive the service's zeroing.
type serviceFakeFactory struct {
	mu         sync.Mutex
	calls      int
	configs    []sshClientConfig
	runners    []*serviceFakeRunner
	factoryErr error
	result     sshRunResult
	runErr     error
}

func (f *serviceFakeFactory) newRunner(config sshClientConfig) (runner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	copied := config
	copied.PrivateKey = append([]byte(nil), config.PrivateKey...)
	copied.Passphrase = append([]byte(nil), config.Passphrase...)
	f.configs = append(f.configs, copied)
	if f.factoryErr != nil {
		return nil, f.factoryErr
	}
	created := &serviceFakeRunner{result: f.result, err: f.runErr}
	f.runners = append(f.runners, created)
	return created, nil
}

func (f *serviceFakeFactory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *serviceFakeFactory) configsSnapshot() []sshClientConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sshClientConfig(nil), f.configs...)
}

func (f *serviceFakeFactory) runnersSnapshot() []*serviceFakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*serviceFakeRunner(nil), f.runners...)
}

func newTestService(t *testing.T, limits Limits, factory runnerFactory, generate requestIDGenerator) *Service {
	t.Helper()
	normalized, err := normalizeLimits(limits)
	if err != nil {
		t.Fatalf("normalizeLimits: %v", err)
	}
	return newService(normalized, factory, generate)
}

func newFakeService(t *testing.T, factory *serviceFakeFactory) *Service {
	t.Helper()
	return newTestService(t, Limits{}, factory.newRunner, func() string { return serviceTestRequestID })
}

func serviceTestConnection() Connection {
	return Connection{
		Host:          "server.example.com",
		Port:          22,
		Username:      "operator",
		PrivateKey:    []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nservice-test-key\n-----END OPENSSH PRIVATE KEY-----"),
		HostKeySHA256: "SHA256:" + strings.Repeat("A", 43),
	}
}

// serviceTestCaptureConnection is the explicitly opted-in capture form of the
// canonical test connection: CaptureHostKey true and no pinned fingerprint.
func serviceTestCaptureConnection() Connection {
	connection := serviceTestConnection()
	connection.HostKeySHA256 = ""
	connection.CaptureHostKey = true
	return connection
}

func validSnapshotOutput() []byte {
	return readActionJoinLines(readActionValidSnapshotLines())
}

func TestNewNormalizesAndRejectsLimits(t *testing.T) {
	defaults, err := New(Limits{})
	if err != nil {
		t.Fatalf("New(Limits{}) error = %v", err)
	}
	if defaults.limits.ConnectTimeout != defaultConnectTimeout ||
		defaults.limits.CommandTimeout != defaultCommandTimeout ||
		defaults.limits.MaxOutputBytes != defaultMaxOutputBytes {
		t.Fatalf("default limits = %#v", defaults.limits)
	}

	exact, err := New(Limits{ConnectTimeout: maxConnectTimeout, CommandTimeout: maxCommandTimeout, MaxOutputBytes: maxOutputBytesCap})
	if err != nil {
		t.Fatalf("New(exact caps) error = %v", err)
	}
	if exact.limits.ConnectTimeout != maxConnectTimeout ||
		exact.limits.CommandTimeout != maxCommandTimeout ||
		exact.limits.MaxOutputBytes != maxOutputBytesCap {
		t.Fatalf("exact-cap limits = %#v", exact.limits)
	}

	rejected := []Limits{
		{ConnectTimeout: -time.Nanosecond},
		{CommandTimeout: -time.Nanosecond},
		{MaxOutputBytes: -1},
		{ConnectTimeout: maxConnectTimeout + time.Nanosecond},
		{CommandTimeout: maxCommandTimeout + time.Nanosecond},
		{MaxOutputBytes: maxOutputBytesCap + 1},
	}
	for index, limits := range rejected {
		if _, err := New(limits); err == nil {
			t.Errorf("New(rejected %d) accepted %#v", index, limits)
		}
	}
}

func TestServiceExportedMethodSetIsExact(t *testing.T) {
	want := map[string]bool{
		"SystemSnapshot":     true,
		"ServiceStatus":      true,
		"JournalTail":        true,
		"ChangeService":      true,
		"RunCommand":         true,
		"ValidateCredential": true,
		"ValidateConnection": true,
	}
	typ := reflect.TypeOf((*Service)(nil))
	if typ.NumMethod() != len(want) {
		t.Fatalf("*Service has %d exported methods, want %d", typ.NumMethod(), len(want))
	}
	for index := 0; index < typ.NumMethod(); index++ {
		method := typ.Method(index)
		if !want[method.Name] {
			t.Errorf("*Service exposes unexpected method %q", method.Name)
		}
	}
}

func TestSystemSnapshotReturnsTypedResultAndExactCommand(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newTestService(t, Limits{ConnectTimeout: 3 * time.Second, CommandTimeout: 4 * time.Second, MaxOutputBytes: 2048}, factory.newRunner, func() string { return serviceTestRequestID })
	connection := serviceTestConnection()

	snapshot, err := service.SystemSnapshot(context.Background(), connection)
	if err != nil {
		t.Fatalf("SystemSnapshot() error = %v", err)
	}
	if snapshot.RequestID != serviceTestRequestID || snapshot.AttemptCount != 1 {
		t.Fatalf("attempt metadata = %#v", snapshot.Attempt)
	}
	if snapshot.Hostname != "server-1" || snapshot.OperatingSystem != "Ubuntu 22.04.3 LTS" ||
		snapshot.KernelVersion != "Linux 5.15.0-91-generic" || snapshot.UptimeSeconds != 123456 ||
		snapshot.LoadAverage1m != 0.42 {
		t.Fatalf("snapshot scalar fields = %#v", snapshot)
	}
	if snapshot.Memory.TotalBytes != 16000000000 || snapshot.Memory.AvailableBytes != 8000000000 ||
		snapshot.Memory.UsedBytes != 8000000000 {
		t.Fatalf("snapshot memory = %#v", snapshot.Memory)
	}
	if len(snapshot.Disks) != 2 || snapshot.Disks[0].MountPoint != "/" ||
		snapshot.Disks[0].Filesystem != "/dev/sda1" || snapshot.Disks[0].TotalBytes != 100000000000 {
		t.Fatalf("snapshot disks = %#v", snapshot.Disks)
	}

	if factory.callCount() != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.callCount())
	}
	config := factory.configsSnapshot()[0]
	if config.Host != "server.example.com" || config.Port != 22 || config.Username != "operator" ||
		config.ConnectTimeout != 3*time.Second || config.CommandTimeout != 4*time.Second ||
		config.MaxOutputBytes != 2048 {
		t.Fatalf("factory config = %#v", config)
	}
	if string(config.PrivateKey) != string(connection.PrivateKey) ||
		config.HostKeySHA256 != connection.HostKeySHA256 {
		t.Fatalf("factory credential copy = %#v", config)
	}
	runners := factory.runnersSnapshot()
	if len(runners) != 1 || runners[0].callCount() != 1 || runners[0].observedCommand() != buildSystemSnapshotCommand() {
		t.Fatalf("runner usage = %#v", runners)
	}
}

func TestServiceStatusReturnsTypedResultAndExactCommand(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: readActionServiceStatusOutput()}}
	service := newFakeService(t, factory)

	status, err := service.ServiceStatus(context.Background(), serviceTestConnection(), "nginx.service")
	if err != nil {
		t.Fatalf("ServiceStatus() error = %v", err)
	}
	if status.ServiceName != "nginx.service" || status.LoadState != "loaded" ||
		status.ActiveState != "active" || status.SubState != "running" ||
		status.Description != "ZGI test unit" {
		t.Fatalf("service status = %#v", status)
	}
	if status.RequestID != serviceTestRequestID || status.AttemptCount != 1 {
		t.Fatalf("attempt metadata = %#v", status.Attempt)
	}
	wantCommand, _, err := buildServiceStatusCommand(map[string]any{"service_name": "nginx.service"})
	if err != nil {
		t.Fatalf("buildServiceStatusCommand: %v", err)
	}
	runners := factory.runnersSnapshot()
	if len(runners) != 1 || runners[0].observedCommand() != wantCommand {
		t.Fatalf("command = %q, want %q", runners[0].observedCommand(), wantCommand)
	}
}

func TestJournalTailReturnsTypedResultAndTruncation(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: []byte("alpha\nbeta\n")}}
	service := newFakeService(t, factory)

	journal, err := service.JournalTail(context.Background(), serviceTestConnection(), "nginx.service", JournalOptions{})
	if err != nil {
		t.Fatalf("JournalTail() error = %v", err)
	}
	if journal.ServiceName != "nginx.service" || !equalStringSlices(journal.Lines, []string{"alpha", "beta"}) ||
		journal.Truncated {
		t.Fatalf("journal = %#v", journal)
	}
	wantCommand, _, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service"})
	if err != nil {
		t.Fatalf("buildJournalTailCommand: %v", err)
	}
	if got := factory.runnersSnapshot()[0].observedCommand(); got != wantCommand {
		t.Fatalf("command = %q, want %q", got, wantCommand)
	}

	truncatedFactory := &serviceFakeFactory{result: sshRunResult{
		Stdout:    append([]byte("caf\u00e9\n"), 0xE2, 0x82),
		Truncated: true,
	}}
	truncatedService := newFakeService(t, truncatedFactory)
	truncated, err := truncatedService.JournalTail(
		context.Background(), serviceTestConnection(), "nginx.service", JournalOptions{Lines: 10, SinceMinutes: 15},
	)
	if err != nil {
		t.Fatalf("JournalTail(truncated) error = %v", err)
	}
	if !truncated.Truncated || !equalStringSlices(truncated.Lines, []string{"caf\u00e9"}) {
		t.Fatalf("truncated journal = %#v", truncated)
	}
	wantTruncatedCommand, _, err := buildJournalTailCommand(map[string]any{
		"service_name": "nginx.service", "lines": 10, "since_minutes": 15,
	})
	if err != nil {
		t.Fatalf("buildJournalTailCommand(explicit): %v", err)
	}
	if got := truncatedFactory.runnersSnapshot()[0].observedCommand(); got != wantTruncatedCommand {
		t.Fatalf("truncated command = %q, want %q", got, wantTruncatedCommand)
	}
}

func TestInvalidInputIsRejectedBeforeFactory(t *testing.T) {
	cases := []struct {
		name string
		call func(*Service) error
	}{
		{"service status empty name", func(s *Service) error {
			_, err := s.ServiceStatus(context.Background(), serviceTestConnection(), "")
			return err
		}},
		{"service status command injection", func(s *Service) error {
			_, err := s.ServiceStatus(context.Background(), serviceTestConnection(), "nginx.service; id")
			return err
		}},
		{"service status path", func(s *Service) error {
			_, err := s.ServiceStatus(context.Background(), serviceTestConnection(), "/bin/sh")
			return err
		}},
		{"journal negative lines", func(s *Service) error {
			_, err := s.JournalTail(context.Background(), serviceTestConnection(), "nginx.service", JournalOptions{Lines: -1})
			return err
		}},
		{"journal lines above cap", func(s *Service) error {
			_, err := s.JournalTail(context.Background(), serviceTestConnection(), "nginx.service", JournalOptions{Lines: maxJournalLines + 1})
			return err
		}},
		{"journal since above cap", func(s *Service) error {
			_, err := s.JournalTail(context.Background(), serviceTestConnection(), "nginx.service", JournalOptions{SinceMinutes: maxJournalSinceMinutes + 1})
			return err
		}},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
		service := newFakeService(t, factory)
		err := testCase.call(service)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: error = %v, want ErrInvalidInput", testCase.name, err)
		}
		if factory.callCount() != 0 {
			t.Errorf("%s: factory calls = %d, want 0", testCase.name, factory.callCount())
		}
	}
}

func TestConnectionValidationRejectsBadConnectionsBeforeFactory(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Connection)
	}{
		{"blank host", func(c *Connection) { c.Host = "   " }},
		{"host with space", func(c *Connection) { c.Host = "bad host" }},
		{"host with control byte", func(c *Connection) { c.Host = "bad\x01host" }},
		{"host too long", func(c *Connection) { c.Host = strings.Repeat("a", maxHostBytes+1) }},
		{"blank username", func(c *Connection) { c.Username = "" }},
		{"username with control byte", func(c *Connection) { c.Username = "bad\x7fuser" }},
		{"username too long", func(c *Connection) { c.Username = strings.Repeat("u", maxUsernameBytes+1) }},
		{"port zero", func(c *Connection) { c.Port = 0 }},
		{"port too large", func(c *Connection) { c.Port = 65536 }},
		{"missing private key", func(c *Connection) { c.PrivateKey = nil }},
		{"blank private key", func(c *Connection) { c.PrivateKey = []byte(" \n\t ") }},
		{"private key too long", func(c *Connection) { c.PrivateKey = bytes.Repeat([]byte("k"), maxPrivateKeyBytes+1) }},
		{"passphrase too long", func(c *Connection) { c.Passphrase = bytes.Repeat([]byte("p"), maxPassphraseBytes+1) }},
		{"missing fingerprint", func(c *Connection) { c.HostKeySHA256 = "" }},
		{"fingerprint too long", func(c *Connection) { c.HostKeySHA256 = strings.Repeat("f", maxFingerprintBytes+1) }},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
		service := newFakeService(t, factory)
		connection := serviceTestConnection()
		testCase.mutate(&connection)
		_, err := service.SystemSnapshot(context.Background(), connection)
		if !errors.Is(err, ErrConnectionInvalid) {
			t.Errorf("%s: error = %v, want ErrConnectionInvalid", testCase.name, err)
		}
		if errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: connection failure must not also classify as ErrInvalidInput", testCase.name)
		}
		if factory.callCount() != 0 {
			t.Errorf("%s: factory calls = %d, want 0", testCase.name, factory.callCount())
		}
	}
}

// TestTargetsAreRejectedUnnormalized proves the capability never trims a host,
// username, or fingerprint into a different dialed target: any surrounding
// whitespace or control byte is rejected before the factory runs, and the
// rejection never echoes the original value.
func TestTargetsAreRejectedUnnormalized(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		apply func(*Connection, string)
	}{
		{"host leading space", " server.example.com", func(c *Connection, v string) { c.Host = v }},
		{"host trailing tab", "server.example.com\t", func(c *Connection, v string) { c.Host = v }},
		{"host leading control", "\x01server.example.com", func(c *Connection, v string) { c.Host = v }},
		{"host interior control", "server\x01.example.com", func(c *Connection, v string) { c.Host = v }},
		{"username leading space", " operator", func(c *Connection, v string) { c.Username = v }},
		{"username trailing newline", "operator\n", func(c *Connection, v string) { c.Username = v }},
		{"username interior control", "oper\x01ator", func(c *Connection, v string) { c.Username = v }},
		{"fingerprint leading space", " SHA256:fingerprint", func(c *Connection, v string) { c.HostKeySHA256 = v }},
		{"fingerprint trailing space", "SHA256:fingerprint ", func(c *Connection, v string) { c.HostKeySHA256 = v }},
		{"fingerprint nested control", "SHA256:\x01fingerprint", func(c *Connection, v string) { c.HostKeySHA256 = v }},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
		service := newFakeService(t, factory)
		connection := serviceTestConnection()
		testCase.apply(&connection, testCase.raw)

		_, err := service.SystemSnapshot(context.Background(), connection)
		if !errors.Is(err, ErrConnectionInvalid) {
			t.Errorf("%s: error = %v, want ErrConnectionInvalid", testCase.name, err)
			continue
		}
		if factory.callCount() != 0 {
			t.Errorf("%s: factory calls = %d, want 0", testCase.name, factory.callCount())
		}
		if strings.Contains(err.Error(), testCase.raw) {
			t.Errorf("%s: error echoed the rejected value: %v", testCase.name, err)
		}
	}
}

// serviceSecretObserver retains the slice headers the factory received, so it
// can prove the service zeroed the same backing arrays before the runner ran.
type serviceSecretObserver struct {
	privateKey []byte
	passphrase []byte
	zeroed     bool
	calls      int
}

func (o *serviceSecretObserver) run(context.Context, string) (sshRunResult, error) {
	o.calls++
	o.zeroed = serviceAllZero(o.privateKey) && serviceAllZero(o.passphrase)
	return sshRunResult{Stdout: validSnapshotOutput()}, nil
}

func serviceAllZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func TestSecretsAreZeroedAfterFactoryReturns(t *testing.T) {
	observer := &serviceSecretObserver{}
	factory := runnerFactory(func(config sshClientConfig) (runner, error) {
		observer.privateKey = config.PrivateKey
		observer.passphrase = config.Passphrase
		return observer, nil
	})
	service := newTestService(t, Limits{}, factory, func() string { return serviceTestRequestID })
	connection := serviceTestConnection()
	connection.Passphrase = []byte("service-test-passphrase")

	if _, err := service.SystemSnapshot(context.Background(), connection); err != nil {
		t.Fatalf("SystemSnapshot() error = %v", err)
	}
	if observer.calls != 1 {
		t.Fatalf("run calls = %d, want 1", observer.calls)
	}
	if len(observer.privateKey) == 0 || len(observer.passphrase) == 0 {
		t.Fatal("the observer did not capture the secret copies")
	}
	if !observer.zeroed {
		t.Error("the key/passphrase copies were not zeroed before the run")
	}
	if bytes.Contains(observer.privateKey, []byte("service-test-key")) {
		t.Error("the private key copy still contains key material")
	}
}

func TestCallerSlicesAreNotMutated(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)
	connection := serviceTestConnection()
	connection.Passphrase = []byte("service-test-passphrase")
	privateKeyBefore := append([]byte(nil), connection.PrivateKey...)
	passphraseBefore := append([]byte(nil), connection.Passphrase...)

	if _, err := service.SystemSnapshot(context.Background(), connection); err != nil {
		t.Fatalf("SystemSnapshot() error = %v", err)
	}
	if !bytes.Equal(connection.PrivateKey, privateKeyBefore) {
		t.Error("the caller private key slice was mutated")
	}
	if !bytes.Equal(connection.Passphrase, passphraseBefore) {
		t.Error("the caller passphrase slice was mutated")
	}
}

func TestRunnerErrorsMapToLocalClasses(t *testing.T) {
	exitErr := &ssh.ExitError{}
	cases := []struct {
		name   string
		runErr error
		want   error
	}{
		{"context canceled", context.Canceled, ErrTimeout},
		{"context deadline", context.DeadlineExceeded, ErrTimeout},
		{"wrapped context canceled", fmt.Errorf("run: %w", context.Canceled), ErrTimeout},
		{"host key mismatch", fmt.Errorf("%w: presented key", errSSHHostKeyMismatch), ErrAuthInvalid},
		{"invalid config", fmt.Errorf("%w: host is blank", errSSHInvalidConfig), ErrConnectionInvalid},
		{"invalid command", fmt.Errorf("%w: command is blank", errSSHInvalidCommand), ErrInvalidInput},
		{"exit error", fmt.Errorf("remote exited: %w", exitErr), ErrProviderRejected},
		{"generic failure", errors.New("boom"), ErrUpstream},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}, runErr: testCase.runErr}
		service := newFakeService(t, factory)
		_, err := service.SystemSnapshot(context.Background(), serviceTestConnection())
		if !errors.Is(err, testCase.want) {
			t.Errorf("%s: error = %v, want class %v", testCase.name, err, testCase.want)
		}
		var capabilityErr *Error
		if !errors.As(err, &capabilityErr) {
			t.Fatalf("%s: error is not a *serverops.Error", testCase.name)
		}
		if capabilityErr.RequestID != serviceTestRequestID || capabilityErr.AttemptCount != 1 {
			t.Errorf("%s: attempt metadata = %#v", testCase.name, capabilityErr)
		}
		if runners := factory.runnersSnapshot(); len(runners) != 1 || runners[0].callCount() != 1 {
			t.Errorf("%s: the run was not attempted exactly once", testCase.name)
		}
	}
}

func TestResponseInvalidOnTruncationAndCorruptOutput(t *testing.T) {
	truncatedFactory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput(), Truncated: true}}
	truncatedService := newFakeService(t, truncatedFactory)
	if _, err := truncatedService.SystemSnapshot(context.Background(), serviceTestConnection()); !errors.Is(err, ErrResponseInvalid) {
		t.Errorf("truncated snapshot error = %v, want ErrResponseInvalid", err)
	}
	if _, err := truncatedService.ServiceStatus(context.Background(), serviceTestConnection(), "nginx.service"); !errors.Is(err, ErrResponseInvalid) {
		t.Errorf("truncated service error = %v, want ErrResponseInvalid", err)
	}

	for _, testCase := range []struct {
		name   string
		action string
		output []byte
	}{
		{"snapshot wrong header", ActionSystemSnapshot, []byte("ZGI_SERVER_SNAPSHOT_V2\nhostname\tleak\n")},
		{"snapshot missing records", ActionSystemSnapshot, []byte("ZGI_SERVER_SNAPSHOT_V1\nhostname\tleak\n")},
		{"service wrong header", ActionServiceStatus, []byte("ZGI_SERVER_SERVICE_STATUS_V2\n")},
		{"service unknown key", ActionServiceStatus, []byte("ZGI_SERVER_SERVICE_STATUS_V1\nSecret=leak\n")},
	} {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: testCase.output}}
		service := newFakeService(t, factory)
		var err error
		if testCase.action == ActionSystemSnapshot {
			_, err = service.SystemSnapshot(context.Background(), serviceTestConnection())
		} else {
			_, err = service.ServiceStatus(context.Background(), serviceTestConnection(), "nginx.service")
		}
		if !errors.Is(err, ErrResponseInvalid) {
			t.Errorf("%s: error = %v, want ErrResponseInvalid", testCase.name, err)
			continue
		}
		if strings.Contains(err.Error(), "leak") {
			t.Errorf("%s: error leaked remote output: %v", testCase.name, err)
		}
	}
}

func TestOperationErrorsDoNotEchoSecretsOrHost(t *testing.T) {
	const (
		secretHost = "leak-probe-host.internal"
		secretUser = "leak-probe-user"
		secretPass = "leak-probe-passphrase"
	)
	secretKey := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nleak-probe-private-body\n-----END OPENSSH PRIVATE KEY-----")
	connection := Connection{
		Host: secretHost, Port: 22, Username: secretUser,
		PrivateKey: secretKey, Passphrase: []byte(secretPass),
		HostKeySHA256: "SHA256:" + strings.Repeat("A", 43),
	}
	factory := &serviceFakeFactory{
		result: sshRunResult{Stdout: []byte("leak"), Stderr: []byte("leak-probe-stderr")},
		runErr: fmt.Errorf("dial %s as %s: %s", secretHost, secretUser, secretPass),
	}
	service := newFakeService(t, factory)
	_, err := service.SystemSnapshot(context.Background(), connection)
	if err == nil {
		t.Fatal("expected a runner error")
	}
	message := err.Error()
	for _, forbidden := range []string{secretHost, secretUser, secretPass, "leak-probe-private-body", "leak-probe-stderr"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("public error string leaked %q: %s", forbidden, message)
		}
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error = %v, want ErrUpstream", err)
	}
}

func TestValidateCredentialParsesLocally(t *testing.T) {
	service := newTestService(t, Limits{}, productionRunnerFactory, func() string { return serviceTestRequestID })
	unencrypted := newTestIdentity(t, "")
	encrypted := newTestIdentity(t, serviceTestPassphrase)

	valid := func(identity testIdentity, passphrase string) Connection {
		connection := Connection{
			Host: "127.0.0.1", Port: 22, Username: "operator",
			PrivateKey:    append([]byte(nil), identity.privateKeyPEM...),
			HostKeySHA256: ssh.FingerprintSHA256(identity.publicKey),
		}
		if passphrase != "" {
			connection.Passphrase = []byte(passphrase)
		}
		return connection
	}

	if err := service.ValidateCredential(context.Background(), valid(unencrypted, "")); err != nil {
		t.Errorf("unencrypted key: %v", err)
	}
	if err := service.ValidateCredential(context.Background(), valid(encrypted, serviceTestPassphrase)); err != nil {
		t.Errorf("encrypted key: %v", err)
	}
	rejected := []struct {
		name       string
		connection Connection
	}{
		{"wrong passphrase", valid(encrypted, "wrong-passphrase")},
		{"missing passphrase", valid(encrypted, "")},
		{"malformed key", Connection{Host: "127.0.0.1", Port: 22, Username: "operator", PrivateKey: []byte("not a pem key"), HostKeySHA256: "SHA256:short"}},
	}
	for _, testCase := range rejected {
		err := service.ValidateCredential(context.Background(), testCase.connection)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: error = %v, want ErrInvalidInput", testCase.name, err)
		}
		if errors.Is(err, ErrConnectionInvalid) {
			t.Errorf("%s: invalid credential must not also classify as ErrConnectionInvalid", testCase.name)
		}
	}

	factory := &serviceFakeFactory{}
	countingService := newFakeService(t, factory)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := countingService.ValidateCredential(canceled, serviceTestConnection()); !errors.Is(err, ErrTimeout) {
		t.Errorf("canceled context: error = %v, want ErrTimeout", err)
	}
	if factory.callCount() != 0 {
		t.Errorf("canceled context: factory calls = %d, want 0", factory.callCount())
	}
}

// TestValidateCredentialHasOnePublicClass pins the classification contract:
// invalid credential/connection material is exactly ErrInvalidInput, a canceled
// context is exactly ErrTimeout, and a factory wiring failure is not masked as
// input.
func TestValidateCredentialHasOnePublicClass(t *testing.T) {
	service := newTestService(t, Limits{}, productionRunnerFactory, func() string { return serviceTestRequestID })

	invalidConnections := []struct {
		name       string
		connection Connection
	}{
		{"blank host", Connection{Host: " ", Port: 22, Username: "operator", PrivateKey: []byte("key"), HostKeySHA256: "SHA256:short"}},
		{"untrimmed host", Connection{Host: " 127.0.0.1", Port: 22, Username: "operator", PrivateKey: []byte("key"), HostKeySHA256: "SHA256:short"}},
		{"control username", Connection{Host: "127.0.0.1", Port: 22, Username: "oper\x01ator", PrivateKey: []byte("key"), HostKeySHA256: "SHA256:short"}},
		{"port out of range", Connection{Host: "127.0.0.1", Port: 0, Username: "operator", PrivateKey: []byte("key"), HostKeySHA256: "SHA256:short"}},
		{"malformed key", Connection{Host: "127.0.0.1", Port: 22, Username: "operator", PrivateKey: []byte("not a pem key"), HostKeySHA256: "SHA256:short"}},
	}
	for _, testCase := range invalidConnections {
		err := service.ValidateCredential(context.Background(), testCase.connection)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: error = %v, want ErrInvalidInput", testCase.name, err)
		}
		if errors.Is(err, ErrConnectionInvalid) {
			t.Errorf("%s: classification is ambiguous with ErrConnectionInvalid", testCase.name)
		}
		if errors.Is(err, ErrTimeout) {
			t.Errorf("%s: classification is ambiguous with ErrTimeout", testCase.name)
		}
		var capabilityErr *Error
		if !errors.As(err, &capabilityErr) {
			t.Errorf("%s: error is not a *serverops.Error", testCase.name)
		} else if capabilityErr.RequestID != "" || capabilityErr.AttemptCount != 0 {
			t.Errorf("%s: validation must not report an attempt: %#v", testCase.name, capabilityErr)
		}
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	canceledErr := service.ValidateCredential(canceled, Connection{})
	if !errors.Is(canceledErr, ErrTimeout) {
		t.Errorf("canceled context: error = %v, want ErrTimeout", canceledErr)
	}
	if errors.Is(canceledErr, ErrInvalidInput) {
		t.Error("canceled context must not also classify as ErrInvalidInput")
	}

	// A missing factory seam is an internal wiring failure and must stay
	// upstream rather than being reported as invalid input.
	wiringService := newTestService(t, Limits{}, nil, func() string { return serviceTestRequestID })
	wiringErr := wiringService.ValidateCredential(context.Background(), serviceTestConnection())
	if !errors.Is(wiringErr, ErrUpstream) {
		t.Errorf("missing factory: error = %v, want ErrUpstream", wiringErr)
	}
	if errors.Is(wiringErr, ErrInvalidInput) {
		t.Error("missing factory must not be masked as ErrInvalidInput")
	}
}

func TestValidateConnectionUsesFixedSnapshotCommand(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)
	profile, err := service.ValidateConnection(context.Background(), serviceTestConnection())
	if err != nil {
		t.Fatalf("ValidateConnection() error = %v", err)
	}
	if profile.Hostname != "server-1" || profile.RequestID != serviceTestRequestID || profile.AttemptCount != 1 {
		t.Fatalf("profile = %#v", profile)
	}
	runners := factory.runnersSnapshot()
	if len(runners) != 1 || runners[0].observedCommand() != buildSystemSnapshotCommand() {
		t.Fatalf("connection test must run the fixed snapshot command")
	}

	truncatedFactory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput(), Truncated: true}}
	truncatedService := newFakeService(t, truncatedFactory)
	if _, err := truncatedService.ValidateConnection(context.Background(), serviceTestConnection()); !errors.Is(err, ErrResponseInvalid) {
		t.Errorf("truncated connection test error = %v, want ErrResponseInvalid", err)
	}

	canceledFactory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	canceledService := newFakeService(t, canceledFactory)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := canceledService.ValidateConnection(canceled, serviceTestConnection()); !errors.Is(err, ErrTimeout) {
		t.Errorf("canceled connection test error = %v, want ErrTimeout", err)
	}
	if canceledFactory.callCount() != 0 {
		t.Errorf("canceled context: factory calls = %d, want 0", canceledFactory.callCount())
	}
}

// TestValidateConnectionCapturesObservedHostKey proves the opted-in capture
// probe surfaces the canonical fingerprint the transport recorded and that the
// factory received an explicit capture config with no pinned fingerprint.
func TestValidateConnectionCapturesObservedHostKey(t *testing.T) {
	observed := serviceTestConnection().HostKeySHA256
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput(), HostKeyFingerprint: observed}}
	service := newFakeService(t, factory)

	profile, err := service.ValidateConnection(context.Background(), serviceTestCaptureConnection())
	if err != nil {
		t.Fatalf("ValidateConnection() error = %v", err)
	}
	if profile.Hostname != "server-1" || profile.HostKeyFingerprint != observed {
		t.Fatalf("profile = %#v", profile)
	}
	config := factory.configsSnapshot()[0]
	if !config.CaptureHostKey || config.HostKeySHA256 != "" {
		t.Fatalf("capture factory config = %#v", config)
	}
}

// TestValidateConnectionCaptureRequiresObservedFingerprint proves a capture
// probe that yields no usable observation fails closed instead of returning an
// empty fingerprint as a successful result.
func TestValidateConnectionCaptureRequiresObservedFingerprint(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)

	if _, err := service.ValidateConnection(context.Background(), serviceTestCaptureConnection()); !errors.Is(err, ErrResponseInvalid) {
		t.Fatalf("capture without an observation: error = %v, want ErrResponseInvalid", err)
	}
}

// TestValidateConnectionCaptureRejectsMalformedObservation proves an observed
// value that is not the canonical bounded form is never surfaced.
func TestValidateConnectionCaptureRejectsMalformedObservation(t *testing.T) {
	for name, observed := range map[string]string{
		"missing prefix": "not-a-fingerprint",
		"padded base64":  serviceTestConnection().HostKeySHA256 + "=",
		"too long":       "SHA256:" + strings.Repeat("A", maxFingerprintBytes),
	} {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput(), HostKeyFingerprint: observed}}
		service := newFakeService(t, factory)
		if _, err := service.ValidateConnection(context.Background(), serviceTestCaptureConnection()); !errors.Is(err, ErrResponseInvalid) {
			t.Errorf("%s: error = %v, want ErrResponseInvalid", name, err)
		}
	}
}

// TestValidateConnectionStrictModeDoesNotSurfaceObservedFingerprint proves a
// strict connection keeps its exact fingerprint config and never surfaces an
// observed value, so capture mode cannot weaken the ordinary path.
func TestValidateConnectionStrictModeDoesNotSurfaceObservedFingerprint(t *testing.T) {
	connection := serviceTestConnection()
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput(), HostKeyFingerprint: connection.HostKeySHA256}}
	service := newFakeService(t, factory)

	profile, err := service.ValidateConnection(context.Background(), connection)
	if err != nil {
		t.Fatalf("ValidateConnection() error = %v", err)
	}
	if profile.HostKeyFingerprint != "" {
		t.Fatalf("strict profile surfaced an observed fingerprint: %#v", profile)
	}
	config := factory.configsSnapshot()[0]
	if config.CaptureHostKey || config.HostKeySHA256 != connection.HostKeySHA256 {
		t.Fatalf("strict factory config = %#v", config)
	}
}

// TestCaptureModeRequiresEmptyFingerprint proves a capture request carrying a
// pinned fingerprint is rejected before the factory runs rather than silently
// dropping the pin.
func TestCaptureModeRequiresEmptyFingerprint(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)
	connection := serviceTestConnection()
	connection.CaptureHostKey = true

	_, err := service.ValidateConnection(context.Background(), connection)
	if !errors.Is(err, ErrConnectionInvalid) {
		t.Fatalf("capture with a pinned fingerprint: error = %v, want ErrConnectionInvalid", err)
	}
	if factory.callCount() != 0 {
		t.Fatalf("factory calls = %d, want 0", factory.callCount())
	}
}

// TestReadActionsRejectCaptureModeBeforeFactory proves capture mode is confined
// to the fixed validation probe: no read action may run with host-key
// verification switched to discovery.
func TestReadActionsRejectCaptureModeBeforeFactory(t *testing.T) {
	cases := []struct {
		name string
		call func(*Service) error
	}{
		{"system snapshot", func(s *Service) error {
			_, err := s.SystemSnapshot(context.Background(), serviceTestCaptureConnection())
			return err
		}},
		{"service status", func(s *Service) error {
			_, err := s.ServiceStatus(context.Background(), serviceTestCaptureConnection(), "nginx.service")
			return err
		}},
		{"journal tail", func(s *Service) error {
			_, err := s.JournalTail(context.Background(), serviceTestCaptureConnection(), "nginx.service", JournalOptions{})
			return err
		}},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
		service := newFakeService(t, factory)
		if err := testCase.call(service); !errors.Is(err, ErrConnectionInvalid) {
			t.Errorf("%s: error = %v, want ErrConnectionInvalid", testCase.name, err)
		}
		if factory.callCount() != 0 {
			t.Errorf("%s: factory calls = %d, want 0", testCase.name, factory.callCount())
		}
	}
}

// TestCaptureModeMalformedKeyStillFails proves capture mode does not bypass
// private-key parsing on either the probe or the local credential validation.
func TestCaptureModeMalformedKeyStillFails(t *testing.T) {
	service := newTestService(t, Limits{}, productionRunnerFactory, func() string { return serviceTestRequestID })

	probeConnection := serviceTestCaptureConnection()
	probeConnection.PrivateKey = []byte("not a pem key")
	if _, err := service.ValidateConnection(context.Background(), probeConnection); !errors.Is(err, ErrConnectionInvalid) {
		t.Errorf("capture probe with a malformed key: error = %v, want ErrConnectionInvalid", err)
	}

	credentialConnection := serviceTestCaptureConnection()
	credentialConnection.PrivateKey = []byte("not a pem key")
	if err := service.ValidateCredential(context.Background(), credentialConnection); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("capture credential with a malformed key: error = %v, want ErrInvalidInput", err)
	}
}

func TestEachCallConstructsAFreshRunner(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)
	const calls = 3
	for index := 0; index < calls; index++ {
		if _, err := service.SystemSnapshot(context.Background(), serviceTestConnection()); err != nil {
			t.Fatalf("call %d: %v", index, err)
		}
	}
	if factory.callCount() != calls {
		t.Fatalf("factory calls = %d, want %d", factory.callCount(), calls)
	}
	runners := factory.runnersSnapshot()
	if len(runners) != calls {
		t.Fatalf("runners = %d, want %d", len(runners), calls)
	}
	seen := make(map[*serviceFakeRunner]bool, calls)
	for index, created := range runners {
		if created.callCount() != 1 {
			t.Errorf("runner %d run calls = %d, want 1", index, created.callCount())
		}
		if seen[created] {
			t.Errorf("runner %d was reused across calls", index)
		}
		seen[created] = true
	}
}

func TestConcurrentCallsUseFreshRunners(t *testing.T) {
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)
	const calls = 12
	var waitGroup sync.WaitGroup
	errs := make([]error, calls)
	for index := 0; index < calls; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			_, errs[index] = service.SystemSnapshot(context.Background(), serviceTestConnection())
		}(index)
	}
	waitGroup.Wait()
	for index, err := range errs {
		if err != nil {
			t.Errorf("concurrent call %d: %v", index, err)
		}
	}
	if factory.callCount() != calls {
		t.Errorf("factory calls = %d, want %d", factory.callCount(), calls)
	}
	seen := make(map[*serviceFakeRunner]bool, calls)
	for _, created := range factory.runnersSnapshot() {
		if created.callCount() != 1 {
			t.Errorf("runner run calls = %d, want 1", created.callCount())
		}
		if seen[created] {
			t.Error("a runner was shared across concurrent calls")
		}
		seen[created] = true
	}
}

func TestRequestIDContract(t *testing.T) {
	for _, invalidID := range []string{"", "   ", strings.Repeat("a", maxRequestIDBytes+1)} {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
		service := newTestService(t, Limits{}, factory.newRunner, func() string { return invalidID })
		if _, err := service.SystemSnapshot(context.Background(), serviceTestConnection()); !errors.Is(err, ErrUpstream) {
			t.Errorf("request id %q: error = %v, want ErrUpstream", invalidID, err)
		}
		if factory.callCount() != 1 {
			t.Errorf("request id %q: factory calls = %d, want 1", invalidID, factory.callCount())
		}
		if runners := factory.runnersSnapshot(); len(runners) != 1 || runners[0].callCount() != 0 {
			t.Errorf("request id %q: the runner ran despite an invalid id", invalidID)
		}
	}

	boundaryID := strings.Repeat("b", maxRequestIDBytes)
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newTestService(t, Limits{}, factory.newRunner, func() string { return boundaryID })
	snapshot, err := service.SystemSnapshot(context.Background(), serviceTestConnection())
	if err != nil {
		t.Fatalf("boundary request id: %v", err)
	}
	if snapshot.RequestID != boundaryID {
		t.Errorf("boundary request id = %q, want %q", snapshot.RequestID, boundaryID)
	}
}
