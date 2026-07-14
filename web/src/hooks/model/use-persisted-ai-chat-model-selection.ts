'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import type { AIChatModelValue } from '@/components/chat/variants/aichat/types';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import type { DefaultModelUseCase, DefaultModelValue, ModelItem } from '@/services/types/model';
import {
  getLastSelectedAiModel,
  saveLastSelectedAiModel,
  type AiModelScope,
} from '@/utils/ui-local';

const EMPTY_MODEL_VALUE: AIChatModelValue = { provider: '', model: '', params: {} };

function modelMatches(
  candidate: Pick<AIChatModelValue, 'provider' | 'model'> | null | undefined,
  model: ModelItem
) {
  if (!candidate?.provider || !candidate.model) return false;
  return (
    model.provider === candidate.provider &&
    (model.model === candidate.model || model.model_name === candidate.model)
  );
}

function isModelAvailable(
  candidate: Pick<AIChatModelValue, 'provider' | 'model'> | null | undefined,
  models: ModelItem[]
) {
  return models.some(model => modelMatches(candidate, model));
}

function toAIChatModelValue(value: DefaultModelValue): AIChatModelValue {
  return {
    provider: value.provider,
    model: value.model,
    params: value.params,
  };
}

export function usePersistedAIChatModelSelection({
  accountId,
  scope,
  useCase = 'agent',
}: {
  accountId?: string | null;
  scope: AiModelScope;
  useCase?: DefaultModelUseCase;
}) {
  const [modelSelectorValue, setModelSelectorValue] = useState<AIChatModelValue>(() => {
    if (!accountId) return EMPTY_MODEL_VALUE;
    const saved = getLastSelectedAiModel(accountId, scope);
    return saved ? { provider: saved.provider, model: saved.model, params: {} } : EMPTY_MODEL_VALUE;
  });
  const [isInitialModelResolved, setIsInitialModelResolved] = useState(false);
  const availableModels = useAvailableModels({ use_case: useCase });
  const defaultModel = useDefaultModelByUseCase(useCase);

  const canValidateModels = !availableModels.isLoading && !availableModels.error;
  const isModelInitializing = !isInitialModelResolved;
  const isSelectedModelUnavailable =
    canValidateModels &&
    Boolean(modelSelectorValue.provider && modelSelectorValue.model) &&
    !isModelAvailable(modelSelectorValue, availableModels.models);

  useEffect(() => {
    if (!accountId) {
      setModelSelectorValue(EMPTY_MODEL_VALUE);
      setIsInitialModelResolved(false);
      return;
    }

    if (!canValidateModels) {
      return;
    }

    if (modelSelectorValue.model) {
      setIsInitialModelResolved(true);
      return;
    }

    const saved = getLastSelectedAiModel(accountId, scope);
    if (saved?.provider && saved.model) {
      setModelSelectorValue({
        provider: saved.provider,
        model: saved.model,
        params: {},
      });
      setIsInitialModelResolved(true);
      return;
    }

    if (!defaultModel.isResolved) {
      return;
    }

    if (defaultModel.value && isModelAvailable(defaultModel.value, availableModels.models)) {
      const next = toAIChatModelValue(defaultModel.value);
      saveLastSelectedAiModel(accountId, scope, {
        provider: next.provider,
        model: next.model,
      });
      setModelSelectorValue(next);
      setIsInitialModelResolved(true);
      return;
    }

    setModelSelectorValue(EMPTY_MODEL_VALUE);
    setIsInitialModelResolved(true);
  }, [
    accountId,
    availableModels.models,
    canValidateModels,
    defaultModel.isResolved,
    defaultModel.value,
    modelSelectorValue,
    scope,
  ]);

  const handleModelChange = useCallback(
    (value: ModelSelectorValue) => {
      setModelSelectorValue(previous => ({
        ...previous,
        provider: value.provider,
        model: value.model,
      }));

      if (accountId) {
        saveLastSelectedAiModel(accountId, scope, {
          provider: value.provider,
          model: value.model,
        });
      }
      setIsInitialModelResolved(true);
    },
    [accountId, scope]
  );

  return useMemo(
    () => ({
      modelSelectorValue,
      setModelSelectorValue,
      isInitialModelResolved,
      isModelInitializing,
      isSelectedModelUnavailable,
      handleModelChange,
      availableModels,
      defaultModel,
    }),
    [
      availableModels,
      defaultModel,
      handleModelChange,
      isInitialModelResolved,
      isModelInitializing,
      isSelectedModelUnavailable,
      modelSelectorValue,
    ]
  );
}
