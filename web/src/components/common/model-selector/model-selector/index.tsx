'use client';

import { useMemo, useState, useCallback, useRef, useEffect } from 'react';
import { Select, SelectContent, SelectTrigger, SelectValue } from '@/components/ui/select';

import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ModelUseCase, ModelItem } from '@/services/types/model';
import { ModelIcon } from 'modelicons';
import { useAvailableModels } from '@/hooks/model/use-model';
import { ModelFeatureIcon } from '@/components/model/model-feature-icon';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { useLocale } from '@/hooks/use-locale';
import { getModelDisplayName } from '@/utils/model-label';

import type {
  ModelSelectorValue,
  ModelSelectorModelProps,
  ProviderGroup,
  FlatRow,
  FeatureLabels,
} from './types';
import { serializeValue, deserializeValue } from './utils';
import {
  LoadingSkeleton,
  EmptyState,
  SearchInput,
  CollapseControls,
  ProviderHeader,
  ModelRowItem,
} from './components';
import { usePermissions } from '@/store';

export interface ModelSelectorProps {
  /** The model use case to query, e.g. 'text-chat', 'embedding', 'rerank'. */
  modelType: ModelUseCase;
  /** Current selected value as an object with provider and model. */
  value?: ModelSelectorValue;
  /** Callback triggered when selection changes. */
  onChange?: (value: ModelSelectorValue) => void;
  /** Placeholder text displayed when no value selected. */
  placeholder?: string;
  /** Additional CSS classes for the trigger. */
  className?: string;
  /** Disable select. */
  disabled?: boolean;
  /** Enable search functionality. */
  searchable?: boolean;
  /** Enable provider group collapsing. */
  collapsible?: boolean;
  /** Default value. */
  defaultValue?: ModelSelectorValue;
  /** Controlled full model props of the current selection. */
  modelProps?: ModelSelectorModelProps | null;
  /** Callback to notify full model props on selection change. */
  onModelPropsChange?: (props: ModelSelectorModelProps | null) => void;
  /** Filter models by capability requirements. Multiple capabilities can be required. */
  capabilityFilter?: {
    /** Require vision support */
    features_vision?: boolean;
    /** Require tool call support */
    features_tool_call?: boolean;
    /** Require attachment support */
    features_attachment?: boolean;
    /** Require reasoning support */
    features_reasoning?: boolean;
    /** Require structured output support */
    features_structured_output?: boolean;
  };
  /** When true, display an error border on the select trigger. */
  hasError?: boolean;
  /** Whether to show capabilities icons in the trigger label. Default: true */
  showCapabilities?: boolean;
}

// Virtualization constants
const HEADER_HEIGHT = 36;
const ROW_HEIGHT = 36;
const OVERSCAN_PX = 200;

function getModelPropsSignature(props: ModelSelectorModelProps | null): string {
  if (!props) return '__empty__';

  const visionEnabled = Boolean(
    props.endpoints?.vision ||
      props.use_cases?.includes('vision') ||
      props.input_modalities?.includes('image')
  );
  return `${props.id}|${props.provider}|${props.model}|${props.updated_at}|${visionEnabled}`;
}

/**
 * ModelSelectorTemp – an enhanced dropdown component for choosing an AI model grouped by provider.
 * Uses the new  model API with type filtering.
 */
