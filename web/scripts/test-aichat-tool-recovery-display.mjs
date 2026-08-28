import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const helperPath = path.join(
  root,
  'src/components/chat/variants/aichat/timeline-display-safety.ts'
);

const source = readFileSync(helperPath, 'utf8');
const output = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: helperPath,
}).outputText;
const testModule = new Module(helperPath);
testModule.filename = helperPath;
testModule.paths = Module._nodeModulePaths(path.dirname(helperPath));
testModule._compile(output, helperPath);

const {
  formatTimelineDebugValue,
  recoveredInvalidArgumentTimelineItemIds,
  sanitizeTimelineDisplayPayload,
  sanitizeTimelineDisplayString,
  summarizeTimelineArgumentsForDisplay,
} = testModule.exports;

const externalDisplayPath = path.join(
  root,
  'src/components/chat/variants/aichat/external-app-display.ts'
);
const externalDisplayOutput = ts.transpileModule(readFileSync(externalDisplayPath, 'utf8'), {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: externalDisplayPath,
}).outputText;
const externalDisplayModule = new Module(externalDisplayPath);
externalDisplayModule.filename = externalDisplayPath;
externalDisplayModule.paths = Module._nodeModulePaths(path.dirname(externalDisplayPath));
externalDisplayModule._compile(externalDisplayOutput, externalDisplayPath);
const {
  getAIChatExternalArgumentDisplayEntries,
  getAIChatExternalActionDisplayName,
  getAIChatExternalAppDescription,
  getAIChatExternalAppDisplayName,
  getAIChatExternalInvocationDisplayName,
  getAIChatLocalizedExternalText,
} = externalDisplayModule.exports;

const translations = {
  'consoleChat.connectedApps.unknownExternalApp': '外部应用',
  'consoleChat.connectedApps.providers.github.name': 'GitHub',
  'consoleChat.connectedApps.providers.github.description': 'GitHub 说明',
  'consoleChat.connectedApps.providers.genericDescription': '通用外部应用说明',
  'consoleChat.connectedApps.actions.githubRepositoryList': '列出 GitHub 仓库',
  'consoleChat.connectedApps.actions.generic': '外部操作',
};
const translate = (key, values) =>
  key === 'consoleChat.governance.externalToolLabel'
    ? `${values.integration} · ${values.action}`
    : (translations[key] ?? key);

assert.equal(
  getAIChatLocalizedExternalText({ 'en-US': 'Send message', 'zh-Hans': '发送消息' }, 'zh-CN'),
  '发送消息'
);
assert.equal(
  getAIChatExternalAppDisplayName('feishu', 'Feishu', translate, {
    locale: 'zh-Hans',
    nameI18n: { 'en-US': 'Feishu', 'zh-Hans': '飞书' },
  }),
  '飞书'
);
assert.equal(
  getAIChatExternalAppDescription('feishu', 'Send messages through Feishu.', 'zh-Hans', translate, {
    descriptionI18n: { 'en-US': 'Send messages through Feishu.', 'zh-Hans': '通过飞书发送消息。' },
  }),
  '通过飞书发送消息。'
);
assert.equal(
  getAIChatExternalActionDisplayName('feishu.message.send', translate, {
    locale: 'zh-Hans',
    fallbackName: 'Send Feishu message',
    nameI18n: { 'en-US': 'Send Feishu message', 'zh-Hans': '发送飞书消息' },
  }),
  '发送飞书消息'
);
assert.equal(
  getAIChatExternalActionDisplayName('future.action', translate, {
    locale: 'zh-Hans',
    fallbackName: 'Future action',
  }),
  '外部操作'
);
assert.deepEqual(
  getAIChatExternalArgumentDisplayEntries(
    { owner: 'zgiai', unknown_field: 'kept without its technical key' },
    {
      locale: 'en-US',
      argumentLabelsI18n: { owner: { 'en-US': 'Repository owner' } },
    }
  ),
  [
    { key: 'owner', label: 'Repository owner', value: 'zgiai' },
    {
      key: 'unknown_field',
      label: null,
      value: 'kept without its technical key',
    },
  ]
);
assert.deepEqual(
  getAIChatExternalArgumentDisplayEntries(
    { state: 'open', filters: { visibility: 'private' }, include_archived: false },
    {
      locale: 'zh-Hans',
      argumentLabelsI18n: {
        state: { 'en-US': 'State', 'zh-Hans': '状态' },
        'filters.visibility': { 'en-US': 'Visibility', 'zh-Hans': '可见性' },
        include_archived: { 'en-US': 'Archive filter', 'zh-Hans': '归档范围' },
      },
      argumentValueLabelsI18n: {
        state: { 'zh-Hans': { open: '未关闭' } },
        'filters.visibility': { 'zh-Hans': { private: '私有' } },
        include_archived: { 'zh-Hans': { false: '仅未归档' } },
      },
    }
  ),
  [
    { key: 'state', label: '状态', value: '未关闭' },
    { key: 'filters.visibility', label: '可见性', value: '私有' },
    { key: 'include_archived', label: '归档范围', value: '仅未归档' },
  ]
);
assert.equal(
  getAIChatExternalInvocationDisplayName(
    {
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      arguments: {
        integration_id: 'github',
        action_id: 'github.repository.list',
      },
    },
    'zh-Hans',
    translate,
    '执行外部应用操作'
  ),
  'GitHub · 列出 GitHub 仓库'
);

