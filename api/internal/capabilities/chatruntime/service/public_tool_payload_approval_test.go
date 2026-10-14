package service

import "testing"

func TestPublicToolPayloadKeepsInlineApprovalToken(t *testing.T) {
	payload := map[string]interface{}{
		"metadata": map[string]interface{}{
			"workflow_runs": []interface{}{map[string]interface{}{
				"workflow_run_id": "run-1",
				"approval": map[string]interface{}{
					"ui_approval_allowed": true,
					"approval_token":      "AbCd1234",
					"approval_form": map[string]interface{}{
						"token": "AbCd1234", "node_id": "approval",
						"fields": []interface{}{map[string]interface{}{"key": "review_notes", "label": "Notes"}},
					},
				},
			}},
			"agent_workflow_continuation": map[string]interface{}{
				"ui_approval_allowed": true,
				"approval_token":      "AbCd1234",
			},
		},
	}
	got := publicToolPayload(payload)
	metadata := got["metadata"].(map[string]interface{})
	approval := metadata["workflow_runs"].([]interface{})[0].(map[string]interface{})["approval"].(map[string]interface{})
	if approval["approval_token"] != "AbCd1234" {
		t.Fatalf("approval_token = %v, want the real token", approval["approval_token"])
	}
	if form := approval["approval_form"].(map[string]interface{}); form["token"] != "AbCd1234" {
		t.Fatalf("approval_form.token = %v, want the real token", form["token"])
	}
	fields := approval["approval_form"].(map[string]interface{})["fields"].([]interface{})
	if fields[0].(map[string]interface{})["key"] != "review_notes" {
		t.Fatalf("form field key = %v, want review_notes so submitted inputs match the form", fields[0].(map[string]interface{})["key"])
	}
	if continuation := metadata["agent_workflow_continuation"].(map[string]interface{}); continuation["approval_token"] != "AbCd1234" {
		t.Fatalf("continuation approval_token = %v, want the real token", continuation["approval_token"])
	}
	// The input payload must not be modified in place.
	if payload["metadata"].(map[string]interface{})["agent_workflow_continuation"].(map[string]interface{})["approval_token"] != "AbCd1234" {
		t.Fatal("source payload was mutated")
	}
}

func TestPublicToolPayloadRedactsApprovalTokenWithoutInlineAllowance(t *testing.T) {
	for name, allowed := range map[string]interface{}{"false": false, "missing": nil} {
		approval := map[string]interface{}{"approval_token": "AbCd1234"}
		if allowed != nil {
			approval["ui_approval_allowed"] = allowed
		}
		got := publicToolPayload(map[string]interface{}{"metadata": map[string]interface{}{"approval": approval}})
		token := got["metadata"].(map[string]interface{})["approval"].(map[string]interface{})["approval_token"]
		if token == "AbCd1234" {
			t.Fatalf("%s: approval token leaked to a viewer that cannot approve inline", name)
		}
	}
}

type testApprovalForm struct {
	Token  string                   `json:"token"`
	Fields []map[string]interface{} `json:"fields"`
}

func TestPublicToolPayloadKeepsTypedApprovalForm(t *testing.T) {
	payload := map[string]interface{}{"metadata": map[string]interface{}{
		"agent_workflow_continuation": map[string]interface{}{
			"ui_approval_allowed": true,
			"approval_token":      "AbCd1234",
			"approval_form": testApprovalForm{Token: "AbCd1234", Fields: []map[string]interface{}{
				{"key": "review_notes", "label": "Notes"},
			}},
		},
	}}
	got := publicToolPayload(payload)
	form := got["metadata"].(map[string]interface{})["agent_workflow_continuation"].(map[string]interface{})["approval_form"].(map[string]interface{})
	field := form["fields"].([]interface{})[0].(map[string]interface{})
	if field["key"] != "review_notes" || form["token"] != "AbCd1234" {
		t.Fatalf("typed approval form was not restored: %#v", form)
	}
}
