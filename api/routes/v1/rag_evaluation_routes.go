package v1

import (
	"github.com/gin-gonic/gin"
	datasetHandler "github.com/zgiai/zgi/api/internal/modules/dataset/handler"
	datasetService "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

type RAGEvaluationRouteDeps struct {
	AccountService            interfaces.AccountService
	OrganizationService       interfaces.OrganizationService
	KnowledgeRetrievalService *datasetService.KnowledgeRetrievalService
	LLMClient                 llmclient.LLMClient
	DefaultModelService       llmdefaultservice.DefaultModelService
}

func RegisterRAGEvaluationRoutes(router *gin.RouterGroup, deps RAGEvaluationRouteDeps) {
	validateRAGEvaluationRouteDeps(deps)

	handler := datasetHandler.NewRAGEvaluationHandler(
		deps.KnowledgeRetrievalService,
		deps.LLMClient,
		deps.DefaultModelService,
		deps.OrganizationService,
	)

	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(deps.AccountService))
	authWithTenant.POST("/rag-evaluation/batch", handler.BatchEvaluate)
}

func validateRAGEvaluationRouteDeps(deps RAGEvaluationRouteDeps) {
	if deps.AccountService == nil {
		panic("rag evaluation routes require account service")
	}
	if deps.OrganizationService == nil {
		panic("rag evaluation routes require organization service")
	}
	if deps.KnowledgeRetrievalService == nil {
		panic("rag evaluation routes require knowledge retrieval service")
	}
	if deps.LLMClient == nil {
		panic("rag evaluation routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("rag evaluation routes require default model service")
	}
}
