import type {
  DigitalHumanResourceBinding,
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
  DraftResourceBindings,
} from '@/services/types/digital-human';

export type ConnectorSelectionStatus = 'editing' | 'saving' | 'conflict' | 'failed';

/**
 * One editing session over a digital human's versioned draft bindings.
 *
 * `baseline` is a snapshot of the server draft that the session was opened
 * against; `pending` is the full replacement list the host would submit. The
 * list always contains every binding type so a Connector edit can never drop a
 * skill or database someone else kept in the same draft.
 */
export interface DigitalHumanConnectorSelection {
  token: string;
  baseline: DraftResourceBindings;
  pending: DigitalHumanResourceBindingInput[];
  status: ConnectorSelectionStatus;
}

/**
 * Project a stored binding into the editable replacement shape.
 *
 * The credential reference is copied as an opaque server-side handle; a
 * Connector binding only ever carries an empty value, so no credential material
 * is read from or written to the browser DTO.
 */
export function digitalHumanBindingInput(
  item: DigitalHumanResourceBinding
): DigitalHumanResourceBindingInput {
  return {
    id: item.id,
    type: item.binding_type,
    resource_id: item.resource_id,
    parent_resource_id: item.parent_resource_id,
    display_name: item.display_name,
    access_mode: item.access_mode,
    resource_revision: item.resource_revision,
    credential_ref: item.credential_ref,
    metadata: structuredClone(item.metadata ?? {}),
  };
}

export function isDigitalHumanConnector(item: { type: string }) {
  return item.type === 'connector' || item.type === 'integration_connection';
}

export function digitalHumanResourceKey(item: DigitalHumanResourceBindingInput) {
  return (
    item.id ||
    `${item.type}:${item.parent_resource_id ?? ''}:${item.resource_id}:${item.access_mode}`
  );
}

/**
 * A per-user slot (ADR 0007) runs with each requester's own connection, so it
 * names no connection: resource_id is the slot key.
 */
export function isPerUserConnectorSlot(item: { metadata?: Record<string, unknown> }) {
  return item.metadata?.credential_mode === 'per_user';
}

/** Turns a selected connection into a per-user slot of the same connector. */
export function toPerUserConnectorSlot(
  item: DigitalHumanResourceBindingInput,
  connectorName: string
): DigitalHumanResourceBindingInput {
  return {
    type: 'connector',
    resource_id: crypto.randomUUID(),
    parent_resource_id: item.parent_resource_id,
    display_name: connectorName || item.display_name,
    access_mode: 'execute',
    resource_revision: 'connector:per-user',
    metadata: {
      allowed_operation_ids: connectorOperationIds(item),
      credential_mode: 'per_user',
    },
  };
}

export function connectorOperationIds(item: { metadata: Record<string, unknown> }): string[] {
  const value = item.metadata.allowed_operation_ids;
  return Array.isArray(value)
    ? [...new Set(value.filter((id): id is string => typeof id === 'string' && Boolean(id.trim())))]
    : [];
}

export function beginConnectorSelection(
  token: string,
  baseline: DraftResourceBindings
): DigitalHumanConnectorSelection {
  return {
    token,
    baseline: structuredClone(baseline),
    pending: baseline.items.map(digitalHumanBindingInput),
    status: 'editing',
  };
}

export function observeConnectorDraft(
  session: DigitalHumanConnectorSelection,
  current: DraftResourceBindings
): DigitalHumanConnectorSelection {
  // onSuccess may refetch before mutateAsync resolves, so the submitted request
  // still determines the outcome while saving rather than its own cache refresh.
  if (
    session.status === 'saving' ||
    (current.draft_version_id === session.baseline.draft_version_id &&
      current.draft_lock_version === session.baseline.draft_lock_version)
  ) {
    return session;
  }
  return { ...session, status: 'conflict' };
}

export function newestConnectorDraft(
  response: DraftResourceBindings,
  cached?: DraftResourceBindings
): DraftResourceBindings {
  return cached &&
    response.draft_version_id &&
    cached.draft_version_id === response.draft_version_id &&
    cached.draft_lock_version > response.draft_lock_version
    ? cached
    : response;
}

export function savedConnectorDraft(
  response: DraftResourceBindings,
  cached?: DraftResourceBindings
): DraftResourceBindings {
  // A delayed save from the old draft must not overwrite a new publication's
  // draft. GET is authoritative across generations; mutation responses are not.
  if (cached && cached.draft_version_id !== response.draft_version_id) return cached;
  return newestConnectorDraft(response, cached);
}

export function queriedConnectorDraft(
  response: DraftResourceBindings,
  cached: DraftResourceBindings | undefined,
  startedDraftVersionId: string | undefined
): DraftResourceBindings {
  // A newer request or publication installed another generation after this GET
  // began. Its cache entry wins over an old, delayed response.
  if (
    cached?.draft_version_id &&
    cached.draft_version_id !== startedDraftVersionId &&
    cached.draft_version_id !== response.draft_version_id
  ) {
    return cached;
  }
  return newestConnectorDraft(response, cached);
}

/**
 * Select a Connector connection into the pending replacement list.
 *
 * Operations default to every currently allowed operation. When the user is
 * replacing an unavailable reference, only the operations both sides admit
 * survive so a replacement never widens authority.
 */
