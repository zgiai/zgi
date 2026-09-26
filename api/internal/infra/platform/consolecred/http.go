package consolecred

import "net/http"

// SignRequest adds the per-instance credential headers to an outgoing OFFICIAL-CHANNEL
// request.
//
// It is deliberately additive: the legacy shared-secret headers written by the caller stay
// in place, and when no instance credential is loaded this is a no-op. Console prefers the
// instance credential whenever X-Internal-Instance-Id is present.
//
// While the console link is stale the instance headers are NOT attached: the official
// channel fails closed instead of trusting a link the sync loop can no longer verify. The
// stale state is expressed by the credential store's flag, not by clearing the credential,
// so the sync and inventory clients keep working. Those clients must sign through
// SignSyncRequest, which ignores the stale flag on purpose.
func SignRequest(req *http.Request) {
	if req == nil || global.Stale() {
		return
	}
	signPath(req, req.URL.Path)
}

// SignSyncRequest signs a sync/inventory request and ALWAYS attaches the instance headers,
// even while the console link is stale.
//
// This is the one surface that must never be gated on the stale flag: it is what carries
// the heartbeat that clears the stale state. Gating it (or clearing the credential in the
// stale gate) locks the instance out permanently, because unsigned sync calls are answered
// with 401 and last_heartbeat_at never advances again.
func SignSyncRequest(req *http.Request) {
	if req == nil {
		return
	}
	signPath(req, req.URL.Path)
}

// SignWithPath adds the instance headers for an explicit signed path on an official-channel
// request. Use it when the signed path differs from req.URL.Path (for example a route
// alias). Like SignRequest, it is a no-op while the console link is stale.
func SignWithPath(req *http.Request, path string) {
	if req == nil || global.Stale() {
		return
	}
	signPath(req, path)
}

func signPath(req *http.Request, path string) {
	for name, value := range Headers(path) {
		req.Header.Set(name, value)
	}
}
