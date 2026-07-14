import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const draftPath = path.join(root, 'src/components/agents/agent-runtime/database-binding-draft.ts');

function loadDraftModule() {
  const source = readFileSync(draftPath, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
    fileName: draftPath,
  }).outputText;
  const testModule = new Module(draftPath);
  testModule.filename = draftPath;
  testModule.paths = Module._nodeModulePaths(path.dirname(draftPath));
  testModule._compile(output, draftPath);
  return testModule.exports;
}

const { normalizeAgentDatabaseBindings, planAgentDatabaseSelection } = loadDraftModule();

assert.deepEqual(
  normalizeAgentDatabaseBindings([
    {
      data_source_id: ' db-a ',
      table_ids: ['table-2', ''],
      writable_table_ids: ['table-2', 'missing'],
    },
    {
      data_source_id: 'db-a',
      table_ids: ['table-1'],
      writable_table_ids: ['table-1'],
    },
    {
      data_source_id: 'db-empty',
      table_ids: [],
      writable_table_ids: [],
    },
  ]),
  [
    {
      data_source_id: 'db-a',
      table_ids: ['table-1', 'table-2'],
      writable_table_ids: ['table-1', 'table-2'],
    },
  ],
  'Empty databases must never become runtime bindings.'
);

assert.deepEqual(
  planAgentDatabaseSelection(
    [
      {
        data_source_id: 'db-a',
        table_ids: ['table-1'],
        writable_table_ids: [],
      },
      {
        data_source_id: 'db-b',
        table_ids: ['table-2'],
        writable_table_ids: [],
      },
    ],
    ['db-a', 'db-c', 'db-empty', 'db-c']
  ),
  {
    initialBindings: [
      {
        data_source_id: 'db-a',
        table_ids: ['table-1'],
        writable_table_ids: [],
      },
    ],
    newDataSourceIds: ['db-c', 'db-empty'],
  },
  'The batch draft must keep selected bindings, stage removals, and preserve new database order.'
);

const sectionSource = readFileSync(
  path.join(root, 'src/components/agents/agent-runtime/sections/database-section.tsx'),
  'utf8'
);
const dialogSource = readFileSync(
  path.join(root, 'src/components/agents/agent-runtime/database-table-dialog.tsx'),
  'utf8'
);
const databaseSelectionSource = readFileSync(
  path.join(root, 'src/components/agents/agent-runtime/database-dialog.tsx'),
  'utf8'
);
const databaseOverviewSource = readFileSync(
  path.join(root, 'src/app/console/db/[dbId]/page.tsx'),
  'utf8'
);

assert.equal(
  sectionSource.includes('pendingTableDialogDbIds'),
  false,
  'The sequential table-dialog queue must not return.'
);
assert.equal(
  sectionSource.includes('useDbsBasic'),
  false,
  'The database section must not load the full database catalog just to hydrate selected bindings.'
);
assert.equal(
  sectionSource.includes('agentService.getAgentDatabaseBindingCandidates'),
  true,
  'The database section must hydrate persisted bindings through the Agent candidate endpoint.'
);
assert.equal(
  sectionSource.includes('tableDialogSession'),
  true,
  'Database table configuration must use one batch session.'
);
assert.equal(
  dialogSource.includes('databases.map(database =>'),
  true,
  'The table dialog must preserve navigation for every database in the batch.'
);
assert.equal(
  dialogSource.includes('enabled: open && Boolean(agentId) && Boolean(activeDataSourceId)'),
  true,
  'The table dialog must load tables only for the active database.'
);
assert.equal(
  dialogSource.includes('agentService.getAgentDatabaseTableBindingCandidates'),
  true,
  'The table dialog must use the Agent-scoped table candidate endpoint.'
);
assert.equal(
  dialogSource.includes('useQueries'),
  false,
  'The table dialog must not preload every selected database.'
);
assert.equal(
  dialogSource.includes('setActiveDataSourceId(databaseIds[0]'),
  true,
  'Opening a new batch must reset the active database.'
);
assert.equal(
  dialogSource.includes('normalizeAgentDatabaseBindings(localBindings)'),
  true,
  'The batch must commit a normalized binding draft once.'
);
assert.equal(
  databaseSelectionSource.includes('availableDatabaseIds'),
  false,
  'A partially loaded page must not remove selections that are not visible yet.'
);
assert.equal(
  databaseSelectionSource.includes('disabled={!hasTables && !selected}'),
  true,
  'A database with no tables must not be newly selectable, while persisted selections remain removable.'
);
assert.equal(
  databaseSelectionSource.includes('availableTablesCount'),
  true,
  'Database cards must display the server-provided table count.'
);
assert.equal(
  databaseSelectionSource.includes(
    'const [showAvailableOnly, setShowAvailableOnly] = useState(true)'
  ),
  true,
  'The database selector must enable the available-only filter by default.'
);
assert.equal(
  databaseSelectionSource.includes('available_only: showAvailableOnly'),
  true,
  'The available-only filter must be applied by the database list API.'
);
assert.equal(
  databaseSelectionSource.includes('agentService.getAgentDatabaseBindingCandidates'),
  true,
  'The database selector must use the Agent-scoped server-paginated candidate endpoint.'
);
assert.equal(
  databaseSelectionSource.includes('dbService.getDbsPage'),
  false,
  'The Agent database selector must not depend on the generic database catalog endpoint.'
);
assert.equal(
  databaseSelectionSource.includes('?createTable=1'),
  true,
  'Empty database cards must provide the direct create-table entry.'
);
assert.equal(
  databaseSelectionSource.includes("refetchOnWindowFocus: 'always'"),
  true,
  'Returning from table creation must refresh database availability.'
);
assert.equal(
  databaseOverviewSource.includes("searchParams.get('createTable') !== '1'"),
  true,
  'The database overview must consume the direct create-table request.'
);

console.log('Agent runtime database batch configuration checks passed.');
