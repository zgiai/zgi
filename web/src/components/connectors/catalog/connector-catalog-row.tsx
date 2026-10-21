'use client';

import Link from 'next/link';
import { useState } from 'react';
import { usePendingSetups } from '@/hooks/connectors/use-pending-setups';
import { ConnectorCreateDialog } from '../setup/connector-create-dialog';
import { supportsConnectorSetup } from '../setup/app-setup-state';
import { ArrowRight, ChevronDown, CircleAlert, CircleCheck, KeyRound, Plus } from 'lucide-react';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem } from '@/services/types/connector';
import { getConnectorCatalogRowState } from './catalog-model';
import { ConnectorTaxonomyBadges } from '../connector-taxonomy-badges';

export function ConnectorCatalogRow({ item }: { item: ConnectorCatalogItem }) {
  const t = useT('connectors');
  const [adding, setAdding] = useState(false);
  const pendingQuery = usePendingSetups(item.id);
  const pendingCount = pendingQuery.data?.data.total_pending ?? 0;
  const canConnect = supportsConnectorSetup(item);
  const hasConnections = item.connection_summary.count > 0;
  const rowState = getConnectorCatalogRowState(item);
  const actionLabel = hasConnections
    ? t('catalog.manageConnections')
    : rowState === 'not_connected'
      ? t('picker.actions.addConnection')
      : rowState === 'connected'
        ? t('catalog.manageConnections')
        : rowState === 'reconnect_required'
          ? t('picker.actions.updateKey')
          : t('actions.viewDetails');
  const actionHref = `/console/connectors/${encodeURIComponent(item.id)}`;
  const summary =
    item.connection_summary.count > 0
      ? t('catalog.connectedCount', { count: item.connection_summary.count })
      : t('picker.states.notConnected');
  const needsAttention =
    rowState === 'reconnect_required' ||
    rowState === 'unavailable' ||
    (item.connection_summary.needs_attention_count ?? 0) > 0;
  const statusHint =
    (item.connection_summary.needs_attention_count ?? 0) > 0
      ? t('catalog.needsAttentionCount', {
          count: item.connection_summary.needs_attention_count ?? 0,
        })
      : rowState === 'reconnect_required'
        ? t('status.reconnectRequired')
        : rowState === 'unavailable'
          ? t('status.unavailable')
          : '';

  return (
    <Card
      role="article"
      className="flex h-full min-w-0 flex-col gap-4 bg-background p-4 shadow-xs sm:p-5"
    >
      <div className="flex min-w-0 items-start gap-4">
        <ConnectorIcon
          connectorId={item.id}
          name={item.name}
          icon={item.icon}
          classification={item.classification}
        />
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-semibold" title={item.name}>
            <Link href={actionHref}>{item.name}</Link>
          </h3>
          <p
            className="mt-1 h-10 line-clamp-2 text-sm leading-5 text-muted-foreground"
            title={item.description}
          >
            {item.description}
          </p>
          <ConnectorTaxonomyBadges item={item} className="mt-2 flex min-w-0 flex-wrap gap-1.5" />
        </div>
      </div>
      <div className="mt-auto flex h-9 min-w-0 items-center justify-between gap-3">
        <div
          className={cn(
            'flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground',
            needsAttention && 'text-warning'
          )}
        >
          {needsAttention ? (
            <CircleAlert aria-hidden="true" className="size-3.5 shrink-0" />
          ) : rowState === 'connected' ? (
            <CircleCheck aria-hidden="true" className="size-3.5 shrink-0 text-success" />
          ) : (
            <KeyRound aria-hidden="true" className="size-3.5 shrink-0" />
          )}
          <div className="min-w-0">
            <p className="truncate" title={summary}>
              {summary}
              {pendingCount > 0 ? ` · ${t('pending.count', { count: pendingCount })}` : ''}
            </p>
            {statusHint ? (
              <p className="truncate text-[11px]" title={statusHint}>
                {statusHint}
              </p>
            ) : null}
          </div>
        </div>
        <div className="flex h-9 w-36 shrink-0">
          {hasConnections || pendingCount > 0 ? (
            <>
              <Button
                asChild
                variant="outline"
                className={cn('h-9 min-w-0 flex-1 px-3', canConnect && 'rounded-r-none')}
              >
                <Link
                  href={actionHref}
                  aria-label={`${t('catalog.manageConnections')}: ${item.name}`}
                >
                  <span className="truncate">
                    {t(hasConnections ? 'catalog.manageConnections' : 'pending.continue')}
                  </span>
                </Link>
              </Button>
              {canConnect && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="outline"
                      isIcon
                      className="h-9 w-9 rounded-l-none border-l-0"
                      aria-label={`${t('catalog.connectionActions')}: ${item.name}`}
                    >
                      <ChevronDown aria-hidden="true" className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => setAdding(true)}>
                      <Plus aria-hidden="true" className="size-4" />
                      {t('connections.addConnection')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </>
          ) : canConnect ? (
            <Button
              variant={rowState === 'not_connected' ? 'default' : 'outline'}
              className="h-9 w-full px-3"
              onClick={() => setAdding(true)}
              aria-label={`${t('picker.actions.addConnection')}: ${item.name}`}
            >
              <Plus aria-hidden="true" className="size-4 shrink-0" />
              <span className="truncate">{t('actions.connect')}</span>
            </Button>
          ) : (
            <Button
              asChild
              variant={rowState === 'not_connected' ? 'default' : 'outline'}
              className="h-9 w-full px-3"
            >
              <Link
                href={actionHref}
                title={actionLabel}
                aria-label={`${actionLabel}: ${item.name}`}
              >
                {rowState === 'not_connected' ? (
                  <Plus aria-hidden="true" className="size-4 shrink-0" />
                ) : null}
                <span className="truncate">{actionLabel}</span>
                {rowState !== 'not_connected' ? (
                  <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
                ) : null}
              </Link>
            </Button>
          )}
        </div>
      </div>
      {adding && <ConnectorCreateDialog connector={item} open onOpenChange={setAdding} />}
    </Card>
  );
}
