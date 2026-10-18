package serverops

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The Skill manifest must keep mapping onto the existing capability methods.
// These compile-time assignments fail if a result type, a parameter list, or a
// method disappears or changes shape, so the manifest can never drift onto a
// route this package does not implement.
var (
	_ func(*Service, context.Context, Connection) (*SystemSnapshot, error)                      = (*Service).SystemSnapshot
	_ func(*Service, context.Context, Connection, string) (*ServiceStatus, error)               = (*Service).ServiceStatus
	_ func(*Service, context.Context, Connection, string, JournalOptions) (*JournalTail, error) = (*Service).JournalTail
)

// The only parameters the Skill may declare, and the kind each one has.
var skillAcceptedParameters = map[string]ParameterKind{
	"service_name":  ParameterKindString,
	"lines":         ParameterKindInteger,
	"since_minutes": ParameterKindInteger,
}

// skillContractSource locates and reads contract.go next to this test.
func skillContractSource(t *testing.T) (string, []byte) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the contract test source")
	}
	path := filepath.Join(filepath.Dir(thisFile), "contract.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the Skill manifest source: %v", err)
	}
	return path, source
}

// skillContractAST parses contract.go so a test can prove properties of the
// declaration itself: no imports, no state, no registration, and no execution.
func skillContractAST(t *testing.T) (*ast.File, string) {
	t.Helper()
	path, source := skillContractSource(t)
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("cannot parse the Skill manifest source: %v", err)
	}
	return file, string(source)
}

