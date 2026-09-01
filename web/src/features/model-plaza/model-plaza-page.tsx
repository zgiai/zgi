'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQueries } from '@tanstack/react-query';
import { Activity, Brain, Copy, Loader2, Search, Sparkles } from 'lucide-react';
import { toast } from 'sonner';
import { useAllModelsInfinite, useProviderModelsInfinite } from '@/hooks/model/use-model';
import { useCustomProviders, useProviders } from '@/hooks/provider/use-provider';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { modelService } from '@/services/model.service';
import type { ModelItem, ModelUseCase } from '@/services/types/model';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { ProviderIcon } from '@/components/common/provider-icon';
import { cn } from '@/lib/utils';
import { normalizeModel } from '@/utils/model-normalize';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { getModelPriceDisplay } from '@/utils/model-price';
import { useOrganizationStore } from '@/store/organization-store';
import { getBillingDisplaySettings } from '@/utils/billing-display';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';

const USE_CASE_VALUES: ModelUseCase[] = [
  'text-chat',
  'vision',
  'image-gen',
  'embedding',
  'rerank',
  'speech-to-text',
  'text-to-speech',
  'reasoning',
  'function-calling',
];

const PROVIDER_SEARCH_ALIASES: Record<string, string[]> = {
  doubao: ['豆包', '火山', '火山引擎', 'volcengine', 'bytedance', '字节'],
  siliconflow: ['硅基流动'],
};

interface ManufacturerFilterItem {
  label: string;
  value: string;
  iconKey?: string;
}

const MODEL_VENDOR_METADATA: ManufacturerFilterItem[] = [
  { label: 'Anthropic', value: 'anthropic', iconKey: 'anthropic' },
  { label: 'DeepSeek', value: 'deepseek', iconKey: 'deepseek' },
  { label: 'Doubao', value: 'doubao', iconKey: 'doubao' },
  { label: 'MiniMax', value: 'minimax', iconKey: 'minimax' },
  { label: 'Moonshot AI', value: 'moonshot', iconKey: 'moonshot' },
  { label: 'OpenAI', value: 'openai', iconKey: 'openai' },
  { label: 'Qwen', value: 'qwen', iconKey: 'qwen' },
  { label: 'Zhipu AI', value: 'zhipu', iconKey: 'zhipu' },
];

function modelManufacturer(model: ModelItem, locale: string): ManufacturerFilterItem {
  const vendor = normalizeSearchValue(model.vendor ?? '');
  const configured = MODEL_VENDOR_METADATA.find(item => item.value === vendor);
  const localizedName = locale.toLowerCase().startsWith('zh')
    ? model.vendor_cn_name || model.vendor_name || model.vendor_en_name
    : model.vendor_en_name || model.vendor_name || model.vendor_cn_name;

  return {
    ...(configured ?? { value: vendor }),
    label:
      nonEmptyText(localizedName) || configured?.label || nonEmptyText(model.vendor) || vendor || '-',
    iconKey: configured?.iconKey || vendor,
  };
}

interface VideoResolutionRate {
  resolution?: string | null;
  rates?: Array<{
    input_video?: boolean | null;
    price_per_million_tokens?: number | null;
  }> | null;
}

interface StructuredModelPricing {
  currency?: string | null;
  video_generation?: {
    currency?: string | null;
    resolution_rates?: VideoResolutionRate[] | null;
  } | null;
}

function normalizeSearchValue(value: unknown): string {
  return typeof value === 'string' ? value.trim().toLowerCase() : '';
}

function getProviderAliases(provider: string, label?: string, rawName?: string): string[] {
  return Array.from(
    new Set(
      [provider, label, rawName, ...(PROVIDER_SEARCH_ALIASES[provider] ?? [])].filter(Boolean)
    )
  ) as string[];
}

function modelDisplayName(model: ModelItem): string {
  return (
    nonEmptyText(model.model_name) ||
    nonEmptyText(model.model) ||
    nonEmptyText(model.id) ||
    '-'
  );
}

