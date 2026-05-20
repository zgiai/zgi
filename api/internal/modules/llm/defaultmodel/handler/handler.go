package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmsharedtypes "github.com/zgiai/ginext/internal/modules/llm/shared/types"
	"github.com/zgiai/ginext/pkg/response"
)

type Handler struct {
	service llmdefaultservice.DefaultModelService
}

type upsertDefaultModelRequest struct {
	Provider string                   `json:"provider" binding:"required"`
	Model    string                   `json:"model" binding:"required"`
	Params   *llmsharedtypes.JSONObject `json:"params"`
}

type listDefaultModelsResponse struct {
	Items []*llmdefaultservice.ResolvedModel `json:"items"`
	Total int                                `json:"total"`
}

func NewHandler(service llmdefaultservice.DefaultModelService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrUnauthorized, "organization context missing")
		return
	}

	items, err := h.service.ListResolved(c.Request.Context(), organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, &listDefaultModelsResponse{
		Items: items,
		Total: len(items),
	})
}

func (h *Handler) Upsert(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrUnauthorized, "organization context missing")
		return
	}

	useCase := llmmodelmodel.UseCase(c.Param("use_case"))
	var req upsertDefaultModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	params := llmsharedtypes.JSONObject{}
	if req.Params != nil {
		params = *req.Params
	}

	actorID := getOptionalActorID(c)
	item, err := h.service.Upsert(c.Request.Context(), organizationID, actorID, useCase, req.Provider, req.Model, params)
	if err != nil {
		h.fail(c, err)
		return
	}

	response.Success(c, item)
}

func (h *Handler) Delete(c *gin.Context) {
	organizationID, err := getOrganizationID(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrUnauthorized, "organization context missing")
		return
	}

	useCase := llmmodelmodel.UseCase(c.Param("use_case"))
	if err := h.service.Delete(c.Request.Context(), organizationID, useCase); err != nil {
		h.fail(c, err)
		return
	}

	response.Success(c, nil)
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, llmdefaultservice.ErrInvalidUseCase), errors.Is(err, llmdefaultservice.ErrInvalidParams):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	case errors.Is(err, llmdefaultservice.ErrModelUnavailable):
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
	default:
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
	}
}

func getOrganizationID(c *gin.Context) (uuid.UUID, error) {
	organizationID := c.GetString("organization_id")
	if organizationID == "" {
		organizationID = c.GetHeader("X-Organization-ID")
	}
	if organizationID == "" {
		return uuid.Nil, llmdefaultservice.ErrOrganizationIDRequired
	}
	return uuid.Parse(organizationID)
}

func getOptionalActorID(c *gin.Context) *uuid.UUID {
	accountID := c.GetString("account_id")
	if accountID == "" {
		return nil
	}
	parsed, err := uuid.Parse(accountID)
	if err != nil {
		return nil
	}
	return &parsed
}

