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
    snippets: ['graph_search', 'fallback_policy'],
  },
  {
    name: 'batch graph retrieval configuration',
    file: 'src/components/datasets/batch-testing/index.tsx',
    snippets: ['graph_search', 'fallback_policy'],
  },
  {
    name: 'graph execution details',
    file: 'src/components/datasets/hit-testing/components/graph-execution-details.tsx',
    snippets: ['requested_method', 'actual_method', 'visibility_revision'],
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
    name: 'inherited graph embedding',
    file: 'src/components/datasets/indexing-config/graph-model-settings.tsx',
    snippets: ["t('graph.inheritedEmbeddingModel')", 'modelType="text-chat"'],
  },
  {
    name: 'simplified Chinese graph translations',
    file: 'src/i18n/modules/datasets/zh-Hans.ts',
    snippets: [
      "enableTitle: '启用知识图谱'",
      "inheritedEmbeddingModel: '继承的嵌入模型'",
      "knowledgeGraphTitle: '知识图谱'",
    ],
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
    snippets: ['next_cursor', 'graph_query_limit_exceeded', 'seed_node_id'],
  },
  {
    name: 'graph detail expansion',
    file: 'src/components/datasets/knowledge-graph/detail-panel.tsx',
    snippets: ['active_source_count', 'onExpandNeighbors'],
  },
];

for (const scenario of scenarios) {
  const source = read(scenario.file);
  for (const snippet of scenario.snippets) {
    if (!source.includes(snippet)) {
      throw new Error(`Missing ${scenario.name} snippet in ${scenario.file}: ${snippet}`);
    }
  }
}

console.log('Knowledge graph recovery baseline checks passed.');
