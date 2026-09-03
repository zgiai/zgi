import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const dialog = read('src/components/chat/variants/aichat/connected-apps-dialog.tsx');
const chat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
const toolbar = read('src/components/chat/variants/aichat/input-toolbar.tsx');
const inputArea = read('src/components/chat/variants/aichat/input-area.tsx');
const connectorService = read('src/services/connector.service.ts');
const connectorTypes = read('src/services/types/connector.ts');
const connectorHooks = read('src/hooks/connectors/use-connectors.ts');
const queryKeys = read('src/hooks/query-keys.ts');

assert.match(dialog, /ConnectorPickerDialog/);
assert.match(dialog, /groupByConnector/);
assert.doesNotMatch(dialog, /ConnectorPickerMenu|PopoverAnchor|role="menu"/);
assert.match(dialog, /anchorElement=\{anchorElement\}/);
assert.doesNotMatch(dialog, /onRequestManageConnector/, 'consumer picker cannot manage other targets');
const stateFocusHandler = dialog.match(
  /onCloseAutoFocus=\{event => \{\s*(event\.preventDefault\(\);\s*if \(!stateDialogOpenRef\.current && anchorElement\?\.isConnected\) \{\s*anchorElement\.focus\(\{ preventScroll: true \}\);\s*\})\s*\}\}/
);
assert.ok(stateFocusHandler, 'loading/error dialogs only restore focus on final dismissal');
let stateFocusCount = 0;
const restoreStateFocus = new Function(
  'event',
  'stateDialogOpenRef',
  'anchorElement',
  stateFocusHandler[1]
);
const stateAnchor = { isConnected: true, focus: () => stateFocusCount++ };
restoreStateFocus({ preventDefault() {} }, { current: true }, stateAnchor);
assert.equal(stateFocusCount, 0, 'loading completion lets the account menu own focus');
restoreStateFocus({ preventDefault() {} }, { current: false }, stateAnchor);
assert.equal(stateFocusCount, 1, 'keyboard dismissal returns to the host button');
assert.match(dialog, /h-\[92px\] rounded-xl/);
assert.match(chat, /anchorElement=\{connectedAppsAnchorElement\}/);
assert.match(chat, /connectedAppsOpen=\{connectedAppsOpen\}/);
assert.match(inputArea, /connectedAppsOpen=\{connectedAppsOpen\}/);
assert.match(toolbar, /aria-haspopup=\{connectedAppsPopup\}/);
assert.match(chat, /connectedAppsPopup="dialog"/);
assert.match(toolbar, /aria-expanded=\{connectedAppsOpen\}/);
const triggerHandler = toolbar.match(
  /onClick=\{event => (onOpenConnectedApps\(event\.currentTarget\))\}/
);
assert.ok(triggerHandler, 'the toolbar must pass the actual clicked button, not guess focus');
const openHandler = chat.match(/onOpenConnectedApps=\{anchorElement => \{([\s\S]*?)\n\s+\}\}/);
assert.ok(openHandler, 'Chat must retain the real trigger for positioning and focus return');
const triggerElement = { isConnected: true };
const triggerEvents = [];
new Function('event', 'onOpenConnectedApps', triggerHandler[1])(
  { currentTarget: triggerElement },
  anchor => triggerEvents.push(anchor)
);
assert.deepEqual(triggerEvents, [triggerElement]);
let opened = false;
let retainedAnchor;
const togglePicker = new Function(
  'anchorElement',
  'setConnectedAppsAnchorElement',
  'setConnectedAppsOpen',
  openHandler[1]
);
const toggle = () =>
  togglePicker(
    triggerElement,
    value => {
      retainedAnchor = value;
    },
    value => {
      opened = typeof value === 'function' ? value(opened) : value;
    }
  );
