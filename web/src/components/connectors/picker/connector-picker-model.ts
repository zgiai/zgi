import { supportsConnectorSetup } from '../setup/app-setup-state';
import type {
  ConnectorCatalogItem,
  ConnectorCatalogStatus,
  ConnectorTargetRef,
  ConnectorTargetBinding,
  PersonalConnection,
} from '@/services/types/connector';
import { connectorCatalogSearchValues } from '../connector-metadata';

export type ConnectorPickerRowState =
  | 'added'
  | 'ready'
  | 'reconnect'
  | 'not_connected'
  | 'unavailable';

export type ConnectorPickerBindingRef = Pick<
  ConnectorTargetBinding,
  'id' | 'connection_id' | 'connector_id' | 'status'
> & { revision?: number };

export interface ConnectorPickerRow {
  key: string;
  connector: ConnectorCatalogItem;
  connection?: PersonalConnection;
  binding?: ConnectorPickerBindingRef;
  /** Current binding in the same target slot that selecting this row would replace. */
  replacedBinding?: ConnectorPickerBindingRef;
  /** Best-effort display metadata for the connection currently occupying that slot. */
  replacedConnection?: PersonalConnection;
  state: ConnectorPickerRowState;
  reason?: string;
}

export interface ConnectorPickerFilters {
  query: string;
  status: ConnectorCatalogStatus;
}

export interface ConnectorPickerActionRequest {
  connector: ConnectorCatalogItem;
  connection?: PersonalConnection;
  binding?: ConnectorPickerBindingRef;
  replacedBinding?: ConnectorPickerBindingRef;
  replacedConnection?: PersonalConnection;
  reason?: string;
  /** Context for the host-owned draft/save transaction; the picker never persists it directly. */
  target?: ConnectorTargetRef;
}

export type ConnectorPickerSwitchRequest = ConnectorPickerActionRequest & {
  connection: PersonalConnection;
  replacedBinding: ConnectorPickerBindingRef;
};

export interface ConnectorPickerActionHandlers {
  onRequestAdd?: (
    request: ConnectorPickerActionRequest & { connection: PersonalConnection }
  ) => void | Promise<void>;
  onRequestSwitch?: (request: ConnectorPickerSwitchRequest) => void | Promise<void>;
  onRequestConnectAndAdd?: (request: ConnectorPickerActionRequest) => void | Promise<void>;
  onRequestUpdateKey?: (
    request: ConnectorPickerActionRequest & { connection: PersonalConnection }
  ) => void | Promise<void>;
  onRequestViewReason?: (request: ConnectorPickerActionRequest) => void | Promise<void>;
  onRequestDetach?: (
    request: ConnectorPickerActionRequest & { binding: ConnectorPickerBindingRef }
  ) => void | Promise<void>;
}

function rowState(
  connector: ConnectorCatalogItem,
  connection: PersonalConnection | undefined,
  binding: ConnectorPickerBindingRef | undefined
): Pick<ConnectorPickerRow, 'state' | 'reason'> {
  // A new Key receives its own verification. Saved connections losing access
  // must not disable the independent create action for an admitted Connector.
  if (!connection && supportsConnectorSetup(connector)) {
    return { state: 'not_connected' };
  }
  if (connection?.state === 'reconnect_required') {
    return { state: 'reconnect', reason: connection.diagnostic_code };
  }
  if (
    connection?.state === 'unavailable' ||
    (!connection && connector.key_fields.length === 0) ||
    !connector.capabilities.some(capability => capability.usable)
  ) {
    return {
      state: 'unavailable',
      reason:
        connection?.diagnostic_code ??
        (!connection && connector.key_fields.length === 0
          ? 'key_connection_unavailable'
          : undefined) ??
        connector.capabilities.find(capability => !capability.usable)?.unavailable_reason,
    };
  }
  if (binding?.status === 'active') {
    return { state: 'added' };
  }
  if (binding?.status === 'needs_replacement') {
    return { state: 'unavailable', reason: 'binding_needs_replacement' };
  }
  if (connection?.state === 'ready') {
    return { state: 'ready' };
  }
  return { state: 'not_connected' };
}

