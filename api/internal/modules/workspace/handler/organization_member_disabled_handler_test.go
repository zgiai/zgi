package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/workspace/memberstatus"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestAcceptInviteLinkRefusesDisabledMemberWithCatalogedError(t *testing.T) {
	t.Parallel()

	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), memberstatus.CatalogDefinitions()...)...)
	require.NoError(t, err)
	projector, err := apptransport.NewProjector(productCatalog)
	require.NoError(t, err)

	handler := &OrganizationHandler{organizationService: fakeOrganizationService{
		getInviteLinkByTokenFn: func(context.Context, string) (*model.OrganizationInviteLink, error) {
			return &model.OrganizationInviteLink{OrganizationID: "org-1", Status: "active"}, nil
		},
		acceptInviteByTokenFn: func(context.Context, string, string, *string) (*model.OrganizationJoinRequest, error) {
			return nil, apperror.Wrap(workspace_service.ErrOrganizationInviteMemberDisabled,
				memberstatus.AppCodeMemberDisabled, apperror.WithOperation("organization.invite.accept"))
		},
	}}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/public/invites/invite-token/accept")
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"name":"Invitee"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept-Language", "zh-Hans")
	c.Params = gin.Params{{Key: "token", Value: "invite-token"}}
	c.Set("account_id", "account-1")
	middleware.SetApplicationErrorProjector(projector)(c)

	handler.AcceptInviteLink(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, memberstatus.AppCodeMemberDisabled.String(), recorder.Header().Get(apptransport.HeaderApplicationErrorCode))
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, strconv.Itoa(response.ErrOrganizationMemberDisabled.Code), body.Code)
	require.Equal(t, "你在该组织的访问权限已被停用，请联系组织管理员。", body.Message)
	require.NotContains(t, body.Message, "invited account")
}

func TestApproveDepartmentJoinRequestRefusesDisabledRequester(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{organizationService: fakeOrganizationService{
		isOrganizationAdminOrOwnerFn: func(context.Context, string, string) (bool, error) {
			return true, nil
		},
		approveDepartmentJoinRequestFn: func(context.Context, string, string, string) (*model.OrganizationJoinRequest, error) {
			return nil, apperror.Wrap(workspace_service.ErrOrganizationInviteMemberDisabled,
				memberstatus.AppCodeMemberDisabled, apperror.WithOperation("organization.invite.apply"))
		},
	}}
	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/join-requests/request-1/approve")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}, {Key: "id", Value: "request-1"}}
	c.Set("account_id", "admin-1")

	handler.ApproveDepartmentJoinRequest(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}