export function selectDigitalHumanConnection(
  items: DigitalHumanResourceBindingInput[],
  candidate: DigitalHumanResourceCandidate,
  replaced?: DigitalHumanResourceBindingInput
): DigitalHumanResourceBindingInput[] {
  if (
    candidate.type !== 'connector' ||
    !candidate.available ||
    connectorOperationIds(candidate).length === 0
  ) {
    throw new Error('Connection is not available to this digital human');
  }
  const existing = items.find(
    item =>
      item.type === 'connector' &&
      item.resource_id === candidate.resource_id &&
      item.parent_resource_id === candidate.parent_resource_id
  );
  if (
    existing &&
    (!replaced || digitalHumanResourceKey(existing) === digitalHumanResourceKey(replaced))
  ) {
    return items;
  }
  // Selecting an already selected connection must not merge two permission sets.
  if (existing) throw new Error('Connection is already selected');
  const allowed = connectorOperationIds(candidate);
  // A new connection starts with what it allows minus the opt-in write
  // operations (ADR 0023); a replaced one keeps its earlier choice.
  const operations = !replaced
    ? createConnectorDefaultOperationIds(candidate)
    : replaced.type === 'connector'
      ? connectorOperationIds(replaced).filter(id => allowed.includes(id))
      : [];
  const next: DigitalHumanResourceBindingInput = {
    type: 'connector',
    resource_id: candidate.resource_id,
    parent_resource_id: candidate.parent_resource_id,
    display_name: candidate.display_name,
    access_mode: 'execute',
    resource_revision: candidate.resource_revision,
    metadata: { ...structuredClone(candidate.metadata), allowed_operation_ids: operations },
  };
  return [
    ...items.filter(
      item => !replaced || digitalHumanResourceKey(item) !== digitalHumanResourceKey(replaced)
    ),
    next,
  ];
}

export function updateDigitalHumanOperations(
  items: DigitalHumanResourceBindingInput[],
  item: DigitalHumanResourceBindingInput,
  candidate: DigitalHumanResourceCandidate | undefined,
  operationId: string,
  checked: boolean
): DigitalHumanResourceBindingInput[] {
  if (item.type !== 'connector' || !candidate?.available) return items;
  if (checked && !connectorOperationIds(candidate).includes(operationId)) return items;
  return items.map(value =>
    digitalHumanResourceKey(value) !== digitalHumanResourceKey(item)
      ? value
      : {
          ...value,
          resource_revision: candidate.resource_revision,
          metadata: {
            ...value.metadata,
            allowed_operation_ids: checked
              ? [...new Set([...connectorOperationIds(value), operationId])]
              : connectorOperationIds(value).filter(id => id !== operationId),
          },
        }
  );
}

function stableValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableValue);
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value)
        .filter(([, item]) => item !== undefined)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, item]) => [key, stableValue(item)])
    );
  }
  return value;
}

export function sameDigitalHumanBinding(
  left: DigitalHumanResourceBindingInput,
  right: DigitalHumanResourceBindingInput
) {
  return JSON.stringify(stableValue(left)) === JSON.stringify(stableValue(right));
}

export function connectorSelectionChanged(session: DigitalHumanConnectorSelection) {
  const before = session.baseline.items.map(digitalHumanBindingInput);
  return (
    before.length !== session.pending.length ||
    session.pending.some(item => {
      const existing = before.find(
        value => digitalHumanResourceKey(value) === digitalHumanResourceKey(item)
      );
      return !existing || !sameDigitalHumanBinding(existing, item);
    })
  );
}

export function connectorSelectionInvalid(
  session: DigitalHumanConnectorSelection,
  candidates: readonly DigitalHumanResourceCandidate[]
) {
  return session.pending.some(item => {
    if (!isDigitalHumanConnector(item)) return false;
    const original = session.baseline.items.find(value => value.id === item.id);
    if (original && sameDigitalHumanBinding(digitalHumanBindingInput(original), item)) return false;
    if (item.type !== 'connector') return true;
    // The server checks a per-user slot against the connector itself.
    if (isPerUserConnectorSlot(item)) return connectorOperationIds(item).length === 0;
    const candidate = candidates.find(
      value =>
        value.resource_id === item.resource_id &&
        value.parent_resource_id === item.parent_resource_id
    );
    const operations = connectorOperationIds(item);
    return (
      !candidate?.available ||
      operations.length === 0 ||
      operations.some(id => !connectorOperationIds(candidate).includes(id))
    );
  });
}

/**
 * Operations that change a server and therefore start unticked.
 *
 * Resource candidates carry only operation ids, not their catalog effect, so
 * the write operations are named explicitly: the four SSH Server Operations
 * service actions. They stay selectable; the user must tick them on purpose.
 */
const CREATE_CONNECTOR_OPT_IN_OPERATION_IDS: ReadonlySet<string> = new Set([
  'server.service.restart',
  'server.service.start',
  'server.service.stop',
  'server.service.reload',
  'server.command.run',
]);

/**
 * Operations that only turn on after the person acknowledges their risk, such
 * as running any shell command on a server.
 */
export const RISK_ACKNOWLEDGED_CONNECTOR_OPERATION_IDS: ReadonlySet<string> = new Set([
  'server.command.run',
]);

/**
 * The operations a newly added connection starts with: every allowed operation
 * except the opt-in write operations. A connection that offers only opt-in
 * operations keeps them all, because an empty set would be unusable.
 */
export function createConnectorDefaultOperationIds(
  candidate: Pick<DigitalHumanResourceCandidate, 'metadata'>
): string[] {
  const allowed = connectorOperationIds(candidate);
  const defaults = allowed.filter(id => !CREATE_CONNECTOR_OPT_IN_OPERATION_IDS.has(id));
  return defaults.length > 0 ? defaults : allowed;
}
