package defaultmodel

import (
	defaultmodelhandler "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/handler"
	defaultmodelrepo "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/repository"
	defaultmodelservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelrepo "github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	llmmodelservice "github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	"gorm.io/gorm"
)

type Module struct {
	Repo    defaultmodelrepo.DefaultModelRepository
	Service defaultmodelservice.DefaultModelService
	Handler *defaultmodelhandler.Handler
}

func NewModule(
	db *gorm.DB,
	availableModelsSvc llmmodelservice.AvailableModelsService,
	globalRepo llmmodelrepo.ModelRepository,
	customRepo llmmodelrepo.CustomModelRepository,
) *Module {
	repo := defaultmodelrepo.NewDefaultModelRepository(db)
	service := defaultmodelservice.NewDefaultModelService(repo, availableModelsSvc, globalRepo, customRepo)
	handler := defaultmodelhandler.NewHandler(service)

	return &Module{
		Repo:    repo,
		Service: service,
		Handler: handler,
	}
}

