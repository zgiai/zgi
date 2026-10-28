/**
 * Pure composition of the create-time bindings replacement list.
 *
 * A new digital human receives its capabilities through a single
 * `replaceDraftBindings` call, so creation must hand that call one list that
 * merges the selected skills and the selected Connector connections.
 *
 * Skills keep the create-time contract they already had: only candidates the
 * workspace really offers are bound, in candidate order. Connector selections
 * are strict instead of best-effort, because a connection carries execution
 * authority. A selection whose connector/connection pair no longer has an
 * exact, available, current candidate fails loudly rather than disappearing
 * from the request, and a selection may only narrow the operations the live
 * candidate allows, never widen them.
 *
 * Only the candidate's own revision, display data, and operation identifiers
 * are copied. The server-side credential handle stays empty, so neither a
 * credential reference nor any credential material can travel through this
 * list.
 *
 * The Connector rules are imported from the shared Digital Human Connector
 * model so create-time composition and draft editing cannot drift apart.
 */

import {
  connectorApprovalPolicy,
  connectorOperationIds,
  connectorSupportsReadOnlyAuto,
  isDigitalHumanConnector,
} from '../digital-human-connector-model';
import { selectCandidateBindings } from './create-digital-human-payload';
import type {
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';

/** Identity of one selected connection: its connector plus its connection id. */
function connectionIdentity(connectorId: string, connectionId: string): string {
  return JSON.stringify([connectorId, connectionId]);
}

/** Human-readable label for errors, never for the request body. */
function connectionLabel(connectorId: string, connectionId: string): string {
  return `${connectorId || '(unknown connector)'} / ${connectionId}`;
}

/** Build the replacement row for one admitted connection, credential-free. */
function connectorBindingFromCandidate(
  candidate: DigitalHumanResourceCandidate,
  operationIds: readonly string[],
  readOnlyAuto = false
): DigitalHumanResourceBindingInput {
  return {
    type: 'connector',
    resource_id: candidate.resource_id,
    parent_resource_id: candidate.parent_resource_id,
    display_name: candidate.display_name,
    access_mode: 'execute',
    resource_revision: candidate.resource_revision,
    // Allowlist, not a spread: candidate metadata is untrusted input, so only
    // the validated operation identifiers may travel. Any other key the server
    // (now or later) puts in candidate metadata is dropped here.
    // The approval policy is the person's own choice (ADR 0026), and only a
    // connection that can prove a call read-only offers it.
    metadata: {
      allowed_operation_ids: [...operationIds],
      ...(readOnlyAuto ? { approval_policy: 'read_only_auto' } : {}),
    },
  };
}

/**
 * Resolve the selected connection bindings against the live candidates.
 *
 * Every selected connection must still match exactly one candidate with
 * `Available=true`, the same revision the user saw, a non-empty allowed
 * operation set, and a non-empty selection that is a subset of those allowed
 * operations. Different connections of the same connector are preserved; an
 * exact repeat is deduplicated while two different operation sets for the same
 * connection are rejected as ambiguous.
 */
function selectCreateConnectorBindings(
  selected: readonly DigitalHumanResourceBindingInput[],
  candidates: readonly DigitalHumanResourceCandidate[]
): DigitalHumanResourceBindingInput[] {
  if (selected.length === 0) return [];

  const candidatesByIdentity = new Map<string, DigitalHumanResourceCandidate>();
  for (const candidate of candidates) {
    if (candidate.type !== 'connector') continue;
    candidatesByIdentity.set(
      connectionIdentity(candidate.parent_resource_id ?? '', candidate.resource_id),
      candidate
    );
  }

  const items: DigitalHumanResourceBindingInput[] = [];
  const chosenOperations = new Map<string, string>();

  for (const binding of selected) {
    if (!isDigitalHumanConnector(binding) || binding.type !== 'connector') {
      throw new Error('Create binding composition only accepts Connector selections');
    }
    const connectorId = binding.parent_resource_id ?? '';
    const connectionId = binding.resource_id;
    const label = connectionLabel(connectorId, connectionId);
    const identity = connectionIdentity(connectorId, connectionId);
    const candidate = candidatesByIdentity.get(identity);
    if (!candidate) {
      throw new Error(`Connector connection ${label} is not offered to this workspace`);
    }
    if (candidate.available !== true) {
      throw new Error(`Connector connection ${label} is not available`);
    }
    if (binding.resource_revision !== candidate.resource_revision) {
      throw new Error(`Connector connection ${label} changed since it was selected`);
    }
    const allowed = connectorOperationIds(candidate);
    if (allowed.length === 0) {
      throw new Error(`Connector connection ${label} offers no operations`);
    }
    const operations = connectorOperationIds(binding);
    if (operations.length === 0) {
      throw new Error(`Connector connection ${label} has no selected operations`);
    }
    if (operations.some(operation => !allowed.includes(operation))) {
      throw new Error(`Connector connection ${label} selects operations outside its allowed set`);
    }

    // An exact repeat is the same connection with the same operations; anything
    // else would silently merge two different authority sets.
    const signature = [...operations].sort().join('\u0000');
    const previous = chosenOperations.get(identity);
    if (previous !== undefined) {
      if (previous !== signature) {
        throw new Error(`Connector connection ${label} is selected twice with different operations`);
      }
      continue;
    }
    chosenOperations.set(identity, signature);
    items.push(
      connectorBindingFromCandidate(
        candidate,
        operations,
        connectorSupportsReadOnlyAuto(binding) &&
          connectorApprovalPolicy(binding) === 'read_only_auto'
      )
    );
  }

  return items;
}

/**
 * Build the single replacement list for the create-time bindings call.
 *
 * Selected skills are resolved exactly as before; selected Connector
 * connections are admitted strictly and appended after them. An empty
 * selection on both sides returns an empty list, so the caller can skip the
 * bindings call entirely.
 */
export function composeCreateBindings(
  selectedSkillIds: readonly string[],
  skillCandidates: readonly DigitalHumanResourceCandidate[],
  selectedConnectorBindings: readonly DigitalHumanResourceBindingInput[],
  connectorCandidates: readonly DigitalHumanResourceCandidate[]
): DigitalHumanResourceBindingInput[] {
  const skillItems = selectCandidateBindings(selectedSkillIds, skillCandidates);
  const connectorItems = selectCreateConnectorBindings(
    selectedConnectorBindings,
    connectorCandidates
  );
  return [...skillItems, ...connectorItems];
}
