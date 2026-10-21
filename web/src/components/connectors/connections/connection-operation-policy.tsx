'use client';

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ChevronRight } from 'lucide-react';

import { useRef, useState } from 'react';
import { useLocale } from 'next-intl';
import { useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { connectorService } from '@/services/connector.service';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import { connectorRequestErrorDetails } from '@/hooks/connectors/connector-request-error';
import { invalidatePersonalConnectionQueries } from '@/hooks/connectors/connection-detail-query';
import { resolveConnectorLocalizedText } from '../connector-metadata';

export function ConnectionOperationPolicy({
  connection,
  connector,
}: {
  connection: PersonalConnection;
  connector: ConnectorCatalogItem;
}) {
  if (!['weixin', 'wecom'].includes(connection.connector_id) || !connection.allowed_operation_ids) {
    return null;
  }
  return (
    <PolicyForm
      key={`${connection.id}:${connection.revision}`}
      connection={connection}
      connector={connector}
    />
  );
}

function PolicyForm({
  connection,
  connector,
}: {
  connection: PersonalConnection;
  connector: ConnectorCatalogItem;
}) {
  const t = useT('connectors');
  const locale = useLocale();
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<string[]>(connection.allowed_operation_ids ?? []);
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'failed' | 'conflict'>('idle');
  const busy = useRef(false);
  const attempt = useRef<{ fingerprint: string; key: string }>();
  const accepted = new Set(connection.usable_operation_ids ?? []);
  const saved = new Set(connection.allowed_operation_ids ?? []);
  const added = connector.capabilities.filter(op => selected.includes(op.id) && !saved.has(op.id));
  const removed = connector.capabilities.filter(
    op => !selected.includes(op.id) && saved.has(op.id)
  );
  const changed =
    JSON.stringify([...selected].sort()) !==
    JSON.stringify([...(connection.allowed_operation_ids ?? [])].sort());
  async function save() {
    if (busy.current || !changed || connection.state !== 'ready' || status === 'conflict') return;
    busy.current = true;
    setStatus('saving');
    const ids = [...selected].sort();
    const fingerprint = JSON.stringify(ids);
    if (attempt.current?.fingerprint !== fingerprint) {
      attempt.current = { fingerprint, key: crypto.randomUUID() };
    }
    try {
      await connectorService.updateOperationPolicy(
        connection.id,
        { expected_revision: connection.revision, operation_ids: ids },
        attempt.current.key
      );
      setStatus('saved');
      invalidatePersonalConnectionQueries(queryClient);
    } catch (error) {
      const details = connectorRequestErrorDetails(error);
      setStatus(details.status === 409 ? 'conflict' : 'failed');
    } finally {
      busy.current = false;
    }
  }
  return (
    <Collapsible className="space-y-3 rounded-xl border p-4">
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          className="group h-auto max-w-full justify-start gap-2 whitespace-normal px-0 py-1 text-left hover:bg-transparent text-sm"
        >
          <ChevronRight
            aria-hidden="true"
            className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-90"
          />
          <span className="min-w-0">{t('operationPolicy.title')}</span>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('operationPolicy.notice')}</p>
        <div className="grid gap-2 sm:grid-cols-2">
          {connector.capabilities.map(op => (
            <Label
              key={op.id}
              htmlFor={`connector-operation-${connection.id}-${op.id}`}
              className="flex items-start gap-2 rounded-lg border p-3 text-sm font-normal"
            >
              <Checkbox
                id={`connector-operation-${connection.id}-${op.id}`}
                checked={selected.includes(op.id)}
                disabled={
                  !accepted.has(op.id) ||
                  connection.state !== 'ready' ||
                  status === 'saving' ||
                  status === 'saved' ||
                  status === 'conflict'
                }
                onCheckedChange={checked => {
                  setSelected(current =>
                    checked === true ? [...current, op.id] : current.filter(id => id !== op.id)
                  );
                  setStatus('idle');
                }}
              />
              <span className="min-w-0 break-words">
                {resolveConnectorLocalizedText(op.name_i18n, locale, op.name)}
                {!accepted.has(op.id) && (
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {t('operationPolicy.notAccepted')}
                  </span>
                )}
              </span>
            </Label>
          ))}
        </div>
        {selected.length === 0 && (
          <p className="text-sm text-warning">{t('operationPolicy.paused')}</p>
        )}
        {changed && (
          <div className="space-y-1 break-words text-sm" aria-live="polite">
            {added.length > 0 && (
              <p>
                {t('operationPolicy.added', {
                  operations: added
                    .map(op => resolveConnectorLocalizedText(op.name_i18n, locale, op.name))
                    .join(', '),
                })}
              </p>
            )}
            {removed.length > 0 && (
              <p>
                {t('operationPolicy.removed', {
                  operations: removed
                    .map(op => resolveConnectorLocalizedText(op.name_i18n, locale, op.name))
                    .join(', '),
                })}
              </p>
            )}
          </div>
        )}
        {status === 'failed' && (
          <p role="alert" className="text-sm text-destructive">
            {t('operationPolicy.failed')}
          </p>
        )}
        {status === 'conflict' && (
          <p role="alert" className="text-sm text-destructive">
            {t('operationPolicy.conflict')}
          </p>
        )}
        {status === 'saved' && (
          <p role="status" className="text-sm">
            {t('operationPolicy.saved')}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={
              !changed ||
              connection.state !== 'ready' ||
              status === 'saving' ||
              status === 'saved' ||
              status === 'conflict'
            }
            onClick={() => void save()}
          >
            {t('operationPolicy.save')}
          </Button>
          {(status === 'conflict' || status === 'failed') && (
            <Button
              variant="outline"
              onClick={() => invalidatePersonalConnectionQueries(queryClient)}
            >
              {t('actions.retry')}
            </Button>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
