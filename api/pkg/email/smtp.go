package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
)

func sendSMTPEmail(ctx context.Context, to []string, subject, body, bodyType string) error {
	if strings.TrimSpace(Cfg.Email.SMTPServer) == "" {
		return fmt.Errorf("EMAIL_SMTP_SERVER is required")
	}
	if Cfg.Email.SMTPPort <= 0 {
		return fmt.Errorf("EMAIL_PORT must be greater than 0")
	}

	from := strings.TrimSpace(Cfg.Email.MailDefaultSendFrom)
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}
	recipients, err := normalizeRecipients(to)
	if err != nil {
		return err
	}
	message, err := buildSMTPMessage(from, recipients, subject, body, bodyType)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(Cfg.Email.SMTPServer, fmt.Sprintf("%d", Cfg.Email.SMTPPort))
	client, err := newSMTPClient(ctx, addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := maybeStartTLS(client); err != nil {
		return err
	}
	if err := maybeAuthSMTP(client); err != nil {
		return err
	}
	if err := writeSMTPMessage(client, fromAddress.Address, recipients, message); err != nil {
		return err
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("failed to quit SMTP session: %w", err)
	}

	logger.Info("Email sent successfully", "provider", "smtp", "recipient_count", len(recipients))
	return nil
}

func newSMTPClient(ctx context.Context, addr string) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect SMTP server: %w", err)
	}
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to set SMTP deadline: %w", err)
	}
	if Cfg.Email.SMTPSecurity == "implicit_tls" || (Cfg.Email.SMTPSecurity == "" && Cfg.Email.SMTPUseTLS) {
		tlsConn := tls.Client(conn, &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: Cfg.Email.SMTPServer,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to negotiate SMTP TLS: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, Cfg.Email.SMTPServer)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create SMTP client: %w", err)
	}
	return client, nil
}

func maybeStartTLS(client *smtp.Client) error {
	requireStartTLS := Cfg.Email.SMTPSecurity == "starttls"
	opportunisticStartTLS := Cfg.Email.SMTPSecurity == "" && Cfg.Email.SMTPOpportunisticTLS
	if !requireStartTLS && !opportunisticStartTLS {
		return nil
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		if requireStartTLS {
			return fmt.Errorf("SMTP server does not support required STARTTLS")
		}
		return nil
	}
	if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: Cfg.Email.SMTPServer}); err != nil {
		return fmt.Errorf("failed to start SMTP TLS: %w", err)
	}
	return nil
}

func maybeAuthSMTP(client *smtp.Client) error {
	if strings.TrimSpace(Cfg.Email.SMTPUsername) == "" {
		return nil
	}

	auth := smtp.PlainAuth("", Cfg.Email.SMTPUsername, Cfg.Email.SMTPPassword, Cfg.Email.SMTPServer)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate SMTP: %w", err)
	}
	return nil
}

func writeSMTPMessage(client *smtp.Client, from string, recipients []string, message []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set SMTP sender: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set SMTP recipient %s: %w", recipient, err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open SMTP data writer: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("failed to write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close SMTP data writer: %w", err)
	}
	return nil
}

func normalizeRecipients(to []string) ([]string, error) {
	recipients := make([]string, 0, len(to))
	for _, raw := range to {
		recipient := strings.TrimSpace(raw)
		if recipient == "" {
			continue
		}
		addr, err := mail.ParseAddress(recipient)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient address %q: %w", recipient, err)
		}
		recipients = append(recipients, addr.Address)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	return recipients, nil
}

func buildSMTPMessage(from string, to []string, subject, body, bodyType string) ([]byte, error) {
	contentType, err := smtpContentType(bodyType)
	if err != nil {
		return nil, err
	}

	var message bytes.Buffer
	message.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	message.WriteString("To: " + sanitizeHeader(strings.Join(to, ", ")) + "\r\n")
	message.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", sanitizeHeader(subject)) + "\r\n")
	message.WriteString("MIME-Version: 1.0\r\n")
	message.WriteString("Content-Type: " + contentType + "; charset=UTF-8\r\n")
	message.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	message.WriteString("\r\n")
	message.WriteString(body)
	return message.Bytes(), nil
}

func smtpContentType(bodyType string) (string, error) {
	switch strings.TrimSpace(bodyType) {
	case "", "text/html":
		return "text/html", nil
	case "text/plain":
		return "text/plain", nil
	default:
		return "", fmt.Errorf("unsupported email body_type: %s", bodyType)
	}
}

func sanitizeHeader(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")), " ")
}
