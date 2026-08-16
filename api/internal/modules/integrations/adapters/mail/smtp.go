package mail

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/netip"
	"net/smtp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type smtpSender struct {
	dialContext func(context.Context, string, string) (net.Conn, error)
}

func newSMTPSender() *smtpSender {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return &smtpSender{dialContext: dialer.DialContext}
}

type outboundMessage struct {
	From                                 string
	To, CC, BCC                          []string
	Subject, Body, InReplyTo, References string
}
type smtpResult struct {
	MessageID string
	Accepted  []string
}

func (sender *smtpSender) authenticate(ctx context.Context, settings serverSettings) error {
	client, conn, err := sender.connect(ctx, settings)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer client.Close()
	if err := authenticateSMTPClient(client, settings); err != nil {
		return authError(err)
	}
	return nil
}

func (sender *smtpSender) send(ctx context.Context, settings serverSettings, message outboundMessage) (smtpResult, error) {
	client, conn, err := sender.connect(ctx, settings)
	if err != nil {
		return smtpResult{}, err
	}
	defer conn.Close()
	defer client.Close()
	if err := authenticateSMTPClient(client, settings); err != nil {
		return smtpResult{}, authError(err)
	}
	if err := client.Mail(settings.Email); err != nil {
		return smtpResult{}, wrapError(integrations.ErrorCodeProviderRejected, "mail provider rejected the sender", err)
	}
	recipients := append(append(append([]string{}, message.To...), message.CC...), message.BCC...)
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return smtpResult{}, wrapError(integrations.ErrorCodeProviderRejected, "mail provider rejected a recipient", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpResult{}, upstreamError(err)
	}
	messageID, raw, err := buildMIMEMessage(settings.Email, message)
	if err != nil {
		_ = writer.Close()
		return smtpResult{}, err
	}
	if _, err = writer.Write(raw); err != nil {
		_ = writer.Close()
		return smtpResult{}, wrapError(integrations.ErrorCodeOperationOutcomeUnknown, "mail submission result is unknown", err)
	}
	if err = writer.Close(); err != nil {
		return smtpResult{}, wrapError(integrations.ErrorCodeOperationOutcomeUnknown, "mail submission result is unknown", err)
	}
	// Data.Close waits for the provider's final acceptance response. Once it
	// succeeds, a later QUIT transport error cannot make delivery ambiguous.
	_ = client.Quit()
	return smtpResult{MessageID: messageID, Accepted: recipients}, nil
}

func authenticateSMTPClient(client *smtp.Client, settings serverSettings) error {
	_, mechanisms := client.Extension("AUTH")
	upper := strings.ToUpper(mechanisms)
	var auth smtp.Auth
	switch {
	case strings.Contains(" "+upper+" ", " PLAIN "):
		auth = smtp.PlainAuth("", settings.Username, settings.Password, settings.SMTPHost)
	case strings.Contains(" "+upper+" ", " LOGIN "):
		auth = &loginAuth{username: settings.Username, password: settings.Password}
	default:
		return wrapError(integrations.ErrorCodeAccessDenied, "mail server does not advertise a supported authentication method", nil)
	}
	return client.Auth(auth)
}

type loginAuth struct{ username, password string }

func (auth *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server == nil || !server.TLS {
		return "", nil, fmt.Errorf("LOGIN authentication requires TLS")
	}
	return "LOGIN", nil, nil
}

func (auth *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(string(fromServer))
	switch {
	case strings.Contains(prompt, "user"):
		return []byte(auth.username), nil
	case strings.Contains(prompt, "pass"):
		return []byte(auth.password), nil
	default:
		return nil, fmt.Errorf("unexpected SMTP LOGIN challenge")
	}
}

