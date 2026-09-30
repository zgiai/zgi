package migrations

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type preferencePostgresCatalog struct{}

func (preferencePostgresCatalog) Get(id string) (connectorcatalog.Definition, bool) {
	definitions, err := connectorcatalog.BuiltinDefinitions()
	if err != nil || id != "github" {
		return connectorcatalog.Definition{}, false
	}
	definition := definitions[0]
	definition.Operations = definition.Operations[:1]
	definition.Operations[0].ID = "github.list_repositories"
	return definition, true
}

// This opt-in suite uses only an explicitly configured isolated test database
// and a random schema. It never falls back to application database settings.
func TestConnectorChatPreferencesPostgresConcurrencyAndSchema(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_PREFERENCES_TEST_DSN")
	if dsn == "" {
		t.Skip("ZGI_CONNECTOR_PREFERENCES_TEST_DSN is required for isolated PostgreSQL lock/concurrency validation")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid isolated PostgreSQL test configuration")
	}
	admin := stdlib.OpenDB(*config)
	defer admin.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	schema := "connector_preferences_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("cannot create isolated preference test schema")
	}
	defer func() { _, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*config)
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("cannot open isolated preference test database")
	}
	if _, err := sqlDB.ExecContext(ctx, strings.ReplaceAll(createConnectorChatPreferencesSQL, "public.", schema+".")); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.ExecContext(ctx, `CREATE TABLE connector_connections(id uuid PRIMARY KEY,organization_id uuid,owner_account_id uuid,connector_id text,name text,external_display_name text,state text,driver_id text,auth_method_id text,usable_operation_ids jsonb,deleted_at timestamptz,unavailable_operations jsonb,connection_constraint jsonb)`); err != nil {
		t.Fatal(err)
	}
	org, owner, connectionID := uuid.New(), uuid.New(), uuid.New()
	constraint, _ := json.Marshal(connectorcore.OwnerConstraint(owner, []connectorcore.Identifier{"github.list_repositories"}))
	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO connector_connections VALUES($1,$2,$3,'github','Test account','fixture','ready','github-rest','personal_access_token','["github.list_repositories"]',NULL,'{}',$4)`, connectionID, org, owner, string(constraint)); err != nil {
		t.Fatal(err)
	}
	service, err := preferences.NewService(db, preferencePostgresCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	command := preferences.UpdateCommand{OrganizationID: org, OwnerAccountID: owner, ConnectorID: "github", Selection: preferences.Selection{Mode: preferences.ModeNone}, IdempotencyKey: "postgres-initial-request-0001"}
	results := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func(index int) {
			<-start
			request := command
			request.IdempotencyKey = fmt.Sprintf("postgres-initial-request-%04d", index)
			_, err := service.Update(ctx, request)
			results <- err
		}(i)
	}
	close(start)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			successes++
		} else if apperror.IsCode(err, preferences.AppCodeRevisionConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent initial insert: success=%d conflict=%d", successes, conflicts)
	}

	// A revision check holds the absent/present owner authority for the complete
	// first-chat transaction; a concurrent preference mutation must wait.
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := preferences.ValidateChatRevision(ctx, tx, org, owner, 1); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	request := command
	request.ExpectedRevision = 1
	request.IdempotencyKey = "postgres-blocked-update-0001"
	go func() { _, err := service.Update(ctx, request); results <- err }()
	select {
	case err := <-results:
		tx.Rollback()
		t.Fatalf("mutation passed locked snapshot: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-results; err != nil {
		t.Fatal(err)
	}

	request.ExpectedRevision = 2
	request.IdempotencyKey = "postgres-select-default-0001"
	request.Selection = preferences.Selection{Mode: preferences.ModeSelected, ConnectionID: connectionID.String()}
	if _, err := service.Update(ctx, request); err != nil {
		t.Fatal(err)
	}
	request.ExpectedRevision = 3
	request.IdempotencyKey = "postgres-race-delete-0001"
	start = make(chan struct{})
	go func() { <-start; _, err := service.Update(ctx, request); results <- err }()
	go func() {
		<-start
		results <- db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := preferences.LockOwner(ctx, tx, org, owner); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE connector_connections SET deleted_at=now() WHERE organization_id=? AND owner_account_id=? AND id=?`, org, owner, connectionID).Error; err != nil {
				return err
			}
			return preferences.ClearRemovedConnection(ctx, tx, org, owner, connectionID)
		})
	}()
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil && !apperror.IsCode(err, preferences.AppCodeRevisionConflict) {
			t.Fatal(err)
		}
	}
	snapshot, err := service.Get(ctx, org, owner)
	if err != nil || snapshot.Defaults["github"].Mode != preferences.ModeNone {
		t.Fatalf("deleted default survived race: %#v %v", snapshot, err)
	}
	for _, invalid := range []string{`{"github":{}}`, `{"github":{"mode":"none","connection_id":"x"}}`, `{"github":{"mode":"selected"}}`, `[]`, `{"github":{"mode":"selected","connection_id":"00000000-0000-0000-0000-000000000000"}}`} {
		if err := db.Exec(`UPDATE connector_chat_preferences SET defaults=?::jsonb WHERE organization_id=? AND owner_account_id=?`, invalid, org, owner).Error; err == nil {
			t.Fatalf("schema admitted invalid defaults: %s", invalid)
		}
	}
}
