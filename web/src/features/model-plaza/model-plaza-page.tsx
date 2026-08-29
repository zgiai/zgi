'use client';

import * as React from 'react';
import Link from 'next/link';
import { useQueries } from '@tanstack/react-query';
import { Activity, BadgeCheck, Brain, Copy, Loader2, Search, Sparkles } from 'lucide-react';
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

const USE_CASE_FILTERS: Array<{ label: string; value: 'all' | ModelUseCase }> = [
  { label: '全部', value: 'all' },
  { label: '文本对话', value: 'text-chat' },
  { label: '视觉理解', value: 'vision' },
  { label: '图像生成', value: 'image-gen' },
  { label: '向量嵌入', value: 'embedding' },
  { label: '重排序', value: 'rerank' },
  { label: '语音识别', value: 'speech-to-text' },
  { label: '语音合成', value: 'text-to-speech' },
  { label: '推理增强', value: 'reasoning' },
  { label: '函数调用', value: 'function-calling' },
];

const USE_CASE_LABELS: Record<string, string> = Object.fromEntries(
  USE_CASE_FILTERS.filter(item => item.value !== 'all').map(item => [item.value, item.label])
);

const PROVIDER_SEARCH_ALIASES: Record<string, string[]> = {
  doubao: ['豆包', '火山', '火山引擎', 'volcengine', 'bytedance', '字节'],
  siliconflow: ['硅基流动'],
};

interface VideoResolutionRate {
  resolution?: string | null;
  rates?: Array<{
    input_video?: boolean | null;
    price_per_million_tokens?: number | null;
  }> | null;
}

interface StructuredModelPricing {
  video_generation?: {
    resolution_rates?: VideoResolutionRate[] | null;
  } | null;
}

function normalizeSearchValue(value: string): string {
  return value.trim().toLowerCase();
}

function getProviderAliases(provider: string, label?: string, rawName?: string): string[] {
  return Array.from(
    new Set(
      [provider, label, rawName, ...(PROVIDER_SEARCH_ALIASES[provider] ?? [])].filter(Boolean)
    )
  ) as string[];
}

function modelDisplayName(model: ModelItem): string {
  return model.model_name || model.model || model.id;
}

function modelMatchesSearch(
  model: ModelItem,
  search: string,
  providerSearchValues: readonly string[] = []
): boolean {
  const keyword = normalizeSearchValue(search);
  if (!keyword) return true;

  const useCaseLabels = (model.use_cases ?? []).map(useCase => USE_CASE_LABELS[useCase] || useCase);
  const featureLabels = [
    model.features?.function_calling ? '函数调用 function_calling' : '',
    model.features?.vision ? '视觉 vision' : '',
    model.features?.reasoning ? '推理 reasoning' : '',
  ];

  return [
    model.model,
    model.model_name,
    model.provider,
    model.family,
    model.family_name,
    model.tagline,
    model.tier,
    ...providerSearchValues,
    ...useCaseLabels,
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
      use_case: useCase === 'all' ? undefined : useCase,
    });
    const list = response.data;
    totalPages =
      typeof list.total_pages === 'number' && list.total_pages > 0 ? list.total_pages : page;

    (list.items ?? []).map(normalizeModel).forEach(model => {
      const key = `${model.provider}:${model.model}`;
      if (seen.has(key)) return;
      seen.add(key);
      models.push(model);
    });

    page += 1;
  }

  return models;
}

function copyModelName(modelName: string) {
  void navigator.clipboard.writeText(modelName);
  toast.success('模型名称已复制');
}

function getExperienceHref(model: ModelItem): string {
  const params = `provider=${encodeURIComponent(model.provider)}&model=${encodeURIComponent(model.model)}`;
  const basePath = model.use_cases?.includes('image-gen')
    ? '/console/work/image'
    : '/console/work/chat';
  return `${basePath}?${params}`;
}

