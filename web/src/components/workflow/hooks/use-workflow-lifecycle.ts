'use client';

import { useEffect, useMemo, useRef } from 'react';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { AgentType } from '@/services/types/agent';
import { useWorkflowStore } from '../store';
import type {
  WorkflowData,
  WorkflowNode,
  WorkflowEdge,
  LLMNodeData,
  ParameterExtractorNodeData,
  SqlGeneratorNodeData,
  WorkflowNodeData,
} from '../store/type';
import { initialWorkflowData } from '../store/initial-data';
import { NODE_THEMES } from '../nodes/custom/config';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useAvailableModels } from '@/hooks/model/use-model';
import { InputVarType } from '../types/input-var';
import {
  DEFAULT_CONVERSATIONAL_PROMPTS,
  DEFAULT_STANDARD_PROMPTS,
  getDefaultWorkflowPrompts,
} from '../nodes/llm/default-prompts';

interface UseWorkflowLifecycleParams {
  /** The agent ID */
  agentId: string;
  /** True when in history mode (viewing run snapshot) - skip loading draft */
  isHistoryMode: boolean;
  /** Workflow data fetched from server */
  workflowData: unknown;
  /** Agent type for determining the initial graph template */
  agentType: AgentType;
  /** Whether the parent is still fetching data */
  isLoading?: boolean;
}

/**
 * Unified hook that handles the complete workflow lifecycle:
 * 1. Bootstrap: Creates initial graph (Start -> LLM -> End/Answer) for empty workflows
 * 2. Initialize: Fills default models on nodes that lack them
 *
 * This consolidates `useWorkflowBootstrap` and `useWorkflowInitializer` into a single
 * cohesive hook with clear phase separation.
 */
