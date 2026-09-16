package serverops

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// testIdentity is an ephemeral SSH key pair plus its OpenSSH PEM encoding.
type testIdentity struct {
	privateKeyPEM []byte
	publicKey     ssh.PublicKey
	signer        ssh.Signer
}

func newTestIdentity(t *testing.T, passphrase string) testIdentity {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("build ssh signer: %v", err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(privateKey, "serverops test key")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "serverops test key", []byte(passphrase))
	}
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return testIdentity{
		privateKeyPEM: pem.EncodeToMemory(block),
		publicKey:     signer.PublicKey(),
		signer:        signer,
	}
}

// testExecHandler handles the exec request of one session. connClosed is closed
// by the server once the underlying SSH connection shuts down, which lets a
// handler block deterministically until the client tears the transport down.
type testExecHandler func(command string, channel ssh.Channel, connClosed <-chan struct{})

// loopbackSSHServer is an in-process SSH server bound to 127.0.0.1 that accepts
// exactly one generated client public key. It is test-only and never dials out.
type loopbackSSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	handler  testExecHandler
	// honorChannelOpen selects the session fixture mode. When true the server
	// accepts session channels and serves exec requests. When false it
	// authenticates and then deliberately never services channel-open, so a
	// client NewSession blocks until the transport is torn down.
	honorChannelOpen bool
	// authHook observes successful client authentication.
	authHook        func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error)
	hostFingerprint string

	connDone chan struct{}
	connOnce sync.Once

	mu     sync.Mutex
	closed bool
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup
}

func newLoopbackSSHServer(t *testing.T, clientPublicKey ssh.PublicKey, handler testExecHandler) *loopbackSSHServer {
	t.Helper()
	return newLoopbackSSHServerWithOptions(t, clientPublicKey, handler, serverFixtureOptions{honorChannelOpen: true})
}

// serverFixtureOptions selects between the normal one-session server and the
// explicit mode that authenticates but never services channel-open requests.
type serverFixtureOptions struct {
	honorChannelOpen bool
	authHook         func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error)
}

func newLoopbackSSHServerWithOptions(
	t *testing.T,
	clientPublicKey ssh.PublicKey,
	handler testExecHandler,
	options serverFixtureOptions,
) *loopbackSSHServer {
	t.Helper()
	hostIdentity := newTestIdentity(t, "")
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if subtle.ConstantTimeCompare(key.Marshal(), clientPublicKey.Marshal()) != 1 {
				return nil, errors.New("serverops test: unknown client public key")
			}
			if options.authHook != nil {
				return options.authHook(metadata, key)
			}
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(hostIdentity.signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}
	server := &loopbackSSHServer{
		listener:         listener,
		config:           config,
		handler:          handler,
		honorChannelOpen: options.honorChannelOpen,
		authHook:         options.authHook,
		hostFingerprint:  ssh.FingerprintSHA256(hostIdentity.signer.PublicKey()),
		connDone:         make(chan struct{}),
		conns:            make(map[net.Conn]struct{}),
	}
	server.wg.Add(1)
	go server.acceptLoop()
	t.Cleanup(server.Close)
	return server
}

func (s *loopbackSSHServer) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *loopbackSSHServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		if !s.track(conn) {
			_ = conn.Close()
			continue
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *loopbackSSHServer) track(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[conn] = struct{}{}
	return true
}

func (s *loopbackSSHServer) untrack(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func (s *loopbackSSHServer) handleConn(raw net.Conn) {
	defer s.wg.Done()
	defer s.untrack(raw)
	defer s.connOnce.Do(func() { close(s.connDone) })
	defer raw.Close()

	serverConn, channels, requests, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		return
	}
	defer serverConn.Close()

	connClosed := make(chan struct{})
	go func() {
		_ = serverConn.Wait()
		close(connClosed)
	}()
	go ssh.DiscardRequests(requests)

	if !s.honorChannelOpen {
		// Deliberately never read from channels: authentication and the global
		// request loop completed, but channel-open is never serviced, so a
		// client NewSession blocks until the transport is torn down.
		<-connClosed
		return
	}

	var sessions sync.WaitGroup
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		sessions.Add(1)
		go func() {
			defer sessions.Done()
			s.serveSession(channel, channelRequests, connClosed)
		}()
	}
	sessions.Wait()
}

