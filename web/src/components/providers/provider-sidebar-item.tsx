'use client';

import React from 'react';
import Link from 'next/link';
import { ProviderIcon } from '@/components/common/provider-icon';
import { Badge } from '@/components/ui/badge';
import type { ProviderItem } from '@/services/types/provider';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { useT } from '@/i18n';
import { getProviderRuntimeState } from '@/utils/provider-runtime-state';
import { CheckCircle2, AlertCircle, Boxes, X } from 'lucide-react';

export interface ProviderSidebarItemProps {
  /** Provider data */
  provider: ProviderItem;
  /** Count of truly available models for this provider */
  availableModelCount?: number;
  /** Whether this item is active/selected */
  isActive: boolean;
  /** Callback when mouse enters for prefetch */
  onMouseEnter?: () => void;
}

/**
 * Single provider item in the sidebar list
 */
export function ProviderSidebarItem({
  provider,
  availableModelCount = 0,
  isActive,
  onMouseEnter,
}: ProviderSidebarItemProps): JSX.Element {
  const getProviderName = useProviderI18n();
  const t = useT('aiProviders');
  const state = getProviderRuntimeState(provider, availableModelCount);

  const badgeContent =
    state === 'available_models'
      ? {
          icon: CheckCircle2,
          label: `${availableModelCount} ${t('providersList.runtimeStates.available_models')}`,
          className:
            'border-transparent bg-success/15 text-success shadow-none ring-1 ring-success/10',
        }
      : state === 'pending_channels'
        ? {
            icon: AlertCircle,
            label: t('providersList.runtimeStates.pending_channels'),
            className:
              'border-transparent bg-warning/15 text-warning shadow-none ring-1 ring-warning/10',
          }
        : state === 'no_catalog_models'
          ? {
            icon: Boxes,
            label: t('providersList.runtimeStates.no_catalog_models'),
            className:
              'border-border bg-muted/70 text-muted-foreground shadow-none ring-1 ring-border/60',
          }
          : {
              icon: X,
              label: t('providersList.runtimeStates.disabled'),
              className:
                'border-border bg-muted/70 text-muted-foreground shadow-none ring-1 ring-border/60',
            };

  const Icon = badgeContent.icon;

  return (
    <Link
      href={`/dashboard/provider/${encodeURIComponent(provider.provider)}`}
      onMouseEnter={onMouseEnter}
      className={`flex items-center justify-between rounded-md border p-2 gap-2 transition-colors bg-background ${
        isActive
          ? 'bg-highlight/10 border-highlight/50'
          : 'hover:bg-highlight/5 hover:border-highlight/30'
      }`}
    >
      <ProviderIcon provider={provider.provider} size={24} />
      <div className="w-0 grow min-w-0">
        <div className="text-xs font-medium truncate">
          {getProviderName(provider.provider, provider.provider_name)}
        </div>
        <div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <span className="truncate">
            {t('providersList.sidebar.modelCount', { count: provider.model_count ?? 0 })}
          </span>
          {provider.provider_type === 'custom' ? (
            <Badge variant="info">{t('providersList.table.custom')}</Badge>
          ) : null}
        </div>
      </div>
      <Badge className={`h-6 px-2.5 text-[11px] font-semibold ${badgeContent.className}`}>
        <Icon className="h-3 w-3" />
        {badgeContent.label}
      </Badge>
    </Link>
  );
}
