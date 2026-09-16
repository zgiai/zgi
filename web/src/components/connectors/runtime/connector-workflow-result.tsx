'use client';
import { parseConnectorNodeResult } from './connector-runtime-source';
import { ConnectorRuntimeSurface } from './connector-runtime-surface';
import { submitConnectorRuntimeDecision } from './connector-runtime-transport';

export function ConnectorWorkflowResult({
  metadata,
  value,
}: {
  metadata: unknown;
  value: unknown;
}) {
  const result = parseConnectorNodeResult(metadata, value);
  return result ? (
    <ConnectorRuntimeSurface result={result} onDecision={submitConnectorRuntimeDecision} compact />
  ) : null;
}