// skillDeclaredConstNames collects the const names declared with one type.
func skillDeclaredConstNames(file *ast.File, typeName string) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := value.Type.(*ast.Ident)
			if !ok || ident.Name != typeName {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

func skillToolByName(t *testing.T, tools []ToolDescriptor, name ToolName) ToolDescriptor {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q is not part of the Skill", name)
	return ToolDescriptor{}
}

func skillParameterByName(t *testing.T, tool ToolDescriptor, name string) ParameterDescriptor {
	t.Helper()
	for _, parameter := range tool.Parameters {
		if parameter.Name == name {
			return parameter
		}
	}
	t.Fatalf("tool %q does not declare parameter %q", tool.Name, name)
	return ParameterDescriptor{}
}

// skillAssertServiceNameParameter pins the one required string parameter shape.
func skillAssertServiceNameParameter(t *testing.T, parameter ParameterDescriptor) {
	t.Helper()
	if parameter.Kind != ParameterKindString {
		t.Fatalf("service_name kind = %q, want %q", parameter.Kind, ParameterKindString)
	}
	if !parameter.Required {
		t.Fatal("service_name must be required")
	}
	// The declared pattern must be the runtime grammar itself, not a copy of it.
	if parameter.Pattern != SystemdServiceNamePattern {
		t.Fatalf("service_name pattern = %q, want the runtime grammar %q", parameter.Pattern, SystemdServiceNamePattern)
	}
	if parameter.Minimum != 0 || parameter.Maximum != 0 || parameter.Default != 0 || parameter.HasDefault {
		t.Fatalf("service_name must declare no numeric bound or default: %#v", parameter)
	}
	if strings.TrimSpace(parameter.Description) == "" {
		t.Fatal("service_name must be described")
	}
}

// skillAssertBoundedIntegerParameter pins one optional bounded integer parameter.
func skillAssertBoundedIntegerParameter(t *testing.T, parameter ParameterDescriptor, wantName string, minimum, maximum, defaultValue int64) {
	t.Helper()
	if parameter.Name != wantName {
		t.Fatalf("integer parameter name = %q, want %q", parameter.Name, wantName)
	}
	if parameter.Kind != ParameterKindInteger {
		t.Fatalf("%s kind = %q, want %q", wantName, parameter.Kind, ParameterKindInteger)
	}
	if parameter.Required {
		t.Fatalf("%s must be optional", wantName)
	}
	if parameter.Minimum != minimum || parameter.Maximum != maximum {
		t.Fatalf("%s bounds = [%d,%d], want [%d,%d]", wantName, parameter.Minimum, parameter.Maximum, minimum, maximum)
	}
	if !parameter.HasDefault || parameter.Default != defaultValue {
		t.Fatalf("%s default = (%d,%v), want %d", wantName, parameter.Default, parameter.HasDefault, defaultValue)
	}
	if parameter.Pattern != "" {
		t.Fatalf("%s must not declare a string pattern", wantName)
	}
	if strings.TrimSpace(parameter.Description) == "" {
		t.Fatalf("%s must be described", wantName)
	}
}

func TestSkillContractHasExactlyThreeReadOnlyTools(t *testing.T) {
	skill := Skill()
	if SkillID != "server-operations" || skill.ID != SkillID {
		t.Fatalf("SkillID = %q, skill.ID = %q, want %q", SkillID, skill.ID, "server-operations")
	}
	if strings.TrimSpace(skill.Summary) == "" {
		t.Fatal("the Skill summary must not be empty")
	}
	if len(skill.Tools) != 3 {
		t.Fatalf("Skill exposes %d tools, want exactly 3", len(skill.Tools))
	}

	want := []struct {
		name     ToolName
		actionID string
		returns  string
	}{
		{ToolSystemSnapshot, ActionSystemSnapshot, "SystemSnapshot"},
		{ToolServiceStatus, ActionServiceStatus, "ServiceStatus"},
		{ToolJournalTail, ActionJournalTail, "JournalTail"},
	}
	seen := make(map[ToolName]bool, len(want))
	for index, expected := range want {
		tool := skill.Tools[index]
		if tool.Name != expected.name {
			t.Errorf("tool %d name = %q, want %q", index, tool.Name, expected.name)
		}
		if tool.ActionID != expected.actionID {
			t.Errorf("tool %q action = %q, want %q", tool.Name, tool.ActionID, expected.actionID)
		}
		if tool.Returns != expected.returns {
			t.Errorf("tool %q returns %q, want the existing %q result type", tool.Name, tool.Returns, expected.returns)
		}
		if !tool.ReadOnly {
			t.Errorf("tool %q must be read-only", tool.Name)
		}
		if tool.RequiredApproval != ApprovalPerUseUserApproval {
			t.Errorf("tool %q approval = %q, want %q", tool.Name, tool.RequiredApproval, ApprovalPerUseUserApproval)
		}
		if strings.TrimSpace(tool.Summary) == "" {
			t.Errorf("tool %q must be summarised", tool.Name)
		}
		if seen[tool.Name] {
			t.Errorf("tool %q is declared twice", tool.Name)
		}
		seen[tool.Name] = true
	}
}

func TestSkillContractParametersMatchRuntimeValidation(t *testing.T) {
	skill := Skill()

	snapshot := skillToolByName(t, skill.Tools, ToolSystemSnapshot)
	if len(snapshot.Parameters) != 0 {
		t.Fatalf("system snapshot must declare no parameter, got %#v", snapshot.Parameters)
	}

	status := skillToolByName(t, skill.Tools, ToolServiceStatus)
	if len(status.Parameters) != 1 {
		t.Fatalf("service status declares %d parameters, want exactly service_name", len(status.Parameters))
	}
	skillAssertServiceNameParameter(t, skillParameterByName(t, status, "service_name"))

	journal := skillToolByName(t, skill.Tools, ToolJournalTail)
	if len(journal.Parameters) != 3 {
		t.Fatalf("journal tail declares %d parameters, want service_name, lines, and since_minutes", len(journal.Parameters))
	}
	skillAssertServiceNameParameter(t, journal.Parameters[0])
	lines := journal.Parameters[1]
	sinceMinutes := journal.Parameters[2]
	skillAssertBoundedIntegerParameter(t, lines, "lines", 1, maxJournalLines, defaultJournalLines)
	skillAssertBoundedIntegerParameter(t, sinceMinutes, "since_minutes", 1, maxJournalSinceMinutes, defaultJournalSinceMinutes)

	// The manifest must advertise the exact documented runtime window.
	if maxJournalLines != 200 || defaultJournalLines != 100 ||
		maxJournalSinceMinutes != 1440 || defaultJournalSinceMinutes != 60 {
		t.Fatalf("runtime journal window moved: lines [1,%d] default %d, since_minutes [1,%d] default %d",
			maxJournalLines, defaultJournalLines, maxJournalSinceMinutes, defaultJournalSinceMinutes)
	}

	// The runtime accepts exactly what the contract declares for service status.
	if _, _, err := buildServiceStatusCommand(map[string]any{"service_name": "nginx.service"}); err != nil {
		t.Fatalf("the declared service_name example is rejected by the runtime: %v", err)
	}
	for _, serviceName := range []any{"nginx", "nginx.service; uptime", " nginx.service", "nginx.service "} {
		if _, _, err := buildServiceStatusCommand(map[string]any{"service_name": serviceName}); err == nil {
			t.Fatalf("runtime accepted %v outside the declared service_name pattern", serviceName)
		}
	}
	if _, _, err := buildServiceStatusCommand(map[string]any{"service_name": "nginx.service", "lines": 5}); err == nil {
		t.Fatal("service status must reject a parameter the contract does not declare")
	}

	// The runtime applies the declared journal defaults.
	command, validated, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service"})
	if err != nil {
		t.Fatalf("the declared journal input is rejected by the runtime: %v", err)
	}
	if validated.Lines != int(lines.Default) || validated.SinceMinutes != int(sinceMinutes.Default) {
		t.Fatalf("runtime defaults = (%d,%d), want the declared (%d,%d)",
			validated.Lines, validated.SinceMinutes, lines.Default, sinceMinutes.Default)
	}
	if !strings.Contains(command, "--lines="+strconv.FormatInt(lines.Default, 10)) ||
		!strings.Contains(command, "--since=-"+strconv.FormatInt(sinceMinutes.Default, 10)+"min") {
		t.Fatalf("runtime command does not carry the declared defaults: %q", command)
	}
}

func TestSkillContractJournalBoundsMatchRuntimeValidation(t *testing.T) {
	journal := skillToolByName(t, Skill().Tools, ToolJournalTail)
	lines := journal.Parameters[1]
	sinceMinutes := journal.Parameters[2]

	for _, value := range []int64{lines.Minimum, lines.Maximum} {
		if _, _, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "lines": value}); err != nil {
			t.Errorf("runtime rejected the declared lines bound %d: %v", value, err)
		}
	}
	for _, value := range []int64{lines.Minimum - 1, lines.Maximum + 1} {
		if _, _, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "lines": value}); err == nil {
			t.Errorf("runtime accepted lines=%d outside the declared bound", value)
		}
	}
	for _, value := range []int64{sinceMinutes.Minimum, sinceMinutes.Maximum} {
		if _, _, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "since_minutes": value}); err != nil {
			t.Errorf("runtime rejected the declared since_minutes bound %d: %v", value, err)
		}
	}
	for _, value := range []int64{sinceMinutes.Minimum - 1, sinceMinutes.Maximum + 1} {
		if _, _, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "since_minutes": value}); err == nil {
			t.Errorf("runtime accepted since_minutes=%d outside the declared bound", value)
		}
	}
	if _, _, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "command": "uptime"}); err == nil {
		t.Fatal("journal tail must reject any key the contract does not declare")
	}
}

