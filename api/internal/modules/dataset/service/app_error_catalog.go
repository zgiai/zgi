package service

import (
	"errors"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// AppCodeEmbeddingModelUnavailable identifies a knowledge base whose embedding
// model cannot be served in the current workspace (no enabled route, or the
// model is unknown or inactive).
var AppCodeEmbeddingModelUnavailable = apperror.MustCode("dataset.embedding_model.unavailable")

// CatalogDefinitions returns the knowledge-base-owned public application errors.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{{
		Code:       AppCodeEmbeddingModelUnavailable,
		Category:   appcatalog.CategoryConfiguration,
		HTTPStatus: 409,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS: "The embedding model {model} of this knowledge base is not available in this workspace. " +
				"Enable a route for the model in model management, or switch the knowledge base to an available embedding model and re-index it.",
			appcatalog.LocaleChineseSimplified: "该知识库使用的 Embedding 模型 {model} 在当前工作空间不可用。" +
				"请在模型管理中为该模型启用路由，或将知识库切换为可用的 Embedding 模型并重新索引。",
		},
		Parameters: []appcatalog.Parameter{{Name: "model", Type: appcatalog.ParamString}},
	}}
}

// IsEmbeddingModelUnavailable reports whether err means the embedding model
// cannot be resolved for the workspace, as opposed to a transient outage.
func IsEmbeddingModelUnavailable(err error) bool {
	return errors.Is(err, llmerrors.DomainErrModelNotFound) ||
		errors.Is(err, llmerrors.DomainErrRouteNotFound) ||
		errors.Is(err, gateway.ErrModelNotFound) ||
		errors.Is(err, gateway.ErrModelNotActive) ||
		errors.Is(err, adapter.ErrModelNotFound)
}

// EmbeddingModelUnavailableError wraps err with the cataloged identity and the
// dataset's configured model name as its only public parameter.
func EmbeddingModelUnavailableError(err error, modelName string) error {
	return apperror.Wrap(err, AppCodeEmbeddingModelUnavailable,
		apperror.WithOperation("dataset.retrieval.embed_query"),
		apperror.WithParams(apperror.StringParam("model", modelName)))
}
