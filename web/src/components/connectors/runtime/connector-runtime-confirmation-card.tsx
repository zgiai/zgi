'use client';

import { useEffect, useRef, useState } from 'react';
import { useLocale } from 'next-intl';
import { RefreshCw } from 'lucide-react';
import { useConnectorRuntimeReceipt } from './use-runtime-receipt';
import { OperationConfirmationCard } from '@/components/aichat/operation-cards';
import { localizeConnectorRuntimeConfirmation } from '@/components/connectors/connector-metadata';
import type {
  OperationCardTextBlock,
  OperationConfirmationStatus,
} from '@/components/aichat/operation-cards/types';
import { useT } from '@/i18n/translations';
import {
  beginConnectorRuntimeDecision,
  createConnectorRuntimeDecisionState,
  isConnectorRuntimeDecisionAvailable,
  markConnectorRuntimeDecisionUnknown,
  resolveConnectorRuntimeDecision,
  type ConnectorRuntimeConfirmationCommand,
  type ConnectorRuntimeConfirmationDecision,
  type ConnectorRuntimeDecisionResult,
  type ConnectorRuntimeDecisionState,
  type ConnectorRuntimeRequiresConfirmationResult,
} from './connector-runtime-model';

export type ConnectorRuntimeDecisionHandler = (
  command: ConnectorRuntimeConfirmationCommand,
  scope?: string
) => Promise<ConnectorRuntimeDecisionResult>;

interface ConnectorRuntimeConfirmationCardProps {
  result: ConnectorRuntimeRequiresConfirmationResult;
  onDecision: ConnectorRuntimeDecisionHandler;
  compact?: boolean;
  className?: string;
}

function effectLabel(effect: string, t: ReturnType<typeof useT<'connectors'>>): string {
  switch (effect) {
    case 'read':
      return t('runtime.effects.read');
    case 'create':
      return t('runtime.effects.create');
    case 'update':
      return t('runtime.effects.update');
    case 'delete':
      return t('runtime.effects.delete');
    case 'publish':
      return t('runtime.effects.publish');
    case 'invoke':
      return t('runtime.effects.invoke');
    case 'schedule':
      return t('runtime.effects.schedule');
    case 'external_send':
      return t('runtime.effects.externalSend');
    case 'unspecified':
      return t('runtime.effects.unspecified');
    default: {
      const readable = effect.replace(/[_-]+/g, ' ').trim();
      return readable || t('runtime.effects.unspecified');
    }
  }
}

function effectiveDecisionState(
  state: ConnectorRuntimeDecisionState,
  routeReceipt: string
): ConnectorRuntimeDecisionState {
  if (state.status === 'idle' || state.routeReceipt === routeReceipt) return state;
  return createConnectorRuntimeDecisionState();
}

