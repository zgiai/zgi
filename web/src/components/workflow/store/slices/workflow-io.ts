// Workflow IO slice: load/save/reset workflow data with normalization
// Strict typing and reusing existing helpers

import type { WorkflowStore } from '../store';
import type {
  WorkflowData,
  WorkflowDraftData,
  WorkflowNode,
  WorkflowEdge,
  WorkflowNodeData,
  HttpRequestNodeData,
  HttpRequestBody,
  HttpRequestBodyDataItem,
  ConversationVariable,
} from '../type';
import type { EnvironmentVariable } from '../type';
import { normalizeToWorkflowData } from '../helpers/normalizers';
import { computeNodeIdToTitle, computeRunnableSets } from '../helpers/graph';
import { validateWorkflow } from '../helpers/validation-engine';
import { initialWorkflowData } from '../initial-data';
import { normalizeApprovalSourceHandle } from '../../nodes/approval/config';
import { useAuthStore } from '@/store/auth-store';

export interface WorkflowIOSlice {
  workflowData: WorkflowData;
  loadWorkflow: (
    data: WorkflowData | WorkflowDraftData,
    agentId: string,
    preserveDirtyState?: boolean
  ) => void;
  saveWorkflow: () => Promise<void>;
  resetWorkflow: () => void;
  // Update workflow-level features and mark draft as dirty
  updateWorkflowFeatures: (features: Partial<WorkflowData['features']>) => void;
  // Update conversation variables (draft-level config stored in workflowData)
  updateConversationVariables: (vars: ConversationVariable[]) => void;
  // Update environment variables
  updateEnvironmentVariables: (vars: EnvironmentVariable[]) => void;
}

