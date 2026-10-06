'use client';

import { useState } from 'react';
import { useLocale } from 'next-intl';
import { personalConnectionDisplayName } from './connections/personal-connections-model';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
  const locale = useLocale();
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
  const connectorName = (connectorID: string) =>
    catalog.data?.data.items.find(value => value.id === connectorID)?.name ?? connectorID;
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
        const connectorLabel = connectorName(item.label);
        const targetLabel = t(
          `usage.surfaces.${item.target.type === 'digital_human' ? 'digitalHuman' : item.target.type}`
        );
        return (
          <div key={item.id} className="space-y-2 rounded-md border p-2">
            <div className="flex min-w-0 items-center justify-between gap-2">
              <p className="break-words font-medium">{connectorLabel || t('status.unavailable')}</p>
              <span className="shrink-0 text-xs text-muted-foreground">
                {targetLabel}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('references.originalConnection', { connection: item.former_reference_id })}
            </p>
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
                <Select
                  value={selection[item.id] ?? ''}
                  disabled={Boolean(pending)}
                  onValueChange={value => {
                    setSelection(previous => ({ ...previous, [item.id]: value }));
                    setOperation(previous => ({ ...previous, [item.id]: '' }));
                  }}
                >
                  <SelectTrigger aria-label={t('references.connection')}>
                    <SelectValue placeholder={t('references.connection')} />
                  </SelectTrigger>
                  <SelectContent>
                    {(connections.data?.data.items ?? [])
                      .filter(value => value.state === 'ready')
                      .map(value => (
                        <SelectItem key={value.id} value={value.id}>
                          {personalConnectionDisplayName(value, value.connector_id, locale)}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
                {target.type === 'workflow' ? (
                  <Select
                    value={operation[item.id] ?? ''}
                    disabled={Boolean(pending) || !selection[item.id]}
                    onValueChange={value =>
                      setOperation(previous => ({ ...previous, [item.id]: value }))
                    }
                  >
                    <SelectTrigger aria-label={t('references.operation')}>
                      <SelectValue placeholder={t('references.operation')} />
                    </SelectTrigger>
                    <SelectContent>
                      {operations.map(value => (
                        <SelectItem key={value.id} value={value.id}>
                          {value.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
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
