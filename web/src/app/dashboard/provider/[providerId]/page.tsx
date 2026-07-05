'use client';

import React, { useState, useCallback, useEffect } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import {
  useBatchToggleModels,
  useConfigureModel,
  useCreateCustomModel,
  useDeleteCustomModel,
  useProviderModelsAll,
  useToggleModel,
  useUpdateCustomModel,
} from '@/hooks/model/use-model';
import {
  useProvider,
  useToggleProvider,
  useUpdateCustomProvider,
  useDeleteCustomProvider,
} from '@/hooks/provider/use-provider';
import type {
  ModelItem,
  ModelUseCase,
  CreateCustomModelRequest,
  UpdateCustomModelRequest,
} from '@/services/types/model';
import type { UpdateCustomProviderRequest } from '@/services/types/provider';
import { ShieldCheck, Puzzle } from 'lucide-react';
import ModelsActionsBar from '@/components/providers/models-actions-bar';
import ModelTypeChips from '@/components/providers/model-type-chips';
import ModelsGroupTable from '@/components/providers/models-group-table';
import ProviderPageHeader from '@/components/providers/provider-page-header';
import { CustomProviderDialog } from '@/components/providers/custom-provider-dialog';
import { CustomModelDialog } from '@/components/providers/custom-model-dialog';
import { ModelPriceDialog } from '@/components/providers/model-price-dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useT } from '@/i18n';
import { useProviderDisplay } from '@/hooks/provider/use-provider-display';
import { IS_CLOUD } from '@/lib/config';
import { ModelUpdatesButton } from '@/components/providers/model-updates-button';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useOrganizationStore } from '@/store/organization-store';
import { toast } from 'sonner';

// Removed local badge color mapping; row badges use mapping inside ModelsGroupTable

