import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { compile } from '@mdx-js/mdx';
import remarkGfm from 'remark-gfm';
import YAML from 'yaml';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const spec = YAML.parse(read('../api/routes/external/digital-human-v1.openapi.yaml'));
const quickstart = read('../docs/digital-human-api-quickstart.md');
const documents = [
  ['en-US', read('src/components/digital-human/md/digital-human-api.en-US.mdx')],
  ['zh-Hans', read('src/components/digital-human/md/digital-human-api.zh-Hans.mdx')],
];
const methods = new Set(['get', 'post', 'put', 'patch', 'delete']);
const tocSource = read('src/components/agents/api/ui/toc.tsx');
const markdownComponentsSource = read('src/components/agents/api/ui/md.tsx');
const digitalHumanDocsSource = read('src/components/digital-human/digital-human-api-docs.tsx');
const nextConfigSource = read('next.config.mjs');

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
    digitalHumanDocsSource,
    new RegExp(`${tag}:\\s*MDComponents\\.${component}`),
    `DigitalHuman API docs are missing the ${tag} renderer`
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

for (const [locale, source] of documents) {
  assert.doesNotMatch(source, /````/, `${locale} docs contain an overlong code fence`);
  for (const [index, line] of source.split('\n').entries()) {
    if (!line.includes('```')) continue;
    assert.match(
      line,
      /^\s*```(?:bash|json|text|ts)?\s*$/,
      `${locale} docs have content on a code-fence line at ${index + 1}`
    );
  }

  const expectedTables = [...source.matchAll(/^\|\s*:?-{3,}/gm)].length;
  const compiled = String(await compile(source, { remarkPlugins: [remarkGfm] }));
  const renderedTables = [...compiled.matchAll(/_components\.table\b/g)].length;
  assert.equal(
    renderedTables,
    expectedTables,
    `${locale} DigitalHuman docs contain a Markdown table that did not compile`
  );
}

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

const publicEventMap = spec['x-zgi-sse-event-data-schemas'] ?? {};
const publicEventNames = Object.keys(publicEventMap).sort();
const requiredLifecycleEvents = [
  'error',
  'message',
  'message_end',
  'message_start',
  'run_accepted',
  'run_end',
];
const requiredSharedRuntimeEvents = [
  'agent_intermediate_answer',
  'agent_progress',
  'approval_expired',
  'approval_requested',
  'approval_result_filled',
  'client_action_required',
  'file_parse_end',
  'file_parse_error',
  'file_parse_start',
  'iteration_completed',
  'iteration_failed',
  'iteration_next',
  'iteration_started',
  'iteration_succeeded',
  'loop_completed',
  'loop_failed',
  'loop_next',
  'loop_started',
  'loop_succeeded',
  'memory_clear',
  'memory_create',
  'memory_delete',
  'memory_update',
  'message_retract',
  'node_finished',
  'node_started',
  'question_answer_requested',
  'question_answer_submitted',
  'runtime_input_accepted',
  'runtime_input_applied',
  'runtime_input_canceled',
  'runtime_input_failed',
  'runtime_input_follow_up_started',
  'skill_artifact_created',
  'skill_call_end',
  'skill_call_error',
  'skill_call_start',
  'skill_load_end',
  'skill_load_start',
  'skill_reference_read',
  'text_chunk',
  'text_replace',
  'token_budget_requested',
  'token_usage_updated',
  'tool_governance_decision',
  'user_input_requested',
  'workflow_failed',
  'workflow_finished',
  'workflow_paused',
  'workflow_resumed',
  'workflow_started',
  'workflow_stopped',
];
assert.deepEqual(
  publicEventNames,
  [...requiredLifecycleEvents, ...requiredSharedRuntimeEvents].sort(),
  'DigitalHuman must publish the lifecycle and shared ChatRuntime event contract'
);

for (const [eventName, schemaRef] of Object.entries(publicEventMap)) {
  const schemaName = schemaRef.split('/').at(-1);
  assert.ok(spec.components?.schemas?.[schemaName], `${eventName} references missing ${schemaName}`);
}

for (const [locale, source] of documents) {
  const guideHeading =
    locale === 'zh-Hans' ? '## 数字人事件接入手册' : '## DigitalHuman Event Integration Guide';
  const guideStart = source.indexOf(guideHeading);
  assert.ok(guideStart >= 0, `${locale} docs are missing the DigitalHuman event guide`);
  const guideEnd = source.indexOf('\n---\n\n## ', guideStart);
  assert.ok(guideEnd > guideStart, `${locale} DigitalHuman event guide has no end marker`);
  const guide = source.slice(guideStart, guideEnd);
  const eventTableStart = guide.search(/^\|/m);
  const eventTableEnd = guide.indexOf('\n### ', eventTableStart);
  const eventTable = guide.slice(
    eventTableStart,
    eventTableEnd > eventTableStart ? eventTableEnd : guide.length
  );
  const documentedEventRows = new Set(
    [...eventTable.matchAll(/`([a-z0-9_]+)`/g)].map(match => match[1])
  );

  for (const eventName of publicEventNames) {
    assert.ok(
      documentedEventRows.has(eventName),
      `${locale} DigitalHuman event guide is missing ${eventName}`
    );
  }
  for (const propertyName of [
    'run_id',
    'request_id',
    'conversation_id',
    'message_id',
    'answer',
    'status',
    'file_id',
    'skill_id',
    'action_id',
    'input_id',
    'workflow_run_id',
  ]) {
    assert.ok(
      guide.includes(propertyName),
      `${locale} DigitalHuman event guide is missing payload field ${propertyName}`
    );
  }
  assert.match(guide, /<Properties>/, `${locale} event guide is missing payload fields`);
  assert.match(guide, /```text/, `${locale} event guide is missing an SSE example`);
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
      const headings = [...source.matchAll(/<Heading[\s\S]*?\/>/g)];
      const heading = headings.find(
        candidate =>
          candidate[0].includes(`url="${documentedURL}"`) &&
          candidate[0].includes(`method="${method.toUpperCase()}"`)
      );
      assert.ok(heading, `${locale} docs are missing ${method.toUpperCase()} ${documentedURL}`);

      const start = heading.index;
      const nextHeading = source.indexOf('<Heading', start + heading[0].length);
      const block = source.slice(start, nextHeading >= 0 ? nextHeading : source.length);
      assert.match(
        block,
        /<Properties>/,
        `${locale} docs are missing fields for ${method.toUpperCase()} ${documentedURL}`
      );
      assert.match(
        block,
        /<CodeGroup title="(?:Request|请求)">\s*```bash\s*\n[\s\S]*?\bcurl [\s\S]*?\n\s*```\s*<\/CodeGroup>/,
        `${locale} docs are missing a standalone, closed cURL block for ${method.toUpperCase()} ${documentedURL}`
      );
      assert.match(
        block,
        /<CodeGroup title="(?:Response|Response Body|响应|响应体)"/,
        `${locale} docs are missing a response example for ${method.toUpperCase()} ${documentedURL}`
      );
      assert.match(
        block,
        /```(?:json|text)/,
        `${locale} docs are missing a response body for ${method.toUpperCase()} ${documentedURL}`
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
      `${locale} docs contain invalid JSON near character ${example.index}`
    );
  }
}

for (const required of [
  'ZGI_DIGITAL_HUMAN_API_KEY',
  'authenticatedZGIUser',
  'X-ZGI-Run-ID',
  'Last-Event-ID',
  'request_id',
  'readZGISSE',
  'applyZGIEvent',
  '/digital-humans/conversations',
  '/digital-humans/files/upload',
]) {
  assert.ok(quickstart.includes(required), `Quickstart is missing ${required}`);
}
assert.ok(
  !quickstart.includes('NEXT_PUBLIC_ZGI_DIGITAL_HUMAN_API_KEY='),
  'Quickstart leaks the API key'
);
assert.ok(
  !quickstart.includes('X-External-User-ID'),
  'DigitalHuman docs must keep the public field named user'
);

for (const [locale, source] of documents) {
  assert.ok(
    !source.includes('X-External-User-ID'),
    `${locale} docs must keep the field named user`
  );
  assert.ok(
    source.includes('docs/digital-human-api-quickstart.md'),
    `${locale} docs must link the demo guide`
  );
}

console.log(
  `DigitalHuman API docs contract passed: ${expectedHeadings.length} operations, ${publicEventNames.length} SSE events, ${documents.length} locales.`
);
