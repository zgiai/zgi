package skills

import (
	"path/filepath"
	"testing"
)

func TestDocumentedScriptSkillTemplatesMatchRuntimeContract(t *testing.T) {
	tests := []struct {
		name              string
		directory         string
		skillID           string
		language          string
		entrypoint        string
		bundledName       string
		bundledPath       string
		expectedInputName string
	}{
		{
			name: "python", directory: "zgi-script-skill-template", skillID: "zgi-script-skill-template",
			language: "python3", entrypoint: "scripts/run.py", bundledName: "zgi_skill_template",
			bundledPath: "scripts/zgi_skill_template", expectedInputName: "source",
		},
		{
			name: "nodejs", directory: "zgi-node-script-skill-template", skillID: "zgi-node-script-skill-template",
			language: "nodejs", entrypoint: "scripts/run.js", bundledName: "zgi_skill_node_template",
			bundledPath: "scripts/zgi_skill_node_template", expectedInputName: "source",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Clean(filepath.Join("..", "..", "..", "..", "docs", "examples", test.directory))
			doc, err := LoadCustomSkillDocument(root)
			if err != nil {
				t.Fatalf("LoadCustomSkillDocument() error = %v", err)
			}
			if doc.Metadata.ID != test.skillID || doc.Metadata.RuntimeType != SkillRuntimeTypePrompt || !doc.Metadata.HasScripts {
				t.Fatalf("unexpected template metadata: %+v", doc.Metadata)
			}
			tool, ok := findSkillTool(doc, SkillScriptToolRun)
			if !ok || len(tool.InputSchema) == 0 {
				t.Fatal("documented template must expose a schema-backed run_script tool")
			}

			valid := map[string]interface{}{
				"mode": "upper", "text": "hello", "count": float64(2), "ratio": 0.75,
				"enabled": true, "tags": []interface{}{"reference", test.name},
				"options": map[string]interface{}{"prefix": "Result: ", "repeat": float64(1), "emit_diagnostic": false},
				"note":    nil,
			}
			if err := validateSkillToolArguments(doc.Metadata.ID, tool, valid); err != nil {
				t.Fatalf("template example arguments are invalid: %v", err)
			}
			for _, invalid := range []map[string]interface{}{
				{"mode": "upper"},
				{
					"mode": "upper", "text": "hello", "count": "2", "ratio": 0.75,
					"enabled": true, "tags": []interface{}{},
					"options": map[string]interface{}{"prefix": "", "repeat": float64(1), "emit_diagnostic": false},
				},
			} {
				if err := validateSkillToolArguments(doc.Metadata.ID, tool, invalid); err == nil {
					t.Fatalf("template invalid arguments were accepted: %v", invalid)
				}
			}

			prepared, err := prepareSkillScriptManifest(root, doc.Metadata.TimeoutSeconds, false)
			if err != nil {
				t.Fatalf("prepareSkillScriptManifest() error = %v", err)
			}
			manifest := prepared.Manifest
			if manifest.Entrypoint != test.entrypoint || manifest.Language != test.language || manifest.ResultMode != "mixed" {
				t.Fatalf("unexpected template manifest: %+v", manifest)
			}
			if len(manifest.BundledPackages) != 1 || manifest.BundledPackages[0].Name != test.bundledName || manifest.BundledPackages[0].Path != test.bundledPath {
				t.Fatalf("unexpected bundled packages: %+v", manifest.BundledPackages)
			}
			if len(manifest.InputFiles) != 2 || manifest.InputFiles[0].Name != test.expectedInputName || !manifest.InputFiles[1].Multiple || manifest.InputFiles[1].MaxCount != 3 {
				t.Fatalf("unexpected template input files: %+v", manifest.InputFiles)
			}
			if skillPackageHasDependencyHints(root) {
				t.Fatal("bundled-only template must not request a managed dependency build")
			}
		})
	}
}
