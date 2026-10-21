package runtimeagent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
)

func TestDirectPackageProxyRejectsNonLoopbackListenerAndInvalidPolicy(t *testing.T) {
	for _, test := range []struct {
		address string
		hosts   []string
	}{
		{"0.0.0.0:0", []string{"pypi.org"}},
		{"localhost:0", []string{"pypi.org"}},
		{"127.0.0.1:0", nil},
		{"127.0.0.1:0", []string{"127.0.0.1"}},
	} {
		proxy, err := newDirectPackageProxy(test.address, egressproxy.Policy{AllowedHosts: test.hosts})
		if err == nil || proxy != nil {
			t.Fatalf("accepted invalid direct proxy configuration: %+v", test)
		}
	}
}

func TestDirectPackageProxyRejectsNonHTTPSPrivateAndUnapprovedDestinations(t *testing.T) {
	proxy, err := newDirectPackageProxy("127.0.0.1:0", egressproxy.Policy{AllowedHosts: []string{"localhost"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	for _, test := range []struct{ method, target string }{
		{http.MethodGet, "localhost:443"},
		{http.MethodConnect, "localhost:443"},
		{http.MethodConnect, "localhost:8443"},
		{http.MethodConnect, "example.com:443"},
		{http.MethodConnect, "169.254.169.254:80"},
	} {
		request := httptest.NewRequest(test.method, "http://"+test.target, nil)
		request.Host = test.target
		response := httptest.NewRecorder()
		proxy.handle(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d, expected forbidden", test.method, test.target, response.Code)
		}
	}
}