function nonEmptyText(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  const text = value.trim();
  return text || undefined;
}

function modelMatchesSearch(
  model: ModelItem,
  search: string,
  providerSearchValues: readonly string[] = [],
  useCaseLabels: Record<string, string> = {}
): boolean {
  const keyword = normalizeSearchValue(search);
  if (!keyword) return true;

  const searchableUseCaseLabels = (model.use_cases ?? []).map(
    useCase => useCaseLabels[useCase] || useCase
  );
  const featureLabels = [
    model.features?.function_calling ? '函数调用 function_calling' : '',
    model.features?.vision ? '视觉 vision' : '',
    model.features?.reasoning ? '推理 reasoning' : '',
  ];

  return [
    model.model,
    model.model_name,
    model.provider,
    model.vendor,
    model.family,
    model.family_name,
    model.tagline,
    model.tier,
    ...providerSearchValues,
    ...searchableUseCaseLabels,
    ...featureLabels,
  ].some(value => normalizeSearchValue(value ?? '').includes(keyword));
}

async function fetchAllProviderModels(
  provider: string,
  useCase: 'all' | ModelUseCase
): Promise<ModelItem[]> {
  const pageSize = 100;
  const models: ModelItem[] = [];
  const seen = new Set<string>();
  let page = 1;
  let totalPages = 1;

  while (page <= totalPages) {
    const response = await modelService.getModels({
      provider,
      page,
      page_size: pageSize,
      available_only: true,
      use_case: useCase === 'all' ? undefined : useCase,
    });
    const list = response.data;
    totalPages =
      typeof list.total_pages === 'number' && list.total_pages > 0 ? list.total_pages : page;

    const items = Array.isArray(list?.items) ? list.items : [];
    items
      .filter(isModelRecord)
      .map(normalizeModel)
      .forEach(model => {
        const key = `${model.provider}:${model.model}`;
        if (seen.has(key)) return;
        seen.add(key);
        models.push(model);
      });

    page += 1;
  }

  return models;
}

