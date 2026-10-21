package serverops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSH transport sentinels. Only classifications the later adapter must act on
// are stable; everything else stays an ordinary wrapped error.
var (
	// errSSHInvalidConfig classifies a rejected sshClientConfig.
	errSSHInvalidConfig = errors.New("serverops ssh: invalid client configuration")
	// errSSHInvalidCommand classifies a malformed trusted command.
	errSSHInvalidCommand = errors.New("serverops ssh: invalid trusted command")
	// errSSHHostKeyMismatch classifies a host key whose fingerprint does not
	// match the configured canonical OpenSSH SHA-256 fingerprint.
	errSSHHostKeyMismatch = errors.New("serverops ssh: host key fingerprint mismatch")
)

// sshSHA256FingerprintPrefix is the only accepted fingerprint prefix, matching
// ssh.FingerprintSHA256. The canonical form is "SHA256:<unpadded-base64>".
const sshSHA256FingerprintPrefix = "SHA256:"

// sshClientConfig is the immutable input used to construct one sshClient.
//
// Password, PrivateKey, and Passphrase are consumed during construction and
// never retained as caller-owned slices; the client keeps only the SSH auth
// method needed for one request-scoped runner. The slices are not mutated.
//
// HostKeySHA256 and CaptureHostKey form one mutually exclusive host-key policy:
// a canonical fingerprint is strictly verified, while CaptureHostKey records
// the presented fingerprint instead. CaptureHostKey is valid only with an empty
// HostKeySHA256; newSSHClient rejects every other combination.
type sshClientConfig struct {
	Host           string
	Port           int
	Username       string
	Password       []byte
	PrivateKey     []byte
	Passphrase     []byte
	HostKeySHA256  string
	CaptureHostKey bool
	ConnectTimeout time.Duration
	CommandTimeout time.Duration
	MaxOutputBytes int
}

// dialContextFunc opens the raw TCP connection for one SSH run. It mirrors
// (*net.Dialer).DialContext so tests can inject an in-process transport.
type dialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

// sshClient is a concrete, immutable SSH transport seam. It is safe for
// concurrent use because every run opens, owns, and closes its own TCP
// connection, SSH client, and session; no persistent connection is shared.
type sshClient struct {
	address            string
	username           string
	auth               ssh.AuthMethod
	hostKeyFingerprint string
	captureHostKey     bool
	connectTimeout     time.Duration
	commandTimeout     time.Duration
	maxOutputBytes     int
	dial               dialContextFunc
}

// newSSHClient validates an sshClientConfig, parses the selected credential once, and
// returns an immutable client. The optional dial function defaults to
// net.Dialer.DialContext; tests inject a loopback transport through it.
func newSSHClient(config sshClientConfig, dial dialContextFunc) (*sshClient, error) {
	// Defensive trim only: Service.connectionConfig rejects an untrimmed host or
	// username before it ever builds a config, so this must never be relied on to
	// normalize a displayed target into a different dialed one.
	host := strings.TrimSpace(config.Host)
	username := strings.TrimSpace(config.Username)

	if host == "" {
		return nil, fmt.Errorf("%w: host is blank", errSSHInvalidConfig)
	}
	if username == "" {
		return nil, fmt.Errorf("%w: username is blank", errSSHInvalidConfig)
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("%w: port %d is outside 1..65535", errSSHInvalidConfig, config.Port)
	}
	hasPassword := len(config.Password) > 0
	hasPrivateKey := len(config.PrivateKey) > 0
	if hasPassword == hasPrivateKey {
		return nil, fmt.Errorf("%w: exactly one authentication method is required", errSSHInvalidConfig)
	}
	if config.ConnectTimeout <= 0 {
		return nil, fmt.Errorf("%w: connect timeout must be positive", errSSHInvalidConfig)
	}
	if config.CommandTimeout <= 0 {
		return nil, fmt.Errorf("%w: command timeout must be positive", errSSHInvalidConfig)
	}
	if config.MaxOutputBytes <= 0 {
		return nil, fmt.Errorf("%w: max output bytes must be positive", errSSHInvalidConfig)
	}
	fingerprint, err := hostKeyPolicy(config.CaptureHostKey, config.HostKeySHA256)
	if err != nil {
		return nil, err
	}
	var auth ssh.AuthMethod
	if hasPassword {
		if len(config.Password) > maxPasswordBytes || hasASCIIControl(string(config.Password)) || len(bytes.TrimSpace(config.Password)) == 0 {
			return nil, fmt.Errorf("%w: password is invalid", errSSHInvalidConfig)
		}
		auth = ssh.Password(string(config.Password))
	} else {
		signer, parseErr := parseSSHSigner(config.PrivateKey, config.Passphrase)
		if parseErr != nil {
			// The wrapped parse error is diagnostic and never contains key or
			// passphrase material.
			return nil, fmt.Errorf("%w: parse private key: %w", errSSHInvalidConfig, parseErr)
		}
		auth = ssh.PublicKeys(signer)
	}
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	return &sshClient{
		address:            net.JoinHostPort(host, strconv.Itoa(config.Port)),
		username:           username,
		auth:               auth,
		hostKeyFingerprint: fingerprint,
		captureHostKey:     config.CaptureHostKey,
		connectTimeout:     config.ConnectTimeout,
		commandTimeout:     config.CommandTimeout,
		maxOutputBytes:     config.MaxOutputBytes,
		dial:               dial,
	}, nil
}

