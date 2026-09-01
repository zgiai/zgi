package handler

import (
	"github.com/gin-gonic/gin"
	authservice "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

var (
	legacyRegistrationInvitationUnavailable = appcatalog.MustLegacyKey("auth.registration:401002")
	legacyRegistrationMemberNameConflict    = appcatalog.MustLegacyKey("auth.registration:199001")
)

// projectRegistrationApplicationError projects only cataloged application
// errors. Legacy sentinels keep their existing response contract, and unknown
// causes remain on the generic handler path instead of leaking diagnostic text.
func projectRegistrationApplicationError(c *gin.Context, projector *apptransport.Projector, err error) bool {
	if projector == nil {
		return false
	}
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	switch {
	case apperror.IsCode(err, authservice.AppCodeRegistrationInvitationUnavailable):
		message := projector.ProjectLegacyMessage(
			err,
			locale,
			legacyRegistrationInvitationUnavailable,
		)
		if message.Resolution != apptransport.ResolutionMatched {
			return false
		}
		response.FailWithMessage(c, response.ErrTokenInvalid, message.Message)
		return true
	case apperror.IsCode(err, authservice.AppCodeRegistrationMemberNameConflict):
		message := projector.ProjectLegacyMessage(
			err,
			locale,
			legacyRegistrationMemberNameConflict,
		)
		if message.Resolution != apptransport.ResolutionMatched {
			return false
		}
		response.FailWithMessage(c, response.ErrInvalidParam, message.Message)
		return true
	default:
		return false
	}
}
