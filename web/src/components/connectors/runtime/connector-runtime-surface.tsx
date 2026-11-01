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
import { isConnectorRuntimeDecisionRejected } from './connector-runtime-transport';
import type {
  ConnectorRuntimeConfirmation,
  ConnectorRuntimeDecisionHandler,
  ConnectorRuntimeExecutionResult,
  ConnectorRuntimeRequiresConfirmationResult,
} from './connector-runtime-model';
import { ConnectorRuntimeConfirmationCard } from './connector-runtime-confirmation-card';
import { ConnectorRuntimePreviewSummary } from './connector-runtime-preview-summary';
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
  confirmation,
  locale,
  preview,
  verified,
  allowsSessionGrant,
  conversationId,
  messageId,
  scope,
  onDecision,
  onRejected,
  t,
}: ConnectorRuntimeDialogApprovalBridgeProps & {
  confirmation: ConnectorRuntimeConfirmation;
  locale: string;
  preview: ReturnType<typeof useConnectorRuntimeReceipt>['preview'];
  verified: boolean;
  allowsSessionGrant: boolean;
  scope: string;
  onRejected: () => void;
  t: ReturnType<typeof useT<'connectors'>>;
}): ToolGovernancePendingApproval {
  return {
    id: `${conversationId}:${messageId}:${result.confirmation.route_receipt}`,
    conversationId,
    messageId,
    title: `${confirmation.connector_name} · ${confirmation.action_name}`,
    toolLabel: confirmation.action_name,
    actionSentence: t('runtime.confirmation.description'),
    assets: [
      ...(preview?.account_identity
        ? [
            {
              key: 'connector-account',
              name: t('runtime.confirmation.previewAccount'),
              meta: preview.account_identity,
            },
          ]
        : []),
      ...(preview?.fields ?? []).map((field, index) => ({
        key: `connector-preview-${index}`,
        name: field.label_i18n?.[locale] ?? field.label,
        meta: field.value,
      })),
    ],
    riskLabel:
      result.confirmation.risk_classification === 'high'
        ? t('runtime.risk.high')
        : t('runtime.risk.unclassified'),
    permissionLabel: null,
    canSubmit: verified && Boolean(preview?.complete),
    isHighImpact: result.confirmation.risk_classification === 'high',
    allowRememberForSession: allowsSessionGrant,
    onSubmitDecision: async (action, rememberForSession) => {
      if (action === 'approve' && !preview?.complete) {
        throw new Error(t('runtime.confirmation.previewUnavailable'));
      }
      const decision = action === 'approve' ? 'confirmed' : 'cancelled';
      const remember = action === 'approve' && allowsSessionGrant && rememberForSession;
      try {
        await onDecision(
          {
            route_receipt: result.confirmation.route_receipt,
            decision,
            ...(remember ? { remember_for_session: true } : {}),
          },
          scope,
          remember
        );
      } catch (error) {
        // Refused before anything ran: the receipt stays open for a one-shot
        // decision, and a fresh status read hides the session option.
        if (isConnectorRuntimeDecisionRejected(error)) {
          onRejected();
          throw new Error(t('runtime.confirmation.sessionGrantUnsupported'));
        }
        throw error;
      }
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
  const t = useT('connectors');
  const locale = useLocale();
  const pendingApprovalScope = useToolGovernancePendingApprovalScope();
  const receipt = useConnectorRuntimeReceipt(result.confirmation.route_receipt);
  const confirmation = localizeConnectorRuntimeConfirmation(result.confirmation, locale);
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
        confirmation,
        locale,
        preview: receipt.preview,
        verified,
        allowsSessionGrant: receipt.allowsSessionGrant,
        conversationId,
        messageId,
        scope: receipt.scope,
        onDecision,
        onRejected: () => void checkReceipt().catch(() => undefined),
        t,
      }),
    [
      checkReceipt,
      confirmation,
      conversationId,
      locale,
      messageId,
      onDecision,
      receipt.allowsSessionGrant,
      receipt.preview,
      receipt.scope,
      result,
      t,
      verified,
    ]
  );

  useEffect(() => {
    if (!verified || receipt.result || receipt.phase === 'submitting') return;
    return publishToolGovernancePendingApproval(approval, pendingApprovalScope);
  }, [approval, pendingApprovalScope, receipt.phase, receipt.result, verified]);

  // The existing Tool Governance approval panel owns the visible confirmation
  // UI. This bridge only registers the server-issued decision and supplies the
  // safe preview fields as panel assets; rendering another Connector card here
  // would recreate the duplicate Agent window.
  return null;
}

function ConnectorRuntimePendingSurface({
  result,
  onDecision,
  compact,
  className,
}: {
  result: ConnectorRuntimeRequiresConfirmationResult;
  onDecision: ConnectorRuntimeDecisionHandler;
  compact: boolean;
  className?: string;
}) {
  const receipt = useConnectorRuntimeReceipt(result.confirmation.route_receipt);
  // The summary reads the owner-scoped preview itself: the card's own read
  // fills only the card, so without this the command never showed.
  const { check: readPreview } = receipt;
  useEffect(() => {
    void readPreview().catch(() => {
      // The card reports a failed status read and offers a retry.
    });
  }, [readPreview]);
  return (
    <div className="space-y-2">
      <ConnectorRuntimePreviewSummary
        result={result}
        preview={receipt.preview}
        compact={compact}
        className={className}
      />
      <ConnectorRuntimeConfirmationCard
        key={result.confirmation.route_receipt}
        result={result}
        onDecision={onDecision}
        compact={compact}
        className={className}
      />
    </div>
  );
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
      <ConnectorRuntimePendingSurface
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
        <ConnectorRuntimeTerminalState
          state={result.state}
          presentation={result.connector_presentation}
          compact={compact}
          className={className}
        />
      );
  }

  return null;
}
