'use client';

import Link from 'next/link';
import { useLocale } from 'next-intl';
import { Info } from 'lucide-react';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import {
  ConnectorPageLoading,
  ConnectorPageMessage,
} from '@/components/connectors/connector-page-state';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import {
  personalConnectionItems,
  useConnector,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import { ConnectorCapabilityList } from './connector-capability-list';
import { ConnectorConnectionsPanel } from './connector-connections-panel';
import { ConnectorExampleList } from './connector-example-list';
import { ConnectorKeyRequirements } from './connector-key-requirements';
import { ConnectorAppPermissions } from './connector-app-permissions';
import { supportsAppSetup } from '@/components/connectors/setup/app-setup-state';

interface ConnectorDetailPageProps {
  connectorId: string;
  enabled: boolean;
  featureLoading?: boolean;
}

export function ConnectorDetailPage({
  connectorId,
  enabled,
  featureLoading = false,
}: ConnectorDetailPageProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const connectorQuery = useConnector(connectorId, enabled);
  const rawConnector = connectorQuery.data?.data;
  const connector = rawConnector
    ? localizeConnectorCatalogItem(rawConnector, locale, {
        description: t('metadata.descriptionUnavailable'),
        capabilityDescription: t('metadata.capabilityDescriptionUnavailable'),
      })
    : undefined;
  const connectionsQuery = usePersonalConnections(
    { connector_id: connectorId },
    enabled && Boolean(connector)
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);

  if (!enabled) {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <ConnectorPageMessage
            kind="disabled"
            title={t('title')}
            description={featureLoading ? t('description') : t('disabledDescription')}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto bg-bg-canvas/50">
      <main className="mx-auto min-h-full w-full max-w-[1240px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        {connectorQuery.isLoading ? (
          <ConnectorPageLoading label={t('catalog.loading')} variant="detail" />
        ) : null}
        {connectorQuery.isError ? (
          <ConnectorPageMessage
            kind="error"
            title={t('detail.loadFailedTitle')}
            description={t('detail.loadFailedDescription')}
            actionLabel={t('actions.retry')}
            onAction={() => void connectorQuery.refetch()}
          />
        ) : null}
        {connector ? (
          <div className="space-y-6">
            <Breadcrumb>
              <BreadcrumbList>
                <BreadcrumbItem>
                  <BreadcrumbLink asChild>
                    <Link href="/console/connectors">{t('detail.breadcrumb')}</Link>
                  </BreadcrumbLink>
                </BreadcrumbItem>
                <BreadcrumbSeparator />
                <BreadcrumbItem>
                  <BreadcrumbPage>{connector.name}</BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>

            <header className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-start">
              <ConnectorIcon
                connectorId={connector.id}
                name={connector.name}
                icon={connector.icon}
                classification={connector.classification}
                className="size-12"
              />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="text-2xl font-semibold tracking-tight">{connector.name}</h1>
                  <Badge variant="outline">
                    {connector.classification === 'data_source'
                      ? t('classification.dataSource')
                      : t('classification.connector')}
                  </Badge>
                  <Badge variant="subtle">
                    {t('connections.count', { count: connector.connection_summary.count })}
                  </Badge>
                </div>
                <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                  {connector.description}
                </p>
              </div>
            </header>

            <Alert>
              <Info aria-hidden="true" className="size-4" />
              <AlertTitle>{t('detail.pluginNoticeTitle')}</AlertTitle>
              <AlertDescription>
                {t(supportsAppSetup(connector) ? 'app.notice' : 'detail.pluginNoticeDescription')}
              </AlertDescription>
            </Alert>

            <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_380px]">
              <div className="order-1 space-y-6">
                <ConnectorCapabilityList capabilities={connector.capabilities} />
                {supportsAppSetup(connector) ? (
                  <ConnectorAppPermissions connector={connector} />
                ) : (
                  <ConnectorKeyRequirements fields={connector.key_fields} />
                )}
                <ConnectorExampleList examples={connector.examples ?? []} />
              </div>
              <aside className="order-2 lg:sticky lg:top-6">
                <ConnectorConnectionsPanel
                  connector={connector}
                  connections={connections}
                  loading={connectionsQuery.isLoading}
                  error={connectionsQuery.isError}
                  onRetry={() => void connectionsQuery.refetch()}
                />
              </aside>
            </div>
          </div>
        ) : null}
      </main>
    </div>
  );
}
