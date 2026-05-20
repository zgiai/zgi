'use client';

import React, { useEffect, useMemo, useState, useCallback } from 'react';
import { BookOpen, ChevronLeft, Pencil } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { IconInput } from '@/components/common/icon-input';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import {
  useDefaultModelByUseCase,
  useInitializeDefaultModelByUseCase,
} from '@/hooks/model/use-default-model-by-use-case';
import { useCreateDataset, useUpdateDataset } from '@/hooks/dataset/use-datasets';
import { useT } from '@/i18n';
import type { Dataset } from '@/services/types/dataset';
import { buildIconValueFromDataset, iconValueToDatasetPayload } from '@/utils/icon-helpers';
import { isValidNameInput, getNameValidationErrors } from '@/utils/validation';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store/workspace-store';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import {
  EmbeddingSettings,
  GraphModelSettings,
  RetrievalSettings as RetrievalConfigCard,
  type RetrievalConfig,
} from '@/components/datasets/indexing-config';
import { normalizeDatasetSearchMethod } from '@/utils/dataset/retrieval-config';

interface CreateDatasetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentFolderId?: string; // optional for create in a folder
}

const DEFAULT_CREATE_RETRIEVAL_CONFIG: RetrievalConfig = {
  search_method: 'semantic_search',
  top_k: 4,
  score_threshold_enabled: true,
  score_threshold: 0.5,
  reranking_enable: false,
  reranking_model: { reranking_provider_name: '', reranking_model_name: '' },
};

