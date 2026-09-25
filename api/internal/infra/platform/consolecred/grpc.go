package consolecred

import (
	"context"
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
func (GRPCCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	cred, ok := global.Get()
	if !ok {
		return map[string]string{}, nil
	}

	metadata := map[string]string{
		strings.ToLower(HeaderAuthorization): "Bearer " + cred.Secret,
	}

	// The full method is attached by the gRPC transport for exactly this purpose.
	method := ""
	if info, found := credentials.RequestInfoFromContext(ctx); found {
		method = info.Method
	}
	if method == "" {
		// Without a method there is nothing to sign; the Bearer token still travels and
		// the branch interceptor can still resolve the instance.
		return metadata, nil
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	metadata[strings.ToLower(HeaderInstanceID)] = cred.InstanceID
	metadata[strings.ToLower(HeaderTimestamp)] = timestamp
	metadata[strings.ToLower(HeaderSignature)] = Sign(cred.Secret, timestamp, method)
	return metadata, nil
}

// RequireTransportSecurity is false: console is reached over plaintext gRPC today,
// exactly like the branch consoleauth implementation.
func (GRPCCredentials) RequireTransportSecurity() bool { return false }
