'use client';

import { useMemo } from 'react';
import { useShallow } from 'zustand/react/shallow';
import {
  sanitizeAIChatContextText,
  usePageContextRegistration,
  type AIChatCapabilityDescriptor,
  type AIChatPageContextItem,
} from '@/components/aichat/page-context';
import type { AgentDetail } from '@/services';
import { WORKFLOW_KEYS } from '@/hooks/query-keys';
import { getAgentDetailEditHref } from '@/utils/agent-detail-routes';
import type {
  StoreValidationError,
  StoreValidationResults,
  WorkflowData,
  WorkflowDraftData,
  WorkflowEdge,
  WorkflowNode,
} from './store/type';
import { useWorkflowStore } from './store';
import { useActivePanel, type WorkflowActivePanel } from './hooks/use-active-panel';

const MAX_CONTEXT_NODES = 12;
const MAX_CONTEXT_EDGES = 16;
const MAX_VALIDATION_ISSUES = 6;
const MAX_FIELD_LENGTH = 720;

interface WorkflowAIChatContextRegistrationProps {
  agentDetail: AgentDetail;
  workflowDraft: WorkflowDraftData | undefined;
  viewNodes: WorkflowNode[];
  viewEdges: WorkflowEdge[];
  isDirty: boolean;
  isSaving: boolean;
  isPublishing: boolean;
  isReadOnly: boolean;
  isHistoryMode: boolean;
  isPermissionReadOnly: boolean;
  canPublish: boolean;
  selectedRunId: string | null;
}

interface WorkflowNodeSummary {
  id: string;
  type: string;
  title: string;
  selected: boolean;
  incomingCount: number;
  outgoingCount: number;
  runStatus?: string;
}

