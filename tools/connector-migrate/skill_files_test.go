package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoredSkillInventoryIsBoundedAndDoesNotRewriteFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skill")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nname: legacy\nintegration_requirements: []\n---\nUser authored prompt")
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), content, 0600); err != nil {
		t.Fatal(err)
	}
	source := fixture(t)
	source["chat_runtime_custom_skills"] = []row{{"id": receiptID, "storage_path": path, "manifest": row{"files": []any{"SKILL.md"}}, "status": "active"}}
	if err := inventorySkillFiles(source, ""); err == nil {
		t.Fatal("uninspected files must not be treated as native")
	}
	if err := inventorySkillFiles(source, root); err != nil {
		t.Fatal(err)
	}
	plan, err := makePlan(source, testKeys())
	if err != nil || len(plan.Report.Issues) > 0 {
		t.Fatalf("%v %#v", err, plan.Report.Issues)
	}
	converted := applyMemory(t, source, plan)
	if text(converted["chat_runtime_custom_skills"][0], "status") != "invalid" {
		t.Fatal("retired skill became executable")
	}
	got, _ := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if string(got) != string(content) {
		t.Fatal("converter rewrote user-authored files")
	}
	if err := inventorySkillFiles(source, t.TempDir()); err == nil {
		t.Fatal("path escaped explicit root")
	}
}

func TestVerifierRejectsCrossScopeSnapshot(t *testing.T) {
	source := fixture(t)
	plan, err := makePlan(source, testKeys())
	if err != nil {
		t.Fatal(err)
	}
	converted := applyMemory(t, source, plan)
	converted["connector_target_bindings"][0]["target_id"] = receiptID
	result := verifySnapshot(converted, testKeys())
	for _, issue := range result.Issues {
		if issue.Code == "agent_snapshot_binding_mismatch" {
			return
		}
	}
	t.Fatal("snapshot accepted another host's binding")
}
