import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(scriptDirectory, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const scenarios = [
  {
    name: 'dataset creation entry',
    file: 'src/components/datasets/dialog/create-dataset-dialog.tsx',
    snippets: [
      'const graphFlowEnabled = formData.enable_graph_flow;',
      "t('datasets.createModal.enableGraphFlowLabel')",
      'checked={graphFlowEnabled}',
      'onCheckedChange={handleGraphFlowChange}',
    ],
  },
  {
    name: 'dataset graph status',
    file: 'src/services/types/dataset.ts',
    snippets: [
      'graph_indexing_status?: string;',
      'enable_graph_flow',
      'GraphRuntimeCapability',
      'GraphDatasetStatus',
    ],
  },
  {
    name: 'graph retrieval configuration',
    file: 'src/components/datasets/hit-testing/index.tsx',
    snippets: [
      'graph_search',
      'fallback_policy',
      'const retrievals: Array<Promise<boolean>>',
      'setVectorResults(response.data)',
      '.finally(() => setIsVectorSearching(false))',
      'isGraphVisibilityNotReadyError',
      'notice={graphPanelNotice}',
    ],
  },
  {
    name: 'graph visibility polling',
    file: 'src/hooks/dataset/use-dataset-graph.ts',
    snippets: ['refetchInterval: number | false = false', 'refetchIntervalInBackground: false'],
  },
  {
    name: 'graph retrieval expected-error handling',
    file: 'src/services/dataset.service.ts',
    snippets: ['retrieve/graph', 'skipErrorHandling: true'],
  },
  {
    name: 'batch graph retrieval configuration',
    file: 'src/components/datasets/batch-testing/index.tsx',
    snippets: ['graph_search', 'fallback_policy'],
  },
  {
    name: 'graph execution details',
    file: 'src/components/datasets/hit-testing/components/graph-execution-details.tsx',
    snippets: [
      "t('hitTesting.viewGraphDetails')",
      "t('hitTesting.hideGraphDetails')",
      "t('hitTesting.executionSteps')",
    ],
    forbiddenSnippets: [
      'execution.requested_method',
      'execution.actual_method',
      'execution.fallback_policy',
      'execution.graph_revision',
      'execution.visibility_revision',
    ],
  },
  {
    name: 'scrollable graph execution results',
    file: 'src/components/datasets/hit-testing/components/results-panel.tsx',
    snippets: [
      '<ScrollArea',
      'className="min-h-0 min-w-0 max-w-full flex-1"',
      '<GraphExecutionDetails execution={graphExecution} />',
      '{results.map((result, index)',
    ],
  },
  {
    name: 'graph result source document',
    file: 'src/components/datasets/hit-testing/components/graph-result-card.tsx',
    snippets: ['segment.document?.id', "t('hitTesting.sourceDocument')", 'segment.document.name'],
  },
  {
    name: 'graph visibility entry',
    file: 'src/app/console/dataset/[datasetId]/layout.tsx',
    snippets: ['enable_graph_flow', "t('datasets.knowledgeGraphTitle')", '/graph'],
  },
  {
    name: 'graph visibility refresh after save',
    file: 'src/hooks/dataset/use-datasets.ts',
    snippets: ['queryClient.setQueryData(DATASET_KEYS.detail(datasetId), response)'],
  },
  {
    name: 'graph extraction model',
    file: 'src/components/datasets/indexing-config/graph-model-settings.tsx',
    snippets: ['modelType="text-chat"'],
  },
  {
    name: 'simplified Chinese graph translations',
    file: 'src/i18n/modules/datasets/zh-Hans.ts',
    snippets: ["enableTitle: '启用知识图谱'", "knowledgeGraphTitle: '知识图谱'"],
  },
  {
    name: 'graph rebuild confirmation',
    file: 'src/app/console/dataset/[datasetId]/settings/page.tsx',
    snippets: ['confirm_graph_rebuild', 'graph_model_change_confirmation_required'],
  },
  {
    name: 'graph controls',
    file: 'src/hooks/dataset/use-dataset-graph.ts',
    snippets: ['useDatasetGraphStatus', 'useRebuildDatasetGraph', 'useRetryDocumentGraph'],
  },
  {
    name: 'bounded graph query types',
    file: 'src/services/types/dataset.ts',
    snippets: ['GraphQueryParams', 'next_cursor', 'active_source_count'],
  },
  {
    name: 'graph page query states',
    file: 'src/app/console/dataset/[datasetId]/graph/page.tsx',
    snippets: [
      'useDatasetGraphStatus(',
      '2_000',
      'node_limit:',
      'edge_limit:',
      'graph_query_limit_exceeded',
      'seed_node_id',
      "graphStatus?.status === 'waiting_content'",
      "t('graph.emptyStatusDescription')",
    ],
  },
  {
    name: 'graph detail expansion',
    file: 'src/components/datasets/knowledge-graph/detail-panel.tsx',
    snippets: [
      'active_source_count',
      'onExpandNeighbors',
      "src.doc.id.replace(/^doc:/, '')",
      '/documents/${encodeURIComponent(documentId)}',
      "tDatasets('documents.fileRefs.openFile')",
      'sourceDocumentReturnTo',
      'restoreSourceDocumentsPosition',
    ],
  },
  {
    name: 'graph source document return state',
    file: 'src/app/console/dataset/[datasetId]/graph/page.tsx',
    snippets: [
      "const GRAPH_SELECTED_ENTITY_PARAM = 'selected_entity'",
      "const GRAPH_EXPLORATION_ROOT_PARAM = 'exploration_root'",
      "const GRAPH_DETAIL_SECTION_PARAM = 'detail_section'",
      'sourceDocumentReturnTo={sourceDocumentReturnTo}',
    ],
  },
  {
    name: 'dataset document return target forwarding',
    file: 'src/app/console/dataset/[datasetId]/documents/[documentId]/page.tsx',
    snippets: [
      'safeDatasetReturnTo',
      "searchParams.get('returnTo')",
      'returnTo=${encodeURIComponent(returnTo)}',
      'router.push(returnTo)',
    ],
  },
];

for (const scenario of scenarios) {
  const source = read(scenario.file);
  for (const snippet of scenario.snippets) {
    if (!source.includes(snippet)) {
      throw new Error(`Missing ${scenario.name} snippet in ${scenario.file}: ${snippet}`);
    }
  }
  for (const snippet of scenario.forbiddenSnippets ?? []) {
    if (source.includes(snippet)) {
      throw new Error(`Unexpected ${scenario.name} snippet in ${scenario.file}: ${snippet}`);
    }
  }
}

const hitTestingSource = read('src/components/datasets/hit-testing/index.tsx');
if (
  !/const vectorRequestData = \{[\s\S]*?record_history: recordHistory,[\s\S]*?\};/.test(
    hitTestingSource
  ) ||
  !/const graphRequestData = \{[\s\S]*?record_history: false,[\s\S]*?\};/.test(hitTestingSource)
) {
  throw new Error('Combined retrieval must record history only for the hybrid request.');
}

console.log('Knowledge graph recovery baseline checks passed.');
