import { personalConnectionDisplayName } from '../connections/personal-connections-model';
import type {
  ConnectorCatalogItem,
  ConnectorChatDefault,
  ConnectorClassification,
  ConnectorTargetRef,
  PersonalConnection,
} from '@/services/types/connector';
import {
  createConnectorPickerRows,
  filterConnectorPickerRows,
  requestConnectorPickerAction,
  type ConnectorPickerActionHandlers,
  type ConnectorPickerBindingRef,
  type ConnectorPickerFilters,
  type ConnectorPickerOptions,
  type ConnectorPickerRow,
  type ConnectorPickerSelectionMode,
} from './connector-picker-model';

export interface ConnectorPickerMenuGroup {
  id: string;
  name: string;
  icon?: string;
  classification?: ConnectorClassification;
  connector?: ConnectorCatalogItem;
  accounts: ConnectorPickerRow[];
  createRow?: ConnectorPickerRow;
  currentBinding?: ConnectorPickerBindingRef;
  bindings: ConnectorPickerBindingRef[];
  selectionMode: ConnectorPickerSelectionMode;
  count: number;
  canToggle: boolean;
  unavailable?: boolean;
}

/** Current identity comes from the binding, including a bound but unusable account. */
export function connectorPickerGroupSummary(
  group: ConnectorPickerMenuGroup,
  preference?: ConnectorChatDefault,
  locale = 'zh-Hans'
) {
  const currentAccount = group.accounts.find(
    row => row.connection?.id === group.currentBinding?.connection_id
  );
  const selected = Boolean(group.currentBinding);
  const selectedAccounts = group.accounts.filter(row => Boolean(row.binding));
  const multiple = group.selectionMode === 'multiple-connections';
  const allReady =
    selectedAccounts.length === group.bindings.length &&
    selectedAccounts.every(row => row.state === 'added');
  return {
    selected,
    selectedCount: group.bindings.length,
    inUse: group.currentBinding?.status === 'active',
    bindingStatus: group.currentBinding?.status,
    currentAccount,
    name: currentAccount?.connection
      ? personalConnectionDisplayName(currentAccount.connection, group.name, locale)
      : undefined,
    availability: !selected
      ? undefined
      : multiple && group.bindings.length > 1
        ? allReady
          ? ('ready' as const)
          : selectedAccounts.some(row => row.state === 'reconnect')
            ? ('reconnect_required' as const)
            : ('unavailable' as const)
        : currentAccount?.state === 'reconnect'
          ? ('reconnect_required' as const)
          : currentAccount?.state === 'added'
            ? ('ready' as const)
            : ('unavailable' as const),
    isDefault:
      !multiple &&
      selected &&
      preference?.mode === 'selected' &&
      preference.connection_id === group.currentBinding?.connection_id,
  };
}

