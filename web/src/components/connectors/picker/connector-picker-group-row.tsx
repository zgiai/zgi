'use client';

import { useRef, type ReactNode } from 'react';
import { useLocale } from 'next-intl';
import { personalConnectionDisplayName } from '../connections/personal-connections-model';
import { ChevronDown, CircleAlert, KeyRound, Minus, Plus, Star } from 'lucide-react';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { ConnectorTaxonomyBadges } from '../connector-taxonomy-badges';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ConnectorChatDefault, ConnectorPendingSetup } from '@/services/types/connector';
import {
  connectorPickerGroupSummary,
  type ConnectorPickerMenuGroup,
} from './connector-picker-menu-model';
import type { ConnectorPickerBindingRef, ConnectorPickerRow } from './connector-picker-model';

interface ConnectorPickerGroupRowProps {
  group: ConnectorPickerMenuGroup;
  hideCreate?: boolean;
  targetLabel: string;
  disabled: boolean;
  preference?: ConnectorChatDefault;
  renderConnectionDetails?: (row: ConnectorPickerRow) => ReactNode;
  onSelect: (row: ConnectorPickerRow, trigger?: HTMLButtonElement | null) => void;
  onDetach: (group: ConnectorPickerMenuGroup, binding?: ConnectorPickerBindingRef) => void;
  pendingSetups?: ConnectorPendingSetup[];
  onResume?: (id: string) => void;
}