export function ModelSelector({
  modelType,
  value,
  onChange,
  placeholder,
  className,
  disabled = false,
  searchable = true,
  collapsible = true,
  defaultValue,
  modelProps,
  onModelPropsChange,
  capabilityFilter,
  hasError = false,
  showCapabilities = true,
}: ModelSelectorProps) {
  const t = useT();
  const { locale } = useLocale();
  const getProviderName = useProviderI18n();
  const [searchQuery, setSearchQuery] = useState('');
  const [collapsedProviders, setCollapsedProviders] = useState<Set<string>>(new Set());
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Virtualization refs and state
  const scrollRef = useRef<HTMLDivElement>(null);
  const [viewportHeight, setViewportHeight] = useState(0);
  const [scrollTop, setScrollTop] = useState(0);
  const lastScrollTopRef = useRef(0);
  const isScrollingRef = useRef(false);
  const scrollEndTimerRef = useRef<number | null>(null);
  const lastModelPropsSignatureRef = useRef<string | null>(null);

  // Track open state
  const [open, setOpen] = useState(false);
  const [internalSelected, setInternalSelected] = useState<ModelSelectorValue | null>(null);

  // Get user role for conditional rendering in empty state
  const { organizationRole } = usePermissions();
  const isAdminOrOwner = ['owner', 'admin'].includes(organizationRole || '');

  // Use translated placeholder if none provided
  const effectivePlaceholder = placeholder || t('models.selector.placeholder');

  // Memoize feature labels to avoid re-creating object on each render
  // Uses aiProviders.models.features translations that match API keys
  const featureLabels: FeatureLabels = useMemo(
    () => ({
      streaming: t('aiProviders.models.features.streaming'),
      function_calling: t('aiProviders.models.features.function_calling'),
      structured_output: t('aiProviders.models.features.structured_output'),
      json_mode: t('aiProviders.models.features.json_mode'),
      reasoning: t('aiProviders.models.features.reasoning'),
      attachment: t('aiProviders.models.features.attachment'),
      web_search: t('aiProviders.models.features.web_search'),
      file_search: t('aiProviders.models.features.file_search'),
      code_interpreter: t('aiProviders.models.features.code_interpreter'),
      vision: t('aiProviders.models.features.vision'),
      system_prompt: t('aiProviders.models.features.system_prompt'),
      distillation: t('aiProviders.models.features.distillation'),
      logprobs: t('aiProviders.models.features.logprobs'),
      computer_use: t('aiProviders.models.features.computer_use'),
      mcp: t('aiProviders.models.features.mcp'),
      reasoning_effort: t('aiProviders.models.features.reasoning_effort'),
    }),
    [t]
  );

  const useCaseLabels: Record<string, string> = useMemo(
    () => ({
      'text-chat': t('models.selector.usecases.text-chat'),
      vision: t('models.selector.usecases.vision'),
      'image-gen': t('models.selector.usecases.image-gen'),
      embedding: t('models.selector.usecases.embedding'),
      rerank: t('models.selector.usecases.rerank'),
      'speech-to-text': t('models.selector.usecases.speech-to-text'),
      'text-to-speech': t('models.selector.usecases.text-to-speech'),
      'realtime-audio': t('models.selector.usecases.realtime-audio'),
      'video-gen': t('models.selector.usecases.video-gen'),
      moderation: t('models.selector.usecases.moderation'),
      reasoning: t('models.selector.usecases.reasoning'),
      'function-calling': t('models.selector.usecases.function-calling'),
    }),
    [t]
  );

  // Fetch available models with use_case filter (non-paginated, non-expiring)
  const { models, isLoading, isFetching, refetch } = useAvailableModels({
    use_case: modelType as ModelUseCase,
  });

  // Apply capability filtering
  const filteredModelsByCapability = useMemo(() => {
    if (!capabilityFilter) return models;
    return models.filter(model => {
      // Check each capability requirement
      if (
        capabilityFilter.features_vision &&
        !model.endpoints?.vision &&
        !model.use_cases?.includes('vision') &&
        !model.input_modalities?.includes('image')
      ) {
        return false;
      }
      if (capabilityFilter.features_tool_call && !model.features?.function_calling) return false;
      if (capabilityFilter.features_attachment && !model.features?.attachment) return false;
      if (capabilityFilter.features_reasoning && !model.features?.reasoning) return false;
      if (capabilityFilter.features_structured_output && !model.features?.structured_output) {
        return false;
      }
      return true;
    });
  }, [models, capabilityFilter]);

  // Group models by provider
  const providerGroups = useMemo<ProviderGroup[]>(() => {
    const groupMap = new Map<string, ModelItem[]>();
    filteredModelsByCapability.forEach(model => {
      const existing = groupMap.get(model.provider) || [];
      existing.push(model);
      groupMap.set(model.provider, existing);
    });
    return Array.from(groupMap.entries()).map(([provider, items]) => ({
      provider,
      models: items,
    }));
  }, [filteredModelsByCapability]);

  // Clear search when dropdown closes or component unmounts
  useEffect(() => {
    return () => {
      setSearchQuery('');
    };
  }, []);

  // Pre-compute lowercase search index
  const searchIndex = useMemo(() => {
    const idx = new Map<string, { providerLower: string; modelLower: Map<string, string> }>();
    providerGroups.forEach(p => {
      const providerLabel = getProviderName(p.provider);
      const providerLower = `${p.provider.toLowerCase()} ${providerLabel.toLowerCase()}`.trim();
      const modelLower = new Map<string, string>();
      p.models.forEach(m => {
        const l = getModelDisplayName(m, locale).toLowerCase();
        modelLower.set(m.model, l);
      });
      idx.set(p.provider, { providerLower, modelLower });
    });
    return idx;
  }, [getProviderName, locale, providerGroups]);

  // Filter providers and models based on search query
  const filteredProviders = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();

    return providerGroups
      .map(provider => {
        const idx = searchIndex?.get(provider.provider);
        const providerMatch = q ? (idx?.providerLower.includes(q) ?? false) : false;

        const sourceModels = provider.models;
        const filteredModels = q
          ? sourceModels.filter(model => {
              const l =
                idx?.modelLower.get(model.model) ??
                getModelDisplayName(model, locale).toLowerCase();
              return l.includes(q) || providerMatch;
            })
          : sourceModels;

        // Deduplicate models by name
        const seen = new Set<string>();
        const dedupedModels: ModelItem[] = [];
        for (const m of filteredModels) {
          if (seen.has(m.model)) continue;
          seen.add(m.model);
          dedupedModels.push(m);
        }

        return dedupedModels.length > 0 ? { ...provider, models: dedupedModels } : null;
      })
      .filter(Boolean) as ProviderGroup[];
  }, [locale, providerGroups, searchQuery, searchIndex]);

  // Flattened rows for virtualization
  const flatRows = useMemo<FlatRow[]>(() => {
    const rows: FlatRow[] = [];
    filteredProviders.forEach(p => {
      rows.push({
        type: 'header',
        providerId: p.provider,
        providerLabel: getProviderName(p.provider),
        modelCount: p.models.length,
      });
      const isCollapsed = collapsible && collapsedProviders.has(p.provider);
      if (!isCollapsed) {
        p.models.forEach(m => rows.push({ type: 'model', providerId: p.provider, model: m }));
      }
    });
    return rows;
  }, [filteredProviders, collapsedProviders, collapsible, getProviderName]);

  // Row heights for windowing
  const rowHeights = useMemo<number[]>(
    () => flatRows.map(r => (r.type === 'header' ? HEADER_HEIGHT : ROW_HEIGHT)),
    [flatRows]
  );
  const totalHeight = useMemo(() => rowHeights.reduce((acc, h) => acc + h, 0), [rowHeights]);

  // Handle open change
  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      setOpen(nextOpen);
      if (nextOpen) {
        requestAnimationFrame(() => {
          const el = scrollRef.current;
          if (el) {
            setViewportHeight(el.clientHeight);
            setScrollTop(el.scrollTop);
            el.scrollTop = lastScrollTopRef.current || 0;
          }
          if (searchable) {
            searchInputRef.current?.focus();
          }
        });
      } else {
        setSearchQuery('');
      }
    },
    [searchable]
  );

  // Track scroll and viewport measurements
  useEffect(() => {
    if (!open) return;
    const el = scrollRef.current;
    if (!el) return;

    const onScroll = () => {
      setScrollTop(el.scrollTop);
      lastScrollTopRef.current = el.scrollTop;
      isScrollingRef.current = true;
      if (scrollEndTimerRef.current) {
        window.clearTimeout(scrollEndTimerRef.current);
      }
      scrollEndTimerRef.current = window.setTimeout(() => {
        isScrollingRef.current = false;
      }, 150);
      const active = document.activeElement as HTMLElement | null;
      if (active && el.contains(active) && active !== searchInputRef.current) {
        active.blur();
      }
    };

    setViewportHeight(el.clientHeight);
    setScrollTop(el.scrollTop);
    el.addEventListener('scroll', onScroll);

    let ro: ResizeObserver | null = null;
    let onWinResize: (() => void) | null = null;
    if (typeof ResizeObserver !== 'undefined') {
      ro = new ResizeObserver(() => setViewportHeight(el.clientHeight));
      ro.observe(el);
    } else {
      onWinResize = () => setViewportHeight(el.clientHeight);
      window.addEventListener('resize', onWinResize);
    }

    return () => {
      el.removeEventListener('scroll', onScroll);
      if (ro) ro.disconnect();
      if (onWinResize) window.removeEventListener('resize', onWinResize);
      if (scrollEndTimerRef.current) {
        window.clearTimeout(scrollEndTimerRef.current);
        scrollEndTimerRef.current = null;
      }
    };
  }, [open, filteredProviders]);

  // Compute visible window indices
  const { startIndex, endIndex, topOffset } = useMemo(() => {
    if (rowHeights.length === 0) return { startIndex: 0, endIndex: 0, topOffset: 0 };
    const startBoundary = Math.max(0, scrollTop - OVERSCAN_PX);
    const endBoundary = scrollTop + viewportHeight + OVERSCAN_PX;

    let start = 0;
    let acc = 0;
    while (start < rowHeights.length && acc + rowHeights[start] <= startBoundary) {
      acc += rowHeights[start];
      start++;
    }

    let end = start;
    let covered = acc;
    while (end < rowHeights.length && covered < endBoundary) {
      covered += rowHeights[end];
      end++;
    }

    return { startIndex: start, endIndex: end, topOffset: acc };
  }, [rowHeights, scrollTop, viewportHeight]);

  // Toggle provider collapse
  const toggleProvider = useCallback((providerId: string) => {
    setCollapsedProviders(prev => {
      const newCollapsed = new Set(prev);
      if (newCollapsed.has(providerId)) {
        newCollapsed.delete(providerId);
      } else {
        newCollapsed.add(providerId);
      }
      return newCollapsed;
    });
  }, []);

  const expandAll = useCallback(() => {
    setCollapsedProviders(new Set());
  }, []);

  const collapseAll = useCallback(() => {
    const allProviderIds = filteredProviders.map(provider => provider.provider);
    setCollapsedProviders(new Set(allProviderIds));
  }, [filteredProviders]);

  // Build index for fast display lookup
  const modelIndex = useMemo(() => {
    const byProvider = new Map<string, Map<string, ModelItem>>();
    providerGroups.forEach(p => {
      const inner = new Map<string, ModelItem>();
      p.models.forEach(m => inner.set(m.model, m));
      byProvider.set(p.provider, inner);
    });
    return byProvider;
  }, [providerGroups]);

  const emitModelPropsChange = useCallback(
    (props: ModelSelectorModelProps | null) => {
      if (!onModelPropsChange) return;

      const signature = getModelPropsSignature(props);
      if (lastModelPropsSignatureRef.current === signature) {
        return;
      }

      lastModelPropsSignatureRef.current = signature;
      onModelPropsChange(props);
    },
    [onModelPropsChange]
  );

  // Handle model selection
  const handleModelSelect = useCallback(
    (modelValue: string) => {
      const parsed = deserializeValue(modelValue);
      if (parsed) {
        onChange?.(parsed);
        if (value === undefined && modelProps === undefined) {
          setInternalSelected(parsed);
        }
        const item = modelIndex.get(parsed.provider)?.get(parsed.model);
        if (item) {
          emitModelPropsChange(item);
        } else {
          emitModelPropsChange(null);
        }
      } else {
        emitModelPropsChange(null);
      }
    },
    [emitModelPropsChange, onChange, modelIndex, value, modelProps]
  );

  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(e.target.value);
    lastScrollTopRef.current = 0;
    setScrollTop(0);
    if (scrollRef.current) {
      scrollRef.current.scrollTop = 0;
    }
  }, []);

  const handleSearchKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    e.stopPropagation();
  }, []);

  // Controlled value handling
  const isControlledByValue = value !== undefined;
  const isControlledByModelProps = !isControlledByValue && modelProps !== undefined;

  const controlledValue = useMemo(() => {
    if (isControlledByValue) {
      if (!value || !value.model) return '';
      return serializeValue(value);
    }
    if (isControlledByModelProps) {
      if (!modelProps || !modelProps.model) return '';
      return serializeValue({ provider: modelProps.provider, model: modelProps.model });
    }
    return undefined;
  }, [isControlledByValue, isControlledByModelProps, value, modelProps]);

  const uncontrolledDefaultValue = useMemo(() => {
    if (isControlledByValue || isControlledByModelProps) return undefined;
    if (defaultValue && defaultValue.model) return serializeValue(defaultValue);
    return undefined;
  }, [isControlledByValue, isControlledByModelProps, defaultValue]);

  useEffect(() => {
    if (!onModelPropsChange) return;

    const selected = isControlledByValue
      ? value
      : isControlledByModelProps && modelProps
        ? { provider: modelProps.provider, model: modelProps.model }
        : internalSelected;
    if (!selected?.provider || !selected.model) {
      emitModelPropsChange(null);
      return;
    }

    emitModelPropsChange(modelIndex.get(selected.provider)?.get(selected.model) ?? null);
  }, [
    emitModelPropsChange,
    internalSelected,
    isControlledByModelProps,
    isControlledByValue,
    modelIndex,
    modelProps,
    onModelPropsChange,
    value,
  ]);

  useEffect(() => {
    if (open && searchable) {
      requestAnimationFrame(() => {
        searchInputRef.current?.focus();
      });
    }
  }, [open, searchable]);

  // Resolve selected info for trigger rendering
  const selectedResolved = useMemo(() => {
    let selected: ModelSelectorValue | null = null;
    if (isControlledByValue) selected = value ?? null;
    else if (isControlledByModelProps) {
      selected = modelProps ? { provider: modelProps.provider, model: modelProps.model } : null;
    } else selected = internalSelected;

    if (!selected || !selected.model) return null;
    const p = modelIndex.get(selected.provider);
    const m = p?.get(selected.model);
    const label = m ? getModelDisplayName(m, locale) : selected.model;
    const features = m
      ? Object.entries(m.features || {})
          .filter(([, enabled]) => enabled)
          .slice(0, 6)
          .map(([key]) => key)
      : [];
    return { modelId: selected.model, label, features };
  }, [
    isControlledByValue,
    isControlledByModelProps,
    value,
    modelProps,
    internalSelected,
    locale,
    modelIndex,
  ]);

  return (
    <div className="relative">
      <Select
        value={controlledValue}
        defaultValue={uncontrolledDefaultValue}
        onValueChange={handleModelSelect}
        disabled={disabled || isLoading}
        onOpenChange={handleOpenChange}
      >
        <SelectTrigger
          className={cn(
            'w-full',
            hasError && 'border-destructive focus:ring-destructive',
            className
          )}
          id="model-selector-temp-trigger"
          isLoading={isLoading}
        >
          <SelectValue placeholder={effectivePlaceholder}>
            {selectedResolved ? (
              <div className="flex items-center gap-1.5 min-w-0">
                <ModelIcon
                  model={selectedResolved.modelId}
                  className="shrink-0 flex items-center justify-center"
                  size={20}
                />
                <span className="truncate text-[13px]">{selectedResolved.label}</span>
                {showCapabilities &&
                  selectedResolved.features &&
                  selectedResolved.features.length > 0 && (
                    <span className="flex items-center gap-1 ml-1 shrink-0">
                      {selectedResolved.features.map(k => (
                        <ModelFeatureIcon key={k} feature={k} className="w-3 h-3" />
                      ))}
                    </span>
                  )}
              </div>
            ) : undefined}
          </SelectValue>
        </SelectTrigger>
        <SelectContent
          className="h-[min(400px,var(--radix-select-content-available-height,calc(100dvh-16px)))] min-w-[300px] px-0"
          onCloseAutoFocus={e => e.preventDefault()}
          onFocusCapture={e => {
            const el = scrollRef.current;
            const target = e.target as HTMLElement | null;
            if (!el || !target) return;
            if (el.contains(target) && target !== searchInputRef.current) {
              searchInputRef.current?.focus();
            }
          }}
        >
          <div className="h-full flex flex-col">
            {searchable && (
              <SearchInput
                inputRef={searchInputRef}
                value={searchQuery}
                placeholder={t('models.selector.searchPlaceholder')}
                onChange={handleSearchChange}
                onKeyDown={handleSearchKeyDown}
                onRefresh={() => {
                  refetch();
                }}
                refreshLabel={t('models.selector.empty.refresh')}
                isFetching={isFetching}
              />
            )}
            {collapsible && filteredProviders && filteredProviders.length > 1 && (
              <CollapseControls
                expandAllText={t('models.selector.expandAll')}
                collapseAllText={t('models.selector.collapseAll')}
                onExpandAll={expandAll}
                onCollapseAll={collapseAll}
              />
            )}
            {isLoading && <LoadingSkeleton />}
            <div ref={scrollRef} className="h-0 grow overflow-y-auto p-1" tabIndex={-1}>
              {flatRows.length > 0 ? (
                <div style={{ height: totalHeight, position: 'relative' }}>
                  <div style={{ transform: `translateY(${topOffset}px)` }}>
                    {flatRows.slice(startIndex, endIndex).map(row => {
                      if (row.type === 'header') {
                        const isCollapsed = collapsible && collapsedProviders.has(row.providerId);
                        const modelCountText =
                          row.modelCount === 1
                            ? t('models.selector.modelCount.single')
                            : t('models.selector.modelCount.multiple');
                        return (
                          <ProviderHeader
                            key={`header-${row.providerId}`}
                            providerId={row.providerId}
                            providerLabel={row.providerLabel}
                            modelCount={row.modelCount}
                            modelCountText={modelCountText}
                            isCollapsed={isCollapsed}
                            collapsible={collapsible}
                            onToggle={toggleProvider}
                          />
                        );
                      }

                      return (
                        <ModelRowItem
                          key={`model-${row.providerId}|${row.model.model}`}
                          model={row.model}
                          providerId={row.providerId}
                          contextLabel={t('models.selector.tooltip.context')}
                          deprecatedUnavailableLabel={t(
                            'models.selector.tooltip.deprecatedUnavailable'
                          )}
                          featuresLabel={t('models.selector.tooltip.features')}
                          replacementSuggestionLabel={t(
                            'models.selector.tooltip.replacementSuggestion'
                          )}
                          useCaseLabel={t('models.selector.tooltip.useCases')}
                          featureLabels={featureLabels}
                          useCaseLabels={useCaseLabels}
                          locale={locale}
                        />
                      );
                    })}
                  </div>
                </div>
              ) : (
                !isLoading && (
                  <EmptyState
                    searchQuery={searchQuery}
                    noModelsTitle={t('models.selector.empty.noModelsTitle')}
                    noResultsText={t('models.selector.empty.noResults')}
                    noModelsText={t('models.selector.empty.noModels', {
                      type: t(`models.selector.usecases.${modelType}`),
                    })}
                    isAdminOrOwner={isAdminOrOwner}
                    contactAdminText={t('models.selector.empty.contactAdmin')}
                    configureText={t('models.selector.empty.configure')}
                    configureDescription={t('models.selector.empty.configureDescription', {
                      type: t(`models.selector.usecases.${modelType}`),
                    })}
                    clearSearchText={t('models.selector.empty.clearSearch')}
                    onClearSearch={() => setSearchQuery('')}
                  />
                )
              )}
            </div>
          </div>
        </SelectContent>
      </Select>
    </div>
  );
}

export type { ModelSelectorValue, ModelSelectorModelProps, FeatureLabels } from './types';