function isModelRecord(value: unknown): value is ModelItem {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function getExperienceHref(model: ModelItem): string {
  const params = `provider=${encodeURIComponent(model.provider)}&model=${encodeURIComponent(model.model)}`;
  const basePath = model.use_cases?.includes('image-gen')
    ? '/console/work/image'
    : '/console/work/chat';
  return `${basePath}?${params}`;
}

export function ModelPlazaPage() {
  const t = useT('models');
  const { locale } = useLocale();
  const [search, setSearch] = React.useState('');
  const debouncedSearch = useDebouncedValue(search, 500);
  const [useCaseFilter, setUseCaseFilter] = React.useState<'all' | ModelUseCase>('all');
  const [providerFilter, setProviderFilter] = React.useState('all');
  const [manufacturerFilter, setManufacturerFilter] = React.useState('all');
  const getProviderName = useProviderI18n();
  const useCaseLabels = React.useMemo<Record<string, string>>(
    () => ({
      'text-chat': t('plaza.useCases.textChat'),
      vision: t('plaza.useCases.vision'),
      'image-gen': t('plaza.useCases.imageGen'),
      embedding: t('plaza.useCases.embedding'),
      rerank: t('plaza.useCases.rerank'),
      'speech-to-text': t('plaza.useCases.speechToText'),
      'text-to-speech': t('plaza.useCases.textToSpeech'),
      reasoning: t('plaza.useCases.reasoning'),
      'function-calling': t('plaza.useCases.functionCalling'),
    }),
    [t]
  );
  const useCaseFilters = React.useMemo(
    () => [
      { label: t('plaza.all'), value: 'all' as const },
      ...USE_CASE_VALUES.map(value => ({ label: useCaseLabels[value], value })),
    ],
    [t, useCaseLabels]
  );
  const {
    items: officialProviders,
    isLoading: isOfficialProvidersLoading,
    isFetching: isOfficialProvidersFetching,
  } = useProviders({ limit: 50, initialPage: 1 });
  const {
    items: customProviders,
    isLoading: isCustomProvidersLoading,
    isFetching: isCustomProvidersFetching,
  } = useCustomProviders({ limit: 50, initialPage: 1 });

  const providerFilters = React.useMemo(() => {
    const providers = [...officialProviders, ...customProviders]
      .filter(provider => provider.is_enabled)
      .sort((left, right) => {
        const sortDelta = (left.sort_order ?? 0) - (right.sort_order ?? 0);
        if (sortDelta !== 0) return sortDelta;
        return left.provider.localeCompare(right.provider);
      })
      .map(provider => ({
        label: getProviderName(provider.provider, provider.provider_name),
        value: provider.provider,
        aliases: getProviderAliases(
          provider.provider,
          getProviderName(provider.provider, provider.provider_name),
          provider.provider_name
        ),
      }));

    return [{ label: t('plaza.all'), value: 'all', aliases: [t('plaza.all')] }, ...providers];
  }, [customProviders, getProviderName, officialProviders, t]);

  const providerSearchMap = React.useMemo(
    () =>
      new Map(
        providerFilters
          .filter(item => item.value !== 'all')
          .map(item => [item.value, item.aliases] as const)
      ),
    [providerFilters]
  );

  const selectedProvider = providerFilter === 'all' ? undefined : providerFilter;
  const shouldUseProviderSearch = Boolean(debouncedSearch.trim() && providerFilter === 'all');
  const providerValues = React.useMemo(
    () => providerFilters.filter(item => item.value !== 'all').map(item => item.value),
    [providerFilters]
  );
  const {
    models: allModels,
    isLoading: isAllModelsLoading,
    isFetching: isAllModelsFetching,
    hasNextPage: hasNextAllModelsPage,
    fetchNextPage: fetchNextAllModelsPage,
    isFetchingNextPage: isFetchingNextAllModelsPage,
    total: allModelsTotal,
    refetch: refetchAllModels,
  } = useAllModelsInfinite({
    limit: 100,
    available_only: true,
    use_case: useCaseFilter === 'all' ? undefined : useCaseFilter,
  });
  const manufacturerFilters = React.useMemo<ManufacturerFilterItem[]>(() => {
    const availableManufacturers = new Map<string, ManufacturerFilterItem>();
    allModels.forEach(model => {
      const manufacturer = modelManufacturer(model, locale);
      availableManufacturers.set(manufacturer.value, manufacturer);
    });
    const configuredManufacturers = MODEL_VENDOR_METADATA.filter(manufacturer =>
      availableManufacturers.has(manufacturer.value)
    ).map(manufacturer => availableManufacturers.get(manufacturer.value) ?? manufacturer);
    const configuredValues = new Set(configuredManufacturers.map(item => item.value));
    const additionalManufacturers = Array.from(availableManufacturers.values())
      .filter(manufacturer => !configuredValues.has(manufacturer.value))
      .sort((left, right) => left.label.localeCompare(right.label));

    return [
      { label: t('plaza.all'), value: 'all' },
      ...configuredManufacturers,
      ...additionalManufacturers,
    ];
  }, [allModels, locale, t]);
  const {
    models: providerModels,
    total: providerModelsTotal,
    isLoading: isProviderModelsLoading,
    isFetching: isProviderModelsFetching,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
    refetch: refetchProviderModels,
  } = useProviderModelsInfinite(selectedProvider, {
    limit: 100,
    available_only: true,
    use_case: useCaseFilter === 'all' ? undefined : useCaseFilter,
  });
  const providerSearchResults = useQueries({
    queries: shouldUseProviderSearch
      ? providerValues.map(provider => ({
          queryKey: ['model-plaza', 'model-search-by-provider', provider, useCaseFilter],
          queryFn: async () => {
            return fetchAllProviderModels(provider, useCaseFilter);
          },
          staleTime: 5 * 60 * 1000,
          gcTime: 30 * 60 * 1000,
          refetchOnWindowFocus: false,
          retry: false,
        }))
      : [],
  });

  const providerSearchModels = React.useMemo(
    () => providerSearchResults.flatMap(result => result.data ?? []),
    [providerSearchResults]
  );
  const refetchProviderSearchResults = React.useCallback(async () => {
    await Promise.all(providerSearchResults.map(result => result.refetch()));
  }, [providerSearchResults]);

  React.useEffect(() => {
    if (providerFilter !== 'all' && !providerFilters.some(item => item.value === providerFilter)) {
      setProviderFilter('all');
    }
  }, [providerFilter, providerFilters]);

  const filteredModels = React.useMemo(
    () =>
      (shouldUseProviderSearch
        ? providerSearchModels
        : selectedProvider
          ? providerModels
          : allModels
      ).filter(model => {
        if (providerFilter !== 'all' && model.provider !== providerFilter) return false;
        if (
          manufacturerFilter !== 'all' &&
          modelManufacturer(model, locale).value !== manufacturerFilter
        ) {
          return false;
        }
        return modelMatchesSearch(
          model,
          debouncedSearch,
          providerSearchMap.get(model.provider) ?? [],
          useCaseLabels
        );
      }),
    [
      allModels,
      providerFilter,
      providerModels,
      providerSearchMap,
      providerSearchModels,
      debouncedSearch,
      manufacturerFilter,
      locale,
      selectedProvider,
      shouldUseProviderSearch,
      useCaseLabels,
    ]
  );

  const isProviderLoading = isOfficialProvidersLoading || isCustomProvidersLoading;
  const isProviderFetching = isOfficialProvidersFetching || isCustomProvidersFetching;
  const isSearchingProviders = providerSearchResults.some(
    result => result.isLoading || result.isFetching
  );
  const isLoading = shouldUseProviderSearch
    ? isSearchingProviders && filteredModels.length === 0
    : selectedProvider
      ? isProviderModelsLoading
      : isAllModelsLoading;
  const isFetching = shouldUseProviderSearch
    ? isSearchingProviders
    : selectedProvider
      ? isProviderModelsFetching || isFetchingNextPage
      : isAllModelsFetching || isFetchingNextAllModelsPage;
  const totalModels = selectedProvider ? providerModelsTotal : allModelsTotal;
  const displayModelCount =
    debouncedSearch.trim() || manufacturerFilter !== 'all' ? filteredModels.length : totalModels;

  React.useEffect(() => {
    if (
      selectedProvider ||
      shouldUseProviderSearch ||
      !hasNextAllModelsPage ||
      isFetchingNextAllModelsPage
    ) {
      return;
    }
    void fetchNextAllModelsPage();
  }, [
    fetchNextAllModelsPage,
    hasNextAllModelsPage,
    isFetchingNextAllModelsPage,
    selectedProvider,
    shouldUseProviderSearch,
  ]);

  React.useEffect(() => {
    if (!selectedProvider || !hasNextPage || isFetchingNextPage) return;
    void fetchNextPage();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage, selectedProvider]);

  const refetch = React.useCallback(async () => {
    if (shouldUseProviderSearch) {
      await refetchProviderSearchResults();
      return;
    }
    if (selectedProvider) {
      await refetchProviderModels();
      return;
    }
    await refetchAllModels();
  }, [
    refetchAllModels,
    refetchProviderModels,
    refetchProviderSearchResults,
    selectedProvider,
    shouldUseProviderSearch,
  ]);

  const handleRefresh = React.useCallback(async () => {
    try {
      await refetch();
      toast.success(t('plaza.refreshSuccess'));
    } catch (err) {
      const message = err instanceof Error ? err.message : t('plaza.retryLater');
      toast.error(t('plaza.refreshFailed'), { description: message });
    }
  }, [refetch, t]);

  return (
    <div className="flex min-h-full flex-col bg-muted/30">
      <div className="border-b bg-background/80">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-5 sm:px-6 lg:px-8">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Sparkles className="size-4 text-primary" />
                <span>{t('plaza.service')}</span>
              </div>
              <h1 className="mt-1.5 text-2xl font-semibold tracking-tight">{t('plaza.title')}</h1>
              <p className="mt-1.5 max-w-2xl text-sm text-muted-foreground">
                {t('plaza.description')}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" onClick={() => void handleRefresh()} disabled={isFetching}>
                {isFetching ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Activity className="size-4" />
                )}
                {t('plaza.refresh')}
              </Button>
            </div>
          </div>

          <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('plaza.searchPlaceholder')}
              leftIcon={<Search />}
              className="h-9 bg-background"
            />
            <div className="rounded-md border bg-background px-3 py-1.5 text-sm text-muted-foreground">
              {t('plaza.modelCount', { count: displayModelCount })}
            </div>
          </div>

          <FilterRow
            title={t('plaza.useCase')}
            items={useCaseFilters}
            value={useCaseFilter}
            onChange={value => setUseCaseFilter(value as 'all' | ModelUseCase)}
          />
          <FilterRow
            title={t('plaza.manufacturer')}
            items={manufacturerFilters}
            value={manufacturerFilter}
            onChange={setManufacturerFilter}
            loading={isLoading || isProviderLoading || isProviderFetching}
          />
        </div>
      </div>

      <div className="mx-auto w-full max-w-7xl flex-1 px-4 py-5 sm:px-6 lg:px-8">
        {isLoading ? (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, index) => (
              <div key={index} className="h-64 animate-pulse rounded-xl border bg-background" />
            ))}
          </div>
        ) : filteredModels.length > 0 ? (
          <div className="space-y-5">
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {filteredModels.map(model => (
                <ModelCardErrorBoundary
                  key={`${normalizeSearchValue(model.provider)}:${normalizeSearchValue(model.model)}:${modelDisplayName(model)}`}
                  fallbackLabel={modelDisplayName(model)}
                  fallbackMessage={t('plaza.metadataUnavailable')}
                >
                  <ModelCard
                    model={model}
                    useCaseLabels={useCaseLabels}
                    manufacturerLabel={modelManufacturer(model, locale).label}
                  />
                </ModelCardErrorBoundary>
              ))}
            </div>
            {isFetching ? (
              <div className="flex justify-center">
                <div className="inline-flex items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" />
                  {t('plaza.loadingMore')}
                </div>
              </div>
            ) : null}
          </div>
        ) : (
          <Card>
            <CardContent className="flex flex-col items-center justify-center gap-3 py-16 text-center">
              <Brain className="size-10 text-muted-foreground" />
              <div className="text-base font-medium">{t('plaza.emptyTitle')}</div>
              <p className="text-sm text-muted-foreground">{t('plaza.emptyDescription')}</p>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}