func (s *loopbackSSHServer) serveSession(channel ssh.Channel, requests <-chan *ssh.Request, connClosed <-chan struct{}) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var payload struct {
			Command string
		}
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			_ = request.Reply(false, nil)
			return
		}
		if err := request.Reply(true, nil); err != nil {
			return
		}
		s.handler(payload.Command, channel, connClosed)
		return
	}
}

// Close stops the listener, tears down every live connection, and waits for all
// server goroutines so a failing test cannot leak or hang cleanup.
func (s *loopbackSSHServer) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.mu.Unlock()

	_ = s.listener.Close()
	for _, conn := range conns {
		_ = conn.Close()
	}
	s.wg.Wait()
}

func sendExitStatus(channel ssh.Channel, status uint32) {
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
}

func execWrites(stdout, stderr string, status uint32) testExecHandler {
	return func(_ string, channel ssh.Channel, _ <-chan struct{}) {
		if stdout != "" {
			_, _ = io.WriteString(channel, stdout)
		}
		if stderr != "" {
			_, _ = io.WriteString(channel.Stderr(), stderr)
		}
		sendExitStatus(channel, status)
	}
}

func execEchoCommand() testExecHandler {
	return func(command string, channel ssh.Channel, _ <-chan struct{}) {
		_, _ = io.WriteString(channel, command)
		sendExitStatus(channel, 0)
	}
}

func execBlockUntilClosed(started chan<- struct{}) testExecHandler {
	return func(_ string, _ ssh.Channel, connClosed <-chan struct{}) {
		if started != nil {
			close(started)
		}
		<-connClosed
	}
}

func testSSHClientConfig(server *loopbackSSHServer, identity testIdentity, passphrase string) sshClientConfig {
	return sshClientConfig{
		Host:           "127.0.0.1",
		Port:           server.port(),
		Username:       "operator",
		PrivateKey:     identity.privateKeyPEM,
		Passphrase:     []byte(passphrase),
		HostKeySHA256:  server.hostFingerprint,
		ConnectTimeout: 5 * time.Second,
		CommandTimeout: 5 * time.Second,
		MaxOutputBytes: 1 << 20,
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func waitForServerClose(t *testing.T, server *loopbackSSHServer) {
	t.Helper()
	waitForSignal(t, server.connDone, "the ssh server to observe connection termination")
}

// contextStubConn lets a transport test control exactly when the SSH handshake
// read completes. It embeds the silent peer so non-read deadline behavior still
// comes from a real net.Pipe connection, while Read waits for the supplied
// context instead of depending on an arbitrary sleep.
type contextStubConn struct {
	net.Conn
	ctx context.Context
}

func (c *contextStubConn) Read([]byte) (int, error) {
	<-c.ctx.Done()
	// The context is already done here, so Err is guaranteed non-nil and the
	// handshake path observes a specific context error.
	return 0, c.ctx.Err()
}

func (c *contextStubConn) Write(p []byte) (int, error) {
	return len(p), nil
}

// TestSSHClientConnectTimeoutIsOneTotalDialAndHandshakeBudget proves the
// connect timeout is a single budget. The injected dial consumes nearly all of
// the supplied context budget before returning, so a correct implementation has
// almost no handshake budget left and returns context.DeadlineExceeded. If the
// handshake were granted a fresh full timeout the run would take about twice
// the configured budget; the upper bound fails that case without depending on a
// tight sleep.
func TestSSHClientConnectTimeoutIsOneTotalDialAndHandshakeBudget(t *testing.T) {
	identity := newTestIdentity(t, "")
	pipeServer, pipePeer := net.Pipe()
	defer func() { _ = pipePeer.Close() }()

	const connectTimeout = 1500 * time.Millisecond

	var dialDuration atomic.Int64
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		start := time.Now()
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("dial received no connect deadline")
		}
		// Consume most of the single budget, leaving only a sliver for the
		// handshake. Coordinating on the deadline keeps this deterministic.
		if wait := time.Until(deadline) - 40*time.Millisecond; wait > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
		}
		dialDuration.Store(int64(time.Since(start)))
		return &contextStubConn{Conn: pipeServer, ctx: ctx}, nil
	}

	config := sshClientConfig{
		Host:           "127.0.0.1",
		Port:           22,
		Username:       "operator",
		PrivateKey:     identity.privateKeyPEM,
		HostKeySHA256:  ssh.FingerprintSHA256(identity.publicKey),
		ConnectTimeout: connectTimeout,
		CommandTimeout: 5 * time.Second,
		MaxOutputBytes: 1024,
	}
	client, err := newSSHClient(config, dial)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	start := time.Now()
	_, err = client.run(context.Background(), "true")
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	// A correct implementation waits out the single connect budget. Windows CI
	// margins are generous, so only the lower bound is well below the budget.
	if elapsed < connectTimeout*3/4 {
		t.Errorf("run returned after %v, before most of the %v connect budget", elapsed, connectTimeout)
	}
	// connectTimeout*4/3 is far under the connectTimeout*2 that a second full
	// handshake budget would produce, proving no second budget was granted.
	if limit := connectTimeout * 4 / 3; elapsed >= limit {
		t.Errorf("run took %v, at or beyond %v: a second full connect budget was granted", elapsed, limit)
	}
	if consumed := time.Duration(dialDuration.Load()); consumed < connectTimeout*3/4 {
		t.Errorf("injected dial consumed only %v of the %v connect budget: the test setup did not exercise a slow dial", consumed, connectTimeout)
	}
}

