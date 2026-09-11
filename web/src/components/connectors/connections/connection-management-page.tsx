'use client';

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useLocale } from 'next-intl';
import { useRouter } from 'next/navigation';
import { Activity, ChevronRight, KeyRound, Unplug } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import {
  ConnectorPageLoading,
  ConnectorPageMessage,
} from '@/components/connectors/connector-page-state';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { Button } from '@/components/ui/button';
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
import { ConnectionReconnectDialog } from './connection-reconnect-dialog';
import { ConnectionRemovalDialog } from './connection-removal-dialog';
import { ConnectionUsageList } from './connection-usage-list';
import { personalConnectionDisplayName } from './personal-connections-model';
import { supportsAppSetup } from '@/components/connectors/setup/app-setup-state';
import { supportsMessagingContract } from '@/components/connectors/setup/messaging-setup-state';
import { ConnectorAppPermissions } from '@/components/connectors/detail/connector-app-permissions';
import { ConnectorCapabilityList } from '@/components/connectors/detail/connector-capability-list';
import { ConnectionAppDialog } from './connection-app-dialog';
import { supportsMailSetup } from '../setup/mail-setup-state';
import { connectionMailUsage, mailFailureMessage } from '../setup/mail-setup-state';
import { ConnectionOperationPolicy } from './connection-operation-policy';

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
    ) {
      return;
    }
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
  const isMail = Boolean(connector && supportsMailSetup(connector));
  const canReconnect = Boolean(connector && (connector.key_fields.length > 0 || isApp || isMail));
  const runDiagnostics = async () => {
    try {
      await diagnoseMutation.mutateAsync();
      toast.success(t('feedback.diagnosed'));
    } catch {
      toast.error(t('feedback.diagnoseFailed'));
    }
  };

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
                {personalConnectionDisplayName(
                  connection,
                  t('connections.unknownConnection'),
                  locale
                )}
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
                {t(
                  isMail
                    ? connectionMailUsage(connection) === 'read_send'
                      ? 'mail.reconnect'
                      : 'mail.upgradeReconnect'
                    : isMessaging
                      ? 'messaging.reauthorizeTitle'
                      : isApp
                        ? 'app.updateTitle'
                        : 'maintenance.updateKeyTitle'
                )}
              </Button>
            </div>
          </header>

          {connection.state !== 'ready' && (
            <p className="text-sm text-warning" role="status">
              {t(
                isMail && connection.diagnostic_code
                  ? mailFailureMessage(connection.diagnostic_code)
                  : connection.diagnostic_code === 'connector_provider_permission_missing'
                    ? 'connection.banner.permissionDescription'
                    : connection.diagnostic_code === 'connector_application_not_ready'
                      ? 'connection.banner.applicationDescription'
                      : connection.diagnostic_code === 'connector_resource_access_denied'
                        ? 'connection.banner.resourceDescription'
                        : connection.diagnostic_code === 'connector_rate_limited'
                          ? 'connection.banner.rateLimitDescription'
                          : connection.state === 'reconnect_required'
                            ? 'connection.banner.reconnectDescription'
                            : 'connection.banner.unavailableDescription'
              )}
            </p>
          )}
          {connection.state !== 'ready' && connection.diagnostic_code && (
            <Collapsible className="text-xs text-muted-foreground">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  className="group h-auto max-w-full justify-start gap-2 whitespace-normal px-0 py-1 text-left hover:bg-transparent text-xs"
                >
                  <ChevronRight
                    aria-hidden="true"
                    className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-90"
                  />
                  <span className="min-w-0">{t('connection.diagnosticDetails')}</span>
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="space-y-3">
                <code className="mt-2 block break-all">{connection.diagnostic_code}</code>
              </CollapsibleContent>
            </Collapsible>
          )}

          <div className="grid items-stretch gap-5 lg:grid-cols-[minmax(280px,0.8fr)_minmax(0,1.7fr)]">
            <ConnectionAccountCard
              connection={connection}
              connectorName={connectorName}
              renameInitiallyOpen={renameInitiallyOpen}
            />
            <ConnectionUsageList bindings={connection.bindings} />
          </div>

          {connector && <ConnectionOperationPolicy connection={connection} connector={connector} />}

          {isApp && !isMessaging && (
            <Button variant="outline" onClick={() => setPermissionsOpen(true)}>
              {t('app.configure')}
            </Button>
          )}

          {isMail && connectionMailUsage(connection) !== 'read_send' && (
            <p className="text-sm text-muted-foreground">
              {t('mail.currentUsage', { usage: t(`mail.${connectionMailUsage(connection)}`) })}
            </p>
          )}
          {(isApp || isMail) && connector && (
            <>
              <ConnectorCapabilityList capabilities={connection.capabilities} />
              {isApp && !isMessaging && (
                <ConnectorAppPermissions
                  connector={connector}
                  evidence={connection.provider_evidence}
                />
              )}
            </>
          )}

          <Button variant="ghost" className="text-destructive" onClick={() => setRemovalOpen(true)}>
            <Unplug aria-hidden="true" className="size-4" />
            {t('removal.trigger')}
          </Button>
        </div>
      </main>

      {connector && !isApp ? (
        <ConnectionReconnectDialog
          open={reconnectOpen}
          onOpenChange={setReconnectOpen}
          connectionId={connection.id}
          connector={connector}
          connection={connection}
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
        connectionName={personalConnectionDisplayName(
          connection,
          t('connections.unknownConnection'),
          locale
        )}
        accountIdentity={connection.external_account_id ?? connection.external_display_name}
        onRemoved={() => router.replace('/console/connectors/connections')}
      />
    </div>
  );
}
