'use client';

import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { connectorService } from '@/services/connector.service';
import type {
  ConnectorReferenceCommand,
  ConnectorTargetRef,
  ConnectorUnavailableReference,
} from '@/services/types/connector';

export interface ConnectorReferenceRepairProps {
  onReferenceCommand?: (command: ConnectorReferenceCommand) => void | Promise<void>;
  pendingReferenceCommands?: readonly ConnectorReferenceCommand[];
}

function referenceReasonKey(reason: string) {
  if (/oauth|organization|shared|unsupported|retired/.test(reason)) {
    return 'references.reasonUnsupported' as const;
  }
  if (/owner|permission|restriction|constraint|denied/.test(reason)) {
    return 'references.reasonAccess' as const;
  }
  if (/selection|import|snapshot|foreign/.test(reason)) {
    return 'references.reasonSelection' as const;
  }
  return 'references.reasonUnavailable' as const;
}

/** Hosts stage commands in their save transaction. Only Chat resolves directly. */
export function ConnectorUnavailableReferences({
  target,
  enabled = true,
  onReferenceCommand,
  pendingReferenceCommands = [],
}: ConnectorReferenceRepairProps & { target: ConnectorTargetRef; enabled?: boolean }) {
  const t = useT('connectors');
  const client = useQueryClient();
  const key = ['connectors', 'unavailable-references', target];
  const references = useQuery({
    queryKey: key,
    queryFn: () => connectorService.getUnavailableReferences(target),
    enabled: enabled && Boolean(target.id),
    retry: false,
  });
  const [selection, setSelection] = useState<Record<string, string>>({});
  const [operation, setOperation] = useState<Record<string, string>>({});
  const [pending, setPending] = useState<string>();
  const [failed, setFailed] = useState(false);
  const items = (references.data?.data.items ?? []).filter(
    item =>
      item.status === 'needs_replacement' &&
      (!target.binding_instance_id ||
        item.target.binding_instance_id === target.binding_instance_id)
  );
  const mutable =
    target.scope !== 'published' && (target.type === 'chat' || Boolean(onReferenceCommand));
  const connections = useQuery({
    queryKey: ['connectors', 'repair-connections'],
    queryFn: () => connectorService.getConnections(),
    enabled: enabled && mutable && items.length > 0,
  });
  const catalog = useQuery({
    queryKey: ['connectors', 'repair-catalog'],
    queryFn: () => connectorService.getCatalog(),
    enabled: enabled && mutable && items.length > 0,
  });
  const resolve = async (item: ConnectorUnavailableReference, action: 'replace' | 'detach') => {
    if (pending || !mutable) return;
    const command: ConnectorReferenceCommand = {
      reference_id: item.id,
      expected_revision: item.revision,
      action,
      ...(action === 'replace'
        ? { connection_id: selection[item.id], operation_id: operation[item.id] }
        : {}),
    };
    setPending(item.id);
    setFailed(false);
    try {
      if (target.type === 'chat') {
        await connectorService.resolveUnavailableReference(target, command);
        await client.invalidateQueries({ queryKey: ['connectors'] });
      } else {
        await onReferenceCommand?.(command);
      }
    } catch {
      setFailed(true);
    } finally {
      setPending(undefined);
    }
  };
  if (!enabled) return null;
  if (references.isError) {
    return (
      <div role="alert" className="p-2 text-sm">
        <p>{t('picker.loadFailedTitle')}</p>
        <Button variant="ghost" onClick={() => void references.refetch()}>
          {t('actions.retry')}
        </Button>
      </div>
    );
  }
  if (!items.length) return null;
  return (
    <section aria-label={t('references.title')} className="space-y-3 border-t p-3 text-sm">
      <p className="font-medium">{t('references.title')}</p>
      {failed ? <p role="alert">{t('picker.actionFailed')}</p> : null}
      {connections.isError || catalog.isError ? (
        <div role="alert">
          <p>{t('picker.loadFailedTitle')}</p>
          <Button
            variant="ghost"
            onClick={() => {
              void connections.refetch();
              void catalog.refetch();
            }}
          >
            {t('actions.retry')}
          </Button>
        </div>
      ) : null}
      {items.map(item => {
        const staged = pendingReferenceCommands.some(command => command.reference_id === item.id);
        const connection = connections.data?.data.items.find(
          value => value.id === selection[item.id]
        );
        const operations =
          catalog.data?.data.items
            .find(value => value.id === connection?.connector_id)
            ?.capabilities.filter(
              value => value.usable && connection?.usable_operation_ids?.includes(value.id)
            ) ?? [];
        return (
          <div key={item.id} className="space-y-2 rounded-md border p-2">
            <p className="break-words">{item.label || t('status.unavailable')}</p>
            <p className="text-xs text-muted-foreground">
              {t(referenceReasonKey(item.reason_code))}
            </p>
            <p className="text-xs text-muted-foreground">
              {t(
                staged
                  ? 'references.pendingSave'
                  : target.scope === 'published'
                    ? 'references.published'
                    : 'references.description'
              )}
            </p>
            {mutable && !staged ? (
              <>
                <select
                  aria-label={t('references.connection')}
                  value={selection[item.id] ?? ''}
                  className="w-full rounded border bg-background p-2"
                  onChange={event => {
                    setSelection(previous => ({ ...previous, [item.id]: event.target.value }));
                    setOperation(previous => ({ ...previous, [item.id]: '' }));
                  }}
                >
                  <option value="">{t('references.connection')}</option>
                  {(connections.data?.data.items ?? [])
                    .filter(value => value.state === 'ready')
                    .map(value => (
                      <option key={value.id} value={value.id}>
                        {value.name || value.external_display_name || value.connector_id}
                      </option>
                    ))}
                </select>
                {target.type === 'workflow' ? (
                  <select
                    aria-label={t('references.operation')}
                    className="w-full rounded border bg-background p-2"
                    value={operation[item.id] ?? ''}
                    onChange={event =>
                      setOperation(previous => ({ ...previous, [item.id]: event.target.value }))
                    }
                  >
                    <option value="">{t('references.operation')}</option>
                    {operations.map(value => (
                      <option key={value.id} value={value.id}>
                        {value.name}
                      </option>
                    ))}
                  </select>
                ) : null}
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    disabled={
                      Boolean(pending) ||
                      !selection[item.id] ||
                      (target.type === 'workflow' && !operation[item.id])
                    }
                    onClick={() => void resolve(item, 'replace')}
                  >
                    {t('references.replace')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={Boolean(pending)}
                    onClick={() => void resolve(item, 'detach')}
                  >
                    {t('references.detach')}
                  </Button>
                </div>
              </>
            ) : null}
          </div>
        );
      })}
    </section>
  );
}
