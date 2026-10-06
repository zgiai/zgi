package mailprotocol

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type fixtureDialer struct {
	mu             sync.Mutex
	certificate    tls.Certificate
	roots          *x509.CertPool
	addresses      []string
	commands       []string
	mode           string
	uidValidity    uint32
	body           string
	delayAuth      bool
	smtpMechanisms string
	imapID         bool
}

func newFixture(t *testing.T) *fixtureDialer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"imap.gmail.com", "smtp.gmail.com", "imap.qq.com", "smtp.qq.com", "imap.163.com", "smtp.163.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &fixtureDialer{certificate: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots: pool, uidValidity: 9, body: "Content-Type: text/plain; charset=utf-8\r\n\r\n你好，邮件正文"}
}
func (f *fixtureDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("unexpected network")
	}
	f.mu.Lock()
	f.addresses = append(f.addresses, address)
	f.mu.Unlock()
	a, b := net.Pipe()
	go func() {
		defer b.Close()
		_ = b.SetDeadline(time.Now().Add(5 * time.Second))
		var conn net.Conn = b
		if !strings.HasSuffix(address, ":587") {
			secured := tls.Server(b, &tls.Config{Certificates: []tls.Certificate{f.certificate}, MinVersion: tls.VersionTLS12})
			if secured.HandshakeContext(ctx) != nil {
				return
			}
			conn = secured
		}
		if strings.HasPrefix(address, "imap.") {
			f.imap(conn)
		} else {
			f.smtp(conn)
		}
	}()
	return a, nil
}
func (f *fixtureDialer) record(command string) {
	f.mu.Lock()
	f.commands = append(f.commands, command)
	f.mu.Unlock()
}
func (f *fixtureDialer) snapshot() ([]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.addresses...), append([]string(nil), f.commands...)
}
func (f *fixtureDialer) smtp(conn net.Conn) {
	reader := bufio.NewReader(conn)
	recipients := 0
	_, _ = io.WriteString(conn, "220 fixture ESMTP\r\n")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		verb := strings.SplitN(line, " ", 2)[0]
		f.record(verb)
		switch verb {
		case "EHLO":
			mechanisms := f.smtpMechanisms
			if mechanisms == "" {
				mechanisms = "PLAIN"
			}
			_, _ = io.WriteString(conn, "250-fixture\r\n250 AUTH "+mechanisms+"\r\n")
		case "AUTH":
			if strings.HasPrefix(line, "AUTH LOGIN") {
				_, _ = io.WriteString(conn, "334 VXNlcm5hbWU6\r\n")
				if _, err := reader.ReadString('\n'); err != nil {
					return
				}
				_, _ = io.WriteString(conn, "334 UGFzc3dvcmQ6\r\n")
				if _, err := reader.ReadString('\n'); err != nil {
					return
				}
				f.record("LOGIN_EXCHANGE")
			}
			if f.delayAuth {
				<-time.After(300 * time.Millisecond)
				return
			}
			if f.mode == "auth_reject" {
				_, _ = io.WriteString(conn, "535 5.7.80 username and password not accepted\r\n")
			} else {
				_, _ = io.WriteString(conn, "235 2.7.0 authenticated\r\n")
			}
		case "MAIL":
			_, _ = io.WriteString(conn, "250 sender accepted\r\n")
		case "RCPT":
			recipients++
			if f.mode == "recipient_reject" && recipients == 2 {
				_, _ = io.WriteString(conn, "550 5.1.1 recipient rejected\r\n")
			} else {
				_, _ = io.WriteString(conn, "250 recipient accepted\r\n")
			}
		case "RSET":
			_, _ = io.WriteString(conn, "250 reset\r\n")
		case "DATA":
			_, _ = io.WriteString(conn, "354 continue\r\n")
			for {
				bodyLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if bodyLine == ".\r\n" {
					break
				}
			}
			f.record("DATA_COMPLETE")
			if f.mode == "unknown" {
				return
			}
			if f.mode == "data_reject" {
				_, _ = io.WriteString(conn, "554 5.7.1 refused\r\n")
			} else if f.mode == "data_temporary" {
				_, _ = io.WriteString(conn, "451 4.3.0 temporarily refused\r\n")
			} else {
				_, _ = io.WriteString(conn, "250 2.0.0 queued\r\n")
			}
		case "QUIT":
			if f.mode == "quit_failure" {
				return
			}
			_, _ = io.WriteString(conn, "221 closing\r\n")
			return
		default:
			return
		}
	}
}
func (f *fixtureDialer) imap(conn net.Conn) {
	capabilities := "IMAP4rev1"
	if f.imapID {
		capabilities += " ID"
	}
	identified := false
	if f.mode == "no_capabilities" || f.mode == "malformed_capabilities" {
		_, _ = io.WriteString(conn, "* OK private-greeting-SENTINEL\r\n")
	} else {
		_, _ = io.WriteString(conn, "* OK [CAPABILITY "+capabilities+"] ready\r\n")
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return
		}
		tag, verb := fields[0], fields[1]
		f.record(verb)
		switch verb {
		case "CAPABILITY":
			if f.mode == "malformed_capabilities" {
				_, _ = fmt.Fprintf(conn, "* UNEXPECTED private-capability-SENTINEL\r\n%s BAD not available\r\n", tag)
				continue
			}
			_, _ = fmt.Fprintf(conn, "* CAPABILITY %s\r\n%s OK capabilities\r\n", capabilities, tag)
		case "LOGIN":
			if f.mode == "imap_reject" {
				_, _ = fmt.Fprintf(conn, "%s NO authentication rejected\r\n", tag)
			} else {
				_, _ = fmt.Fprintf(conn, "%s OK login\r\n", tag)
			}
		case "ID":
			if !f.imapID || !strings.Contains(line, "ZGI") {
				_, _ = fmt.Fprintf(conn, "%s NO ID rejected\r\n", tag)
				continue
			}
			identified = true
			_, _ = fmt.Fprintf(conn, "* ID NIL\r\n%s OK ID accepted\r\n", tag)
		case "EXAMINE":
			if (f.imapID && !identified) || f.mode == "unsafe_login" {
				_, _ = fmt.Fprintf(conn, "%s NO Unsafe Login\r\n", tag)
				continue
			}
			omitValidity := f.mode == "uid_codes_absent" || f.mode == "uid_codes_missing_everywhere"
			omitNext := omitValidity || f.mode == "uid_next_absent" || f.mode == "uid_search_empty"
			response := "* FLAGS (\\Seen)\r\n* 1 EXISTS\r\n"
			if !omitValidity {
				response += fmt.Sprintf("* OK [UIDVALIDITY %d] valid\r\n", f.uidValidity)
			}
			if !omitNext {
				response += "* OK [UIDNEXT 8] next\r\n"
			}
			_, _ = fmt.Fprintf(conn, "%s%s OK [READ-ONLY] examined\r\n", response, tag)
		case "STATUS":
			if f.mode == "uid_codes_missing_everywhere" || f.mode == "uid_search_empty" {
				_, _ = fmt.Fprintf(conn, "* STATUS \"INBOX\" (MESSAGES 1)\r\n%s OK status\r\n", tag)
				continue
			}
			_, _ = fmt.Fprintf(conn, "* STATUS \"INBOX\" (MESSAGES 1 UIDVALIDITY %d UIDNEXT 8)\r\n%s OK status\r\n", f.uidValidity, tag)
		case "LIST":
			_, _ = fmt.Fprintf(conn, "* LIST () \"/\" INBOX\r\n%s OK folders\r\n", tag)
		case "UID":
			f.record(strings.TrimSpace(strings.Join(fields[2:], " ")))
			if strings.Contains(line, "SEARCH") {
				if f.mode == "uid_search_empty" {
					_, _ = fmt.Fprintf(conn, "* SEARCH\r\n%s OK searched\r\n", tag)
					continue
				}
				_, _ = fmt.Fprintf(conn, "* SEARCH 7\r\n%s OK searched\r\n", tag)
				continue
			}
			switch {
			case strings.Contains(line, "RFC822.SIZE"):
				_, _ = fmt.Fprintf(conn, "* 1 FETCH (UID 7 RFC822.SIZE %d)\r\n", len(f.body))
			case strings.Contains(line, "BODY.PEEK"):
				if f.mode == "huge_literal" {
					_, _ = fmt.Fprintf(conn, "* 1 FETCH (UID 7 BODY[] {4294967295}\r\n")
					return
				}
				_, _ = fmt.Fprintf(conn, "* 1 FETCH (UID 7 BODY[]<0> {%d}\r\n%s)\r\n", len(f.body), f.body)
			default:
				subject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte("中文主题")) + "?="
				_, _ = fmt.Fprintf(conn, "* 1 FETCH (UID 7 ENVELOPE (\"Sun, 6 Sep 2026 12:00:00 +0000\" \"%s\" ((NIL NIL \"sender\" \"example.com\")) NIL NIL NIL NIL NIL NIL NIL))\r\n", subject)
			}
			_, _ = fmt.Fprintf(conn, "%s OK fetched\r\n", tag)
		case "LOGOUT":
			_, _ = fmt.Fprintf(conn, "* BYE\r\n%s OK logout\r\n", tag)
			return
		default:
			_, _ = fmt.Fprintf(conn, "%s BAD unsupported\r\n", tag)
		}
	}
}
func testClient(t *testing.T, f *fixtureDialer, port int) *Client {
	t.Helper()
	c, err := New(Options{Dialer: f, RootCAs: f.roots, SMTPPort: port, HandleKey: strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func testInput(usage string) providers.CredentialInput {
	return providers.CredentialInput{Values: map[string]string{"email": "User.Name+tag@gmail.com", "app_password": "AbcdefghijklMNOP"}, MailUsage: usage, ResourceScope: providers.ResourceScope{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectionID: uuid.New(), CredentialVersion: 1, BindingID: uuid.New(), BindingRevision: 1, LogicalOperationID: uuid.NewString(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}}}
}

func TestVerificationUsesOnlySelectedProtocolsAndNeverSends(t *testing.T) {
	for _, usage := range []string{"read", "send", "read_send"} {
		t.Run(usage, func(t *testing.T) {
			f := newFixture(t)
			c := testClient(t, f, 465)
			v, err := c.VerifyCredentials(context.Background(), testInput(usage))
			if err != nil {
				t.Fatal(err)
			}
			addresses, commands := f.snapshot()
			joined := strings.Join(commands, " ")
			if strings.Contains(joined, "DATA") || strings.Contains(joined, "MAIL") || strings.Contains(joined, "RCPT") || strings.Contains(joined, "SELECT") {
				t.Fatal("verification performed a write")
			}
			if usage == "send" && (len(addresses) != 1 || addresses[0] != "smtp.gmail.com:465" || strings.Contains(joined, "LOGIN")) {
				t.Fatal("send-only used IMAP")
			}
			if usage == "read" && (len(addresses) != 1 || addresses[0] != "imap.gmail.com:993" || strings.Contains(joined, "AUTH")) {
				t.Fatal("read-only used SMTP")
			}
			if usage != "read" && v.Evidence.Mail.Operations[len(v.Evidence.Mail.Operations)-1].Availability != "conditional" {
				t.Fatal("SMTP auth claimed delivery")
			}
		})
	}
}
func TestPartialVerificationAndTLSFailuresNeverBecomeReady(t *testing.T) {
	f := newFixture(t)
	f.mode = "auth_reject"
	c := testClient(t, f, 465)
	v, err := c.VerifyCredentials(context.Background(), testInput("read_send"))
	if err == nil || v.Evidence.Mail.IMAPStatus != "verified" || v.Evidence.Mail.SMTPStatus != "failed" || v.Evidence.Mail.CredentialVerified {
		t.Fatal("partial verification promoted")
	}
	f = newFixture(t)
	c = testClient(t, f, 587)
	_, err = c.VerifyCredentials(context.Background(), testInput("send"))
	if providers.FailureOf(err).Code != core.ErrorCodeMailTLSRequired {
		t.Fatal("missing STARTTLS accepted")
	}
	_, commands := f.snapshot()
	for _, command := range commands {
		if command == "AUTH" {
			t.Fatal("plaintext credentials sent")
		}
	}
	f = newFixture(t)
	c = testClient(t, f, 465)
	c.roots = x509.NewCertPool()
	_, err = c.VerifyCredentials(context.Background(), testInput("send"))
	if providers.FailureOf(err).Code != core.ErrorCodeMailTLSRequired {
		t.Fatal("untrusted certificate accepted")
	}
}
func TestSMTPFinalResultSemantics(t *testing.T) {
	for _, mode := range []string{"accepted", "unknown", "data_reject", "data_temporary", "recipient_reject", "quit_failure"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.mode = mode
			c := testClient(t, f, 465)
			result, err := c.InvokeCredentials(context.Background(), "gmail.message.send", testInput("send"), map[string]any{"to": []string{"receiver@example.com", "other@example.com"}, "subject": "中文主题", "text": "中文正文"})
			switch mode {
			case "accepted", "quit_failure":
				if err != nil || result.Value["status"] != "accepted_by_server" {
					t.Fatalf("accepted result: %v", err)
				}
			case "unknown":
				if err == nil || !providers.FailureOf(err).Unknown {
					t.Fatal("lost final response must be unknown")
				}
			case "data_reject":
				if err == nil || providers.FailureOf(err).Code != core.ErrorCodeMailSubmissionRejected || providers.FailureOf(err).Unknown || providers.FailureOf(err).Retryable {
					t.Fatal("explicit final rejection misclassified")
				}
			case "data_temporary":
				if err == nil || providers.FailureOf(err).Code != core.ErrorCodeMailSubmissionRejected || providers.FailureOf(err).Unknown || !providers.FailureOf(err).Retryable {
					t.Fatal("temporary final rejection misclassified")
				}
			case "recipient_reject":
				if err == nil || providers.FailureOf(err).Code != core.ErrorCodeMailRecipientRejected {
					t.Fatal("recipient rejection lost")
				}
			}
			_, commands := f.snapshot()
			count := 0
			for _, command := range commands {
				if command == "DATA_COMPLETE" {
					count++
				}
			}
			if count > 1 {
				t.Fatal("SMTP retried")
			}
			if mode == "recipient_reject" && count != 0 {
				t.Fatal("partial send occurred")
			}
		})
	}
}
func TestMailHandlesAreScopedAndReadsUsePeek(t *testing.T) {
	f := newFixture(t)
	c := testClient(t, f, 465)
	in := testInput("read")
	result, err := c.InvokeCredentials(context.Background(), "gmail.message.list", in, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	messages := result.Value["messages"].([]map[string]any)
	if len(messages) != 1 {
		t.Fatal("missing list item")
	}
	if messages[0]["subject"] != "中文主题" {
		t.Fatal("MIME subject not decoded")
	}
	handle := messages[0]["message_handle"].(string)
	result, err = c.InvokeCredentials(context.Background(), "gmail.message.get", in, map[string]any{"message_handle": handle})
	if err != nil || result.Value["text"] != "你好，邮件正文" {
		t.Fatalf("read: %v", err)
	}
	_, commands := f.snapshot()
	if !strings.Contains(strings.Join(commands, " "), "BODY.PEEK[]<0.1048576>") {
		t.Fatal("body read did not preserve unread")
	}
	before, _ := f.snapshot()
	other := in
	other.ResourceScope.OwnerAccountID = uuid.New()
	_, err = c.InvokeCredentials(context.Background(), "gmail.message.get", other, map[string]any{"message_handle": handle})
	after, _ := f.snapshot()
	if err == nil || len(before) != len(after) {
		t.Fatal("cross-owner handle reached provider")
	}
	f.uidValidity++
	_, err = c.InvokeCredentials(context.Background(), "gmail.message.get", in, map[string]any{"message_handle": handle})
	if providers.FailureOf(err).Code != core.ErrorCodeMailReferenceExpired {
		t.Fatal("UIDVALIDITY change did not expire handle")
	}
}

// RFC 3501 lets a server withhold UIDNEXT and UIDVALIDITY from EXAMINE. A bare
// message list carries no reference, so that must not become reference_expired.
func TestReadsRecoverUIDRangeFromStatus(t *testing.T) {
	for _, mode := range []string{"uid_next_absent", "uid_codes_absent"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			f.mode = mode
			c := testClient(t, f, 465)
			result, err := c.InvokeCredentials(context.Background(), "gmail.message.list", testInput("read"), map[string]any{})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if messages := result.Value["messages"].([]map[string]any); len(messages) != 1 {
				t.Fatal("missing list item")
			}
			_, commands := f.snapshot()
			if !strings.Contains(strings.Join(commands, " "), "STATUS") {
				t.Fatal("UID range was not recovered with STATUS")
			}
		})
	}
}