func (sender *smtpSender) connect(ctx context.Context, settings serverSettings) (*smtp.Client, net.Conn, error) {
	if err := validateServerName(settings.SMTPHost); err != nil {
		return nil, nil, invalidInput("SMTP server is invalid", err)
	}
	addresses, err := resolvePublicMailHost(ctx, settings.SMTPHost)
	if err != nil {
		return nil, nil, err
	}
	// Pin the TCP connection to an address that passed the public-address
	// policy. Dialing the hostname again would leave a DNS-rebinding gap
	// between validation and connection establishment.
	conn, err := sender.dialContext(ctx, "tcp", serverAddress(addresses[0].String(), settings.SMTPPort))
	if err != nil {
		return nil, nil, upstreamError(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	if value, ok := ctx.Deadline(); ok {
		deadline = value
	}
	_ = conn.SetDeadline(deadline)
	tlsConfig := &tls.Config{ServerName: settings.SMTPHost, MinVersion: tls.VersionTLS12}
	if settings.SMTPSecurity == "implicit_tls" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, nil, upstreamError(err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, settings.SMTPHost)
	if err != nil {
		conn.Close()
		return nil, nil, upstreamError(err)
	}
	if settings.SMTPSecurity == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			client.Close()
			conn.Close()
			return nil, nil, wrapError(integrations.ErrorCodeAccessDenied, "mail server does not support STARTTLS", nil)
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Close()
			conn.Close()
			return nil, nil, upstreamError(err)
		}
	}
	return client, conn, nil
}

func resolvePublicMailHost(ctx context.Context, host string) ([]netip.Addr, error) {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, upstreamError(err)
	}
	result := make([]netip.Addr, 0, len(addresses))
	for _, resolved := range addresses {
		address, ok := netip.AddrFromSlice(resolved.IP)
		if !ok || !address.IsValid() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() || isCGNAT(address) {
			return nil, wrapError(integrations.ErrorCodeAccessDenied, "SMTP server resolved to a non-public address", nil)
		}
		result = append(result, address.Unmap())
	}
	return result, nil
}

func isCGNAT(address netip.Addr) bool {
	prefix := netip.MustParsePrefix("100.64.0.0/10")
	return address.Is4() && prefix.Contains(address)
}

func parseAddressList(value string, limit int) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	addresses, err := mail.ParseAddressList(value)
	if err != nil || len(addresses) == 0 || len(addresses) > limit {
		return nil, fmt.Errorf("address list is invalid")
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address == nil || len(address.Address) > 320 || strings.ContainsAny(address.Address+address.Name, "\r\n") {
			return nil, fmt.Errorf("address is invalid")
		}
		result = append(result, address.Address)
	}
	return result, nil
}

func buildMIMEMessage(from string, message outboundMessage) (string, []byte, error) {
	if strings.TrimSpace(message.Subject) == "" || len([]rune(message.Subject)) > 998 || strings.ContainsAny(message.Subject, "\r\n") || strings.TrimSpace(message.Body) == "" || len([]rune(message.Body)) > 100000 {
		return "", nil, invalidInput("email subject or body is invalid", nil)
	}
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return "", nil, err
	}
	domain := strings.SplitN(from, "@", 2)
	if len(domain) != 2 {
		return "", nil, invalidInput("sender address is invalid", nil)
	}
	messageID := "<" + base64.RawURLEncoding.EncodeToString(random) + "@" + domain[1] + ">"
	var builder strings.Builder
	writeHeader := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			builder.WriteString(key + ": " + value + "\r\n")
		}
	}
	writeHeader("From", from)
	writeHeader("To", strings.Join(message.To, ", "))
	writeHeader("Cc", strings.Join(message.CC, ", "))
	writeHeader("Date", time.Now().UTC().Format(time.RFC1123Z))
	writeHeader("Message-ID", messageID)
	writeHeader("Subject", mime.QEncoding.Encode("UTF-8", message.Subject))
	writeHeader("In-Reply-To", safeThreadHeader(message.InReplyTo, 998))
	writeHeader("References", safeThreadHeader(message.References, 4000))
	builder.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(message.Body))
	for len(encoded) > 76 {
		builder.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	builder.WriteString(encoded + "\r\n")
	return messageID, []byte(builder.String()), nil
}

func safeThreadHeader(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}

// keep these imports exercised on all supported Go versions where net/smtp internals differ.
var _ = bufio.NewReader
var _ io.Reader