// hostKeyPolicy resolves the one accepted host-key mode. A strict request must
// supply a canonical fingerprint that is verified constant-time; capture mode
// is accepted only when it is explicitly requested and no fingerprint was
// supplied. Every mixed or empty combination fails closed.
func hostKeyPolicy(capture bool, value string) (string, error) {
	if !capture {
		return canonicalHostKeyFingerprint(value)
	}
	if value != "" {
		return "", fmt.Errorf("%w: host key capture requires an empty fingerprint", errSSHInvalidConfig)
	}
	return "", nil
}

// canonicalHostKeyFingerprint accepts only canonical OpenSSH SHA-256
// fingerprints ("SHA256:<unpadded-base64>") that decode to exactly 32 bytes.
// There is deliberately no MD5, skip, TOFU, or known-host fallback.
func canonicalHostKeyFingerprint(value string) (string, error) {
	if !strings.HasPrefix(value, sshSHA256FingerprintPrefix) {
		return "", fmt.Errorf("%w: host key fingerprint must use the %q prefix", errSSHInvalidConfig, sshSHA256FingerprintPrefix)
	}
	encoded := strings.TrimPrefix(value, sshSHA256FingerprintPrefix)
	if encoded == "" {
		return "", fmt.Errorf("%w: host key fingerprint is empty", errSSHInvalidConfig)
	}
	digest, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: host key fingerprint is not unpadded base64", errSSHInvalidConfig)
	}
	if len(digest) != sha256.Size {
		return "", fmt.Errorf("%w: host key fingerprint decodes to %d bytes, want %d", errSSHInvalidConfig, len(digest), sha256.Size)
	}
	canonical := sshSHA256FingerprintPrefix + base64.RawStdEncoding.EncodeToString(digest)
	if canonical != value {
		return "", fmt.Errorf("%w: host key fingerprint is not canonical", errSSHInvalidConfig)
	}
	return canonical, nil
}

// canonicalObservedHostKeyFingerprint validates one fingerprint produced by the
// transport before it is exposed to a caller. ssh.FingerprintSHA256 always emits
// the canonical form, so this is a defensive canonicality and bound gate: an
// unexpected value fails closed as an authentication failure instead of being
// returned. Observed values are classified separately from caller-supplied
// configuration so they can never be confused with an invalid Connection.
func canonicalObservedHostKeyFingerprint(value string) (string, error) {
	canonical, err := canonicalHostKeyFingerprint(value)
	if err != nil {
		return "", fmt.Errorf("%w: presented host key fingerprint is not canonical", errSSHHostKeyMismatch)
	}
	if len(canonical) > maxFingerprintBytes {
		return "", fmt.Errorf("%w: presented host key fingerprint exceeds %d bytes", errSSHHostKeyMismatch, maxFingerprintBytes)
	}
	return canonical, nil
}