export function ModelPlazaPage() {
  const [search, setSearch] = React.useState('');
  const debouncedSearch = useDebouncedValue(search, 500);
  const [useCaseFilter, setUseCaseFilter] = React.useState<'all' | ModelUseCase>('all');
  const [providerFilter, setProviderFilter] = React.useState('all');
  const getProviderName = useProviderI18n();

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

    return [{ label: '全部', value: 'all', aliases: ['全部'] }, ...providers];
  }, [customProviders, getProviderName, officialProviders]);

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
    use_case: useCaseFilter === 'all' ? undefined : useCaseFilter,
  });
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
        return modelMatchesSearch(
          model,
          debouncedSearch,
          providerSearchMap.get(model.provider) ?? []
        );
      }),
    [
      allModels,
      providerFilter,
      providerModels,
      providerSearchMap,
      providerSearchModels,
      debouncedSearch,
      selectedProvider,
      shouldUseProviderSearch,
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
  const displayModelCount = debouncedSearch.trim() ? filteredModels.length : totalModels;

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
      toast.success('模型列表已刷新');
    } catch (err) {
      const message = err instanceof Error ? err.message : '请稍后再试';
      toast.error('刷新失败', { description: message });
    }
  }, [refetch]);

  return (
    <div className="flex min-h-full flex-col bg-muted/30">
      <div className="border-b bg-background/80">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 px-4 py-5 sm:px-6 lg:px-8">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
            <div>
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Sparkles className="size-4 text-primary" />
                <span>模型服务</span>
              </div>
              <h1 className="mt-1.5 text-2xl font-semibold tracking-tight">模型广场</h1>
              <p className="mt-1.5 max-w-2xl text-sm text-muted-foreground">
                查看当前平台可用模型，按价格、场景和提供方筛选，快速复制模型 ID 用于 API
                调用或工作流配置。
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" onClick={() => void handleRefresh()} disabled={isFetching}>
                {isFetching ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Activity className="size-4" />
                )}
                刷新
              </Button>
            </div>
          </div>

          <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder="搜索模型名称、提供方或能力"
              leftIcon={<Search />}
              className="h-9 bg-background"
            />
            <div className="rounded-md border bg-background px-3 py-1.5 text-sm text-muted-foreground">
              共 <span className="font-semibold text-foreground">{displayModelCount}</span> 个模型
            </div>
          </div>

          <FilterRow
            title="使用场景"
            items={USE_CASE_FILTERS}
            value={useCaseFilter}
            onChange={value => setUseCaseFilter(value as 'all' | ModelUseCase)}
          />
          <FilterRow
            title="提供方"
            items={providerFilters}
            value={providerFilter}
            onChange={setProviderFilter}
            loading={isProviderLoading || isProviderFetching}
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
                <ModelCard
                  key={`${model.provider}:${model.model}`}
                  model={model}
                  providerName={
                    providerFilters.find(item => item.value === model.provider)?.label ??
                    model.provider
                  }
                />
              ))}
            </div>
            {isFetching ? (
              <div className="flex justify-center">
                <div className="inline-flex items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" />
                  正在加载更多模型
                </div>
              </div>
            ) : null}
          </div>
        ) : (
          <Card>
            <CardContent className="flex flex-col items-center justify-center gap-3 py-16 text-center">
              <Brain className="size-10 text-muted-foreground" />
              <div className="text-base font-medium">暂无匹配模型</div>
              <p className="text-sm text-muted-foreground">调整筛选条件后再试。</p>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}

function FilterRow({
  title,
  items,
  value,
  onChange,
  loading = false,
}: {
  title: string;
  items: ReadonlyArray<{ label: string; value: string }>;
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
              'rounded-md border px-2.5 py-1 text-sm transition-colors',
              value === item.value
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-transparent bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground'
            )}
          >
            {item.label}
          </button>
        ))}
        {loading ? <Loader2 className="mt-2 size-4 animate-spin text-muted-foreground" /> : null}
      </div>
    </section>
  );
}

