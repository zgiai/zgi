package text2sql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/pkg/sql_base"
	"gorm.io/gorm"
)

// CatalogLoader resolves caller-selected business table IDs to fresh physical
// metadata. Implementations must not trust physical IDs supplied by callers.
type CatalogLoader interface {
	Load(ctx context.Context, req CatalogRequest) (Catalog, error)
}

type CatalogRequest struct {
	Scope        Scope
	DataSourceID string
	Tables       []TableRef
}

type SQLCatalogLoader struct {
	db  *gorm.DB
	sql sql_base.SQLBase
}

func NewSQLCatalogLoader(db *gorm.DB, sql sql_base.SQLBase) *SQLCatalogLoader {
	return &SQLCatalogLoader{db: db, sql: sql}
}

func (l *SQLCatalogLoader) Load(ctx context.Context, req CatalogRequest) (Catalog, error) {
	if l == nil || l.db == nil || l.sql == nil || strings.TrimSpace(req.Scope.OrganizationID) == "" ||
		strings.TrimSpace(req.Scope.WorkspaceID) == "" || strings.TrimSpace(req.DataSourceID) == "" || len(req.Tables) == 0 {
		return Catalog{}, ErrSchemaLoad
	}

	var dataSource struct{ ID string }
	if err := l.db.WithContext(ctx).
		Table("data_sources").
		Select("id").
		Where("id = ? AND organization_id = ? AND workspace_id = ?", req.DataSourceID, req.Scope.OrganizationID, req.Scope.WorkspaceID).
		Take(&dataSource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Catalog{}, ErrTableOutOfScope
		}
		return Catalog{}, fmt.Errorf("%w: validate data source scope: %v", ErrSchemaLoad, err)
	}

	filters := make([]string, 0, len(req.Tables)*2)
	filterArgs := make([]any, 0, len(req.Tables)*2)
	for _, ref := range req.Tables {
		assetID := strings.TrimSpace(ref.AssetID)
		if assetID == "" && ref.PhysicalID <= 0 {
			return Catalog{}, ErrTableOutOfScope
		}
		if assetID != "" {
			filters = append(filters, "id = ?")
			filterArgs = append(filterArgs, assetID)
		} else if ref.PhysicalID > 0 {
			filters = append(filters, "table_id = ?")
			filterArgs = append(filterArgs, ref.PhysicalID)
		}
	}

	type tableMapping struct {
		ID      string
		TableID int
	}
	var mappings []tableMapping
	if err := l.db.WithContext(ctx).
		Table("data_source_tables").
		Select("id, table_id").
		Where("organization_id = ? AND data_source_id = ?", req.Scope.OrganizationID, req.DataSourceID).
		Where("("+strings.Join(filters, " OR ")+")", filterArgs...).
		Find(&mappings).Error; err != nil {
		return Catalog{}, fmt.Errorf("%w: load data source table mappings: %v", ErrSchemaLoad, err)
	}

	catalog := Catalog{Tables: make([]CatalogTable, 0, len(req.Tables))}
	seenMappings := make(map[string]struct{}, len(req.Tables))
	for _, requested := range req.Tables {
		var matched *tableMapping
		for idx := range mappings {
			candidate := &mappings[idx]
			if assetID := strings.TrimSpace(requested.AssetID); assetID != "" && candidate.ID != assetID {
				continue
			}
			if strings.TrimSpace(requested.AssetID) == "" && requested.PhysicalID > 0 && candidate.TableID != requested.PhysicalID {
				continue
			}
			matched = candidate
			break
		}
		if matched == nil || strings.TrimSpace(matched.ID) == "" || matched.TableID <= 0 {
			return Catalog{}, ErrTableOutOfScope
		}
		if _, duplicate := seenMappings[matched.ID]; duplicate {
			return Catalog{}, ErrTableOutOfScope
		}
		seenMappings[matched.ID] = struct{}{}
		table, err := l.sql.GetTable(ctx, matched.TableID)
		if err != nil || table == nil {
			return Catalog{}, fmt.Errorf("%w: table %s", ErrSchemaLoad, matched.ID)
		}
		catalog.Tables = append(catalog.Tables, CatalogTable{
			Ref:   TableRef{AssetID: matched.ID, PhysicalID: matched.TableID, Schema: table.Schema, Name: table.Name},
			Table: table,
		})
	}
	return catalog, nil
}
