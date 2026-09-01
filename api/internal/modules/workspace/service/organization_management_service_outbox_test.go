package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type organizationManagementConsoleRecorder struct {
	platformconsole.ConsoleProvider
	registerCalls int
}

func (r *organizationManagementConsoleRecorder) IsAvailable() bool { return true }
func (r *organizationManagementConsoleRecorder) RegisterOrganization(context.Context, *platformconsole.RegisterOrganizationRequest) error {
	r.registerCalls++
	return nil
}

func TestOrganizationManagementCreateOrganizationHasNoRemoteSideEffect(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}))
	console := &organizationManagementConsoleRecorder{}
	service := NewOrganizationManagementService(db, console)

	organization, err := service.CreateOrganization(t.Context(), "Transaction Owned Organization")

	require.NoError(t, err)
	require.NotNil(t, organization)
	require.Zero(t, console.registerCalls, "low-level transaction services must not perform remote calls")
}