func TestSSHClientNormalizesHostAndUsername(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execEchoCommand())

	config := testSSHClientConfig(server, identity, "")
	config.Host = "  127.0.0.1\t"
	config.Username = "  operator  "

	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}
	if want := net.JoinHostPort("127.0.0.1", strconv.Itoa(server.port())); client.address != want {
		t.Errorf("stored address = %q, want trimmed %q", client.address, want)
	}
	if client.username != "operator" {
		t.Errorf("stored username = %q, want trimmed %q", client.username, "operator")
	}

	// The trimmed username must also be the value used to authenticate.
	result, err := client.run(t.Context(), "trimmed")
	if err != nil {
		t.Fatalf("run with a trimmed host and username: %v", err)
	}
	if got := string(result.Stdout); got != "trimmed" {
		t.Errorf("stdout = %q, want %q", got, "trimmed")
	}
	waitForServerClose(t, server)

	config.Host = "   "
	if _, err := newSSHClient(config, nil); !errors.Is(err, errSSHInvalidConfig) {
		t.Errorf("whitespace-only host: error = %v, want errSSHInvalidConfig", err)
	}
	config = testSSHClientConfig(server, identity, "")
	config.Username = "\t\n"
	if _, err := newSSHClient(config, nil); !errors.Is(err, errSSHInvalidConfig) {
		t.Errorf("whitespace-only username: error = %v, want errSSHInvalidConfig", err)
	}
}

// TestSSHClientCallerCancellationDuringNewSession proves caller cancellation
// interrupts a NewSession that is blocked because the server never services
// channel-open.
func TestSSHClientCallerCancellationDuringNewSession(t *testing.T) {
	identity := newTestIdentity(t, "")
	authenticated := make(chan struct{})
	server := newLoopbackSSHServerWithOptions(t, identity.publicKey, nil, serverFixtureOptions{
		honorChannelOpen: false,
		authHook: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			close(authenticated)
			return &ssh.Permissions{}, nil
		},
	})

	config := testSSHClientConfig(server, identity, "")
	config.CommandTimeout = time.Minute
	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type outcome struct {
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		_, err := client.run(ctx, "never serviced")
		done <- outcome{err: err}
	}()

	waitForSignal(t, authenticated, "the server to authenticate the client")
	cancel()

	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after caller cancellation during NewSession")
	}
	waitForServerClose(t, server)
}

