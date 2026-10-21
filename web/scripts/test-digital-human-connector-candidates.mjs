/**
 * Contract tests for the pre-creation Digital Human Connector candidate reader.
 *
 * The create flow can list the caller's own Connector connections before any
 * draft exists. These tests cover the data-access layer only: the request hits
 * the reviewed endpoint with just `query` and `limit`, carries no draft or
 * resource id and no credential value, reuses the existing credential-free
 * candidate list shape, and keeps a server search and the full 512-candidate
 * page in separate React Query cache entries.
 *
 * Run with: pnpm test:digital-human-connector-candidates
 */

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const transpile = relativePath =>
  ts.transpileModule(read(relativePath), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const sourceModuleURL = source =>
  `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;

// ---------------------------------------------------------------------------
// Query key separation
// ---------------------------------------------------------------------------

// query-keys.ts is pure: every runtime value is built from literals, so the
// real factories can be executed instead of pattern-matched.
const { DIGITAL_HUMAN_KEYS } = await import(
  sourceModuleURL(transpile('src/hooks/query-keys.ts'))
);

const emptyKey = DIGITAL_HUMAN_KEYS.connectorCandidates({});
assert.equal(
  emptyKey[emptyKey.length - 2],
  'connector-candidates',
  'connector candidates must live under their own key segment'
);
assert.equal(
  emptyKey[0],
  'digital-humans',
  'connector candidates must stay on the digital-human root'
);
assert.notDeepEqual(
  DIGITAL_HUMAN_KEYS.connectorCandidates({ query: '55' }),
  emptyKey,
  'a server search must not share the default cache entry'
);
assert.notDeepEqual(
  DIGITAL_HUMAN_KEYS.connectorCandidates({ limit: 512 }),
  emptyKey,
  'the full 512-candidate page must not share the default cache entry'
);
assert.notDeepEqual(
  DIGITAL_HUMAN_KEYS.connectorCandidates({ query: '55', limit: 512 }),
  DIGITAL_HUMAN_KEYS.connectorCandidates({ query: '55' }),
  'limit must be part of the key even when a search is present'
);

// ---------------------------------------------------------------------------
// Service endpoint, params, and shape
// ---------------------------------------------------------------------------

const serviceSource = read('src/services/digital-human.service.ts');
const connectorCandidatesMethod =
  serviceSource.match(
    /listConnectorCandidates\([\s\S]*?Promise<ApiResponseData<DigitalHumanResourceCandidateList>>[\s\S]*?return this\.request\('get', '\/digital-human-connector-candidates',[\s\S]*?\n {2}\}/
  )?.[0] ?? '';
assert.ok(connectorCandidatesMethod, 'the connector candidates service method must exist');
assert.match(
  connectorCandidatesMethod,
  /this\.request\('get', '\/digital-human-connector-candidates', undefined,/,
  'the reader must be a bodyless GET of /digital-human-connector-candidates'
);
assert.match(
  connectorCandidatesMethod,
  /Promise<ApiResponseData<DigitalHumanResourceCandidateList>>/,
  'the reader must reuse the existing candidate list shape'
);
// The params object may be built from query and limit and nothing else: this
// exact match is the id- and credential-free request guarantee.
assert.match(
  connectorCandidatesMethod,
  /params:\s*\{\s*query:\s*params\.query,\s*limit:\s*params\.limit\s*\}/,
  'the request must build its params object from query and limit only'
);
for (const forbidden of ['credential', 'secret', 'token', 'password', 'api_key', 'ssh']) {
  assert.ok(
    !connectorCandidatesMethod.toLowerCase().includes(forbidden),
    `the connector candidate request must not carry ${forbidden}`
  );
}
assert.doesNotMatch(
  connectorCandidatesMethod,
  /\bid\b|resource_id|connection_id/,
  'the request must not carry a draft, resource, or connection id'
);

const serviceImportsConnectorCandidates = serviceSource.includes(
  'listConnectorCandidates'
);
assert.ok(serviceImportsConnectorCandidates, 'the service export must remain discoverable');

// The candidate projection the create surface consumes declares no credential
// field either; it stays the shared binding-candidate shape.
const typesSource = read('src/services/types/digital-human.ts');
const candidateType =
  typesSource.match(/export interface DigitalHumanResourceCandidate \{[\s\S]*?\n\}/)?.[0] ?? '';
assert.ok(candidateType, 'DigitalHumanResourceCandidate must exist');
for (const forbidden of ['credential', 'secret', 'token', 'password', 'encrypted']) {
  assert.ok(
    !candidateType.toLowerCase().includes(forbidden),
    `the candidate projection must not declare ${forbidden}`
  );
}

// ---------------------------------------------------------------------------
// Hook wiring
// ---------------------------------------------------------------------------

const hooksSource = read('src/hooks/digital-human/use-digital-humans.ts');
const connectorCandidatesHook =
  hooksSource.match(/export function useDigitalHumanConnectorCandidates\([\s\S]*?\n\}/)?.[0] ?? '';
assert.ok(connectorCandidatesHook, 'the connector candidates hook must exist');
assert.match(connectorCandidatesHook, /useQuery\(/, 'the hook must read through useQuery');
assert.match(
  connectorCandidatesHook,
  /query:\s*params\.query\?\.trim\(\)\s*\|\|\s*undefined/,
  'the hook must normalize the optional server search'
);
assert.match(
  connectorCandidatesHook,
  /limit:\s*params\.limit/,
  'the hook must pass limit through unchanged so callers can request 512'
);
assert.doesNotMatch(
  connectorCandidatesHook,
  /Math\.min|Math\.max|slice\(|limit:\s*\d+/,
  'the hook must not clamp the caller-supplied limit'
);
assert.match(
  connectorCandidatesHook,
  /queryKey:\s*DIGITAL_HUMAN_KEYS\.connectorCandidates\(queryParams\)/,
  'the hook must put query and limit in the query key'
);
assert.match(
  connectorCandidatesHook,
  /queryFn:\s*\(\)\s*=>\s*digitalHumanService\.listConnectorCandidates\(queryParams\)/,
  'the hook must pass the same params to the service'
);
assert.match(connectorCandidatesHook, /enabled,/, 'the hook must pass enabled through');
assert.match(connectorCandidatesHook, /staleTime:\s*30_000/, 'the hook must cache for 30 seconds');

console.log('Digital human connector candidate data-access checks passed');