// parseSSHSigner parses the private key from private copies so the caller-owned
// slices are never mutated, and returns only the signer. Every internal copy is
// zeroed before the function returns, on both the success and the error path, so
// the plain key material does not outlive the parse.
func parseSSHSigner(privateKey, passphrase []byte) (ssh.Signer, error) {
	keyCopy := append([]byte(nil), privateKey...)
	defer zeroBytes(keyCopy)
	if len(passphrase) == 0 {
		return ssh.ParsePrivateKey(keyCopy)
	}
	passCopy := append([]byte(nil), passphrase...)
	defer zeroBytes(passCopy)
	return ssh.ParsePrivateKeyWithPassphrase(keyCopy, passCopy)
}

// hostKeyCapture is the per-run sink for an opted-in host-key capture. It is a
// local value owned by exactly one run, so concurrent runs never share or
// overwrite each other's observed fingerprint.
type hostKeyCapture struct {
	fingerprint string
}

// clientConfig builds the per-connection SSH client configuration. Host key
// verification is mandatory: a strict client compares the presented key against
// its pinned fingerprint, while a capture client records the presented
// fingerprint into the per-run capture sink. There is no skip or insecure
// option.
func (c *sshClient) clientConfig(capture *hostKeyCapture) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: c.username,
		Auth: []ssh.AuthMethod{c.auth},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			return c.verifyHostKey(key, capture)
		},
	}
}

// verifyHostKey handles the configured host-key mode for one handshake. Strict
// clients compare the presented fingerprint with the configured canonical
// fingerprint in constant time. Capture clients validate and record the
// presented fingerprint into their own per-run sink and accept the key for this
// one probe only.
func (c *sshClient) verifyHostKey(key ssh.PublicKey, capture *hostKeyCapture) error {
	presented := ssh.FingerprintSHA256(key)
	if c.captureHostKey {
		observed, err := canonicalObservedHostKeyFingerprint(presented)
		if err != nil {
			return err
		}
		if capture != nil {
			capture.fingerprint = observed
		}
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(presented), []byte(c.hostKeyFingerprint)) != 1 {
		return fmt.Errorf("%w: presented %s", errSSHHostKeyMismatch, presented)
	}
	return nil
}

// dialContext opens one raw TCP connection and completes the SSH handshake
// under a single connect budget while still honoring caller cancellation. The
// returned client owns the connection and must be closed by the caller.
//
// ConnectTimeout is one total dial+handshake budget, not one budget per phase:
// the effective connect context is created once, used as the DialContext
// context, and its existing deadline is copied onto the raw connection before
// ssh.NewClientConn runs. A slow dial therefore leaves the handshake only the
// budget that remains.
//
// The connect watcher and the handshake are arbitrated once, after
// ssh.NewClientConn returns. If the watcher closed the raw connection, the
// successful-or-failed handshake is discarded and the connect context's cause
// is returned instead of a client.
func (c *sshClient) dialContext(ctx context.Context, capture *hostKeyCapture) (*ssh.Client, error) {
	// WithTimeout keeps the earlier of the caller deadline and the configured
	// connect timeout, so caller cancellation always wins.
	connectCtx, cancel := context.WithTimeout(ctx, c.connectTimeout)
	defer cancel()

	// Install the connection watcher before dialing so a dial and the handshake
	// share exactly one cancellation path.
	rawConn, err := c.dial(connectCtx, "tcp", c.address)
	if err != nil {
		if ctxErr := connectCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("serverops ssh: dial %s: %w", c.address, ctxErr)
		}
		return nil, fmt.Errorf("serverops ssh: dial %s: %w", c.address, err)
	}

	// Apply the effective connect context's own deadline. A zero deadline means
	// the context was created without one, in which case the raw connection
	// keeps no deadline.
	if deadline, ok := connectCtx.Deadline(); ok {
		if err := rawConn.SetDeadline(deadline); err != nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("serverops ssh: set handshake deadline: %w", err)
		}
	}

	// The watcher observes the effective connect context, so it interrupts a
	// stalled handshake on either caller cancellation or the connect budget.
	stopWatcher := watchContext(connectCtx, func() { _ = rawConn.Close() })

	sshConn, channels, requests, err := ssh.NewClientConn(rawConn, c.address, c.clientConfig(capture))
	// Arbitrate the handshake exactly once. stopWatcher reports whether the
	// connect watcher already invoked its closer: if it did, the watcher won
	// the cancellation race and rawConn may be closed, so a successful
	// NewClientConn is not trustworthy and must never escape as a client.
	if closerDidRun := stopWatcher(); closerDidRun {
		if sshConn != nil {
			_ = sshConn.Close()
		}
		return nil, fmt.Errorf("serverops ssh: handshake: %w", connectCtx.Err())
	}
	if err != nil {
		_ = rawConn.Close()
		if ctxErr := connectCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("serverops ssh: handshake: %w", ctxErr)
		}
		return nil, fmt.Errorf("serverops ssh: handshake: %w", err)
	}
	// stopWatcher reported that it prevented the connect watcher before the
	// closer could run, so no late closer can touch the connection now. Command
	// execution owns its own timeout/cancellation mechanism, so the raw
	// connection must not keep the connect deadline.
	if err := rawConn.SetDeadline(time.Time{}); err != nil {
		_ = sshConn.Close()
		return nil, fmt.Errorf("serverops ssh: clear handshake deadline: %w", err)
	}
	return ssh.NewClient(sshConn, channels, requests), nil
}