// TestSSHClientCommandTimeoutDuringNewSession proves the configured command
// timeout covers a blocked NewSession, not only Session.Run.
func TestSSHClientCommandTimeoutDuringNewSession(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServerWithOptions(t, identity.publicKey, nil, serverFixtureOptions{
		honorChannelOpen: false,
	})

	config := testSSHClientConfig(server, identity, "")
	config.CommandTimeout = 250 * time.Millisecond
	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	type outcome struct {
		err error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		_, err := client.run(context.Background(), "never serviced")
		done <- outcome{err: err}
	}()

	select {
	case got := <-done:
		if !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want context.DeadlineExceeded", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the command timeout during NewSession")
	}
	if elapsed := time.Since(start); elapsed < config.CommandTimeout {
		t.Errorf("run returned after %v, before the configured command timeout %v", elapsed, config.CommandTimeout)
	}
	waitForServerClose(t, server)
}

func TestSSHClientRunCapturesOutputWithVerifiedHostKey(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("hello stdout\n", "hello stderr\n", 0))

	client, err := newSSHClient(testSSHClientConfig(server, identity, ""), nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	result, err := client.run(t.Context(), "printf hello")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := string(result.Stdout); got != "hello stdout\n" {
		t.Errorf("stdout = %q, want %q", got, "hello stdout\n")
	}
	if got := string(result.Stderr); got != "hello stderr\n" {
		t.Errorf("stderr = %q, want %q", got, "hello stderr\n")
	}
	if result.Truncated {
		t.Error("Truncated = true, want false")
	}
	waitForServerClose(t, server)
}

func TestSSHClientParsesEncryptedPrivateKey(t *testing.T) {
	const passphrase = "correct horse battery staple"
	identity := newTestIdentity(t, passphrase)
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("ok", "", 0))

	config := testSSHClientConfig(server, identity, passphrase)
	keyBefore := append([]byte(nil), config.PrivateKey...)
	passphraseBefore := append([]byte(nil), config.Passphrase...)

	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient with the correct passphrase: %v", err)
	}
	if !bytes.Equal(config.PrivateKey, keyBefore) {
		t.Error("newSSHClient mutated the caller-owned private key")
	}
	if !bytes.Equal(config.Passphrase, passphraseBefore) {
		t.Error("newSSHClient mutated the caller-owned passphrase")
	}

	result, err := client.run(t.Context(), "whoami")
	if err != nil {
		t.Fatalf("run with an encrypted key: %v", err)
	}
	if got := string(result.Stdout); got != "ok" {
		t.Errorf("stdout = %q, want %q", got, "ok")
	}
	waitForServerClose(t, server)

	for name, candidate := range map[string]sshClientConfig{
		"wrong passphrase":   testSSHClientConfig(server, identity, "not the passphrase"),
		"missing passphrase": testSSHClientConfig(server, identity, ""),
	} {
		_, err := newSSHClient(candidate, nil)
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if !errors.Is(err, errSSHInvalidConfig) {
			t.Errorf("%s: error = %v, want errSSHInvalidConfig", name, err)
		}
		if strings.Contains(err.Error(), passphrase) {
			t.Errorf("%s: error leaked the passphrase", name)
		}
		if strings.Contains(err.Error(), string(identity.privateKeyPEM)) {
			t.Errorf("%s: error leaked the private key", name)
		}
	}
}

func TestSSHClientRejectsMalformedHostKeyFingerprints(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("", "", 0))

	digest := sha256.Sum256(identity.publicKey.Marshal())
	malformed := map[string]string{
		"empty":             "",
		"prefix only":       "SHA256:",
		"md5 prefix":        "MD5:aa:bb:cc",
		"lowercase prefix":  "sha256:" + base64.RawStdEncoding.EncodeToString(digest[:]),
		"not base64":        "SHA256:not base64!",
		"wrong digest size": "SHA256:" + base64.RawStdEncoding.EncodeToString(make([]byte, 16)),
		"padded base64":     "SHA256:" + base64.StdEncoding.EncodeToString(digest[:]),
		"trailing space":    "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:]) + " ",
	}
	for name, fingerprint := range malformed {
		config := testSSHClientConfig(server, identity, "")
		config.HostKeySHA256 = fingerprint
		if _, err := newSSHClient(config, nil); !errors.Is(err, errSSHInvalidConfig) {
			t.Errorf("%s: error = %v, want errSSHInvalidConfig", name, err)
		}
	}
}

