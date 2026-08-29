package integrations

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestDefaultSafetyCheckerBlocksSensitiveExternalInput(t *testing.T) {
	action := safetyTestAction(ActionWebSearch, "search_web")
	action.DataEgress = true
	tests := []struct {
		name  string
		value string
	}{
		{name: "bearer token", value: "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"},
		{name: "basic authorization", value: "Authorization: Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="},
		{name: "aws authorization", value: "Authorization: AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20260720/us-east-1/s3/aws4_request, Signature=abcdef1234567890"},
		{name: "API key header", value: "X-API-Key: abcdefghijklmnopqrstuvwxyz"},
		{name: "session cookie", value: "Cookie: session=abcdefghijklmnopqrstuvwxyz"},
		{name: "private key", value: "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----"},
		{name: "api key assignment", value: "api_key=abcdefghijklmnopqrstuvwxyz"},
		{name: "provider key", value: "exa-abcdefghijklmnopqrstuvwxyz"},
		{name: "github pat", value: "ghp_abcdefghijklmnopqrstuvwxyz1234567890"},
		{name: "github fine-grained pat", value: "github_pat_11ABCDEFG0abcdefghijklmnopqrstuvwxyz_123456789"},
		{name: "jwt", value: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"},
		{name: "aws access key", value: "AKIAIOSFODNN7EXAMPLE"},
		{name: "credential URL", value: "postgres://alice:supersecret@db.example.com/app"},
		{name: "internal connection string", value: "postgresql://db.internal:5432/app"},
		{name: "bare internal hostname", value: "query service.internal:8443 metrics"},
		{name: "bare private IPv4", value: "query 10.0.0.1 diagnostics"},
		{name: "bare loopback IPv6", value: "query [::1] diagnostics"},
		{name: "token query", value: "https://example.com/article?token=secret"},
		{name: "embedded internal URL", value: "find documentation for http://service.internal/admin today"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (DefaultSafetyChecker{}).Check(context.Background(), action, map[string]interface{}{"query": tt.value})
			if ErrorCode(err) != ErrorCodeSensitiveInput {
				t.Fatalf("Check() error = %v, code = %q", err, ErrorCode(err))
			}
		})
	}
}

func TestDefaultSafetyCheckerValidatesSearchDomainFilters(t *testing.T) {
	action := safetyTestAction(ActionWebSearch, "search_web")
	action.DataEgress = true
	publicResolver := &testDNSResolver{addresses: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
	checker := DefaultSafetyChecker{Resolver: publicResolver}
	if err := checker.Check(context.Background(), action, map[string]interface{}{
		"query":           "current information",
		"include_domains": []string{"example.com"},
		"exclude_domains": []interface{}{"news.example.com"},
	}); err != nil {
		t.Fatalf("Check() public domain filters error = %v", err)
	}

	tests := []struct {
		name     string
		value    string
		resolver DNSResolver
	}{
		{name: "local suffix", value: "service.internal", resolver: publicResolver},
		{name: "private literal", value: "10.0.0.1", resolver: publicResolver},
		{name: "URL instead of domain", value: "https://example.com/path", resolver: publicResolver},
		{name: "DNS resolves private", value: "attacker.example", resolver: &testDNSResolver{addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (DefaultSafetyChecker{Resolver: tt.resolver}).Check(context.Background(), action, map[string]interface{}{
				"query":           "current information",
				"include_domains": []string{tt.value},
			})
			if ErrorCode(err) != ErrorCodeSensitiveInput {
				t.Fatalf("Check() error = %v, code = %q", err, ErrorCode(err))
			}
		})
	}
}

func TestDefaultSafetyCheckerDoesNotScanNonEgressInput(t *testing.T) {
	action := safetyTestAction(ActionWebSearch, "search_web")
	err := (DefaultSafetyChecker{}).Check(context.Background(), action, map[string]interface{}{
		"query": "Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
	})
	if err != nil {
		t.Fatalf("Check() non-egress error = %v", err)
	}
}

func TestDefaultSafetyCheckerScansMapKeys(t *testing.T) {
	action := safetyTestAction(ActionWebSearch, "search_web")
	action.DataEgress = true
	err := (DefaultSafetyChecker{}).Check(context.Background(), action, map[string]interface{}{
		"query": "current information",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz": "x",
	})
	if ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("Check() error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestValidateExternalWebURL(t *testing.T) {
	allowed := []string{
		"https://example.com/article",
		"http://8.8.8.8/index.html?lang=en",
	}
	for _, raw := range allowed {
		if err := validateExternalWebURL(raw); err != nil {
			t.Errorf("validateExternalWebURL(%q) error = %v", raw, err)
		}
	}

	blocked := []string{
		"not-a-url",
		"ftp://example.com/file",
		"https://alice:secret@example.com/article",
		"http://localhost/admin",
		"http://service.local/resource",
		"http://service.internal/resource",
		"http://127.0.0.1/admin",
		"http://10.0.0.1/admin",
		"http://169.254.169.254/latest/meta-data",
		"http://0.0.0.0/admin",
		"http://[::1]/admin",
		"http://[2001:db8::1]/admin",
		"http://127.1/admin",
		"http://2130706433/admin",
		"http://100.64.0.1/admin",
		"http://127.0.0.1.nip.io/admin",
		"http://localtest.me/admin",
		"https://example.com/article?access_token=secret",
		"https://example.com/article?X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20260720%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Signature=abcdef",
		"https://example.com/article?X-Goog-Credential=service%40example.com&X-Goog-Signature=abcdef",
		"https://example.com/article#token=opaquecredential1234567890",
	}
	for _, raw := range blocked {
		if err := validateExternalWebURL(raw); err == nil {
			t.Errorf("validateExternalWebURL(%q) error = nil, want blocked", raw)
		}
	}
}

func TestDefaultSafetyCheckerValidatesFetchURLs(t *testing.T) {
	action := safetyTestAction(ActionWebFetch, "fetch_webpage")
	action.DataEgress = true
	checker := DefaultSafetyChecker{Resolver: &testDNSResolver{
		addresses: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}},
	}}

	if err := checker.Check(context.Background(), action, map[string]interface{}{
		"urls": []interface{}{"https://example.com/article"},
	}); err != nil {
		t.Fatalf("Check() public URL error = %v", err)
	}

	err := checker.Check(context.Background(), action, map[string]interface{}{
		"urls": []string{"http://192.168.1.10/internal"},
	})
	if ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("Check() private URL error = %v, code = %q", err, ErrorCode(err))
	}

	err = checker.Check(context.Background(), action, map[string]interface{}{
		"urls": []string{"https://example.com/article?api_key=secret"},
	})
	if ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("Check() token URL error = %v, code = %q", err, ErrorCode(err))
	}
}

