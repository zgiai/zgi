'use client';

import React from 'react';
import Link from 'next/link';
import { ProviderIcon } from '@/components/common/provider-icon';
import { Badge } from '@/components/ui/badge';
import type { ProviderItem } from '@/services/types/provider';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { useT } from '@/i18n';
import { getProviderSidebarRuntimeState } from '@/utils/provider-runtime-state';

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
  availableModelCount,
  isActive,
  onMouseEnter,
}: ProviderSidebarItemProps): JSX.Element {
  const getProviderName = useProviderI18n();
  const t = useT('aiProviders');
  const state = getProviderSidebarRuntimeState(provider, availableModelCount);

  let status: { label: string; dotClassName: string };

  if (state === 'disabled') {
    status = {
      label: t('providersList.runtimeStates.disabled'),
      dotClassName: 'bg-muted-foreground/50',
    };
  } else if (state === 'no_catalog_models') {
    status = {
      label: t('providersList.runtimeStates.no_catalog_models'),
      dotClassName: 'bg-muted-foreground/50',
    };
  } else if (state === 'available_models') {
    status = {
      label: `${availableModelCount} ${t('providersList.runtimeStates.available_models')}`,
      dotClassName: 'bg-emerald-500',
    };
  } else if (state === 'pending_channels') {
    status = {
      label: t('providersList.runtimeStates.pending_channels'),
      dotClassName: 'bg-muted-foreground/50',
    };
  } else if (state === 'unknown') {
    status = {
      label: t('providersList.runtimeStates.unknown'),
      dotClassName: 'bg-muted-foreground/50',
    };
  } else {
    status = {
      label: t('providersList.runtimeStates.configured_no_models'),
      dotClassName: 'bg-amber-500',
    };
  }

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
            <Badge variant="outline" className="h-4 px-1 text-[9px] font-normal">
              {t('providersList.table.custom')}
            </Badge>
          ) : null}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground">
        <span className={`size-1.5 rounded-full ${status.dotClassName}`} />
        <span>{status.label}</span>
      </div>
    </Link>
  );
}