export function useWorkflowLifecycle({
  agentId,
  isHistoryMode,
  workflowData,
  agentType,
  isLoading,
}: UseWorkflowLifecycleParams) {
  const t = useT('nodes');
  const { locale } = useLocale();

  // Store selectors
  const loadWorkflow = useWorkflowStore.use.loadWorkflow();
  const generateNodeId = useWorkflowStore.use.generateNodeId();
  const nodes = useWorkflowStore.use.nodes();
  const updateNodesData = useWorkflowStore.use.updateNodesData();

  // Default models
  const { value: defaultLlm } = useDefaultModelByUseCase('text-chat');
  const { models: availableLlmModels } = useAvailableModels({ use_case: 'text-chat' });
  const defaultLlmProvider = defaultLlm?.provider ?? '';
  const defaultLlmName = defaultLlm?.model ?? '';
  const defaultWorkflowPromptIds = useMemo(
    () => [
      DEFAULT_CONVERSATIONAL_PROMPTS.zh.promptId,
      DEFAULT_CONVERSATIONAL_PROMPTS.en.promptId,
      DEFAULT_STANDARD_PROMPTS.zh.promptId,
      DEFAULT_STANDARD_PROMPTS.en.promptId,
    ],
    []
  );

  // Refs for one-time initialization
  const hasBootstrappedRef = useRef(false);
  const hasInitializedModelsRef = useRef(false);
  const hasNormalizedDefaultPromptsRef = useRef(false);

  const { conversational: defaultConversationalPrompt, standard: defaultStandardPrompt } =
    getDefaultWorkflowPrompts(locale);

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 1: Bootstrap - Create initial graph for empty workflows
  // ─────────────────────────────────────────────────────────────────────────
  useEffect(() => {
    const st = useWorkflowStore.getState();
    const isIdMismatch = st.agentId !== agentId;

    // Skip in history mode
    if (isHistoryMode) return;

    // If ID mismatched, we MUST allow re-bootstrapping/loading even if ref is true
    if (!isIdMismatch && hasBootstrappedRef.current) return;

    // If data is missing and we are still loading, wait
    if (!workflowData) {
      if (isLoading) return;
      // If loading finished but data is still missing, we bootstrap an empty one
      bootstrapStandardWorkflow();
      hasBootstrappedRef.current = true;
      return;
    }

    const needsBootstrap =
      (workflowData as { features?: unknown; graph?: unknown }).features == null &&
      (workflowData as { features?: unknown; graph?: unknown }).graph == null;

    if (!needsBootstrap) {
      // Load existing workflow data
      const remoteHash = (workflowData as { hash?: string })?.hash ?? '';
      const localHash = st.workflowData?.hash ?? '';

      // A local graph is considered "empty" for the current context if it has no nodes
      // OR if it belongs to a different agent (stale data from previous mount)
      const isLocalEmpty = st.nodes.length === 0 || isIdMismatch;

      const hasRemoteGraph = Array.isArray(
        (workflowData as { graph?: { nodes?: unknown } } | null | undefined)?.graph?.nodes
      )
        ? (
            (workflowData as { graph?: { nodes?: unknown[] } } | null | undefined)?.graph
              ?.nodes as unknown[]
          ).length > 0
        : false;

      // We should perform a full load (reset local graph) if:
      // 1. Local graph is empty (fresh load)
      // 2. Remote hash exists and differs from local (server has newer/different version)
      if (isLocalEmpty || (remoteHash && remoteHash !== localHash && hasRemoteGraph)) {
        loadWorkflow(workflowData as WorkflowData, agentId, false);
      } else {
        // Just sync metadata/features without resetting graph
        loadWorkflow(workflowData as WorkflowData, agentId, true);
      }
      hasBootstrappedRef.current = true;
      return;
    }

    // Bootstrap new workflow based on agent type
    try {
      if (agentType === AgentType.CONVERSATIONAL_AGENT) {
        bootstrapConversationalWorkflow();
      } else if (agentType === AgentType.WORKFLOW) {
        bootstrapStandardWorkflow();
      } else {
        // Fallback: load as-is
        loadWorkflow(workflowData as WorkflowData, agentId, false);
      }
      hasBootstrappedRef.current = true;
    } catch (_e) {
      // Fallback on error
      loadWorkflow(workflowData as WorkflowData, agentId, false);
      hasBootstrappedRef.current = true;
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isHistoryMode, workflowData, agentType, agentId, isLoading]);

  // Helper: Create conversational agent workflow (Start -> LLM -> Answer)
  function bootstrapConversationalWorkflow() {
    const startId = generateNodeId();
    const llmId = generateNodeId();
    const answerId = generateNodeId();

    const startTheme = NODE_THEMES.start;
    const llmTheme = NODE_THEMES.llm;
    const answerTheme = NODE_THEMES.answer;

    const startNode: WorkflowNode = {
      id: startId,
      type: 'custom',
      position: { x: 180, y: 280 },
      width: typeof startTheme.width === 'number' ? startTheme.width : 243,
      height: typeof startTheme.height === 'number' ? startTheme.height : 48,
      data: {
        type: 'start' as const,
        title: safeT('catalog.start.title', 'Start'),
        desc: '',
        variables: [],
        unique: true,
      },
    };

    const llmNode: WorkflowNode = {
      id: llmId,
      type: 'custom',
      position: { x: 530, y: 280 },
      width: typeof llmTheme.width === 'number' ? llmTheme.width : 243,
      height: typeof llmTheme.height === 'number' ? llmTheme.height : 88,
      data: {
        type: 'llm' as const,
        title: safeT('catalog.llm.title', 'LLM'),
        desc: '',
        variables: [],
        model: {
          provider: defaultLlmProvider,
          name: defaultLlmName,
          mode: 'chat' as const,
          completion_params: {},
        },
        prompt_template: [
          {
            id: `${llmId}-system`,
            role: 'system' as const,
            text: defaultConversationalPrompt.text,
          },
          {
            id: `${llmId}-current-user`,
            role: 'user' as const,
            text: '{{#sys.query#}}',
            group_id: `${llmId}-current-user`,
            group_kind: 'current_user' as const,
          },
        ],
        prompt_layout: {
          version: 1 as const,
          items: [{ type: 'group' as const, group_id: `${llmId}-current-user` }],
        },
        prompt_source: 'inline',
        prompt_reference: undefined,
        prompt_config: { jinja2_variables: [] },
        conversation_history: { enabled: true, history_window_size: 3 },
        vision: { enabled: false },
        structured_output_enabled: false,
        isInLoop: false,
        isInIteration: false,
      },
    };

    const answerNode: WorkflowNode = {
      id: answerId,
      type: 'custom',
      position: { x: 880, y: 280 },
      width: typeof answerTheme.width === 'number' ? answerTheme.width : 243,
      height: typeof answerTheme.height === 'number' ? answerTheme.height : 88,
      data: {
        type: 'answer' as const,
        title: safeT('catalog.answer.title', 'Answer'),
        desc: '',
        variables: [],
        answer: `{{#${llmId}.text#}}`,
        isInLoop: false,
        isInIteration: false,
      },
    };

    const e1: WorkflowEdge = {
      id: `${startId}-${llmId}`,
      source: startId,
      target: llmId,
      sourceHandle: 'source' as const,
      targetHandle: 'target' as const,
      type: 'custom' as const,
      data: { isInLoop: false, sourceType: 'default', targetType: 'default' },
    };
    const e2: WorkflowEdge = {
      id: `${llmId}-${answerId}`,
      source: llmId,
      target: answerId,
      sourceHandle: 'source' as const,
      targetHandle: 'target' as const,
      type: 'custom' as const,
      data: { isInLoop: false, sourceType: 'default', targetType: 'default' },
    };

    const bootstrapped: WorkflowData = {
      graph: {
        nodes: [startNode, llmNode, answerNode],
        edges: [e1, e2],
        viewport: { x: 260, y: 125, zoom: 0.8 },
      },
      features: initialWorkflowData.features,
      environment_variables: [],
      conversation_variables: [],
      hash: '',
    } as WorkflowData;

    loadWorkflow(bootstrapped, agentId, false);
  }

  // Helper: Create standard workflow (Start -> LLM -> End)
  function bootstrapStandardWorkflow() {
    const startId = generateNodeId();
    const llmId = generateNodeId();
    const endId = generateNodeId();

    const startTheme = NODE_THEMES.start;
    const llmTheme = NODE_THEMES.llm;
    const endTheme = NODE_THEMES.end;

    const startNode: WorkflowNode = {
      id: startId,
      type: 'custom',
      position: { x: 180, y: 280 },
      width: typeof startTheme.width === 'number' ? startTheme.width : 243,
      height: typeof startTheme.height === 'number' ? startTheme.height : 48,
      data: {
        type: 'start' as const,
        title: safeT('catalog.start.title', '开始'),
        desc: '',
        unique: true,
        variables: [
          {
            type: InputVarType.PARAGRAPH,
            variable: 'input',
            label: safeT('start.inputVariableLabel', 'Input'),
            required: true,
          },
        ],
      },
    };

    const llmNode: WorkflowNode = {
      id: llmId,
      type: 'custom',
      position: { x: 530, y: 280 },
      width: typeof llmTheme.width === 'number' ? llmTheme.width : 243,
      height: typeof llmTheme.height === 'number' ? llmTheme.height : 88,
      data: {
        type: 'llm' as const,
        title: safeT('catalog.llm.title', 'LLM'),
        desc: '',
        variables: [],
        model: {
          provider: defaultLlmProvider,
          name: defaultLlmName,
          mode: 'chat' as const,
          completion_params: {},
        },
        prompt_template: [
          {
            id: `${llmId}-system`,
            role: 'system' as const,
            text: defaultStandardPrompt.text,
          },
          {
            id: `${llmId}-u1`,
            role: 'user' as const,
            text: `{{#${startId}.input#}}`,
            group_id: `${llmId}-current-user`,
            group_kind: 'current_user' as const,
          },
        ],
        prompt_layout: {
          version: 1 as const,
          items: [{ type: 'group' as const, group_id: `${llmId}-current-user` }],
        },
        prompt_source: 'inline',
        prompt_reference: undefined,
        prompt_config: { jinja2_variables: [] },
        conversation_history: { enabled: false, history_window_size: 3 },
        vision: { enabled: false },
        structured_output_enabled: false,
        isInLoop: false,
        isInIteration: false,
      },
    };

    const endNode: WorkflowNode = {
      id: endId,
      type: 'custom',
      position: { x: 880, y: 280 },
      width: typeof endTheme.width === 'number' ? endTheme.width : 243,
      height: typeof endTheme.height === 'number' ? endTheme.height : 48,
      data: {
        type: 'end' as const,
        title: safeT('catalog.end.title', '结束'),
        desc: '',
        outputs: [
          {
            variable: 'output',
            type: 'string',
            value_selector: [llmId, 'text'],
          },
        ],
        isInLoop: false,
        isInIteration: false,
      },
    };

    const e1: WorkflowEdge = {
      id: `${startId}-${llmId}`,
      source: startId,
      target: llmId,
      sourceHandle: 'source' as const,
      targetHandle: 'target' as const,
      type: 'custom' as const,
      data: { isInLoop: false, sourceType: 'default', targetType: 'default', isInIteration: false },
    };
    const e2: WorkflowEdge = {
      id: `${llmId}-${endId}`,
      source: llmId,
      target: endId,
      sourceHandle: 'source' as const,
      targetHandle: 'target' as const,
      type: 'custom' as const,
      data: { isInLoop: false, sourceType: 'default', targetType: 'default', isInIteration: false },
    };

    const bootstrapped: WorkflowData = {
      graph: {
        nodes: [startNode, llmNode, endNode],
        edges: [e1, e2],
        viewport: { x: 260, y: 125, zoom: 0.8 },
      },
      features: initialWorkflowData.features,
      environment_variables: [],
      conversation_variables: [],
      hash: '',
    } as WorkflowData;

    loadWorkflow(bootstrapped, agentId, false);
  }

  // Safe translation helper
  function safeT(key: string, fallback: string): string {
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return (t as any)(key) ?? fallback;
    } catch {
      return fallback;
    }
  }

  // Compatibility normalization: previously default workflow prompts were mounted as managed references.
  // They now behave as editable inline copies so users can apply and continue editing naturally.
  useEffect(() => {
    if (hasNormalizedDefaultPromptsRef.current) return;
    if (nodes.length === 0) return;

    const updates: Array<{ nodeId: string; data: Partial<WorkflowNodeData> }> = [];

    nodes.forEach(node => {
      if (node.data.type !== 'llm') return;
      const data = node.data as LLMNodeData;
      if (data.prompt_source !== 'managed') return;
      const promptID = data.prompt_reference?.prompt_id;
      if (!promptID || !defaultWorkflowPromptIds.includes(promptID)) return;

      updates.push({
        nodeId: node.id,
        data: {
          prompt_source: 'inline',
          prompt_reference: undefined,
        },
      });
    });

    if (updates.length > 0) {
      updateNodesData(updates, { markDirty: false, pushHistory: false });
    }

    hasNormalizedDefaultPromptsRef.current = true;
  }, [defaultWorkflowPromptIds, nodes, updateNodesData]);

  // ─────────────────────────────────────────────────────────────────────────
  // Phase 2: Initialize - Fill default models on nodes that lack them
  // ─────────────────────────────────────────────────────────────────────────
  useEffect(() => {
    // Only proceed if we have model data and haven't initialized yet
    if (!defaultLlm && availableLlmModels.length === 0) return;
    if (hasInitializedModelsRef.current) return;
    if (nodes.length === 0) return;

    const updates: Array<{ nodeId: string; data: Partial<WorkflowNodeData> }> = [];

    nodes.forEach(node => {
      const type = node.data.type;
      if (type === 'llm' || type === 'parameter-extractor') {
        const d = node.data as LLMNodeData | ParameterExtractorNodeData;
        if (!d.model?.name || !d.model?.provider) {
          // Skip for vision-enabled LLM nodes as they should use vision defaults
          if (type === 'llm' && (d as LLMNodeData).vision?.enabled) return;

          // Follow priority: Default -> First -> Clear
          const p = defaultLlm?.provider || (availableLlmModels[0]?.provider ?? '');
          const n = defaultLlm?.model || (availableLlmModels[0]?.model ?? '');

          if (p && n) {
            updates.push({
              nodeId: node.id,
              data: {
                model: {
                  provider: p,
                  name: n,
                  mode: d.model?.mode ?? 'chat',
                  completion_params: d.model?.completion_params ?? {},
                },
              },
            });
          }
        }
      } else if (type === 'sql-generator') {
        const d = node.data as SqlGeneratorNodeData;
        if (!d.data?.model?.name || !d.data?.model?.provider) {
          const p = defaultLlm?.provider || (availableLlmModels[0]?.provider ?? '');
          const n = defaultLlm?.model || (availableLlmModels[0]?.model ?? '');

          if (p && n) {
            updates.push({
              nodeId: node.id,
              data: {
                data: {
                  ...(d.data || {}),
                  model: {
                    provider: p,
                    name: n,
                    mode: d.data?.model?.mode ?? 'chat',
                    completion_params: d.data?.model?.completion_params ?? {},
                  },
                },
              } as Partial<SqlGeneratorNodeData>,
            });
          }
        }
      }
    });

    if (updates.length > 0) {
      updateNodesData(updates, { markDirty: false, pushHistory: false });
    }
    hasInitializedModelsRef.current = true;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultLlm, availableLlmModels, nodes.length, updateNodesData]);
}
