/** @param {unknown} value @returns {Record<string, unknown>} */
function asRecord(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
}

/** @param {unknown} value @returns {string} */
function nonEmptyString(value) {
  return typeof value === 'string' && value.trim() ? value.trim() : '';
}

/**
 * Returns the business payload from either a durable workflow event envelope
 * or a legacy flat workflow event.
 *
 * @param {unknown} payload
 * @returns {Record<string, unknown>}
 */
export function workflowEventData(payload) {
  const envelope = asRecord(payload);
  const nested = asRecord(envelope.data);
  return Object.keys(nested).length > 0 ? nested : envelope;
}

/**
 * Extracts only fields whose contract explicitly identifies a workflow run.
 * Generic `id` is deliberately excluded: pause snapshots use it for pause_id,
 * while node events use it for node_execution_id.
 *
 * @param {unknown} payload
 * @returns {string}
 */
export function explicitWorkflowRunId(payload) {
  const data = workflowEventData(payload);
  return (
    nonEmptyString(data.workflow_run_id) ||
    nonEmptyString(data.workflowRunId) ||
    nonEmptyString(data.correlation_id)
  );
}

/**
 * Resolves a run id without allowing an ambiguous event `id` to override a
 * run already owned by the caller.
 *
 * @param {unknown} payload
 * @param {{ fallback?: string, allowLegacyId?: boolean }=} options
 * @returns {string}
 */
export function resolveWorkflowRunId(payload, options = {}) {
  const explicit = explicitWorkflowRunId(payload);
  if (explicit) return explicit;

  const fallback = nonEmptyString(options.fallback);
  if (fallback) return fallback;

  if (!options.allowLegacyId) return '';
  return nonEmptyString(workflowEventData(payload).id);
}

/**
 * Pins a workflow run identity for one transport invocation. Once established,
 * node, pause, approval, and replay events cannot replace it.
 *
 * @param {string} current
 * @param {unknown} payload
 * @param {{ allowLegacyId?: boolean }=} options
 * @returns {string}
 */
export function pinWorkflowRunId(current, payload, options = {}) {
  const pinned = nonEmptyString(current);
  if (pinned) return pinned;
  return resolveWorkflowRunId(payload, options);
}
