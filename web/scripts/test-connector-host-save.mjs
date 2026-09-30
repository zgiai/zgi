import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

function load(file, imports = {}) {
  const compiled = ts.transpileModule(readFileSync(file, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', compiled)(
    name => {
      assert.ok(name in imports, name);
      return imports[name];
    },
    module,
    module.exports
  );
  return module.exports;
}
const { mergeSavedConnectorNodes } = load(
  'src/components/workflow/store/helpers/merge-saved-connectors.ts'
);
const submitted = {
  id: 'node',
  position: { x: 1, y: 2 },
  data: {
    provider_type: 'connector',
    provider_id: 'github',
    tool_name: 'get_user',
    connector: { connector_id: 'github', connection_id: 'chosen', operation_id: 'github.user.get' },
    inputs: { value: 'before' },
  },
};
const saved = {
  ...submitted,
  data: {
    ...submitted.data,
    connector: { ...submitted.data.connector, binding_id: 'canonical', revision: 1 },
  },
};
const current = {
  ...submitted,
  position: { x: 20, y: 30 },
  data: { ...submitted.data, inputs: { value: 'typed-during-save' } },
};
const [merged] = mergeSavedConnectorNodes([current], [submitted], [saved]);
assert.equal(merged.data.connector.binding_id, 'canonical');
assert.deepEqual(merged.position, current.position);
assert.deepEqual(merged.data.inputs, current.data.inputs);
for (const field of ['connection_id', 'operation_id']) {
  const changed = {
    ...current,
    data: {
      ...current.data,
      connector: { ...current.data.connector, [field]: 'different-choice' },
    },
  };
  assert.equal(
    mergeSavedConnectorNodes([changed], [submitted], [saved])[0],
    changed,
    `${field} edited during save must survive`
  );
}
assert.deepEqual(
  mergeSavedConnectorNodes([], [submitted], [saved]),
  [],
  'save cannot revive a deleted node'
);
assert.equal(
  mergeSavedConnectorNodes([current], [], [saved])[0],
  current,
  'save cannot modify an unsubmitted node'
);

const calls = [];
class BaseService {
  request(...args) {
    calls.push(args);
    return Promise.resolve({ data: { items: [] } });
  }
}
const { connectorService } = load('src/services/connector.service.ts', {
  '@/lib/http/services': { BaseService },
});
const target = { type: 'workflow', id: 'workflow', scope: 'published', version_id: 'version' };
await connectorService.getTargetBindings(target);
await connectorService.getUnavailableReferences(target);
assert.deepEqual(calls[0][3].params, {
  target_type: 'workflow',
  target_id: 'workflow',
  target_scope: 'published',
  target_version_id: 'version',
});
assert.deepEqual(calls[1][3].params, calls[0][3].params);
const command = { reference_id: 'ref/encoded', expected_revision: 3, action: 'detach' };
for (const type of ['agent', 'workflow'])
  await assert.rejects(
    connectorService.resolveUnavailableReference({ type, id: 'host', scope: 'draft' }, command)
  );
assert.equal(calls.length, 2, 'host repair must be staged without a separate HTTP mutation');
await connectorService.resolveUnavailableReference(
  { type: 'chat', id: 'chat', scope: 'live' },
  command
);
assert.equal(calls[2][1], '/connector-unavailable-references/ref%2Fencoded/resolve');
assert.equal(calls[2][3].retryAttemptsOverride, 0);
assert.ok(calls.every(call => !call[1].includes('integrations')));
console.log('Connector host save concurrency, scope and native HTTP boundary passed.');
