'use client';

import { useEffect, useMemo, useState } from 'react';
import { useLocale } from 'next-intl';
import type { ToolGovernancePendingApproval } from '@/components/chat/variants/aichat/tool-governance-decision-card';
import {
  publishToolGovernancePendingApproval,
  useToolGovernancePendingApprovalScope,
} from '@/components/chat/variants/aichat/tool-governance-decision-card';
import { useT } from '@/i18n/translations';
import { localizeConnectorRuntimeConfirmation } from '@/components/connectors/connector-metadata';
import { useConnectorRuntimeReceipt } from './use-runtime-receipt';
import type {
  ConnectorRuntimeDecisionHandler,
  ConnectorRuntimeExecutionResult,
  ConnectorRuntimeRequiresConfirmationResult,
} from './connector-runtime-model';
import { ConnectorRuntimeConfirmationCard } from './connector-runtime-confirmation-card';
import { ConnectorRuntimeBlockedState } from './connector-runtime-blocked-state';
import { ConnectorRuntimeTerminalState } from './connector-runtime-terminal-state';

interface ConnectorRuntimeSurfaceProps {
  result: ConnectorRuntimeExecutionResult;
  onDecision: ConnectorRuntimeDecisionHandler;
  compact?: boolean;
  className?: string;
  readOnly?: boolean;
  /** Render pending confirmation through the existing Chat approval panel. */
  dialogMode?: boolean;
  conversationId?: string;
  messageId?: string;
}

interface ConnectorRuntimeDialogApprovalBridgeProps {
  result: ConnectorRuntimeRequiresConfirmationResult;
  conversationId: string;
  messageId: string;
  onDecision: ConnectorRuntimeDecisionHandler;
}

function connectorRuntimePendingApproval({
  result,
  preview,
  verified,
  conversationId,
  messageId,
  scope,
  onDecision,
  locale,
  t,
}: ConnectorRuntimeDialogApprovalBridgeProps & {
  preview: ReturnType<typeof useConnectorRuntimeReceipt>['preview'];
  verified: boolean;
  scope: string;
  locale: string;
  t: ReturnType<typeof useT<'connectors'>>;
}): ToolGovernancePendingApproval {
  const confirmation = localizeConnectorRuntimeConfirmation(result.confirmation, locale);
  const actionSentence = `${confirmation.connector_name} · ${confirmation.action_name}`;
  const assets = [] as ToolGovernancePendingApproval['assets'];
  if (preview?.account_identity) {
    assets.push({
      key: 'account',
      name: t('runtime.confirmation.previewAccount'),
      meta: preview.account_identity,
    });
  }
  if (preview) {
    preview.fields.forEach((field, index) => {
      assets.push({
        key: `preview-${index}`,
        name: field.label_i18n[locale] ?? field.label,
        meta: field.value,
      });
    });
  } else if (confirmation.consequence.external_destination) {
    assets.push({
      key: 'destination',
      name: t('runtime.confirmation.fields.destination'),
      meta: confirmation.consequence.external_destination,
    });
  }

  return {
    id: `${conversationId}:${messageId}:${confirmation.route_receipt}`,
    conversationId,
    messageId,
    title: actionSentence,
    toolLabel: confirmation.connector_name,
    actionSentence,
    assets,
    riskLabel:
      confirmation.risk_classification === 'high'
        ? t('runtime.risk.high')
        : t('runtime.risk.unclassified'),
    permissionLabel: null,
    canSubmit: verified,
    isHighImpact: confirmation.risk_classification === 'high',
    onSubmitDecision: async (action, rememberForSession) => {
      if (action === 'approve' && !preview?.complete) {
        throw new Error(t('runtime.confirmation.previewUnavailable'));
      }
      const decision = action === 'approve' ? 'confirmed' : 'cancelled';
      await onDecision(
        {
          route_receipt: confirmation.route_receipt,
          decision,
          ...(action === 'approve' && rememberForSession ? { remember_for_session: true } : {}),
        },
        scope,
        action === 'approve' ? rememberForSession : false
      );
      // The Chat host records transport_unconfirmed as an explicit unknown
      // terminal so the original approval panel can close without offering a
      // duplicate send. A later status refresh may replace that projection.
    },
  };
}

/**
 * Registers a Connector receipt with the existing Tool Governance pending
 * store. It intentionally renders nothing; InputArea owns the confirmation UI.
 */
export function ConnectorRuntimeDialogApprovalBridge({
  result,
  conversationId,
  messageId,
  onDecision,
}: ConnectorRuntimeDialogApprovalBridgeProps) {
  const locale = useLocale();
  const t = useT('connectors');
  const pendingScopeId = useToolGovernancePendingApprovalScope();
  const receipt = useConnectorRuntimeReceipt(result.confirmation.route_receipt);
  const checkReceipt = receipt.check;
  const [verified, setVerified] = useState(false);
  useEffect(() => {
    let active = true;
    setVerified(false);
    void checkReceipt()
      .then(() => {
        if (active) setVerified(true);
      })
      .catch(() => {
        if (active) setVerified(false);
      });
    return () => {
      active = false;
    };
  }, [checkReceipt]);

  const approval = useMemo(
    () =>
      connectorRuntimePendingApproval({
        result,
        preview: receipt.preview,
        verified,
        conversationId,
        messageId,
        scope: receipt.scope,
        onDecision,
        locale,
        t,
      }),
    [
      conversationId,
      locale,
      messageId,
      onDecision,
      receipt.preview,
      receipt.scope,
      result,
      t,
      verified,
    ]
  );

  useEffect(() => {
    if (!verified || receipt.result || receipt.phase === 'submitting') return;
    return publishToolGovernancePendingApproval(approval, pendingScopeId);
  }, [approval, pendingScopeId, receipt.phase, receipt.result, verified]);

  return null;
}

/** Shared host surface for Chat, Agent, and Workflow runtime adapters. */
export function ConnectorRuntimeSurface({
  result,
  onDecision,
  compact = false,
  className,
  dialogMode = false,
  readOnly = false,
  conversationId,
  messageId,
}: ConnectorRuntimeSurfaceProps) {
  if (result.state === 'requires_confirmation') {
    if (!readOnly && dialogMode && conversationId && messageId) {
      return (
        <ConnectorRuntimeDialogApprovalBridge
          result={result}
          conversationId={conversationId}
          messageId={messageId}
          onDecision={onDecision}
        />
      );
    }
    if (readOnly) return null;
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
