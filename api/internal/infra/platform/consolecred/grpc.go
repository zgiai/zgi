package consolecred

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/credentials"
)

// GRPCCredentials carries the dual credential on every billing RPC: the
// branch-compatible "authorization: Bearer <secret>" plus the instance HMAC headers
// signed over the full gRPC method name.
//
// The signature covers the method so a captured header set cannot be replayed against a
// different RPC. When no credential is loaded the metadata is empty and the call behaves
// exactly as before the control plane existed.
type GRPCCredentials struct{}

// NewGRPCCredentials returns the per-RPC credentials to attach at every dial site.
func NewGRPCCredentials() credentials.PerRPCCredentials { return GRPCCredentials{} }

// GetRequestMetadata implements credentials.PerRPCCredentials.
//
// While the console link is stale it returns ErrConsoleLinkStale and attaches no metadata:
// every gRPC official-channel dial site (billing, gateway quota, the official credit
// checker) therefore fails closed. The loaded credential is left untouched so the
// sync/inventory HTTP client, which is not attached to this credential, keeps signing.
func (GRPCCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	// The full method is attached by the gRPC transport for exactly this purpose.
	method := ""
	if info, found := credentials.RequestInfoFromContext(ctx); found {
		method = info.Method
	}

	if global.Stale() {
		if method == "" {
			return nil, ErrConsoleLinkStale
		}
		return nil, fmt.Errorf("%w: %s", ErrConsoleLinkStale, method)
	}

	cred, ok := global.Get()
	if !ok {
		return map[string]string{}, nil
	}

	metadata := map[string]string{
		strings.ToLower(HeaderAuthorization): "Bearer " + cred.Secret,
	}

	if method == "" {
		// Without a method there is nothing to sign; the Bearer token still travels and
		// the branch interceptor can still resolve the instance.
		return metadata, nil
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	metadata[strings.ToLower(HeaderInstanceID)] = cred.InstanceID
	metadata[strings.ToLower(HeaderTimestamp)] = timestamp
	metadata[strings.ToLower(HeaderSignature)] = signGRPCMethod(cred.Secret, timestamp, method)
	return metadata, nil
}

// signGRPCMethod signs the unchanged gRPC string timestamp + "|" + method.
//
// The hardened HTTP protocol (nonce + method + path + body digest) cannot be applied here
// cheaply: GetRequestMetadata never sees the marshaled request message, gRPC has no HTTP
// method, and the console gRPC interceptor still verifies this legacy shape. gRPC is
// therefore deliberately left as-is; only HTTP requests carry the nonce and body digest.
// Do not route HTTP callers through this helper.
func signGRPCMethod(secret, timestamp, method string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "|" + method))
	return hex.EncodeToString(mac.Sum(nil))
}

// RequireTransportSecurity is false: console is reached over plaintext gRPC today,
// exactly like the branch consoleauth implementation.
func (GRPCCredentials) RequireTransportSecurity() bool { return false }
