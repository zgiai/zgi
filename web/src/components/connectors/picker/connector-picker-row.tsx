'use client';

import { ArrowRightLeft, Minus, Plus } from 'lucide-react';
import { useLocale } from 'next-intl';
import { personalConnectionDisplayName } from '../connections/personal-connections-model';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ConnectorPickerRow as ConnectorPickerRowModel } from './connector-picker-model';
import { ConnectorTaxonomyBadges } from '../connector-taxonomy-badges';

interface ConnectorPickerRowProps {
  row: ConnectorPickerRowModel;
  pending?: boolean;
  disabled?: boolean;
  onAction: (row: ConnectorPickerRowModel) => void;
}

export function ConnectorPickerRow({
  row,
  pending = false,
  disabled = false,
  onAction,
}: ConnectorPickerRowProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const isApp = Boolean(row.connector.extension_schema_version && row.connector.setup);
  const connectionName = row.connection
    ? personalConnectionDisplayName(row.connection, row.connector.name, locale)
    : undefined;
  const rowTitle = connectionName || row.connector.name;
  const summary = row.connection
    ? [
        row.connector.name,
        row.connection.external_display_name
          ? t('picker.connectedAs', { name: row.connection.external_display_name })
          : undefined,
      ]
        .filter(Boolean)
        .join(' · ')
    : row.connector.description;
  const isSwitch = row.state === 'ready' && Boolean(row.replacedBinding);
  const canDetach =
    row.state === 'added' || (row.hostUnavailable && row.binding?.status === 'active');
  const canUpdateUnavailable =
    row.state === 'unavailable' && Boolean(row.connection) && !row.hostUnavailable;

  const actionLabel = canDetach
    ? t('picker.actions.remove')
    : isSwitch
      ? t('picker.actions.switch')
      : row.state === 'ready'
        ? t('picker.actions.add')
        : row.state === 'reconnect' || canUpdateUnavailable
          ? t(isApp ? 'app.updateTitle' : 'picker.actions.updateKey')
          : row.state === 'not_connected'
            ? t('picker.actions.addConnection')
            : t('picker.actions.viewReason');

  return (
    <article className="flex min-w-0 flex-col gap-3 rounded-xl border bg-background p-3.5 shadow-xs sm:flex-row sm:items-center">
      <ConnectorIcon
        connectorId={row.connector.id}
        name={row.connector.name}
        icon={row.connector.icon}
        classification={row.connector.classification}
      />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h3 className="truncate text-sm font-semibold">{rowTitle}</h3>
          {row.state === 'added' ? (
            <Badge variant="success">{t('picker.states.inUse')}</Badge>
          ) : row.state === 'ready' ? (
            <Badge variant="subtle">{t('status.ready')}</Badge>
          ) : row.state === 'reconnect' ? (
            <Badge variant="warning">{t('status.reconnectRequired')}</Badge>
          ) : row.state === 'unavailable' ? (
            <Badge variant="subtle">{t('status.unavailable')}</Badge>
          ) : null}
        </div>
        <p className="mt-1 truncate text-xs leading-5 text-muted-foreground">{summary}</p>
        <ConnectorTaxonomyBadges
          item={row.connector}
          maxTags={2}
          className="mt-2 flex min-w-0 flex-wrap gap-1"
        />
      </div>
      <Button
        type="button"
        variant={row.state === 'not_connected' ? 'default' : 'outline'}
        className={cn(
          'w-full sm:w-32',
          row.state === 'reconnect' &&
            'border-warning/50 text-warning hover:border-warning hover:bg-warning/10 hover:text-warning'
        )}
        disabled={disabled || pending}
        loading={pending}
        aria-label={`${actionLabel}: ${rowTitle}`}
        onClick={() => onAction(row)}
      >
        {canDetach ? <Minus aria-hidden="true" className="size-4" /> : null}
        {isSwitch ? <ArrowRightLeft aria-hidden="true" className="size-4" /> : null}
        {row.state === 'not_connected' ? <Plus aria-hidden="true" className="size-4" /> : null}
        {actionLabel}
      </Button>
    </article>
  );
}