export function createWorkflowIOSlice(
  set: (
    partial: Partial<WorkflowStore> | ((state: WorkflowStore) => Partial<WorkflowStore>),
    replace?: boolean,
    action?: string
  ) => void,
  get: () => WorkflowStore
): WorkflowIOSlice {
  return {
    workflowData: initialWorkflowData,

    loadWorkflow: (data, agentId, preserveDirtyState = false) => {
      // const start = performance.now();
      const workflowData = normalizeToWorkflowData(data);

      if (preserveDirtyState) {
        set(
          state => ({
            workflowData: {
              ...workflowData,
              graph: {
                nodes: state.nodes,
                edges: state.edges,
                viewport: state.viewport,
              },
            },
            agentId,
            isInitialized: true,
          }),
          false,
          'loadWorkflow'
        );
        get().syncRunnableSetsDebounced();
        return;
      }

      const validatedNodes = (workflowData.graph.nodes || [])
        .filter((node): node is WorkflowNode => !!node)
        .map(node => {
          const pos = (node as Partial<WorkflowNode>).position as
            | { x?: unknown; y?: unknown }
            | undefined;
          const hasValidPosition =
            !!pos &&
            typeof pos.x === 'number' &&
            typeof pos.y === 'number' &&
            Number.isFinite(pos.x) &&
            Number.isFinite(pos.y);

          let sanitizedData = node.data as WorkflowNodeData;
          const dataType = (sanitizedData as { type?: string })?.type;

          if (dataType === 'http-request') {
            const httpData = sanitizedData as HttpRequestNodeData & Record<string, unknown>;
            const incomingBody = (httpData as { body?: unknown }).body;

            let normalizedBody: HttpRequestBody = { type: 'none', data: [] };

            if (
              typeof incomingBody === 'object' &&
              incomingBody !== null &&
              'type' in (incomingBody as Record<string, unknown>) &&
              'data' in (incomingBody as Record<string, unknown>)
            ) {
              const b = incomingBody as { type: unknown; data: unknown };
              const type: HttpRequestNodeData['body']['type'] =
                b.type === 'json' ||
                b.type === 'raw-text' ||
                b.type === 'form-data' ||
                b.type === 'none'
                  ? (b.type as HttpRequestNodeData['body']['type'])
                  : 'none';

              const dataArr = Array.isArray(b.data) ? (b.data as unknown[]) : [];
              const sanitizedDataArr: HttpRequestBodyDataItem[] = dataArr.map((rawItem, idx) => {
                const item =
                  rawItem && typeof rawItem === 'object'
                    ? (rawItem as Record<string, unknown>)
                    : {};
                const base = {
                  id: typeof item.id === 'string' ? item.id : `key-value-${Date.now()}-${idx}`,
                  key: typeof item.key === 'string' ? item.key : '',
                  value: typeof item.value === 'string' ? item.value : '',
                };
                if (item.type === 'file') {
                  return {
                    ...base,
                    type: 'file',
                    file: Array.isArray(item.file)
                      ? item.file.filter((path): path is string => typeof path === 'string')
                      : [],
                  };
                }
                return { ...base, type: 'text' };
              });

              normalizedBody =
                type === 'none' ? { type: 'none', data: [] } : { type, data: sanitizedDataArr };
            }

            const normalizedParams = Array.isArray(httpData.params) ? httpData.params : [];
            const normalizedHeaders = Array.isArray(httpData.headers) ? httpData.headers : [];
            sanitizedData = {
              ...(httpData as unknown as WorkflowNodeData),
              params: normalizedParams,
              headers: normalizedHeaders,
              body: normalizedBody,
            } as WorkflowNodeData;
          }

          return {
            ...node,
            data: sanitizedData as WorkflowNode['data'],
            type:
              node.type ||
              (dataType === 'note' ||
              dataType === 'approval' ||
              dataType === 'announcement' ||
              dataType === 'question-answer'
                ? dataType
                : 'custom'),
            position: hasValidPosition ? (pos as { x: number; y: number }) : { x: 0, y: 0 },
          } as WorkflowNode;
        });

      const sanitizedNodes = validatedNodes;

      const validatedEdges = (workflowData.graph.edges || [])
        .filter((e): e is WorkflowEdge => !!e)
        .map(e => ({
          ...e,
          type: e.type || 'custom',
          sourceHandle: normalizeApprovalSourceHandle(e.sourceHandle) ?? 'source',
          targetHandle: e.targetHandle ?? 'target',
        }));

      const validatedViewport = workflowData.graph.viewport || { x: 0, y: 0, zoom: 1 };

      // Compute runnable sets and validation results BEFORE the set call
      // to batch everything into a single React render cycle
      const runnableSets = computeRunnableSets(sanitizedNodes, validatedEdges);
      const validationResults = validateWorkflow(
        sanitizedNodes,
        validatedEdges,
        get().agentType,
        runnableSets,
        useAuthStore.getState().systemFeatures
      );

      set(
        {
          workflowData,
          nodes: sanitizedNodes,
          edges: validatedEdges,
          viewport: validatedViewport,
          nodeIdToTitle: computeNodeIdToTitle(sanitizedNodes),
          runnableSets,
          validationResults,
          agentId,
          isInitialized: true,
          isDirty: false,
          hasLayoutChanges: false,
          suppressNextLayoutDirty: true,
          suppressNextViewportDirty: true,
          historyPast: [],
          historyFuture: [],
          graphVersion: get().graphVersion + 1,
        },
        false,
        'loadWorkflow'
      );

      // console.log(`[Workflow Performance] loadWorkflow took ${performance.now() - start}ms`);
    },

    updateWorkflowFeatures: features => {
      const prev = get().workflowData;
      const next: WorkflowData = {
        ...prev,
        features: { ...prev.features, ...features },
      } as WorkflowData;
      set(
        {
          workflowData: next,
          isDirty: true,
        },
        false,
        'updateWorkflowFeatures'
      );
    },

    updateConversationVariables: (vars: ConversationVariable[]) => {
      const prev = get().workflowData;
      const next: WorkflowData = {
        ...prev,
        conversation_variables: Array.isArray(vars) ? vars : [],
      } as WorkflowData;
      set(
        {
          workflowData: next,
          isDirty: true,
        },
        false,
        'updateConversationVariables'
      );
    },

    updateEnvironmentVariables: (vars: EnvironmentVariable[]) => {
      const prev = get().workflowData;
      const next: WorkflowData = {
        ...prev,
        environment_variables: Array.isArray(vars) ? vars : [],
      } as WorkflowData;
      set(
        {
          workflowData: next,
          isDirty: true,
        },
        false,
        'updateEnvironmentVariables'
      );
    },

    saveWorkflow: async () => {
      const { nodes, edges, viewport, workflowData } = get();
      const updatedWorkflowData: WorkflowData = {
        ...workflowData,
        graph: { nodes, edges, viewport },
      } as WorkflowData;
      set(
        {
          workflowData: updatedWorkflowData,
          isDirty: false,
          hasLayoutChanges: false,
          suppressNextLayoutDirty: false,
          suppressNextViewportDirty: false,
          lastSavedAt: Date.now(),
        },
        false,
        'saveWorkflow'
      );
      return Promise.resolve();
    },

    resetWorkflow: () => {
      set(
        {
          workflowData: initialWorkflowData,
          nodes: [],
          edges: [],
          viewport: { x: 0, y: 0, zoom: 1 },
          selectedNodeId: null,
          agentId: null,
          mode: 'edit',
          selectedRunId: null,
          isDirty: false,
          isInitialized: false,
          hasLayoutChanges: false,
          suppressNextLayoutDirty: false,
          suppressNextViewportDirty: false,
          historyPast: [],
          historyFuture: [],
        },
        false,
        'resetWorkflow'
      );
      get().syncRunnableSets();
    },
  };
}
