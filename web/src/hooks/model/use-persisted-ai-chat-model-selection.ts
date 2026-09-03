'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import type {
  ModelSelectorValue,
  RoutingStrategy,
} from '@/components/common/model-selector';
import type { AIChatModelValue } from '@/components/chat/variants/aichat/types';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import type {
  AvailableModelUseCase,
  DefaultModelUseCase,
  DefaultModelValue,
  ModelItem,
} from '@/services/types/model';
import {
  getLastSelectedAiModel,
  saveLastSelectedAiModel,
  type AiModelScope,
} from '@/utils/ui-local';

const EMPTY_MODEL_VALUE: AIChatModelValue = {
  provider: '',
  model: '',
  routing_strategy: 'stability',
  params: {},
};

function normalizeRoutingStrategy(value: string | undefined): RoutingStrategy {
  switch (value) {
    case 'speed':
    case 'price':
    case 'stability':
      return value;
    default:
      return 'stability';
  }
}

function modelMatches(
  candidate: Pick<AIChatModelValue, 'provider' | 'model'> | null | undefined,
  model: ModelItem
) {
  if (!candidate?.provider || !candidate.model) return false;
  return model.provider === candidate.provider && model.model === candidate.model;
}

function isModelAvailable(
  candidate: Pick<AIChatModelValue, 'provider' | 'model'> | null | undefined,
  models: ModelItem[]
) {
  if (models.some(model => modelMatches(candidate, model))) return true;
  if (!candidate?.provider || !candidate.model) return false;
  const equivalents = models.filter(
    model =>
      !model.is_custom &&
      model.model === candidate.model &&
      (model.vendor === candidate.provider || model.provider === candidate.provider)
  );
  if (equivalents.length > 0) return true;
  const vendors = new Set(
    models
      .filter(model => !model.is_custom && model.model === candidate.model)
      .map(model => model.vendor || model.provider)
  );
  return vendors.size === 1;
}

function toAIChatModelValue(value: DefaultModelValue): AIChatModelValue {
  return {
    provider: value.provider,
    model: value.model,
    routing_strategy: 'stability',
    params: value.params,
  };
}

function firstAvailableAIChatModel(models: ModelItem[]): AIChatModelValue | null {
  const candidate = models.find(model => model.provider && model.model);
  if (!candidate) return null;
  return {
    provider: candidate.provider,
    model: candidate.model,
    routing_strategy: 'stability',
    params: {},
  };
}

export function usePersistedAIChatModelSelection({
  accountId,
  scope,
  legacyScope,
  repairUnavailableSelection = false,
  useCase = 'agent',
  availabilityUseCase,
  preferredUseCase,
}: {
  accountId?: string | null;
  scope: AiModelScope;
  legacyScope?: AiModelScope;
  repairUnavailableSelection?: boolean;
  useCase?: DefaultModelUseCase;
  availabilityUseCase?: AvailableModelUseCase;
  preferredUseCase?: DefaultModelUseCase;
}) {
  const [modelSelectorValue, setModelSelectorValue] = useState<AIChatModelValue>(() => {
    if (!accountId) return EMPTY_MODEL_VALUE;
    const saved = getLastSelectedAiModel(accountId, scope);
    return saved
      ? {
          provider: saved.provider,
          model: saved.model,
          routing_strategy: normalizeRoutingStrategy(saved.routing_strategy),
          params: {},
        }
      : EMPTY_MODEL_VALUE;
  });
  const [isInitialModelResolved, setIsInitialModelResolved] = useState(false);
  const availableModels = useAvailableModels({ use_case: availabilityUseCase ?? useCase });
  const preferredDefaultModel = useDefaultModelByUseCase(preferredUseCase ?? useCase);
  const fallbackDefaultModel = useDefaultModelByUseCase(useCase);
  const defaultModel =
    preferredDefaultModel.value &&
    isModelAvailable(preferredDefaultModel.value, availableModels.models)
      ? preferredDefaultModel
      : fallbackDefaultModel;
  const areDefaultModelsResolved =
    preferredDefaultModel.isResolved && fallbackDefaultModel.isResolved;

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
      if (
        repairUnavailableSelection &&
        !isModelAvailable(modelSelectorValue, availableModels.models)
      ) {
        if (!areDefaultModelsResolved) {
          return;
        }

        const replacement =
          defaultModel.value && isModelAvailable(defaultModel.value, availableModels.models)
            ? toAIChatModelValue(defaultModel.value)
            : (firstAvailableAIChatModel(availableModels.models) ?? EMPTY_MODEL_VALUE);
        if (replacement.provider && replacement.model) {
          saveLastSelectedAiModel(accountId, scope, {
            provider: replacement.provider,
            model: replacement.model,
          });
        }
        setModelSelectorValue(replacement);
        setIsInitialModelResolved(true);
        return;
      }

      setIsInitialModelResolved(true);
      return;
    }

    const saved = getLastSelectedAiModel(accountId, scope);
    if (saved?.provider && saved.model) {
      if (!repairUnavailableSelection || isModelAvailable(saved, availableModels.models)) {
        setModelSelectorValue({
          provider: saved.provider,
          model: saved.model,
          routing_strategy: normalizeRoutingStrategy(saved.routing_strategy),
          params: {},
        });
        setIsInitialModelResolved(true);
        return;
      }
    }

    const legacySaved =
      legacyScope && legacyScope !== scope
        ? getLastSelectedAiModel(accountId, legacyScope)
        : null;
    if (
      legacySaved?.provider &&
      legacySaved.model &&
      isModelAvailable(legacySaved, availableModels.models)
    ) {
      const next: AIChatModelValue = {
        provider: legacySaved.provider,
        model: legacySaved.model,
        routing_strategy: normalizeRoutingStrategy(legacySaved.routing_strategy),
        params: {},
      };
      saveLastSelectedAiModel(accountId, scope, {
        provider: next.provider,
        model: next.model,
      });
      setModelSelectorValue(next);
      setIsInitialModelResolved(true);
      return;
    }

    if (!areDefaultModelsResolved) {
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

    if (repairUnavailableSelection) {
      const next = firstAvailableAIChatModel(availableModels.models);
      if (next) {
        saveLastSelectedAiModel(accountId, scope, {
          provider: next.provider,
          model: next.model,
        });
        setModelSelectorValue(next);
        setIsInitialModelResolved(true);
        return;
      }
    }

    setModelSelectorValue(EMPTY_MODEL_VALUE);
    setIsInitialModelResolved(true);
  }, [
    accountId,
    availableModels.models,
    canValidateModels,
    defaultModel.value,
    areDefaultModelsResolved,
    legacyScope,
    modelSelectorValue,
    repairUnavailableSelection,
    scope,
  ]);

  const handleModelChange = useCallback(
    (value: ModelSelectorValue) => {
      const routingStrategy = normalizeRoutingStrategy(
        value.routing_strategy ?? modelSelectorValue.routing_strategy
      );
      setModelSelectorValue(previous => ({
        ...previous,
        provider: value.provider,
        model: value.model,
        routing_strategy: routingStrategy,
      }));

      if (accountId) {
        saveLastSelectedAiModel(accountId, scope, {
          provider: value.provider,
          model: value.model,
          routing_strategy: routingStrategy,
        });
      }
      setIsInitialModelResolved(true);
    },
    [accountId, modelSelectorValue.routing_strategy, scope]
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