export function ConnectorRuntimeConfirmationCard({
  result,
  onDecision,
  compact = false,
  className,
}: ConnectorRuntimeConfirmationCardProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const [decisionState, setDecisionState] = useState<ConnectorRuntimeDecisionState>(() =>
    createConnectorRuntimeDecisionState()
  );
  const decisionStateRef = useRef<ConnectorRuntimeDecisionState>(decisionState);
  const confirmation = localizeConnectorRuntimeConfirmation(result.confirmation, locale);
  const receipt = useConnectorRuntimeReceipt(confirmation.route_receipt);
  const { check: readStatus, scope: receiptScope } = receipt;
  const identity = JSON.stringify([receipt.scope, confirmation.route_receipt]);
  const identityRef = useRef(identity);
  identityRef.current = identity;
  const decisionScope = useRef(receipt.scope);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [checking, setChecking] = useState(false);
  const [verifiedIdentity, setVerifiedIdentity] = useState('');
  const localState =
    decisionScope.current === receipt.scope
      ? effectiveDecisionState(decisionState, confirmation.route_receipt)
      : createConnectorRuntimeDecisionState();
  const visibleState: ConnectorRuntimeDecisionState =
    receipt.phase === 'submitting'
      ? {
          status: 'submitting',
          routeReceipt: confirmation.route_receipt,
          decision: localState.status === 'idle' ? 'confirmed' : localState.decision,
        }
      : receipt.result
        ? receipt.result.state === 'transport_unconfirmed'
          ? { status: 'unknown', routeReceipt: confirmation.route_receipt, decision: 'confirmed' }
          : {
              status: 'resolved',
              routeReceipt: confirmation.route_receipt,
              decision: receipt.result.state === 'cancelled' ? 'cancelled' : 'confirmed',
              outcome: receipt.result.state,
            }
        : localState;
  useEffect(() => {
    let current = true;
    void readStatus()
      .then(() => {
        if (current) setVerifiedIdentity(identity);
      })
      .catch(() => {
        if (!current) return;
        decisionScope.current = receiptScope;
        const unconfirmed: ConnectorRuntimeDecisionState = {
          status: 'unknown',
          routeReceipt: confirmation.route_receipt,
          decision: 'confirmed',
        };
        decisionStateRef.current = unconfirmed;
        setDecisionState(unconfirmed);
      });
    return () => {
      current = false;
    };
  }, [readStatus, identity, receiptScope, confirmation.route_receipt]);
  const checkStatus = async () => {
    if (checking) return;
    setChecking(true);
    try {
      await receipt.check();
    } finally {
      if (mounted.current && identityRef.current === identity) setChecking(false);
    }
  };
  const isSubmitting = visibleState.status === 'submitting';
  const resolvedDecision = visibleState.status === 'resolved' ? visibleState.decision : null;
  const canDecide =
    verifiedIdentity === identity &&
    Boolean(receipt.preview?.complete) &&
    isConnectorRuntimeDecisionAvailable(visibleState, result);
  const riskIsHigh = confirmation.risk_classification === 'high';
  const effect = effectLabel(confirmation.consequence.external_effect, t);

  const updateDecisionState = (next: ConnectorRuntimeDecisionState) => {
    if (!mounted.current || identityRef.current !== identity) return;
    decisionScope.current = receipt.scope;
    decisionStateRef.current = next;
    setDecisionState(next);
  };

  const submitDecision = async (decision: ConnectorRuntimeConfirmationDecision) => {
    if (!canDecide) return;
    const current =
      decisionScope.current === receipt.scope
        ? effectiveDecisionState(decisionStateRef.current, confirmation.route_receipt)
        : createConnectorRuntimeDecisionState();
    const attempt = beginConnectorRuntimeDecision(current, result, decision);
    if (!attempt) return;

    updateDecisionState(attempt.state);
    try {
      const outcome = await onDecision(attempt.command, receipt.scope);
      updateDecisionState(
        outcome.state === 'transport_unconfirmed'
          ? markConnectorRuntimeDecisionUnknown(decisionStateRef.current, attempt.command)
          : resolveConnectorRuntimeDecision(decisionStateRef.current, attempt.command, outcome)
      );
    } catch {
      updateDecisionState(
        markConnectorRuntimeDecisionUnknown(decisionStateRef.current, attempt.command)
      );
    }
  };

  let status: OperationConfirmationStatus = 'pending';
  let statusLabel = t('runtime.confirmation.status.required');
  if (isSubmitting) {
    statusLabel = t('runtime.confirmation.status.submitting');
  } else if (visibleState.status === 'unknown') {
    status = 'rejected';
    statusLabel = t('runtime.confirmation.transportUnconfirmed');
  } else if (visibleState.status === 'resolved' && resolvedDecision === 'cancelled') {
    status = 'rejected';
    statusLabel = t('runtime.confirmation.status.cancelled');
  } else if (visibleState.status === 'resolved' && visibleState.outcome === 'completed') {
    status = 'confirmed';
    statusLabel = t('runtime.confirmation.status.completed');
  } else if (visibleState.status === 'resolved' && visibleState.outcome === 'failed') {
    status = 'rejected';
    statusLabel = t('runtime.confirmation.status.failed');
  }

  const warnings: OperationCardTextBlock[] = [];
  if (confirmation.risk_classification === 'unclassified') {
    warnings.push({
      id: 'unclassified',
      title: t('runtime.confirmation.unclassifiedTitle'),
      description: t('runtime.confirmation.unclassifiedDescription'),
      tone: 'warning',
    });
  }
  if (visibleState.status === 'resolved' && visibleState.outcome === 'failed') {
    warnings.push({
      id: 'operation-failed',
      description: t('runtime.confirmation.operationFailed'),
      tone: 'destructive',
    });
  }
  if (visibleState.status === 'resolved' && visibleState.outcome === 'unknown') {
    status = 'rejected';
    statusLabel = t('runtime.confirmation.status.unknown');
    warnings.push({
      id: 'unknown-outcome',
      title: t('runtime.confirmation.unknownTitle'),
      description: t('runtime.confirmation.unknownDescription'),
      tone: 'warning',
    });
  }

  if (visibleState.status === 'unknown') {
    warnings.push({
      id: 'unconfirmed',
      description: t('runtime.confirmation.transportUnconfirmedHint'),
      tone: 'warning',
    });
  }
  const resolved = visibleState.status === 'resolved' || visibleState.status === 'unknown';
  return (
    <OperationConfirmationCard
      className={className}
      compact={compact}
      eyebrow={t('runtime.confirmation.eyebrow', {
        risk: riskIsHigh ? t('runtime.risk.high') : t('runtime.risk.unclassified'),
      })}
      title={t('runtime.confirmation.title')}
      description={t('runtime.confirmation.description')}
      status={status}
      statusLabel={statusLabel}
      summary={riskIsHigh ? t('runtime.confirmation.consequenceFallback', { effect }) : undefined}
      items={[]}
      warnings={warnings}
      actions={
        visibleState.status === 'unknown'
          ? [
              {
                id: 'check-connector-status',
                label: t('runtime.confirmation.checkStatus'),
                icon: <RefreshCw className="size-3.5" />,
                disabled: checking,
                loading: checking,
                onClick: () => {
                  void checkStatus().catch(() => {});
                },
              },
            ]
          : undefined
      }
      cancelAction={
        resolved
          ? undefined
          : {
              id: 'cancel-connector-operation',
              label: t('runtime.confirmation.cancel'),
              disabled: !canDecide,
              loading: isSubmitting && visibleState.decision === 'cancelled',
              onClick: () => void submitDecision('cancelled'),
            }
      }
      confirmAction={
        resolved
          ? undefined
          : {
              id: 'confirm-connector-operation',
              label: t('runtime.confirmation.confirm'),
              variant: riskIsHigh ? 'destructive' : 'default',
              disabled: !canDecide,
              loading: isSubmitting && visibleState.decision === 'confirmed',
              onClick: () => void submitDecision('confirmed'),
            }
      }
    />
  );
}