export function createConnectorPickerRows(
  catalog: readonly ConnectorCatalogItem[],
  connections: readonly PersonalConnection[],
  bindings: readonly ConnectorPickerBindingRef[]
): ConnectorPickerRow[] {
  const connectionsByConnector = new Map<string, PersonalConnection[]>();
  for (const connection of connections) {
    const existing = connectionsByConnector.get(connection.connector_id) ?? [];
    existing.push(connection);
    connectionsByConnector.set(connection.connector_id, existing);
  }

  const bindingByConnection = new Map<string, ConnectorPickerBindingRef>();
  const currentBindingByConnector = new Map<string, ConnectorPickerBindingRef>();
  for (const binding of bindings) {
    bindingByConnection.set(binding.connection_id, binding);
    if (!currentBindingByConnector.has(binding.connector_id)) {
      currentBindingByConnector.set(binding.connector_id, binding);
    }
  }

  const rows: ConnectorPickerRow[] = [];
  for (const connector of catalog) {
    const connectorConnections = connectionsByConnector.get(connector.id) ?? [];
    const activeBinding = currentBindingByConnector.get(connector.id);
    const activeConnection = activeBinding
      ? connectorConnections.find(connection => connection.id === activeBinding.connection_id)
      : undefined;

    for (const connection of connectorConnections) {
      const binding = bindingByConnection.get(connection.id);
      const isCurrentConnection = binding?.id === activeBinding?.id && binding?.status === 'active';
      rows.push({
        key: `connection:${connection.id}`,
        connector,
        connection,
        binding,
        replacedBinding: activeBinding && !isCurrentConnection ? activeBinding : undefined,
        replacedConnection: activeBinding && !isCurrentConnection ? activeConnection : undefined,
        ...rowState(connector, connection, binding),
      });
    }

    // Setup is an independent create operation. Keep it available after the
    // first connection so users can save multiple Keys without overwriting an
    // existing connection or its target binding.
    if (connectorConnections.length === 0 || supportsConnectorSetup(connector)) {
      rows.push({
        key: `connector:${connector.id}:new`,
        connector,
        replacedBinding: activeBinding,
        replacedConnection: activeConnection,
        ...rowState(connector, undefined, undefined),
      });
    }
  }
  return rows;
}

function searchableText(row: ConnectorPickerRow): string {
  return [
    ...connectorCatalogSearchValues(row.connector),
    row.connection?.name,
    row.connection?.external_display_name,
  ]
    .filter((value): value is string => Boolean(value))
    .join(' ')
    .toLocaleLowerCase();
}

function matchesStatus(row: ConnectorPickerRow, status: ConnectorCatalogStatus): boolean {
  if (status === 'all') return true;
  if (status === 'connected') {
    return row.state === 'added' || row.state === 'ready';
  }
  return row.state === 'reconnect' || row.state === 'unavailable';
}

export function filterConnectorPickerRows(
  rows: readonly ConnectorPickerRow[],
  filters: ConnectorPickerFilters
): ConnectorPickerRow[] {
  const terms = filters.query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  return rows.filter(row => {
    if (!matchesStatus(row, filters.status)) return false;
    if (terms.length === 0) return true;
    const haystack = searchableText(row);
    return terms.every(term => haystack.includes(term));
  });
}

/** Recheck the displayed source slot before confirming an in-editor switch. */
export function isConnectorPickerSwitchCurrent(
  request: ConnectorPickerSwitchRequest,
  connections: readonly PersonalConnection[],
  bindings: readonly ConnectorPickerBindingRef[],
  target: ConnectorTargetRef
): boolean {
  if (
    request.target?.type !== target.type ||
    request.target?.id !== target.id ||
    (request.target?.binding_instance_id ?? '') !== (target.binding_instance_id ?? '')
  ) {
    return false;
  }
  const selectedConnection = connections.find(
    connection =>
      connection.id === request.connection.id && connection.connector_id === request.connector.id
  );
  if (selectedConnection?.state !== 'ready') return false;
  const currentBinding = bindings.find(binding => binding.connector_id === request.connector.id);
  return Boolean(
    currentBinding &&
      currentBinding.id === request.replacedBinding.id &&
      currentBinding.connection_id === request.replacedBinding.connection_id &&
      currentBinding.status === request.replacedBinding.status &&
      currentBinding.revision === request.replacedBinding.revision
  );
}

export async function requestConnectorPickerAction(
  row: ConnectorPickerRow,
  handlers: ConnectorPickerActionHandlers,
  target?: ConnectorTargetRef
): Promise<void> {
  const request: ConnectorPickerActionRequest = {
    connector: row.connector,
    connection: row.connection,
    binding: row.binding,
    replacedBinding: row.replacedBinding,
    replacedConnection: row.replacedConnection,
    reason: row.reason,
    target,
  };

  if (row.state === 'ready' && row.connection) {
    if (row.replacedBinding) {
      await handlers.onRequestSwitch?.({
        ...request,
        connection: row.connection,
        replacedBinding: row.replacedBinding,
      });
      return;
    }
    await handlers.onRequestAdd?.({ ...request, connection: row.connection });
    return;
  }
  if (row.state === 'added' && row.binding) {
    await handlers.onRequestDetach?.({ ...request, binding: row.binding });
    return;
  }
  if (row.state === 'not_connected') {
    await handlers.onRequestConnectAndAdd?.(request);
    return;
  }
  if (row.state === 'reconnect' && row.connection) {
    await handlers.onRequestUpdateKey?.({ ...request, connection: row.connection });
    return;
  }
  if (row.state === 'unavailable' && row.connection) {
    await handlers.onRequestUpdateKey?.({ ...request, connection: row.connection });
    return;
  }
  if (row.state === 'unavailable') {
    await handlers.onRequestViewReason?.(request);
  }
}
