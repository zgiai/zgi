import assert from 'node:assert/strict';

import { isPermissionError } from '../src/components/skills/skill-availability.ts';

// A refused enable shows the "ask an administrator" notice; anything else is
// an ordinary failure.
assert.equal(isPermissionError({ status: 403 }), true);
assert.equal(isPermissionError({ response: { status: 401 } }), true);
assert.equal(isPermissionError({ code: 'aichat.skill.forbidden' }), true);
assert.equal(isPermissionError({ status: 500, code: 'internal' }), false);
assert.equal(isPermissionError(new Error('network')), false);
assert.equal(isPermissionError(null), false);

console.log('Skill availability checks passed');
