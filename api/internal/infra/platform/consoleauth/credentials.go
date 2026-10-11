package consoleauth

import "context"

// Credentials attaches the Console-issued downstream API key to every RPC as
// "authorization: Bearer <token>". Console validates it server-side and resolves
// the downstream client, so the key is never trusted as caller-supplied identity
// by itself. It is shared by every zgi-pre BillingService client so all billing
// calls carry the same credential.
type Credentials struct {
	Token string
}

// GetRequestMetadata implements credentials.PerRPCCredentials.
func (c Credentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + c.Token}, nil
}

// RequireTransportSecurity is false: Console runs on the internal network over
// plaintext gRPC.
func (c Credentials) RequireTransportSecurity() bool { return false }
