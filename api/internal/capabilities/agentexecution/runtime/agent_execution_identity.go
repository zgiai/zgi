package executionruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
)

// Negotiate before any identity-scoped operation, including reads and file
// transfer. An old Worker ignores unknown headers and would otherwise treat an
// external request as a legacy account request. Do not cache across restarts.
func (g *agentExecutionHTTPGateway) verifyExecutionIdentity(ctx context.Context, scope agentExecutionScope) error {
	if scope.Identity == nil {
		return nil
	}
	if err := scope.Identity.Validate(); err != nil {
		return err
	}
	var release struct {
		IdentityVersion           int `json:"execution_identity_version"`
		BackgroundIdentityVersion int `json:"background_execution_identity_version"`
	}
	legacyScope := agentExecutionScope{OrganizationID: scope.OrganizationID, SubjectID: scope.SubjectID}
	if err := g.request(ctx, legacyScope, http.MethodGet, "release", nil, &release); err != nil {
		return fmt.Errorf("negotiate execution identity: %w", err)
	}
	if release.IdentityVersion != execution.IdentityVersion {
		return fmt.Errorf("Worker does not support the required execution identity contract")
	}
	if scope.Identity.Version == execution.BackgroundIdentityVersion && release.BackgroundIdentityVersion != execution.BackgroundIdentityVersion {
		return fmt.Errorf("worker does not support background execution identities")
	}
	return nil
}

func setExecutionIdentityHeader(request *http.Request, scope agentExecutionScope) error {
	if scope.Identity == nil {
		return nil
	}
	if err := scope.Identity.Validate(); err != nil {
		return err
	}
	value, err := json.Marshal(scope.Identity)
	if err != nil {
		return fmt.Errorf("encode execution identity: %w", err)
	}
	request.Header.Set(execution.IdentityHeader, string(value))
	return nil
}
