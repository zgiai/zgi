import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { compile } from '@mdx-js/mdx';
import remarkGfm from 'remark-gfm';
import YAML from 'yaml';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const spec = YAML.parse(read('../api/routes/external/external-v1.openapi.yaml'));
const quickstart = read('../docs/agent-api-quickstart.md');
const documents = [
  ['en-US', read('src/components/agents/api/md/agent-api.en-US.mdx')],
  ['zh-Hans', read('src/components/agents/api/md/agent-api.zh-Hans.mdx')],
];
const renderDocuments = [
  ...documents.map(([locale, source]) => [`agent-${locale}`, source]),
  ['workflow-en-US', read('src/components/agents/api/md/workflow-api.en-US.mdx')],
  ['workflow-zh-Hans', read('src/components/agents/api/md/workflow-api.zh-Hans.mdx')],
  ['chat-workflow-en-US', read('src/components/agents/api/md/chat-workflow-api.en-US.mdx')],
  ['chat-workflow-zh-Hans', read('src/components/agents/api/md/chat-workflow-api.zh-Hans.mdx')],
];
const methods = new Set(['get', 'post', 'put', 'patch', 'delete']);
const tocSource = read('src/components/agents/api/ui/toc.tsx');
const markdownComponentsSource = read('src/components/agents/api/ui/md.tsx');
const docsTabSource = read('src/components/agents/api/api-docs-tab.tsx');
const nextConfigSource = read('next.config.mjs');
const serializedSpec = JSON.stringify(spec);

