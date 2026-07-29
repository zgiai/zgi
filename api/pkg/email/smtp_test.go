package email

import (
	"context"
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
	if _, ok := fields["error"]; !ok {
		t.Fatalf("failure log omitted error: %#v", fields)
	}
	if _, ok := fields["recipient"]; ok {
		t.Fatalf("failure log exposed recipient: %#v", fields)
	}
}
