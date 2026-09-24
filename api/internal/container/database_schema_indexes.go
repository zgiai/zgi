package container

import (
	"github.com/zgiai/zgi/api/internal/modules/datasource/indexing"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

func (c *ServiceContainer) GetDatabaseSchemaIndexes() *indexing.Service {
	if c.databaseSchemaIndexes == nil {
		c.databaseSchemaIndexes = indexing.NewService(c.db, c.GetSQLBase(), c.GetDefaultModelService(), c.GetLLMClient(), vectordb.NewWeaviateClient(&c.config.VectorStore), c.GetTaskManager(), c.GetOrganizationService(), logger.L())
	}
	return c.databaseSchemaIndexes
}
