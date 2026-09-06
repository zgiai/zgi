package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomPackageScriptContract(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('{}')"), 0600); err != nil {
		t.Fatal(err)
	}
	markdown := `---
name: private-example
description: Synthetic package contract example.
script_input_schema:
  type: object
  required: [count]
  additionalProperties: false
  properties:
    count:
      type: integer
      minimum: 1
---
Execute the provided script.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(markdown), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadCustomSkillDocument(root)
	if err != nil {
		t.Fatal(err)
	}
	ensureScriptTool(&doc)
	if len(doc.Tools) != 1 {
		t.Fatalf("unexpected tools: %d", len(doc.Tools))
	}
	if err := validateSkillToolArguments(doc.Metadata.ID, doc.Tools[0], map[string]interface{}{"count": float64(2)}); err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]interface{}{{}, {"count": float64(0)}, {"count": "2"}, {"count": float64(2), "extra": true}} {
		if err := validateSkillToolArguments(doc.Metadata.ID, doc.Tools[0], args); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
}

func TestPackageScriptSchemaRejectsUnsafeSchema(t *testing.T) {
	for _, schema := range []map[string]interface{}{
		{"type": "array"},
		{"type": "object", "$ref": "https://example.com/schema"},
		{"type": "object", "$id": "file:///tmp/schema"},
		{"type": "object", "properties": map[string]interface{}{"x": map[string]interface{}{"type": "invalid"}}},
	} {
		if err := validatePackageScriptSchema(schema); err == nil {
			t.Fatalf("accepted invalid schema: %v", schema)
		}
	}
}
