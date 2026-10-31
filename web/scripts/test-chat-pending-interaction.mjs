import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

// ADR 0050 step 2b: one answer to "what must the person resolve now", and
// pending approvals that belong to one chat.

const read = path => readFile(new URL(path, import.meta.url), 'utf8');
const load = async source =>
  import(
    `data:text/javascript;base64,${Buffer.from(
      ts.transpileModule(source, {
        compilerOptions: {
          module: ts.ModuleKind.ESNext,
          target: ts.ScriptTarget.ES2022,
          jsx: ts.JsxEmit.Preserve,
        },
      }).outputText
    ).toString('base64')}`
  );

const { selectPendingInteraction } = await load(
  await read('../src/components/chat/controllers/aichat/pending-interaction.ts')
);

const none = { workflowApproval: false, hasQuestion: false };
assert.equal(selectPendingInteraction({ ...none }), null);
assert.equal(selectPendingInteraction({ ...none, leafStatus: 'completed' }), null);
assert.equal(selectPendingInteraction({ ...none, leafStatus: 'streaming' }), null);
for (const [leafStatus, kind] of [
  ['waiting_approval', 'tool_approval'],
  ['waiting_question', 'question'],
  ['waiting_client_action', 'page_action'],
  ['waiting_budget', 'budget'],
]) {
  assert.equal(selectPendingInteraction({ ...none, leafStatus }).kind, kind, leafStatus);
}
// The order the composer shows them in.
assert.deepEqual(
  selectPendingInteraction({
    workflowApproval: true,
    toolApprovalId: 'a1',
    hasQuestion: true,
    leafStatus: 'waiting_approval',
  }),
  { kind: 'workflow_approval' }
);
assert.deepEqual(selectPendingInteraction({ ...none, toolApprovalId: 'a1', hasQuestion: true }), {
  kind: 'tool_approval',
  approvalId: 'a1',
});
assert.deepEqual(
  selectPendingInteraction({ ...none, hasQuestion: true, questionRequestId: 'q1' }),
  { kind: 'question', requestId: 'q1' }
);

// The approval scope is taken out of the card component on its own; it needs
// nothing else from that file.
const cardPath = '../src/components/chat/variants/aichat/tool-governance-decision-card.tsx';
const cardText = await read(cardPath);
const card = ts.createSourceFile(
  cardPath,
  cardText,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TSX
);
const scopeSource = card.statements
  .filter(
    statement =>
      (ts.isFunctionDeclaration(statement) &&
        statement.name?.text === 'createToolGovernancePendingApprovalScope') ||
      (ts.isInterfaceDeclaration(statement) &&
        [
          'PendingApprovalEntry',
          'ToolGovernancePendingApprovalScope',
          'ToolGovernancePendingApproval',
        ].includes(statement.name.text))
  )
  .map(statement => statement.getText(card))
  .join('\n');
assert.match(scopeSource, /createToolGovernancePendingApprovalScope/);
assert.doesNotMatch(
  cardText,
  /^const (pendingApprovalEntries|dismissedPendingApprovalKeys|pendingApprovalSnapshots) = new/m,
  'no module-wide approval state remains'
);
const { createToolGovernancePendingApprovalScope: createScope } = await load(scopeSource);

const approval = (id, createdAt) => ({
  id,
  conversationId: 'c1',
  messageId: 'm1',
  title: id,
  actionSentence: id,
  assets: [],
  canSubmit: true,
  isHighImpact: false,
  createdAt,
});

const sidebar = createScope();
const page = createScope();
let notified = 0;
sidebar.subscribe(() => {
  notified += 1;
});
const versionAtStart = sidebar.getVersion();
const unpublishOld = sidebar.publish(approval('old', 1));
sidebar.publish(approval('new', 2));
assert.equal(sidebar.getSnapshot().id, 'new', 'the latest approval is shown');
assert.equal(page.getSnapshot(), null, 'another chat does not see it');
assert.equal(notified, 2);
assert.ok(sidebar.getVersion() > versionAtStart);

const restore = sidebar.dismiss(approval('new', 2));
assert.equal(sidebar.isDismissed('new'), true);
assert.equal(page.isDismissed('new'), false, 'a dismissal stays in its chat');
assert.equal(sidebar.getSnapshot().id, 'old');
sidebar.publish(approval('new', 2));
assert.equal(sidebar.getSnapshot().id, 'old', 'a dismissed approval is not published again');
restore();
assert.equal(sidebar.isDismissed('new'), false);
assert.equal(sidebar.getSnapshot().id, 'new', 'restoring brings it back');

// A stale unpublish does not remove a newer copy of the same approval.
const unpublishFirst = sidebar.publish(approval('same', 3));
sidebar.publish(approval('same', 3));
unpublishFirst();
assert.equal(sidebar.getSnapshot().id, 'same');
unpublishOld();

console.log('chat pending interaction and approval scope: ok');
