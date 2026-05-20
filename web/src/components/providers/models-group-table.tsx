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
import { ModelIcon } from '@lobehub/icons';
import type { ModelItem } from '@/services/types/model';
import { Pencil, SearchX, ShieldCheck, Trash2 } from 'lucide-react';
import { ModelFeatureIcon } from '@/components/model/model-feature-icon';
import { useLocale } from '@/hooks/use-locale';
import { formatTokens } from '@/utils/format';
import { getModelPriceDisplay } from '@/utils/model-price';
import { USE_CASE_BADGE_COLORS } from '@/config/model-colors';
import { useT, type AiProvidersKey } from '@/i18n';

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
}: ModelsGroupTableProps): JSX.Element {
  const headerState = headerAllSelected
    ? true
    : headerSomeSelected
      ? ('indeterminate' as const)
      : false;
  const router = useRouter();
  const t = useT();
  const { locale } = useLocale();
  const showSelectionColumn = !readOnly;
  const showEnabledColumn = !readOnly;
  const showActionsColumn = Boolean(onEditModel || onDeleteModel);
  const columnCount =
    6 + Number(showSelectionColumn) + Number(showEnabledColumn) + Number(showActionsColumn);

  const renderChannelStatus = (model: ModelItem) => {
    if (model.is_available) {
      return (
        <div className="space-y-1">
          <Badge variant="success">{t('aiProviders.models.channelStates.connected')}</Badge>
          <p className="text-xs text-muted-foreground">
            {t('aiProviders.models.channelHints.connected')}
          </p>
        </div>
      );
    }

    return (
      <div className="space-y-1">
        <Badge variant="warning">{t('aiProviders.models.channelStates.missing')}</Badge>
        <p className="text-xs text-muted-foreground">
          {t('aiProviders.models.channelHints.missing')}
        </p>
      </div>
    );
  };

  const renderPolicyControl = (model: ModelItem) => {
    const policyLabel = model.is_enabled
      ? t('aiProviders.models.policyStates.allowed')
      : t('aiProviders.models.policyStates.disabled');

    return (
      <div className="flex items-center justify-end gap-2">
        <Badge variant={model.is_enabled ? 'success' : 'outline'}>{policyLabel}</Badge>
        <Switch
          checked={model.is_enabled}
          onCheckedChange={checked => onToggleModel(model, checked as boolean)}
          disabled={
            togglingModel === model.model ||
            isTogglingAll ||
            isBatchToggling ||
            model.is_configured === false
          }
          className="data-[state=checked]:bg-green-600"
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
                <TableHead className="text-right">
                  {t('aiProviders.models.table.policy')}
                </TableHead>
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
                .sort((a, b) => (a.model_name || a.model).localeCompare(b.model_name || b.model))
                .map(m => (
                  <TableRow key={m.id}>
                    {showSelectionColumn && (
                      <TableCell>
                        <Checkbox
                          aria-label={m.model_name || m.model}
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
                            {m.model_name || m.model}
                            {m.zgi_official_available && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <ShieldCheck className="w-3.5 h-3.5 text-green-600 cursor-help" />
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
                        {m.use_cases?.map(uc => (
                          <Badge
                            key={uc}
                            variant="outline"
                            className={`text-[10px] px-1.5 py-0 leading-3 h-5 ${USE_CASE_BADGE_COLORS[uc] || ''}`}
                          >
                            {t(`aiProviders.models.usecases.${uc}`)}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-2">
                        {Object.entries(m.features || {})
                          .filter(([, enabled]) => enabled)
                          .slice(0, 6)
                          .map(([key]) => (
                            <Tooltip key={key}>
                              <TooltipTrigger asChild>
                                <span
                                  role="img"
                                  aria-label={t(
                                    `aiProviders.models.features.${key}` as AiProvidersKey
                                  )}
                                  className="inline-flex items-center justify-center size-6 rounded-md bg-accent/60"
                                >
                                  <ModelFeatureIcon feature={key} />
                                </span>
                              </TooltipTrigger>
                              <TooltipContent>
                                {t(`aiProviders.models.features.${key}` as AiProvidersKey)}
                              </TooltipContent>
                            </Tooltip>
                          ))}
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="bg-accent text-secondary-foreground text-sm px-2 py-0.5 rounded-full">
                        {formatTokens(m.context_window)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="space-y-1 text-sm">
                        {getModelPriceDisplay({
                          inputPrice: m.input_price,
                          outputPrice: m.output_price,
                          useCases: m.use_cases,
                          locale,
                        }).map(item => {
                          const unitKey =
                            `aiProviders.models.pricing.${item.unit}` as AiProvidersKey;
                          const labelKey =
                            `aiProviders.models.pricing.${item.label}` as AiProvidersKey;
                          const displayText =
                            item.formattedValue === '-'
                              ? item.formattedValue
                              : `${item.formattedValue}${t(unitKey)}`;

                          return (
                            <div key={item.label} className="flex items-center gap-1.5">
                              <span className="text-xs text-muted-foreground">{t(labelKey)}</span>
                              <span className="font-medium text-xs">{displayText}</span>
                            </div>
                          );
                        })}
                      </div>
                    </TableCell>
                    {showEnabledColumn && (
                      <TableCell className="text-right">{renderPolicyControl(m)}</TableCell>
                    )}
                    {showActionsColumn && (
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          {onEditModel && (
                            <Button
                              variant="ghost"
                              isIcon
                              className="h-8 w-8"
                              onClick={() => onEditModel(m)}
                            >
                              <span className="sr-only">Edit</span>
                              <Pencil className="w-4 h-4" />
                            </Button>
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
                ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