func TestDefaultSafetyCheckerBlocksDNSRebindingAnswers(t *testing.T) {
	resolver := &testDNSResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("93.184.216.34")},
		{IP: net.ParseIP("127.0.0.1")},
	}}
	checker := DefaultSafetyChecker{Resolver: resolver}

	fetch := safetyTestAction(ActionWebFetch, "fetch_webpage")
	fetch.DataEgress = true
	err := checker.Check(context.Background(), fetch, map[string]interface{}{
		"urls": []string{"https://attacker.example/article"},
	})
	if ErrorCode(err) != ErrorCodeInvalidInput {
		t.Fatalf("Check() mixed DNS answer error = %v, code = %q", err, ErrorCode(err))
	}

	search := safetyTestAction(ActionWebSearch, "search_web")
	search.DataEgress = true
	err = checker.Check(context.Background(), search, map[string]interface{}{
		"query": "summarize https://attacker.example/article",
	})
	if ErrorCode(err) != ErrorCodeSensitiveInput {
		t.Fatalf("Check() embedded mixed DNS answer error = %v, code = %q", err, ErrorCode(err))
	}
	if resolver.calls != 2 {
		t.Fatalf("resolver calls = %d, want 2", resolver.calls)
	}
}

func TestDefaultSafetyCheckerBlocksNonPublicDNSAnswers(t *testing.T) {
	action := safetyTestAction(ActionWebFetch, "fetch_webpage")
	action.DataEgress = true
	tests := []struct {
		name string
		ip   string
	}{
		{name: "private", ip: "10.0.0.1"},
		{name: "loopback", ip: "127.0.0.1"},
		{name: "link local", ip: "169.254.169.254"},
		{name: "carrier grade NAT", ip: "100.64.0.1"},
		{name: "multicast", ip: "224.0.0.1"},
		{name: "reserved documentation IPv4", ip: "198.51.100.10"},
		{name: "reserved documentation IPv6", ip: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := DefaultSafetyChecker{Resolver: &testDNSResolver{
				addresses: []net.IPAddr{{IP: net.ParseIP(tt.ip)}},
			}}
			err := checker.Check(context.Background(), action, map[string]interface{}{
				"urls": []string{"https://attacker.example/article"},
			})
			if ErrorCode(err) != ErrorCodeInvalidInput {
				t.Fatalf("Check() DNS answer %s error = %v, code = %q", tt.ip, err, ErrorCode(err))
			}
		})
	}
}

func TestDefaultSafetyCheckerFailsClosedWhenDNSResolutionFails(t *testing.T) {
	action := safetyTestAction(ActionWebFetch, "fetch_webpage")
	action.DataEgress = true

	tests := []struct {
		name     string
		resolver *testDNSResolver
	}{
		{name: "lookup error", resolver: &testDNSResolver{err: errors.New("DNS unavailable")}},
		{name: "empty answer", resolver: &testDNSResolver{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (DefaultSafetyChecker{Resolver: tt.resolver}).Check(context.Background(), action, map[string]interface{}{
				"urls": []string{"https://unresolved.example/article"},
			})
			if ErrorCode(err) != ErrorCodeInvalidInput {
				t.Fatalf("Check() error = %v, code = %q", err, ErrorCode(err))
			}
		})
	}
}

func TestDefaultSafetyCheckerDoesNotResolvePlainSearchQuery(t *testing.T) {
	resolver := &testDNSResolver{err: errors.New("must not be called")}
	checker := DefaultSafetyChecker{Resolver: resolver}
	action := safetyTestAction(ActionWebSearch, "search_web")
	action.DataEgress = true

	if err := checker.Check(context.Background(), action, map[string]interface{}{
		"query": "latest database security guidance",
	}); err != nil {
		t.Fatalf("Check() plain query error = %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
}

type testDNSResolver struct {
	addresses []net.IPAddr
	err       error
	calls     int
}

func (resolver *testDNSResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	resolver.calls++
	return resolver.addresses, resolver.err
}

func safetyTestAction(id, toolName string) ActionDefinition {
	return ActionDefinition{ID: id, ToolName: toolName}
}