// watchContext closes the owned connection as soon as ctx is done, until the
// returned stop function is called. The stop function reports the exact
// arbitration outcome: closerDidRun is true when the context callback ran (or
// had already been launched) and invoked closeOwned, and false when stop
// prevented the callback so the closer will never run.
//
// The completion protocol is explicit. context.AfterFunc's stop function does
// not wait for a callback that has started, so the callback itself closes
// callbackDone when it returns. When stop prevented the callback it returns
// immediately; otherwise it waits for callbackDone, which joins a callback
// that AfterFunc has launched but that has not yet executed.
func watchContext(ctx context.Context, closeOwned func()) (stop func() (closerDidRun bool)) {
	callbackDone := make(chan struct{})
	stopCallback := context.AfterFunc(ctx, func() {
		defer close(callbackDone)
		closeOwned()
	})
	return func() (closerDidRun bool) {
		if stopCallback() {
			// The callback was prevented before it could start, so there is no
			// lifecycle work to join and the closer will never run.
			return false
		}
		// The context won and the callback was launched. Wait until it has
		// finished closing the owned connection before reporting that the
		// closer ran.
		<-callbackDone
		return true
	}
}

// sshRunResult is the bounded output of one command. Stdout and Stderr are
// always separate byte slices; Truncated reports that the combined budget was
// exhausted; HostKeyFingerprint carries the canonical OpenSSH SHA-256
// fingerprint observed during an opted-in capture handshake and is empty for
// every strict verification.
type sshRunResult struct {
	Stdout             []byte
	Stderr             []byte
	Truncated          bool
	HostKeyFingerprint string
}

// run executes one trusted command on a fresh connection. trustedCommand is
// adapter-constructed internal input, never model or user input, so run only
// rejects blank commands and embedded NUL and does not build product commands.
func (c *sshClient) run(ctx context.Context, trustedCommand string) (sshRunResult, error) {
	if err := validateTrustedCommand(trustedCommand); err != nil {
		return sshRunResult{}, err
	}

	// The capture sink is constructed for this run only, so the host-key
	// callback writes request-scoped state and concurrent runs never share or
	// overwrite each other's observation.
	var capture hostKeyCapture
	client, err := c.dialContext(ctx, &capture)
	if err != nil {
		return sshRunResult{}, err
	}
	// The owner of the transport is established before any session work so the
	// whole post-handshake lifecycle, including NewSession, is bounded.
	ownedClient := &ownedSSHClient{client: client}
	defer ownedClient.close()
	result, err := c.runCmd(ctx, ownedClient, trustedCommand)
	if c.captureHostKey {
		result.HostKeyFingerprint = capture.fingerprint
	}
	return result, err
}

