'use client';

import React from 'react';
import { ChevronDown, ChevronUp, RadioTower, SearchX } from 'lucide-react';
import { ModelIcon } from 'modelicons';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import type { ModelItem } from '@/services/types/model';
import { useLocale } from '@/hooks/use-locale';
import { useT, type AiProvidersKey } from '@/i18n';
import { useOrganizationStore } from '@/store/organization-store';
import { getBillingDisplaySettings } from '@/utils/billing-display';
import { formatTokens } from '@/utils/format';
import { getModelDisplayName } from '@/utils/model-label';
import { getModelPriceDisplay } from '@/utils/model-price';

const INITIAL_VISIBLE_MODELS = 5;

interface PendingModelsListProps {
  models: ModelItem[];
  selected: Set<string>;
  onSelectRow: (modelName: string, next: boolean) => void;
  headerAllSelected: boolean;
  headerSomeSelected: boolean;
  onHeaderToggle: () => void;
  onConnectModel: (model: ModelItem) => void;
  onConnectSelected: () => void;
  canManage: boolean;
  hasActiveFilters?: boolean;
  onClearFilters?: () => void;
}

export default function PendingModelsList({
  models,
  selected,
  onSelectRow,
  headerAllSelected,
  headerSomeSelected,
  onHeaderToggle,
  onConnectModel,
  onConnectSelected,
  canManage,
  hasActiveFilters = false,
  onClearFilters,
}: PendingModelsListProps): JSX.Element | null {
  const t = useT();
  const { locale } = useLocale();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = getBillingDisplaySettings(currentOrganization);
  const [expanded, setExpanded] = React.useState(false);

  const sortedModels = React.useMemo(
    () =>
      [...models].sort((a, b) =>
        getModelDisplayName(a, locale).localeCompare(getModelDisplayName(b, locale))
      ),
    [locale, models]
  );
  const visibleModels = expanded ? sortedModels : sortedModels.slice(0, INITIAL_VISIBLE_MODELS);
  const selectedCount = models.reduce(
    (count, model) => count + (selected.has(model.model) ? 1 : 0),
    0
  );
  const headerState = headerAllSelected
    ? true
    : headerSomeSelected
      ? ('indeterminate' as const)
      : false;
  const canToggleExpanded = sortedModels.length > INITIAL_VISIBLE_MODELS;

  if (models.length === 0 && !hasActiveFilters) return null;

  return (
    <section className="space-y-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0">
          <h3 className="text-base font-medium">
            {t('aiProviders.models.groups.extensible')} · {models.length}
          </h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('aiProviders.models.pending.description')}
          </p>
        </div>

        {canManage && models.length > 0 ? (
          <Button
            type="button"
            size="sm"
            variant={selectedCount > 0 ? 'default' : 'outline'}
            disabled={selectedCount === 0}
            onClick={onConnectSelected}
          >
            <RadioTower className="size-3.5" />
            {selectedCount > 0
              ? t('aiProviders.models.pending.connectSelectedCount', {
                  count: selectedCount,
                })
              : t('aiProviders.models.pending.connectSelected')}
          </Button>
        ) : null}
      </div>

      <div className="overflow-hidden rounded-lg border bg-background">
        {models.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 px-4 py-8 text-center">
            <SearchX className="size-5 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              {t('aiProviders.models.pending.noMatches')}
            </p>
            {onClearFilters ? (
              <Button type="button" size="sm" variant="secondary" onClick={onClearFilters}>
                {t('aiProviders.models.actions.clearFilters')}
              </Button>
            ) : null}
          </div>
        ) : (
          <>
            {canManage ? (
              <div className="flex items-center gap-2 border-b bg-muted/30 px-4 py-2.5">
                <Checkbox
                  aria-label={t('aiProviders.models.actions.selectAll')}
                  checked={headerState}
                  onCheckedChange={onHeaderToggle}
                />
                <span className="text-xs text-muted-foreground">
                  {t('aiProviders.models.pending.selectHint')}
                </span>
                <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                  {t('aiProviders.models.pending.selectionSummary', {
                    selected: selectedCount,
                    total: models.length,
                  })}
                </span>
              </div>
            ) : null}

            <div className="divide-y">
              {visibleModels.map(model => {
                const modelLabel = getModelDisplayName(model, locale);
                const visibleUseCases = model.use_cases?.slice(0, 2) ?? [];
                const remainingUseCases = Math.max((model.use_cases?.length ?? 0) - 2, 0);
                const priceItems = getModelPriceDisplay({
                  inputPrice: model.input_price,
                  outputPrice: model.output_price,
                  inputPriceConfigured: model.input_price_configured,
                  outputPriceConfigured: model.output_price_configured,
                  useCases: model.use_cases,
                  billingDisplay,
                });

                return (
                  <div
                    key={model.id}
                    className="grid grid-cols-[auto_minmax(0,1fr)_auto] gap-x-3 gap-y-2 px-4 py-3 lg:grid-cols-[auto_minmax(13rem,1.4fr)_minmax(11rem,1fr)_minmax(11rem,1fr)_auto] lg:items-center"
                  >
                    {canManage ? (
                      <Checkbox
                        aria-label={modelLabel}
                        checked={selected.has(model.model)}
                        onCheckedChange={checked => onSelectRow(model.model, Boolean(checked))}
                      />
                    ) : (
                      <span className="size-4" aria-hidden />
                    )}

                    <div className="flex min-w-0 items-center gap-2.5">
                      <ModelIcon model={model.model} size={28} />
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium text-foreground">
                          {modelLabel}
                        </div>
                        <div className="truncate text-xs text-muted-foreground">{model.model}</div>
                      </div>
                    </div>

                    <div className="col-start-2 flex flex-wrap items-center gap-1.5 lg:col-auto">
                      {visibleUseCases.map(useCase => (
                        <Badge
                          key={useCase}
                          variant="outline"
                          className="h-5 border-border bg-muted/30 px-1.5 py-0 text-[10px] font-normal text-muted-foreground shadow-none"
                        >
                          {t(`aiProviders.models.usecases.${useCase}` as AiProvidersKey)}
                        </Badge>
                      ))}
                      {remainingUseCases > 0 ? (
                        <Badge
                          variant="outline"
                          className="h-5 border-border bg-background px-1.5 py-0 text-[10px] font-normal text-muted-foreground shadow-none"
                        >
                          +{remainingUseCases}
                        </Badge>
                      ) : null}
                      <span className="text-xs tabular-nums text-muted-foreground">
                        {formatTokens(model.context_window)}
                      </span>
                    </div>

                    <div className="col-start-2 flex flex-wrap items-center gap-x-3 gap-y-1 lg:col-auto">
                      {priceItems.map(item => {
                        const unitKey = `aiProviders.models.pricing.${item.unit}` as AiProvidersKey;
                        const labelKey =
                          `aiProviders.models.pricing.${item.label}` as AiProvidersKey;
                        const displayText = !item.isConfigured
                          ? t('aiProviders.models.pricing.unconfigured')
                          : item.isFree
                            ? t('aiProviders.models.pricing.free')
                            : `${item.formattedValue}${t(unitKey)}`;

                        return (
                          <span key={item.label} className="text-xs">
                            <span className="text-muted-foreground">{t(labelKey)} </span>
                            <span
                              className={
                                item.isConfigured
                                  ? 'font-medium text-foreground'
                                  : 'font-medium text-amber-600 dark:text-amber-400'
                              }
                            >
                              {displayText}
                            </span>
                          </span>
                        );
                      })}
                    </div>

                    {canManage ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        className="col-start-3 row-start-1 lg:col-auto lg:row-auto"
                        onClick={() => onConnectModel(model)}
                      >
                        {t('aiProviders.models.actions.connectSource')}
                      </Button>
                    ) : (
                      <span className="col-start-3 row-start-1 text-xs text-muted-foreground lg:col-auto lg:row-auto">
                        {t('aiProviders.models.pending.adminRequired')}
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          </>
        )}
      </div>

      {canToggleExpanded ? (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-muted-foreground"
            onClick={() => setExpanded(value => !value)}
          >
            {expanded ? (
              <>
                {t('aiProviders.models.pending.collapse')}
                <ChevronUp className="size-3.5" />
              </>
            ) : (
              <>
                {t('aiProviders.models.pending.expandAll', {
                  count: sortedModels.length,
                })}
                <ChevronDown className="size-3.5" />
              </>
            )}
          </Button>
        </div>
      ) : null}
    </section>
  );
}
