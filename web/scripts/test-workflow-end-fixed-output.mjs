import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import { checkValid } from '../src/components/workflow/nodes/end/config.ts';
import { validateComplexOutputValue } from '../src/components/workflow/nodes/end/manager/complex-output-value.ts';

const constantResult = checkValid({
  type: 'end',
  title: 'End',
  desc: '',
  isInLoop: false,
  isInIteration: false,
  outputs: [
    {
      variable: 'message',
      type: 'string',
      value_type: 'constant',
      value: 'workflow completed',
    },
  ],
});
assert.equal(constantResult.isValid, true);

const unboundVariableResult = checkValid({
  type: 'end',
  title: 'End',
  desc: '',
  isInLoop: false,
  isInIteration: false,
  outputs: [
    {
      variable: 'message',
      type: 'string',
      value_type: 'variable',
      value_selector: [],
    },
  ],
});
assert.equal(unboundVariableResult.isValid, false);

assert.deepEqual(validateComplexOutputValue('array[string]', '["a", "b"]'), {
  value: ['a', 'b'],
});
assert.deepEqual(validateComplexOutputValue('array[string]', '[1]'), {
  error: 'invalidArrayItems',
});
assert.deepEqual(validateComplexOutputValue('object', '[]'), {
  error: 'expectedObject',
});
assert.deepEqual(validateComplexOutputValue('object', '{'), {
  error: 'invalidJson',
});

const manager = readFileSync(
  path.join(process.cwd(), 'src/components/workflow/nodes/end/manager/index.tsx'),
  'utf8'
);
assert.match(manager, /value_type: 'constant'/);
assert.match(manager, /DefaultValueEditor/);
assert.match(manager, /ComplexOutputValueDialog/);
assert.match(manager, /BooleanConstantEditor/);
assert.match(manager, /layout="inline"/);
assert.match(manager, /constantLabel=\{t\('end\.outputs\.constant'\)\}/);

console.log('Workflow end fixed output regression checks passed.');
