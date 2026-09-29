package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStageAgentSkillPackageCachesByWorkspaceMarker(t *testing.T) {
	root := testPortableSkillPackage(t)
	digest, err := skills.SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	fake := &fakeAgentExecutionGateway{job: agentExecutionJob{ID: "setup-job", Status: "succeeded", ExitCode: &exitCode}}
	scope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	binding := agentExecutionBinding{ID: "binding-1", Status: "active", Generation: 1}
	capabilities := agentExecutionCapabilities{Shell: true, FileTransfer: true, NetworkMode: "disabled", MaxUploadBytes: 64 << 20}
	pack := agentSkillPackage{ID: "portable-example", Root: root, ExpectedDigest: digest}
	for i := 0; i < 2; i++ {
		got, err := stageAgentSkillPackage(context.Background(), fake, scope, binding, capabilities, pack)
		if err != nil {
			t.Fatalf("stage %d: %v", i, err)
		}
		if got != ".zgi/skills/portable-example/"+digest {
			t.Fatalf("stage %d path = %q", i, got)
		}
	}
	if fake.setupJobs != 1 || len(fake.uploads) != 1 {
		t.Fatalf("setup jobs = %d, uploads = %d, want one each", fake.setupJobs, len(fake.uploads))
	}
	fake.downloads = nil // A recreated workspace has no staged package.
	binding.ID = "binding-2"
	if _, err := stageAgentSkillPackage(context.Background(), fake, scope, binding, capabilities, pack); err != nil {
		t.Fatal(err)
	}
	if fake.setupJobs != 2 {
		t.Fatalf("setup jobs after recreation = %d, want 2", fake.setupJobs)
	}
}

func TestStageAgentSkillPackageRejectsChangedVersionBeforeUpload(t *testing.T) {
	root := testPortableSkillPackage(t)
	fake := &fakeAgentExecutionGateway{}
	_, err := stageAgentSkillPackage(context.Background(), fake, agentExecutionScope{}, agentExecutionBinding{ID: "binding-1"}, agentExecutionCapabilities{FileTransfer: true, MaxUploadBytes: 64 << 20}, agentSkillPackage{ID: "portable-example", Root: root, ExpectedDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err == nil || len(fake.uploads) != 0 {
		t.Fatalf("version mismatch error = %v, uploads = %d", err, len(fake.uploads))
	}
}

func TestAgentExecutionShellStagesRequestedSkillBeforeUserCommand(t *testing.T) {
	root := testPortableSkillPackage(t)
	digest, err := skills.SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	fake := &fakeAgentExecutionGateway{
		binding:      agentExecutionBinding{ID: "binding-1", Status: "active", Generation: 1},
		job:          agentExecutionJob{ID: "job-1", Status: "succeeded", ExitCode: &exitCode},
		capabilities: agentExecutionCapabilities{Shell: true, FileTransfer: true, NetworkMode: "disabled", MaxUploadBytes: 64 << 20},
	}
	resolve := func(context.Context, string) (agentSkillPackage, error) {
		return agentSkillPackage{ID: "portable-example", Root: root, ExpectedDigest: digest}, nil
	}
	tool := newAgentExecutionShellTool(fake, agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", nil, nil, resolve)
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"skill_id":"portable-example","command":"python3 \"$ZGI_SKILL_DIR/scripts/task.py\""}`}})
	if result.Error != nil || result.Status != "success" {
		t.Fatalf("shell result = %#v, want successful staged command", result)
	}
	if fake.setupJobs != 1 || fake.command != "/bin/sh" || !strings.Contains(fake.args[1], "ZGI_SKILL_DIR='/workspace/.zgi/skills/portable-example/"+digest+"'") {
		t.Fatalf("setup jobs = %d, command = %q, args = %#v", fake.setupJobs, fake.command, fake.args)
	}
	if result.ModelResult["skill_workspace_path"] != ".zgi/skills/portable-example/"+digest {
		t.Fatalf("model Skill path = %#v", result.ModelResult["skill_workspace_path"])
	}
}

func TestAgentExecutionShellDoesNotRunCommandWhenSkillSetupFails(t *testing.T) {
	root := testPortableSkillPackage(t)
	exitCode := 1
	fake := &fakeAgentExecutionGateway{
		binding:      agentExecutionBinding{ID: "binding-1", Status: "active", Generation: 1},
		job:          agentExecutionJob{ID: "setup-job", Status: "failed", ExitCode: &exitCode},
		capabilities: agentExecutionCapabilities{Shell: true, FileTransfer: true, NetworkMode: "disabled", MaxUploadBytes: 64 << 20},
	}
	resolve := func(context.Context, string) (agentSkillPackage, error) {
		return agentSkillPackage{ID: "portable-example", Root: root}, nil
	}
	tool := newAgentExecutionShellTool(fake, agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}, "agent-1", "conversation-1", "message-1", "standard", "local-gvisor", nil, nil, resolve)
	result := tool.Handler(t.Context(), adapter.ToolCall{ID: "call-1", Function: adapter.FunctionCall{Arguments: `{"skill_id":"portable-example","command":"echo should-not-run"}`}})
	if result.Status != "failed" || result.ModelResult["error_code"] != "skill_package_staging_failed" || fake.command != "python3" {
		t.Fatalf("shell result = %#v, command = %q, want setup failure before user command", result, fake.command)
	}
}

func TestAgentSkillPackageResolverRejectsOtherTenantAndDisabledSkill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:agent-skill-package?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.CustomSkill{}); err != nil {
		t.Fatal(err)
	}
	organizationID := uuid.New()
	otherOrganizationID := uuid.New()
	record := runtimemodel.CustomSkill{
		ID: uuid.New(), OrganizationID: otherOrganizationID, SkillID: "portable-example",
		Name: "portable-example", Description: "Example", WhenToUse: "Example", RuntimeType: "prompt",
		StoragePath: testPortableSkillPackage(t), Status: runtimemodel.CustomSkillStatusActive,
		CreatedBy: uuid.New(), Display: map[string]interface{}{}, Manifest: map[string]interface{}{},
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	resolve := newAgentSkillPackageResolver(db, organizationID, []string{"portable-example"})
	if _, err := resolve(t.Context(), "portable-example"); err == nil {
		t.Fatal("other tenant's Skill was resolved")
	}
	resolve = newAgentSkillPackageResolver(db, otherOrganizationID, nil)
	if _, err := resolve(t.Context(), "portable-example"); err == nil {
		t.Fatal("disabled Skill was resolved")
	}
}

func testPortableSkillPackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: portable-example\ndescription: Example.\n---\nRun scripts/task.py.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "task.py"), []byte("print('done')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