class ModelCardErrorBoundary extends React.Component<
  React.PropsWithChildren<{ fallbackLabel: string; fallbackMessage: string }>,
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: unknown) {
    console.error('Failed to render model plaza card', error);
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <Card className="min-h-64 border-border/70 bg-background shadow-none">
        <CardContent className="flex h-full flex-col justify-center gap-2 p-5">
          <h2 className="truncate text-lg font-medium" title={this.props.fallbackLabel}>
            {this.props.fallbackLabel}
          </h2>
          <p className="text-sm text-muted-foreground">{this.props.fallbackMessage}</p>
        </CardContent>
      </Card>
    );
  }
}

function FilterRow({
  title,
  items,
  value,
  onChange,
  loading = false,
}: {
  title: string;
  items: ReadonlyArray<{ label: string; value: string; iconKey?: string }>;
  value: string;
  onChange: (value: string) => void;
  loading?: boolean;
}) {
  return (
    <section className="flex flex-col gap-2 sm:flex-row sm:items-start">
      <div className="w-20 shrink-0 pt-1 text-sm font-medium text-foreground">{title}</div>
      <div className="flex min-w-0 flex-wrap gap-1.5">
        {items.map(item => (
          <button
            key={item.value}
            type="button"
            onClick={() => onChange(item.value)}
            className={cn(
              'inline-flex items-center whitespace-nowrap rounded-md border px-2.5 py-1 text-sm transition-colors',
              value === item.value
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-transparent bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
            )}
          >
            {item.iconKey ? (
              <ProviderIcon provider={item.iconKey} size={16} className="mr-1.5 shrink-0" />
            ) : null}
            {item.label}
          </button>
        ))}
        {loading ? <Loader2 className="mt-2 size-4 animate-spin text-muted-foreground" /> : null}
      </div>
    </section>
  );
}

