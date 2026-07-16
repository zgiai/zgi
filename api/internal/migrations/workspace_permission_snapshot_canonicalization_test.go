package migrations

import (
	"strings"
	"testing"
)

func TestCanonicalWorkspaceAssignablePermissionJSONRemovesRetiredAndCompatibilityCodes(t *testing.T) {
	raw := `[
		"workspace.manage",
		"workspace.view",
		"dashboard.view",
		"prompt.optimize",
		"content_parse.chunk.preview",
		"agent.view",
		"agent.manage",
		"knowledge_base.view",
		"database.view",
		"database.data_edit",
		"database.ai_query",
		"file.upload_create",
		"file.move_create"
	]`

	gotJSON, err := canonicalWorkspaceAssignablePermissionJSON(raw)
	if err != nil {
		t.Fatalf("canonicalize workspace permissions: %v", err)
	}
	permissions, err := decodeWorkspaceMemberPermissionSeedJSON(gotJSON)
	if err != nil {
		t.Fatalf("decode canonical permissions: %v", err)
	}

	for _, want := range []string{
		"agent.view",
		"agent.create",
		"agent.update",
		"workflow.view",
		"workflow.publish",
		"workflow.update",
		"workflow.run.draft",
		"knowledge_base.view",
		"knowledge_base.document.view",
		"knowledge_base.graph.view",
		"database.view",
		"database.schema.view",
		"database.record.view",
		"database.operation_logs.view",
		"database.record.create",
		"database.ai_query.read",
		"file.upload",
		"file.text.create",
		"file.move",
		"file.folder.manage",
	} {
		if !containsString(permissions, want) {
			t.Fatalf("canonical permissions = %#v, want contain %q", permissions, want)
		}
	}

	for _, permission := range permissions {
		if strings.HasPrefix(permission, "workspace.") ||
			strings.HasPrefix(permission, "dashboard.") ||
			strings.HasPrefix(permission, "prompt.") ||
			strings.HasPrefix(permission, "content_parse.") {
			t.Fatalf("canonical permissions should not contain retired permission %q: %#v", permission, permissions)
		}
	}

	for _, retired := range []string{
		"agent.manage",
		"database.data_edit",
		"database.ai_query",
		"file.upload_create",
		"file.move_create",
	} {
		if containsString(permissions, retired) {
			t.Fatalf("canonical permissions should not contain retired or compatibility permission %q: %#v", retired, permissions)
		}
	}
}

func TestCanonicalWorkspaceAssignablePermissionJSONIsIdempotent(t *testing.T) {
	first, err := canonicalWorkspaceAssignablePermissionJSON(`["agent.manage","database.ai_query","workspace.member.manage"]`)
	if err != nil {
		t.Fatalf("canonicalize first snapshot: %v", err)
	}
	second, err := canonicalWorkspaceAssignablePermissionJSON(first)
	if err != nil {
		t.Fatalf("canonicalize second snapshot: %v", err)
	}
	if second != first {
		t.Fatalf("canonical permission JSON = %s, want idempotent %s", second, first)
	}

	permissions, err := decodeWorkspaceMemberPermissionSeedJSON(first)
	if err != nil {
		t.Fatalf("decode canonical permissions: %v", err)
	}
	for _, want := range []string{"agent.view", "workflow.view", "agent.create", "workflow.publish"} {
		if !containsString(permissions, want) {
			t.Fatalf("canonical permissions = %#v, want contain %q", permissions, want)
		}
	}
}

func TestCanonicalWorkspaceAssignablePermissionJSONRejectsInvalidJSON(t *testing.T) {
	if _, err := canonicalWorkspaceAssignablePermissionJSON(`{"agent.manage": true}`); err == nil {
		t.Fatal("expected object permission payload to be rejected")
	}
}
