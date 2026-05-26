'use client';

import React, { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { useQuery } from '@tanstack/react-query';
import { modelService } from '@/services/model.service';
import type { ApiResponseData } from '@/services/types/common';
import type { ModelItem, ModelList } from '@/services/types/model';

import { Search, X, ChevronDown, Info } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { ProviderIcon } from '@/components/common/provider-icon';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ModelTooltipContent } from '@/components/model/model-tooltip-content';

type ModelSelectionPolicy = 'available' | 'catalog';

export interface ModelMultiSelectorProps {
  // Controlled selected model names
  value?: string[];
  // Change callback with selected model names
  onChange?: (models: string[]) => void;
  // Placeholder text
  placeholder?: string;
  // Disable control
  disabled?: boolean;
  // Filter by enabled status
  isEnabled?: boolean;
  className?: string;
  // Number of columns per row for model list (default: 3)
  columns?: 2 | 3 | 4;
  preferredProvider?: string;
  autoCollapseOthers?: boolean;
  providerFilter?: string;
  onSelectionMetaChange?: (models: ModelItem[]) => void;
  supplementalModels?: ModelItem[];
  selectionPolicy?: ModelSelectionPolicy;
}

// Group models by provider
interface ProviderGroup {
  provider: string;
  models: ModelItem[];
}

/**
 * ModelMultiSelector - Panel style multi-select with chips, search, and provider grouping.
 * Fetches all models from all providers and groups them by provider.
 */
// Map columns to Tailwind grid class
const COLUMNS_CLASS: Record<2 | 3 | 4, string> = {
  2: 'grid-cols-2',
  3: 'grid-cols-3',
  4: 'grid-cols-4',
};

function getModelSelectionKey(model: Pick<ModelItem, 'provider' | 'model'>): string {
  return `${model.provider || 'unknown'}\t${model.model}`;
}

function getModelNameFromSelectionKey(key: string): string {
  const separatorIndex = key.indexOf('\t');
  return separatorIndex >= 0 ? key.slice(separatorIndex + 1) : key;
}

function isModelSelectable(
  model: ModelItem,
  selectionPolicy: ModelSelectionPolicy,
  catalogModelKeys: ReadonlySet<string>
): boolean {
  if (selectionPolicy === 'catalog') {
    return catalogModelKeys.has(getModelSelectionKey(model));
  }

  return model.callable !== false && model.is_available !== false;
}