func TestSkillContractParameterSurfaceIsClosed(t *testing.T) {
	used := make(map[string]bool, len(skillAcceptedParameters))
	for _, tool := range Skill().Tools {
		for _, parameter := range tool.Parameters {
			kind, ok := skillAcceptedParameters[parameter.Name]
			if !ok {
				t.Fatalf("tool %q declares parameter %q, which is outside the accepted parameter set", tool.Name, parameter.Name)
			}
			if parameter.Kind != kind {
				t.Fatalf("parameter %q kind = %q, want %q", parameter.Name, parameter.Kind, kind)
			}
			used[parameter.Name] = true
		}
	}
	if len(used) != len(skillAcceptedParameters) {
		t.Fatalf("the manifest uses %d of the %d accepted parameters", len(used), len(skillAcceptedParameters))
	}

	// No parameter may name a secret, a target, or an execution surface.
	for _, tool := range Skill().Tools {
		for _, parameter := range tool.Parameters {
			lower := strings.ToLower(parameter.Name)
			for _, forbidden := range []string{
				"host", "username", "user_name", "port", "key", "passphrase", "password",
				"credential", "secret", "token", "connection", "command", "shell", "exec",
				"script", "path", "sudo", "fingerprint",
			} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("parameter %q must not reference %q", parameter.Name, forbidden)
				}
			}
		}
	}
}

func TestSkillContractCarriesNoSecretMetadata(t *testing.T) {
	// No descriptor field may even hold a target, a credential, or a command.
	for _, typ := range []reflect.Type{
		reflect.TypeOf(SkillDescriptor{}),
		reflect.TypeOf(ToolDescriptor{}),
		reflect.TypeOf(ParameterDescriptor{}),
	} {
		for index := 0; index < typ.NumField(); index++ {
			name := strings.ToLower(typ.Field(index).Name)
			for _, forbidden := range []string{
				"host", "username", "port", "key", "passphrase", "password",
				"credential", "secret", "token", "connection", "command", "shell", "exec", "path",
			} {
				if strings.Contains(name, forbidden) {
					t.Errorf("descriptor field %s.%s must not reference %q", typ.Name(), typ.Field(index).Name, forbidden)
				}
			}
		}
	}

	encoded, err := json.Marshal(Skill())
	if err != nil {
		t.Fatalf("cannot encode the Skill manifest: %v", err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"private_key", "privatekey", "host_key", "ssh_key", "key", "passphrase",
		"password", "credential", "secret", "token", "connection", "fingerprint",
		"driver_id", "auth_method",
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("the encoded Skill manifest must not carry %q", forbidden)
		}
	}
}