const uuid = '123e4567-e89b-12d3-a456-426614174000';
const compactUuid = '123e4567e89b12d3a456426614174000';
const safePayload = sanitizeTimelineDisplayPayload({
  connection_id: uuid,
  workspace_id: 'platform-workspace',
  issue_id: '142',
  repository_id: 'zgi',
  chat_id: 'oc_public_chat',
  message_id: 'om_public_message',
  resource_id: 'github-resource-17',
  file_id: 'external-file-9',
  nested: {
    requestUuid: uuid,
    compact_reference: compactUuid,
    connection_name: 'Work GitHub',
    note: `completed ${uuid}`,
  },
});
assert.deepEqual(safePayload, {
  issue_id: '142',
  repository_id: 'zgi',
  chat_id: 'oc_public_chat',
  message_id: 'om_public_message',
  resource_id: 'github-resource-17',
  file_id: 'external-file-9',
  nested: {
    requestUuid: '[hidden]',
    compact_reference: '[hidden]',
    connection_name: 'Work GitHub',
    note: 'completed [hidden]',
  },
});
assert.equal(
  sanitizeTimelineDisplayString(`connection_id ${uuid} is invalid`),
  'connection_id [hidden] is invalid'
);
assert.equal(
  sanitizeTimelineDisplayString('skill tool external-apps/execute_action has invalid arguments'),
  'skill tool external-apps/execute_action has invalid arguments'
);
assert.equal(sanitizeTimelineDisplayString(uuid, '已隐藏'), '已隐藏');
assert.equal(sanitizeTimelineDisplayString('__zgi_hidden_reference__', '已隐藏'), '已隐藏');
assert.equal(
  sanitizeTimelineDisplayString('connection __zgi_hidden_reference__ is unavailable', '已隐藏'),
  'connection 已隐藏 is unavailable'
);
assert.deepEqual(sanitizeTimelineDisplayPayload({ request_id: uuid }, '已隐藏'), {
  request_id: '已隐藏',
});
assert.equal(
  sanitizeTimelineDisplayString('__zgi_redacted__', 'Redacted', 'Content truncated'),
  'Redacted'
);
assert.equal(
  sanitizeTimelineDisplayString('[REDACTED]', 'Redacted', 'Content truncated'),
  'Redacted'
);
assert.equal(
  sanitizeTimelineDisplayString('__zgi_truncated__', 'Redacted', 'Content truncated'),
  'Content truncated'
);
assert.equal(
  sanitizeTimelineDisplayString('[TRUNCATED]', 'Redacted', 'Content truncated'),
  'Content truncated'
);
assert.equal(
  formatTimelineDebugValue(
    { public: '__zgi_truncated__', private: '__zgi_redacted__' },
    'Redacted',
    'Content truncated'
  ),
  JSON.stringify({ public: 'Content truncated', private: 'Redacted' })
);
assert.doesNotMatch(JSON.stringify(safePayload), /123e4567|connection_id|workspace_id/i);
assert.match(JSON.stringify(safePayload), /issue_id|repository_id|chat_id|message_id|resource_id/);
assert.match(JSON.stringify(safePayload), /requestUuid|\[hidden\]/);

const argumentSummary = summarizeTimelineArgumentsForDisplay({
  connection_id: uuid,
  arguments: {
    query: 'private repository query',
    limit: 5,
    resource_id: uuid,
    issue_id: '142',
  },
});
assert.deepEqual(argumentSummary, {
  arguments: {
    query: { type: 'string', length: 24 },
    limit: { type: 'number' },
    resource_id: { type: 'string', length: 8 },
    issue_id: { type: 'string', length: 3 },
  },
});
assert.doesNotMatch(JSON.stringify(argumentSummary), /private repository|123e4567/i);
assert.match(JSON.stringify(argumentSummary), /resource_id|issue_id/);
assert.deepEqual(
  summarizeTimelineArgumentsForDisplay({
    action_id: uuid,
    arguments: { type: 'object', keys: 0 },
  }),
  {
    action_id: { type: 'string', length: 8 },
    arguments: { type: 'object', keys: 0 },
  }
);

const missingIdentityNotRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'first-attempt',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      status: 'error',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      message: 'skill tool external-apps/execute_action has invalid arguments',
    },
  },
  {
    id: 'unrelated-success',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'search_actions',
    },
  },
  {
    id: 'corrected-attempt',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
    },
  },
]);
assert.equal(missingIdentityNotRecovered.size, 0);

const notRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'failed-only',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      status: 'error',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      error: 'invalid_arguments',
    },
  },
]);
assert.equal(notRecovered.size, 0);

const sameExternalActionRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'same-action-failed',
    type: 'skill_event',
    invocation: {
      status: 'error',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      integration_id: 'github',
      action_id: 'github.repository.list',
      error: 'invalid arguments',
    },
  },
  {
    id: 'same-action-succeeded',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      result: {
        integration_id: 'github',
        action_id: 'github.repository.list',
      },
    },
  },
]);
assert.deepEqual([...sameExternalActionRecovered], ['same-action-failed']);

const differentExternalActionNotRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'different-action-failed',
    type: 'skill_event',
    invocation: {
      status: 'error',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      arguments: {
        integration_id: 'github',
        action_id: 'github.issue.list',
      },
      error: 'invalid arguments',
    },
  },
  {
    id: 'different-action-succeeded',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      integration_id: 'github',
      action_id: 'github.repository.list',
    },
  },
]);
assert.equal(differentExternalActionNotRecovered.size, 0);

const identifiedFailureUnknownSuccessNotRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'identified-failure',
    type: 'skill_event',
    invocation: {
      status: 'error',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      integration_id: 'github',
      action_id: 'github.repository.list',
      error: 'invalid arguments',
    },
  },
  {
    id: 'identity-free-success',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
    },
  },
]);
assert.equal(identifiedFailureUnknownSuccessNotRecovered.size, 0);

const identifiedMismatchWinsOverUnknownSuccess = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'identified-failed',
    type: 'skill_event',
    invocation: {
      status: 'error',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      integration_id: 'github',
      action_id: 'github.issue.list',
      error: 'invalid arguments',
    },
  },
  {
    id: 'unknown-success',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
    },
  },
  {
    id: 'identified-other-success',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'external-apps',
      tool_name: 'execute_action',
      integration_id: 'github',
      action_id: 'github.repository.list',
    },
  },
]);
assert.equal(identifiedMismatchWinsOverUnknownSuccess.size, 0);

const ordinaryToolRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'ordinary-failed',
    type: 'skill_event',
    invocation: {
      status: 'error',
      skill_id: 'calculator',
      tool_name: 'calculate',
      error: 'argument validation failed',
    },
  },
  {
    id: 'ordinary-succeeded',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'calculator',
      tool_name: 'calculate',
    },
  },
]);
assert.deepEqual([...ordinaryToolRecovered], ['ordinary-failed']);

const timelineSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/agentic-timeline.tsx'),
  'utf8'
);
const resultSummarySource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/skill-result-summary.tsx'),
  'utf8'
);
const skillTraceSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/skill-trace-panel.tsx'),
  'utf8'
);
const sharedIntegrationErrorSource = readFileSync(
  path.join(root, 'src/services/integration-error-i18n.ts'),
  'utf8'
);
const timelineI18nSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/timeline-display-i18n.ts'),
  'utf8'
);
const externalAppDisplaySource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/external-app-display.ts'),
  'utf8'
);
const connectedAppsSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/connected-apps-dialog.tsx'),
  'utf8'
);
const aichatTypesSource = readFileSync(path.join(root, 'src/services/types/aichat.ts'), 'utf8');
const skillReducerSource = readFileSync(
  path.join(root, 'src/components/chat/controllers/aichat/reducers/skill.ts'),
  'utf8'
);
const enMessages = readFileSync(path.join(root, 'src/i18n/modules/webapp/en-US.ts'), 'utf8');
const zhMessages = readFileSync(path.join(root, 'src/i18n/modules/webapp/zh-Hans.ts'), 'utf8');
assert.match(timelineSource, /recoveredInvalidArgumentTimelineItemIds/);
assert.match(timelineSource, /formatAIChatTimelineArgumentSummary\(argumentSummary, t\)/);
assert.doesNotMatch(timelineSource, /compactConnectionId/);
assert.match(
  resultSummarySource,
  /sanitizeTimelineDisplayPayload\([\s\S]*result,[\s\S]*values\.hidden/
);
assert.match(timelineI18nSource, /formatAIChatTimelineArgumentSummary/);
assert.match(timelineI18nSource, /integrationErrorTranslationKey/);
assert.match(timelineI18nSource, /localizeAIChatRuntimeErrorCode/);
assert.match(timelineI18nSource, /errors\.externalAppFailed/);
assert.match(timelineI18nSource, /looksLikeIntegrationErrorCode/);
assert.doesNotMatch(timelineI18nSource, /const INTEGRATION_ERROR_KEYS/);
assert.match(sharedIntegrationErrorSource, /integration_sensitive_input_blocked/);
assert.match(externalAppDisplaySource, /github\.repository\.list/);
assert.match(externalAppDisplaySource, /getAIChatLocalizedExternalText/);
assert.match(connectedAppsSource, /nameI18n: provider\?\.name_i18n/);
assert.match(connectedAppsSource, /descriptionI18n: provider\?\.description_i18n/);
assert.match(resultSummarySource, /integration_name_i18n/);
assert.match(resultSummarySource, /action_name_i18n/);
assert.match(resultSummarySource, /values\.returned/);
assert.doesNotMatch(resultSummarySource, /formatJSON\(result\.result\)/);
assert.match(resultSummarySource, /normalizedSkillId === 'external-apps'/);
assert.doesNotMatch(resultSummarySource, /normalizedSkillId === 'web-search'/);
assert.match(resultSummarySource, /case 'list_connections'/);
assert.match(resultSummarySource, /case 'search_actions'/);
assert.match(resultSummarySource, /case 'get_action_guide'/);
assert.match(timelineSource, /frozenArguments\?\.integration_name_i18n/);
assert.match(timelineSource, /frozenArguments\?\.action_name_i18n/);
assert.match(timelineSource, /frozenArguments\?\.argument_labels_i18n/);
assert.match(timelineSource, /frozenArguments\?\.argument_value_labels_i18n/);
assert.match(timelineSource, /getAIChatExternalArgumentDisplayEntries/);
assert.match(timelineSource, /approvalPanel\.otherArgument/);
assert.doesNotMatch(timelineSource, /JSON\.stringify\(safeArguments/);
assert.match(timelineSource, /isExternalApp \? null : invocation\.skill_id/);
assert.match(timelineSource, /isExternalApp \? null : invocation\.path/);
assert.match(skillTraceSource, /isExternalApp \? null : invocation\.skill_id/);
assert.match(skillTraceSource, /isExternalApp \? null : invocation\.path/);
assert.match(skillTraceSource, /getAIChatExternalInvocationDisplayName/);
assert.doesNotMatch(timelineSource, /return '<1ms'/);
assert.doesNotMatch(skillTraceSource, /return '<1ms'/);
assert.match(timelineSource, /values\.lessThanOneMillisecond/);
assert.match(skillTraceSource, /values\.lessThanOneMillisecond/);
assert.match(externalAppDisplaySource, /localizedArgumentLabel/);
assert.match(externalAppDisplaySource, /localizedArgumentValue/);
assert.match(externalAppDisplaySource, /fieldName !== 'argument_labels_i18n'/);
assert.match(aichatTypesSource, /error_code\?: string/);
assert.match(skillReducerSource, /error_code: payload\.error_code/);
assert.match(enMessages, /arguments: 'Argument summary'/);
assert.match(enMessages, /invalidArgumentsCorrected:/);
assert.match(enMessages, /githubRepositoryList: 'List GitHub repositories'/);
assert.match(enMessages, /invalidArguments: 'The tool arguments were invalid/);
assert.match(enMessages, /returned: 'Returned'/);
assert.match(enMessages, /lessThanOneMillisecond: 'Less than 1 ms'/);
assert.match(enMessages, /otherArgument: 'Other parameter'/);
assert.match(enMessages, /externalAppFailed: 'The external app request failed/);
assert.match(zhMessages, /arguments: '参数摘要'/);
assert.match(zhMessages, /invalidArgumentsCorrected:/);
assert.match(zhMessages, /returned: '已返回'/);
assert.match(zhMessages, /lessThanOneMillisecond: '少于 1 毫秒'/);
assert.match(zhMessages, /otherArgument: '其他参数'/);
assert.match(zhMessages, /externalAppFailed: '外部应用请求失败/);
assert.match(zhMessages, /githubRepositoryList: '列出 GitHub 仓库'/);
assert.match(zhMessages, /invalidArguments: '工具参数不符合要求/);

console.log('AIChat tool recovery and timeline display safety checks passed.');
