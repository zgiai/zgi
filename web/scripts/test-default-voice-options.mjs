import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';
import { getDefaultVoiceOptions } from '../src/app/dashboard/settings/model/default-voice.ts';

const rules = [
  {
    name: 'default_voice',
    options: [' voice-1 ', 'voice-2', 'voice-1', ''],
    option_labels: {
      'voice-1': { zh_Hans: '音色一', en_US: 'Voice One' },
      'voice-2': { zh_Hans: '音色二' },
    },
  },
];

assert.deepEqual(getDefaultVoiceOptions(rules, 'zh-Hans'), [
  { value: 'voice-1', label: '音色一' },
  { value: 'voice-2', label: '音色二' },
]);

assert.deepEqual(getDefaultVoiceOptions(rules, 'en-US'), [
  { value: 'voice-1', label: 'Voice One' },
  { value: 'voice-2', label: '音色二' },
]);

assert.deepEqual(
  getDefaultVoiceOptions([{ name: 'default_voice', options: ['voice-3'] }], 'zh-Hans'),
  [{ value: 'voice-3', label: 'voice-3' }]
);

const modelSettingsSource = readFileSync(
  new URL('../src/app/dashboard/settings/model/page.tsx', import.meta.url),
  'utf8'
);
assert.doesNotMatch(
  modelSettingsSource,
  />\s*\{voice\.value\}\s*</,
  'The voice selector must not expose provider-internal identifiers as visible option text.'
);

console.log('default voice option tests passed');
