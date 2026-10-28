/**
 * Pure create-time Connector selection state.
 *
 * Creation happens before any draft exists, so it cannot reuse the draft
 * editor's versioned session. This module keeps the browser-side selection in
 * one shape the picker edits and the create flow submits:
 *
 * - every selected connection carries the candidate snapshot the user saw, so
 *   `composeCreateBindings` can validate the exact connector/connection pair,
 *   revision, and allowed operations it was chosen from;
 * - a connection is only selectable while the server marks it available and
 *   offers at least one operation;
 * - the selected operations are always a non-empty subset of the candidate's
 *   allowed operations, so the browser can narrow authority but never widen it;
 * - the copy built here only carries operation identifiers. Credential material
 *   and credential handles never enter this state.
 *
 * The operation identifiers themselves come from the shared Digital Human
 * Connector model, so create-time selection and draft editing cannot drift.
 */

import {
  connectorOperationIds,
  createConnectorDefaultOperationIds,
  updateDigitalHumanApprovalPolicy,
  type ConnectorApprovalPolicy,
} from '../digital-human-connector-model';
import type {
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';

/** One selected connection: the replacement row plus the candidate it came from. */
export interface DigitalHumanCreateConnectorSelection {
  binding: DigitalHumanResourceBindingInput;
  candidate: DigitalHumanResourceCandidate;
}

/** Identity of a connection: its connector plus its own connection id. */
export function createConnectorSelectionKey(
  resourceId: string,
  parentResourceId?: string
): string {
  return JSON.stringify([parentResourceId ?? '', resourceId]);
}

export function createConnectorCandidateKey(
  candidate: Pick<DigitalHumanResourceCandidate, 'resource_id' | 'parent_resource_id'>
): string {
  return createConnectorSelectionKey(candidate.resource_id, candidate.parent_resource_id);
}

/**
 * A candidate the create picker may add.
 *
 * Only the native Connector type counts; the server marks availability, and a
 * connection without operations cannot be granted any authority.
 */
export function createConnectorCandidateSelectable(
  candidate: DigitalHumanResourceCandidate
): boolean {
  return (
    candidate.type === 'connector' &&
    candidate.available === true &&
    connectorOperationIds(candidate).length > 0
  );
}

/**
 * Snapshot the candidate so later server responses cannot mutate the selection.
 *
 * This is an allowlist, not a spread: the fetched candidate is untrusted input,
 * so only the fields the UI and `composeCreateBindings` actually read are
 * copied. Any extra top-level property, extra metadata key, or credential-like
 * value the server sends now or later stays out of the user's snapshot, and both
 * arrays are deep-copied so in-place mutation of the source cannot silently
 * rewrite the authority the user selected.
 */
function snapshotCandidate(
  candidate: DigitalHumanResourceCandidate
): DigitalHumanResourceCandidate {
  return {
    type: candidate.type,
    resource_id: candidate.resource_id,
    parent_resource_id: candidate.parent_resource_id,
    display_name: candidate.display_name,
    description: candidate.description,
    resource_revision: candidate.resource_revision,
    access_modes: [...candidate.access_modes],
    status: candidate.status,
    available: candidate.available,
    metadata: {
      // `connectorOperationIds` already returns a fresh, validated array.
      allowed_operation_ids: connectorOperationIds(candidate),
    },
  };
}

/**
 * Add a connection with its default operations: everything the candidate
 * currently allows except the opt-in write operations, which remain available
 * to tick.
 *
 * An unavailable connection, a connection with no operations, or a repeat of an
 * already selected connection leaves the list unchanged.
 */
export function addCreateConnectorSelection(
  selections: readonly DigitalHumanCreateConnectorSelection[],
  candidate: DigitalHumanResourceCandidate
): readonly DigitalHumanCreateConnectorSelection[] {
  if (!createConnectorCandidateSelectable(candidate)) return selections;
  const key = createConnectorCandidateKey(candidate);
  if (selections.some(item => createConnectorCandidateKey(item.candidate) === key)) {
    return selections;
  }
  const snapshot = snapshotCandidate(candidate);
  return [
    ...selections,
    {
      binding: {
        type: 'connector',
        resource_id: snapshot.resource_id,
        parent_resource_id: snapshot.parent_resource_id,
        display_name: snapshot.display_name,
        access_mode: 'execute',
        resource_revision: snapshot.resource_revision,
        metadata: { allowed_operation_ids: createConnectorDefaultOperationIds(snapshot) },
      },
      candidate: snapshot,
    },
  ];
}

export function removeCreateConnectorSelection(
  selections: readonly DigitalHumanCreateConnectorSelection[],
  key: string
): readonly DigitalHumanCreateConnectorSelection[] {
  return selections.filter(item => createConnectorCandidateKey(item.candidate) !== key);
}

export function toggleCreateConnectorSelection(
  selections: readonly DigitalHumanCreateConnectorSelection[],
  candidate: DigitalHumanResourceCandidate
): readonly DigitalHumanCreateConnectorSelection[] {
  const key = createConnectorCandidateKey(candidate);
  if (selections.some(item => createConnectorCandidateKey(item.candidate) === key)) {
    return removeCreateConnectorSelection(selections, key);
  }
  return addCreateConnectorSelection(selections, candidate);
}

/**
 * Narrow or restore one operation on one selected connection.
 *
 * Checking is refused unless the candidate currently allows that operation, so
 * the browser can never invent authority. Unchecking the last selected
 * operation is refused too, because an empty subset would mean "no operation"
 * and the create request must never send an unusable binding.
 */
export function setCreateConnectorSelectionOperation(
  selections: readonly DigitalHumanCreateConnectorSelection[],
  key: string,
  operationId: string,
  checked: boolean
): readonly DigitalHumanCreateConnectorSelection[] {
  return selections.map(item => {
    if (createConnectorCandidateKey(item.candidate) !== key) return item;
    const allowed = connectorOperationIds(item.candidate);
    const current = connectorOperationIds(item.binding);
    if (checked) {
      if (!allowed.includes(operationId) || current.includes(operationId)) return item;
      return {
        ...item,
        binding: {
          ...item.binding,
          metadata: {
            ...item.binding.metadata,
            allowed_operation_ids: [...current, operationId],
          },
        },
      };
    }
    if (!current.includes(operationId) || current.length <= 1) return item;
    return {
      ...item,
      binding: {
        ...item.binding,
        metadata: {
          ...item.binding.metadata,
          allowed_operation_ids: current.filter(id => id !== operationId),
        },
      },
    };
  });
}

/** Choose how one selected connection asks before a call (ADR 0026). */
export function setCreateConnectorSelectionApprovalPolicy(
  selections: readonly DigitalHumanCreateConnectorSelection[],
  key: string,
  policy: ConnectorApprovalPolicy
): readonly DigitalHumanCreateConnectorSelection[] {
  return selections.map(item =>
    createConnectorCandidateKey(item.candidate) !== key
      ? item
      : {
          ...item,
          binding: updateDigitalHumanApprovalPolicy([item.binding], item.binding, policy)[0],
        }
  );
}

export function createConnectorSelectionBindings(
  selections: readonly DigitalHumanCreateConnectorSelection[]
): DigitalHumanResourceBindingInput[] {
  return selections.map(item => item.binding);
}

/**
 * Candidate snapshots that must travel to `composeCreateBindings`.
 *
 * They are the exact candidates the user selected against, so composition
 * validates the revision and operation set the user actually saw rather than a
 * later, silently widened server response.
 */
export function createConnectorSelectionCandidates(
  selections: readonly DigitalHumanCreateConnectorSelection[]
): DigitalHumanResourceCandidate[] {
  return selections.map(item => item.candidate);
}

/** A selection is a deliberate edit, so the form asks before discarding it. */
export function createConnectorSelectionsDirty(
  selections: readonly DigitalHumanCreateConnectorSelection[]
): boolean {
  return selections.length > 0;
}

/**
 * What a server connection lets the operations specialist do, as three
 * presets instead of one checkbox per operation.
 */
export type ServerCapabilityPreset = 'inspect' | 'manage' | 'full';

const SERVER_PRESET_OPERATIONS: Record<ServerCapabilityPreset, readonly string[]> = {
  inspect: ['server.system.snapshot', 'server.service.status', 'server.journal.tail'],
  manage: [
    'server.system.snapshot',
    'server.service.status',
    'server.journal.tail',
    'server.service.restart',
    'server.service.start',
    'server.service.stop',
    'server.service.reload',
  ],
  full: [
    'server.system.snapshot',
    'server.service.status',
    'server.journal.tail',
    'server.service.restart',
    'server.service.start',
    'server.service.stop',
    'server.service.reload',
    'server.command.run',
  ],
};

/** The operations a preset grants, limited to what the connection allows. */
export function serverPresetOperationIds(
  preset: ServerCapabilityPreset,
  candidate: Pick<DigitalHumanResourceCandidate, 'metadata'>
): string[] {
  const allowed = connectorOperationIds(candidate);
  return SERVER_PRESET_OPERATIONS[preset].filter(id => allowed.includes(id));
}

/** The preset a selection's operations amount to: the narrowest that covers them. */
export function serverPresetOf(
  selection: DigitalHumanCreateConnectorSelection
): ServerCapabilityPreset {
  const operations = connectorOperationIds(selection.binding);
  if (operations.includes('server.command.run')) return 'full';
  const inspect = SERVER_PRESET_OPERATIONS.inspect;
  return operations.some(id => !inspect.includes(id)) ? 'manage' : 'inspect';
}

/**
 * Use one server for the specialist: it replaces any earlier connection of the
 * same connector, with the preset's operations and the approval policy.
 */
export function selectServerConnection(
  selections: readonly DigitalHumanCreateConnectorSelection[],
  candidate: DigitalHumanResourceCandidate,
  preset: ServerCapabilityPreset,
  policy: ConnectorApprovalPolicy
): readonly DigitalHumanCreateConnectorSelection[] {
  const others = selections.filter(
    item => item.candidate.parent_resource_id !== candidate.parent_resource_id
  );
  const added = addCreateConnectorSelection(others, candidate);
  const key = createConnectorCandidateKey(candidate);
  const operations = serverPresetOperationIds(preset, candidate);
  if (operations.length === 0) return selections;
  return setCreateConnectorSelectionApprovalPolicy(
    added.map(item =>
      createConnectorCandidateKey(item.candidate) !== key
        ? item
        : {
            ...item,
            binding: {
              ...item.binding,
              metadata: { ...item.binding.metadata, allowed_operation_ids: operations },
            },
          }
    ),
    key,
    policy
  );
}