assert.match(
  nextConfigSource,
  /remarkPlugins:\s*\[\s*['"]remark-gfm['"]\s*\]/,
  'Next MDX config must enable GitHub-flavored Markdown'
);
for (const [tag, component] of [
  ['table', 'Table'],
  ['thead', 'TableHead'],
  ['tbody', 'TableBody'],
  ['tr', 'TableRow'],
  ['th', 'TableHeaderCell'],
  ['td', 'TableCell'],
]) {
  assert.match(
    docsTabSource,
    new RegExp(`${tag}:\\s*MDComponents\\.${component}`),
    `API docs are missing the ${tag} renderer`
  );
}

const gfmSmokeOutput = String(
  await compile('| Header |\n| --- |\n| Cell |\n\n~~removed~~\n\n- [x] complete', {
    remarkPlugins: [remarkGfm],
  })
);
for (const tag of ['table', 'del', 'input']) {
  assert.match(
    gfmSmokeOutput,
    new RegExp(`_components\\.${tag}`),
    `GFM ${tag} syntax is not compiled`
  );
}

for (const [documentName, source] of renderDocuments) {
  const expectedTables = [...source.matchAll(/^\|\s*:?-{3,}/gm)].length;
  const compiled = String(await compile(source, { remarkPlugins: [remarkGfm] }));
  const renderedTables = [...compiled.matchAll(/_components\.table\b/g)].length;
  assert.equal(
    renderedTables,
    expectedTables,
    `${documentName} API docs contain a Markdown table that did not compile`
  );
}

for (const forbidden of [
  'client_action_required',
  'client_action_result',
  '/client-actions/',
  'waiting_client_action',
]) {
  assert.ok(!serializedSpec.includes(forbidden), `Agent OpenAPI exposes ${forbidden}`);
  for (const [locale, source] of documents) {
    assert.ok(!source.includes(forbidden), `${locale} Agent docs expose ${forbidden}`);
  }
}

const governancePath =
  '/agents/conversations/{conversation_id}/messages/{message_id}/tool-governance/{correlation_id}/continue';
const governance = spec.paths?.[governancePath]?.post;
assert.ok(governance, 'Agent OpenAPI must expose the admitted platform CLI continuation');
assert.equal(governance.operationId, 'continueAgentPlatformToolGovernance');
assert.deepEqual(
  governance.requestBody?.content?.['application/json']?.schema?.properties?.action?.enum,
  ['approve', 'reject'],
  'CLI continuation must only confirm or reject a frozen invocation'
);
assert.equal(governance.responses?.['200']?.$ref, '#/components/responses/AgentEventStream');
assert.match(governance.description, /never the original Shell/);
assert.match(governance.description, /resource authorization/);
assert.match(governance.description, /Rejection remains available after execution is disabled/);
assert.deepEqual(
  Object.keys(spec.paths).filter(apiPath => apiPath.includes('/tool-governance/')),
  [governancePath],
  'Only the dedicated Agent CLI continuation is public, not internal governance endpoints'
);

assert.match(
  tocSource,
  /querySelectorAll\('\[data-toc-entry="true"\]'\)/,
  'API docs TOC must opt headings in instead of listing every response subsection'
);
assert.doesNotMatch(
  tocSource,
  /querySelectorAll\('h1,h2,h3,h4,h5,h6'\)/,
  'API docs TOC must not include every rendered heading'
);
for (const level of ['1', '2', '3']) {
  assert.match(
    markdownComponentsSource,
    new RegExp(`data-toc-level="${level}"`),
    `API docs TOC is missing level ${level}`
  );
}

const assertDescribedProperties = (schema, location) => {
  if (!schema || typeof schema !== 'object') return;

  for (const [name, property] of Object.entries(schema.properties ?? {})) {
    assert.ok(
      property.description || property.$ref,
      `${location}.${name} is missing a field description`
    );
    assertDescribedProperties(property, `${location}.${name}`);
  }

  for (const [index, variant] of (schema.allOf ?? []).entries()) {
    assertDescribedProperties(variant, `${location}.allOf[${index}]`);
  }
  for (const [index, variant] of (schema.oneOf ?? []).entries()) {
    assertDescribedProperties(variant, `${location}.oneOf[${index}]`);
  }
};

for (const [name, schema] of Object.entries(spec.components?.schemas ?? {})) {
  assertDescribedProperties(schema, `components.schemas.${name}`);
}

const publicEventNames = Object.keys(spec['x-zgi-sse-event-data-schemas'] ?? {}).sort();
assert.ok(publicEventNames.length > 0, 'Agent OpenAPI is missing its public SSE event map');
assert.equal(
  spec['x-zgi-sse-event-data-schemas']?.approval_requested,
  '#/components/schemas/ApprovalRequestedEventData',
  'approval_requested must use its nested public event contract'
);

const collectEventPropertyNames = (schema, names = new Set(), visitedRefs = new Set()) => {
  if (!schema || typeof schema !== 'object') return names;

  if (schema.$ref) {
    if (visitedRefs.has(schema.$ref)) return names;
    visitedRefs.add(schema.$ref);
    const schemaName = schema.$ref.split('/').at(-1);
    return collectEventPropertyNames(spec.components?.schemas?.[schemaName], names, visitedRefs);
  }

  for (const [name, property] of Object.entries(schema.properties ?? {})) {
    names.add(name);
    collectEventPropertyNames(property, names, visitedRefs);
  }
  for (const child of [...(schema.allOf ?? []), ...(schema.oneOf ?? []), ...(schema.anyOf ?? [])]) {
    collectEventPropertyNames(child, names, visitedRefs);
  }
  collectEventPropertyNames(schema.items, names, visitedRefs);
  return names;
};

const publicEventPropertyNames = new Set();
for (const schemaRef of Object.values(spec['x-zgi-sse-event-data-schemas'])) {
  collectEventPropertyNames({ $ref: schemaRef }, publicEventPropertyNames);
}

const approvalEventProperties = collectEventPropertyNames({
  $ref: spec['x-zgi-sse-event-data-schemas'].approval_requested,
});
for (const propertyName of [
  'approval_form_id',
  'approval_token',
  'ui_approval_allowed',
  'approval_form',
  'actions',
  'fields',
  'expiration_at',
]) {
  assert.ok(
    approvalEventProperties.has(propertyName),
    `approval_requested schema is missing ${propertyName}`
  );
}

for (const [name, source] of [...documents, ['quickstart', quickstart]]) {
  assert.match(source, /approval_form/, `${name} approval docs must describe the nested form`);
  assert.match(source, /approval_token/, `${name} approval docs must describe the canonical token`);
  assert.match(source, /ui_approval_allowed/, `${name} approval docs must describe API approval access`);
}

for (const [locale, source] of documents) {
  assert.match(source, /requires_approval=true/, `${locale} must describe admitted CLI approval`);
  assert.match(source, /remember_for_session/, `${locale} must explain non-reusable CLI decisions`);
  assert.doesNotMatch(
    source,
    /Always `false` in the published Agent API|在已发布 Agent API 中固定为 `false`|has no human continuation endpoint|不对应人工继续接口/,
    `${locale} must not describe all published tools as unconditionally non-interactive`
  );
  const guideHeading =
    locale === 'zh-Hans' ? '## Agent 事件接入手册' : '## Agent Event Integration Guide';
  const guideStart = source.indexOf(guideHeading);
  assert.ok(guideStart >= 0, `${locale} docs are missing the Agent event integration guide`);

  const guideEnd = source.indexOf('\n---\n\n## ', guideStart);
  assert.ok(guideEnd > guideStart, `${locale} Agent event integration guide has no end marker`);
  const guide = source.slice(guideStart, guideEnd);
  const documentedEventRows = new Set(
    [...guide.matchAll(/^\|\s*`([a-z0-9_]+)`\s*\|/gm)].map(match => match[1])
  );

  for (const eventName of publicEventNames) {
    assert.ok(
      documentedEventRows.has(eventName),
      `${locale} Agent event guide is missing a table entry for ${eventName}`
    );
  }
  for (const propertyName of publicEventPropertyNames) {
    assert.ok(
      guide.includes(propertyName),
      `${locale} Agent event guide is missing the ${propertyName} payload field`
    );
  }

  assert.match(guide, /<Properties>/, `${locale} Agent event guide is missing payload fields`);
  assert.match(guide, /```ts/, `${locale} Agent event guide is missing client integration code`);
  assert.match(guide, /```text/, `${locale} Agent event guide is missing SSE examples`);
}

for (const [apiPath, pathItem] of Object.entries(spec.paths ?? {})) {
  for (const [method, operation] of Object.entries(pathItem ?? {})) {
    if (!methods.has(method)) continue;

    assert.ok(operation.summary, `${method.toUpperCase()} ${apiPath} is missing summary`);
    assert.ok(operation.description, `${method.toUpperCase()} ${apiPath} is missing description`);
    assert.ok(operation.operationId, `${method.toUpperCase()} ${apiPath} is missing operationId`);
    assert.ok(
      operation.responses?.['200'] ?? operation.responses?.['201'],
      `${method.toUpperCase()} ${apiPath} is missing a success response`
    );
    assert.ok(
      operation.responses?.default,
      `${method.toUpperCase()} ${apiPath} is missing the common error response`
    );

    const documentedURL = `/api/v1${apiPath}`;
    for (const [locale, source] of documents) {
      const urlToken = `url="${documentedURL}"`;
      const methodToken = `method="${method.toUpperCase()}"`;
      const headings = [...source.matchAll(/<Heading[\s\S]*?\/>/g)];
      const match = headings.find(
        candidate => candidate[0].includes(urlToken) && candidate[0].includes(methodToken)
      );
      assert.ok(match, `${locale} docs are missing ${method.toUpperCase()} ${documentedURL}`);

      const headingStart = match.index;
      const headingEnd = headingStart + match[0].length;
      const nextHeading = source.indexOf('<Heading', headingEnd);
      const block = source.slice(headingStart, nextHeading >= 0 ? nextHeading : source.length);
      assert.match(
        block,
        /<Properties>/,
        `${locale} docs are missing parameter or response fields for ${method.toUpperCase()} ${documentedURL}`
      );
      assert.match(
        block,
        /```bash[\s\S]*?curl /,
        `${locale} docs are missing a cURL request for ${method.toUpperCase()} ${documentedURL}`
      );
      assert.match(
        block,
        /<CodeGroup title="(?:Response|Response Body|响应|响应体)"/,
        `${locale} docs are missing a response example for ${method.toUpperCase()} ${documentedURL}`
      );
      assert.match(
        block,
        /```(?:json|text)/,
        `${locale} docs are missing a JSON or SSE body example for ${method.toUpperCase()} ${documentedURL}`
      );
    }
  }
}

const documentedHeadings = source =>
  [
    ...source.matchAll(
      /<Heading[\s\S]*?url="(\/api\/v1[^"]+)"[\s\S]*?method="(GET|POST|PUT|PATCH|DELETE)"[\s\S]*?\/>/g
    ),
  ]
    .map(match => `${match[2].toLowerCase()} ${match[1].slice('/api/v1'.length)}`)
    .sort();
const expectedHeadings = Object.entries(spec.paths ?? {})
  .flatMap(([apiPath, pathItem]) =>
    Object.keys(pathItem ?? {})
      .filter(method => methods.has(method))
      .map(method => `${method} ${apiPath}`)
  )
  .sort();

for (const [locale, source] of documents) {
  assert.deepEqual(
    documentedHeadings(source),
    expectedHeadings,
    `${locale} endpoint list differs from OpenAPI`
  );

  for (const example of source.matchAll(/```json[^\n]*\n([\s\S]*?)```/g)) {
    assert.doesNotThrow(
      () => JSON.parse(example[1]),
      `${locale} docs contain an invalid JSON response or request example near character ${example.index}`
    );
  }
}