export function createConnectorPickerMenuGroups(
  catalog: readonly ConnectorCatalogItem[],
  connections: readonly PersonalConnection[],
  bindings: readonly ConnectorPickerBindingRef[],
  options: ConnectorPickerOptions = {}
): ConnectorPickerMenuGroup[] {
  const rows = createConnectorPickerRows(catalog, connections, bindings, options);
  const selectionMode = options.selectionMode ?? 'single-per-connector';
  const groups = new Map<string, ConnectorPickerMenuGroup>();
  for (const connector of catalog) {
    const connectorRows = rows.filter(row => row.connector.id === connector.id);
    const accounts = connectorRows.filter(row => row.connection);
    const currentBinding = bindings.find(binding => binding.connector_id === connector.id);
    if (connectorRows.length === 0 && !currentBinding) continue;
    groups.set(connector.id, {
      id: connector.id,
      name: connector.name,
      icon: connector.icon,
      classification: connector.classification,
      connector,
      accounts,
      createRow: connectorRows.find(row => !row.connection),
      currentBinding,
      bindings: bindings.filter(binding => binding.connector_id === connector.id),
      selectionMode,
      count: accounts.length,
      canToggle:
        Boolean(currentBinding) || accounts.filter(row => row.state === 'ready').length === 1,
    });
  }

  // A removed plugin must not hide an existing selection or make it impossible to detach.
  for (const binding of bindings) {
    if (groups.has(binding.connector_id)) continue;
    const saved = connections.filter(
      connection => connection.connector_id === binding.connector_id
    );
    const savedBindings = bindings.filter(item => item.connector_id === binding.connector_id);
    const connector: ConnectorCatalogItem = {
      id: binding.connector_id,
      plugin_id: '',
      name: '',
      description: '',
      classification: 'connector',
      key_fields: [],
      capabilities: [],
      connection_summary: { count: saved.length, state: 'unavailable' },
    };
    groups.set(binding.connector_id, {
      id: binding.connector_id,
      name: '',
      connector,
      accounts: createConnectorPickerRows([connector], saved, savedBindings, options).filter(
        row => row.connection
      ),
      currentBinding: binding,
      bindings: savedBindings,
      selectionMode,
      count: saved.length,
      canToggle: true,
      unavailable: true,
    });
  }

  return Array.from(groups.values());
}

/** Filter services, not their account lists: counts and repair/create choices stay complete. */
export function filterConnectorPickerGroups(
  groups: readonly ConnectorPickerMenuGroup[],
  filters: ConnectorPickerFilters,
  pendingConnectorIDs: ReadonlySet<string> = new Set()
): ConnectorPickerMenuGroup[] {
  return groups.filter(
    group =>
      filterConnectorPickerRows(
        [...group.accounts, ...(group.createRow ? [group.createRow] : [])],
        filters.status === 'needs_attention' && pendingConnectorIDs.has(group.id)
          ? { ...filters, status: 'all' }
          : filters
      ).length > 0 ||
      Boolean(
        group.currentBinding &&
          !filters.query.trim() &&
          (group.unavailable ||
            group.bindings.some(
              binding => !group.accounts.some(row => row.connection?.id === binding.connection_id)
            ))
      )
  );
}

/** Default radio selection is stable; hosts with independent connections use checkboxes. */
export async function requestConnectorPickerMenuAccount(
  row: ConnectorPickerRow,
  handlers: ConnectorPickerActionHandlers,
  target: ConnectorTargetRef,
  selectionMode: ConnectorPickerSelectionMode = 'single-per-connector'
): Promise<void> {
  if (row.state === 'added' && selectionMode === 'single-per-connector') return;
  if (
    selectionMode === 'multiple-connections' &&
    row.binding &&
    (row.state === 'added' || row.state === 'ready')
  ) {
    await handlers.onRequestDetach?.({
      connector: row.connector,
      binding: row.binding,
      connection: row.connection,
      target,
    });
    return;
  }
  await requestConnectorPickerAction(row, handlers, target);
}

/** This target-only toggle neither deletes a Key nor guesses among multiple accounts. */
export async function requestConnectorPickerMenuToggle(
  group: ConnectorPickerMenuGroup,
  handlers: ConnectorPickerActionHandlers,
  target: ConnectorTargetRef
): Promise<void> {
  if (!group.connector || !group.canToggle) return;
  // Multiple connections require an explicit per-binding removal, never a guessed service slot.
  if (group.selectionMode === 'multiple-connections' && group.bindings.length > 1) return;
  if (group.currentBinding) {
    await handlers.onRequestDetach?.({
      connector: group.connector,
      binding: group.currentBinding,
      connection: group.accounts.find(
        row => row.connection?.id === group.currentBinding?.connection_id
      )?.connection,
      target,
    });
    return;
  }
  const eligible = group.accounts.filter(row => row.state === 'ready');
  if (eligible.length === 1) await requestConnectorPickerAction(eligible[0], handlers, target);
}
