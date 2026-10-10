package consolecred

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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

// Headers builds the instance authentication headers for one request: the instance id, a
// fresh timestamp, a fresh nonce and the HMAC over
//
//	nonce|timestamp|METHOD|path|hex(sha256(body))
//
// or nil when no credential is loaded (or the body cannot be read back). It is the raw
// accessor: it does NOT consult the stale flag. Use SignRequest for an official-channel
// request and SignSyncRequest for the sync/inventory client.
func Headers(req *http.Request) map[string]string {
	if req == nil {
		return nil
	}
	return headersForPath(req, req.URL.Path)
}

func signPath(req *http.Request, path string) {
	for name, value := range headersForPath(req, path) {
		req.Header.Set(name, value)
	}
}

func headersForPath(req *http.Request, path string) map[string]string {
	cred, ok := global.Get()
	if !ok {
		return nil
	}
	digest, ok := requestBodyDigest(req)
	if !ok {
		// The body could not be read back, so signing would commit to a digest that does
		// not cover what is actually sent. Attach nothing instead: console answers the
		// unsigned call with 401, which is loud, rather than opening a replay window.
		return nil
	}
	nonce, err := NewNonce()
	if err != nil {
		return nil
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	method := strings.ToUpper(req.Method)
	return map[string]string{
		HeaderInstanceID: cred.InstanceID,
		HeaderTimestamp:  timestamp,
		HeaderNonce:      nonce,
		HeaderSignature:  Sign(cred.Secret, nonce, timestamp, method, signedPath(path), digest),
	}
}

// signedPath drops the query string. Console verifies against r.URL.Path, which never
// carries a query, so a signed path including one would never match. The query is still
// sent; it just is not covered by the signature.
func signedPath(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

// requestBodyDigest returns hex(sha256(body)) for the exact bytes the request will send.
//
// The digest is read from req.GetBody when net/http installed it (it does for the
// bytes.Buffer/Reader and strings.Reader bodies every console caller builds), so the live
// body stream is left untouched and retries/redirects still replay from the original body.
// A request built from a bare reader is buffered once and its body restored, because the
// digest has to cover what is actually sent. A body that cannot be read back reports
// ok=false and the caller then signs nothing (fail closed).
func requestBodyDigest(req *http.Request) (string, bool) {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return emptyBodyDigest, true
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return "", false
		}
		defer body.Close()
		raw, err := io.ReadAll(body)
		if err != nil {
			return "", false
		}
		return BodyDigest(raw), true
	}

	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return "", false
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(raw)), nil
	}
	req.ContentLength = int64(len(raw))
	return BodyDigest(raw), true
}
