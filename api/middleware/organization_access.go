package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/memberstatus"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

const applicationErrorProjectorKey = "application_error_projector"

// SetApplicationErrorProjector makes the process-wide error projector
// available to authentication middleware and handlers that have no projector
// of their own, so cataloged errors keep their localized public messages.
func SetApplicationErrorProjector(projector *apptransport.Projector) gin.HandlerFunc {
	return func(c *gin.Context) {
		if projector != nil {
			c.Set(applicationErrorProjectorKey, projector)
		}
		c.Next()
	}
}

func contextAccountService(c *gin.Context) interfaces.AccountService {
	raw, ok := c.Get(accountServiceKey)
	if !ok {
		return nil
	}
	service, _ := raw.(interfaces.AccountService)
	return service
}

// enforceOrganizationAccess rejects the request when the account's membership
// in the resolved organization is disabled, or when its current workspace is
// outside that organization. It aborts the request and returns false on
// rejection. A missing account service fails closed.
func enforceOrganizationAccess(c *gin.Context, accountService interfaces.AccountService, accountID, organizationID string) bool {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return true
	}
	if accountService == nil {
		logger.Error("auth: account service unavailable for organization access check", nil)
		response.Fail(c, response.ErrWorkspaceNotFound)
		c.Abort()
		return false
	}
	err := accountService.CheckOrganizationAccess(c.Request.Context(), accountID, organizationID)
	switch {
	case err == nil:
		return true
	case apperror.IsCode(err, memberstatus.AppCodeMemberDisabled):
		logger.Warn("auth: organization membership is disabled; account_id=%s organization_id=%s", accountID, organizationID)
		FailOrganizationMemberDisabled(c, err)
	case errors.Is(err, interfaces.ErrCurrentWorkspaceOutsideOrganization):
		logger.Warn("auth: current workspace is outside the resolved organization; account_id=%s organization_id=%s", accountID, organizationID)
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		c.Abort()
	default:
		logger.Error("auth: organization access check failed", err)
		response.Fail(c, response.ErrSystemError)
		c.Abort()
	}
	return false
}

// FailOrganizationMemberDisabled writes the cataloged
// organization.member.disabled response (HTTP 403) and aborts the request.
// It localizes the message through the request's error projector when one is
// installed and falls back to the legacy response message otherwise.
func FailOrganizationMemberDisabled(c *gin.Context, err error) {
	if !apperror.IsCode(err, memberstatus.AppCodeMemberDisabled) {
		err = memberstatus.NewMemberDisabledError("middleware.organization_member_disabled")
	}
	c.Header(apptransport.HeaderApplicationErrorCode, memberstatus.AppCodeMemberDisabled.String())
	if raw, ok := c.Get(applicationErrorProjectorKey); ok {
		if projector, ok := raw.(*apptransport.Projector); ok && projector != nil {
			message := projector.ProjectLegacyMessage(
				err,
				apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language")),
				memberstatus.LegacyMemberDisabled,
			)
			if message.Resolution == apptransport.ResolutionMatched {
				response.FailWithMessage(c, response.ErrOrganizationMemberDisabled, message.Message)
				c.Abort()
				return
			}
		}
	}
	response.Fail(c, response.ErrOrganizationMemberDisabled)
	c.Abort()
}
