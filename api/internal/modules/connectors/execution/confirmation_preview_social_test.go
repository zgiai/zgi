package execution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestSocialConfirmationPreviewShowsAccountActionAndExactPost(t *testing.T) {
	account := "test-account-123"
	connection := core.PersonalConnection{Name: "Social account", ExternalAccountID: &account}
	for _, input := range []struct {
		operation string
		arguments map[string]any
	}{
		{"x-social.account.get", map[string]any{}},
		{"x-social.post.create", map[string]any{"text": "Testing the ZGI X integration. This is a connectivity test."}},
		{"x-social.post.get", map[string]any{"post_id": "123"}},
	} {
		t.Run(input.operation, func(t *testing.T) {
			preview := buildConfirmationPreview(ExecutionRequest{ConnectorID: "x-social", OperationID: core.Identifier(input.operation), Input: input.arguments}, connection, "")
			if !preview.Complete || preview.AccountIdentity != account || len(preview.Fields) == 0 {
				t.Fatalf("incomplete preview: %#v", preview)
			}
			if text, ok := input.arguments["text"].(string); ok {
				found := false
				for _, f := range preview.Fields {
					found = found || f.Value == text
				}
				if !found {
					t.Fatal("post text missing or changed")
				}
			}
		})
	}
}

func TestSocialConfirmationPreviewRejectsUnknownIncompleteAndAlteredRequests(t *testing.T) {
	account := "account-1"
	connection := core.PersonalConnection{ExternalAccountID: &account, EncryptedCredentials: "CREDENTIAL-SECRET"}
	for _, input := range []struct {
		operation string
		arguments map[string]any
	}{
		{"x-social.unknown", map[string]any{}},
		{"x-social.post.create", map[string]any{}},
		{"x-social.post.create", map[string]any{"text": "hello", "token": "PRIVATE-SECRET"}},
		{"x-social.post.create", map[string]any{"text": true}},
		{"x-social.post.create", map[string]any{"text": "hidden\u202epost"}},
	} {
		preview := buildConfirmationPreview(ExecutionRequest{ConnectorID: "x-social", OperationID: core.Identifier(input.operation), Input: input.arguments}, connection, "")
		if preview.Complete {
			t.Fatalf("unsafe preview admitted: %#v", input)
		}
		encoded, _ := json.Marshal(preview)
		for _, secret := range []string{"PRIVATE-SECRET", "CREDENTIAL-SECRET"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatal("secret leaked")
			}
		}
	}
}

func TestSocialConfirmationPreviewRequiresPublishingAccountAndPinsImage(t *testing.T) {
	account := "test-account-123"
	connection := core.PersonalConnection{ExternalAccountID: &account}
	request := ExecutionRequest{ConnectorID: "linkedin-social", OperationID: "linkedin-social.post.create", Input: map[string]any{"text": "Test"}}
	if buildConfirmationPreview(request, connection, "").Complete {
		t.Fatal("missing required publishing account admitted")
	}
	request.Input.(map[string]any)["account_id"] = "urn:li:organization:test"
	if !buildConfirmationPreview(request, connection, "").Complete {
		t.Fatal("valid company-page target rejected")
	}
	request = ExecutionRequest{ConnectorID: "x-social", OperationID: "x-social.post.create", Input: map[string]any{"text": "Test", "image_data": "aW1hZ2UtYnl0ZXM=", "mime_type": "image/png"}}
	preview := buildConfirmationPreview(request, connection, "")
	if !preview.Complete {
		t.Fatal("image request rejected")
	}
	digestFound := false
	for _, field := range preview.Fields {
		if field.Label == "Image SHA-256" && len(field.Value) == 64 {
			digestFound = true
		}
	}
	raw, _ := json.Marshal(preview)
	if !digestFound || strings.Contains(string(raw), "aW1hZ2UtYnl0ZXM=") {
		t.Fatal("attachment bytes exposed or not pinned")
	}
	account = "bad\u202eaccount"
	if buildConfirmationPreview(request, connection, "").Complete {
		t.Fatal("altered account identity admitted")
	}
}
