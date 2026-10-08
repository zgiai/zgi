package catalog

import "testing"

func TestGitHubMCPDefinition(t *testing.T) {
	definition := GitHubMCPDefinition()
	for _, operation := range definition.Operations {
		if err := compileSchemaDocument(operation.InputSchema); err != nil {
			t.Fatalf("%s input schema: %v", operation.ID, err)
		}
		if err := compileSchemaDocument(operation.OutputSchema); err != nil {
			t.Fatalf("%s output schema: %v", operation.ID, err)
		}
	}
	if err := Validate(definition); err != nil {
		t.Fatal(err)
	}
}