function ModelMultiSelectorBase({
  value,
  onChange,
  placeholder = '',
  disabled = false,
  isEnabled,
  className,
  columns = 3,
  preferredProvider,
  autoCollapseOthers = false,
  providerFilter,
  onSelectionMetaChange,
  supplementalModels = [],
  selectionPolicy = 'available',
}: ModelMultiSelectorProps): JSX.Element {
  const t = useT();

  const [search, setSearch] = useState('');

  // Track collapsed provider groups
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());

  // Uncontrolled selection state
  const [internalSelected, setInternalSelected] = useState<string[]>([]);
  const [selectedItemKeys, setSelectedItemKeys] = useState<Set<string>>(new Set());

  const controlled = value !== undefined;
  const selected = controlled ? (value as string[]) : internalSelected;
  const selectedSet = useMemo(() => new Set<string>(selected), [selected]);

  // Fetch all models at once without pagination
  const { data, isLoading } = useQuery<ApiResponseData<ModelList>>({
    queryKey: ['models', 'multi-selector', { is_enabled: isEnabled }],
    queryFn: () => modelService.getModels({ is_enabled: isEnabled, page_size: 1000 }),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const catalogItems = useMemo(() => data?.data?.items ?? [], [data]);

  const catalogModelKeys = useMemo(
    () =>
      new Set(
        [...catalogItems, ...supplementalModels].map(item => getModelSelectionKey(item))
      ),
    [catalogItems, supplementalModels]
  );

  const isSelectable = useCallback(
    (model: ModelItem) => isModelSelectable(model, selectionPolicy, catalogModelKeys),
    [catalogModelKeys, selectionPolicy]
  );

  const allItems = useMemo(() => {
    const merged = new Map<string, ModelItem>();
    catalogItems.forEach(item => {
      merged.set(getModelSelectionKey(item), item);
    });
    supplementalModels.forEach(item => {
      merged.set(getModelSelectionKey(item), item);
    });
    return Array.from(merged.values());
  }, [catalogItems, supplementalModels]);

  const selectedItems = useMemo(
    () =>
      allItems.filter(
        item => selectedSet.has(item.model) && selectedItemKeys.has(getModelSelectionKey(item))
      ),
    [allItems, selectedItemKeys, selectedSet]
  );

  useEffect(() => {
    onSelectionMetaChange?.(selectedItems);
  }, [onSelectionMetaChange, selectedItems]);

  useEffect(() => {
    setSelectedItemKeys(prev => {
      if (selectedSet.size === 0 && prev.size === 0) return prev;

      const next = new Set(
        Array.from(prev).filter(key => selectedSet.has(getModelNameFromSelectionKey(key)))
      );

      return next.size === prev.size ? prev : next;
    });
  }, [selectedSet]);

  // Frontend filtering by model, model_name, provider
  const items = useMemo(() => {
    const normalizedProviderFilter = providerFilter?.trim().toLowerCase();
    const providerItems =
      normalizedProviderFilter && normalizedProviderFilter !== 'all'
        ? allItems.filter(m => (m.provider || 'unknown').toLowerCase() === normalizedProviderFilter)
        : allItems;

    if (!search.trim()) return providerItems;
    const keyword = search.trim().toLowerCase();
    return providerItems.filter(
      m =>
        m.model.toLowerCase().includes(keyword) ||
        (m.model_name && m.model_name.toLowerCase().includes(keyword)) ||
        (m.provider && m.provider.toLowerCase().includes(keyword))
    );
  }, [allItems, providerFilter, search]);

  // Group models by provider
  const groupedModels = useMemo<ProviderGroup[]>(() => {
    const normalizedPreferred = preferredProvider?.trim().toLowerCase();
    const groupMap = new Map<string, ModelItem[]>();
    items.forEach(m => {
      const provider = m.provider || 'unknown';
      if (!groupMap.has(provider)) {
        groupMap.set(provider, []);
      }
      const providerModels = groupMap.get(provider);
      if (providerModels) {
        providerModels.push(m);
      }
    });
    // Sort providers alphabetically
    return Array.from(groupMap.entries())
      .sort((a, b) => {
        if (!normalizedPreferred || normalizedPreferred === 'all') return a[0].localeCompare(b[0]);
        const aMatch = a[0].toLowerCase() === normalizedPreferred;
        const bMatch = b[0].toLowerCase() === normalizedPreferred;
        if (aMatch && !bMatch) return -1;
        if (!aMatch && bMatch) return 1;
        return a[0].localeCompare(b[0]);
      })
      .map(([provider, models]) => ({ provider, models }));
  }, [items, preferredProvider]);

  useEffect(() => {
    if (!autoCollapseOthers) return;
    if (!preferredProvider?.trim() || preferredProvider.trim().toLowerCase() === 'all') {
      setCollapsedGroups(new Set());
      return;
    }
    const normalizedPreferred = preferredProvider.trim().toLowerCase();
    const allProviders = new Set(allItems.map(item => item.provider || 'unknown'));
    setCollapsedGroups(
      new Set(
        Array.from(allProviders).filter(provider => provider.toLowerCase() !== normalizedPreferred)
      )
    );
  }, [allItems, autoCollapseOthers, preferredProvider]);

  const effectivePlaceholder = useMemo(
    () => placeholder || t('common.modelMultiSelector.placeholder'),
    [placeholder, t]
  );

  const setSelected = useCallback(
    (next: string[]) => {
      if (controlled) onChange?.(next);
      else setInternalSelected(next);
    },
    [controlled, onChange]
  );

  const toggleSelectedName = useCallback(
    (name: string) => {
      const next = new Set(selectedSet);
      if (next.has(name)) {
        next.delete(name);
        setSelectedItemKeys(prev => {
          const nextKeys = new Set(
            Array.from(prev).filter(key => getModelNameFromSelectionKey(key) !== name)
          );
          return nextKeys.size === prev.size ? prev : nextKeys;
        });
      } else {
        next.add(name);
      }
      setSelected(Array.from(next));
    },
    [selectedSet, setSelected]
  );

  const toggleModel = useCallback(
    (model: ModelItem) => {
      if (!isSelectable(model)) return;

      const modelKey = getModelSelectionKey(model);
      const next = new Set(selectedSet);
      if (next.has(model.model)) {
        next.delete(model.model);
        setSelectedItemKeys(prev => {
          const nextKeys = new Set(prev);
          nextKeys.delete(modelKey);
          return nextKeys;
        });
      } else {
        next.add(model.model);
        setSelectedItemKeys(prev => {
          const nextKeys = new Set(prev);
          nextKeys.add(modelKey);
          return nextKeys;
        });
      }
      setSelected(Array.from(next));
    },
    [isSelectable, selectedSet, setSelected]
  );

  const clearAll = useCallback(() => {
    if (selected.length === 0) return;
    setSelectedItemKeys(new Set());
    setSelected([]);
  }, [selected.length, setSelected]);

  // Toggle all models in a provider group
  const toggleProviderGroup = useCallback(
    (group: ProviderGroup) => {
      const selectableModels = group.models.filter(isSelectable);
      if (selectableModels.length === 0) return;

      const groupModelNames = selectableModels.map(m => m.model);
      const allSelected = groupModelNames.every(name => selectedSet.has(name));

      const next = new Set(selectedSet);
      if (allSelected) {
        // Deselect all in this group
        groupModelNames.forEach(name => next.delete(name));
        setSelectedItemKeys(prev => {
          const nextKeys = new Set(prev);
          selectableModels.forEach(model => nextKeys.delete(getModelSelectionKey(model)));
          return nextKeys;
        });
      } else {
        // Select all in this group
        groupModelNames.forEach(name => next.add(name));
        setSelectedItemKeys(prev => {
          const nextKeys = new Set(prev);
          selectableModels.forEach(model => nextKeys.add(getModelSelectionKey(model)));
          return nextKeys;
        });
      }
      setSelected(Array.from(next));
    },
    [isSelectable, selectedSet, setSelected]
  );

  // Toggle collapse state for a provider group
  const toggleCollapse = useCallback((provider: string) => {
    setCollapsedGroups(prev => {
      const next = new Set(prev);
      if (next.has(provider)) {
        next.delete(provider);
      } else {
        next.add(provider);
      }
      return next;
    });
  }, []);

  // Check if all models in a group are selected
  const isGroupAllSelected = useCallback(
    (group: ProviderGroup): boolean => {
      const selectableModels = group.models.filter(isSelectable);
      return (
        selectableModels.length > 0 && selectableModels.every(m => selectedSet.has(m.model))
      );
    },
    [isSelectable, selectedSet]
  );

  // Check if some (but not all) models in a group are selected
  const isGroupPartiallySelected = useCallback(
    (group: ProviderGroup): boolean => {
      const selectableModels = group.models.filter(isSelectable);
      const someSelected = selectableModels.some(m => selectedSet.has(m.model));
      const allSelected =
        selectableModels.length > 0 && selectableModels.every(m => selectedSet.has(m.model));
      return someSelected && !allSelected;
    },
    [isSelectable, selectedSet]
  );

  return (
    <div className={cn('w-full rounded-xl border p-3 flex flex-col h-full', className)}>
      {/* Selected chips row */}
      <div>
        <div className="flex justify-between items-center mb-1">
          <div className="text-sm text-foreground">
            {t('common.modelMultiSelector.selectedTitle')} ({selected.length})
          </div>
          <div className="flex justify-end gap-4 text-sm">
            <button
              type="button"
              className="text-highlight hover:underline disabled:opacity-50"
              onClick={clearAll}
              disabled={disabled || selected.length === 0}
            >
              {t('common.modelMultiSelector.clear')}
            </button>
          </div>
        </div>

        {selected.length === 0 ? (
          <div className="text-xs text-muted-foreground">{effectivePlaceholder}</div>
        ) : (
          <div className="flex flex-wrap gap-2 max-h-[86px] overflow-y-auto">
            {selected.map(m => (
              <span
                key={m}
                className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-accent text-accent-foreground border"
              >
                {m}
                <button
                  type="button"
                  className="ml-1 opacity-70 hover:opacity-100"
                  onClick={() => toggleSelectedName(m)}
                  disabled={disabled}
                >
                  <X size={12} />
                </button>
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Collapsible content */}
      <div className="mt-2 border rounded-lg overflow-hidden flex flex-col h-0 grow">
        {/* Search */}
        <div className="p-2 border-b">
          <div className="relative">
            <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground pointer-events-none" />
            <Input
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder={t('common.modelMultiSelector.searchPlaceholder')}
              className="pl-8"
              autoComplete="off"
              disabled={disabled}
            />
          </div>
        </div>

        {/* List grouped by provider */}
        {isLoading ? (
          <div className="p-3 space-y-2">
            {Array.from({ length: 8 }).map((_, i) => (
              <div key={i} className="flex items-center gap-2">
                <Skeleton className="h-4 w-52" />
              </div>
            ))}
          </div>
        ) : (
          <div className="h-0 grow overflow-y-auto bg-muted/30">
            {groupedModels.length === 0 ? (
              <div className="p-4 text-center text-sm text-muted-foreground">
                {t('common.modelMultiSelector.noModels')}
              </div>
            ) : (
              <>
                {groupedModels.map(group => {
                  const isCollapsed = collapsedGroups.has(group.provider);
                  const allSelected = isGroupAllSelected(group);
                  const partialSelected = isGroupPartiallySelected(group);
                  const hasSelectableModels = group.models.some(isSelectable);

                  return (
                    <div key={group.provider} className="border-b last:border-b-0">
                      {/* Provider group header */}
                      <div
                        className="flex items-center justify-between px-3 py-2 bg-muted cursor-pointer hover:bg-accent sticky top-0 z-10"
                        onClick={() => toggleCollapse(group.provider)}
                      >
                        <div className="flex items-center gap-2">
                          <ChevronDown
                            className={cn(
                              'h-4 w-4 transition-transform shrink-0',
                              isCollapsed && '-rotate-90'
                            )}
                          />
                          <Checkbox
                            checked={
                              allSelected
                                ? true
                                : partialSelected
                                  ? ('indeterminate' as const)
                                  : false
                            }
                            className={
                              allSelected
                                ? 'bg-[var(--checkbox-selected)] text-white border-[var(--checkbox-hover)]'
                                : partialSelected
                                  ? 'border-[var(--checkbox-hover)]'
                                  : undefined
                            }
                            onCheckedChange={() => toggleProviderGroup(group)}
                            onClick={e => e.stopPropagation?.()}
                            disabled={disabled || !hasSelectableModels}
                          />
                          <ProviderIcon size={24} provider={group.provider} />
                          <span className="text-sm font-medium">{group.provider}</span>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {group.models.filter(m => selectedSet.has(m.model)).length}/
                          {group.models.length}
                        </span>
                      </div>

                      {/* Models list */}
                      {!isCollapsed && (
                        <ul className={cn('p-2 grid gap-1', COLUMNS_CLASS[columns])}>
                          {group.models.map(m => {
                            const id = m.model;
                            const checked = selectedSet.has(id);
                            const selectable = isSelectable(m);
                            return (
                              <label
                                htmlFor={`model-${m.provider}-${id}`}
                                key={`${m.provider}|${id}`}
                                className={cn(
                                  'flex items-center gap-2 px-3 py-2 rounded-md group',
                                  selectable && !disabled
                                    ? 'cursor-pointer hover:bg-accent/80'
                                    : 'cursor-not-allowed opacity-55'
                                )}
                              >
                                <Checkbox
                                  checked={checked}
                                  onCheckedChange={() => toggleModel(m)}
                                  id={`model-${m.provider}-${id}`}
                                  disabled={disabled || !selectable}
                                />
                                <ProviderIcon provider={m.provider} size={18} />
                                <span className="text-xs truncate flex-1" title={m.model_name}>
                                  {m.model_name || id}
                                </span>
                                <div className="flex items-center gap-1.5 shrink-0">
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <div className="flex items-center cursor-help">
                                        <Info className="size-3.5 text-muted-foreground" />
                                      </div>
                                    </TooltipTrigger>
                                    <TooltipContent side="right" className="p-3">
                                      <ModelTooltipContent
                                        model={m}
                                        featureLabels={{
                                          streaming: t('aiProviders.models.features.streaming'),
                                          function_calling: t(
                                            'aiProviders.models.features.function_calling'
                                          ),
                                          structured_output: t(
                                            'aiProviders.models.features.structured_output'
                                          ),
                                          json_mode: t('aiProviders.models.features.json_mode'),
                                          reasoning: t('aiProviders.models.features.reasoning'),
                                          attachment: t('aiProviders.models.features.attachment'),
                                          web_search: t('aiProviders.models.features.web_search'),
                                          file_search: t('aiProviders.models.features.file_search'),
                                          code_interpreter: t(
                                            'aiProviders.models.features.code_interpreter'
                                          ),
                                          vision: t('aiProviders.models.features.vision'),
                                          system_prompt: t(
                                            'aiProviders.models.features.system_prompt'
                                          ),
                                        }}
                                        useCaseLabels={{
                                          'text-chat': t('models.selector.usecases.text-chat'),
                                          vision: t('models.selector.usecases.vision'),
                                          'image-gen': t('models.selector.usecases.image-gen'),
                                          embedding: t('models.selector.usecases.embedding'),
                                          rerank: t('models.selector.usecases.rerank'),
                                          'speech-to-text': t(
                                            'models.selector.usecases.speech-to-text'
                                          ),
                                          'text-to-speech': t(
                                            'models.selector.usecases.text-to-speech'
                                          ),
                                          'realtime-audio': t(
                                            'models.selector.usecases.realtime-audio'
                                          ),
                                          'video-gen': t('models.selector.usecases.video-gen'),
                                          moderation: t('models.selector.usecases.moderation'),
                                          reasoning: t('models.selector.usecases.reasoning'),
                                          'function-calling': t(
                                            'models.selector.usecases.function-calling'
                                          ),
                                        }}
                                        labels={{
                                          context: t('models.selector.tooltip.context'),
                                          features: t('models.selector.tooltip.features'),
                                          useCases: t('models.selector.tooltip.useCases'),
                                        }}
                                      />
                                    </TooltipContent>
                                  </Tooltip>
                                </div>
                              </label>
                            );
                          })}
                        </ul>
                      )}
                    </div>
                  );
                })}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

const ModelMultiSelector = memo(ModelMultiSelectorBase);
export default ModelMultiSelector;
