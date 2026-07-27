'use client';

import { useMemo, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Pagination } from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useT } from '@/i18n';
import {
  integrationCatalogItems,
  integrationConnectionItems,
  integrationExecutionItems,
  useIntegrationCatalog,
  useIntegrationConnections,
  useIntegrationExecutions,
} from '@/hooks';
import type { IntegrationExecutionStatus } from '@/services/types/integration';
import {
  containsOpaqueUUID,
  safeIntegrationDisplayText,
  safeOptionalIntegrationDisplayText,
} from './display-utils';
import { integrationCatalogID } from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';

const PAGE_SIZE = 20;

function executionStatusVariant(status: IntegrationExecutionStatus) {
  if (status === 'succeeded') return 'default' as const;
  if (status === 'failed' || status === 'timed_out') return 'destructive' as const;
  return 'secondary' as const;
}

export function IntegrationExecutionsPanel({ integrationId }: { integrationId?: string } = {}) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<'all' | IntegrationExecutionStatus>('all');
  const query = useIntegrationExecutions({
    page,
    limit: PAGE_SIZE,
    integration_id: integrationId,
    status: status === 'all' ? undefined : status,
  });
  const connectionsQuery = useIntegrationConnections({
    integration_id: integrationId,
    page: 1,
    limit: 100,
  });
  const catalogQuery = useIntegrationCatalog(true, 'organization');
  const executions = integrationExecutionItems(query.data?.data);
  const response = query.data?.data;
  const connectionNames = useMemo(
    () =>
      new Map(
        integrationConnectionItems(connectionsQuery.data?.data).map(connection => [
          connection.id,
          connection.name,
        ])
      ),
    [connectionsQuery.data?.data]
  );
  const catalogByID = useMemo(
    () =>
      new Map(
        integrationCatalogItems(catalogQuery.data?.data).map(provider => [
          integrationCatalogID(provider),
          provider,
        ])
      ),
    [catalogQuery.data?.data]
  );
  const total = response?.total ?? executions.length;
  const totalPages = Math.max(
    1,
    response?.total
      ? Math.ceil(response.total / (response.page_size || response.limit || PAGE_SIZE))
      : response?.has_more
        ? page + 1
        : page
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Alert className="max-w-2xl py-2">
          <AlertDescription className="text-xs">{t('executions.description')}</AlertDescription>
        </Alert>
        <Select
          value={status}
          onValueChange={value => {
            setStatus(value as 'all' | IntegrationExecutionStatus);
            setPage(1);
          }}
        >
          <SelectTrigger className="w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('executions.statusFilter')}</SelectItem>
            <SelectItem value="running">{t('executions.status.running')}</SelectItem>
            <SelectItem value="succeeded">{t('executions.status.succeeded')}</SelectItem>
            <SelectItem value="failed">{t('executions.status.failed')}</SelectItem>
            <SelectItem value="timed_out">{t('executions.status.timedOut')}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="overflow-x-auto rounded-lg border">
        <Table className="min-w-[1180px]">
          <TableHeader>
            <TableRow>
              <TableHead>{t('executions.table.time')}</TableHead>
              <TableHead>{t('executions.table.integration')}</TableHead>
              <TableHead>{t('executions.table.connection')}</TableHead>
              <TableHead>{t('executions.table.caller')}</TableHead>
              <TableHead>{t('executions.table.status')}</TableHead>
              <TableHead>{t('executions.table.duration')}</TableHead>
              <TableHead>{t('executions.table.cost')}</TableHead>
              <TableHead>{t('executions.table.requestId')}</TableHead>
              <TableHead>{t('executions.table.error')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isLoading ? (
              Array.from({ length: 5 }).map((_, row) => (
                <TableRow key={row}>
                  {Array.from({ length: 9 }).map((__, cell) => (
                    <TableCell key={cell}>
                      <Skeleton className="h-4 w-24" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : query.isError ? (
              <TableRow>
                <TableCell colSpan={9} className="h-32 text-center text-muted-foreground">
                  <p>{t('executions.loadFailed')}</p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-3"
                    onClick={() => void query.refetch()}
                  >
                    <RefreshCw className="size-4" />
                    {t('executions.retry')}
                  </Button>
                </TableCell>
              </TableRow>
            ) : executions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={9} className="h-40 text-center text-muted-foreground">
                  {t('executions.empty')}
                </TableCell>
              </TableRow>
            ) : (
              executions.map(execution => {
                const providerRequestId = safeOptionalIntegrationDisplayText(
                  execution.provider_request_id
                );
                const errorCode = safeOptionalIntegrationDisplayText(execution.error_code);
                const connectionName = execution.connection_id
                  ? connectionNames.get(execution.connection_id)
                  : null;
                const provider = catalogByID.get(execution.integration_id);
                const action = provider?.actions.find(item => item.id === execution.action_id);
                return (
                  <TableRow key={execution.id}>
                    <TableCell className="whitespace-nowrap">
                      {metadata.date(execution.created_at, t('executions.noValue'))}
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">
                        {provider
                          ? metadata.providerName(provider)
                          : metadata.providerName(execution.integration_id)}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {action
                          ? metadata.actionName(action)
                          : metadata.actionNameByID(execution.action_id, t('common.unknownAction'))}
                      </div>
                    </TableCell>
                    <TableCell>
                      {execution.connection_id
                        ? safeIntegrationDisplayText(
                            connectionName,
                            t('executions.unknownConnection')
                          )
                        : t('executions.noConnection')}
                    </TableCell>
                    <TableCell>{metadata.invokeFrom(execution.invoke_from)}</TableCell>
                    <TableCell>
                      <Badge variant={executionStatusVariant(execution.status)}>
                        {t(
                          execution.status === 'timed_out'
                            ? 'executions.status.timedOut'
                            : `executions.status.${execution.status}`
                        )}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {typeof execution.duration_ms === 'number'
                        ? metadata.duration(execution.duration_ms)
                        : t('executions.noValue')}
                    </TableCell>
                    <TableCell>
                      {typeof execution.cost_usd === 'number'
                        ? metadata.usd(execution.cost_usd)
                        : t('executions.noValue')}
                    </TableCell>
                    <TableCell>
                      <code
                        className="block max-w-40 truncate text-xs"
                        title={providerRequestId ?? undefined}
                      >
                        {providerRequestId ||
                          (containsOpaqueUUID(execution.provider_request_id)
                            ? t('executions.hiddenReference')
                            : t('executions.noValue'))}
                      </code>
                    </TableCell>
                    <TableCell>
                      <span
                        className="block max-w-40 truncate text-xs text-destructive"
                        title={errorCode ? metadata.error(errorCode) : undefined}
                      >
                        {errorCode ? metadata.error(errorCode) : t('executions.noValue')}
                      </span>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>

      {totalPages > 1 ? (
        <Pagination
          currentPage={page}
          totalPages={totalPages}
          total={total}
          pageSize={PAGE_SIZE}
          onPageChange={setPage}
          showInfo={Boolean(response?.total)}
        />
      ) : null}
    </div>
  );
}
