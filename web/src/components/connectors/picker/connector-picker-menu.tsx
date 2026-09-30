'use client';

import {
  groupMailPickerProviders,
  isMailPickerGroup,
  type PickerDisplayGroup,
} from './mail-picker-groups';
import { personalConnectionDisplayName } from '../connections/personal-connections-model';

import { listedConnectorGroups } from '../connector-visibility';

import { ConnectorUnavailableReferences } from '../unavailable-references';

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowLeft,
  ArrowRightLeft,
  Check,
  ChevronRight,
  CircleAlert,
  KeyRound,
  Loader2,
  Plus,
} from 'lucide-react';
import { Menu } from 'radix-ui/internal';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useLocale } from '@/hooks/use-locale';
import { useIsMobile } from '@/hooks/use-mobile';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ConnectorPickerDialogProps } from './connector-picker-dialog';
import {
  isConnectorPickerSwitchCurrent,
  type ConnectorPickerActionHandlers,
  type ConnectorPickerRow,
  type ConnectorPickerSwitchRequest,
} from './connector-picker-model';
import {
  createConnectorPickerMenuGroups,
  requestConnectorPickerMenuAccount,
  requestConnectorPickerMenuToggle,
  type ConnectorPickerMenuGroup,
} from './connector-picker-menu-model';

export interface ConnectorPickerMenuProps extends ConnectorPickerDialogProps {
  /** The actual host button that opened this menu, never inferred from focus. */
  anchorElement: HTMLElement | null;
}

const PANEL =
  'z-50 w-64 max-w-[calc(100vw-24px)] max-h-[min(440px,calc(100dvh-32px))] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md outline-hidden';
const ITEM =
  'relative flex min-h-8 cursor-pointer select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden focus:bg-accent focus:text-accent-foreground data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50';
const LABEL = 'px-2 py-1.5 text-xs text-muted-foreground';

function accountName(row: ConnectorPickerRow, locale?: string): string {
  return row.connection
    ? personalConnectionDisplayName(row.connection, row.connector.name, locale)
    : row.connector.name;
}

