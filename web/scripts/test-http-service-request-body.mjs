// Guards BaseService.request: every HTTP method must forward its body argument.
// A DELETE that silently drops the payload makes the Go connector detach endpoint
// reject the call, because it reads the optimistic-lock revision from the body.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const read = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');
const transpile = code =>
  ts.transpileModule(code, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const moduleURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`;

const calls = [];
const record = method => (...args) => {
  calls.push({ method, args });
  return Promise.resolve({ data: { ok: true } });
};

const clientStub = moduleURL(
  transpile(`export const httpClientFactory = { getClient: () => ({}), };`)
);
const configStub = moduleURL(
  transpile(`export const buildApiUrl = p => p; export const UPLOAD_TIMEOUT_MS = 1;`)
);

const servicesURL = moduleURL(
  transpile(read('src/lib/http/services.ts'))
    .replace("'./client'", JSON.stringify(clientStub))
    .replace("'./config'", JSON.stringify(configStub))
);
const { BaseService } = await import(servicesURL);

const recorder = {
  get: record('get'),
  post: record('post'),
  put: record('put'),
  patch: record('patch'),
  delete: record('delete'),
};

class TestService extends BaseService {
  constructor() {
    super({ endpoint: undefined, basePath: '' });
    this.client = recorder;
  }
  call(method, path, data, options) {
    return this.request(method, path, data, options);
  }
}

const service = new TestService();

await service.call('delete', '/connector-target-bindings/binding-1', { revision: 1 });
const [bodyCall] = calls;
assert.equal(bodyCall.method, 'delete');
assert.equal(bodyCall.args[0], '/connector-target-bindings/binding-1');
assert.deepEqual(
  bodyCall.args[1]?.data,
  { revision: 1 },
  'DELETE must forward its body through axios config.data'
);

// A bodyless DELETE must stay bodyless: adding `data: undefined` would make axios
// send the request differently than every other DELETE caller already expects.
calls.length = 0;
await service.call('delete', '/connector-connections/connection-1', undefined, {
  params: { force: true },
});
const [plainCall] = calls;
assert.equal('data' in (plainCall.args[1] ?? {}), false, 'bodyless DELETE must not gain a data key');
assert.deepEqual(plainCall.args[1].params, { force: true });

for (const method of ['post', 'put', 'patch']) {
  calls.length = 0;
  await service.call(method, '/resource', { revision: 2 });
  assert.deepEqual(calls[0].args[1], { revision: 2 }, `${method} must forward its body argument`);
}

console.log('http service request body contract passed');