function ModelCard({
  model,
  useCaseLabels,
  manufacturerLabel,
}: {
  model: ModelItem;
  useCaseLabels: Record<string, string>;
  manufacturerLabel?: string;
}) {
  const t = useT('models');
  const { locale } = useLocale();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = getBillingDisplaySettings(currentOrganization);
  const displayName = modelDisplayName(model);
  const manufacturer = modelManufacturer(model, locale);
  const displayManufacturer = manufacturerLabel ?? manufacturer.label;
  const experienceHref = getExperienceHref(model);
  const priceItems = getModelPriceDisplay({
    inputPrice: model.input_price,
    outputPrice: model.output_price,
    inputPriceConfigured: model.input_price_configured,
    outputPriceConfigured: model.output_price_configured,
    useCases: model.use_cases,
    currency: model.currency,
    pricing: model.pricing,
    billingDisplay,
    videoDisplayMode: 'summary',
    labels: {
      withVideoInput: t('plaza.withVideoInput'),
      withoutVideoInput: t('plaza.withoutVideoInput'),
      image: t('plaza.image'),
      input: t('plaza.input'),
      output: t('plaza.output'),
      speechGeneration: t('plaza.speechGeneration'),
      transcription: t('plaza.transcription'),
      musicGeneration: t('plaza.musicGeneration'),
      lyricsGeneration: t('plaza.lyricsGeneration'),
      meteredPrice: t('plaza.meteredPrice'),
      perImage: t('plaza.perImage'),
      perMillionTokens: t('plaza.perMillionTokens'),
      perTenThousandCharacters: t('plaza.perTenThousandCharacters'),
      perHour: t('plaza.perHour'),
      perTrack: t('plaza.perTrack'),
      perRequest: t('plaza.perRequest'),
      perQuantity: (quantity, unit) => t('plaza.perQuantity', { quantity, unit }),
      perUnit: unit => t('plaza.perUnit', { unit }),
    },
  });
  const structuredPricing = model.pricing as StructuredModelPricing | null | undefined;
  const videoRates = Array.isArray(structuredPricing?.video_generation?.resolution_rates)
    ? structuredPricing.video_generation.resolution_rates
    : [];
  const videoResolutions = Array.from(
    new Set(
      videoRates
        .filter(item => item !== null && typeof item === 'object' && !Array.isArray(item))
        .map(item => item.resolution)
        .filter((resolution): resolution is string => Boolean(resolution))
    )
  );
  const videoPriceVariesByResolution = [true, false].some(inputVideo => {
    const prices = videoRates.flatMap(item =>
      (Array.isArray(item?.rates) ? item.rates : [])
        .filter(rate => rate !== null && typeof rate === 'object' && !Array.isArray(rate))
        .filter(rate => rate.input_video === inputVideo)
        .map(rate => rate.price_per_million_tokens)
        .filter((price): price is number => typeof price === 'number' && Number.isFinite(price))
    );
    return new Set(prices).size > 1;
  });
  const featureLabels = [
    model.features?.function_calling ? t('plaza.functionCalling') : null,
    model.features?.vision ? t('plaza.vision') : null,
    model.features?.reasoning ? t('plaza.reasoning') : null,
    model.context_window
      ? t('plaza.context', { count: Math.round(model.context_window / 1000) })
      : null,
  ].filter(Boolean) as string[];
  const capabilityLabels = Array.from(
    new Set([
      ...(model.use_cases ?? []).map(useCase => useCaseLabels[useCase] || useCase),
      ...featureLabels,
    ])
  );

  return (
    <Card className="flex h-full flex-col overflow-hidden border-border/70 bg-background shadow-none transition-colors hover:border-primary/30">
      <CardContent className="flex h-full flex-col gap-3 p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate text-lg font-medium" title={displayName}>
              {displayName}
            </h2>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 shrink-0 gap-1 px-2 text-xs text-muted-foreground"
            onClick={() => {
              void navigator.clipboard.writeText(
                typeof model.model === 'string' ? model.model : displayName
              );
              toast.success(t('plaza.copySuccess'));
            }}
            title={t('plaza.copyTitle')}
          >
            <Copy className="size-3.5" />
            {t('plaza.copy')}
          </Button>
        </div>

        <div className="flex min-h-6 flex-wrap gap-1.5">
          {model.is_recommended ? <Badge>{t('plaza.recommended')}</Badge> : null}
          {capabilityLabels.length > 0 ? (
            capabilityLabels.map(label => (
              <Badge key={label} variant="secondary">
                {label}
              </Badge>
            ))
          ) : (
            <Badge variant="subtle">{t('plaza.generalModel')}</Badge>
          )}
        </div>

        <div className="mt-auto border-y py-3.5">
          {videoResolutions.length > 0 ? (
            <div className="mb-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>
                {t('plaza.supportedResolutions', { resolutions: videoResolutions.join(' / ') })}
              </span>
              {videoPriceVariesByResolution ? (
                <span>{t('plaza.priceVariesByResolution')}</span>
              ) : null}
            </div>
          ) : null}
          <div
            className={cn(
              'grid gap-x-6 gap-y-3',
              priceItems.length > 1 ? 'grid-cols-2' : 'grid-cols-1'
            )}
          >
            {priceItems.map((item, index) => (
              <PriceStat
                key={`${item.label}-${item.detail ?? item.unit}-${index}`}
                label={
                  item.detail ||
                  (item.unit === 'perImage'
                    ? t('plaza.image')
                    : item.label === 'input'
                      ? t('plaza.input')
                      : item.label === 'output'
                        ? t('plaza.output')
                        : t('plaza.video'))
                }
                value={item.formattedValue}
                unit={
                  item.unit === 'perImage'
                    ? t('plaza.perImage')
                    : item.unit === 'perSecond'
                      ? t('plaza.perSecond')
                      : item.unit === 'perTask'
                        ? t('plaza.perTask')
                        : item.displayUnit ||
                          (item.unit === 'perMillionVideoTokens'
                            ? t('plaza.perMillionVideoTokens')
                            : t('plaza.perMillionTokens'))
                }
              />
            ))}
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 pt-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
            <span
              className="flex min-w-0 items-center gap-2"
              title={t('plaza.manufacturerTitle', { name: displayManufacturer })}
            >
              <ProviderIcon provider={manufacturer.iconKey} size={18} className="shrink-0" />
              <span className="truncate">{displayManufacturer}</span>
            </span>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button asChild size="sm" variant="ghost">
              <Link href={experienceHref}>{t('plaza.tryNow')}</Link>
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function PriceStat({ label, value, unit }: { label: string; value: string; unit: string }) {
  return (
    <div className="min-w-0">
      <div className="truncate text-xs text-muted-foreground" title={label}>
        {label}
      </div>
      <div className="mt-1 whitespace-nowrap text-base font-semibold tabular-nums sm:text-lg">
        {value}
      </div>
      <div className="mt-1 whitespace-nowrap text-xs text-muted-foreground">{unit}</div>
    </div>
  );
}
