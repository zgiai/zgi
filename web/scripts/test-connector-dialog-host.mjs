import assert from 'node:assert/strict';
import { harness, load, flush } from './connector-fixture.mjs';

const pending = {
  state: 'requires_confirmation',
  confirmation: {
    route_receipt: 'receipt-dialog-1',
    connector_id: 'qqmail',
    connector_name: 'QQ Mail',
    operation_id: 'qqmail.message.send',
    action_name: 'Send mail',
    target: { type: 'chat', id: 'conversation-1' },
    consequence: {
      external_effect: 'external_send',
      data_egress: true,
      external_destination: 'qqmail',
    },
    risk_classification: 'high',
  },
};
const preview = {
  account_name: 'Mailbox',
  account_identity: 'sender@qq.com',
  kind: 'mail',
  complete: true,
  fields: [
    {
      label: 'Recipients',
      label_i18n: { 'zh-Hans': '收件人' },
      value: 'recipient@163.com',
      truncated: false,
    },
    { label: 'Subject', label_i18n: {}, value: 'Hello', truncated: false },
  ],
};

let checked = 0;
const decisions = [];
const published = [];
const receipt = {
  scope: 'org-owner',
  preview,
  phase: undefined,
  result: undefined,
  check: async () => {
    checked++;
  },
};

const bridge = harness(actualReact => {
  const surface = load('src/components/connectors/runtime/connector-runtime-surface.tsx', {
    react: actualReact,
    'react/jsx-runtime': {
      jsx: (type, props) => ({ type, props: props ?? {} }),
      Fragment: 'Fragment',
    },
    'next-intl': { useLocale: () => 'zh-Hans' },
    '@/components/chat/variants/aichat/tool-governance-decision-card': {
      publishToolGovernancePendingApproval: (approval, scope) => {
        published.push({ approval, scope });
        return () => undefined;
      },
      useToolGovernancePendingApprovalScope: () => 'host-scope',
    },
    '@/i18n/translations': { useT: () => key => key },
    '@/components/connectors/connector-metadata': {
      localizeConnectorRuntimeConfirmation: confirmation => confirmation,
    },
    './use-runtime-receipt': { useConnectorRuntimeReceipt: () => receipt },
    './connector-runtime-confirmation-card': { ConnectorRuntimeConfirmationCard: 'LegacyCard' },
    './connector-runtime-blocked-state': { ConnectorRuntimeBlockedState: 'BlockedState' },
    './connector-runtime-terminal-state': { ConnectorRuntimeTerminalState: 'TerminalState' },
  });
  return () =>
    surface.ConnectorRuntimeDialogApprovalBridge({
      result: pending,
      conversationId: 'conversation-1',
      messageId: 'message-1',
      onDecision: async (...args) => {
        decisions.push(args);
        return { route_receipt: 'receipt-dialog-1', state: 'completed' };
      },
    });
});

bridge.render();
await flush();
bridge.render();
assert.equal(checked, 1);
assert.equal(published.length, 1);
assert.equal(published[0].scope, 'host-scope');
assert.equal(published[0].approval.canSubmit, true);
assert.equal(published[0].approval.title, 'QQ Mail · Send mail');
assert.deepEqual(
  published[0].approval.assets.map(asset => asset.meta),
  ['sender@qq.com', 'recipient@163.com', 'Hello']
);
await published[0].approval.onSubmitDecision('approve', true);
assert.deepEqual(decisions[0][0], {
  route_receipt: 'receipt-dialog-1',
  decision: 'confirmed',
  remember_for_session: true,
});
await published[0].approval.onSubmitDecision('reject', true);
assert.deepEqual(decisions[1][0], {
  route_receipt: 'receipt-dialog-1',
  decision: 'cancelled',
});
bridge.unmount();
console.log(
  'Connector pending approvals register with the original dialog host and map actions correctly'
);
