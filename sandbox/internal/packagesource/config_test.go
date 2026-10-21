package packagesource

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfiguredHostsAreNormalizedAndDeduplicated(t *testing.T) {
	sources := Config{PyPI: "https://MIRROR.example/simple", APT: "https://mirror.example/debian", APTSecurity: "https://mirror.example/security"}
	if hosts := sources.Hosts(); !slices.Equal(hosts, []string{"mirror.example"}) {
		t.Fatalf("configured hosts=%v", hosts)
	}
}

func TestConfigRejectsCredentialsInjectionAndUnapprovedSources(t *testing.T) {
	for _, value := range []string{"http://packages.example/simple", "https://token@packages.example/simple", "https://packages.example:8443/simple", "https://127.0.0.1/simple", "https://packages.example/simple?q=token", "https://packages.example/simple#fragment", "https://packages.example/\ninject", "https://packages.example/%22inject"} {
		if err := (Config{PyPI: value}).Validate(nil); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	c := Config{PyPI: "https://packages.example/simple"}
	if err := c.Validate([]string{"pypi.org"}); err == nil {
		t.Fatal("unapproved mirror accepted")
	}
	if err := c.Validate([]string{"packages.example"}); err != nil {
		t.Fatal(err)
	}
	if got := (Config{}).Resolved(); got.PyPI != "https://pypi.org/simple" || got.NPM != "https://registry.npmjs.org/" {
		t.Fatalf("defaults = %+v", got)
	}
}

func TestAPTConfigRetainsSuitesAndSignaturesAndRefusesExistingDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "config")
	c := Config{APT: "https://mirror.example/debian", APTSecurity: "https://mirror.example/security"}
	path, err := c.WriteAPTConfig(directory)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := os.ReadFile(filepath.Join(directory, "sources.sources"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"https://mirror.example/debian", "bookworm bookworm-updates", "bookworm-security", "Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg"} {
		if !strings.Contains(string(sources), expected) {
			t.Errorf("missing %q in %s", expected, sources)
		}
	}
	config, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(config), `Dir::Etc::sourceparts "-"`) {
		t.Fatalf("apt config %s err=%v", config, err)
	}
	if _, err := c.WriteAPTConfig(directory); err == nil {
		t.Fatal("replaced existing configuration")
	}
}

type probeTransport func(*http.Request) (*http.Response, error)

func (f probeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSourceProbeIsBoundedAndRejectsHTTPFailures(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: probeTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		status := http.StatusOK
		if strings.Contains(r.URL.Path, "bookworm-security") {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("metadata")), Header: make(http.Header), Request: r}, nil
	})}
	if err := (Config{}).Check(t.Context(), client); err == nil || requests != 4 {
		t.Fatalf("probes=%d error=%v", requests, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client.Transport = probeTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	if err := (Config{}).Check(ctx, client); err == nil {
		t.Fatal("canceled probe succeeded")
	}
}
