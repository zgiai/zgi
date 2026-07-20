/**
 * @typedef {Object} WorkflowRuntimeEventEnvelope
 * @property {string=} event
 * @property {number} sequence
 * @property {number=} schemaVersion
 * @property {number=} payloadVersion
 * @property {string=} executionId
 * @property {number=} executionGeneration
 * @property {string=} pauseId
 * @property {number=} pauseGeneration
 * @property {Record<string, unknown>} payload
 */

/** @param {unknown} value @returns {Record<string, unknown>} */
function asRecord(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
}

/** @param {unknown} value @returns {number | undefined} */
function finitePositiveNumber(value) {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : undefined;
}

/** @param {unknown} value @returns {string | undefined} */
function stringValue(value) {
  return typeof value === 'string' && value ? value : undefined;
}

/**
 * Normalizes both the native Workflow V2 envelope and Agent-relayed flat workflow events.
 * Durable envelope fields always win over same-named payload fields.
 *
 * @param {unknown} value
 * @param {string | null=} fallbackEvent
 * @returns {WorkflowRuntimeEventEnvelope}
 */
export function normalizeWorkflowRuntimeEvent(value, fallbackEvent) {
  const envelope = asRecord(value);
  const nested = asRecord(envelope.data);
  const sequence = finitePositiveNumber(envelope.sequence) ?? finitePositiveNumber(nested.sequence) ?? 0;
  const event = stringValue(envelope.event) ?? stringValue(nested.event) ?? fallbackEvent ?? undefined;
  const schemaVersion = finitePositiveNumber(envelope.schema_version) ??
    finitePositiveNumber(nested.schema_version);
  const payloadVersion = finitePositiveNumber(envelope.payload_version) ??
    finitePositiveNumber(nested.payload_version);
  const executionId = stringValue(envelope.execution_id) ?? stringValue(nested.execution_id);
  const executionGeneration = finitePositiveNumber(envelope.execution_generation) ??
    finitePositiveNumber(nested.execution_generation);
  const pauseId = stringValue(envelope.pause_id) ?? stringValue(nested.pause_id);
  const pauseGeneration = finitePositiveNumber(envelope.pause_generation) ??
    finitePositiveNumber(nested.pause_generation);

  return {
    event,
    sequence,
    schemaVersion,
    payloadVersion,
    executionId,
    executionGeneration,
    pauseId,
    pauseGeneration,
    payload: {
      ...envelope,
      ...nested,
      ...(event ? { event } : {}),
      ...(sequence > 0 ? { sequence } : {}),
      ...(schemaVersion ? { schema_version: schemaVersion } : {}),
      ...(payloadVersion ? { payload_version: payloadVersion } : {}),
      ...(executionId ? { execution_id: executionId } : {}),
      ...(executionGeneration ? { execution_generation: executionGeneration } : {}),
      ...(pauseId ? { pause_id: pauseId } : {}),
      ...(pauseGeneration ? { pause_generation: pauseGeneration } : {}),
    },
  };
}

/** @param {unknown} value @returns {number} */
export function workflowRuntimeEventSequence(value) {
  return normalizeWorkflowRuntimeEvent(value).sequence;
}

/** @param {unknown} value @param {number=} cursor @returns {boolean} */
export function isStaleWorkflowRuntimeEvent(value, cursor) {
  const sequence = workflowRuntimeEventSequence(value);
  return sequence > 0 && typeof cursor === 'number' && sequence <= cursor;
}
