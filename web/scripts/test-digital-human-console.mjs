import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';

const list = await readFile(
  new URL('../src/components/digital-human/digital-human-list.tsx', import.meta.url),
  'utf8'
);
const configuration = await readFile(
  new URL('../src/components/digital-human/digital-human-configuration.tsx', import.meta.url),
  'utf8'
);
const resourceBindings = await readFile(
  new URL('../src/components/digital-human/digital-human-resource-bindings.tsx', import.meta.url),
  'utf8'
);
const service = await readFile(
  new URL('../src/services/digital-human.service.ts', import.meta.url),
  'utf8'
);
const hooks = await readFile(
  new URL('../src/hooks/digital-human/use-digital-humans.ts', import.meta.url),
  'utf8'
);
const runs = await readFile(
  new URL('../src/components/digital-human/digital-human-runs.tsx', import.meta.url),
  'utf8'
);
const runDetail = await readFile(
  new URL('../src/components/digital-human/digital-human-run-detail-drawer.tsx', import.meta.url),
  'utf8'
);
const chat = await readFile(
  new URL('../src/components/digital-human/digital-human-chat.tsx', import.meta.url),
  'utf8'
);
const runtimeTransport = await readFile(
  new URL('../src/components/chat/transports/digital-human-runtime-transport.ts', import.meta.url),
  'utf8'
);
const overview = await readFile(
  new URL('../src/components/digital-human/digital-human-overview.tsx', import.meta.url),
  'utf8'
);
const layout = await readFile(
  new URL('../src/app/console/digital-humans/[digitalHumanId]/layout.tsx', import.meta.url),
  'utf8'
);
const memory = await readFile(
  new URL('../src/components/digital-human/digital-human-memory.tsx', import.meta.url),
  'utf8'
);

for (const [name, source] of [
  ['list', list],
  ['configuration', configuration],
]) {
  assert.match(
    source,
    /useAvailableModels\(\{ use_case: 'agent-runtime' \}\)/,
    `${name} must use agent-runtime candidates`
  );
  assert.doesNotMatch(
    source,
    /availableModels\.models\.filter\([^)]*callable/,
    `${name} must not filter the compact available-model response by its omitted callable field`
  );
}

assert.doesNotMatch(
  service,
  /[`'"/]agents(?:[`'"/]|$)/,
  'DigitalHuman service must not call Agent endpoints'
);
assert.match(
  memory,
  /useDigitalHumanMemories[\s\S]*memory_class[\s\S]*scope_type[\s\S]*scope_id[\s\S]*status[\s\S]*source/,
  'DigitalHuman memory must expose class, scope type, scope ID, status, and source filters'
);
assert.match(
  memory,
  /evidence[\s\S]*last_operation_id[\s\S]*undoMemory/,
  'DigitalHuman memory must expose evidence, provenance, and undo'
);
assert.match(
  memory,
  /ConfirmDialog[\s\S]*invalidateTitle/,
  'DigitalHuman memory invalidation must require confirmation'
);
assert.doesNotMatch(
  memory,
  /MEMORY_KEYS\.me|\/console\/api\/agents\//,
  'DigitalHuman memory must not use account memory or Agent endpoints'
);

assert.match(
  service,
  /\/digital-humans\/\$\{id\}\/draft\/binding-candidates/,
  'DigitalHuman bindings must use the independent resource candidate endpoint'
);
assert.match(
  resourceBindings,
  /useDigitalHumanBindingCandidates/,
  'DigitalHuman configuration must use searchable resource candidates'
);
assert.doesNotMatch(
  resourceBindings,
  /JSON\.parse\(metadata|setResourceId|setRevision/,
  'DigitalHuman configuration must not require raw IDs, revisions, or metadata JSON'
);
assert.match(
  configuration,
  /configuration\.preflight\.title/,
  'DigitalHuman publication must show a preflight checklist'
);
assert.match(
  overview,
  /DigitalHumanPublishPanel[\s\S]*overview\.setupTitle/,
  'DigitalHuman overview must guide setup before publication'
);
assert.match(
  layout,
  /tabs\.overview[\s\S]*tabs\.settings[\s\S]*tabs\.capabilities[\s\S]*tabs\.chat[\s\S]*tabs\.runs/,
  'DigitalHuman detail navigation must separate overview, identity, capabilities, chat, and runs'
);
assert.match(
  resourceBindings,
  /ResourceBindingSection[\s\S]*ResourceSelectionDialog[\s\S]*saveSelection/,
  'DigitalHuman capabilities must use categorized sections and a batch resource picker'
);
for (const section of ['skills', 'knowledge', 'databases', 'workflows', 'integrations']) {
  assert.match(
    resourceBindings,
    new RegExp(`bindings\\.sections\\.${section}`),
    `DigitalHuman capabilities must expose the ${section} resource section`
  );
}
assert.doesNotMatch(
  list,
  /fields\.persona|fields\.rules|fields\.model/,
  'DigitalHuman creation must defer detailed configuration until after creation'
);
for (const [name, source] of [
  ['list', list],
  ['configuration', configuration],
]) {
  assert.match(source, /DialogBody/, `${name} dialogs must use the shared padded body`);
}
for (const primitive of ['StickyDataTable', 'RunStatusBadge', 'DigitalHumanRunDetailDrawer']) {
  assert.match(runs, new RegExp(primitive), `DigitalHuman run history must use ${primitive}`);
}
assert.match(
  runDetail,
  /SheetContent[\s\S]*executionTimeline[\s\S]*technicalDetails/,
  'DigitalHuman run details must use an Agent-style drawer with an execution timeline'
);
assert.match(
  chat,
  /AIChatShell[\s\S]*createDigitalHumanRuntimeTransport[\s\S]*defaultSidebarOpen/,
  'DigitalHuman chat must use the shared conversation UI with its sidebar'
);
assert.match(
  runtimeTransport,
  /submitInvocation[\s\S]*recoverConversationStream/,
  'DigitalHuman chat turns must enter through durable runs and recover from ChatRuntime'
);
assert.doesNotMatch(
  `${chat}\n${runtimeTransport}`,
  /MEMORY_KEYS\.me|\/console\/api\/agents\//,
  'DigitalHuman chat must not use account memory or Agent endpoints'
);
assert.match(
  hooks,
  /digital_human\.revision\.conflict[\s\S]*invalidateDetail/,
  'DigitalHuman optimistic conflicts must refetch the server draft'
);
assert.match(runDetail, /useDigitalHumanRun/, 'Run details must load the latest durable run state');
assert.match(
  runDetail,
  /execution_snapshot_revision/,
  'Run details must expose the sealed snapshot revision'
);
assert.doesNotMatch(
  `${runs}\n${runDetail}`,
  /JSON\.stringify\(run\.(run_spec|execution_snapshot)/,
  'Run details must not dump raw execution snapshots into the console'
);

console.log('DigitalHuman console contract checks passed.');