// 163 omits UIDNEXT and UIDVALIDITY from EXAMINE and reports neither via
// STATUS. Listing must still work by deriving the bound from the UIDs.
func TestReadsDeriveUIDBoundWhenServerWithholdsUIDMetadata(t *testing.T) {
	f := newFixture(t)
	f.mode = "uid_codes_missing_everywhere"
	c := testClient(t, f, 465)
	in := testInput("read")
	result, err := c.InvokeCredentials(context.Background(), "gmail.message.list", in, map[string]any{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	messages := result.Value["messages"].([]map[string]any)
	if len(messages) != 1 {
		t.Fatal("missing list item")
	}
	_, commands := f.snapshot()
	if !strings.Contains(strings.Join(commands, " "), "SEARCH") {
		t.Fatal("UID bound was not derived from UID SEARCH")
	}
	handle := messages[0]["message_handle"].(string)
	if _, err = c.InvokeCredentials(context.Background(), "gmail.message.get", in, map[string]any{"message_handle": handle}); err != nil {
		t.Fatalf("handle from a UID-less mailbox: %v", err)
	}

	f = newFixture(t)
	f.mode = "uid_codes_missing_everywhere"
	c = testClient(t, f, 465)
	v, err := c.VerifyCredentials(context.Background(), testInput("read"))
	if err != nil || v.Evidence.Mail.IMAPStatus != "verified" {
		t.Fatalf("verification rejected a listable mailbox: %v", err)
	}
}

func TestUIDBoundWithoutUIDsFailsVerificationUpFront(t *testing.T) {
	f := newFixture(t)
	f.mode = "uid_search_empty"
	c := testClient(t, f, 465)
	_, err := c.InvokeCredentials(context.Background(), "gmail.message.list", testInput("read"), map[string]any{})
	if providers.FailureOf(err).Code != core.ErrorCodeMailUidStateUnavailable {
		t.Fatalf("list failure: %v", err)
	}

	f = newFixture(t)
	f.mode = "uid_search_empty"
	c = testClient(t, f, 465)
	v, err := c.VerifyCredentials(context.Background(), testInput("read"))
	if providers.FailureOf(err).Code != core.ErrorCodeMailUidStateUnavailable {
		t.Fatalf("verification failure: %v", err)
	}
	if v.Evidence.Mail.IMAPStatus != "failed" || v.Evidence.Mail.CredentialVerified {
		t.Fatal("mailbox without a UID bound was promoted")
	}
}
func TestCancellationAndResourceBounds(t *testing.T) {
	f := newFixture(t)
	f.delayAuth = true
	c := testClient(t, f, 465)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.VerifyCredentials(ctx, testInput("send"))
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("cancellation did not close socket")
	}
	f = newFixture(t)
	f.mode = "huge_literal"
	c = testClient(t, f, 465)
	in := testInput("read")
	handle, err := c.sealReference(in, reference{Kind: "message", Mailbox: "INBOX", Validity: 9, UID: 7})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.InvokeCredentials(context.Background(), "gmail.message.get", in, map[string]any{"message_handle": handle})
	if err == nil {
		t.Fatal("oversized declared literal accepted")
	}
	if _, _, err := plainText([]byte("Subject: " + strings.Repeat("x", maxHeaderBytes) + "\r\n\r\nbody")); err == nil {
		t.Fatal("oversized header accepted")
	}
	text, truncated, err := plainText([]byte("Content-Type: text/plain\r\n\r\n" + strings.Repeat("x", maxTextBytes+10)))
	if err != nil || !truncated || len(text) > maxTextBytes {
		t.Fatal("body output not bounded")
	}
}

func TestMailReferenceTamperingAndScopeChangesNeverDial(t *testing.T) {
	f := newFixture(t)
	c := testClient(t, f, 465)
	in := testInput("read")
	handle, err := c.sealReference(in, reference{Kind: "message", Mailbox: "INBOX", Validity: 9, UID: 7})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*providers.CredentialInput){func(v *providers.CredentialInput) { v.ResourceScope.OrganizationID = uuid.New() }, func(v *providers.CredentialInput) { v.ResourceScope.ConnectionID = uuid.New() }, func(v *providers.CredentialInput) { v.ResourceScope.CredentialVersion++ }, func(v *providers.CredentialInput) { v.ResourceScope.BindingRevision++ }, func(v *providers.CredentialInput) { v.ResourceScope.Target.ID = uuid.NewString() }} {
		other := in
		mutate(&other)
		if _, err := c.InvokeCredentials(context.Background(), "gmail.message.get", other, map[string]any{"message_handle": handle}); err == nil {
			t.Fatal("cross-scope handle accepted")
		}
	}
	if _, err := c.openReference(in, handle[:len(handle)-3]+"aaa", "message"); err == nil {
		t.Fatal("tampered handle accepted")
	}
	c.now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, err := c.openReference(in, handle, "message"); err == nil {
		t.Fatal("expired handle accepted")
	}
	addresses, _ := f.snapshot()
	if len(addresses) != 0 {
		t.Fatal("invalid handle reached protocol")
	}
}
func TestMailMIMEOutputDoesNotExecuteHTMLOrIncludeAttachments(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=test\r\n\r\n--test\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>你好</p><script>secret_script()</script><img src=\"https://tracking.invalid/pixel\">\r\n--test\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=private.txt\r\n\r\nattachment secret\r\n--test--\r\n"
	text, _, err := plainText([]byte(raw))
	if err != nil || !strings.Contains(text, "你好") || strings.Contains(text, "script") || strings.Contains(text, "https:") || strings.Contains(text, "attachment secret") {
		t.Fatalf("unsafe MIME projection: %v", err)
	}
	f := newFixture(t)
	c := testClient(t, f, 465)
	_, err = c.InvokeCredentials(context.Background(), "gmail.message.send", testInput("send"), map[string]any{"to": []string{"a@example.com"}, "subject": "subject\r\nBcc: attacker@example.com", "text": "body"})
	addresses, _ := f.snapshot()
	if err == nil || len(addresses) != 0 {
		t.Fatal("header injection reached SMTP")
	}
}

