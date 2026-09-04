'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useLocale } from 'next-intl';
import { useRouter } from 'next/navigation';
import { Activity, KeyRound, Unplug } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import {
  ConnectorPageLoading,
  ConnectorPageMessage,
  ConnectorReadinessBanner,
} from '@/components/connectors/connector-page-state';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import {
  useConnector,
  useDiagnosePersonalConnection,
  usePersonalConnection,
} from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import { ConnectionAccountCard } from './connection-account-card';
import { getConnectionManagementPageState } from './connection-management-model';
import { ConnectionMaintenanceCard } from './connection-maintenance-card';
import { ConnectionReconnectDialog } from './connection-reconnect-dialog';
import { ConnectionRemovalDialog } from './connection-removal-dialog';
import { ConnectionUsageList } from './connection-usage-list';
import { personalConnectionDisplayName } from './personal-connections-model';
import { supportsAppSetup } from '@/components/connectors/setup/app-setup-state';
import { supportsMessagingContract } from '@/components/connectors/setup/messaging-setup-state';
import { ConnectorAppPermissions } from '@/components/connectors/detail/connector-app-permissions';
import { ConnectorCapabilityList } from '@/components/connectors/detail/connector-capability-list';
import { ConnectionAppDialog } from './connection-app-dialog';

interface ConnectionManagementPageProps {
  connectionId: string;
  enabled: boolean;
  featureLoading?: boolean;
  renameInitiallyOpen?: boolean;
  credentialsInitiallyOpen?: boolean;
}

