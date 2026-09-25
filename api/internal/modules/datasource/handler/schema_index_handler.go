package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datasource/indexing"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SchemaIndexHandler struct {
	service   *indexing.Service
	projector *transport.Projector
}

func NewSchemaIndexHandler(service *indexing.Service, productCatalog *catalog.Catalog) *SchemaIndexHandler {
	projector, err := transport.NewProjector(productCatalog)
	if err != nil {
		panic(err)
	}
	return &SchemaIndexHandler{service, projector}
}
func (h *SchemaIndexHandler) Status(c *gin.Context) { h.handle(c, false) }
func (h *SchemaIndexHandler) Start(c *gin.Context)  { h.handle(c, true) }

type schemaIndexConfigureRequest struct {
	Enabled                bool   `json:"enabled"`
	EmbeddingModelProvider string `json:"embedding_model_provider"`
	EmbeddingModel         string `json:"embedding_model"`
}

func (h *SchemaIndexHandler) Configure(c *gin.Context) {
	org, id, account, ok := dataSourceAccessPolicyRequestScope(c)
	if !ok {
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		h.fail(c, apperror.New(catalog.CodeRequestInvalid))
		return
	}
	var request schemaIndexConfigureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.fail(c, apperror.Wrap(err, catalog.CodeRequestInvalid))
		return
	}
	err := h.service.Configure(c.Request.Context(), org, id, account, indexing.ConfigureRequest{
		Enabled: request.Enabled, Provider: request.EmbeddingModelProvider, Model: request.EmbeddingModel,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, map[string]bool{"accepted": true})
}

func (h *SchemaIndexHandler) History(c *gin.Context) {
	org, id, account, ok := h.requestScope(c)
	if !ok {
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		h.fail(c, apperror.New(catalog.CodeRequestInvalid))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	result, err := h.service.History(c.Request.Context(), org, id, account, page, limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SchemaIndexHandler) handle(c *gin.Context, start bool) {
	org, id, account, ok := h.requestScope(c)
	if !ok {
		return
	}
	if _, err := uuid.Parse(id); err != nil {
		h.fail(c, apperror.New(catalog.CodeRequestInvalid))
		return
	}
	if start {
		if err := h.service.Start(c.Request.Context(), org, id, account); err != nil {
			h.fail(c, err)
			return
		}
		response.Success(c, map[string]bool{"accepted": true})
		return
	}
	result, err := h.service.Status(c.Request.Context(), org, id, account)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, result)
}

func (h *SchemaIndexHandler) requestScope(c *gin.Context) (organizationID, dataSourceID, accountID string, ok bool) {
	organizationID = util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		h.fail(c, apperror.New(catalog.CodeRequestInvalid))
		return "", "", "", false
	}
	accountID = c.GetString("account_id")
	if accountID == "" {
		h.fail(c, apperror.New(catalog.CodeAuthenticationRequired))
		return "", "", "", false
	}
	return organizationID, c.Param("id"), accountID, true
}

func (h *SchemaIndexHandler) fail(c *gin.Context, err error) {
	logger.L().Warn("database schema index request failed", zap.Error(err))
	code := catalog.CodeInternal
	switch {
	case errors.Is(err, indexing.ErrAccessDenied):
		code = catalog.CodePermissionDenied
	case errors.Is(err, gorm.ErrRecordNotFound):
		code = catalog.CodeResourceNotFound
	}
	if _, ok := apperror.As(err); !ok {
		err = apperror.Wrap(err, code, apperror.WithOperation("database.schema_index"))
	}
	locale, _ := catalog.ParseLocale(c.GetHeader("Accept-Language"))
	result := h.projector.Project(err, locale).Presentation
	c.JSON(result.HTTPStatus, response.Response{Code: result.Code.String(), Message: result.Message})
}
