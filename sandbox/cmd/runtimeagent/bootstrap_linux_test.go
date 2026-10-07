//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2BBootstrapRequiresJobMetadataGuard(t *testing.T) {
	source, err := os.ReadFile("../../scripts/agent-runtime-e2b-bootstrap.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, mode, uid, firewall string
		wantSuccess, wantGuard    bool
	}{
		{"new-rule", "direct", "65532", "new", true, true},
		{"existing-rule", "direct", "65532", "existing", true, true},
		{"insert-fails", "direct", "65532", "fail", false, true},
		{"root-job", "direct", "0", "existing", false, false},
		{"invalid-job", "direct", "65532 -j ACCEPT", "existing", false, false},
		{"bridge", "bridge", "65532", "fail", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			firewall := filepath.Join(root, "nft")
			agent := filepath.Join(root, "agent")
			bootstrap := filepath.Join(root, "bootstrap")
			for name, content := range map[string]string{
				firewall:                     "#!/bin/sh\ncase \"$*\" in '--file -') cat >> \"$TEST_ROOT/firewall.log\"; [ \"$TEST_FIREWALL\" != fail ];; *) printf '%s\\n' 'meta skuid 65532 reject with icmp 3' 'meta skuid 65532 ip daddr 127.0.0.1 tcp dport 3128 accept';; esac\n",
				agent:                        "#!/bin/sh\ntouch \"$TEST_ROOT/agent-started\"\n",
				filepath.Join(root, "chown"): "#!/bin/sh\nprintf '%s\\n' \"chown $*\" >> \"$TEST_ROOT/permissions.log\"\n",
				filepath.Join(root, "chmod"): "#!/bin/sh\nprintf '%s\\n' \"chmod $*\" >> \"$TEST_ROOT/permissions.log\"\n",
				filepath.Join(root, "token"): "test-bootstrap-token",
			} {
				if err := os.WriteFile(name, []byte(content), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			fixture := strings.ReplaceAll(string(source), "/usr/sbin/nft", firewall)
			fixture = strings.ReplaceAll(fixture, "/usr/local/bin/zgi-runtime-agent", agent)
			fixture = strings.ReplaceAll(fixture, "/usr/bin/chown", filepath.Join(root, "chown"))
			fixture = strings.ReplaceAll(fixture, "/usr/bin/chmod", filepath.Join(root, "chmod"))
			if err := os.WriteFile(bootstrap, []byte(fixture), 0o700); err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(t.Context(), "/bin/sh", bootstrap)
			command.Env = append(os.Environ(), "TEST_ROOT="+root, "TEST_FIREWALL="+test.firewall,
				"ZGI_RUNTIME_AGENT_TOKEN_FILE="+filepath.Join(root, "token"),
				"ZGI_RUNTIME_AGENT_EGRESS_MODE="+test.mode, "ZGI_RUNTIME_AGENT_JOB_UID="+test.uid)
			output, err := command.CombinedOutput()
			if (err == nil) != test.wantSuccess {
				t.Fatalf("bootstrap error=%v output=%s", err, output)
			}
			_, startedErr := os.Stat(filepath.Join(root, "agent-started"))
			if (startedErr == nil) != test.wantSuccess {
				t.Fatalf("agent startup=%v, expected=%v", startedErr, test.wantSuccess)
			}
			log, _ := os.ReadFile(filepath.Join(root, "firewall.log"))
			if strings.Contains(string(log), "flush chain inet zgi_runtime_guard output") != test.wantGuard {
				t.Fatalf("unexpected firewall calls: %s", log)
			}
			if test.mode == "direct" && test.wantSuccess && (!strings.Contains(string(log), "meta skuid 65532 reject") || !strings.Contains(string(log), "meta skuid 65532 ip daddr 127.0.0.1 tcp dport 3128 accept")) {
				t.Fatalf("Job metadata guard missing: %s", log)
			}
			permissions, _ := os.ReadFile(filepath.Join(root, "permissions.log"))
			if test.wantGuard && (!strings.Contains(string(permissions), "chown -R root:root /usr/local /opt/zgi-profile /opt/corepack") || !strings.Contains(string(permissions), "chmod -R go-w /usr/local /opt/zgi-profile /opt/corepack")) {
				t.Fatalf("Profile permission hardening missing: %s", permissions)
			}
		})
	}
}

func TestE2BBootstrapFailsClosedWhenProfileHardeningFails(t *testing.T) {
	source, err := os.ReadFile("../../scripts/agent-runtime-e2b-bootstrap.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("test-bootstrap-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	// No real permissions or firewall operations may run from this fixture.
	fixture := strings.ReplaceAll(string(source), "/usr/bin/chown", "/bin/false")
	fixture = strings.ReplaceAll(fixture, "/usr/bin/chmod", "/bin/false")
	fixture = strings.ReplaceAll(fixture, "/usr/sbin/nft", "/bin/false")
	fixture = strings.ReplaceAll(fixture, "exec /usr/local/bin/zgi-runtime-agent", "exit 0")
	command := exec.CommandContext(t.Context(), "/bin/sh", "-c", fixture)
	command.Env = append(os.Environ(), "ZGI_RUNTIME_AGENT_TOKEN_FILE="+token,
		"ZGI_RUNTIME_AGENT_EGRESS_MODE=direct", "ZGI_RUNTIME_AGENT_JOB_UID=65532")
	if err := command.Run(); err == nil {
		t.Fatal("Runtime Agent started despite failed Profile permission preparation")
	}
}
