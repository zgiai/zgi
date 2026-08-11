import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const webRoot = path.resolve(scriptDirectory, '..');
const read = relativePath => readFileSync(path.join(webRoot, relativePath), 'utf8');

const logPage = read('src/app/console/agents/[agentId]/logs/page.tsx');
const workflowTypes = read('src/services/types/workflow.ts');
const agentZh = read('src/i18n/modules/agents/zh-Hans.ts');
const agentEn = read('src/i18n/modules/agents/en-US.ts');
const webappZh = read('src/i18n/modules/webapp/zh-Hans.ts');
const webappEn = read('src/i18n/modules/webapp/en-US.ts');
const permissions = read('src/constants/permissions.ts');
const runDropdown = read('src/components/workflow/ui/workflow-runs-dropdown/index.tsx');
const workflowDrawer = read(
  'src/app/console/agents/[agentId]/logs/_components/log-detail-drawer.tsx'
);
const conversationContext = read(
  'src/app/console/agents/[agentId]/logs/_components/conversation-context.tsx'
);
const conversationDialog = read(
  'src/app/console/agents/[agentId]/logs/_components/conversation-log-dialog.tsx'
);
const historyContent = read(
  'src/components/workflow/ui/workflow-run-panel/components/history-content.tsx'
);
const agentDrawer = read(
  'src/app/console/agents/[agentId]/logs/_components/agent-runtime-log-detail-drawer.tsx'
);
const runStatusBadge = read('src/components/workflow/ui/run-status-badge.tsx');

assert.match(
  logPage,
  /type WorkflowRuntimeLogSource = 'web-app' \| 'external-api' \| 'workflow' \| 'automation'/,
  'Workflow runtime logs must expose only the four supported non-debug sources.'
);
assert.match(
  logPage,
  /query: \{ triggered_from: workflowLogSource \}/,
  'Workflow runtime log requests must use the selected source.'
);
assert.doesNotMatch(
  logPage.match(/const workflowSourceOptions[\s\S]*?\);/)?.[0] ?? '',
  /debugging/,
  'The standalone Workflow runtime log page must not expose debugging runs.'
);
for (const source of ['web-app', 'external-api', 'workflow', 'automation']) {
  assert.match(
    logPage.match(/const workflowSourceOptions[\s\S]*?\);/)?.[0] ?? '',
    new RegExp(`value: '${source}'`),
    `Workflow source option ${source} is missing.`
  );
}

