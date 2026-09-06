package developeraccess

import (
	"context"
	"testing"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

func TestPrincipalQuotaClassificationAfterAuthorization(t *testing.T) {
	for _, scenario := range []string{"exhausted", "unlimited", "removed_member", "disabled_policy", "revoked_key", "archived_workspace"} {
		t.Run(scenario, func(t *testing.T) {
			db := openDeveloperAccessTestDB(t)
			workspaceID, _, ownerID, _ := seedDeveloperWorkspace(t, db)
			repo := apikeyrepo.NewAPIKeyRepository(db)
			service := NewService(db, repo, nil)
			created, err := service.CreateKey(context.Background(), workspaceID, ownerID, CreateKeyInput{Name: "quota classification"})
			if err != nil {
				t.Fatal(err)
			}
			var key apikeymodel.TenantAPIKey
			if err := db.First(&key, "id = ?", created.ID).Error; err != nil {
				t.Fatal(err)
			}
			updates := map[string]any{"quota_limit": int64(100), "used_quota": int64(100), "remain_quota": int64(0)}
			if scenario == "unlimited" {
				updates["quota_limit"] = nil
			}
			if err := db.Model(&accessmodel.Grant{}).Where("id = ?", *key.AccessGrantID).Updates(updates).Error; err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "removed_member":
				err = db.Where("workspace_id = ? AND account_id = ?", workspaceID, ownerID).Delete(&workspacemodel.WorkspaceMember{}).Error
			case "disabled_policy":
				_, err = service.PutPolicy(context.Background(), workspaceID, ownerID, PolicyInput{Mode: accessmodel.AccessModeDisabled, MaxKeys: 3})
			case "revoked_key":
				err = db.Model(&apikeymodel.TenantAPIKey{}).Where("id = ?", key.ID).Update("status", "revoked").Error
			case "archived_workspace":
				err = db.Model(&workspacemodel.Workspace{}).Where("id = ?", workspaceID).Update("status", workspacemodel.WorkspaceStatusArchived).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			err = repo.ValidatePrincipalAccess(context.Background(), &key)
			if scenario == "unlimited" {
				if err != nil {
					t.Fatalf("unlimited allowance rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected rejection")
			}
			if got := apperror.IsCode(err, llmerrors.AppCodeDeveloperQuotaExhausted); got != (scenario == "exhausted") {
				t.Fatalf("quota classification=%v: %v", got, err)
			}
		})
	}
}
