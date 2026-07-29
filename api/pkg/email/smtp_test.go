package email

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSMTPDeliveryFailureIsLoggedWithoutRecipientData(t *testing.T) {
	previousConfig := Cfg
	Cfg = &config.Config{Email: config.EmailConfig{
		MailType:            "smtp",
		MailDefaultSendFrom: "sender@example.com",
	}}
	t.Cleanup(func() { Cfg = previousConfig })

	previousLogger := logger.L()
	core, observed := observer.New(zapcore.ErrorLevel)
	logger.SetLogger(zap.New(core))
	t.Cleanup(func() { logger.SetLogger(previousLogger) })

	err := sendSMTPEmail(context.Background(), []string{"private@example.com"}, "secret subject", "secret body", "text/plain")
	if err == nil {
		t.Fatal("sendSMTPEmail() error = nil, want invalid configuration error")
	}

	entries := observed.FilterMessage("SMTP email delivery failed").All()
	if len(entries) != 1 {
		t.Fatalf("failure log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["provider"] != "smtp" || fields["stage"] != "validate" {
		t.Fatalf("unexpected failure log fields: %#v", fields)
	}
	if _, ok := fields["recipient"]; ok {
		t.Fatalf("failure log exposed recipient: %#v", fields)
	}
	serialized := fmt.Sprint(entries[0].Message, fields)
	for _, secret := range []string{"private@example.com", "secret subject", "secret body"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("failure log exposed sensitive email data %q: %s", secret, serialized)
		}
	}
}

func TestSMTPRecipientRejectionDoesNotLeakMessageData(t *testing.T) {
	host, port := startRejectingSMTPServer(t)
	previousConfig := Cfg
	Cfg = &config.Config{Email: config.EmailConfig{
		MailType:            "smtp",
		MailDefaultSendFrom: "sender@example.com",
		SMTPServer:          host,
		SMTPPort:            port,
		SMTPSecurity:        "none",
	}}
	t.Cleanup(func() { Cfg = previousConfig })

	previousLogger := logger.L()
	core, observed := observer.New(zapcore.ErrorLevel)
	logger.SetLogger(zap.New(core))
	t.Cleanup(func() { logger.SetLogger(previousLogger) })

	err := sendSMTPEmail(context.Background(), []string{"private@example.com"}, "secret subject", "secret body", "text/plain")
	if err == nil {
		t.Fatal("sendSMTPEmail() error = nil, want recipient rejection")
	}
	for _, secret := range []string{"private@example.com", "secret subject", "secret body"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("SMTP error exposed sensitive email data %q: %v", secret, err)
		}
	}

	entries := observed.FilterMessage("SMTP email delivery failed").All()
	if len(entries) != 1 {
		t.Fatalf("failure log count = %d, want 1", len(entries))
	}
	serialized := fmt.Sprint(entries[0].Message, entries[0].ContextMap())
	for _, secret := range []string{"private@example.com", "secret subject", "secret body"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("SMTP failure log exposed sensitive email data %q: %s", secret, serialized)
		}
	}
}

func startRejectingSMTPServer(t *testing.T) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = fmt.Fprint(conn, "220 smtp.test ESMTP\r\n")
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				_, _ = fmt.Fprint(conn, "250 smtp.test\r\n")
			case strings.HasPrefix(line, "HELO"), strings.HasPrefix(line, "MAIL FROM"):
				_, _ = fmt.Fprint(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "RCPT TO"):
				_, _ = fmt.Fprint(conn, "550 private@example.com secret subject secret body\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "250 OK\r\n")
			}
		}
	}()

	host, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
