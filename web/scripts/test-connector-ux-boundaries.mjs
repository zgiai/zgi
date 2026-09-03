import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const source = path => readFileSync('src/' + path, 'utf8');
const chat = source('components/chat/variants/aichat/aichat-chat.tsx');
assert.doesNotMatch(chat, /ConnectorDefaultsNotice|connectorDraft\.block|initialConnectorDefaultsRevision:/);
for (const file of [
  'components/chat/variants/aichat/connector-draft-dialog.tsx',
  'components/chat/variants/aichat/connected-apps-dialog.tsx',
  'components/connectors/picker/connector-picker-dialog.tsx',
]) {
  assert.doesNotMatch(source(file), /ConnectorFirstUseDialog|useSetChatConnectorPreference|switchDefaultChoice/);
}
assert.doesNotMatch(source('components/workflow/ui/workflow-run-nodes-list/runtime-structured-view.tsx'), /parseConnectorRuntime|ConnectorRuntime/);
assert.doesNotMatch(source('components/connectors/runtime/connector-runtime-confirmation-card.tsx'), /ToolGovernanceDecisionCard|rememberForSession/);
assert.match(source('components/chat/variants/aichat/tool-governance-decision-card.tsx'), /rememberForSession/);
assert.doesNotMatch(source('components/common/image-safe/index.tsx'), /connectorId|web-search|feishu/);
assert.doesNotMatch(source('components/workflow/nodes/tool/manager/tool-config-dialog.tsx'), /connectors\.picker/);
assert.match(source('components/chat/variants/aichat/message-list.tsx'), /\(command, scope\)/);
console.log('Connector UX host boundaries and unchanged ordinary approval contract passed');