function compactText(value: string | null | undefined, maxLength = MAX_FIELD_LENGTH): string {
  const text = sanitizeAIChatContextText(value ?? '').replace(/\s+/g, ' ').trim();
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength).trim()}...`;
}

function getNodeType(node: WorkflowNode): string {
  const value = node.data?.type;
  return typeof value === 'string' && value.trim() ? value.trim() : 'unknown';
}

function getNodeTitle(node: WorkflowNode): string {
  const value = node.data?.title;
  if (typeof value === 'string' && value.trim()) return compactText(value, 120);
  return getNodeType(node);
}

function summarizeNodeTypes(nodes: WorkflowNode[]): string {
  const counts = new Map<string, number>();
  nodes.forEach(node => {
    const type = getNodeType(node);
    counts.set(type, (counts.get(type) ?? 0) + 1);
  });

  return Array.from(counts.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([type, count]) => `${type}=${count}`)
    .join(', ');
}

function summarizeNodes(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  selectedNodeId: string | null,
  runStatusByNodeId: Record<string, string>
): WorkflowNodeSummary[] {
  const incomingCount = new Map<string, number>();
  const outgoingCount = new Map<string, number>();

  edges.forEach(edge => {
    outgoingCount.set(edge.source, (outgoingCount.get(edge.source) ?? 0) + 1);
    incomingCount.set(edge.target, (incomingCount.get(edge.target) ?? 0) + 1);
  });

  return nodes.slice(0, MAX_CONTEXT_NODES).map(node => ({
    id: node.id,
    type: getNodeType(node),
    title: getNodeTitle(node),
    selected: selectedNodeId === node.id,
    incomingCount: incomingCount.get(node.id) ?? 0,
    outgoingCount: outgoingCount.get(node.id) ?? 0,
    runStatus: runStatusByNodeId[node.id],
  }));
}

function formatNodeSummaries(nodes: WorkflowNodeSummary[], totalCount: number): string {
  if (totalCount === 0) return 'No workflow nodes are currently loaded.';

  const visible = nodes
    .map(
      (node, index) =>
        `${index + 1}. ${node.title} (${node.type}, in=${node.incomingCount}, out=${
          node.outgoingCount
        }${node.selected ? ', selected' : ''}${node.runStatus ? `, run=${node.runStatus}` : ''})`
    )
    .join(' | ');
  const omitted = totalCount > nodes.length ? ` | omitted_nodes=${totalCount - nodes.length}` : '';

  return compactText(`${visible}${omitted}`, 1400);
}

function formatEdgeSummary(edges: WorkflowEdge[], nodesById: Map<string, WorkflowNode>): string {
  if (edges.length === 0) return 'No workflow edges are currently loaded.';

  const visible = edges
    .slice(0, MAX_CONTEXT_EDGES)
    .map(edge => {
      const source = nodesById.get(edge.source);
      const target = nodesById.get(edge.target);
      return `${getNodeTitle(source ?? ({ id: edge.source, data: {} } as WorkflowNode))} -> ${getNodeTitle(
        target ?? ({ id: edge.target, data: {} } as WorkflowNode)
      )}`;
    })
    .join(' | ');
  const omitted =
    edges.length > MAX_CONTEXT_EDGES ? ` | omitted_edges=${edges.length - MAX_CONTEXT_EDGES}` : '';

  return compactText(`${visible}${omitted}`, 1200);
}

function formatValidationIssue(issue: StoreValidationError): string {
  const node = issue.nodeTitle ? ` on ${issue.nodeTitle}` : issue.nodeId ? ` on ${issue.nodeId}` : '';
  return `${issue.type}:${issue.code}${node}`;
}

function formatValidationSummary(validationResults: StoreValidationResults): string {
  const issues = [...validationResults.errors, ...validationResults.warnings];
  if (issues.length === 0) return 'No validation errors or warnings are currently reported.';

  const visible = issues.slice(0, MAX_VALIDATION_ISSUES).map(formatValidationIssue).join(' | ');
  const omitted =
    issues.length > MAX_VALIDATION_ISSUES ? ` | omitted_issues=${issues.length - MAX_VALIDATION_ISSUES}` : '';

  return compactText(`${visible}${omitted}`, 1200);
}

function summarizeVariableNames(values: Array<{ name?: string; type?: string }>, limit = 12): string {
  const names = values
    .map(value => {
      const name = compactText(value.name, 80);
      if (!name) return '';
      return value.type ? `${name}:${value.type}` : name;
    })
    .filter(Boolean)
    .slice(0, limit);
  const omitted = values.length > names.length ? `, omitted=${values.length - names.length}` : '';

  return names.length > 0 ? `${names.join(', ')}${omitted}` : 'none';
}

function buildWorkflowCapabilities(params: {
  selectedNodeId: string | null;
  hasValidationIssues: boolean;
}): AIChatCapabilityDescriptor[] {
  return [
    {
      id: 'workflow.inspect_page',
      title: 'Inspect workflow page',
      description: 'Read the visible Workflow editor state and summarize the current page.',
      risk: 'low',
      status: 'available',
    },
    {
      id: 'workflow.summarize_graph',
      title: 'Summarize workflow graph',
      description: 'Summarize the draft graph using bounded node, edge, and validation metadata.',
      risk: 'low',
      status: 'available',
    },
    {
      id: 'workflow.inspect_selected_node',
      title: 'Inspect selected workflow node',
      description: 'Read the selected node title, type, connection counts, and validation state.',
      risk: 'low',
      status: params.selectedNodeId ? 'available' : 'unavailable',
    },
    {
      id: 'workflow.explain_validation',
      title: 'Explain workflow validation',
      description: 'Explain currently reported workflow validation errors and warnings.',
      risk: 'low',
      status: params.hasValidationIssues ? 'available' : 'unavailable',
    },
  ];
}

function buildWorkflowAIChatItems(params: {
  agentDetail: AgentDetail;
  workflowDraft: WorkflowDraftData | undefined;
  workflowData: WorkflowData;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  selectedNodeId: string | null;
  currentRunningNodeId: string | null;
  runStatusByNodeId: Record<string, string>;
  validationResults: StoreValidationResults;
  mode: 'edit' | 'history';
  activePanel: WorkflowActivePanel;
  isDirty: boolean;
  isSaving: boolean;
  isPublishing: boolean;
  isReadOnly: boolean;
  isHistoryMode: boolean;
  isPermissionReadOnly: boolean;
  canPublish: boolean;
  selectedRunId: string | null;
}): AIChatPageContextItem[] {
  const {
    agentDetail,
    workflowDraft,
    workflowData,
    nodes,
    edges,
    selectedNodeId,
    currentRunningNodeId,
    runStatusByNodeId,
    validationResults,
    mode,
    activePanel,
    isDirty,
    isSaving,
    isPublishing,
    isReadOnly,
    isHistoryMode,
    isPermissionReadOnly,
    canPublish,
    selectedRunId,
  } = params;
  const workflowResourceId = workflowDraft?.id || `${agentDetail.id}:workflow-draft`;
  const nodesById = new Map(nodes.map(node => [node.id, node]));
  const selectedNode = selectedNodeId ? nodesById.get(selectedNodeId) : undefined;
  const currentRunningNode = currentRunningNodeId ? nodesById.get(currentRunningNodeId) : undefined;
  const hasValidationIssues =
    validationResults.errors.length > 0 || validationResults.warnings.length > 0;
  const workflowCapabilities = buildWorkflowCapabilities({
    selectedNodeId,
    hasValidationIssues,
  });
  const nodeSummaries = summarizeNodes(nodes, edges, selectedNodeId, runStatusByNodeId);
  const status = isReadOnly ? 'readonly' : isDirty ? 'dirty' : 'draft';
  const workflowHref = getAgentDetailEditHref(agentDetail.id, agentDetail.agent_type);
  const includeDraftConfiguration = !isHistoryMode;
  const featureConfig = workflowData.features;
  const envVariables = workflowData.environment_variables ?? [];
  const conversationVariables = workflowData.conversation_variables ?? [];
  const envSecretCount = envVariables.filter(variable => variable.type === 'secret').length;

  const items: AIChatPageContextItem[] = [
    {
      id: `page:${workflowHref}`,
      type: 'page',
      title: `Workflow editor: ${agentDetail.name}`,
      subtitle: activePanel ? `Panel: ${activePanel}` : 'Workflow editor page',
      description: compactText(
        [
          'Current page is the Workflow editor. This context is read-only for AIChat.',
          'AIChat may answer questions about visible workflow state, but must not edit, run, publish, or save the workflow from this context.',
          `Mode=${mode}; read_only=${isReadOnly}; dirty=${isDirty}; draft_validation_passed=${canPublish}.`,
        ].join(' ')
      ),
      href: workflowHref,
      source: 'Workflow Editor',
      risk: 'low',
      status: isReadOnly ? 'readonly' : 'available',
      hints: {
        // The editor owns workflow assets but stays read-only for AIChat; do not
        // refresh draft graph state from successful external workflow mutations.
        handledAssetTypes: ['workflow', 'workflow_run'],
        refreshHints: [
          { assetType: 'workflow_run', queryKey: WORKFLOW_KEYS.runs(agentDetail.id) },
          { assetType: 'workflow_run', queryKey: WORKFLOW_KEYS.runDetails() },
          { assetType: 'workflow_run', queryKey: WORKFLOW_KEYS.executions() },
        ],
      },
      relations: [
        {
          type: 'shows_workflow',
          resourceType: 'workflow',
          resourceId: workflowResourceId,
          title: agentDetail.name,
        },
      ],
      capabilities: workflowCapabilities,
      metadata: {
        page_path: workflowHref,
        agent_id: agentDetail.id,
        workflow_id: workflowResourceId,
        workspace_id: agentDetail.workspace?.id,
        workspace_name: agentDetail.workspace?.name,
        active_panel: activePanel,
        mode,
        is_history_mode: isHistoryMode,
        is_permission_readonly: isPermissionReadOnly,
        is_dirty: isDirty,
        is_saving: isSaving,
        is_publishing: isPublishing,
        selected_run_id: selectedRunId,
      },
    },
    {
      id: agentDetail.id,
      type: 'workflow',
      title: agentDetail.name,
      subtitle: `${agentDetail.agent_type} in ${agentDetail.workspace?.name ?? 'workspace'}`,
      description: compactText(agentDetail.description || 'Workflow asset.'),
      href: getAgentDetailEditHref(agentDetail.id, agentDetail.agent_type),
      source: 'Workflow Editor',
      risk: 'low',
      status: agentDetail.is_published ? 'published' : 'draft',
      relations: [
        {
          type: 'shows_workflow',
          resourceType: 'workflow',
          resourceId: workflowResourceId,
          title: agentDetail.name,
        },
      ],
      capabilities: [
        {
          id: 'workflow.inspect_identity',
          title: 'Inspect Workflow identity',
          description: 'Read the Workflow identity for the current workflow draft.',
          risk: 'low',
          status: 'available',
        },
      ],
      metadata: {
        agent_id: agentDetail.id,
        agent_type: agentDetail.agent_type,
        workspace_id: agentDetail.workspace?.id,
        workspace_name: agentDetail.workspace?.name,
        can_edit_agent: agentDetail.can_edit,
        is_published: Boolean(agentDetail.is_published),
        web_app_status: agentDetail.web_app_status,
      },
    },
    {
      id: workflowResourceId,
      type: 'workflow',
      title: agentDetail.name,
      subtitle: `${nodes.length} nodes, ${edges.length} edges`,
      description: compactText(
        [
          `Node type inventory: ${summarizeNodeTypes(nodes) || 'none'}.`,
          `Visible node summary: ${formatNodeSummaries(nodeSummaries, nodes.length)}.`,
          `Visible edge summary: ${formatEdgeSummary(edges, nodesById)}.`,
          `Validation: ${
            isHistoryMode
              ? 'History snapshot mode; current draft validation is omitted to avoid mixing states.'
              : formatValidationSummary(validationResults)
          }.`,
          'Node configuration payloads, prompts, tool parameters, environment values, and secret values are intentionally omitted.',
        ].join(' '),
        3200
      ),
      href: workflowHref,
      source: 'Workflow Editor',
      risk: 'low',
      status,
      relations: [
        {
          type: 'belongs_to_agent',
          resourceType: 'agent',
          resourceId: agentDetail.id,
          title: agentDetail.name,
          metadata: {
            agent_type: agentDetail.agent_type,
            workspace_id: agentDetail.workspace?.id,
          },
        },
      ],
      capabilities: workflowCapabilities,
      metadata: {
        agent_id: agentDetail.id,
        workflow_draft_id: workflowDraft?.id,
        workflow_agent_id: workflowDraft?.agent_id,
        agent_type: agentDetail.agent_type,
        workspace_id: agentDetail.workspace?.id,
        node_count: nodes.length,
        edge_count: edges.length,
        selected_node_id: selectedNodeId,
        current_running_node_id: isHistoryMode ? undefined : currentRunningNodeId,
        validation_error_count: isHistoryMode ? undefined : validationResults.errors.length,
        validation_warning_count: isHistoryMode ? undefined : validationResults.warnings.length,
        draft_validation_passed: includeDraftConfiguration ? canPublish : undefined,
        mode,
        is_dirty: isDirty,
        is_readonly: isReadOnly,
        draft_file_upload_enabled: includeDraftConfiguration
          ? Boolean(featureConfig?.file_upload?.enabled)
          : undefined,
        draft_retriever_resource_enabled: includeDraftConfiguration
          ? Boolean(featureConfig?.retriever_resource?.enabled)
          : undefined,
        draft_conversation_history_enabled: includeDraftConfiguration
          ? Boolean(featureConfig?.conversation_history?.enabled)
          : undefined,
        draft_suggested_question_count: includeDraftConfiguration
          ? (featureConfig?.suggested_questions?.length ?? 0)
          : undefined,
        draft_environment_variable_count: includeDraftConfiguration ? envVariables.length : undefined,
        draft_environment_secret_count: includeDraftConfiguration ? envSecretCount : undefined,
        draft_conversation_variable_count: includeDraftConfiguration
          ? conversationVariables.length
          : undefined,
        draft_conversation_variable_names: includeDraftConfiguration
          ? summarizeVariableNames(conversationVariables)
          : undefined,
      },
    },
  ];

  if (selectedNode) {
    const selectedValidationIssues = isHistoryMode
      ? []
      : [
          ...(validationResults.errorMap.get(selectedNode.id) ?? []),
          ...(validationResults.warningMap.get(selectedNode.id) ?? []),
        ];
    items.push({
      id: `${workflowResourceId}:selected:${selectedNode.id}`,
      type: 'selection',
      title: `Selected node: ${getNodeTitle(selectedNode)}`,
      subtitle: getNodeType(selectedNode),
      description: compactText(
        [
          `Selected workflow node: ${getNodeTitle(selectedNode)} (${getNodeType(selectedNode)}).`,
          `Validation issues: ${
            isHistoryMode
              ? 'omitted in history snapshot mode'
              : selectedValidationIssues.length > 0
                ? selectedValidationIssues.map(formatValidationIssue).join(' | ')
                : 'none'
          }.`,
          'Detailed node configuration is intentionally omitted from AIChat page context.',
        ].join(' '),
        1200
      ),
      source: 'Workflow Editor',
      risk: 'low',
      status: 'available',
      relations: [
        {
          type: 'selected_in_workflow',
          resourceType: 'workflow',
          resourceId: workflowResourceId,
          title: agentDetail.name,
        },
      ],
      capabilities: [
        {
          id: 'workflow.inspect_selected_node',
          title: 'Inspect selected workflow node',
          description: 'Read selected node summary and validation state.',
          risk: 'low',
          status: 'available',
        },
      ],
      metadata: {
        workflow_id: workflowResourceId,
        agent_id: agentDetail.id,
        node_id: selectedNode.id,
        node_type: getNodeType(selectedNode),
        validation_issue_count: selectedValidationIssues.length,
        run_status: runStatusByNodeId[selectedNode.id],
      },
    });
  }

  if (currentRunningNode && !isHistoryMode) {
    items.push({
      id: `${workflowResourceId}:running:${currentRunningNode.id}`,
      type: 'log',
      title: `Current running node: ${getNodeTitle(currentRunningNode)}`,
      subtitle: getNodeType(currentRunningNode),
      description: compactText(
        `Workflow runtime is currently focused on ${getNodeTitle(currentRunningNode)} (${getNodeType(
          currentRunningNode
        )}). Runtime inputs and outputs are intentionally omitted.`
      ),
      source: 'Workflow Editor',
      risk: 'low',
      status: 'available',
      relations: [
        {
          type: 'runtime_focus_in_workflow',
          resourceType: 'workflow',
          resourceId: workflowResourceId,
          title: agentDetail.name,
        },
      ],
      metadata: {
        workflow_id: workflowResourceId,
        agent_id: agentDetail.id,
        node_id: currentRunningNode.id,
        node_type: getNodeType(currentRunningNode),
        run_status: runStatusByNodeId[currentRunningNode.id],
      },
    });
  }

  return items;
}

export function WorkflowAIChatContextRegistration({
  agentDetail,
  workflowDraft,
  viewNodes,
  viewEdges,
  isDirty,
  isSaving,
  isPublishing,
  isReadOnly,
  isHistoryMode,
  isPermissionReadOnly,
  canPublish,
  selectedRunId,
}: WorkflowAIChatContextRegistrationProps) {
  const activePanel = useActivePanel(state => state.active);
  const {
    selectedNodeId,
    currentRunningNodeId,
    runStatusByNodeId,
    validationResults,
    workflowData,
    mode,
  } = useWorkflowStore(
    useShallow(state => ({
      selectedNodeId: state.selectedNodeId,
      currentRunningNodeId: state.currentRunningNodeId,
      runStatusByNodeId: state.runStatusByNodeId,
      validationResults: state.validationResults,
      workflowData: state.workflowData,
      mode: state.mode,
    }))
  );

  const items = useMemo(
    () =>
      buildWorkflowAIChatItems({
        agentDetail,
        workflowDraft,
        workflowData,
        nodes: viewNodes,
        edges: viewEdges,
        selectedNodeId,
        currentRunningNodeId,
        runStatusByNodeId,
        validationResults,
        mode,
        activePanel,
        isDirty,
        isSaving,
        isPublishing,
        isReadOnly,
        isHistoryMode,
        isPermissionReadOnly,
        canPublish,
        selectedRunId,
      }),
    [
      activePanel,
      agentDetail,
      canPublish,
      currentRunningNodeId,
      isDirty,
      isHistoryMode,
      isPermissionReadOnly,
      isPublishing,
      isReadOnly,
      isSaving,
      mode,
      runStatusByNodeId,
      selectedNodeId,
      selectedRunId,
      validationResults,
      viewEdges,
      viewNodes,
      workflowData,
      workflowDraft,
    ]
  );

  usePageContextRegistration(items, {
    scopeId: `workflow-editor:${agentDetail.id}`,
    replace: true,
    visibility: 'current',
  });

  return null;
}
