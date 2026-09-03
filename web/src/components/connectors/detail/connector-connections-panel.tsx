'use client';

import { useState } from 'react';
import Link from 'next/link';
import { ArrowRight, KeyRound, Plus } from 'lucide-react';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { isConnectorListed } from '@/components/connectors/connector-visibility';
import { ConnectorCreateDialog } from '@/components/connectors/setup/connector-create-dialog';
import { ConnectionChatDefaultButton } from '@/components/connectors/connections/connection-chat-default-card';
import {
  personalConnectionDisplayName,
  personalConnectionSecondaryIdentity,
} from '@/components/connectors/connections/personal-connections-model';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';

interface ConnectorConnectionsPanelProps {
  connector: ConnectorCatalogItem;
  connections: PersonalConnection[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
}

export function ConnectorConnectionsPanel({
  connector,
  connections,
  loading,
  error,
  onRetry,
}: ConnectorConnectionsPanelProps) {
  const t = useT('connectors');
  const [adding, setAdding] = useState(false);

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle>{t('connections.title')}</CardTitle>
              {!loading && !error ? (
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('connections.count', { count: connections.length })}
                </p>
              ) : null}
            </div>
            {isConnectorListed(connector.id) && (
              <Button size="sm" onClick={() => setAdding(true)}>
                <Plus aria-hidden="true" className="size-4" />
                {t('connections.addConnection')}
              </Button>
            )}
          </div>
          <p className="text-sm leading-5 text-muted-foreground">
            {t('connections.groupDescription')}
          </p>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="space-y-3" role="status" aria-label={t('connections.loading')}>
              <Skeleton className="h-20 rounded-lg" />
              <Skeleton className="h-20 rounded-lg" />
            </div>
          ) : error ? (
            <div className="rounded-lg border border-dashed px-4 py-6 text-center">
              <p className="text-sm text-muted-foreground">
                {t('connections.loadFailedDescription')}
              </p>
              <Button type="button" size="sm" variant="outline" className="mt-3" onClick={onRetry}>
                {t('actions.retry')}
              </Button>
            </div>
          ) : connections.length === 0 ? (
            <div className="rounded-lg border border-dashed px-4 py-6 text-center">
              <KeyRound aria-hidden="true" className="mx-auto size-6 text-muted-foreground" />
              <p className="mt-2 text-sm font-medium">{t('connections.emptyTitle')}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {t('connections.emptyDescription')}
              </p>
            </div>
          ) : (
            <div className="max-h-80 space-y-2 overflow-y-auto overscroll-contain pr-1">
              {connections.map(connection => {
                const name = personalConnectionDisplayName(
                  connection,
                  t('connections.unknownConnection')
                );
                const identity = personalConnectionSecondaryIdentity(
                  connection,
                  t('connections.unknownIdentity')
                );
                return (
                  <article key={connection.id} className="rounded-lg border p-3">
                    <div className="flex min-w-0 items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold">{name}</p>
                        <p className="mt-1 truncate text-xs text-muted-foreground">{identity}</p>
                      </div>
                      <ConnectorStatusBadge status={connection.state} />
                    </div>
                    <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                      <ConnectionChatDefaultButton connection={connection} showLabel />
                      <Button asChild size="sm" variant="ghost">
                        <Link
                          href={`/console/connectors/connections/${encodeURIComponent(connection.id)}`}
                        >
                          {t('connections.manageConnection')}
                          <ArrowRight aria-hidden="true" className="size-4" />
                        </Link>
                      </Button>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <ConnectorCreateDialog connector={connector} open={adding} onOpenChange={setAdding} />
    </div>
  );
}
