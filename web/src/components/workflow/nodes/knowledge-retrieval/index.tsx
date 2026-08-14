import React from 'react';
import {
  getValidKnowledgeDatasetIds,
  type KnowledgeRetrievalNodeData,
} from './config';
import { Database, Network, Zap } from 'lucide-react';
import { useT } from '@/i18n';
import ValueBadge from '../../ui/value-badge';

export interface KnowledgeRetrievalContentProps {
  nodeId: string;
  data: KnowledgeRetrievalNodeData;
}

const KnowledgeRetrievalContent: React.FC<KnowledgeRetrievalContentProps> = ({ nodeId, data }) => {
  const t = useT('nodes');
  const datasetCount = getValidKnowledgeDatasetIds(data.dataset_ids).length;
  const topK =
    data.retrieval_mode === 'multiple' ? data.multiple_retrieval_config?.top_k : undefined;
  const graphRetrieval =
    data.retrieval_mode === 'multiple' &&
    data.multiple_retrieval_config?.search_method === 'graph_search';
  const querySelector =
    Array.isArray(data.query_variable_selector) && data.query_variable_selector.length >= 2
      ? (data.query_variable_selector as [string, string])
      : undefined;

  return (
    <div className="space-y-2">
      {querySelector ? (
        <div className="space-y-1">
          <div className="text-xs font-medium text-primary">
            {t('knowledgeRetrieval.queryVariable')}
          </div>
          <ValueBadge selector={querySelector} currentNodeId={nodeId} className="max-w-full" />
        </div>
      ) : null}

      {datasetCount > 0 || typeof topK === 'number' ? (
        <div className="flex flex-wrap gap-1.5">
          <div className="flex items-center gap-1 rounded-md border bg-background/70 px-2 py-1 text-xs text-muted-foreground">
            {graphRetrieval ? <Network className="size-3" /> : <Zap className="size-3" />}
            <span>
              {graphRetrieval
                ? t('knowledgeRetrieval.recall.graph.label')
                : t('knowledgeRetrieval.recall.vector.label')}
            </span>
          </div>
          {datasetCount > 0 ? (
            <div className="flex items-center gap-1 rounded-md border bg-background/70 px-2 py-1 text-xs text-muted-foreground">
              <Database className="size-3" />
              <span>
                {t('knowledgeRetrieval.datasets')}: {datasetCount}
              </span>
            </div>
          ) : null}
          {typeof topK === 'number' ? (
            <div className="rounded-md border bg-background/70 px-2 py-1 text-xs text-muted-foreground">
              {t('knowledgeRetrieval.recall.topK')}: {topK}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
};

export default KnowledgeRetrievalContent;
