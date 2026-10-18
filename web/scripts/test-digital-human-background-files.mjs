import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { load, jsx, nodes } from './connector-fixture.mjs';

const calls = [];
const { digitalHumanService } = load('src/services/digital-human.service.ts', {
  '@/lib/http/services': {
    BaseService: class {
      request(...args) {
        calls.push(args);
        return Promise.resolve({});
      }
    },
  },
});
const { DigitalHumanBackgroundRunFiles } = load(
  'src/components/digital-human/digital-human-background-run-files.tsx',
  {
    'react/jsx-runtime': jsx,
    '@/components/chat/variants/aichat/protected-generated-file-card': {
      ProtectedGeneratedFileCard: 'FileCard',
    },
    '@/services/digital-human.service': { digitalHumanService },
    '@/i18n': { useT: () => key => key },
  }
);
const appID = '11111111-2222-4333-8444-555555555555';
const runID = '22222222-2222-4333-8444-555555555555';
const fileID = '33333333-2222-4333-8444-555555555555';
const run = {
  id: runID,
  invocation_event: { origin: { kind: 'schedule' } },
  execution_trace: {
    generated_files: [
      {
        file_id: fileID,
        filename: 'result.csv',
        mime_type: 'text/csv',
        size: 12,
        url: 'https://untrusted.invalid/file',
      },
      { file_id: fileID, filename: 'duplicate' },
      { file_id: '../escape' },
    ],
  },
};
const cards = nodes(DigitalHumanBackgroundRunFiles({ digitalHumanId: appID, run })).filter(
  node => node.type === 'FileCard'
);
assert.equal(cards.length, 1);
assert.equal(cards[0].props.file.url, '');
assert.equal(cards[0].props.file.access_mode, 'authenticated');
assert.equal(cards[0].props.file.extension, '.csv');
const signal = new globalThis.AbortController().signal;
await cards[0].props.readFile('ignored', fileID, signal);
assert.deepEqual(calls, [
  [
    'get',
    `/digital-humans/${appID}/runs/${runID}/files/${fileID}`,
    undefined,
    { responseType: 'blob', signal },
  ],
]);
assert.equal(DigitalHumanBackgroundRunFiles({ digitalHumanId: '../escape', run }), null);
assert.equal(
  DigitalHumanBackgroundRunFiles({
    digitalHumanId: appID,
    run: { ...run, invocation_event: { origin: { kind: 'manual' } } },
  }),
  null
);
assert.equal(
  existsSync('src/app/console/digital-humans/[digitalHumanId]/files/[runId]/[fileId]/page.tsx'),
  false
);
console.log('DigitalHuman inline protected background files passed; no file landing page');