function ModelCard({ model, providerName }: { model: ModelItem; providerName?: string }) {
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = getBillingDisplaySettings(currentOrganization);
  const displayName = modelDisplayName(model);
  const providerDisplayName = providerName || model.provider || '未知提供方';
  const familyDisplayName = model.family_name || model.family || model.tier || '标准模型';
  const shouldShowFamily =
    familyDisplayName.trim().toLocaleLowerCase() !== providerDisplayName.trim().toLocaleLowerCase();
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
  });
  const structuredPricing = model.pricing as StructuredModelPricing | null | undefined;
  const videoResolutions = Array.from(
    new Set(
      (structuredPricing?.video_generation?.resolution_rates ?? [])
        .map(item => item.resolution)
        .filter((resolution): resolution is string => Boolean(resolution))
    )
  );
  const videoRates = structuredPricing?.video_generation?.resolution_rates ?? [];
  const videoPriceVariesByResolution = [true, false].some(inputVideo => {
    const prices = videoRates.flatMap(item =>
      (item.rates ?? [])
        .filter(rate => rate.input_video === inputVideo)
        .map(rate => rate.price_per_million_tokens)
        .filter((price): price is number => typeof price === 'number' && Number.isFinite(price))
    );
    return new Set(prices).size > 1;
  });
  const featureLabels = [
    model.features?.function_calling ? '函数调用' : null,
    model.features?.vision ? '视觉' : null,
    model.features?.reasoning ? '推理' : null,
    model.context_window ? `${Math.round(model.context_window / 1000)}K 上下文` : null,
  ].filter(Boolean) as string[];
  const capabilityLabels = Array.from(
    new Set([
      ...(model.use_cases ?? []).map(useCase => USE_CASE_LABELS[useCase] || useCase),
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
            onClick={() => copyModelName(model.model)}
            title="复制模型 ID"
          >
            <Copy className="size-3.5" />
            复制
          </Button>
        </div>

        <div className="flex min-h-6 flex-wrap gap-1.5">
          {model.is_recommended ? <Badge>推荐</Badge> : null}
          <Badge variant={model.is_available ? 'success' : 'subtle'}>
            {model.is_available ? '可用' : '不可用'}
          </Badge>
          {capabilityLabels.length > 0 ? (
            capabilityLabels.map(label => (
              <Badge key={label} variant="secondary">
                {label}
              </Badge>
            ))
          ) : (
            <Badge variant="subtle">通用模型</Badge>
          )}
        </div>

        <div className="mt-auto border-y py-3.5">
          {videoResolutions.length > 0 ? (
            <div className="mb-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>支持分辨率：{videoResolutions.join(' / ')}</span>
              {videoPriceVariesByResolution ? <span>价格随分辨率变化</span> : null}
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
                    ? '图像'
                    : item.label === 'input'
                      ? '输入'
                      : item.label === 'output'
                        ? '输出'
                        : '视频')
                }
                value={item.formattedValue}
                unit={
                  item.unit === 'perImage'
                    ? '/ 张'
                    : item.unit === 'perSecond'
                      ? '/ 秒'
                      : item.unit === 'perTask'
                        ? '/ 次'
                        : '/ 百万 tokens'
                }
              />
            ))}
          </div>
        </div>

        <div className="flex items-center justify-between gap-3 pt-1">
          <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
            <span className="flex min-w-0 items-center gap-2">
              <ProviderIcon provider={model.provider} size={18} className="shrink-0" />
              <span className="truncate">{providerDisplayName}</span>
            </span>
            {shouldShowFamily ? (
              <span
                className="flex min-w-0 items-center gap-1.5"
                title={`模型系列：${familyDisplayName}`}
              >
                <BadgeCheck className="size-4 shrink-0" />
                <span className="truncate">{familyDisplayName}</span>
              </span>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {model.is_available ? (
              <Button asChild size="sm" variant="ghost">
                <Link href={experienceHref}>立即体验</Link>
              </Button>
            ) : (
              <Button asChild size="sm" variant="ghost">
                <Link
                  href={`/dashboard/provider/${encodeURIComponent(model.provider)}?model=${encodeURIComponent(model.model)}`}
                >
                  去启用
                </Link>
              </Button>
            )}
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
