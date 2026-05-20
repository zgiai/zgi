package email_test

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/email"
)

func TestSendEmailWithSMTPBackend(t *testing.T) {
	server := startSMTPTestServer(t)
	host, portValue, err := net.SplitHostPort(server.addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	previousCfg := email.Cfg
	email.Init(&config.Config{
		Email: config.EmailConfig{
			MailType:            "smtp",
			MailDefaultSendFrom: "noreply@zgi.local",
			SMTPServer:          host,
			SMTPPort:            port,
		},
	})
	t.Cleanup(func() {
		email.Init(previousCfg)
	})

	err = email.SendEmailWithBodyType(
		[]string{"approver@example.com"},
		"Approval\nRequest",
		"<b>hello</b>",
		"text/html",
	)
	if err != nil {
		t.Fatalf("SendEmailWithBodyType: %v", err)
	}

	select {
	case msg := <-server.messages:
		assertSMTPMessage(t, msg)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SMTP message")
	}
}

func assertSMTPMessage(t *testing.T, msg string) {
	t.Helper()

	if !strings.Contains(msg, "From: noreply@zgi.local") {
		t.Fatalf("expected from header, got %q", msg)
	}
	if !strings.Contains(msg, "To: approver@example.com") {
		t.Fatalf("expected to header, got %q", msg)
	}
	if strings.Contains(msg, "Approval\nRequest") || strings.Contains(msg, "Approval\r\nRequest") {
		t.Fatalf("subject header was not sanitized: %q", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("expected html content type, got %q", msg)
	}
	if !strings.Contains(msg, "<b>hello</b>") {
		t.Fatalf("expected body, got %q", msg)
	}
}

type smtpTestServer struct {
	addr     string
	messages chan string
}

func startSMTPTestServer(t *testing.T) smtpTestServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := smtpTestServer{
		addr:     ln.Addr().String(),
		messages: make(chan string, 1),
	}
	t.Cleanup(func() {
		_ = ln.Close()
	})

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		serveSMTPTestConnection(conn, server.messages)
	}()

	return server
}

func serveSMTPTestConnection(conn net.Conn, messages chan<- string) {
	reader := bufio.NewReader(conn)
	writeLine := func(format string, args ...any) bool {
		_, err := fmt.Fprintf(conn, format+"\r\n", args...)
		return err == nil
	}

	if !writeLine("220 localhost ESMTP") {
		return
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			if !writeLine("250-localhost") || !writeLine("250 OK") {
				return
			}
		case strings.HasPrefix(cmd, "MAIL FROM:"), strings.HasPrefix(cmd, "RCPT TO:"):
			if !writeLine("250 OK") {
				return
			}
		case strings.HasPrefix(cmd, "DATA"):
			if !writeLine("354 End data with <CR><LF>.<CR><LF>") {
				return
			}
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
				body.WriteString(dataLine)
			}
			messages <- body.String()
			if !writeLine("250 queued") {
				return
			}
		case strings.HasPrefix(cmd, "QUIT"):
			_ = writeLine("221 bye")
			return
		default:
			if !writeLine("250 OK") {
				return
			}
		}
	}
}