export default function ModelPage() {
  const params = useParams<{ providerId: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const providerId = Array.isArray(params?.providerId) ? params?.providerId[0] : params?.providerId;
  const provider = decodeURIComponent(providerId || '');
  const t = useT();
  const getProviderName = useProviderI18n();

  // Search state
  const [query, setQuery] = useState('');

  // Data hooks
  const { provider: detail } = useProvider(provider);
  const [selectedUseCase, setSelectedUseCase] = useState<ModelUseCase | null>(null);
  const { models: allModels, isLoading, isFetching: _isFetching } = useProviderModelsAll(provider);

  const { updateCustomProvider, isUpdating } = useUpdateCustomProvider();
  const { deleteCustomProvider, isDeleting } = useDeleteCustomProvider();

  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

  // Custom Model State
  const { createCustomModel, isCreating: isCreatingModel } = useCreateCustomModel();
  const { configureModel, isConfiguring } = useConfigureModel();
  const { updateCustomModel: updateModelAction, isUpdating: isUpdatingModel } =
    useUpdateCustomModel();
  const { deleteCustomModel: deleteModelAction, isDeleting: isDeletingModel } =
    useDeleteCustomModel();

  const [isModelDialogOpen, setIsModelDialogOpen] = useState(false);
  const [editingModel, setEditingModel] = useState<ModelItem | null>(null);
  const [isModelDeleteConfirmOpen, setIsModelDeleteConfirmOpen] = useState(false);
  const [deletingModel, setDeletingModel] = useState<ModelItem | null>(null);
  const [isPriceDialogOpen, setIsPriceDialogOpen] = useState(false);
  const [pricingModel, setPricingModel] = useState<ModelItem | null>(null);
  const openedPricingQueryRef = React.useRef<string | null>(null);

  // Frontend filtering by search query (name, display_name)
  const models = React.useMemo(() => {
    if (!query.trim()) return allModels;
    const keyword = query.trim().toLowerCase();
    return allModels.filter(
      m =>
        m.model.toLowerCase().includes(keyword) ||
        (m.model_name && m.model_name.toLowerCase().includes(keyword))
    );
  }, [allModels, query]);
  // Selection state for batch operations (must be defined before derived memos)
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const selectedCount = selected.size;
  const officialVisible = React.useMemo(
    () =>
      models
        .filter(m => m.is_available)
        .filter(m => (selectedUseCase === null ? true : m.use_cases?.includes(selectedUseCase))),
    [models, selectedUseCase]
  );
  const extensibleVisible = React.useMemo(
    () =>
      models
        .filter(m => !m.is_available)
        .filter(m => (selectedUseCase === null ? true : m.use_cases?.includes(selectedUseCase))),
    [models, selectedUseCase]
  );
  const isAllOfficialSelected = React.useMemo(
    () => officialVisible.length > 0 && officialVisible.every(m => selected.has(m.model)),
    [officialVisible, selected]
  );
  const isSomeOfficialSelected = React.useMemo(
    () => officialVisible.some(m => selected.has(m.model)),
    [officialVisible, selected]
  );
  const officialSelectedCount = React.useMemo(
    () => officialVisible.reduce((acc, m) => acc + (selected.has(m.model) ? 1 : 0), 0),
    [officialVisible, selected]
  );
  const isAllExtensibleSelected = React.useMemo(
    () => extensibleVisible.length > 0 && extensibleVisible.every(m => selected.has(m.model)),
    [extensibleVisible, selected]
  );
  const isSomeExtensibleSelected = React.useMemo(
    () => extensibleVisible.some(m => selected.has(m.model)),
    [extensibleVisible, selected]
  );
  const extensibleSelectedCount = React.useMemo(
    () => extensibleVisible.reduce((acc, m) => acc + (selected.has(m.model) ? 1 : 0), 0),
    [extensibleVisible, selected]
  );
  const availableUseCases = React.useMemo(() => {
    const set = new Set<ModelUseCase>();
    models.forEach(m => m.use_cases?.forEach(uc => set.add(uc)));
    return set;
  }, [models]);
  const visibleModelKeys = React.useMemo(
    () => new Set([...officialVisible, ...extensibleVisible].map(model => model.model)),
    [extensibleVisible, officialVisible]
  );
  const toggleableVisibleModels = React.useMemo(
    () => officialVisible.filter(model => model.is_configured !== false),
    [officialVisible]
  );
  const hasActiveFilters = Boolean(query.trim() || selectedUseCase !== null);
  useEffect(() => {
    // Clear selected use case if it's no longer available
    if (selectedUseCase !== null && !availableUseCases.has(selectedUseCase)) {
      setSelectedUseCase(null);
    }
  }, [availableUseCases, selectedUseCase]);
  useEffect(() => {
    setSelected(prev => {
      let changed = false;
      const next = new Set<string>();

      prev.forEach(modelName => {
        if (visibleModelKeys.has(modelName)) {
          next.add(modelName);
        } else {
          changed = true;
        }
      });

      return changed ? next : prev;
    });
  }, [visibleModelKeys]);
  const { toggleModel } = useToggleModel();
  const { toggleProvider } = useToggleProvider();
  const { toggleBatchModels, isBatchToggling } = useBatchToggleModels();
  const isCustom = detail?.provider_type === 'custom';
  const { name, description } = useProviderDisplay(detail);
  const accountPermissions = useAccountPermissions();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const organizationRole =
    accountPermissions.organizationRole ?? currentOrganization?.organization_role ?? null;
  const canManageModels = organizationRole === 'owner' || organizationRole === 'admin';
  const isModelPermissionLoading =
    !organizationRole && (accountPermissions.isLoading || accountPermissions.isFetching);

  // Track which item is being toggled (only disable that specific switch)
  const [togglingModel, setTogglingModel] = useState<string | null>(null);
  const [togglingProvider, setTogglingProvider] = useState(false);

  // Non-paginated view: no infinite scroll

  const onToggleModel = useCallback(
    async (m: ModelItem, next: boolean) => {
      if (!canManageModels) return;
      setTogglingModel(m.model);
      try {
        await toggleModel(provider, m.model, next);
      } finally {
        setTogglingModel(null);
      }
    },
    [canManageModels, provider, toggleModel]
  );

  const onToggleProvider = useCallback(
    async (next: boolean) => {
      setTogglingProvider(true);
      try {
        await toggleProvider(provider, next);
      } finally {
        setTogglingProvider(false);
      }
    },
    [provider, toggleProvider]
  );

  const onSelectRow = useCallback((modelName: string, next: boolean) => {
    setSelected(prev => {
      const nextSet = new Set(prev);
      if (next) nextSet.add(modelName);
      else nextSet.delete(modelName);
      return nextSet;
    });
  }, []);

  const clearSelection = useCallback(() => setSelected(new Set<string>()), []);

  const onBatchEnableDisable = useCallback(
    async (next: boolean) => {
      if (!canManageModels || !provider || selected.size === 0) return;
      try {
        await toggleBatchModels(provider, Array.from(selected), next);
      } finally {
        clearSelection();
      }
    },
    [canManageModels, provider, selected, toggleBatchModels, clearSelection]
  );

  const onToggleVisibleModels = useCallback(
    async (next: boolean) => {
      if (!canManageModels || !provider || toggleableVisibleModels.length === 0) return;

      await toggleBatchModels(
        provider,
        toggleableVisibleModels.map(model => model.model),
        next
      );

      clearSelection();
    },
    [canManageModels, clearSelection, provider, toggleableVisibleModels, toggleBatchModels]
  );

  const handleAdd = useCallback(() => {
    if (!canManageModels) return;
    if (isCustom) {
      setEditingModel(null);
      setIsModelDialogOpen(true);
      return;
    }

    router.push(`/dashboard/channel?create=1&provider=${encodeURIComponent(provider)}`);
  }, [canManageModels, isCustom, provider, router]);

  const handleUpdate = async (data: UpdateCustomProviderRequest) => {
    if (detail) {
      await updateCustomProvider(detail.id, data);
      setIsEditDialogOpen(false);
    }
  };

  const handleDelete = async () => {
    if (detail) {
      await deleteCustomProvider(detail.id);
      setIsDeleteDialogOpen(false);
      router.push('/dashboard/provider');
    }
  };

  const handleModelSubmit = async (data: CreateCustomModelRequest | UpdateCustomModelRequest) => {
    if (!canManageModels) return;
    if (editingModel) {
      // For updates, the model name (slug) often remains the same, but we need the model ID if it was changed
      // Documentation says /llm/models/custom/{id}
      await updateModelAction(editingModel.id, data as UpdateCustomModelRequest);
    } else {
      await createCustomModel(data as CreateCustomModelRequest);
    }
    setIsModelDialogOpen(false);
    setEditingModel(null);
  };

  const openPriceDialog = useCallback(
    (model: ModelItem) => {
      if (!canManageModels) {
        toast.warning(t('aiProviders.models.priceDialog.adminRequired'));
        return;
      }
      setPricingModel(model);
      setIsPriceDialogOpen(true);
    },
    [canManageModels, t]
  );

  useEffect(() => {
    const shouldOpen =
      searchParams.get('pricing') === '1' || searchParams.get('open') === 'pricing';
    const targetModel = searchParams.get('model');
    if (!shouldOpen || !targetModel || allModels.length === 0) return;

    const match = allModels.find(model => model.id === targetModel || model.model === targetModel);
    if (!match) return;
    if (!organizationRole || (!canManageModels && isModelPermissionLoading)) return;

    const openKey = `${match.id}:${targetModel}`;
    if (openedPricingQueryRef.current === openKey) return;
    openedPricingQueryRef.current = openKey;
    if (!canManageModels) {
      toast.warning(t('aiProviders.models.priceDialog.adminRequired'));
      return;
    }
    openPriceDialog(match);
  }, [
    allModels,
    canManageModels,
    isModelPermissionLoading,
    openPriceDialog,
    organizationRole,
    searchParams,
    t,
  ]);

  const handlePriceSubmit = useCallback(
    async (values: { inputPrice: string; outputPrice: string }) => {
      if (!pricingModel || !canManageModels) return;

      if (isCustom) {
        await updateModelAction(pricingModel.id, {
          input_price: values.inputPrice,
          output_price: values.outputPrice,
        });
      } else {
        await configureModel({
          model_id: pricingModel.id,
          is_enabled: pricingModel.is_enabled,
          input_price_override: values.inputPrice,
          output_price_override: values.outputPrice,
        });
      }

      setIsPriceDialogOpen(false);
      setPricingModel(null);
    },
    [canManageModels, configureModel, isCustom, pricingModel, updateModelAction]
  );

  const handleDeleteModel = async () => {
    if (deletingModel && canManageModels) {
      await deleteModelAction(deletingModel.id);
      setIsModelDeleteConfirmOpen(false);
      setDeletingModel(null);
    }
  };

  if (isLoading && !detail) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Skeleton className="w-10 h-10 rounded-full" />
            <div>
              <Skeleton className="h-4 w-40" />
              <Skeleton className="h-3 w-64 mt-2" />
            </div>
          </div>
          <Skeleton className="h-6 w-12" />
        </div>
        <div className="flex gap-3">
          <Skeleton className="h-9 w-80" />
        </div>
        <div className="border rounded-lg">
          <Skeleton className="h-72" />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ProviderPageHeader
        providerId={detail?.provider}
        displayName={name}
        description={
          description ||
          t('aiProviders.management.description', {
            provider: name,
          })
        }
        isEnabled={Boolean(detail?.is_enabled)}
        onToggle={onToggleProvider}
        toggling={togglingProvider}
        onEdit={isCustom ? () => setIsEditDialogOpen(true) : undefined}
        onDelete={isCustom ? () => setIsDeleteDialogOpen(true) : undefined}
      />

      <div className="rounded-xl border border-border/70 bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
        <span className="font-medium text-foreground">
          {t('aiProviders.management.strategyHint')}
        </span>{' '}
        {t('aiProviders.management.strategyDescription')}
      </div>

      <div className="space-y-2">
        <ModelsActionsBar
          totalCount={models.length}
          visibleCount={officialVisible.length + extensibleVisible.length}
          query={query}
          onQueryChange={setQuery}
          onEnableVisible={() => onToggleVisibleModels(true)}
          onDisableVisible={() => onToggleVisibleModels(false)}
          extraActions={
            canManageModels && !IS_CLOUD && !isCustom ? (
              <ModelUpdatesButton providerId={provider} />
            ) : undefined
          }
          onAdd={canManageModels ? handleAdd : undefined}
          addLabel={
            isCustom
              ? (t('aiProviders.models.actions.add') as string)
              : (t('aiProviders.models.actions.addChannel') as string)
          }
          disabled={!canManageModels || isBatchToggling}
          hasActiveFilters={hasActiveFilters}
        />
        <ModelTypeChips
          availableTypes={availableUseCases}
          selectedType={selectedUseCase}
          onSelect={setSelectedUseCase}
        />
      </div>

      <ModelsGroupTable
        title={t('aiProviders.models.groups.official')}
        tooltip={t('aiProviders.models.tooltips.official')}
        IconSlot={
          <span className="inline-flex items-center justify-center size-6 rounded-md bg-accent/60">
            <ShieldCheck className="w-4 h-4 text-green-600" />
          </span>
        }
        groupType="official"
        models={officialVisible}
        selected={selected}
        onSelectRow={onSelectRow}
        headerAllSelected={isAllOfficialSelected}
        headerSomeSelected={isSomeOfficialSelected}
        onHeaderToggle={() => {
          const names = officialVisible.map(m => m.model);
          setSelected(prev => {
            const nextSet = new Set(prev);
            const allSelected = officialSelectedCount === officialVisible.length;
            if (allSelected) names.forEach(n => nextSet.delete(n));
            else names.forEach(n => nextSet.add(n));
            return nextSet;
          });
        }}
        isLoading={isLoading}
        isTogglingAll={isBatchToggling}
        isBatchToggling={isBatchToggling}
        togglingModel={togglingModel}
        onToggleModel={onToggleModel}
        onEditPrice={canManageModels ? openPriceDialog : undefined}
        searchQuery={query}
        hasTypeFilter={selectedUseCase !== null}
        onClearFilters={() => {
          setSelectedUseCase(null);
          setQuery('');
        }}
        onEditModel={
          isCustom && canManageModels
            ? m => {
                setEditingModel(m);
                setIsModelDialogOpen(true);
              }
            : undefined
        }
        onDeleteModel={
          isCustom && canManageModels
            ? m => {
                setDeletingModel(m);
                setIsModelDeleteConfirmOpen(true);
              }
            : undefined
        }
        readOnly={!canManageModels}
        isCustom
      />

      <ModelsGroupTable
        title={t('aiProviders.models.groups.extensible')}
        tooltip={t('aiProviders.models.tooltips.extensible')}
        IconSlot={
          <span className="inline-flex items-center justify-center size-6 rounded-md bg-accent/60">
            <Puzzle className="w-4 h-4 text-blue-600" />
          </span>
        }
        groupType="extensible"
        models={extensibleVisible}
        selected={selected}
        onSelectRow={onSelectRow}
        headerAllSelected={isAllExtensibleSelected}
        headerSomeSelected={isSomeExtensibleSelected}
        onHeaderToggle={() => {
          const names = extensibleVisible.map(m => m.model);
          setSelected(prev => {
            const nextSet = new Set(prev);
            const allSelected = extensibleSelectedCount === extensibleVisible.length;
            if (allSelected) names.forEach(n => nextSet.delete(n));
            else names.forEach(n => nextSet.add(n));
            return nextSet;
          });
        }}
        isLoading={isLoading}
        isTogglingAll={isBatchToggling}
        isBatchToggling={isBatchToggling}
        togglingModel={togglingModel}
        onToggleModel={onToggleModel}
        onEditPrice={canManageModels ? openPriceDialog : undefined}
        searchQuery={query}
        hasTypeFilter={selectedUseCase !== null}
        readOnly
        onClearFilters={() => {
          setSelectedUseCase(null);
          setQuery('');
        }}
        onEditModel={
          isCustom && canManageModels
            ? m => {
                setEditingModel(m);
                setIsModelDialogOpen(true);
              }
            : undefined
        }
        onDeleteModel={
          isCustom && canManageModels
            ? m => {
                setDeletingModel(m);
                setIsModelDeleteConfirmOpen(true);
              }
            : undefined
        }
        onCreateModel={
          isCustom && canManageModels
            ? () => {
                setEditingModel(null);
                setIsModelDialogOpen(true);
              }
            : undefined
        }
      />

      {/* Floating action bar for batch operations */}
      {canManageModels && selectedCount > 0 && (
        <div className="fixed bottom-6 left-6 right-6 z-40">
          <div className="mx-auto max-w-5xl rounded-xl border bg-background/95 backdrop-blur-sm p-4 shadow-xl flex items-center justify-between">
            <div className="text-sm font-medium">
              {t('aiProviders.models.selectedCount', {
                count: selectedCount,
                plural: selectedCount > 1 ? 's' : '',
              })}
            </div>
            <div className="flex items-center gap-3">
              <Button
                size="sm"
                onClick={() => onBatchEnableDisable(true)}
                disabled={isBatchToggling}
              >
                {t('aiProviders.models.actions.enableSelected')}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => onBatchEnableDisable(false)}
                disabled={isBatchToggling}
              >
                {t('aiProviders.models.actions.disableSelected')}
              </Button>
              <div className="w-px h-5 bg-accent" />
              <Button size="sm" variant="ghost" onClick={clearSelection} disabled={isBatchToggling}>
                {t('aiProviders.models.actions.clearSelection')}
              </Button>
            </div>
          </div>
        </div>
      )}

      <CustomProviderDialog
        open={isEditDialogOpen}
        onOpenChange={setIsEditDialogOpen}
        initialData={detail}
        onSubmit={handleUpdate}
        isSubmitting={isUpdating}
      />

      <ConfirmDialog
        variant="danger"
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
        title={t('aiProviders.custom.delete.title')}
        description={t('aiProviders.custom.delete.content', {
          name: getProviderName(detail?.provider, detail?.provider_name),
        })}
        confirmText={
          isDeleting
            ? (t('aiProviders.actions.saving') as string)
            : (t('aiProviders.custom.delete.confirm') as string)
        }
        cancelText={t('aiProviders.custom.delete.cancel') as string}
        onConfirm={handleDelete}
        loading={isDeleting}
      />

      <CustomModelDialog
        open={isModelDialogOpen}
        onOpenChange={v => {
          setIsModelDialogOpen(v);
          if (!v) setEditingModel(null);
        }}
        providerId={provider}
        initialData={editingModel || undefined}
        onSubmit={handleModelSubmit}
        isSubmitting={isCreatingModel || isUpdatingModel}
      />

      <ModelPriceDialog
        open={isPriceDialogOpen}
        onOpenChange={open => {
          setIsPriceDialogOpen(open);
          if (!open) setPricingModel(null);
        }}
        model={pricingModel}
        onSubmit={handlePriceSubmit}
        isSubmitting={isConfiguring || isUpdatingModel}
      />

      <ConfirmDialog
        variant="danger"
        open={isModelDeleteConfirmOpen}
        onOpenChange={setIsModelDeleteConfirmOpen}
        title={t('aiProviders.customModel.delete.title')}
        description={t('aiProviders.customModel.delete.content', {
          name: deletingModel?.model_name || deletingModel?.model || '',
        })}
        confirmText={
          isDeletingModel
            ? (t('aiProviders.actions.saving') as string)
            : (t('aiProviders.customModel.delete.confirm') as string)
        }
        cancelText={t('aiProviders.customModel.delete.cancel') as string}
        onConfirm={handleDeleteModel}
        loading={isDeletingModel}
      />
    </div>
  );
}
