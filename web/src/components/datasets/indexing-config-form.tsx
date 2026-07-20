'use client';

import React, { useState, useEffect, useRef, forwardRef, useCallback } from 'react';
import { useT } from '@/i18n';
import { ArrowUpDown, Settings } from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  useDefaultModelByUseCase,
  useInitializeDefaultModelByUseCase,
} from '@/hooks/model/use-default-model-by-use-case';
import { ErrorBoundary } from '@/components/error-boundary';
import type { DatasetUploadFormData, Dataset } from '@/services/types/dataset';
import {
  EmbeddingSettings as EmbeddingModelCard,
  type EmbeddingSettingsRef as EmbeddingModelCardRef,
  GraphModelSettings,
  RetrievalSettings as RetrievalConfigCard,
  type RetrievalConfig,
  type RetrievalSettingsRef as RetrievalConfigCardRef,
  PreprocessingSettings as PreprocessingRulesCard,
  type PreprocessingSettingsRef as PreprocessingRulesCardRef,
} from './indexing-config';
import { useAvailableModels } from '@/hooks/model/use-model';
import { ModelSelector } from '@/components/common/model-selector';
import type { ProcessConfiguration } from '@/services/types/dataset';
import { normalizeDatasetSearchMethod } from '@/utils/dataset/retrieval-config';
import { ModelFieldSection } from './indexing-config/model-field-section';

// Indexing types
enum IndexingType {
  QUALIFIED = 'high_quality',
}
export { IndexingType };

/**
 * ConfigErrorFallback - Error fallback for configuration form
 */
function ConfigErrorFallback() {
  const t = useT('common');
  return (
    <div className="flex min-h-[400px] flex-col items-center justify-center rounded-md border border-dashed p-8 text-center">
      <div className="flex h-20 w-20 items-center justify-center rounded-full bg-muted">
        <Settings className="h-10 w-10 text-orange-500" />
      </div>
      <h2 className="mt-6 text-xl font-semibold">{t('errorBoundary.configurationError')}</h2>
      <p className="mb-8 mt-2 text-center text-sm text-muted-foreground max-w-md">
        {t('errorBoundary.configurationErrorMessage')}
      </p>
    </div>
  );
}

// Component props interface
interface DatasetIndexingConfigFormProps {
  /** Form data */
  data: DatasetUploadFormData;
  /** Change handler */
  onChange: (stepData: Partial<DatasetUploadFormData>) => void;
  /** Additional CSS classes */
  className?: string;
  /** Whether the form is in settings mode (vs creation mode) */
  isSettingMode?: boolean;
  /** Current dataset (for settings mode) */
  currentDataset?: Dataset;
  /** Document ID for settings mode */
  documentId?: string;
  /** Loading state */
  isLoading?: boolean;
}

export interface FormRef {
  getFormData: () => DatasetUploadFormData;
}

/**
 * DatasetIndexingConfigForm - Reusable form component for dataset indexing configuration
 *
 * Features:
 * - Dynamic rendering based on mode (creation vs settings)
 * - Chunking mode selection with conditional rendering
 * - Pre-processing rules configuration
 * - Indexing technique and embedding model selection
 * - Retrieval configuration with reranking support
 * - Parent-child mode support
 * - Validation and error handling
 */
