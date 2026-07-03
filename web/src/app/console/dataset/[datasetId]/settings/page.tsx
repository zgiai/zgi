'use client';

import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams } from 'next/navigation';
import { useT } from '@/i18n';
import { Save, ShieldAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useDataset, useUpdateDataset } from '@/hooks/dataset/use-datasets';
import {
  DatasetIndexingConfigForm,
  type FormRef,
} from '@/components/datasets/indexing-config-form';
import type {
  DatasetUploadFormData,
  SearchMethod,
  UpdateDatasetRequest,
} from '@/services/types/dataset';
import { IconInput } from '@/components/common/icon-input';
import {
  type IconValue,
  createTextIconValue,
  createImageIconValue,
} from '@/components/common/icon-input/types';
import { getNameValidationErrors } from '@/utils/validation';
import { cn } from '@/lib/utils';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { normalizeDatasetSearchMethod } from '@/utils/dataset/retrieval-config';
import { toast } from 'sonner';

export default function DatasetSettingsPage() {
  const { datasetId } = useParams<{ datasetId: string }>();
  const { data, isLoading } = useDataset(datasetId);
  const t = useT();
  const updateDataset = useUpdateDataset(datasetId);
  const currentWorkspace = useCurrentWorkspace();

  // Permission checking
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canManage = hasPermission('knowledge_base.manage');

  // Form state
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue(ICON_TEXT, ICON_BG));
  // const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false);

  // Ref to get data from child component
  const configFormRef = useRef<FormRef>(null);

  // Configuration form data - simplified, will be retrieved from child component on save
  const [configData, setConfigData] = useState<DatasetUploadFormData>({
    files: [],
    notionPages: [],
    crawlResults: [],
    processConfig: null,
    indexType: 'high_quality',
    embeddingModel: null,
    embeddingModelProvider: null,
    enableGraphFlow: false,
    entityModel: null,
    entityModelProvider: null,
    retrievalConfig: {
      search_method: 'hybrid_search',
      top_k: 10,
      score_threshold_enabled: true,
      score_threshold: 0.35,
      reranking_enable: true,
      reranking_model: {
        reranking_model_name: '',
        reranking_provider_name: '',
      },
    },
  });

  const dataset = data?.data;
  const isGraphFlowEnabled = Boolean(dataset?.enable_graph_flow);
  // const isExternalDataSource = !!dataset?.external_knowledge_info?.external_knowledge_id;

  // Initialize form data from dataset
  useEffect(() => {
    if (dataset) {
      setName(dataset.name || '');
      setDescription(dataset.description || '');

      // Initialize iconValue based on dataset icon data
      if (dataset.icon_type === 'image') {
        setIconValue(createImageIconValue(dataset.icon_url || '', dataset.icon || ''));
      } else {
        setIconValue(
          createTextIconValue(dataset.icon || ICON_TEXT, dataset.icon_background || ICON_BG)
        );
      }

      // Initialize config data
      setConfigData({
        files: [],
        notionPages: [],
        crawlResults: [],
        processConfig: null,
        indexType: 'high_quality',
        embeddingModel: dataset.embedding_model || null,
        embeddingModelProvider: dataset.embedding_model_provider || null,
        enableGraphFlow: dataset.enable_graph_flow ?? false,
        entityModel: dataset.entity_model || null,
        entityModelProvider: dataset.entity_model_provider || null,
        retrievalConfig: dataset.retrieval_config
          ? {
              search_method: normalizeDatasetSearchMethod(
                dataset.retrieval_config.search_method as SearchMethod,
                Boolean(dataset.enable_graph_flow)
              ),
              top_k: dataset.retrieval_config.top_k,
              score_threshold_enabled: dataset.retrieval_config.score_threshold_enabled,
              score_threshold: dataset.retrieval_config.score_threshold,
              reranking_enable: dataset.retrieval_config.reranking_enable,
              reranking_model: {
                reranking_model_name:
                  dataset.retrieval_config.reranking_model?.reranking_model_name || '',
                reranking_provider_name:
                  dataset.retrieval_config.reranking_model?.reranking_provider_name || '',
              },
            }
          : null,
      });
    }
  }, [dataset]);

  // Handle config form changes - simplified since data is retrieved from child component on save
  const handleConfigChange = useCallback(
    (changes: Partial<DatasetUploadFormData>) => {
      setConfigData(prev => {
        const nextGraphFlowEnabled = Boolean(dataset?.enable_graph_flow);
        const nextRetrievalConfig = changes.retrievalConfig
          ? {
              ...changes.retrievalConfig,
              search_method: normalizeDatasetSearchMethod(
                changes.retrievalConfig.search_method,
                nextGraphFlowEnabled
              ),
            }
          : prev.retrievalConfig;

        return {
          ...prev,
          ...changes,
          enableGraphFlow: nextGraphFlowEnabled,
          retrievalConfig: nextRetrievalConfig,
        };
      });
    },
    [dataset?.enable_graph_flow]
  );

  // Inline validation for name field
  const nameErrors = useMemo(() => getNameValidationErrors(name, { allowSpace: true }), [name]);
  const isNameValid = nameErrors.length === 0;

  const handleSave = useCallback(async () => {
    if (updateDataset.isPending || !dataset) return;

    if (!isNameValid) {
      // Show validation error without using toast directly
      return;
    }

    // Get current form data from child component
    const currentFormData = configFormRef.current?.getFormData() || configData;
    const nextGraphFlowEnabled = Boolean(dataset.enable_graph_flow);
    const workspaceId = currentWorkspace?.id || dataset.workspace_id || dataset.workspace?.id || '';
    const normalizedRetrievalConfig = currentFormData.retrievalConfig
      ? {
          ...currentFormData.retrievalConfig,
          search_method: normalizeDatasetSearchMethod(
            currentFormData.retrievalConfig.search_method,
            nextGraphFlowEnabled
          ),
        }
      : undefined;

    if (
      nextGraphFlowEnabled &&
      !(currentFormData.entityModel && currentFormData.entityModelProvider)
    ) {
      toast.error(t('datasets.validation.graphModel.required'));
      return;
    }

    // Build update payload with proper icon_type handling (text/image)
    const requestParams: UpdateDatasetRequest = {
      name,
      description,
      icon_type: iconValue.type,
      icon: iconValue.type === 'image' ? iconValue.imageId || iconValue.iconUrl : iconValue.icon,
      icon_background: iconValue.type === 'text' ? iconValue.iconBackground : undefined,
      workspace_id: workspaceId,
      enable_graph_flow: nextGraphFlowEnabled,
      ...(nextGraphFlowEnabled
        ? {
            entity_model: currentFormData.entityModel || undefined,
            entity_model_provider: currentFormData.entityModelProvider || undefined,
          }
        : {}),
      retrieval_config: normalizedRetrievalConfig
        ? {
            search_method: normalizedRetrievalConfig.search_method,
            top_k: normalizedRetrievalConfig.top_k,
            score_threshold_enabled: normalizedRetrievalConfig.score_threshold_enabled,
            score_threshold: normalizedRetrievalConfig.score_threshold,
            reranking_enable: normalizedRetrievalConfig.reranking_enable,
            reranking_model: normalizedRetrievalConfig.reranking_model,
          }
        : undefined,
    };

    updateDataset.mutate(requestParams);
  }, [
    updateDataset,
    dataset,
    name,
    description,
    iconValue,
    currentWorkspace,
    configData,
    isNameValid,
    t,
  ]);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('datasets.loading')}
      </div>
    );
  }

  if (!dataset) {
    return <div>Dataset not found</div>;
  }

  // Check manage permission - show empty state if no permission
  if (!isPermissionsLoading && !canManage) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 text-center p-8">
        <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
          <ShieldAlert className="w-8 h-8 text-muted-foreground" />
        </div>
        <div className="space-y-2">
          <h2 className="text-lg font-semibold text-foreground">{t('common.accessDenied')}</h2>
          <p className="text-sm text-muted-foreground max-w-md">
            {t('common.unauthorizedDescription')}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto bg-background">
      <div className="mx-auto max-w-4xl px-6 py-6">
        <div className="mb-5 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">
              {t('datasets.settingsTitle')}
            </h1>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t('datasets.settingsDescription')}
            </p>
          </div>
          <Button
            className="h-9 gap-2"
            variant="default"
            disabled={updateDataset.isPending}
            onClick={handleSave}
          >
            <Save className="h-4 w-4" />
            {updateDataset.isPending ? t('datasets.saving') : t('datasets.save')}
          </Button>
        </div>

        <div className="space-y-5">
          <Card className="border-border/80 shadow-sm">
            <CardHeader className="space-y-1.5">
              <CardTitle className="text-base">{t('datasets.settings.basicInfo')}</CardTitle>
              <CardDescription>{t('datasets.settings.basicInfoDescription')}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-1.5">
                <Label htmlFor="name" className="text-sm font-medium leading-5">
                  {t('datasets.fieldName')}
                  <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="name"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder={t('datasets.settings.namePlaceholder')}
                  disabled={!dataset?.embedding_available}
                  aria-invalid={isNameValid ? 'false' : 'true'}
                  className={cn(
                    'h-9',
                    !isNameValid && 'border-destructive focus-visible:ring-destructive'
                  )}
                />
                {!isNameValid && (
                  <div className="text-xs text-destructive">
                    {(() => {
                      const code = nameErrors[0];
                      return code === 'required'
                        ? t('datasets.validation.name.required')
                        : code === 'tooShort'
                          ? t('datasets.validation.name.tooShort')
                          : code === 'tooLong'
                            ? t('datasets.validation.name.tooLong')
                            : code === 'invalidChars'
                              ? t('datasets.validation.name.invalidChars')
                              : t('datasets.validation.name.onlySpaces');
                    })()}
                  </div>
                )}
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="description" className="text-sm font-medium leading-5">
                  {t('datasets.fieldDescription')}
                </Label>
                <Textarea
                  id="description"
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  placeholder={t('datasets.settings.descriptionPlaceholder')}
                  disabled={!dataset?.embedding_available}
                  className="min-h-24"
                />
              </div>

              <IconInput value={iconValue} onChange={setIconValue} />

              {isGraphFlowEnabled ? (
                <div className="rounded-lg border border-border/70 bg-muted/30 p-4">
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary" className="rounded-full px-2 py-0.5 text-[11px]">
                      {t('datasets.graphFlowBadge')}
                    </Badge>
                    <span className="text-sm font-semibold">
                      {t('datasets.settings.graphFlowEnabledLabel')}
                    </span>
                  </div>
                  <p className="mt-2 max-w-[520px] text-sm leading-6 text-muted-foreground">
                    {t('datasets.settings.graphFlowEnabledDescription')}
                  </p>
                </div>
              ) : null}
            </CardContent>
          </Card>

          <Card className="border-border/80 shadow-sm">
            <CardHeader className="space-y-1.5">
              <CardTitle className="text-base">{t('datasets.hitTesting.configuration')}</CardTitle>
              <CardDescription>{t('datasets.hitTesting.configDescription')}</CardDescription>
            </CardHeader>
            <CardContent>
              <DatasetIndexingConfigForm
                ref={configFormRef}
                data={configData}
                onChange={handleConfigChange}
                isSettingMode
                currentDataset={dataset}
                className="space-y-5"
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
