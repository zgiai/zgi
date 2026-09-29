import assert from 'node:assert/strict';

import {
  DEFAULT_CALL_DATABASE_NODE_DATA,
  checkValid,
} from '../src/components/workflow/nodes/call-database/config.ts';

const node = structuredClone(DEFAULT_CALL_DATABASE_NODE_DATA);
node.data.data_source = {
  id: 'database-1',
  name: 'Database 1',
  type: 'postgres',
  schema_name: 'public',
};
node.data.table_selection = [];
node.data.manual_sql = 'SELECT 1';

assert.deepEqual(checkValid(node), {
  isValid: true,
  errors: [],
  warnings: [],
});

console.log('call database validation regression checks passed');
