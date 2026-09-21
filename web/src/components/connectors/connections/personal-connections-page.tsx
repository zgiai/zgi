'use client';

import { useState } from 'react';
import { isMailProvider, selectMailProvider } from '../mail-display-model';
import { usePendingSetups } from '@/hooks/connectors/use-pending-setups';
import { ConnectorPendingRows } from '@/components/connectors/setup/connector-pending-rows';
import Link from 'next/link';
import { useLocale } from 'next-intl';
import { ArrowLeft, ArrowRight, Plus } from 'lucide-react';
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

export function PersonalConnectionsPage({
  mailOnly = false,
  preferredProvider,
}: { mailOnly?: boolean; preferredProvider?: string } = {}) {
  const t = useT('connectors');
  const locale = useLocale();
  const [connectorFilter, setConnectorFilter] = useState('');
  const [addingConnectorID, setAddingConnectorID] = useState('');
  const [pendingID, setPendingID] = useState<string>();
  const pendingQuery = usePendingSetups();
  const pendingItems = mailOnly ? [] : (pendingQuery.data?.data.items ?? []);
  const connectionsQuery = usePersonalConnections();
  const catalogQuery = useConnectorCatalog();
  const connections = personalConnectionItems(connectionsQuery.data?.data).filter(
    item => !mailOnly || isMailProvider(item.connector_id)
  );
  const catalog = connectorCatalogItems(catalogQuery.data?.data).map(item =>
    localizeConnectorCatalogItem(item, locale)
  );
  const catalogByID = new Map(catalog.map(item => [item.id, item]));
  const addingConnector =
    addingConnectorID === 'mail'
      ? selectMailProvider(catalog, preferredProvider)
      : catalogByID.get(addingConnectorID);
  const groups = groupPersonalConnections(connections);
  for (const item of pendingItems) {
    if (!groups.some(group => group.connectorID === item.connector_id)) {
      groups.push({ connectorID: item.connector_id, connections: [] });
    }
  }
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
          <h1 className="text-2xl font-semibold tracking-tight">
            {t(mailOnly ? 'mail.title' : 'connections.title')}
          </h1>
          {mailOnly && (
            <Button
              disabled={!selectMailProvider(catalog)}
              onClick={() => setAddingConnectorID('mail')}
            >
              <Plus className="size-4" />
              {t('connections.addConnection')}
            </Button>
          )}
          {!connectionsQuery.isLoading && !connectionsQuery.isError ? (
            <div className="flex flex-wrap gap-2 pt-1">
              <Badge variant="subtle">
                {t('connections.count', { count: connections.length })}
              </Badge>
              {pendingItems.length > 0 && (
                <Badge variant="outline">
                  {t('pending.count', { count: pendingItems.length })}
                </Badge>
              )}
              <Badge variant="outline">
                {t('connections.connectorCount', { count: groups.length })}
              </Badge>
            </div>
          ) : null}
        </header>

        {pendingQuery.isError && (
          <p className="mb-3 text-sm text-warning">
            {t('pending.loadFailed')}{' '}
            <Button variant="ghost" onClick={() => void pendingQuery.refetch()}>
              {t('actions.retry')}
            </Button>
          </p>
        )}
        {connectionsQuery.isLoading || pendingQuery.isLoading ? (
          <ConnectorPageLoading label={t('connections.loading')} />
        ) : connectionsQuery.isError ? (
          <ConnectorPageMessage
            kind="error"
            title={t('connections.loadFailedTitle')}
            description={t('connections.loadFailedDescription')}
            actionLabel={t('actions.retry')}
            onAction={() => void connectionsQuery.refetch()}
          />
        ) : connections.length === 0 && pendingItems.length === 0 ? (
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
                  {group.connectorID === 'mail'
                    ? t('mail.title')
                    : (catalogByID.get(group.connectorID)?.name ?? group.connectorID)}
                </Button>
              ))}
            </div>
            {visibleGroups.map(group => {
              const connector =
                group.connectorID === 'mail'
                  ? selectMailProvider(catalog)
                  : catalogByID.get(group.connectorID);
              const connectorName =
                group.connectorID === 'mail'
                  ? t('mail.title')
                  : (connector?.name ?? group.connectorID);
              return (
                <Card key={group.connectorID}>
                  <CardHeader className="flex-row flex-wrap items-center gap-3 space-y-0">
                    <ConnectorIcon
                      connectorId={group.connectorID}
                      name={connectorName}
                      icon={group.connectorID === 'mail' ? undefined : connector?.icon}
                      classification={connector?.classification ?? 'connector'}
                    />
                    <div className="min-w-0 flex-1">
                      <CardTitle>{connectorName}</CardTitle>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t('connections.count', { count: group.connections.length })}
                      </p>
                    </div>
                    {!mailOnly && isConnectorListed(group.connectorID) && (
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
                              t('connections.unknownConnection'),
                              locale
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
                    <ConnectorPendingRows
                      items={pendingItems.filter(item => item.connector_id === group.connectorID)}
                      onContinue={id => {
                        setPendingID(id);
                        setAddingConnectorID(group.connectorID);
                      }}
                    />
                  </CardContent>
                </Card>
              );
            })}
          </div>
        )}
      </main>
      {addingConnector ? (
        <ConnectorCreateDialog
          connector={addingConnector}
          pendingSetupID={pendingID}
          open
          onOpenChange={open => {
            if (!open) {
              setAddingConnectorID('');
              setPendingID(undefined);
            }
          }}
        />
      ) : null}
    </div>
  );
}