export const DatasetIndexingConfigForm = forwardRef<FormRef, DatasetIndexingConfigFormProps>(
  ({ data, onChange, className, isSettingMode = false, currentDataset }, ref) => {
    const t = useT('datasets');
    const graphFlowEnabled = Boolean(data.enableGraphFlow ?? currentDataset?.enable_graph_flow);
    const defaultRerankModel = useDefaultModelByUseCase('rerank');
    const { models: rerankModels } = useAvailableModels({ use_case: 'rerank' });

    const defaultRerankModelData =
      rerankModels?.find(m => m.model === defaultRerankModel?.value?.model) || rerankModels?.[0];

    // Initialize default embedding model if missing
    useInitializeDefaultModelByUseCase({
      useCase: 'embedding',
      currentModel: {
        provider: data.embeddingModelProvider || undefined,
        model: data.embeddingModel || undefined,
      },
      enabled: !isSettingMode,
      onInitialize: v => {
        onChange({
          embeddingModel: v.model,
          embeddingModelProvider: v.provider,
        });
      },
    });

    // Initialize default graph model when graph flow is enabled and the current value is empty
    useInitializeDefaultModelByUseCase({
      useCase: 'text-chat',
      currentModel: {
        provider: data.entityModelProvider || undefined,
        model: data.entityModel || undefined,
      },
      enabled: graphFlowEnabled,
      onInitialize: v => {
        onChange({
          entityModel: v.model,
          entityModelProvider: v.provider,
        });
      },
    });

    // Initialize default rerank model if missing
    useInitializeDefaultModelByUseCase({
      useCase: 'rerank',
      currentModel: {
        provider: (data.retrievalConfig as RetrievalConfig)?.reranking_model
          ?.reranking_provider_name,
        model: (data.retrievalConfig as RetrievalConfig)?.reranking_model?.reranking_model_name,
      },
      enabled: !isSettingMode,
      onInitialize: v => {
        const currentRetrieval = (data.retrievalConfig ?? {}) as RetrievalConfig;
        onChange({
          retrievalConfig: {
            ...currentRetrieval,
            reranking_model: {
              reranking_model_name: v.model,
              reranking_provider_name: v.provider,
            },
          },
        });
      },
    });

    // Initialize local retrieval state management
    const [retrieval, setRetrieval] = useState<RetrievalConfig>(() => {
      const incomingRetrieval = (data.retrievalConfig ?? {}) as RetrievalConfig;
      const initialConfig: RetrievalConfig = {
        search_method: normalizeDatasetSearchMethod(
          incomingRetrieval.search_method,
          graphFlowEnabled
        ),
        top_k: incomingRetrieval.top_k ?? 10,
        score_threshold_enabled: incomingRetrieval.score_threshold_enabled ?? true,
        score_threshold: incomingRetrieval.score_threshold ?? 0.35,
        reranking_enable: incomingRetrieval.reranking_enable ?? true,
        reranking_model: incomingRetrieval.reranking_model ?? {
          reranking_model_name: defaultRerankModelData?.model || '',
          reranking_provider_name: defaultRerankModelData?.provider || '',
        },
      };

      return initialConfig;
    });

    // Update local retrieval state when data changes
    useEffect(() => {
      const incomingRetrieval = (data.retrievalConfig ?? {}) as RetrievalConfig;
      const newRetrieval: RetrievalConfig = {
        ...retrieval,
        search_method: normalizeDatasetSearchMethod(
          incomingRetrieval.search_method ?? retrieval.search_method,
          graphFlowEnabled
        ),
        top_k: incomingRetrieval.top_k ?? retrieval.top_k,
        score_threshold_enabled:
          incomingRetrieval.score_threshold_enabled ?? retrieval.score_threshold_enabled,
        score_threshold: incomingRetrieval.score_threshold ?? retrieval.score_threshold,
        reranking_enable: incomingRetrieval.reranking_enable ?? retrieval.reranking_enable,
        reranking_model: incomingRetrieval.reranking_model ?? retrieval.reranking_model,
      };

      // Only update if there are actual changes to prevent infinite loop
      if (JSON.stringify(newRetrieval) !== JSON.stringify(retrieval)) {
        setRetrieval(newRetrieval);
      }
    }, [data.retrievalConfig, graphFlowEnabled, retrieval]);

    // Expose getFormData method via ref
    React.useImperativeHandle(
      ref,
      () => ({
        getFormData: () => {
          // Get latest data from all child components

          const retrievalData = retrievalConfigRef.current?.getFormData() || retrieval;
          const embeddingModelData = embeddingModelCardRef.current?.getFormData();
          // Enforce high_quality indexing; selector removed
          const preprocessingRulesData = preprocessingRulesCardRef.current?.getFormData();

          const currentFormData: DatasetUploadFormData = {
            ...data,
            docLanguage: preprocessingRulesData?.doc_language || data.docLanguage,
            indexType: IndexingType.QUALIFIED,
            retrievalConfig: retrievalData,
            embeddingModel: embeddingModelData?.model || data.embeddingModel,
            embeddingModelProvider: embeddingModelData?.provider || data.embeddingModelProvider,
            enableGraphFlow: graphFlowEnabled,
            entityModel: data.entityModel,
            entityModelProvider: data.entityModelProvider,
            processConfig: {
              ...data.processConfig,
              clean_mode: data.processConfig?.clean_mode || 'automatic',
              pre_processing_rules:
                preprocessingRulesData?.pre_processing_rules ||
                data.processConfig?.pre_processing_rules ||
                [],
              rules: {
                ...data.processConfig?.rules,
                remove_extra_spaces:
                  preprocessingRulesData?.pre_processing_rules?.find(
                    rule => rule.id === 'remove_extra_spaces'
                  )?.enabled || false,
                remove_urls_emails:
                  preprocessingRulesData?.pre_processing_rules?.find(
                    rule => rule.id === 'remove_urls_emails'
                  )?.enabled || false,
              },
            } as ProcessConfiguration,
          };

          return currentFormData;
        },
      }),
      [data, retrieval, graphFlowEnabled]
    );

    // Modified logic: only disable if embedding is not available in settings mode
    const isModelAndRetrievalConfigDisabled = isSettingMode && !currentDataset?.embedding_available;

    const showEmbeddingModelConfig = true;
    const showRetrievalMethodConfig = true;

    // Store onChange ref to avoid dependency issues
    const onChangeRef = useRef(onChange);
    onChangeRef.current = onChange;

    // Refs for child components
    const retrievalConfigRef = useRef<RetrievalConfigCardRef>(null);
    const embeddingModelCardRef = useRef<EmbeddingModelCardRef>(null);
    const preprocessingRulesCardRef = useRef<PreprocessingRulesCardRef>(null);

    const handleDocLanguageChange = useCallback((docLanguage: string) => {
      onChangeRef.current({ docLanguage });
    }, []);

    const handleProcessConfigChange = useCallback((processConfig: ProcessConfiguration) => {
      onChangeRef.current({ processConfig });
    }, []);

    // Handle embedding model change
    const handleEmbeddingModelChange = (embeddingModel: { provider: string; model: string }) => {
      // Sync to parent as this affects retrieval config
      onChange({
        embeddingModel: embeddingModel.model,
        embeddingModelProvider: embeddingModel.provider,
      });
    };

    const handleRerankModelChange = ({ provider, model }: { provider: string; model: string }) => {
      setRetrieval(prev => {
        const nextRetrieval = {
          ...prev,
          reranking_enable: true,
          reranking_model: {
            reranking_provider_name: provider,
            reranking_model_name: model,
          },
        };
        onChange({ retrievalConfig: nextRetrieval });
        return nextRetrieval;
      });
    };

    // In settings mode, render configuration sections directly without chunking mode selection
    if (isSettingMode) {
      return (
        <div className={cn('space-y-4', className)}>
          {/* Embedding Model */}
          {showEmbeddingModelConfig && (
            <div className="grid gap-4 md:grid-cols-2">
              <EmbeddingModelCard
                ref={embeddingModelCardRef}
                embeddingModel={{
                  provider: data.embeddingModelProvider || '',
                  model: data.embeddingModel || '',
                }}
                onChange={handleEmbeddingModelChange}
                disabled
                readOnlyDisplay
                titleTooltip={t('createWizard.processConfig.embeddingModel.lockedTooltip')}
              />
              <ModelFieldSection
                icon={ArrowUpDown}
                title={t('createWizard.processConfig.changeRerankModel')}
              >
                <ModelSelector
                  modelType="rerank"
                  value={{
                    provider: retrieval.reranking_model?.reranking_provider_name || '',
                    model: retrieval.reranking_model?.reranking_model_name || '',
                  }}
                  onChange={handleRerankModelChange}
                  disabled={isModelAndRetrievalConfigDisabled}
                />
              </ModelFieldSection>
            </div>
          )}

          {graphFlowEnabled ? (
            <div>
              <GraphModelSettings
                graphModel={{
                  provider: data.entityModelProvider || '',
                  model: data.entityModel || '',
                }}
                onChange={graphModel => {
                  onChange({
                    entityModel: graphModel.model,
                    entityModelProvider: graphModel.provider,
                  });
                }}
                disabled={isModelAndRetrievalConfigDisabled}
              />
            </div>
          ) : null}

          {/* Retrieval Configuration */}
          {showRetrievalMethodConfig && (
            <div>
              <RetrievalConfigCard
                ref={retrievalConfigRef}
                retrieval={retrieval as RetrievalConfig}
                disabled={isModelAndRetrievalConfigDisabled}
                isGraphEnabled={graphFlowEnabled}
                rerankingLabel={t('createWizard.processConfig.changeRerankModel')}
                showRerankingModel={false}
                onChange={updatedRetrieval => {
                  setRetrieval(updatedRetrieval);
                  onChange({ retrievalConfig: updatedRetrieval });
                }}
              />
            </div>
          )}
        </div>
      );
    }

    // Regular creation mode rendering
    return (
      <div className={cn('space-y-4', className)}>
        {/* Pre-processing Rules - Direct component without card wrapper */}
        <PreprocessingRulesCard
          ref={preprocessingRulesCardRef}
          processConfig={data.processConfig}
          onChange={handleProcessConfigChange}
          docLanguage={data.docLanguage}
          onDocLanguageChange={handleDocLanguageChange}
          ruleColumns={2}
        />

        {/* Embedding Model - Direct component without card wrapper */}
        {showEmbeddingModelConfig && (
          <EmbeddingModelCard
            ref={embeddingModelCardRef}
            embeddingModel={{
              provider: data.embeddingModelProvider || '',
              model: data.embeddingModel || '',
            }}
            onChange={handleEmbeddingModelChange}
            disabled={isModelAndRetrievalConfigDisabled}
          />
        )}

        {graphFlowEnabled ? (
          <GraphModelSettings
            graphModel={{
              provider: data.entityModelProvider || '',
              model: data.entityModel || '',
            }}
            onChange={graphModel => {
              onChange({
                entityModel: graphModel.model,
                entityModelProvider: graphModel.provider,
              });
            }}
            disabled={isModelAndRetrievalConfigDisabled}
          />
        ) : null}

        {/* Retrieval Configuration - Direct component without card wrapper */}
        {showRetrievalMethodConfig && (
          <RetrievalConfigCard
            ref={retrievalConfigRef}
            retrieval={retrieval as RetrievalConfig}
            disabled={isModelAndRetrievalConfigDisabled}
            isGraphEnabled={graphFlowEnabled}
            onChange={updatedRetrieval => {
              onChange({ retrievalConfig: updatedRetrieval });
            }}
          />
        )}
      </div>
    );
  }
);

// Set a display name for better debugging and to satisfy eslint-react/display-name rule
DatasetIndexingConfigForm.displayName = 'DatasetIndexingConfigForm';

// Export the component wrapped with ErrorBoundary for better error handling
export const DatasetIndexingConfigFormWithErrorBoundary = forwardRef<
  FormRef,
  DatasetIndexingConfigFormProps
>((props, ref) => {
  return (
    <ErrorBoundary fallback={<ConfigErrorFallback />}>
      <DatasetIndexingConfigForm {...props} ref={ref} />
    </ErrorBoundary>
  );
});

DatasetIndexingConfigFormWithErrorBoundary.displayName =
  'DatasetIndexingConfigFormWithErrorBoundary';
