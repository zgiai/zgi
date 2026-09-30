package execution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestConfirmationPreviewFrozenMailAndSecretExclusion(t *testing.T) {
	account := "sender@qq.com"
	connection := core.PersonalConnection{Name: "Work", ExternalAccountID: &account, EncryptedCredentials: "CREDENTIAL-SECRET"}
	request := ExecutionRequest{OperationID: "qqmail.message.send", Input: map[string]any{"to": []any{"first@example.com", "second@example.com"}, "subject": "Hello", "text": strings.Repeat("文", 3000), "app_password": "PRIVATE-KEY", "headers": map[string]any{"Authorization": "BEARER-SECRET"}}}
	preview := buildConfirmationPreview(request, connection, "")
	if !preview.Complete || preview.AccountIdentity != account || len(preview.Fields) != 3 || !preview.Fields[2].Truncated {
		t.Fatalf("invalid preview: %#v", preview)
	}
	if preview.Fields[0].Value != "first@example.com\nsecond@example.com" || len([]rune(preview.Fields[2].Value)) != 2048 {
		t.Fatal("targets or length changed")
	}
	data, _ := json.Marshal(preview)
	for _, secret := range []string{"PRIVATE-KEY", "BEARER-SECRET", "CREDENTIAL-SECRET", "app_password"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("secret escaped")
		}
	}
	connection.Name = "Changed"
	request.Input.(map[string]any)["subject"] = "Changed"
	if preview.AccountName != "Work" || preview.Fields[1].Value != "Hello" {
		t.Fatal("preview was not frozen")
	}
	for _, value := range []any{RuntimeConfirmation{Preview: preview}, FrozenConfirmationRequest{Preview: preview}} {
		encoded, _ := json.Marshal(value)
		if strings.Contains(string(encoded), account) || strings.Contains(string(encoded), "first@example") {
			t.Fatal("preview escaped public runtime projection")
		}
	}
}

func TestConfirmationPreviewRequiresResolvedContextAndCompleteTargets(t *testing.T) {
	account := "original-account"
	conn := core.PersonalConnection{ExternalAccountID: &account}
	req := ExecutionRequest{OperationID: "weixin.messages.reply", Input: map[string]any{"context_handle": "OPAQUE-HANDLE", "resolved_recipient": "FORGED", "text": "Hi"}}
	missing := buildConfirmationPreview(req, conn, "")
	if missing.Complete {
		t.Fatal("accepted client-supplied context target")
	}
	preview := buildConfirmationPreview(req, conn, "verified-recipient")
	if !preview.Complete || preview.Fields[0].Value != "verified-recipient" {
		t.Fatal("resolved recipient missing")
	}
	data, _ := json.Marshal(preview)
	if strings.Contains(string(data), "OPAQUE") || strings.Contains(string(data), "FORGED") {
		t.Fatal("opaque or forged target exposed")
	}
	req.OperationID = "unknown.operation"
	if buildConfirmationPreview(req, conn, "").Complete {
		t.Fatal("unknown operation admitted")
	}
	req.OperationID = "qqmail.message.send"
	req.Input = map[string]any{"to": []string{strings.Repeat("a", 9000)}, "subject": "x", "text": "body"}
	if buildConfirmationPreview(req, conn, "").Complete {
		t.Fatal("truncated recipient admitted")
	}
}

func TestConfirmationPreviewPreservesAllRecipientsAndAccount(t *testing.T) {
	account := "sender@example.com"
	conn := core.PersonalConnection{ExternalAccountID: &account}
	to := make([]string, 20)
	for i := range to {
		to[i] = strings.Repeat("a", 242) + "@example.com"
	}
	req := ExecutionRequest{OperationID: "gmail.message.send", Input: map[string]any{"to": to, "subject": "", "text": "body"}}
	p := buildConfirmationPreview(req, conn, "")
	if !p.Complete || p.Fields[0].Value != strings.Join(to, "\n") {
		t.Fatal("valid recipients were omitted")
	}
	for _, identity := range []string{strings.Repeat("a", 513), "sender\u202e@example.com"} {
		conn.ExternalAccountID = &identity
		if buildConfirmationPreview(req, conn, "").Complete {
			t.Fatal("altered account admitted")
		}
	}
	conn.ExternalAccountID = &account
	req.Input.(map[string]any)["to"] = []string{"recipient\u202e@example.com"}
	if buildConfirmationPreview(req, conn, "").Complete {
		t.Fatal("altered recipient admitted")
	}
}
