'use client';

import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { useConnectorOAuthFlow } from '@/hooks/connectors/use-connector-oauth';

export function ConnectorOAuthFlowStatus({
  flowId,
  reconnectId,
}: {
  flowId: string;
  reconnectId?: string;
}) {
  const t = useT('connectors');
  const { query, flow, selectPage } = useConnectorOAuthFlow(flowId);
  return (
    <div className="space-y-3" aria-live="polite">
      {query.isLoading && <p role="status">{t('oauth.loading')}</p>}
      {query.isError && (
        <div role="alert" className="space-y-2">
          <p>{t('oauth.statusFailed')}</p>
          <Button variant="outline" onClick={() => void query.refetch()}>
            {t('actions.retry')}
          </Button>
        </div>
      )}
      {(flow?.status === 'pending' || flow?.status === 'processing') && (
        <p role="status" className="text-sm text-muted-foreground">
          {t('oauth.waiting')}
        </p>
      )}
      {flow?.status === 'select_page' && (
        <fieldset className="space-y-2" disabled={selectPage.isPending}>
          <legend className="mb-2 text-sm font-medium">{t('oauth.selectPage')}</legend>
          {flow.pages?.map(page => (
            <Button
              key={page.id}
              type="button"
              variant="outline"
              className="h-auto w-full whitespace-normal break-words text-left"
              onClick={() => selectPage.mutate(page.id)}
            >
              {page.name}
            </Button>
          ))}
          {!flow.pages?.length && <p className="text-sm">{t('oauth.noPages')}</p>}
          {selectPage.isError && (
            <p role="alert" className="text-sm text-destructive">
              {t('oauth.pageFailed')}
            </p>
          )}
        </fieldset>
      )}
      {flow?.status === 'failed' && (
        <p role="alert" className="text-sm text-destructive">
          {t('oauth.failed')}
        </p>
      )}
      {flow?.status === 'ready' && (
        <div className="space-y-3">
          <p role="status" className="text-sm">
            {t(
              reconnectId && flow.connection_id !== reconnectId
                ? 'oauth.changedIdentity'
                : 'oauth.ready'
            )}
          </p>
          {flow.connection_id && (
            <Button asChild variant="outline">
              <Link
                href={`/console/connectors/connections/${encodeURIComponent(flow.connection_id)}`}
              >
                {t('connections.manageConnection')}
              </Link>
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
