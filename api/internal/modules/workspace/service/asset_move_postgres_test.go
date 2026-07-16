package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWorkspaceAssetMoveAndDatasetDeleteUseSamePostgresLockOrder(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL asset move concurrency test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	schema := "asset_move_lock_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schema + `"`
	require.NoError(t, db.Exec("CREATE SCHEMA "+quotedSchema).Error)
	t.Cleanup(func() { _ = db.Exec("DROP SCHEMA IF EXISTS " + quotedSchema + " CASCADE").Error })
	require.NoError(t, db.Exec("CREATE TABLE "+quotedSchema+".datasets (id text PRIMARY KEY)").Error)

	organizationID := uuid.New()
	datasetID := uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO "+quotedSchema+".datasets (id) VALUES (?)", datasetID).Error)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bindingRepo := agentbindings.NewRepository(db)
	svc := NewWorkspaceAssetMoveService(db, nil, bindingRepo)
	ref := agentbindings.ResourceRef{
		OrganizationID: organizationID,
		BindingType:    agentbindings.BindingTypeKnowledgeDataset,
		ResourceID:     datasetID,
	}

	moveTx := db.WithContext(ctx).Begin()
	require.NoError(t, moveTx.Error)
	require.NoError(t, moveTx.Exec("SET LOCAL search_path TO "+quotedSchema).Error)
	if err := svc.lockMoveAssets(ctx, moveTx, organizationID.String(), []dto.WorkspaceAssetMoveItem{{
		Type: AssetMoveTypeDataset,
		ID:   datasetID,
	}}); err != nil {
		_ = moveTx.Rollback().Error
		t.Fatalf("lock move resources and asset: %v", err)
	}

	deleteStarted := make(chan int, 1)
	deleteDone := make(chan error, 1)
	go func() {
		deleteTx := db.WithContext(ctx).Begin()
		if deleteTx.Error != nil {
			deleteDone <- deleteTx.Error
			return
		}
		defer func() { _ = deleteTx.Rollback().Error }()
		if err := deleteTx.Exec("SET LOCAL search_path TO " + quotedSchema).Error; err != nil {
			deleteDone <- err
			return
		}
		var backendPID int
		if err := deleteTx.Raw("SELECT pg_backend_pid()").Scan(&backendPID).Error; err != nil {
			deleteDone <- err
			return
		}
		deleteStarted <- backendPID
		if err := bindingRepo.WithTx(deleteTx).LockResources(ctx, deleteTx, []agentbindings.ResourceRef{ref}); err != nil {
			deleteDone <- err
			return
		}
		if err := deleteTx.Exec("DELETE FROM datasets WHERE id = ?", datasetID).Error; err != nil {
			deleteDone <- err
			return
		}
		deleteDone <- deleteTx.Commit().Error
	}()

	var deletePID int
	select {
	case deletePID = <-deleteStarted:
	case <-ctx.Done():
		_ = moveTx.Rollback().Error
		t.Fatalf("wait for delete transaction: %v", ctx.Err())
	}

	waitErr := waitForPostgresLockWait(ctx, db, deletePID)
	var moveErr error
	if waitErr == nil {
		// ApplyMoveImpact takes this lock again after preview. With the global
		// advisory-lock-first order this is reentrant; with the old row-first
		// order it completes the move/delete deadlock cycle.
		moveErr = bindingRepo.WithTx(moveTx).LockResources(ctx, moveTx, []agentbindings.ResourceRef{ref})
	}
	if waitErr != nil || moveErr != nil {
		_ = moveTx.Rollback().Error
	} else {
		moveErr = moveTx.Commit().Error
	}

	var deleteErr error
	select {
	case deleteErr = <-deleteDone:
	case <-ctx.Done():
		deleteErr = ctx.Err()
	}
	require.NoError(t, waitErr)
	require.NoError(t, moveErr)
	require.NoError(t, deleteErr)
	var remaining int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM "+quotedSchema+".datasets WHERE id = ?", datasetID).Scan(&remaining).Error)
	require.Zero(t, remaining)
}

func waitForPostgresLockWait(ctx context.Context, db *gorm.DB, backendPID int) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.WithContext(ctx).Raw(`
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE pid = ? AND wait_event_type = 'Lock'
			)
		`, backendPID).Scan(&waiting).Error; err != nil {
			return fmt.Errorf("inspect PostgreSQL lock wait: %w", err)
		}
		if waiting {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for PostgreSQL lock contention: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