assert.match(
  workflowTypes,
  /export interface WorkflowRunItem \{[\s\S]*?triggered_from\?: string/,
  'Workflow run list items must expose triggered_from.'
);
assert.match(
  workflowTypes,
  /export interface WorkflowRunItem \{[\s\S]*?query\?: string;[\s\S]*?answer_preview\?: string/,
  'Conversation Workflow run items must expose query and answer summaries.'
);
assert.match(
  logPage,
  /const showsConversationSummary = isAgentRuntime \|\| isConversationWorkflow/,
  'Agent and Conversation Workflow lists must share the input/output summary columns.'
);
assert.doesNotMatch(
  logPage,
  /isAgentRuntimeRunItem|'query' in item/,
  'Workflow rows must not be inferred from fields shared with Agent runtime rows.'
);
assert.match(
  logPage,
  /agentRuntimeItem\?\.source \?\? workflowItem\?\.triggered_from/,
  'Each runtime domain must read its own persisted source field.'
);
assert.match(
  logPage,
  /\['app-run', t\('appLogs\.filters\.sources\.webapp'\)\]/,
  'Legacy app-run Workflow records must be labeled as app calls.'
);
assert.doesNotMatch(
  conversationContext,
  /<Select|sameConversation|onInspect/,
  'Current conversation details must not embed a second turn selector.'
);
assert.doesNotMatch(
  conversationContext,
  /overflow-y-auto/,
  'The current-conversation summary must not introduce a nested vertical scrollbar.'
);
assert.match(
  conversationContext,
  /whitespace-pre-wrap[\s\S]*?<MarkdownViewer content=\{activeMessage\.answer\}/,
  'The dedicated conversation tab must preserve the input and render the complete Markdown answer.'
);
assert.doesNotMatch(
  conversationContext,
  /line-clamp|max-h-48 overflow-hidden/,
  'The dedicated conversation tab must not truncate the current turn.'
);
assert.doesNotMatch(
  workflowDrawer,
  /max-h-\[320px\][^\n]*overflow-y-auto/,
  'The conversation summary container must not scroll independently from the runtime detail.'
);
assert.match(
  workflowDrawer,
  /appLogs\.conversationContent[\s\S]*?appLogs\.executionDetails[\s\S]*?appLogs\.viewConversation/,
  'Conversation Workflow details must expose conversation and execution as peer tabs.'
);
assert.match(
  workflowDrawer,
  /value="conversation"[\s\S]*?<ConversationContext[\s\S]*?value="execution"[\s\S]*?executionContent/,
  'Conversation and execution content must render in separate top-level tab panels.'
);
assert.match(
  conversationDialog,
  /<Dialog[\s\S]*?messages\.map[\s\S]*?appLogs\.switchToTurn/,
  'The conversation dialog must render the message transcript and provide run switching.'
);
assert.match(
  workflowDrawer,
  /visibleTabs=\{isConversationWorkflow \? \['execution'\] : \['execution', 'results'\]\}/,
  'Conversation Workflow details must omit the duplicated final-result surface.'
);
assert.match(
  historyContent,
  /navigationVariant\?: 'default' \| 'compact'/,
  'Workflow history content must support compact navigation in the log drawer.'
);
assert.match(agentZh, /debugRuns: '调试日志'/);
assert.match(agentEn, /debugRuns: 'Debug Logs'/);
assert.match(agentZh, /仅展示当前草稿在编辑器中产生的调试运行/);
assert.match(agentEn, /Only shows runs created while debugging the current draft in the editor/);
assert.doesNotMatch(agentZh, /调试记录/);
assert.doesNotMatch(agentEn, /debug record/i);
assert.match(webappZh, /runtimeTitle: '运行日志'/);
assert.match(webappEn, /runtimeTitle: 'Runtime Logs'/);
assert.match(webappZh, /workflow: '智能体调用'/);
assert.match(webappEn, /workflow: 'Agent Calls'/);
assert.match(
  permissions,
  /'workflow\.logs\.view': \{ zhHans: '查看运行日志', enUS: 'View runtime logs' \}/
);
assert.match(runDropdown, /description\?: string/);
assert.match(runDropdown, /\{description \? \(/);
assert.match(workflowDrawer, /<RuntimeLogDetailHeader/);
assert.match(agentDrawer, /<RuntimeLogDetailHeader/);
for (const [rawStatus, normalizedStatus] of [
  ['pending-approval', 'pendingApproval'],
  ['pending-question', 'pendingQuestion'],
  ['waiting-client-action', 'pendingClientAction'],
  ['waiting-user-input', 'pendingUserInput'],
  ['waiting-for-user', 'pendingUserInput'],
  ['partial-succeeded', 'partialSucceeded'],
  ['resuming', 'resuming'],
  ['stopping', 'stopping'],
  ['exception', 'failed'],
  ['retry', 'retrying'],
  ['retrying', 'retrying'],
  ['submitted', 'submitted'],
  ['approved', 'approved'],
  ['rejected', 'rejected'],
  ['answered', 'answered'],
  ['blocked', 'blocked'],
  ['skipped', 'skipped'],
  ['expired', 'expired'],
]) {
  assert.match(
    runStatusBadge,
    new RegExp(`['\"]${rawStatus}['\"][\\s\\S]{0,160}['\"]${normalizedStatus}['\"]`),
    `Runtime status ${rawStatus} must be normalized to ${normalizedStatus}.`
  );
}
assert.match(runStatusBadge, /t\(`workflow\.status\.\$\{tone\}`\)/);
assert.match(agentZh, /pendingQuestion: '等待回答'/);
assert.match(agentEn, /pendingQuestion: 'Awaiting response'/);

console.log('Runtime log experience regression checks passed.');
