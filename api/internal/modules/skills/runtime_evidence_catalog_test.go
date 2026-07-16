package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyAIChatSkillsDeclareExecutionEvidenceContracts(t *testing.T) {
	tests := []struct {
		skillID string
		want    []string
	}{
		{
			skillID: SkillFileGenerator,
			want: []string{
				"Treat the generated tool result as authoritative",
				"`file_id`",
				"`tool_file_id`",
				"do not say the file was generated",
				"Retry at most once",
			},
		},
		{
			skillID: SkillFileManager,
			want: []string{
				"Success evidence",
				"`managed_file_id`",
				"`upload_file_id`",
				"Truthfulness Contract",
				"Retry at most once",
			},
		},
		{
			skillID: SkillFileReader,
			want: []string{
				"Success evidence",
				"`content_status`",
				"`content_status=extracted`",
				"Truthfulness Contract",
			},
		},
		{
			skillID: SkillConsoleNavigator,
			want: []string{
				"Success Evidence",
				"`route_loaded`",
				"`observed_path`",
				"Truthfulness Contract",
				"old page context",
			},
		},
		{
			skillID: SkillAgentManagement,
			want: []string{
				"Success Evidence",
				"`agent_id`",
				"`updated_fields`",
				"`provider` and `model` together",
				"Truthfulness Contract",
			},
		},
	}

	catalogDir := defaultSkillCatalogDir()
	for _, tt := range tests {
		t.Run(tt.skillID, func(t *testing.T) {
			path := filepath.Join(catalogDir, tt.skillID, "SKILL.md")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", path, err)
			}
			body := string(raw)
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Fatalf("SKILL.md missing evidence contract phrase %q", want)
				}
			}
		})
	}
}
