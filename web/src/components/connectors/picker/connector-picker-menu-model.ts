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
  type ConnectorPickerRow,
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
  return {
    selected,
    inUse: group.currentBinding?.status === 'active',
    bindingStatus: group.currentBinding?.status,
    currentAccount,
    name: currentAccount?.connection
      ? personalConnectionDisplayName(currentAccount.connection, group.name, locale)
      : undefined,
    availability: !selected
      ? undefined
      : currentAccount?.state === 'reconnect'
        ? ('reconnect_required' as const)
        : currentAccount?.state === 'added'
          ? ('ready' as const)
          : ('unavailable' as const),
    isDefault:
      selected &&
      preference?.mode === 'selected' &&
      preference.connection_id === group.currentBinding?.connection_id,
  };
}

export function createConnectorPickerMenuGroups(
  catalog: readonly ConnectorCatalogItem[],
  connections: readonly PersonalConnection[],
  bindings: readonly ConnectorPickerBindingRef[]
): ConnectorPickerMenuGroup[] {
  const rows = createConnectorPickerRows(catalog, connections, bindings);
  const groups = new Map<string, ConnectorPickerMenuGroup>();
  for (const connector of catalog) {
    const connectorRows = rows.filter(row => row.connector.id === connector.id);
    const accounts = connectorRows.filter(row => row.connection);
    const currentBinding = bindings.find(binding => binding.connector_id === connector.id);
    groups.set(connector.id, {
      id: connector.id,
      name: connector.name,
      icon: connector.icon,
      classification: connector.classification,
      connector,
      accounts,
      createRow: connectorRows.find(row => !row.connection),
      currentBinding,
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
      accounts: createConnectorPickerRows([connector], saved, [binding]).filter(
        row => row.connection
      ),
      currentBinding: binding,
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
      ).length > 0 || Boolean(group.unavailable && group.currentBinding && !filters.query.trim())
  );
}

/** An account radio selects an account; clicking its current value never turns it off. */
export async function requestConnectorPickerMenuAccount(
  row: ConnectorPickerRow,
  handlers: ConnectorPickerActionHandlers,
  target: ConnectorTargetRef
): Promise<void> {
  if (row.state === 'added') return;
  await requestConnectorPickerAction(row, handlers, target);
}

/** This target-only toggle neither deletes a Key nor guesses among multiple accounts. */
export async function requestConnectorPickerMenuToggle(
  group: ConnectorPickerMenuGroup,
  handlers: ConnectorPickerActionHandlers,
  target: ConnectorTargetRef
): Promise<void> {
  if (!group.connector || !group.canToggle) return;
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
