'use client';

import React from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { ModelIcon } from 'modelicons';
import type { ModelItem } from '@/services/types/model';
import { Pencil, SearchX, ShieldCheck, Trash2 } from 'lucide-react';
import { ModelFeatureIcon } from '@/components/model/model-feature-icon';
import { useLocale } from '@/hooks/use-locale';
import { formatTokens } from '@/utils/format';
import { getBillingDisplaySettings } from '@/utils/billing-display';
import { getModelPriceDisplay } from '@/utils/model-price';
import { getModelDisplayName } from '@/utils/model-label';
import { useT, type AiProvidersKey } from '@/i18n';
import { useOrganizationStore } from '@/store/organization-store';

interface ModelsGroupTableProps {
  title: string;
  tooltip: string;
  IconSlot: React.ReactNode;
  models: ModelItem[];
  groupType: 'official' | 'extensible';
  selected: Set<string>;
  onSelectRow: (modelName: string, next: boolean) => void;
  headerAllSelected: boolean;
  headerSomeSelected: boolean;
  onHeaderToggle: () => void;
  isLoading: boolean;
  isTogglingAll: boolean;
  isBatchToggling: boolean;
  togglingModel: string | null;
  onToggleModel: (m: ModelItem, next: boolean) => void;
  searchQuery?: string;
  hasTypeFilter?: boolean;
  onClearFilters?: () => void;
  isCustom?: boolean;
  readOnly?: boolean;
  onEditModel?: (m: ModelItem) => void;
  onDeleteModel?: (m: ModelItem) => void;
  onCreateModel?: () => void;
  onConfigureChannel?: (m: ModelItem) => void;
}

