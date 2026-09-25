package consolecred

import "net/http"

// SignRequest adds the per-instance credential headers to an outgoing HTTP request.
//
// It is deliberately additive: the legacy shared-secret headers written by the caller
// stay in place, and when no instance credential is loaded this is a no-op. Console
// prefers the instance credential whenever X-Internal-Instance-Id is present.
func SignRequest(req *http.Request) {
	if req == nil {
		return
	}
	for name, value := range Headers(req.URL.Path) {
		req.Header.Set(name, value)
	}
}

// SignWithPath adds the instance headers for an explicit signed path. Use it when the
// signed path differs from req.URL.Path (for example a route alias).
func SignWithPath(req *http.Request, path string) {
	if req == nil {
		return
	}
	for name, value := range Headers(path) {
		req.Header.Set(name, value)
	}
}