// runCmd executes one trusted command on an already-dialed connection. The
// effective command context is created before NewSession so channel-open is
// covered by the same cancellation and timeout budget as Session.Run.
func (c *sshClient) runCmd(ctx context.Context, owned *ownedSSHClient, trustedCommand string) (sshRunResult, error) {
	// The caller's context may carry an earlier deadline; WithTimeout keeps the
	// earlier of the two so caller cancellation always wins.
	runCtx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	// A command timeout or caller cancellation must close the owned transport
	// for the entire post-handshake lifecycle, including a NewSession that is
	// blocked waiting for a server that never services channel-open. The
	// deferred stop joins a started watcher; its arbitration boolean is not
	// needed here because owned is always cleaned up and runCtx.Err decides
	// the result.
	stopWatcher := watchContext(runCtx, owned.close)
	defer stopWatcher()

	session, err := owned.client.NewSession()
	if err != nil {
		if ctxErr := runCtx.Err(); ctxErr != nil {
			return sshRunResult{}, fmt.Errorf("serverops ssh: open session: %w", ctxErr)
		}
		return sshRunResult{}, fmt.Errorf("serverops ssh: open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	output := &combinedOutputBudget{remaining: c.maxOutputBytes}
	stdout := &budgetWriter{budget: output}
	stderr := &budgetWriter{budget: output}
	session.Stdout = stdout
	session.Stderr = stderr

	runDone := make(chan error, 1)
	go func() {
		runDone <- session.Run(trustedCommand)
	}()

	var runErr error
	select {
	case runErr = <-runDone:
	case <-runCtx.Done():
		// Close the owned transport so the session wait and its copy
		// goroutines observe the cancellation and exit, then join the run
		// goroutine before returning.
		owned.close()
		runErr = <-runDone
	}

	result := sshRunResult{
		Stdout:    stdout.bytes(),
		Stderr:    stderr.bytes(),
		Truncated: output.isTruncated(),
	}
	if ctxErr := runCtx.Err(); ctxErr != nil {
		return result, fmt.Errorf("serverops ssh: run command: %w", ctxErr)
	}
	if runErr != nil {
		return result, wrapSSHRunError(runErr)
	}
	return result, nil
}

// ownedSSHClient is the single owner of one SSH client's lifetime. close is
// idempotent and safe to call from a context watcher and from cleanup.
type ownedSSHClient struct {
	client *ssh.Client
	once   sync.Once
}

func (o *ownedSSHClient) close() {
	o.once.Do(func() { _ = o.client.Close() })
}

// validateTrustedCommand rejects the only malformed trusted commands the
// transport must defend against.
func validateTrustedCommand(trustedCommand string) error {
	if strings.TrimSpace(trustedCommand) == "" {
		return fmt.Errorf("%w: command is blank", errSSHInvalidCommand)
	}
	if strings.IndexByte(trustedCommand, 0) >= 0 {
		return fmt.Errorf("%w: command contains NUL", errSSHInvalidCommand)
	}
	return nil
}

// wrapSSHRunError keeps *ssh.ExitError discoverable through errors.As while
// classifying remote failures with a status code.
func wrapSSHRunError(err error) error {
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("serverops ssh: remote command exited with status %d: %w", exitErr.ExitStatus(), err)
	}
	return fmt.Errorf("serverops ssh: run command: %w", err)
}

// combinedOutputBudget is the one byte budget shared by the stdout and stderr
// writers of a run. Its mutex makes it safe for the two concurrent SSH copy
// goroutines.
type combinedOutputBudget struct {
	mu        sync.Mutex
	remaining int
	truncated bool
}

// reserve reports how many of n bytes may be retained. Bytes beyond the budget
// are dropped and recorded as truncation.
func (b *combinedOutputBudget) reserve(n int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 {
		return 0
	}
	if b.remaining <= 0 {
		b.truncated = true
		return 0
	}
	if n <= b.remaining {
		b.remaining -= n
		return n
	}
	allowed := b.remaining
	b.remaining = 0
	b.truncated = true
	return allowed
}

func (b *combinedOutputBudget) isTruncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// budgetWriter retains at most the shared budget and always reports a
// successful full write so remote output can neither force unbounded
// allocation nor turn truncation into a transport error. Each writer is used
// by exactly one SSH copy goroutine.
type budgetWriter struct {
	budget *combinedOutputBudget
	dst    bytes.Buffer
}

func (w *budgetWriter) Write(p []byte) (int, error) {
	if allowed := w.budget.reserve(len(p)); allowed > 0 {
		w.dst.Write(p[:allowed])
	}
	return len(p), nil
}

func (w *budgetWriter) bytes() []byte {
	return w.dst.Bytes()
}