// CreateDatasetDialog component for creating datasets.
function CreateDatasetDialog({ open, onOpenChange, currentFolderId }: CreateDatasetDialogProps) {
  const t = useT();

  const isEditMode = false;
  const dataset = undefined as Dataset | undefined;
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();
  const requiresWorkspaceSelection = isOrganizationMode && !isEditMode;

  // Hooks for create/edit operations
  const createDatasetMutation = useCreateDataset(currentFolderId);
  const updateDatasetMutation = useUpdateDataset(dataset?.id || '');
  const { value: defaultEmbeddingModel } = useDefaultModelByUseCase('embedding');

  // Initial icon value
  const initialIconValue: IconValue = useMemo(() => {
    if (isEditMode && dataset) {
      return buildIconValueFromDataset(dataset);
    }
    // Create mode default icon
    return createTextIconValue('', ICON_BG);
  }, [isEditMode, dataset]);

  // Unified form state
  const [iconValue, setIconValue] = useState<IconValue>(initialIconValue);
  const [formData, setFormData] = useState<{
    name: string;
    description: string;
    provider?: string;
    data_source_type?: string;
    indexing_technique?: string;
    embedding_model_provider?: string;
    embedding_model?: string;
    entity_model_provider?: string;
    entity_model?: string;
    enable_graph_flow: boolean;
  }>(() => {
    if (isEditMode && dataset) {
      return {
        name: dataset.name || '',
        description: dataset.description || '',
        entity_model_provider: dataset.entity_model_provider || '',
        entity_model: dataset.entity_model || '',
        enable_graph_flow: dataset.enable_graph_flow ?? false,
      };
    }
    return {
      name: '',
      description: '',
      provider: 'vendor',
      data_source_type: 'upload_file',
      indexing_technique: 'high_quality',
      embedding_model_provider: '',
      embedding_model: '',
      entity_model_provider: '',
      entity_model: '',
      enable_graph_flow: false,
    };
  });
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const [retrievalConfig, setRetrievalConfig] = useState<RetrievalConfig>(
    DEFAULT_CREATE_RETRIEVAL_CONFIG
  );

  const [hasManuallySetSearchMethod, setHasManuallySetSearchMethod] = useState(false);
  // Track if form has been submitted to show validation errors only after submit
  const [hasSubmitted, setHasSubmitted] = useState(false);
  const graphFlowEnabled = isEditMode
    ? Boolean(dataset?.enable_graph_flow)
    : formData.enable_graph_flow;

  // Reset icon and form when dataset changes or dialog opens
  useEffect(() => {
    if (!open) return;
    // Reset submitted state when dialog opens
    setHasSubmitted(false);
    setHasManuallySetSearchMethod(false);
    if (isEditMode && dataset) {
      setFormData({
        name: dataset.name || '',
        description: dataset.description || '',
        entity_model_provider: dataset.entity_model_provider || '',
        entity_model: dataset.entity_model || '',
        enable_graph_flow: dataset.enable_graph_flow ?? false,
      });
      setRetrievalConfig({
        ...DEFAULT_CREATE_RETRIEVAL_CONFIG,
        ...(dataset.retrieval_config || {}),
        search_method: normalizeDatasetSearchMethod(
          dataset.retrieval_config?.search_method,
          Boolean(dataset.enable_graph_flow)
        ),
      });
      setIconValue(initialIconValue);
    } else {
      setFormData({
        name: '',
        description: '',
        provider: 'vendor',
        data_source_type: 'upload_file',
        indexing_technique: 'high_quality',
        embedding_model_provider: '',
        embedding_model: '',
        entity_model_provider: '',
        entity_model: '',
        enable_graph_flow: false,
      });
      setRetrievalConfig(DEFAULT_CREATE_RETRIEVAL_CONFIG);
      setIconValue(createTextIconValue('', ICON_BG));
      setSelectedWorkspace(undefined);
    }
  }, [dataset, open, isEditMode, initialIconValue]);

  // Default embedding model for create mode
  useInitializeDefaultModelByUseCase({
    useCase: 'embedding',
    currentModel: {
      provider: formData.embedding_model_provider,
      model: formData.embedding_model,
    },
    enabled: open && !isEditMode,
    onInitialize: v => {
      setFormData(prev => ({
        ...prev,
        embedding_model_provider: v.provider,
        embedding_model: v.model,
      }));
    },
  });

  // Default graph model when graph flow is enabled
  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: {
      provider: formData.entity_model_provider,
      model: formData.entity_model,
    },
    enabled: open && graphFlowEnabled,
    onInitialize: v => {
      setFormData(prev => ({
        ...prev,
        entity_model_provider: v.provider,
        entity_model: v.model,
      }));
    },
  });

  // Typed change handler
  function handleInputChange<K extends keyof typeof formData>(
    field: K,
    value: (typeof formData)[K]
  ) {
    setFormData(prev => ({ ...prev, [field]: value }));
  }

  const handleGraphFlowChange = useCallback(
    (checked: boolean) => {
      setFormData(prev => ({
        ...prev,
        enable_graph_flow: checked,
      }));

      if (!checked) {
        setRetrievalConfig(prev => ({
          ...prev,
          search_method: 'semantic_search',
        }));
        return;
      }

      if (!isEditMode && !hasManuallySetSearchMethod) {
        setRetrievalConfig(prev => ({
          ...prev,
          search_method: 'graph_search',
        }));
      }
    },
    [hasManuallySetSearchMethod, isEditMode]
  );

  // Validate name: 2-32 Unicode chars; letters, numbers, underscore, hyphen, optional spaces
  const isNameValid = useMemo(
    () => isValidNameInput(formData.name, { allowSpace: true }),
    [formData.name]
  );
  const nameErrors = useMemo(
    () => getNameValidationErrors(formData.name, { allowSpace: true }),
    [formData.name]
  );
  const isEmbeddingModelValid = useMemo(
    () => isEditMode || Boolean(formData.embedding_model_provider && formData.embedding_model),
    [isEditMode, formData.embedding_model_provider, formData.embedding_model]
  );
  const isGraphModelValid = useMemo(
    () => !graphFlowEnabled || Boolean(formData.entity_model_provider && formData.entity_model),
    [graphFlowEnabled, formData.entity_model_provider, formData.entity_model]
  );

  // Validate form and show toast for errors
  const validateForm = useCallback((): boolean => {
    const workspaceId = requiresWorkspaceSelection
      ? selectedWorkspace?.id || ''
      : currentWorkspace?.id || dataset?.workspace_id || dataset?.workspace?.id || '';

    // Check name validation
    if (!isNameValid) {
      const errorKey = nameErrors[0] || 'required';
      toast.error(t(`datasets.validation.name.${errorKey}`));
      return false;
    }

    // Check embedding model (create mode only)
    if (!isEmbeddingModelValid) {
      toast.error(t('datasets.validation.embeddingModel.required'));
      return false;
    }

    if (!isGraphModelValid) {
      toast.error(t('datasets.validation.graphModel.required'));
      return false;
    }

    // Check workspace
    if (!workspaceId) {
      toast.error(t('datasets.validation.workspace.required'));
      return false;
    }

    return true;
  }, [
    isNameValid,
    nameErrors,
    isEmbeddingModelValid,
    isGraphModelValid,
    currentWorkspace?.id,
    dataset?.workspace_id,
    dataset?.workspace?.id,
    requiresWorkspaceSelection,
    selectedWorkspace?.id,
    t,
  ]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Mark form as submitted to show validation errors
    setHasSubmitted(true);

    // Validate form and show toast for errors
    if (!validateForm()) {
      return;
    }

    const workspaceId = requiresWorkspaceSelection
      ? selectedWorkspace?.id || ''
      : currentWorkspace?.id || dataset?.workspace_id || dataset?.workspace?.id || '';

    const iconPayload = iconValueToDatasetPayload(iconValue, {
      existing: isEditMode ? dataset : undefined,
      defaultTextFromName: formData.name,
    });
    const normalizedSearchMethod = normalizeDatasetSearchMethod(
      retrievalConfig.search_method,
      graphFlowEnabled
    );

    try {
      if (isEditMode && dataset) {
        await updateDatasetMutation.mutateAsync({
          name: formData.name,
          description: formData.description,
          enable_graph_flow: Boolean(dataset.enable_graph_flow),
          ...(graphFlowEnabled
            ? {
                entity_model_provider: formData.entity_model_provider || '',
                entity_model: formData.entity_model || '',
              }
            : {}),
          workspace_id: workspaceId,
          ...iconPayload,
        });
        onOpenChange(false);
      } else {
        await createDatasetMutation.mutateAsync({
          name: formData.name,
          description: formData.description,
          provider: formData.provider || 'vendor',
          data_source_type: formData.data_source_type || 'upload_file',
          indexing_technique: formData.indexing_technique || 'high_quality',
          embedding_model_provider: formData.embedding_model_provider || '',
          embedding_model: formData.embedding_model || '',
          enable_graph_flow: graphFlowEnabled,
          ...(graphFlowEnabled
            ? {
                entity_model_provider: formData.entity_model_provider || '',
                entity_model: formData.entity_model || '',
              }
            : {}),
          workspace_id: workspaceId,
          ...(currentFolderId ? { folder_id: currentFolderId } : {}),
          ...iconPayload,
          retrieval_config: {
            search_method: normalizedSearchMethod,
            top_k: retrievalConfig.top_k,
            score_threshold: retrievalConfig.score_threshold,
            score_threshold_enabled: retrievalConfig.score_threshold_enabled,
            reranking_enable: retrievalConfig.reranking_enable,
            reranking_model: retrievalConfig.reranking_model,
          },
        });
        onOpenChange(false);
        // Reset create form
        setFormData(prev => ({
          ...prev,
          name: '',
          description: '',
          provider: 'vendor',
          data_source_type: 'upload_file',
          indexing_technique: 'high_quality',
          embedding_model_provider: defaultEmbeddingModel?.provider || '',
          embedding_model: defaultEmbeddingModel?.model || '',
          entity_model_provider: '',
          entity_model: '',
          enable_graph_flow: false,
        }));
        setRetrievalConfig(DEFAULT_CREATE_RETRIEVAL_CONFIG);
        setIconValue(createTextIconValue('', ICON_BG));
        setSelectedWorkspace(undefined);
      }
    } catch (error) {
      console.error('CreateDatasetDialog submit error:', error);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="default"
        className="max-h-[85vh] p-0 overflow-hidden"
        aria-describedby={undefined}
      >
        <DialogHeader className="pb-2">
          <DialogTitle className="flex items-center gap-2.5 text-xl font-bold tracking-tight">
            {isEditMode ? (
              <Pencil className="size-5 text-primary" />
            ) : (
              <BookOpen className="size-5 text-primary" />
            )}
            {isEditMode ? t('datasets.actions.edit') : t('datasets.createModal.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 pt-4">
          <form id="dataset-form" onSubmit={handleSubmit} className="space-y-6">
            <div className="space-y-6">
              {/* Dataset Name */}
              <div className="space-y-2.5">
                <Label className="flex items-center gap-1 text-sm font-semibold">
                  {t('datasets.createModal.nameLabel')} <span className="text-destructive">*</span>
                </Label>
                <Input
                  placeholder={t('datasets.createModal.namePlaceholder')}
                  value={formData.name}
                  onChange={e => handleInputChange('name', e.target.value)}
                  required
                  className={cn(
                    'h-11',
                    hasSubmitted &&
                      !isNameValid &&
                      'border-destructive focus-visible:ring-destructive'
                  )}
                />
                {hasSubmitted && !isNameValid && (
                  <p className="text-xs text-destructive font-medium animate-in fade-in slide-in-from-top-1">
                    {t(`datasets.validation.name.${nameErrors[0] || 'required'}`)}
                  </p>
                )}
              </div>

              {/* Description */}
              <div className="space-y-2.5">
                <Label className="text-sm font-semibold">
                  {t('datasets.createModal.descriptionLabel')}
                </Label>
                <Textarea
                  placeholder={t('datasets.createModal.descriptionPlaceholder')}
                  value={formData.description}
                  onChange={e => handleInputChange('description', e.target.value)}
                  rows={3}
                  className="resize-none min-h-[100px]"
                />
              </div>

              {requiresWorkspaceSelection ? (
                <div className="space-y-2.5">
                  <Label className="flex items-center gap-1 text-sm font-semibold">
                    {t('datasets.createModal.workspaceLabel')}{' '}
                    <span className="text-destructive">*</span>
                  </Label>
                  <WorkspaceSelector
                    value={selectedWorkspace}
                    placeholder={t('datasets.createModal.workspacePlaceholder')}
                    autoSelectFirst
                    onChange={workspace => setSelectedWorkspace(workspace)}
                  />
                  {hasSubmitted && !selectedWorkspace?.id ? (
                    <p className="text-xs text-destructive font-medium animate-in fade-in slide-in-from-top-1">
                      {t('datasets.validation.workspace.required')}
                    </p>
                  ) : null}
                </div>
              ) : null}

              {!isEditMode ? (
                <div className="flex items-center justify-between rounded-xl border border-border/50 bg-muted/20 p-4 transition-colors hover:bg-muted/30">
                  <div className="space-y-0.5">
                    <Label
                      className="cursor-pointer text-sm font-semibold"
                      htmlFor="enable-graph-flow"
                    >
                      {t('datasets.createModal.enableGraphFlowLabel')}
                    </Label>
                    <p className="max-w-[320px] text-xs text-muted-foreground">
                      {t('datasets.createModal.enableGraphFlowDescription')}
                    </p>
                  </div>
                  <Switch
                    id="enable-graph-flow"
                    checked={formData.enable_graph_flow}
                    onCheckedChange={handleGraphFlowChange}
                  />
                </div>
              ) : null}

              {isEditMode && graphFlowEnabled ? (
                <div className="rounded-xl border border-border/50 bg-muted/20 p-4">
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary" className="rounded-full px-2 py-0.5 text-[11px]">
                      {t('datasets.graphFlowBadge')}
                    </Badge>
                    <span className="text-sm font-semibold">
                      {t('datasets.settings.graphFlowEnabledLabel')}
                    </span>
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {t('datasets.settings.graphFlowEnabledDescription')}
                  </p>
                </div>
              ) : null}

              {graphFlowEnabled ? (
                <GraphModelSettings
                  graphModel={{
                    provider: formData.entity_model_provider || '',
                    model: formData.entity_model || '',
                  }}
                  onChange={graphModel => {
                    setFormData(prev => ({
                      ...prev,
                      entity_model_provider: graphModel.provider,
                      entity_model: graphModel.model,
                    }));
                  }}
                  required
                  title={t('datasets.createModal.graphModelLabel')}
                  description={t('datasets.createModal.graphModelDescription')}
                  placeholder={t('datasets.createModal.graphModelPlaceholder')}
                  hasError={hasSubmitted && !isGraphModelValid}
                  errorMessage={
                    hasSubmitted && !isGraphModelValid
                      ? t('datasets.validation.graphModel.required')
                      : undefined
                  }
                />
              ) : null}

              {/* Embedding Model Selector */}
              {!isEditMode && (
                <EmbeddingSettings
                  embeddingModel={{
                    provider: formData.embedding_model_provider || '',
                    model: formData.embedding_model || '',
                  }}
                  onChange={embeddingModel => {
                    setFormData(prev => ({
                      ...prev,
                      embedding_model_provider: embeddingModel.provider,
                      embedding_model: embeddingModel.model,
                    }));
                  }}
                  required
                  title={t('datasets.createModal.embeddingModelLabel')}
                  placeholder={t('datasets.createModal.embeddingModelPlaceholder')}
                  hasError={hasSubmitted && !isEmbeddingModelValid}
                  errorMessage={
                    hasSubmitted && !isEmbeddingModelValid
                      ? t('datasets.validation.embeddingModel.required')
                      : undefined
                  }
                />
              )}

              {/* Advanced Settings */}
              <div className="space-y-3">
                <button
                  type="button"
                  onClick={() => setShowAdvancedSettings(!showAdvancedSettings)}
                  className="flex items-center gap-2 text-sm font-semibold hover:text-primary transition-colors focus:outline-none"
                >
                  <ChevronLeft
                    className={cn(
                      'size-4 transition-transform duration-300',
                      showAdvancedSettings ? '-rotate-90' : ''
                    )}
                  />
                  {t('datasets.createModal.advancedSettingsLabel')}
                </button>

                <div
                  className={cn(
                    'overflow-hidden transition-all duration-300 ease-in-out',
                    showAdvancedSettings ? 'max-h-[800px] opacity-100 pt-1' : 'max-h-0 opacity-0'
                  )}
                >
                  <div className="space-y-4">
                    <div className="space-y-2.5">
                      <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                        {t('datasets.createModal.iconLabel')}
                      </Label>
                      <IconInput
                        value={iconValue}
                        defaultValue={createTextIconValue(
                          (formData.name || '').slice(0, 2).toUpperCase() || ICON_TEXT,
                          ICON_BG
                        )}
                        onChange={setIconValue}
                      />
                    </div>

                    {/* Retrieval Config - create mode only */}
                    {!isEditMode && (
                      <div className="space-y-2.5">
                        <Label className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                          {t('datasets.createModal.retrievalConfigLabel')}
                        </Label>
                        <RetrievalConfigCard
                          retrieval={retrievalConfig}
                          isGraphEnabled={graphFlowEnabled}
                          onChange={(config: RetrievalConfig) => {
                            if (config.search_method !== retrievalConfig.search_method) {
                              setHasManuallySetSearchMethod(true);
                            }
                            setRetrievalConfig(config);
                          }}
                        />
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </form>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50">
          <Button
            form="dataset-form"
            type="submit"
            size="lg"
            className="px-10 font-bold"
            disabled={
              isEditMode ? updateDatasetMutation.isPending : createDatasetMutation.isPending
            }
          >
            {isEditMode
              ? t('common.confirm')
              : createDatasetMutation.isPending
                ? t('datasets.createModal.creatingButton')
                : t('datasets.createModal.createButton')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default CreateDatasetDialog;