export function ConnectorPickerGroupRow({
  group,
  hideCreate = false,
  targetLabel,
  disabled,
  preference,
  renderConnectionDetails,
  onSelect,
  onDetach,
  pendingSetups = [],
  onResume,
}: ConnectorPickerGroupRowProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const handoffRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const createRow = hideCreate ? undefined : group.createRow;
  const multiple = group.selectionMode === 'multiple-connections';
  const groupName = group.name || t('connection.connectorFallback');
  const summary = connectorPickerGroupSummary(group, preference);
  const currentAccount = summary.currentAccount;
  const currentName =
    (multiple && summary.selectedCount > 1
      ? t('picker.group.selectedCount', { count: summary.selectedCount })
      : summary.name) ||
    t(currentAccount ? 'connections.unknownConnection' : 'picker.group.currentMissing');
  const availabilityLabel =
    summary.bindingStatus === 'disabled'
      ? t('usage.status.disabled')
      : summary.bindingStatus === 'needs_replacement'
        ? t('usage.status.needsReplacement')
        : summary.availability === 'ready'
          ? t('status.ready')
          : summary.availability === 'reconnect_required'
            ? t('status.reconnectRequired')
            : t('status.unavailable');
  const selectAccount = (row: ConnectorPickerRow) => {
    // Switch/repair/setup can open a dialog which takes ownership of focus.
    handoffRef.current =
      row.state !== 'added' && (row.state !== 'ready' || Boolean(row.replacedBinding));
    onSelect(row, triggerRef.current);
  };
  const summaryClass =
    'h-[54px] w-full min-w-0 shrink-0 justify-between rounded-lg px-3 md:col-start-3';

  return (
    <article className="min-w-0 rounded-xl border bg-background p-3.5 shadow-xs">
      <div className="grid min-w-0 grid-cols-[40px_minmax(0,1fr)] items-center gap-3 sm:flex sm:min-h-[62px] sm:gap-4">
        <ConnectorIcon
          connectorId={group.id}
          name={groupName}
          icon={group.icon}
          classification={group.classification ?? 'connector'}
        />
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-semibold" title={groupName}>
            {groupName}
          </h3>
          <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
            {group.unavailable
              ? t('picker.group.serviceUnavailable')
              : group.connector?.description}
          </p>
          {group.connector ? (
            <ConnectorTaxonomyBadges
              item={group.connector}
              maxTags={2}
              className="mt-2 flex min-w-0 flex-wrap gap-1"
            />
          ) : null}
        </div>
        {group.count > 0 || group.currentBinding || pendingSetups.length > 0 ? (
          <DropdownMenu
            onOpenChange={next => {
              if (next) handoffRef.current = false;
            }}
          >
            <DropdownMenuTrigger asChild>
              <Button
                ref={triggerRef}
                variant="outline"
                className={cn(
                  summaryClass,
                  'text-left data-[state=open]:border-primary',
                  summary.selected && 'border-primary/25 bg-primary/[0.03]'
                )}
                disabled={disabled}
                aria-label={
                  summary.selected
                    ? t(summary.inUse ? 'picker.group.currentLabel' : 'picker.group.savedLabel', {
                        connector: groupName,
                        name: currentName,
                        status: availabilityLabel,
                      })
                    : t('picker.group.connectionsLabel', {
                        connector: groupName,
                        count: group.count,
                      })
                }
                title={summary.selected ? currentName + ' · ' + availabilityLabel : undefined}
              >
                <span className="min-w-0 flex-1">
                  <span className="flex min-w-0 items-center gap-1.5">
                    <span className="truncate font-medium">
                      {summary.selected
                        ? currentName
                        : t(
                            pendingSetups.length > 0 && group.count === 0
                              ? 'pending.continue'
                              : 'picker.group.chooseConnection'
                          )}
                    </span>
                    {summary.isDefault ? (
                      <Star
                        className="size-3.5 shrink-0 fill-primary text-primary"
                        aria-label={t('defaults.newChatDefault')}
                      />
                    ) : null}
                  </span>
                  <span
                    className={cn(
                      'mt-0.5 flex min-w-0 items-center gap-1 text-[11px] font-normal',
                      summary.selected && summary.availability !== 'ready'
                        ? 'text-warning'
                        : 'text-muted-foreground'
                    )}
                  >
                    {summary.selected ? (
                      <>
                        {summary.inUse ? (
                          <span className="truncate">
                            {t('picker.group.usedInTarget', { target: targetLabel })}
                          </span>
                        ) : (
                          <span className="truncate">{t('picker.group.savedReference')}</span>
                        )}
                        <span className="shrink-0">· {availabilityLabel}</span>
                      </>
                    ) : pendingSetups.length > 0 ? (
                      t('pending.count', { count: pendingSetups.length })
                    ) : (
                      t('picker.group.savedCount', { count: group.count })
                    )}
                  </span>
                </span>
                <ChevronDown aria-hidden="true" className="ml-2 size-4 shrink-0" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              sideOffset={8}
              collisionPadding={16}
              className="w-80 max-w-[calc(100vw-2rem)] max-h-[min(420px,var(--radix-dropdown-menu-content-available-height))] rounded-xl p-2 shadow-lg"
              aria-label={t('picker.group.accountsTitle', { connector: groupName })}
              onCloseAutoFocus={event => {
                if (handoffRef.current) event.preventDefault();
              }}
            >
              <DropdownMenuLabel className="px-2 pb-3 pt-1">
                <span className="block text-sm font-semibold">
                  {t('picker.group.accountsTitle', { connector: groupName })}
                </span>
                <span className="mt-1 block text-xs font-normal text-muted-foreground">
                  {t(
                    multiple
                      ? 'picker.group.chooseMultipleForTarget'
                      : 'picker.group.chooseForTarget',
                    { target: targetLabel }
                  )}
                </span>
              </DropdownMenuLabel>
              <DropdownMenuRadioGroup
                value={summary.inUse ? (group.currentBinding?.connection_id ?? '') : ''}
              >
                {group.accounts.map(row => {
                  const connection = row.connection;
                  if (!connection) return null;
                  const name = personalConnectionDisplayName(
                    connection,
                    t('connections.unknownConnection'),
                    locale
                  );
                  const usable = row.state === 'ready' || row.state === 'added';
                  const current = multiple
                    ? Boolean(row.binding)
                    : summary.inUse && group.currentBinding?.connection_id === connection.id;
                  const isDefault =
                    preference?.mode === 'selected' && preference.connection_id === connection.id;
                  const label = usable
                    ? t('status.ready')
                    : row.state === 'reconnect'
                      ? t('status.reconnectRequired')
                      : t('status.unavailable');
                  const content = (
                    <>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-medium" title={name}>
                          {name}
                        </span>
                        <span
                          className="mt-0.5 block truncate text-xs text-muted-foreground"
                          title={connection.external_display_name}
                        >
                          {connection.external_display_name || t('connections.unknownIdentity')}
                        </span>
                        <span className="mt-0.5 flex flex-wrap gap-x-2 text-[11px]">
                          {current ? (
                            <span className="text-primary">{t('picker.states.inUse')}</span>
                          ) : null}
                          {isDefault ? (
                            <Star
                              className="size-3.5 fill-primary text-primary"
                              aria-label={t('defaults.newChatDefault')}
                            />
                          ) : null}
                        </span>
                      </span>
                      <span
                        className={cn(
                          'max-w-20 shrink-0 text-right text-xs',
                          usable ? 'text-muted-foreground' : 'text-warning'
                        )}
                      >
                        {label}
                      </span>
                    </>
                  );
                  return (
                    <div key={row.key} className="flex min-w-0 items-stretch gap-0.5">
                      {multiple && (usable || (row.hostUnavailable && row.binding)) ? (
                        <DropdownMenuCheckboxItem
                          checked={current}
                          textValue={name}
                          disabled={disabled}
                          className="min-h-14 min-w-0 flex-1 gap-2 rounded-lg py-2 data-[state=checked]:bg-primary/5"
                          onSelect={() => selectAccount(row)}
                        >
                          {content}
                        </DropdownMenuCheckboxItem>
                      ) : usable ? (
                        <DropdownMenuRadioItem
                          value={connection.id}
                          textValue={name}
                          disabled={disabled}
                          className="min-h-14 min-w-0 flex-1 gap-2 rounded-lg py-2 data-[state=checked]:bg-primary/5"
                          onSelect={() => selectAccount(row)}
                        >
                          {content}
                        </DropdownMenuRadioItem>
                      ) : (
                        <DropdownMenuItem
                          textValue={name}
                          disabled={disabled}
                          className="min-h-14 min-w-0 flex-1 rounded-lg py-2"
                          aria-label={
                            name +
                            ': ' +
                            label +
                            ', ' +
                            t(
                              row.hostUnavailable
                                ? 'picker.actions.viewReason'
                                : 'picker.actions.updateKey'
                            )
                          }
                          onSelect={() => selectAccount(row)}
                        >
                          <KeyRound aria-hidden="true" className="size-4 text-warning" />
                          {content}
                        </DropdownMenuItem>
                      )}
                    </div>
                  );
                })}
              </DropdownMenuRadioGroup>
              {pendingSetups.length > 0 && (
                <DropdownMenuLabel className="text-xs text-muted-foreground">
                  {t('pending.title')}
                </DropdownMenuLabel>
              )}
              {pendingSetups.map(item => (
                <DropdownMenuItem
                  key={item.id}
                  disabled={disabled}
                  className="min-h-12 gap-2 rounded-lg"
                  onSelect={() => {
                    handoffRef.current = true;
                    onResume?.(item.id);
                  }}
                >
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">{item.app_id}</span>
                  <span className="shrink-0 text-xs text-warning">{t('pending.continue')}</span>
                </DropdownMenuItem>
              ))}
              {group.currentBinding && !currentAccount ? (
                <p className="flex items-start gap-2 px-2 py-2 text-xs text-warning">
                  <CircleAlert aria-hidden="true" className="size-4 shrink-0" />
                  {t('picker.group.currentUnavailable')}
                </p>
              ) : null}
              <DropdownMenuSeparator />
              {createRow ? (
                <DropdownMenuItem
                  disabled={disabled}
                  className="min-h-9 rounded-lg"
                  onSelect={() => selectAccount(createRow)}
                >
                  <Plus aria-hidden="true" className="size-4" />
                  {t(
                    createRow.state === 'not_connected'
                      ? 'picker.actions.addConnection'
                      : 'picker.actions.viewReason'
                  )}
                </DropdownMenuItem>
              ) : null}
              {multiple ? (
                group.bindings.map(binding => {
                  const connection = group.accounts.find(
                    row => row.connection?.id === binding.connection_id
                  )?.connection;
                  const name =
                    connection?.name.trim() ||
                    connection?.external_display_name ||
                    t('picker.group.currentMissing');
                  return (
                    <DropdownMenuItem
                      key={binding.id}
                      disabled={disabled}
                      className="min-h-9 rounded-lg"
                      onSelect={() => onDetach(group, binding)}
                    >
                      <Minus aria-hidden="true" className="size-4" />
                      <span className="truncate">
                        {t('picker.group.removeConnectionFromTarget', {
                          target: targetLabel,
                          name,
                        })}
                      </span>
                    </DropdownMenuItem>
                  );
                })
              ) : group.currentBinding ? (
                <DropdownMenuItem
                  disabled={disabled}
                  className="min-h-9 rounded-lg"
                  onSelect={() => onDetach(group)}
                >
                  <Minus aria-hidden="true" className="size-4" />
                  {t('picker.group.removeFromTarget', { target: targetLabel })}
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : createRow ? (
          <Button
            className={summaryClass}
            variant="outline"
            disabled={disabled}
            onClick={() => onSelect(createRow)}
          >
            <span className="min-w-0 flex-1 text-left">
              <span className="block truncate">
                {t(
                  createRow.state === 'not_connected'
                    ? 'picker.actions.addConnection'
                    : 'picker.actions.viewReason'
                )}
              </span>
              <span className="mt-0.5 block text-[11px] font-normal text-muted-foreground">
                {t('picker.group.noSavedConnection')}
              </span>
            </span>
            {createRow.state === 'not_connected' ? (
              <Plus aria-hidden="true" className="ml-2 size-4 shrink-0" />
            ) : (
              <CircleAlert aria-hidden="true" className="ml-2 size-4 shrink-0" />
            )}
          </Button>
        ) : null}
      </div>
      {renderConnectionDetails
        ? group.accounts
            .filter(row => row.binding)
            .map(row => (
              <div key={row.key} className="mt-3 min-w-0">
                {renderConnectionDetails(row)}
              </div>
            ))
        : null}
    </article>
  );
}