func TestSSHClientRejectsMismatchedHostKeyFingerprint(t *testing.T) {
	identity := newTestIdentity(t, "")
	otherHost := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("", "", 0))

	config := testSSHClientConfig(server, identity, "")
	config.HostKeySHA256 = ssh.FingerprintSHA256(otherHost.publicKey)

	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}
	if _, err := client.run(t.Context(), "true"); !errors.Is(err, errSSHHostKeyMismatch) {
		t.Fatalf("error = %v, want errSSHHostKeyMismatch", err)
	}
	waitForServerClose(t, server)
}

func TestSSHClientConfigValidation(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("", "", 0))
	valid := testSSHClientConfig(server, identity, "")

	cases := map[string]func(*sshClientConfig){
		"blank host":           func(c *sshClientConfig) { c.Host = "   " },
		"port zero":            func(c *sshClientConfig) { c.Port = 0 },
		"port too large":       func(c *sshClientConfig) { c.Port = 65536 },
		"blank username":       func(c *sshClientConfig) { c.Username = "  " },
		"blank private key":    func(c *sshClientConfig) { c.PrivateKey = nil },
		"whitespace key":       func(c *sshClientConfig) { c.PrivateKey = []byte(" \n\t") },
		"zero connect timeout": func(c *sshClientConfig) { c.ConnectTimeout = 0 },
		"zero command timeout": func(c *sshClientConfig) { c.CommandTimeout = 0 },
		"negative command":     func(c *sshClientConfig) { c.CommandTimeout = -time.Second },
		"zero output budget":   func(c *sshClientConfig) { c.MaxOutputBytes = 0 },
		"bad fingerprint":      func(c *sshClientConfig) { c.HostKeySHA256 = "SHA256:short" },
	}
	for name, mutate := range cases {
		config := valid
		mutate(&config)
		if _, err := newSSHClient(config, nil); !errors.Is(err, errSSHInvalidConfig) {
			t.Errorf("%s: error = %v, want errSSHInvalidConfig", name, err)
		}
	}
}

func TestSSHClientCallerCancellationWinsOverCommandTimeout(t *testing.T) {
	identity := newTestIdentity(t, "")
	started := make(chan struct{})
	server := newLoopbackSSHServer(t, identity.publicKey, execBlockUntilClosed(started))

	config := testSSHClientConfig(server, identity, "")
	config.CommandTimeout = time.Minute
	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	type outcome struct {
		result sshRunResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := client.run(ctx, "wait for cancellation")
		done <- outcome{result: result, err: err}
	}()

	waitForSignal(t, started, "the server to start executing the command")
	cancel()

	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after caller cancellation")
	}
	waitForServerClose(t, server)
}

func TestSSHClientCommandTimeoutTerminatesRun(t *testing.T) {
	identity := newTestIdentity(t, "")
	started := make(chan struct{})
	server := newLoopbackSSHServer(t, identity.publicKey, execBlockUntilClosed(started))

	config := testSSHClientConfig(server, identity, "")
	config.CommandTimeout = 250 * time.Millisecond
	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	start := time.Now()
	_, err = client.run(t.Context(), "wait for timeout")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed < config.CommandTimeout {
		t.Errorf("run returned after %v, before the configured timeout %v", elapsed, config.CommandTimeout)
	}
	waitForServerClose(t, server)
}

