package skills

import "testing"

// A unified-mode model reads a signature instead of the JSON schema; the
// server still validates against the schema (ADR 0044).
func TestCompactSignatureIsShortAndKeepsRequiredFields(t *testing.T) {
	contract := skillToolArgumentContracts()[SkillFileGenerator+"/generate_file"]
	got := compactSignature(contract.ToolName, contract.Schema)
	want := "generate_file(content: string, format: txt|md|html|json|csv|svg|docx|xlsx|pdf, filename?: string, lifecycle?: persistent|temporary, title?: string)"
	if got != want {
		t.Fatalf("signature = %s\nwant %s", got, want)
	}
	docx := skillToolArgumentContracts()[SkillFileGenerator+"/generate_docx"]
	if signature := compactSignature(docx.ToolName, docx.Schema); len(signature) > 1500 {
		t.Fatalf("docx signature = %d characters, want at most 1500", len(signature))
	}
}
