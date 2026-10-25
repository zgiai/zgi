package execution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestSocialConfirmationPreviewShowsAccountActionAndExactPost(t *testing.T) {
	account := "2112563196477485056"
	connection := core.PersonalConnection{Name: "hank han", ExternalAccountID: &account}
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
