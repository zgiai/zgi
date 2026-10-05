//go:build linux

package containerd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	runtimeoptions "github.com/containerd/containerd/api/types/runtimeoptions/v1"
	corecontainers "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const testImage = "registry.example.invalid/zgi/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestValidateOptionsRequiresGVisorAndPinnedImages(t *testing.T) {
	valid := defaultOptions(Options{
		RootDir:  t.TempDir(),
		Profiles: map[string]Profile{"python": {Image: testImage}},
	})
	if err := validateOptions(valid); err != nil {
		t.Fatalf("valid options: %v", err)
	}
	invalidRuntime := valid
	invalidRuntime.RuntimeName = "io.containerd.runc.v2"
	if err := validateOptions(invalidRuntime); err == nil {
		t.Fatal("runc runtime was accepted")
	}
	invalidImage := valid
	invalidImage.Profiles = map[string]Profile{"python": {Image: "registry.example.invalid/zgi/runtime:latest", RuntimeAgentPath: defaultAgentPath}}
	if err := validateOptions(invalidImage); err == nil {
		t.Fatal("floating image was accepted")
	}
}

func TestHardenSpecDropsPrivilegesAndAppliesLimits(t *testing.T) {
	provider := &Provider{memoryBytes: 1024, pids: 8, cpuPeriod: 1000, cpuQuota: 500, userID: 123, groupID: 456}
	spec := &oci.Spec{
		Process: &specs.Process{}, Linux: &specs.Linux{},
		Annotations: map[string]string{"example": "keep", gvisorHostUDSAnnotation: "all"},
	}
	var container *corecontainers.Container
	if err := provider.hardenSpec()(context.Background(), nil, container, spec); err != nil {
		t.Fatalf("harden spec: %v", err)
	}
	if spec.Process.Cwd != "/workspace" || !spec.Process.NoNewPrivileges {
		t.Fatalf("process hardening = %+v", spec.Process)
	}
	if got := spec.Annotations[gvisorContainerTypeAnnotation]; got != gvisorSandboxContainerType {
		t.Fatalf("gVisor container type = %q, want %q", got, gvisorSandboxContainerType)
	}
	if got := spec.Annotations[gvisorHostUDSAnnotation]; got != gvisorHostUDSCreate {
		t.Fatalf("gVisor host UDS mode = %q, want %q", got, gvisorHostUDSCreate)
	}
	if got := spec.Annotations["example"]; got != "keep" {
		t.Fatalf("existing annotation = %q, want keep", got)
	}
	if spec.Process.User.UID != 123 || spec.Process.User.GID != 456 {
		t.Fatalf("process user = %+v", spec.Process.User)
	}
	if spec.Process.Capabilities == nil || len(spec.Process.Capabilities.Effective) != 0 {
		t.Fatalf("capabilities = %+v", spec.Process.Capabilities)
	}
	if *spec.Linux.Resources.Memory.Limit != 1024 || *spec.Linux.Resources.Pids.Limit != 8 || *spec.Linux.Resources.CPU.Quota != 500 {
		t.Fatalf("resources = %+v", spec.Linux.Resources)
	}
}

func TestPrepareOwnedDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := prepareOwnedDirectory(link, uint32(os.Getuid()), uint32(os.Getgid()), 0o750); err == nil {
		t.Fatal("symlink directory was accepted")
	}
}

func TestResolveCreateRejectsUnknownProfileAndMismatchedAllocation(t *testing.T) {
	provider := &Provider{profiles: map[string]Profile{"python": {Image: testImage}}}
	allocation, _, err := provider.resolveCreate(agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "missing"})
	if err == nil || allocation.BindingRef == "" {
		t.Fatalf("resolve = %+v, %v", allocation, err)
	}
	if err := validateAllocation(agentexecution.Allocation{BindingRef: "../escape", WorkspaceRef: "workspace-1"}); !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("allocation error = %v", err)
	}
}

func TestLaunchRejectsUnknownNetworkModeBeforeAllocation(t *testing.T) {
	provider := &Provider{}
	_, err := provider.launch(context.Background(), agentexecution.CreateSpec{NetworkMode: "public-web"})
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("unsupported network mode error = %v, want ErrBackendUnavailable", err)
	}
}

func TestResolveCreateUsesApprovedPinnedHomeImage(t *testing.T) {
	provider := &Provider{profiles: map[string]Profile{"python": {Image: testImage}}}
	prepared := "registry.example.invalid/zgi/prepared@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, profile, err := provider.resolveCreate(agentexecution.CreateSpec{
		BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "python",
		HomeSnapshotID: "prepared", HomeSnapshotRef: prepared,
	})
	if err != nil || profile.Image != prepared {
		t.Fatalf("prepared image = %+v, %v", profile, err)
	}
	if _, _, err := provider.resolveCreate(agentexecution.CreateSpec{
		BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "python",
		HomeSnapshotID: "prepared", HomeSnapshotRef: "registry.example.invalid/zgi/prepared:latest",
	}); !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("floating prepared image accepted: %v", err)
	}
}

func TestResolveCreateRequiresControlledEgressAllowlist(t *testing.T) {
	provider := &Provider{profiles: map[string]Profile{"python": {Image: testImage}}}
	_, _, err := provider.resolveCreate(agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "python", NetworkMode: "controlled-http"})
	if !errors.Is(err, agentexecution.ErrInvalidRequest) {
		t.Fatalf("missing egress allowlist error = %v", err)
	}
	provider.profiles["python"] = Profile{Image: testImage, EgressAllowedHosts: []string{"pypi.org"}}
	if _, _, err := provider.resolveCreate(agentexecution.CreateSpec{BindingID: "binding-1", WorkspaceID: "workspace-1", ProfileID: "python", NetworkMode: "controlled-http"}); err != nil {
		t.Fatalf("valid controlled profile rejected: %v", err)
	}
}

func TestRunscRuntimeOptionsOnlyEnableControlledLoopback(t *testing.T) {
	if options := runscRuntimeOptions("disabled", filepath.Join(t.TempDir(), "runsc.toml")); options != nil {
		t.Fatalf("disabled runtime options = %#v, want nil", options)
	}
	options, ok := runscRuntimeOptions("controlled-http", "/var/lib/zgi/runsc-controlled.toml").(*runtimeoptions.Options)
	if !ok || options.TypeUrl != "io.containerd.runsc.v1.options" || options.ConfigPath != "/var/lib/zgi/runsc-controlled.toml" {
		t.Fatalf("controlled runtime options = %#v", options)
	}
}

func TestEnsureControlledRunscConfigRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.toml")
	if err := os.WriteFile(target, []byte("[runsc_config]\nnetwork = \"host\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "runsc-controlled.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := (&Provider{runscConfig: link}).ensureControlledRunscConfig(); err == nil {
		t.Fatal("symlinked runsc config was accepted")
	}
}