func TestIMAPInitializationCapabilityDiscoveryNeverLogsProviderText(t *testing.T) {
	readLog, writeLog, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writeLog
	defer func() { os.Stderr = original; _ = writeLog.Close(); _ = readLog.Close() }()
	for _, mode := range []string{"no_capabilities", "malformed_capabilities"} {
		f := newFixture(t)
		f.mode = mode
		c := testClient(t, f, 465)
		_, err := c.VerifyCredentials(context.Background(), testInput("read"))
		if mode == "no_capabilities" && err != nil {
			t.Fatal("real capability query failed")
		}
		if mode == "malformed_capabilities" && err == nil {
			t.Fatal("malformed capability response accepted")
		}
	}
	_ = writeLog.Close()
	data, err := io.ReadAll(readLog)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatal("IMAP initialization wrote provider diagnostics")
	}
}

func TestCompiledMailProfilesProtocolFlow(t *testing.T) {
	for _, provider := range []string{"qqmail", "netease163"} {
		for _, mechanism := range []string{"PLAIN", "LOGIN"} {
			t.Run(provider+mechanism, func(t *testing.T) {
				f := newFixture(t)
				f.smtpMechanisms = mechanism
				f.imapID = provider == "netease163"
				c, err := New(Options{Provider: provider, Dialer: f, RootCAs: f.roots, HandleKey: strings.Repeat("k", 32)})
				if err != nil {
					t.Fatal(err)
				}
				in := testInput("read_send")
				in.Values = map[string]string{"email": "User.Name+tag@" + c.profile.Domain, "authorization_code": "AbcdefghijklMNOP"}
				v, err := c.VerifyCredentials(context.Background(), in)
				if err != nil {
					t.Fatal(err)
				}
				if v.Evidence.Mail.Provider != provider || len(v.Operations) != 4 {
					t.Fatal(v)
				}
				folders, err := c.InvokeCredentials(context.Background(), provider+".folder.list", in, map[string]any{})
				if err != nil || folders.Value["provider"] != provider {
					t.Fatalf("folders: %v %v", folders, err)
				}
				addresses, commands := f.snapshot()
				for _, address := range addresses {
					if address != c.profile.IMAPHost+":993" && address != c.profile.SMTPHost+":465" {
						t.Fatal(address)
					}
				}
				if strings.Contains(strings.Join(commands, " "), "DATA") {
					t.Fatal("binding must not send")
				}
				if mechanism == "LOGIN" && !strings.Contains(strings.Join(commands, " "), "LOGIN_EXCHANGE") {
					t.Fatal("missing LOGIN")
				}
				if _, err := c.InvokeCredentials(context.Background(), "gmail.folder.list", in, map[string]any{}); err == nil {
					t.Fatal("cross-provider operation accepted")
				}
			})
		}
	}
}
func TestMailProfileProtocolFailures(t *testing.T) {
	for _, tc := range []struct {
		provider, mechanism, mode, usage string
		id                               bool
		want                             core.ErrorCode
	}{
		{"qqmail", "XOAUTH2", "", "send", false, core.ErrorCodeMailAuthMechanismUnsupported},
		{"qqmail", "LOGIN", "auth_reject", "send", false, core.ErrorCodeMailAuthenticationRejected},
		{"netease163", "PLAIN", "", "read", false, core.ErrorCodeMailClientIdentityRequired},
		{"netease163", "PLAIN", "unsafe_login", "read", true, core.ErrorCodeMailClientIdentityRequired},
	} {
		t.Run(tc.provider+tc.mode+tc.mechanism, func(t *testing.T) {
			f := newFixture(t)
			f.smtpMechanisms = tc.mechanism
			f.mode = tc.mode
			f.imapID = tc.id
			c, err := New(Options{Provider: tc.provider, Dialer: f, RootCAs: f.roots, HandleKey: strings.Repeat("k", 32)})
			if err != nil {
				t.Fatal(err)
			}
			in := testInput(tc.usage)
			in.Values = map[string]string{"email": "test@" + c.profile.Domain, "authorization_code": "AbcdefghijklMNOP"}
			_, err = c.VerifyCredentials(context.Background(), in)
			failure, ok := err.(*providers.Failure)
			if !ok || failure.Code != tc.want {
				t.Fatalf("%v", err)
			}
			_, commands := f.snapshot()
			auths := 0
			for _, command := range commands {
				if command == "AUTH" {
					auths++
				}
			}
			if auths > 1 {
				t.Fatal("authentication retried")
			}
		})
	}
}
