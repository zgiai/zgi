package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
)

func TestArtifactOwnersAreCountedNamedAndFiltered(t *testing.T) {
	_, repo := newTestService(t)
	db := repo.db
	if err := db.AutoMigrate(&artifacts.Artifact{}, &artifacts.Version{}, &artifacts.SourceFile{}, &artifacts.Share{}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE accounts (id text PRIMARY KEY, name text, email text)`,
		`CREATE TABLE runtime_workspace_end_users (subject_id text PRIMARY KEY, external_user_id text)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := artifacts.NewService(db, ToolFileArtifactBlobs{})
	ctx := t.Context()
	scope := artifacts.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	ada, grace := artifacts.Owner{Kind: artifacts.OwnerAccount, ID: uuid.New()}, artifacts.Owner{Kind: artifacts.OwnerAccount, ID: uuid.New()}
	customer := artifacts.Owner{Kind: artifacts.OwnerAPIEndUser, ID: uuid.New()}
	team := artifacts.Owner{Kind: artifacts.OwnerWorkspace, ID: scope.WorkspaceID}
	if err := db.Exec(`INSERT INTO accounts (id, name, email) VALUES (?, 'Ada', 'ada@example.com'), (?, '', 'grace@example.com')`, ada.ID, grace.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO runtime_workspace_end_users (subject_id, external_user_id) VALUES (?, 'crm-42')`, customer.ID).Error; err != nil {
		t.Fatal(err)
	}
	publish := func(owner artifacts.Owner, visibility artifacts.Visibility, title string) {
		t.Helper()
		if _, err := service.Publish(ctx, artifacts.PublishRequest{Scope: scope, Owner: owner, Visibility: visibility,
			Kind: artifacts.KindDocument, Title: title, Text: title}); err != nil {
			t.Fatal(err)
		}
	}
	publish(ada, artifacts.VisibilityTeam, "ada 1")
	publish(ada, artifacts.VisibilityTeam, "ada 2")
	publish(grace, artifacts.VisibilityOwner, "grace private")
	publish(customer, artifacts.VisibilityOwner, "customer")
	publish(team, artifacts.VisibilityTeam, "team")

	// A task manager sees every owner; a member does not see others' private ones.
	manager := artifacts.Reader{Scope: scope, Principal: artifacts.Owner{Kind: artifacts.OwnerAccount, ID: uuid.New()}, Member: true, Operator: true}
	member := artifacts.Reader{Scope: scope, Principal: artifacts.Owner{Kind: artifacts.OwnerAccount, ID: uuid.New()}, Member: true}
	names := NewArtifactOwnerNames(db)
	page := func(reader artifacts.Reader, kind artifacts.OwnerKind, search string) map[string]int64 {
		t.Helper()
		owners, err := service.Owners(ctx, reader, kind, artifacts.ArchiveActive)
		if err != nil {
			t.Fatal(err)
		}
		result, err := names.Page(ctx, owners, search, 1, 20)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]int64{}
		for _, item := range result.Items {
			found[string(item.Kind)+":"+item.Name] = item.Count
		}
		if result.Total != int64(len(found)) {
			t.Fatalf("total %d for %v", result.Total, found)
		}
		return found
	}
	if got := page(manager, "", ""); len(got) != 4 || got["account:Ada"] != 2 || got["account:grace@example.com"] != 1 ||
		got["api_end_user:crm-42"] != 1 || got["workspace:"] != 1 {
		t.Fatalf("manager owners = %v", got)
	}
	if got := page(member, "", ""); len(got) != 2 || got["account:Ada"] != 2 || got["workspace:"] != 1 {
		t.Fatalf("member owners = %v", got)
	}
	if got := page(manager, artifacts.OwnerAPIEndUser, ""); len(got) != 1 || got["api_end_user:crm-42"] != 1 {
		t.Fatalf("end user owners = %v", got)
	}
	if got := page(manager, "", "GRACE@"); len(got) != 1 || got["account:grace@example.com"] != 1 {
		t.Fatalf("searched owners = %v", got)
	}
	if got := page(manager, "", "crm"); len(got) != 1 {
		t.Fatalf("end user search = %v", got)
	}

	list := func(query artifacts.ListQuery) int64 {
		t.Helper()
		result, err := service.List(ctx, manager, query)
		if err != nil {
			t.Fatal(err)
		}
		return result.Total
	}
	if list(artifacts.ListQuery{OwnerKind: artifacts.OwnerAccount}) != 3 || list(artifacts.ListQuery{OwnerID: &ada.ID}) != 2 ||
		list(artifacts.ListQuery{OwnerKind: artifacts.OwnerWorkspace}) != 1 {
		t.Fatal("the owner filters kept the wrong artifacts")
	}
	if _, err := service.List(ctx, manager, artifacts.ListQuery{OwnerKind: "robot"}); err == nil {
		t.Fatal("an unknown owner kind was accepted")
	}
}