func TestSSHClientEnforcesCombinedOutputBudget(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites(strings.Repeat("o", 8192), strings.Repeat("e", 8192), 0))

	config := testSSHClientConfig(server, identity, "")
	config.MaxOutputBytes = 100
	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	result, err := client.run(t.Context(), "produce too much output")
	if err != nil {
		t.Fatalf("run returned a transport error: %v", err)
	}
	if !result.Truncated {
		t.Error("Truncated = false, want true")
	}
	if total := len(result.Stdout) + len(result.Stderr); total != config.MaxOutputBytes {
		t.Errorf("retained %d combined bytes, want exactly %d", total, config.MaxOutputBytes)
	}
	waitForServerClose(t, server)
}

func TestSSHClientOutputWithinBudgetIsNotTruncated(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("12345", "67890", 0))

	config := testSSHClientConfig(server, identity, "")
	config.MaxOutputBytes = 10
	client, err := newSSHClient(config, nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	result, err := client.run(t.Context(), "produce exact output")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Truncated {
		t.Error("Truncated = true, want false for output exactly at budget")
	}
	if string(result.Stdout) != "12345" || string(result.Stderr) != "67890" {
		t.Errorf("stdout/stderr = %q/%q, want 12345/67890", result.Stdout, result.Stderr)
	}
	waitForServerClose(t, server)
}

func TestSSHClientPreservesOutputOnNonZeroExit(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execWrites("partial out", "partial err", 3))

	client, err := newSSHClient(testSSHClientConfig(server, identity, ""), nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	result, err := client.run(t.Context(), "exit with failure")
	if err == nil {
		t.Fatal("expected a non-zero exit error")
	}
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want *ssh.ExitError", err)
	}
	if exitErr.ExitStatus() != 3 {
		t.Errorf("exit status = %d, want 3", exitErr.ExitStatus())
	}
	if string(result.Stdout) != "partial out" || string(result.Stderr) != "partial err" {
		t.Errorf("stdout/stderr = %q/%q, want partial out/partial err", result.Stdout, result.Stderr)
	}
	if result.Truncated {
		t.Error("Truncated = true, want false")
	}
	waitForServerClose(t, server)
}

func TestSSHClientRejectsInvalidCommandBeforeDialing(t *testing.T) {
	identity := newTestIdentity(t, "")
	dialed := false
	config := sshClientConfig{
		Host:           "127.0.0.1",
		Port:           22,
		Username:       "operator",
		PrivateKey:     identity.privateKeyPEM,
		HostKeySHA256:  ssh.FingerprintSHA256(identity.publicKey),
		ConnectTimeout: time.Second,
		CommandTimeout: time.Second,
		MaxOutputBytes: 1024,
	}
	client, err := newSSHClient(config, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("dial must not be called for an invalid command")
	})
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	for _, command := range []string{"", "   ", "\n\t", "echo\x00injected"} {
		if _, err := client.run(t.Context(), command); !errors.Is(err, errSSHInvalidCommand) {
			t.Errorf("command %q: error = %v, want errSSHInvalidCommand", command, err)
		}
	}
	if dialed {
		t.Error("the transport was dialed for an invalid command")
	}
}

func TestSSHClientConcurrentRunsOwnSeparateConnections(t *testing.T) {
	identity := newTestIdentity(t, "")
	server := newLoopbackSSHServer(t, identity.publicKey, execEchoCommand())

	client, err := newSSHClient(testSSHClientConfig(server, identity, ""), nil)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}

	ctx := t.Context()
	const runs = 4
	var wg sync.WaitGroup
	errs := make([]error, runs)
	outputs := make([]string, runs)
	for index := 0; index < runs; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			command := fmt.Sprintf("command-%d", index)
			result, err := client.run(ctx, command)
			errs[index] = err
			outputs[index] = string(result.Stdout)
		}(index)
	}
	wg.Wait()

	for index := 0; index < runs; index++ {
		if errs[index] != nil {
			t.Errorf("run %d: %v", index, errs[index])
			continue
		}
		if want := fmt.Sprintf("command-%d", index); outputs[index] != want {
			t.Errorf("run %d stdout = %q, want %q", index, outputs[index], want)
		}
	}
	waitForServerClose(t, server)
}