func TestSkillContractReturnsIndependentCopies(t *testing.T) {
	if !reflect.DeepEqual(Skill(), Skill()) {
		t.Fatal("the Skill manifest must be deterministic")
	}

	tampered := Skill()
	tampered.ID = "tampered"
	tampered.Summary = "tampered"
	tampered.Tools[0].Name = "tampered"
	tampered.Tools[0].ActionID = "tampered"
	tampered.Tools[0].Summary = "tampered"
	tampered.Tools[1].Parameters[0].Name = "tampered"
	tampered.Tools[2].Parameters = append(tampered.Tools[2].Parameters, ParameterDescriptor{Name: "command"})

	pristine := Skill()
	if pristine.ID != SkillID ||
		pristine.Tools[0].Name != ToolSystemSnapshot ||
		pristine.Tools[1].Parameters[0].Name != "service_name" ||
		len(pristine.Tools[2].Parameters) != 3 {
		t.Fatalf("mutating one manifest changed another: %#v", pristine)
	}
	if !reflect.DeepEqual(pristine, Skill()) {
		t.Fatal("the manifest must be unchanged after a caller mutates a previous copy")
	}

	looked, ok := SkillTool(ToolJournalTail)
	if !ok {
		t.Fatal("the journal tail tool must be addressable by name")
	}
	looked.Parameters[0].Name = "tampered"
	looked.Parameters = looked.Parameters[:1]
	again, ok := SkillTool(ToolJournalTail)
	if !ok || again.Parameters[0].Name != "service_name" || len(again.Parameters) != 3 {
		t.Fatalf("mutating a looked-up tool changed the manifest: %#v", again.Parameters)
	}
}

func TestSkillContractToolLookup(t *testing.T) {
	for _, tool := range Skill().Tools {
		looked, ok := SkillTool(tool.Name)
		if !ok {
			t.Fatalf("tool %q must be addressable by name", tool.Name)
		}
		if !reflect.DeepEqual(looked, tool) {
			t.Fatalf("lookup for %q = %#v, want %#v", tool.Name, looked, tool)
		}
	}
	for _, unknown := range []ToolName{"", "execute_action", "delete_service", "system.snapshot", "server.system.snapshot"} {
		if _, ok := SkillTool(unknown); ok {
			t.Errorf("%q must not resolve to a tool", unknown)
		}
	}
}

func TestSkillContractRequiresPerUseApproval(t *testing.T) {
	if ApprovalPerUseUserApproval != "per_use_user_approval" {
		t.Fatalf("ApprovalPerUseUserApproval = %q", ApprovalPerUseUserApproval)
	}
	for _, tool := range Skill().Tools {
		if tool.RequiredApproval != ApprovalPerUseUserApproval {
			t.Fatalf("tool %q approval = %q, want per-use user approval", tool.Name, tool.RequiredApproval)
		}
	}

	// The policy set is closed: contract.go declares exactly one ApprovalPolicy
	// constant, so no session, standing, or blanket policy can exist.
	file, source := skillContractAST(t)
	policies := skillDeclaredConstNames(file, "ApprovalPolicy")
	if len(policies) != 1 || policies[0] != "ApprovalPerUseUserApproval" {
		t.Fatalf("contract.go must declare exactly ApprovalPerUseUserApproval as an ApprovalPolicy, got %v", policies)
	}

	// This file declares the constraint without implementing it: no approval
	// check, approval store, or approval bypass may live here.
	for _, forbidden := range []string{
		"CheckApproval", "RequireApproval", "HasApproval", "SkipApproval",
		"approvalStore", "approvalCache", "approvalToken", "approvalGrant",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("contract.go must not implement approval behaviour (%q)", forbidden)
		}
	}
}

func TestSkillContractDoesNotRegisterOrHoldState(t *testing.T) {
	file, source := skillContractAST(t)

	if len(file.Imports) != 0 {
		t.Fatalf("contract.go must not import any package, found %d", len(file.Imports))
	}
	for _, decl := range file.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			if typed.Recv != nil {
				t.Fatalf("contract.go must declare no method, found %q", typed.Name.Name)
			}
			if typed.Name.Name == "init" {
				t.Fatal("contract.go must not declare init: reading the manifest must have no side effect")
			}
			lower := strings.ToLower(typed.Name.Name)
			for _, prefix := range []string{"register", "mustregister", "setup", "install", "execute", "invoke", "run"} {
				if strings.HasPrefix(lower, prefix) {
					t.Errorf("contract.go must not declare a runtime entry point (%q)", typed.Name.Name)
				}
			}
		case *ast.GenDecl:
			if typed.Tok == token.VAR {
				t.Fatal("contract.go must not declare package-level state")
			}
		}
	}

	// The manifest describes the capability; it never builds or runs a command.
	for _, forbidden := range []string{
		"buildSystemSnapshotCommand", "buildServiceStatusCommand", "buildJournalTailCommand",
		"shellSingleQuote", "newSSHClient", "prepareRunner", "newService",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("contract.go must not reference the execution path (%q)", forbidden)
		}
	}
}