export default function ModelsGroupTable({
  title,
  tooltip,
  IconSlot,
  models,
  groupType,
  selected,
  onSelectRow,
  headerAllSelected,
  headerSomeSelected,
  onHeaderToggle,
  isLoading,
  isTogglingAll,
  isBatchToggling,
  togglingModel,
  onToggleModel,
  searchQuery,
  hasTypeFilter,
  onClearFilters,
  readOnly = false,
  onEditModel,
  onDeleteModel,
  onCreateModel,
  onConfigureChannel,
}: ModelsGroupTableProps): JSX.Element {
  const headerState = headerAllSelected
    ? true
    : headerSomeSelected
      ? ('indeterminate' as const)
      : false;
  const router = useRouter();
  const t = useT();
  const { locale } = useLocale();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = getBillingDisplaySettings(currentOrganization);
  const showSelectionColumn = !readOnly;
  const showEnabledColumn = !readOnly;
  const showActionsColumn = Boolean(onEditModel || onDeleteModel);
  const columnCount =
    6 + Number(showSelectionColumn) + Number(showEnabledColumn) + Number(showActionsColumn);

  const renderChannelStatus = (model: ModelItem) => {
    const configureAction = onConfigureChannel ? (
      <Button
        type="button"
        variant="link"
        size="sm"
        className={`h-auto p-0 text-xs font-normal ${
          model.is_available ? 'text-muted-foreground hover:text-foreground' : ''
        }`}
        onClick={() => onConfigureChannel(model)}
      >
        {t(
          model.is_available
            ? 'aiProviders.models.actions.addSource'
            : 'aiProviders.models.actions.connectSource'
        )}
      </Button>
    ) : null;

    if (model.is_available) {
      return (
        <div className="space-y-1">
          <div className="flex items-center gap-2 text-xs font-medium text-foreground">
            <span className="size-1.5 rounded-full bg-emerald-500" />
            {t('aiProviders.models.channelStates.connected')}
          </div>
          {configureAction}
        </div>
      );
    }

    return (
      <div className="space-y-1">
        <div className="flex items-center gap-2 text-xs font-medium text-foreground">
          <span className="size-1.5 rounded-full bg-amber-500" />
          {t('aiProviders.models.channelStates.missing')}
        </div>
        {configureAction}
      </div>
    );
  };

  const renderPolicyControl = (model: ModelItem) => {
    const policyLabel = model.is_enabled
      ? t('aiProviders.models.policyStates.allowed')
      : t('aiProviders.models.policyStates.disabled');

    return (
      <div className="flex items-center justify-end gap-2">
        <span className="text-xs text-muted-foreground">{policyLabel}</span>
        <Switch
          checked={model.is_enabled}
          onCheckedChange={checked => onToggleModel(model, checked as boolean)}
          disabled={
            togglingModel === model.model ||
            isTogglingAll ||
            isBatchToggling ||
            model.is_configured === false
          }
        />
      </div>
    );
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <h3 className="text-base font-medium">{title}</h3>
        <Tooltip>
          <TooltipTrigger asChild>{IconSlot}</TooltipTrigger>
          <TooltipContent>{tooltip}</TooltipContent>
        </Tooltip>
        <div className="ml-auto">
          {groupType === 'extensible' && (
            <Button
              variant="link"
              size="sm"
              asChild
              className="h-auto p-0 text-highlight font-normal"
            >
              <Link href="/dashboard/channel">
                {t('aiProviders.models.actions.configureChannels')}
              </Link>
            </Button>
          )}
        </div>
      </div>
      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              {showSelectionColumn && (
                <TableHead className="w-8">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Checkbox
                        aria-label={t('aiProviders.models.actions.selectAll')}
                        checked={headerState}
                        className={
                          headerAllSelected
                            ? 'bg-[var(--checkbox-selected)] text-white border-[var(--checkbox-hover)]'
                            : undefined
                        }
                        onCheckedChange={() => onHeaderToggle()}
                      />
                    </TooltipTrigger>
                    <TooltipContent>{t('aiProviders.models.actions.selectAll')}</TooltipContent>
                  </Tooltip>
                </TableHead>
              )}
              <TableHead>{t('aiProviders.models.table.model')}</TableHead>
              <TableHead>{t('aiProviders.models.table.channelStatus')}</TableHead>
              <TableHead>{t('aiProviders.models.table.type')}</TableHead>
              <TableHead>{t('aiProviders.models.table.features')}</TableHead>
              <TableHead>{t('aiProviders.models.table.context')}</TableHead>
              <TableHead className="min-w-[12rem]">{t('aiProviders.models.table.price')}</TableHead>
              {showEnabledColumn && (
                <TableHead className="text-right">{t('aiProviders.models.table.policy')}</TableHead>
              )}
              {showActionsColumn && (
                <TableHead className="text-right w-24">
                  {t('aiProviders.models.table.actions') || 'Actions'}
                </TableHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && models.length === 0 ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={`skeleton-${i}`}>
                  {showSelectionColumn && (
                    <TableCell>
                      <Skeleton className="h-4 w-4 rounded" />
                    </TableCell>
                  )}
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-6 w-6 rounded" />
                      <div className="space-y-1">
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-3 w-48" />
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="space-y-1">
                      <Skeleton className="h-5 w-16 rounded-full" />
                      <Skeleton className="h-3 w-28" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-12 rounded-full" />
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Skeleton className="h-6 w-6 rounded" />
                      <Skeleton className="h-6 w-6 rounded" />
                      <Skeleton className="h-6 w-6 rounded" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-12 rounded-full" />
                  </TableCell>
                  <TableCell>
                    <div className="space-y-1">
                      <Skeleton className="h-4 w-28" />
                      <Skeleton className="h-4 w-24" />
                    </div>
                  </TableCell>
                  {showEnabledColumn && (
                    <TableCell className="text-right">
                      <Skeleton className="h-5 w-9 rounded-full ml-auto" />
                    </TableCell>
                  )}
                  {showActionsColumn && (
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        <Skeleton className="h-8 w-8 rounded" />
                        <Skeleton className="h-8 w-8 rounded" />
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              ))
            ) : models.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columnCount} className="py-16">
                  <div className="flex flex-col items-center justify-center text-center space-y-4">
                    <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center">
                      <SearchX className="w-6 h-6 text-muted-foreground" />
                    </div>
                    <div className="space-y-1">
                      <p className="text-sm font-medium text-foreground">
                        {t('aiProviders.models.empty.title')}
                      </p>
                      <p className="text-sm text-muted-foreground">
                        {searchQuery || hasTypeFilter
                          ? t('aiProviders.models.empty.noMatches')
                          : groupType === 'official'
                            ? t('aiProviders.models.empty.officialUnsupported')
                            : t('aiProviders.models.empty.extensibleUnavailable')}
                      </p>
                    </div>
                    {(searchQuery || hasTypeFilter) && onClearFilters && (
                      <Button size="sm" variant="secondary" onClick={onClearFilters}>
                        {t('aiProviders.models.actions.clearFilters')}
                      </Button>
                    )}
                    {!searchQuery && !hasTypeFilter && groupType === 'official' && (
                      <Button
                        size="sm"
                        className="mt-1"
                        onClick={() => router.push('/dashboard/channel')}
                      >
                        {t('aiProviders.models.actions.goToChannel')}
                      </Button>
                    )}
                    {!searchQuery &&
                      !hasTypeFilter &&
                      groupType === 'extensible' &&
                      onCreateModel && (
                        <Button size="sm" className="mt-1" onClick={onCreateModel}>
                          {t('aiProviders.models.empty.createCustomModel')}
                        </Button>
                      )}
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              models
                .sort((a, b) =>
                  getModelDisplayName(a, locale).localeCompare(getModelDisplayName(b, locale))
                )
                .map(m => {
                  const modelLabel = getModelDisplayName(m, locale);
                  const visibleUseCases = m.use_cases?.slice(0, 2) ?? [];
                  const remainingUseCases = Math.max((m.use_cases?.length ?? 0) - 2, 0);
                  const enabledFeatures = Object.entries(m.features || {}).filter(
                    ([, enabled]) => enabled
                  );
                  const visibleFeatures = enabledFeatures.slice(0, 3);
                  const remainingFeatures = Math.max(enabledFeatures.length - 3, 0);

                  return (
                    <TableRow key={m.id}>
                      {showSelectionColumn && (
                        <TableCell>
                          <Checkbox
                            aria-label={modelLabel}
                            checked={selected.has(m.model)}
                            onCheckedChange={checked => onSelectRow(m.model, Boolean(checked))}
                          />
                        </TableCell>
                      )}
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <ModelIcon model={m.model} size={24} />
                          <div>
                            <div className="flex items-center gap-1.5 font-medium text-sm">
                              {modelLabel}
                              {m.zgi_official_available && (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <ShieldCheck className="h-3.5 w-3.5 cursor-help text-muted-foreground" />
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    {t('aiProviders.models.tooltips.systemChannel')}
                                  </TooltipContent>
                                </Tooltip>
                              )}
                            </div>
                            <div className="text-xs text-muted-foreground">{m.model}</div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>{renderChannelStatus(m)}</TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {visibleUseCases.map(uc => (
                            <Badge
                              key={uc}
                              variant="outline"
                              className="h-5 border-border bg-muted/40 px-1.5 py-0 text-[10px] leading-3 text-muted-foreground shadow-none"
                            >
                              {t(`aiProviders.models.usecases.${uc}`)}
                            </Badge>
                          ))}
                          {remainingUseCases > 0 ? (
                            <Badge
                              variant="outline"
                              className="h-5 border-border bg-background px-1.5 py-0 text-[10px] leading-3 text-muted-foreground shadow-none"
                            >
                              +{remainingUseCases}
                            </Badge>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {visibleFeatures.map(([key]) => (
                            <Tooltip key={key}>
                              <TooltipTrigger asChild>
                                <span
                                  role="img"
                                  aria-label={t(
                                    `aiProviders.models.features.${key}` as AiProvidersKey
                                  )}
                                  className="inline-flex size-6 items-center justify-center rounded-md bg-muted/60 text-muted-foreground"
                                >
                                  <ModelFeatureIcon
                                    feature={key}
                                    colored={false}
                                    className="text-muted-foreground"
                                  />
                                </span>
                              </TooltipTrigger>
                              <TooltipContent>
                                {t(`aiProviders.models.features.${key}` as AiProvidersKey)}
                              </TooltipContent>
                            </Tooltip>
                          ))}
                          {remainingFeatures > 0 ? (
                            <span className="inline-flex h-6 items-center text-[10px] text-muted-foreground">
                              +{remainingFeatures}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell>
                        <span className="text-sm tabular-nums text-muted-foreground">
                          {formatTokens(m.context_window)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <div className="text-sm">
                          <div className="space-y-1.5">
                            {getModelPriceDisplay({
                              inputPrice: m.input_price,
                              outputPrice: m.output_price,
                              inputPriceConfigured: m.input_price_configured,
                              outputPriceConfigured: m.output_price_configured,
                              pricing: m.pricing,
                              currency: m.currency,
                              useCases: m.use_cases,
                              labels: {
                                unspecifiedResolution: t(
                                  `aiProviders.models.pricing.videoUnspecifiedResolution` as AiProvidersKey
                                ),
                                withVideoInput: t(
                                  `aiProviders.models.pricing.videoWithInput` as AiProvidersKey
                                ),
                                withoutVideoInput: t(
                                  `aiProviders.models.pricing.videoWithoutInput` as AiProvidersKey
                                ),
                              },
                              billingDisplay,
                            }).map((item, index) => {
                              const unitKey =
                                `aiProviders.models.pricing.${item.unit}` as AiProvidersKey;
                              const labelKey =
                                `aiProviders.models.pricing.${item.label}` as AiProvidersKey;
                              const displayText = !item.isConfigured
                                ? t('aiProviders.models.pricing.unconfigured')
                                : item.isFree
                                  ? t('aiProviders.models.pricing.free')
                                  : `${item.formattedValue}${item.displayUnit || t(unitKey)}`;

                              return (
                                <div
                                  key={`${item.label}-${item.detail ?? index}`}
                                  className="flex items-baseline gap-x-4 leading-5"
                                >
                                  <span className="w-36 shrink-0 text-xs text-muted-foreground">
                                    {item.detail || t(labelKey)}
                                  </span>
                                  <span
                                    className={`whitespace-nowrap text-xs font-medium tabular-nums ${
                                      item.isConfigured
                                        ? 'text-foreground'
                                        : 'text-amber-600 dark:text-amber-400'
                                    }`}
                                  >
                                    {displayText}
                                  </span>
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      </TableCell>
                      {showEnabledColumn && (
                        <TableCell className="text-right">{renderPolicyControl(m)}</TableCell>
                      )}
                      {showActionsColumn && (
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            {onEditModel && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    isIcon
                                    className="h-8 w-8"
                                    onClick={() => onEditModel(m)}
                                  >
                                    <span className="sr-only">
                                      {t('aiProviders.models.actions.edit')}
                                    </span>
                                    <Pencil className="w-4 h-4" />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  {t('aiProviders.models.actions.edit')}
                                </TooltipContent>
                              </Tooltip>
                            )}
                            {onDeleteModel && (
                              <Button
                                variant="ghost"
                                isIcon
                                className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                                onClick={() => onDeleteModel(m)}
                              >
                                <span className="sr-only">Delete</span>
                                <Trash2 className="w-4 h-4" />
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      )}
                    </TableRow>
                  );
                })
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
