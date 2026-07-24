import type { WorkflowNode } from '@/components/workflow/store/type';
import type { WorkflowPrecheckResult, WorkflowPrecheckWarning } from '@/services/types/workflow';

const PRECHECK_NODE_TYPES = new Set([
  'llm',
  'knowledge-retrieval',
  'parameter-extractor',
  'sql-generator',
  'image-gen',
]);

interface WorkflowPrecheckNodeData {
  type?: string;
  model?: unknown;
  single_retrieval_config?: unknown;
  metadata_model_config?: unknown;
  multiple_retrieval_config?: unknown;
}

interface WorkflowPrecheckGraphNode {
  id: string;
  data: WorkflowPrecheckNodeData;
}

type UnknownRecord = Record<string, unknown>;

function asRecord(value: unknown): UnknownRecord | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined;
  }
  return value as UnknownRecord;
}

function modelRoute(value: unknown): unknown {
  const model = asRecord(value);
  if (!model) {
    return value;
  }

  const route: UnknownRecord = {};
  if (model.provider !== undefined) {
    route.provider = model.provider;
  }
  if (model.name !== undefined) {
    route.name = model.name;
  } else if (model.model !== undefined) {
    route.name = model.model;
  }
  return route;
}

function multipleRetrievalRoutes(value: unknown): unknown {
  const config = asRecord(value);
  if (!config) {
    return value;
  }

  const routes: UnknownRecord = {};
  if (config.reranking_model !== undefined) {
    routes.reranking_model = modelRoute(config.reranking_model);
  }

  const weights = asRecord(config.weights);
  const vectorSetting = asRecord(weights?.vector_setting);
  if (vectorSetting) {
    routes.weights = {
      vector_setting: {
        embedding_provider_name: vectorSetting.embedding_provider_name,
        embedding_model_name: vectorSetting.embedding_model_name,
      },
    };
  }
  return routes;
}

export function workflowPrecheckGraphNodes(nodes: WorkflowNode[]): WorkflowPrecheckGraphNode[] {
  return nodes.flatMap(node => {
    const data = node.data as WorkflowPrecheckNodeData;
    if (!data.type || !PRECHECK_NODE_TYPES.has(data.type)) {
      return [];
    }

    return [
      {
        id: node.id,
        data: {
          type: data.type,
          ...(data.model === undefined ? {} : { model: modelRoute(data.model) }),
          ...(data.single_retrieval_config === undefined
            ? {}
            : { single_retrieval_config: modelRoute(data.single_retrieval_config) }),
          ...(data.metadata_model_config === undefined
            ? {}
            : { metadata_model_config: modelRoute(data.metadata_model_config) }),
          ...(data.multiple_retrieval_config === undefined
            ? {}
            : {
                multiple_retrieval_config: multipleRetrievalRoutes(data.multiple_retrieval_config),
              }),
        },
      },
    ];
  });
}

export function workflowPrecheckModelFingerprint(nodes: WorkflowNode[]): string {
  return JSON.stringify(workflowPrecheckGraphNodes(nodes));
}

export async function loadAdvisoryWorkflowPrecheckWarnings(
  precheck: () => Promise<WorkflowPrecheckResult>
): Promise<WorkflowPrecheckWarning[]> {
  try {
    const result = await precheck();
    if (result.status !== 'warning') {
      return [];
    }
    if (Array.isArray(result.warnings)) {
      return result.warnings.filter(Boolean);
    }
    return result.warning ? [result.warning] : [];
  } catch {
    return [];
  }
}
