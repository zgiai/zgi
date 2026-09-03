'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useLocale } from 'next-intl';
import { ArrowLeft, ArrowRight, LockKeyhole, Plus } from 'lucide-react';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { isConnectorListed } from '@/components/connectors/connector-visibility';
import {
  formatConnectorDateTime,
  localizeConnectorCatalogItem,
} from '@/components/connectors/connector-metadata';
import {
  ConnectorPageLoading,
  ConnectorPageMessage,
} from '@/components/connectors/connector-page-state';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { ConnectorCreateDialog } from '@/components/connectors/setup/connector-create-dialog';
import { Badge } from '@/components/ui/badge';
import { ConnectionChatDefaultButton } from './connection-chat-default-card';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import {
  groupPersonalConnections,
  personalConnectionDisplayName,
  personalConnectionSecondaryIdentity,
} from './personal-connections-model';

export function PersonalConnectionsPage() {
  const t = useT('connectors');
  const locale = useLocale();
  const [connectorFilter, setConnectorFilter] = useState('');
  const [addingConnectorID, setAddingConnectorID] = useState('');
  const connectionsQuery = usePersonalConnections();
  const catalogQuery = useConnectorCatalog();
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  const catalog = connectorCatalogItems(catalogQuery.data?.data).map(item =>
    localizeConnectorCatalogItem(item, locale)
  );
  const catalogByID = new Map(catalog.map(item => [item.id, item]));
  const addingConnector = catalogByID.get(addingConnectorID);
  const groups = groupPersonalConnections(connections);
  const visibleGroups = connectorFilter
    ? groups.filter(group => group.connectorID === connectorFilter)
    : groups;

  return (
    <div className="h-full overflow-y-auto bg-bg-canvas/50">
      <main className="mx-auto min-h-full w-full max-w-[1240px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <Button asChild variant="ghost" size="sm" className="mb-5 -ml-2">
          <Link href="/console/connectors">
            <ArrowLeft aria-hidden="true" className="size-4" />
            {t('actions.backToConnectors')}
          </Link>
        </Button>
        <header className="mb-6 space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight">{t('connections.title')}</h1>
          <p className="text-sm leading-6 text-muted-foreground">{t('connections.description')}</p>
          {!connectionsQuery.isLoading && !connectionsQuery.isError ? (
            <div className="flex flex-wrap gap-2 pt-1">
              <Badge variant="subtle">
                {t('connections.count', { count: connections.length })}
              </Badge>
              <Badge variant="outline">
                {t('connections.connectorCount', { count: groups.length })}
              </Badge>
            </div>
          ) : null}
        </header>

        {connectionsQuery.isLoading ? (
          <ConnectorPageLoading label={t('connections.loading')} />
        ) : connectionsQuery.isError ? (
          <ConnectorPageMessage
            kind="error"
            title={t('connections.loadFailedTitle')}
            description={t('connections.loadFailedDescription')}
            actionLabel={t('actions.retry')}
            onAction={() => void connectionsQuery.refetch()}
          />
        ) : connections.length === 0 ? (
          <ConnectorPageMessage
            kind="empty"
            title={t('connections.emptyTitle')}
            description={t('connections.emptyDescription')}
            actionLabel={t('actions.backToConnectors')}
            actionHref="/console/connectors"
          />
        ) : (
          <div className="space-y-5">
            <div className="flex flex-wrap gap-2" role="group" aria-label={t('filters.label')}>
              <Button
                size="sm"
                variant={connectorFilter === '' ? 'default' : 'outline'}
                onClick={() => setConnectorFilter('')}
                aria-pressed={connectorFilter === ''}
              >
                {t('filters.all')}
              </Button>
              {groups.map(group => (
                <Button
                  key={group.connectorID}
                  size="sm"
                  variant={connectorFilter === group.connectorID ? 'default' : 'outline'}
                  onClick={() => setConnectorFilter(group.connectorID)}
                  aria-pressed={connectorFilter === group.connectorID}
                >
                  {catalogByID.get(group.connectorID)?.name ?? group.connectorID}
                </Button>
              ))}
            </div>
            {visibleGroups.map(group => {
              const connector = catalogByID.get(group.connectorID);
              const connectorName = connector?.name ?? group.connectorID;
              return (
                <Card key={group.connectorID}>
                  <CardHeader className="flex-row flex-wrap items-center gap-3 space-y-0">
                    <ConnectorIcon
                      connectorId={group.connectorID}
                      name={connectorName}
                      icon={connector?.icon}
                      classification={connector?.classification ?? 'connector'}
                    />
                    <div className="min-w-0 flex-1">
                      <CardTitle>{connectorName}</CardTitle>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t('connections.count', { count: group.connections.length })}
                      </p>
                    </div>
                    {isConnectorListed(group.connectorID) && (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!connector}
                        onClick={() => setAddingConnectorID(group.connectorID)}
                      >
                        <Plus aria-hidden="true" className="size-4" />
                        {t('connections.addConnection')}
                      </Button>
                    )}
                  </CardHeader>
                  <CardContent className="divide-y">
                    {group.connections.map(connection => (
                      <article
                        key={connection.id}
                        className="flex min-w-0 flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center"
                      >
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-semibold">
                            {personalConnectionDisplayName(
                              connection,
                              t('connections.unknownConnection')
                            )}
                          </p>
                          <p className="mt-1 truncate text-sm text-muted-foreground">
                            {personalConnectionSecondaryIdentity(
                              connection,
                              t('connections.unknownIdentity')
                            )}
                          </p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {t('connections.updatedAt', {
                              date: formatConnectorDateTime(
                                connection.updated_at,
                                locale,
                                t('account.neverVerified')
                              ),
                            })}
                          </p>
                        </div>
                        <div className="flex shrink-0 items-center justify-between gap-3 sm:justify-end">
                          <ConnectionChatDefaultButton connection={connection} />
                          <ConnectorStatusBadge status={connection.state} />
                          <Button asChild size="sm" variant="outline">
                            <Link
                              href={`/console/connectors/connections/${encodeURIComponent(connection.id)}`}
                            >
                              {t('connections.manageConnection')}
                              <ArrowRight aria-hidden="true" className="size-4" />
                            </Link>
                          </Button>
                        </div>
                      </article>
                    ))}
                  </CardContent>
                </Card>
              );
            })}
          </div>
        )}
        <p className="mt-6 flex items-start gap-2 border-t pt-5 text-xs leading-5 text-muted-foreground">
          <LockKeyhole aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
          {t('setup.ownerOnly')}
        </p>
      </main>
      {addingConnector ? (
        <ConnectorCreateDialog
          connector={addingConnector}
          open
          onOpenChange={open => {
            if (!open) setAddingConnectorID('');
          }}
        />
      ) : null}
    </div>
  );
}