export function ConnectorPickerMenu({
  open,
  target,
  targetName,
  connectorId,
  bindings,
  anchorElement,
  onOpenChange,
  onRequestAdd,
  onRequestSwitch,
  onRequestConnectAndAdd,
  onRequestUpdateKey,
  onRequestViewReason,
  onRequestDetach,
  onReferenceCommand,
  pendingReferenceCommands,
}: ConnectorPickerMenuProps) {
  const t = useT('connectors');
  const { locale } = useLocale();
  const isMobile = useIsMobile();
  const [activeConnectorID, setActiveConnectorID] = useState<string>();
  const [pendingSwitch, setPendingSwitch] = useState<ConnectorPickerSwitchRequest>();
  const [pending, setPending] = useState(false);
  const [actionFailed, setActionFailed] = useState(false);
  const busyRef = useRef(false);
  const restoreFocusRef = useRef(true);
  const outsideInteractionRef = useRef(false);
  const serviceRefs = useRef(new Map<string, HTMLDivElement>());
  const accountRefs = useRef(new Map<string, HTMLDivElement>());
  const contentRef = useRef<HTMLDivElement>(null);
  const confirmRef = useRef<HTMLDivElement>(null);
  const backRef = useRef<HTMLDivElement>(null);
  const returnServiceRef = useRef<string | undefined>(undefined);
  const returnAccountRef = useRef<string | undefined>(undefined);
  const anchorRef = useMemo(
    () => (anchorElement ? { current: anchorElement } : undefined),
    [anchorElement]
  );
  const scope = JSON.stringify([open, target.type, target.id, target.binding_instance_id]);
  // Object identity distinguishes close/reopen and A -> B -> A target changes.
  // Equal target strings must never let an old completion unlock a new session.
  const sessionRef = useRef({ scope });
  if (sessionRef.current.scope !== scope) sessionRef.current = { scope };
  const session = sessionRef.current;
  const visible = open && Boolean(anchorElement?.isConnected);
  const targetLabel = targetName.trim() || t(`picker.targetTypes.${target.type}`);
  const catalogQuery = useConnectorCatalog(undefined, visible);
  const connectionsQuery = usePersonalConnections(
    { connector_id: connectorId, target_type: target.type, target_id: target.id },
    visible && Boolean(target.id)
  );
  const catalog = useMemo(
    () =>
      connectorCatalogItems(catalogQuery.data?.data)
        .filter(item => !connectorId || item.id === connectorId)
        .map(item =>
          localizeConnectorCatalogItem(item, locale, {
            description: t('metadata.descriptionUnavailable'),
            capabilityDescription: t('metadata.capabilityDescriptionUnavailable'),
          })
        ),
    [catalogQuery.data?.data, connectorId, locale, t]
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  const groups = useMemo(
    () => listedConnectorGroups(createConnectorPickerMenuGroups(catalog, connections, bindings)),
    [bindings, catalog, connections]
  );
  const displayGroups = groupMailPickerProviders(groups, t('mail.title'));
  const activeGroup = displayGroups.find(group => group.id === activeConnectorID);
  const loading = catalogQuery.isLoading || connectionsQuery.isLoading;
  const failed = catalogQuery.isError || connectionsQuery.isError;

  useEffect(() => {
    setActiveConnectorID(undefined);
    setPendingSwitch(undefined);
    setPending(false);
    setActionFailed(false);
    busyRef.current = false;
    if (open) {
      restoreFocusRef.current = true;
      outsideInteractionRef.current = false;
    }
    returnServiceRef.current = undefined;
    returnAccountRef.current = undefined;
  }, [scope, open]);

  useEffect(() => {
    if (!open) return;
    if (!anchorElement?.isConnected) {
      onOpenChange(false);
      return;
    }
    const observer = new MutationObserver(() => {
      if (!anchorElement.isConnected) onOpenChange(false);
    });
    observer.observe(anchorElement.ownerDocument.documentElement, {
      childList: true,
      subtree: true,
    });
    return () => observer.disconnect();
  }, [anchorElement, onOpenChange, open]);

  useEffect(() => {
    if (!isMobile) return;
    if (activeConnectorID) {
      backRef.current?.focus();
    } else if (returnServiceRef.current) {
      serviceRefs.current.get(returnServiceRef.current)?.focus();
      returnServiceRef.current = undefined;
    }
  }, [activeConnectorID, isMobile]);

  useEffect(() => {
    if (pending) return;
    if (pendingSwitch) {
      confirmRef.current?.focus();
    } else if (returnAccountRef.current) {
      accountRefs.current.get(returnAccountRef.current)?.focus();
      returnAccountRef.current = undefined;
    }
  }, [pending, pendingSwitch]);

  const focusFirstService = () => {
    contentRef.current
      ?.querySelector<HTMLElement>('[role="menuitem"]:not([data-disabled])')
      ?.focus();
  };

  useEffect(() => {
    // Data may arrive after the empty loading panel received initial focus.
    if (visible && !loading && !failed) focusFirstService();
  }, [visible, loading, failed]);

  const handlers: ConnectorPickerActionHandlers = {
    onRequestAdd,
    onRequestSwitch: request => setPendingSwitch(request),
    onRequestConnectAndAdd,
    onRequestUpdateKey,
    onRequestViewReason,
    onRequestDetach,
  };

  const perform = async (action: () => void | Promise<void>, handoff = false) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setPending(true);
    setActionFailed(false);
    // A Key dialog or destination page owns its own focus. Menu teardown must
    // not steal that focus back to the host button during this handoff.
    restoreFocusRef.current = !handoff;
    try {
      await action();
    } catch {
      if (sessionRef.current !== session) return;
      restoreFocusRef.current = true;
      setPendingSwitch(undefined);
      setActionFailed(true);
    } finally {
      if (sessionRef.current === session) {
        busyRef.current = false;
        setPending(false);
      }
    }
  };

  const selectAccount = (row: ConnectorPickerRow) => {
    if (pendingSwitch) return;
    const handoff =
      row.state === 'not_connected' || row.state === 'reconnect' || row.state === 'unavailable';
    void perform(() => requestConnectorPickerMenuAccount(row, handlers, target), handoff);
  };

  const confirmSwitch = () => {
    const request = pendingSwitch;
    if (!request || busyRef.current) return;
    if (!isConnectorPickerSwitchCurrent(request, connections, bindings, target)) {
      setPendingSwitch(undefined);
      setActionFailed(true);
      return;
    }
    void perform(async () => {
      await onRequestSwitch(request);
      if (sessionRef.current !== session) return;
      setPendingSwitch(undefined);
    });
  };

  const cancelSwitch = () => {
    if (busyRef.current) return;
    returnAccountRef.current = pendingSwitch?.connection.id;
    setPendingSwitch(undefined);
  };

  const backToServices = () => {
    returnServiceRef.current = activeConnectorID;
    setPendingSwitch(undefined);
    setActiveConnectorID(undefined);
  };

  const setSubmenuOpen = (groupID: string, nextOpen: boolean) => {
    if (busyRef.current) return;
    if (nextOpen && activeConnectorID === groupID) return;
    if (!nextOpen && activeConnectorID !== groupID) return;
    setPendingSwitch(undefined);
    setActiveConnectorID(nextOpen ? groupID : undefined);
  };

  const serviceLabel = (group: PickerDisplayGroup) => (
    <>
      <ConnectorIcon
        connectorId={group.id}
        name={group.name}
        icon={isMailPickerGroup(group) ? undefined : group.icon}
        classification={
          isMailPickerGroup(group) ? 'connector' : (group.classification ?? 'connector')
        }
        size="sm"
        className="size-6 rounded-sm border-0 bg-transparent shadow-none [&_svg]:size-4 [&_img]:size-4"
      />
      <span className="min-w-0 flex-1 truncate">{group.name}</span>
      <span
        className="text-xs tabular-nums text-muted-foreground"
        aria-label={t('picker.menu.accountCount', { count: group.count })}
      >
        {group.count}
      </span>
      <ChevronRight aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
    </>
  );

  const renderAccounts = (group: ConnectorPickerMenuGroup, hideCreate = false) => {
    const request = pendingSwitch?.connector.id === group.id ? pendingSwitch : undefined;
    const sourceName = request?.replacedConnection
      ? personalConnectionDisplayName(request.replacedConnection, group.name, locale)
      : t('status.unavailable');
    const nextName = request?.connection
      ? personalConnectionDisplayName(request.connection, group.name, locale)
      : group.name;
    const busy = pending || Boolean(request);
    const createRow = hideCreate ? undefined : group.createRow;
    return (
      <>
        {actionFailed ? (
          <Menu.Label className={cn(LABEL, 'text-destructive')} role="alert">
            {t('picker.actionFailed')}
          </Menu.Label>
        ) : null}
        {group.connector ? (
          <>
            <Menu.CheckboxItem
              checked={Boolean(group.currentBinding)}
              disabled={busy || !group.canToggle}
              className={ITEM}
              textValue={t('picker.menu.useInTarget', { target: targetLabel })}
              onSelect={event => {
                event.preventDefault();
                void perform(() => requestConnectorPickerMenuToggle(group, handlers, target));
              }}
            >
              <span className="min-w-0 flex-1 truncate">
                {t('picker.menu.useInTarget', { target: targetLabel })}
              </span>
              <span
                aria-hidden="true"
                className={cn(
                  'flex h-4 w-7 shrink-0 rounded-full p-0.5 transition-colors',
                  group.currentBinding ? 'justify-end bg-primary' : 'bg-muted-foreground/30'
                )}
              >
                <span className="size-3 rounded-full bg-background shadow-xs" />
              </span>
            </Menu.CheckboxItem>
            {!group.canToggle ? (
              <Menu.Label className={LABEL}>{t('picker.menu.chooseAccountFirst')}</Menu.Label>
            ) : null}
            <Menu.Separator className="-mx-1 my-1 h-px bg-border" />
          </>
        ) : null}

        <Menu.RadioGroup
          value={group.currentBinding?.connection_id ?? ''}
          aria-label={t('picker.menu.accounts', { connector: group.name })}
        >
          {group.accounts.map(row => {
            const selected = row.connection?.id === group.currentBinding?.connection_id;
            const unavailableLabel =
              row.state === 'reconnect'
                ? t('picker.actions.updateKey')
                : t('picker.menu.unavailableAccount');
            const content = (
              <>
                <span className="flex size-4 shrink-0 items-center justify-center">
                  {selected ? <Check aria-hidden="true" className="size-3.5" /> : null}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{accountName(row, locale)}</span>
                  {row.connection?.external_display_name &&
                  row.connection.external_display_name !== accountName(row, locale) ? (
                    <span className="block truncate text-xs text-muted-foreground">
                      {row.connection.external_display_name}
                    </span>
                  ) : null}
                  {row.state !== 'ready' && row.state !== 'added' ? (
                    <span className="block truncate text-xs text-muted-foreground">
                      {unavailableLabel}
                    </span>
                  ) : null}
                </span>
              </>
            );
            return row.state === 'ready' || row.state === 'added' ? (
              <Menu.RadioItem
                key={row.key}
                ref={element => {
                  if (element) accountRefs.current.set(row.connection?.id ?? row.key, element);
                  else accountRefs.current.delete(row.connection?.id ?? row.key);
                }}
                value={row.connection?.id ?? row.key}
                disabled={busy}
                textValue={accountName(row, locale)}
                className={ITEM}
                onSelect={event => {
                  event.preventDefault();
                  selectAccount(row);
                }}
              >
                {content}
              </Menu.RadioItem>
            ) : (
              <Menu.Item
                key={row.key}
                disabled={busy}
                textValue={accountName(row, locale)}
                className={ITEM}
                aria-label={`${accountName(row, locale)}: ${unavailableLabel}`}
                onSelect={event => {
                  event.preventDefault();
                  selectAccount(row);
                }}
              >
                {content}
                {row.state === 'reconnect' ? (
                  <KeyRound aria-hidden="true" className="size-3.5 text-muted-foreground" />
                ) : (
                  <CircleAlert aria-hidden="true" className="size-3.5 text-muted-foreground" />
                )}
              </Menu.Item>
            );
          })}
        </Menu.RadioGroup>
        {group.accounts.length === 0 ? (
          <Menu.Label className={LABEL}>{t('picker.menu.noAccounts')}</Menu.Label>
        ) : null}
        {group.currentBinding &&
        !group.accounts.some(row => row.connection?.id === group.currentBinding?.connection_id) ? (
          <Menu.Label className={LABEL}>{t('status.unavailable')}</Menu.Label>
        ) : null}

        {request ? (
          <Menu.Group
            aria-label={t('picker.menu.pending')}
            className="my-1 rounded-sm bg-muted/60 p-1"
          >
            <Menu.Label className={cn(LABEL, 'text-foreground')} role="status">
              {t('picker.menu.pendingSwitch', { current: sourceName, next: nextName })}
            </Menu.Label>
            <Menu.Item
              ref={confirmRef}
              disabled={pending}
              className={cn(ITEM, 'font-medium')}
              onSelect={event => {
                event.preventDefault();
                confirmSwitch();
              }}
            >
              {pending ? (
                <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
              ) : (
                <ArrowRightLeft aria-hidden="true" className="size-3.5" />
              )}
              {t('picker.switchConfirm')}
            </Menu.Item>
            <Menu.Item
              disabled={pending}
              className={ITEM}
              onSelect={event => {
                event.preventDefault();
                cancelSwitch();
              }}
            >
              {t('picker.switchCancel')}
            </Menu.Item>
          </Menu.Group>
        ) : null}

        {group.createRow || group.connector ? (
          <Menu.Separator className="-mx-1 my-1 h-px bg-border" />
        ) : null}
        {createRow ? (
          <Menu.Item
            disabled={busy}
            className={ITEM}
            onSelect={event => {
              event.preventDefault();
              selectAccount(createRow);
            }}
          >
            <Plus aria-hidden="true" className="size-4" />
            {createRow.state === 'not_connected'
              ? t('picker.menu.connectAnotherAccount')
              : t('picker.actions.viewReason')}
          </Menu.Item>
        ) : null}
      </>
    );
  };

  const renderDisplayAccounts = (group: PickerDisplayGroup) => {
    if (!isMailPickerGroup(group)) return renderAccounts(group);
    const create = group.providers.flatMap(provider =>
      provider.createRow ? [provider.createRow] : []
    );
    return (
      <>
        {group.providers
          .filter(provider => provider.count > 0 || provider.currentBinding)
          .map(provider => (
            <Menu.Group key={provider.id}>
              <Menu.Label className={LABEL}>{provider.name}</Menu.Label>
              {renderAccounts(provider, true)}
            </Menu.Group>
          ))}
        {create.length > 0 && (
          <Menu.Sub>
            <Menu.SubTrigger className={ITEM} disabled={pending || Boolean(pendingSwitch)}>
              <Plus aria-hidden="true" className="size-4" />
              {t('connections.addConnection')}
              <ChevronRight className="ml-auto size-4" />
            </Menu.SubTrigger>
            <Menu.Portal>
              <Menu.SubContent className={PANEL} collisionPadding={12}>
                {create.map(row => (
                  <Menu.Item
                    key={row.connector.id}
                    className={ITEM}
                    onSelect={event => {
                      event.preventDefault();
                      selectAccount(row);
                    }}
                  >
                    {row.connector.name}
                  </Menu.Item>
                ))}
              </Menu.SubContent>
            </Menu.Portal>
          </Menu.Sub>
        )}
      </>
    );
  };

  if (!visible) return null;

  return (
    <Menu.Root open={visible} onOpenChange={onOpenChange} modal={false}>
      <Menu.Anchor virtualRef={anchorRef} />
      <Menu.Portal>
        <Menu.Content
          ref={contentRef}
          className={PANEL}
          side="top"
          align="start"
          sideOffset={6}
          collisionPadding={12}
          loop
          aria-label={t('picker.menu.label', { target: targetLabel })}
          onEntryFocus={event => {
            // The opening Enter may precede Menu.Root's keyboard listener.
            // Explicit entry focus works for both pointer and keyboard opens.
            event.preventDefault();
            focusFirstService();
          }}
          onCloseAutoFocus={event => {
            event.preventDefault();
            if (
              restoreFocusRef.current &&
              !outsideInteractionRef.current &&
              anchorElement?.isConnected
            ) {
              anchorElement.focus({ preventScroll: true });
            }
          }}
          onInteractOutside={event => {
            if (event.target instanceof Node && anchorElement?.contains(event.target)) {
              event.preventDefault();
            } else {
              outsideInteractionRef.current = true;
            }
          }}
          onEscapeKeyDown={event => {
            if (pendingSwitch) {
              event.preventDefault();
              cancelSwitch();
            } else if (isMobile && activeConnectorID) {
              event.preventDefault();
              backToServices();
            }
          }}
          onKeyDown={event => {
            if (isMobile && activeConnectorID && event.key === 'ArrowLeft') {
              event.preventDefault();
              backToServices();
            }
          }}
        >
          {loading ? (
            <Menu.Label role="status" className={cn(LABEL, 'flex items-center gap-2')}>
              <Loader2 aria-hidden="true" className="size-4 animate-spin" />
              {t('picker.loading')}
            </Menu.Label>
          ) : failed ? (
            <>
              <Menu.Label role="alert" className={LABEL}>
                {t('picker.loadFailedTitle')}
              </Menu.Label>
              <Menu.Item
                className={ITEM}
                onSelect={event => {
                  event.preventDefault();
                  void Promise.all([catalogQuery.refetch(), connectionsQuery.refetch()]);
                }}
              >
                {t('actions.retry')}
              </Menu.Item>
            </>
          ) : isMobile && activeGroup ? (
            <>
              <Menu.Item
                ref={backRef}
                disabled={pending}
                className={ITEM}
                onSelect={event => {
                  event.preventDefault();
                  backToServices();
                }}
              >
                <ArrowLeft aria-hidden="true" className="size-4" />
                <span className="min-w-0 flex-1 truncate">{activeGroup.name}</span>
                <span className="sr-only">{t('picker.menu.back')}</span>
              </Menu.Item>
              <Menu.Separator className="-mx-1 my-1 h-px bg-border" />
              {renderDisplayAccounts(activeGroup)}
            </>
          ) : (
            <>
              {actionFailed ? (
                <Menu.Label role="alert" className={cn(LABEL, 'text-destructive')}>
                  {t('picker.actionFailed')}
                </Menu.Label>
              ) : null}
              {groups.length === 0 ? (
                <Menu.Label className={LABEL}>{t('picker.emptyTitle')}</Menu.Label>
              ) : null}
              {displayGroups.map(group =>
                isMobile ? (
                  <Menu.Item
                    key={group.id}
                    className={ITEM}
                    textValue={group.name}
                    disabled={pending}
                    ref={element => {
                      if (element) serviceRefs.current.set(group.id, element);
                      else serviceRefs.current.delete(group.id);
                    }}
                    onSelect={event => {
                      event.preventDefault();
                      setSubmenuOpen(group.id, true);
                    }}
                  >
                    {serviceLabel(group)}
                  </Menu.Item>
                ) : (
                  <Menu.Sub
                    key={group.id}
                    open={activeConnectorID === group.id}
                    onOpenChange={next => setSubmenuOpen(group.id, next)}
                  >
                    <Menu.SubTrigger
                      className={cn(ITEM, 'data-[state=open]:bg-accent')}
                      textValue={group.name}
                      disabled={pending}
                    >
                      {serviceLabel(group)}
                    </Menu.SubTrigger>
                    <Menu.Portal>
                      <Menu.SubContent
                        className={PANEL}
                        sideOffset={6}
                        alignOffset={-4}
                        collisionPadding={12}
                        loop
                        aria-label={t('picker.menu.accounts', { connector: group.name })}
                        onEscapeKeyDown={event => {
                          if (pendingSwitch) {
                            event.preventDefault();
                            cancelSwitch();
                          }
                        }}
                      >
                        {renderDisplayAccounts(group)}
                      </Menu.SubContent>
                    </Menu.Portal>
                  </Menu.Sub>
                )
              )}
            </>
          )}
          <ConnectorUnavailableReferences
            target={target}
            enabled={open}
            onReferenceCommand={onReferenceCommand}
            pendingReferenceCommands={pendingReferenceCommands}
          />
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