export function ConnectionManagementPage({
  connectionId,
  enabled,
  featureLoading = false,
  renameInitiallyOpen = false,
  credentialsInitiallyOpen = false,
}: ConnectionManagementPageProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const router = useRouter();
  const [reconnectOpen, setReconnectOpen] = useState(false);
  const [removalOpen, setRemovalOpen] = useState(false);
  const [permissionsOpen, setPermissionsOpen] = useState(false);
  const connectionQuery = usePersonalConnection(connectionId, enabled);
  const connection = connectionQuery.data?.data;
  const connectorQuery = useConnector(
    connection?.connector_id ?? '',
    enabled && Boolean(connection?.connector_id)
  );
  const rawConnector = connectorQuery.data?.data;
  const connector = rawConnector ? localizeConnectorCatalogItem(rawConnector, locale) : undefined;
  const credentialsOpened = useRef('');
  useEffect(() => {
    if (
      !credentialsInitiallyOpen ||
      !connection ||
      !connector ||
      credentialsOpened.current === connection.id
    )
      {return;}
    credentialsOpened.current = connection.id;
    setReconnectOpen(true);
    router.replace(`/console/connectors/connections/${encodeURIComponent(connection.id)}`);
  }, [credentialsInitiallyOpen, connection, connector, router]);
  const diagnoseMutation = useDiagnosePersonalConnection(connectionId);
  const pageState = getConnectionManagementPageState({
    enabled,
    featureLoading,
    isLoading: connectionQuery.isLoading,
    error: connectionQuery.error,
    connection,
  });

  if (pageState === 'loading') {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50">
        <main className="mx-auto min-h-full w-full max-w-[1240px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <ConnectorPageLoading label={t('connection.loading')} variant="connection" />
        </main>
      </div>
    );
  }

  if (pageState === 'disabled') {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <ConnectorPageMessage
            kind="disabled"
            title={t('title')}
            description={t('disabledDescription')}
          />
        </div>
      </div>
    );
  }

  if (pageState === 'error') {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <ConnectorPageMessage
            kind="error"
            title={t('connection.loadFailedTitle')}
            description={t('connection.loadFailedDescription')}
            actionLabel={t('actions.retry')}
            onAction={() => void connectionQuery.refetch()}
          />
        </div>
      </div>
    );
  }

  if (pageState === 'not_found' || !connection) {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <ConnectorPageMessage
            kind="not_found"
            title={t('connection.notFoundTitle')}
            description={t('connection.notFoundDescription')}
            actionLabel={t('actions.backToConnectors')}
            actionHref="/console/connectors"
          />
        </div>
      </div>
    );
  }

  const connectorName =
    connector?.name || connection.connector_id || t('connection.connectorFallback');
  const isMessaging = Boolean(connector && supportsMessagingContract(connector));
  const isApp = Boolean(connector && (supportsAppSetup(connector) || isMessaging));
  const canReconnect = Boolean(connector && (connector.key_fields.length > 0 || isApp));
  const runDiagnostics = async () => {
    try {
      await diagnoseMutation.mutateAsync();
      toast.success(t('feedback.diagnosed'));
    } catch {
      toast.error(t('feedback.diagnoseFailed'));
    }
  };
  const bannerTitle =
    connection.state === 'ready'
      ? t('connection.banner.readyTitle')
      : connection.state === 'reconnect_required'
        ? t(isApp ? 'app.reconnectTitle' : 'connection.banner.reconnectTitle')
        : t('connection.banner.unavailableTitle');
  const defaultBannerDescription =
    connection.state === 'ready'
      ? t('connection.banner.readyDescription')
      : connection.state === 'reconnect_required'
        ? t(isApp ? 'app.reconnectDescription' : 'connection.banner.reconnectDescription')
        : t('connection.banner.unavailableDescription');
  const bannerDescription = connection.diagnostic_code
    ? `${defaultBannerDescription} (${connection.diagnostic_code})`
    : defaultBannerDescription;

  return (
    <div className="h-full overflow-y-auto bg-bg-canvas/50">
      <main className="mx-auto min-h-full w-full max-w-[1240px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <div className="space-y-5">
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink asChild>
                  <Link href="/console/connectors">{t('detail.breadcrumb')}</Link>
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbLink asChild>
                  <Link href={`/console/connectors/${encodeURIComponent(connection.connector_id)}`}>
                    {connectorName}
                  </Link>
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>{t('connection.breadcrumbManage')}</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>

          <header className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-center">
            <ConnectorIcon
              connectorId={connection.connector_id}
              name={connectorName}
              icon={connector?.icon}
              classification={connector?.classification ?? 'connector'}
              size="lg"
            />
            <div className="min-w-0 flex-1">
              <h1 className="truncate text-2xl font-semibold tracking-tight">
                {personalConnectionDisplayName(connection, t('connections.unknownConnection'))}
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                {connectorName} · {t('connection.subtitle')}
              </p>
              <ConnectorStatusBadge status={connection.state} className="mt-2" />
            </div>
            <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
              <Button
                variant="outline"
                className="w-full sm:w-auto"
                loading={diagnoseMutation.isPending}
                aria-label={
                  diagnoseMutation.isPending
                    ? t('maintenance.diagnosing')
                    : t('maintenance.diagnoseAction')
                }
                onClick={() => void runDiagnostics()}
              >
                <Activity aria-hidden="true" className="size-4" />
                {t('maintenance.diagnoseTitle')}
              </Button>
              <Button
                className="w-full sm:w-auto"
                disabled={!canReconnect}
                onClick={() => setReconnectOpen(true)}
              >
                <KeyRound aria-hidden="true" className="size-4" />
                {t(isApp ? 'app.updateTitle' : 'maintenance.updateKeyTitle')}
              </Button>
            </div>
          </header>

          <ConnectorReadinessBanner
            status={connection.state}
            title={bannerTitle}
            description={bannerDescription}
          />

          <div className="grid items-stretch gap-5 lg:grid-cols-[minmax(280px,0.8fr)_minmax(0,1.7fr)]">
            <ConnectionAccountCard
              connection={connection}
              connectorName={connectorName}
              renameInitiallyOpen={renameInitiallyOpen}
            />
            <ConnectionUsageList bindings={connection.bindings} />
          </div>

          <ConnectionMaintenanceCard
            isApp={isApp}
            isMessaging={isMessaging}
            onConfigure={isApp && !isMessaging ? () => setPermissionsOpen(true) : undefined}
            canReconnect={canReconnect}
            diagnosing={diagnoseMutation.isPending}
            onReconnect={() => setReconnectOpen(true)}
            onDiagnose={() => void runDiagnostics()}
          />

          {isApp && connector && (
            <>
              <ConnectorCapabilityList capabilities={connection.capabilities} />
              {!isMessaging && (
                <ConnectorAppPermissions
                  connector={connector}
                  evidence={connection.provider_evidence}
                />
              )}
            </>
          )}

          <Card className="border-destructive/40 bg-destructive/[0.02]">
            <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center">
              <div className="min-w-0 flex-1">
                <h2 className="text-lg font-semibold text-destructive">{t('removal.trigger')}</h2>
                <p className="mt-1 text-sm leading-5 text-muted-foreground">
                  {t('removal.disconnectDescription', { count: connection.bindings.length })}
                </p>
              </div>
              <Button
                variant="outline"
                className="w-full border-destructive/50 text-destructive hover:bg-destructive/5 hover:text-destructive sm:w-44"
                onClick={() => setRemovalOpen(true)}
              >
                <Unplug aria-hidden="true" className="size-4" />
                {t('removal.trigger')}
              </Button>
            </CardContent>
          </Card>
        </div>
      </main>

      {connector && !isApp ? (
        <ConnectionReconnectDialog
          open={reconnectOpen}
          onOpenChange={setReconnectOpen}
          connectionId={connection.id}
          connector={connector}
        />
      ) : null}
      {connector && isApp && (
        <>
          <ConnectionAppDialog
            open={reconnectOpen}
            onOpenChange={setReconnectOpen}
            connector={connector}
            connection={connection}
          />
          <ConnectionAppDialog
            open={permissionsOpen}
            onOpenChange={setPermissionsOpen}
            connector={connector}
            connection={connection}
            mode="qr"
          />
        </>
      )}
      <ConnectionRemovalDialog
        open={removalOpen}
        onOpenChange={setRemovalOpen}
        connectionId={connection.id}
        onRemoved={() => router.replace('/console/connectors/connections')}
      />
    </div>
  );
}