toggle();
assert.equal(retainedAnchor, triggerElement);
assert.equal(opened, true);
toggle();
assert.equal(opened, false, 'clicking the same trigger closes the menu');
assert.match(dialog, /ConnectorKeySetupPanel/);
assert.match(dialog, /pickerOpenRef\.current = open/);
const setupFocusHandler = dialog.match(
  /onCloseAutoFocus=\{event => \{\s*(event\.preventDefault\(\);\s*if \(!pickerOpenRef\.current && anchorElement\?\.isConnected\) anchorElement\.focus\(\);)\s*\}\}/
);
assert.ok(setupFocusHandler, 'Key setup must restore its real trigger only on final close');
const restoreSetupFocus = new Function(
  'event',
  'pickerOpenRef',
  'anchorElement',
  setupFocusHandler[1]
);
let focusCount = 0;
const setupAnchor = { isConnected: true, focus: () => focusCount++ };
const closeEvent = { preventDefault: () => {} };
restoreSetupFocus(closeEvent, { current: true }, setupAnchor);
assert.equal(focusCount, 0, 'returning to the account menu must not steal its focus');
restoreSetupFocus(closeEvent, { current: false }, setupAnchor);
assert.equal(focusCount, 1, 'finally closing Key setup restores the trigger');
setupAnchor.isConnected = false;
restoreSetupFocus(closeEvent, { current: false }, setupAnchor);
assert.equal(focusCount, 1, 'a removed trigger must not receive focus');
assert.match(dialog, /\{\s*type:\s*'chat',\s*id:\s*conversationId\s*\}/);
assert.match(dialog, /createTargetBinding/);
assert.match(dialog, /binding\.connector_id\s*===\s*connection\.connector_id/);
assert.match(dialog, /expected_revision:\s*replacedBinding\.revision/);
assert.match(dialog, /onRequestSwitch/);
assert.match(dialog, /request\.replacedBinding/);
assert.match(dialog, /onReady/);
assert.match(dialog, /connectionReadyNotAdded/);
assert.match(dialog, /JSON\.stringify\(\[open, conversationId, preferencesQuery\.scope\]\)/);
assert.match(dialog, /\[sessionScope\]/);
assert.match(dialog, /sessionRef\.current !== session \|\| !pickerOpenRef\.current/);
assert.doesNotMatch(
  dialog,
  /integrationService|integrationPreferences|selected_connection_ids|preferred_connection_id/
);
// Local org/account session scope is isolation, not an editable authorization policy.
assert.doesNotMatch(dialog, /Checkbox|allowed_action_ids|\bgrant\b|\bpolicy\b/);
assert.doesNotMatch(dialog, /name=['"](?:scope|permission)['"]|target_scope\s*:/);
assert.match(dialog, /result\.attachment/);
assert.doesNotMatch(
  dialog,
  /void attachConnection\(connection, setup\.target\)/,
  'a newly saved connection must not silently replace an occupied or concurrently claimed slot'
);
assert.match(dialog, /target=\{setup\.replacedBinding \? undefined : setup\.target\}/);

assert.doesNotMatch(chat, /enable_external_integrations/);
assert.match(chat, /const connectorsEnabled = enableAIChatSkillPreference;/);
assert.match(
  chat,
  /const enableAIChatSkillPreference =\s*surface === 'aichat' && \(!isEmbedded \|\| runtimeSurface === 'contextual_sidebar'\)/,
  'always-on Connectors must retain the independent AI Chat surface capability boundary'
);
assert.match(chat, /systemFeatures\.data\?\.enable_connectors/);
assert.match(chat, /conversationId=\{connectorConversationId\}/);
assert.match(chat, /connectorsEnabled=\{connectorsEnabled\}/);
assert.match(
  chat,
  /connectedAppsDisabled=\{connectorsEnabled && !connectorConversationId && isSending\}/
);
assert.match(chat, /draft=\{!connectorConversationId \? connectorDraft\.draft : undefined\}/);
assert.match(dialog, /AIChatConnectorDraftDialog/);
assert.match(toolbar, /connectedAppsDisabled\?: boolean/);
assert.match(toolbar, /disabled=\{connectedAppsDisabled\}/);
assert.match(inputArea, /connectedAppsDisabled/);

assert.match(connectorService, /getTargetBindings\(/);
assert.match(connectorService, /createTargetBinding\(/);
assert.match(connectorService, /['"]\/connector-target-bindings['"]/);
assert.match(connectorService, /Idempotency-Key/);
assert.match(connectorTypes, /ConnectorTargetBindingListResponse/);
assert.match(connectorTypes, /CreateConnectorTargetBindingRequest/);
assert.match(connectorHooks, /useConnectorTargetBindings/);
assert.match(queryKeys, /targetBindings:/);

console.log('AI Chat Connector picker and explicit conversation binding checks passed.');
