package gmail

import (
	"strings"
	"testing"
)

func TestGmailAdapterIdentityIsFixed(t *testing.T) {
	c, err := New(Options{Provider: "qqmail", HandleKey: strings.Repeat("k", 32)})
	if err != nil || c.ConnectorID() != "gmail" {
		t.Fatal("Gmail adapter identity changed")
	}
}
