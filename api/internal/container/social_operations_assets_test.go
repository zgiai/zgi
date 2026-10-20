package container

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	core "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"image"
	"image/png"
	"testing"
)

func TestSocialOperationsAssetsAndAccountOptionsRequireExactPublishedOwnerScope(t *testing.T) {
	fixture := newSocialIntegrationFixture(t)
	scope := digitalhuman.Scope{OrganizationID: fixture.scope.OrganizationID, WorkspaceID: fixture.scope.WorkspaceID, ActorID: fixture.scope.OwnerID}
	employee := fixture.scope.EmployeeID
	version := fixture.gateway.restored.Run.DigitalHumanVersionID
	detail := &digitalhuman.Detail{Definition: &digitalhuman.Definition{ID: employee, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, Status: digitalhuman.StatusActive, PublishedVersionID: &version}, Published: &digitalhuman.Version{ID: version, DigitalHumanID: employee, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, State: digitalhuman.VersionStatePublished}}
	account := "acct-7"
	connection := core.PersonalConnection{ID: fixture.connection, OrganizationID: scope.OrganizationID, OwnerAccountID: scope.ActorID, ConnectorID: "x-social", ExternalAccountID: &account, State: core.ReadinessReady}
	binding := core.TargetBinding{ID: uuid.New(), OrganizationID: scope.OrganizationID, OwnerAccountID: scope.ActorID, ConnectionID: connection.ID, ConnectorID: connection.ConnectorID, Target: core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: employee.String(), Scope: core.ScopePublished, VersionID: version.String()}, Status: core.BindingStatusActive}
	foreign := binding
	foreign.ID = uuid.New()
	foreign.OwnerAccountID = uuid.New()
	foreign.ConnectionID = uuid.New()
	service := &SocialOperationsSetupService{provisioning: new(socialops.ProvisioningService), employees: backgroundEmployeeReaderStub{detail: detail}, journal: fixture.service, restorer: &SocialOperationsPreflightRestorer{bindings: socialOperationsBindingReaderStub{items: []core.TargetBinding{binding, foreign}}, connections: socialOperationsConnectionReaderStub{items: map[uuid.UUID]core.PersonalConnection{connection.ID: connection}}}}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	artifact, err := service.UploadBrandImage(context.Background(), scope, employee, data.Bytes())
	if err != nil || artifact.OwnerID != scope.ActorID || artifact.EmployeeID != employee {
		t.Fatal(artifact, err)
	}
	items, err := service.ListAccounts(context.Background(), scope, employee)
	if err != nil || len(items) != 1 || len(items[0].Accounts) != 1 || items[0].Accounts[0].ID != account {
		t.Fatal(items, err)
	}
	wrong := scope
	wrong.OrganizationID = uuid.New()
	if _, err = service.UploadBrandImage(context.Background(), wrong, employee, data.Bytes()); err == nil {
		t.Fatal("foreign scope uploaded")
	}
	if _, err = service.ListAccounts(context.Background(), wrong, employee); err == nil {
		t.Fatal("foreign scope listed accounts")
	}
	if _, err = service.UploadBrandImage(context.Background(), scope, employee, []byte("not an image")); err == nil {
		t.Fatal("invalid image saved")
	}
}
