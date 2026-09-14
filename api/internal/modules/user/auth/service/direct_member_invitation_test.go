package service

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/email"
)

// Exercise the actual mail-to-activation boundary, rather than constructing an
// already-correct invitation in Redis and missing lost destination metadata.
func TestDirectMemberEmailPreservesInvitationDestination(t *testing.T) {
	t.Chdir("../../../../..") // Mail templates are loaded relative to the API root.
	for _, workspaceID := range []string{"workspace-invited", ""} {
		t.Run("workspace="+workspaceID, func(t *testing.T) {
			tokenMgr := newInvitationTestTokenManager(t)
			messages := make(chan email.EmailRequest, 1)
			mailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var message email.EmailRequest
				if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
					http.Error(w, "invalid mail request", http.StatusBadRequest)
					return
				}
				messages <- message
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"test-mail"}`))
			}))
			t.Cleanup(mailServer.Close)
			previousConfig := email.Cfg
			email.Init(&config.Config{Email: config.EmailConfig{
				MailType: "resend", ResendAPIKey: "test-only", ResendAPIURL: mailServer.URL,
				MailDefaultSendFrom: "ZGI <test@example.com>", ConsoleWebURL: "https://console.example.com",
			}})
			t.Cleanup(func() { email.Init(previousConfig) })
			account := auth_model.Account{ID: "invitee", Email: "invitee+test@example.com", Name: "Member", Status: auth_model.AccountStatusPending}
			organization := &workspace_model.Organization{ID: "org-invited", Name: "Test Org", Status: workspace_model.OrganizationStatusActive}
			workspace := &workspace_model.Workspace{ID: workspaceID, Name: "Assigned Workspace", OrganizationID: &organization.ID, Status: workspace_model.WorkspaceStatusNormal}
			service := &AccountService{
				tokenMgr: tokenMgr, accountRepo: &invitationAcceptanceAccountRepository{account: account},
				organizationService: directMemberInvitationOrganizationService{organization: organization},
				workspaceManagementService: &invitationWorkspaceService{
					workspace: workspace,
					member:    &workspace_model.WorkspaceMember{AccountID: account.ID, WorkspaceID: workspaceID, Role: workspace_model.WorkspaceRoleNormal},
				},
			}
			require.NoError(t, service.SendDirectAddMemberEmail(t.Context(), &account, "inviter", organization.ID, workspaceID, organization.Name, "", "en-US"))
			message := <-messages
			require.Equal(t, []string{account.Email}, message.To)
			var activationURL *url.URL
			for _, match := range regexp.MustCompile(`href="([^"]+)"`).FindAllStringSubmatch(message.Html, -1) {
				candidate, err := url.Parse(html.UnescapeString(match[1]))
				require.NoError(t, err)
				if candidate.Path == "/activate" {
					activationURL = candidate
					break
				}
			}
			require.NotNil(t, activationURL)
			require.Equal(t, "console.example.com", activationURL.Host)
			require.Equal(t, account.Email, activationURL.Query().Get("email"))
			token := activationURL.Query().Get("token")
			require.NotEmpty(t, token)
			stored, err := tokenMgr.GetInvitationByToken(token, "", account.Email)
			require.NoError(t, err)
			require.NotNil(t, stored)
			require.Equal(t, workspaceID, stored.WorkspaceID)
			require.Equal(t, organization.ID, stored.OrganizationID)
			require.Equal(t, "inviter", stored.InviterID)
			require.Equal(t, string(workspace_model.WorkspaceRoleNormal), stored.Role)

			// The existing URL need not contain workspace_id: the server resolves
			// the stored invitation's destination, not an arbitrary client hint.
			result, valid := service.ActivateCheck(t.Context(), "", account.Email, token)
			require.True(t, valid)
			data := result["data"].(map[string]interface{})
			require.Equal(t, workspaceID, data["workspace_id"])
			require.Equal(t, organization.ID, data["organization_id"])
			if workspaceID != "" {
				require.Equal(t, workspace.Name, data["workspace_name"])
				_, valid = service.ActivateCheck(t.Context(), "other-workspace", account.Email, token)
				require.False(t, valid, "a different workspace must not use this invitation")
				workspace.Status = workspace_model.WorkspaceStatusArchived
				_, valid = service.ActivateCheck(t.Context(), "", account.Email, token)
				require.False(t, valid, "an unavailable assigned workspace must fail closed")
			} else {
				require.Empty(t, data["workspace_name"], "organization-only invites must stay organization-only")
			}
			require.False(t, strings.Contains(message.Html, "test-only"), "provider credentials must not appear in email")
		})
	}
}

type directMemberInvitationOrganizationService struct {
	interfaces.OrganizationService
	organization *workspace_model.Organization
}

func (s directMemberInvitationOrganizationService) GetOrganizationByID(context.Context, string) (*workspace_model.Organization, error) {
	return s.organization, nil
}
