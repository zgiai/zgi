'use client';

import type { ConnectorRuntimeExecutionResult } from './connector-runtime-model';
import {
  ConnectorRuntimeConfirmationCard,
  type ConnectorRuntimeDecisionHandler,
} from './connector-runtime-confirmation-card';
import { ConnectorRuntimeBlockedState } from './connector-runtime-blocked-state';
import { ConnectorRuntimeTerminalState } from './connector-runtime-terminal-state';

interface ConnectorRuntimeSurfaceProps {
  result: ConnectorRuntimeExecutionResult;
  onDecision: ConnectorRuntimeDecisionHandler;
  compact?: boolean;
  className?: string;
}

/**
 * Shared host surface for AI Chat, Agent, and Workflow runtime adapters.
 * Every durable terminal remains visible after its host reloads persisted state.
 */
export function ConnectorRuntimeSurface({
  result,
  onDecision,
  compact = false,
  className,
}: ConnectorRuntimeSurfaceProps) {
  if (result.state === 'requires_confirmation') {
    return (
      <ConnectorRuntimeConfirmationCard
        key={result.confirmation.route_receipt}
        result={result}
        onDecision={onDecision}
        compact={compact}
        className={className}
      />
    );
  }

  if (result.state === 'blocked') {
    return <ConnectorRuntimeBlockedState compact={compact} className={className} />;
  }

  if (
    result.state === 'cancelled' ||
    result.state === 'completed' ||
    result.state === 'failed' ||
    result.state === 'unknown'
  ) {
    return (
      <ConnectorRuntimeTerminalState state={result.state} compact={compact} className={className} />
    );
  }

  return null;
}