// TestWatchContextStopJoinsStartedCloser proves stop waits for a closer the
// context has already launched: a closer that blocks must delay stop's return,
// and stop must then report that the closer ran.
func TestWatchContextStopJoinsStartedCloser(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	closerStarted := make(chan struct{})
	releaseCloser := make(chan struct{})
	stop := watchContext(ctx, func() {
		close(closerStarted)
		<-releaseCloser
	})

	cancel()
	waitForSignal(t, closerStarted, "the context closer to start")

	stopReturned := make(chan bool, 1)
	go func() { stopReturned <- stop() }()

	// While the closer is blocked, stop must not report completion.
	select {
	case <-stopReturned:
		t.Fatal("stop returned before the started closer finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseCloser)
	select {
	case closerDidRun := <-stopReturned:
		if !closerDidRun {
			t.Error("closerDidRun = false, want true for a closer that already ran")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return after the closer was released")
	}
}

// TestWatchContextStopPreventsCloser proves stop before cancellation reports
// that the closer did not run, and that cancelling afterwards never invokes a
// prevented closer.
func TestWatchContextStopPreventsCloser(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	closerRan := make(chan struct{})
	stop := watchContext(ctx, func() { close(closerRan) })

	if closerDidRun := stop(); closerDidRun {
		t.Error("closerDidRun = true, want false when stop precedes cancellation")
	}

	cancel()
	select {
	case <-closerRan:
		t.Error("the closer ran after stop reported it was prevented")
	case <-time.After(100 * time.Millisecond):
	}
}

// observedCloseConn is a real loopback transport whose Close is recorded but
// deliberately not forwarded. That lets the arbitration regression force the
// "connect watcher closed the connection, yet ssh.NewClientConn succeeded"
// outcome deterministically, without sleeps or production test hooks.
type observedCloseConn struct {
	net.Conn
	closeCalls *atomic.Int32
}

func (c *observedCloseConn) Close() error {
	c.closeCalls.Add(1)
	return nil
}

// TestSSHClientDialContextDiscardsClientClosedByConnectWatcher is the
// regression for connect-watcher/handshake arbitration. The caller context is
// already cancelled when dialContext runs, so the connect watcher is guaranteed
// to win and to invoke its closer; the injected connector records that close
// without forwarding it, so ssh.NewClientConn still reports success. dialContext
// must detect that the watcher won, discard the successful client, and return an
// error that preserves errors.Is(err, context.Canceled). No command may reach
// the server, which is exactly what a leaked but usable client would do.
func TestSSHClientDialContextDiscardsClientClosedByConnectWatcher(t *testing.T) {
	identity := newTestIdentity(t, "")
	var commandsRun atomic.Int32
	server := newLoopbackSSHServer(t, identity.publicKey, func(command string, channel ssh.Channel, _ <-chan struct{}) {
		commandsRun.Add(1)
		_, _ = io.WriteString(channel, command)
		sendExitStatus(channel, 0)
	})

	config := testSSHClientConfig(server, identity, "")
	config.ConnectTimeout = time.Minute

	var closeCalls atomic.Int32
	var rawConn net.Conn
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		// The connect context is intentionally ignored: the fixture needs the
		// raw transport to outlive its cancellation so the handshake can still
		// complete after the watcher has won.
		conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", address)
		if err != nil {
			return nil, err
		}
		rawConn = conn
		return &observedCloseConn{Conn: conn, closeCalls: &closeCalls}, nil
	}

	client, err := newSSHClient(config, dial)
	if err != nil {
		t.Fatalf("newSSHClient: %v", err)
	}
	defer func() {
		if rawConn != nil {
			_ = rawConn.Close()
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.run(ctx, "true"); err == nil {
		t.Fatal("run returned no error for a client closed by the connect watcher")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if closeCalls.Load() == 0 {
		t.Error("the connect watcher never closed the raw connection: the fixture did not exercise the arbitration")
	}
	if commandsRun.Load() != 0 {
		t.Errorf("%d commands reached the server: dialContext returned a usable client after the watcher won", commandsRun.Load())
	}
}
